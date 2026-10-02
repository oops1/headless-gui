//go:build linux && !android

package window

// x11_cursor_linux.go — формы курсора под X11.
//
// Курсор над полем ввода, над краем окна и над ссылкой должен выглядеть
// по-разному — это первое, по чему человек понимает, что здесь можно делать.
// Win32 и Wayland это умели, X11 не умел вовсе: окно всегда показывало
// стрелку, и край окна ничем не отличался от середины.
//
// Берём курсоры из шрифта «cursor» — он есть в любом X-сервере с незапамятных
// времён, не требует ни темы, ни библиотеки Xcursor, ни внешних файлов. Это
// тот же набор, которым пользуются простые тулкиты.

import (
	"encoding/binary"
	"sync"
)

// Номера глифов в шрифте «cursor» (X11 cursorfont.h). Каждому курсору там
// отведены два глифа подряд: сам рисунок и маска прозрачности.
const (
	xcLeftPtr        = 68  // обычная стрелка
	xcXterm          = 152 // текстовая каретка
	xcHand2          = 60  // «рука» над ссылкой
	xcSbHDoubleArrow = 108 // ↔
	xcSbVDoubleArrow = 116 // ↕
	xcTopLeftCorner  = 134 // ⤡ — угол northwest↔southeast
	xcTopRightCorner = 136 // ⤢ — угол northeast↔southwest
)

// x11Cursors — формы курсора, созданные по требованию.
//
// Каждая форма — отдельный ресурс X-сервера, и создавать её заново на каждое
// движение мыши нельзя: курсор меняется сотни раз за минуту.
type x11Cursors struct {
	mu    sync.Mutex
	font  uint32         // шрифт «cursor», 0 — ещё не открыт
	byID  map[int]uint32 // форма движка → ресурс курсора
	shown int            // что показано сейчас
	known bool           // показывали ли хоть раз
}

// SetCursor задаёт форму курсора окна (значения widget.Cursor).
//
// Реализует необязательный интерфейс, который опрашивает window.surface:
// бэкенд без него оставляет курсор системным, как было до сих пор.
func (w *X11Window) SetCursor(c int) {
	if w.wid == 0 {
		return
	}
	w.cursors.mu.Lock()
	if w.cursors.known && w.cursors.shown == c {
		w.cursors.mu.Unlock()
		return // та же форма — незачем беспокоить сервер
	}
	w.cursors.shown, w.cursors.known = c, true
	id := w.cursorResourceLocked(c)
	w.cursors.mu.Unlock()
	if id == 0 {
		return
	}
	w.x11SetWindowCursor(w.wid, id)
}

// cursorResourceLocked возвращает ресурс курсора, создавая его при первом
// обращении. Вызывать под w.cursors.mu.
func (w *X11Window) cursorResourceLocked(c int) uint32 {
	if id, ok := w.cursors.byID[c]; ok {
		return id
	}
	if w.cursors.font == 0 {
		w.cursors.font = w.x11GenID()
		w.x11OpenFont(w.cursors.font, "cursor")
	}
	glyph, ok := x11CursorGlyph(c)
	if !ok {
		return 0
	}
	id := w.x11GenID()
	w.x11CreateGlyphCursor(id, w.cursors.font, glyph)
	if w.cursors.byID == nil {
		w.cursors.byID = map[int]uint32{}
	}
	w.cursors.byID[c] = id
	return id
}

// x11CursorGlyph переводит форму движка в глиф шрифта «cursor».
//
// Диагональных «размеров» в этом шрифте нет — там углы рамки, и это ровно то
// же самое по смыслу: курсор угла окна.
func x11CursorGlyph(c int) (int, bool) {
	switch c {
	case wlCursorArrow:
		return xcLeftPtr, true
	case wlCursorIBeam:
		return xcXterm, true
	case wlCursorHand:
		return xcHand2, true
	case wlCursorSizeWE:
		return xcSbHDoubleArrow, true
	case wlCursorSizeNS:
		return xcSbVDoubleArrow, true
	case wlCursorSizeNWSE:
		return xcTopLeftCorner, true
	case wlCursorSizeNESW:
		return xcTopRightCorner, true
	}
	return 0, false
}

// ─── Запросы протокола ──────────────────────────────────────────────────────

// x11OpenFont открывает шрифт по имени (opcode 45).
func (w *X11Window) x11OpenFont(fid uint32, name string) {
	pad := (4 - len(name)%4) % 4
	buf := make([]byte, 12+len(name)+pad)
	buf[0] = 45
	binary.LittleEndian.PutUint16(buf[2:4], uint16(len(buf)/4))
	binary.LittleEndian.PutUint32(buf[4:8], fid)
	binary.LittleEndian.PutUint16(buf[8:10], uint16(len(name)))
	copy(buf[12:], name)
	w.x11Send(buf)
}

// x11CreateGlyphCursor создаёт курсор из пары глифов шрифта (opcode 94).
//
// Маска — следующий глиф за рисунком: так устроен шрифт «cursor», где формы
// идут парами. Цвета заданы чёрным по белому — тем же, чем рисует курсоры
// сам X-сервер.
func (w *X11Window) x11CreateGlyphCursor(cid, font uint32, glyph int) {
	buf := make([]byte, 32)
	buf[0] = 94
	binary.LittleEndian.PutUint16(buf[2:4], 8)
	binary.LittleEndian.PutUint32(buf[4:8], cid)
	binary.LittleEndian.PutUint32(buf[8:12], font)  // source-font
	binary.LittleEndian.PutUint32(buf[12:16], font) // mask-font
	binary.LittleEndian.PutUint16(buf[16:18], uint16(glyph))
	binary.LittleEndian.PutUint16(buf[18:20], uint16(glyph+1))
	// fore-red/green/blue = 0 (чёрный), back-* = 0xFFFF (белый)
	binary.LittleEndian.PutUint16(buf[26:28], 0xFFFF)
	binary.LittleEndian.PutUint16(buf[28:30], 0xFFFF)
	binary.LittleEndian.PutUint16(buf[30:32], 0xFFFF)
	w.x11Send(buf)
}

// x11SetWindowCursor вешает курсор на окно (ChangeWindowAttributes, opcode 2,
// маска CWCursor).
func (w *X11Window) x11SetWindowCursor(wid, cursor uint32) {
	const cwCursor = 0x00004000
	buf := make([]byte, 16)
	buf[0] = 2
	binary.LittleEndian.PutUint16(buf[2:4], 4)
	binary.LittleEndian.PutUint32(buf[4:8], wid)
	binary.LittleEndian.PutUint32(buf[8:12], cwCursor)
	binary.LittleEndian.PutUint32(buf[12:16], cursor)
	w.x11Send(buf)
}
