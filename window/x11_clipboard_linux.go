//go:build linux && !android

package window

// x11_clipboard_linux.go — буфер обмена X11 своими силами, без xclip и xsel.
//
// Прежний путь запускал подпроцесс: утилиты может не быть в системе вовсе
// (в пакете WinLine её и нет), запуск стоит дороже самой операции, а
// завершающий перевод строки по дороге терялся. X11 не хранит буфер обмена
// где-то в сервере — его держит окно-владелец и отдаёт по запросу, так что
// всё, что нужно, клиент умеет сам.
//
// Копирование: объявляем себя владельцем CLIPBOARD и отвечаем на
// SelectionRequest — кладём текст в свойство окна-просителя и сообщаем ему
// SelectionNotify.
//
// Оформленный текст (SetHTML) отдаётся тем же владением выделения: в списке
// TARGETS рядом с текстовыми целями появляется text/html, а на запрос этой цели
// отвечаем разметкой. Проситель выбирает сам: Блокнот спросит текст, Word или
// браузер — HTML.
//
// Вставка: просим сервер сконвертировать CLIPBOARD в UTF8_STRING; ответ
// приходит событием в цикле событий, поэтому GetText ждёт его по каналу с
// коротким тайм-аутом. Владелец — чужой процесс и вправе не ответить вовсе.

import (
	"encoding/binary"
	"sync"
	"time"
)

// x11ClipboardTimeout — сколько ждём ответ владельца буфера.
const x11ClipboardTimeout = 300 * time.Millisecond

// x11Clipboard — буфер обмена одного X11-соединения.
type x11Clipboard struct {
	w *X11Window

	mu    sync.Mutex
	owned string // наш текст, пока владеем CLIPBOARD
	// ownedHTML — оформленная версия нашего содержимого; пусто — только текст.
	ownedHTML string
	owns      bool        // владеем ли мы буфером
	wait      chan string // ждёт ответа на ConvertSelection; nil — не ждём
}

func newX11Clipboard(w *X11Window) *x11Clipboard { return &x11Clipboard{w: w} }

// SetText объявляет нас владельцем буфера обмена.
//
// Реализует widget.ClipboardProvider.
func (c *x11Clipboard) SetText(s string) { c.setOwned(s, "") }

// SetHTML объявляет нас владельцем буфера и готовит к выдаче и текст, и HTML.
//
// Реализует widget.ClipboardHTMLProvider.
func (c *x11Clipboard) SetHTML(html, plain string) { c.setOwned(plain, html) }

// setOwned запоминает содержимое и становится владельцем CLIPBOARD. Прежнее
// оформление затирается всегда: иначе после SetText проситель text/html
// получил бы разметку от прошлого копирования.
func (c *x11Clipboard) setOwned(text, html string) {
	if c == nil || c.w == nil || c.w.wid == 0 || c.w.atomClipboard == 0 {
		return
	}
	c.mu.Lock()
	c.owned, c.ownedHTML, c.owns = text, html, true
	c.mu.Unlock()
	c.w.x11SetSelectionOwner(c.w.atomClipboard, c.w.wid)
}

// GetText читает текст из буфера обмена.
func (c *x11Clipboard) GetText() string {
	if c == nil || c.w == nil || c.w.wid == 0 || c.w.atomClipboard == 0 {
		return ""
	}
	c.mu.Lock()
	if c.owns {
		s := c.owned
		c.mu.Unlock()
		return s // владеем сами — незачем ходить через сервер
	}
	c.mu.Unlock()
	return c.convert(c.w.atomUTF8String)
}

// GetHTML читает оформленный текст из буфера обмена.
//
// Реализует widget.ClipboardHTMLProvider. Запрашиваем text/html напрямую: если
// у владельца такой цели нет, сервер вернёт отказ (свойство None), и мы честно
// скажем «HTML нет» — вызывающий возьмёт простой текст через GetText.
func (c *x11Clipboard) GetHTML() (string, bool) {
	if c == nil || c.w == nil || c.w.wid == 0 || c.w.atomClipboard == 0 || c.w.atomTextHTML == 0 {
		return "", false
	}
	c.mu.Lock()
	if c.owns {
		html := c.ownedHTML
		c.mu.Unlock()
		return html, html != ""
	}
	c.mu.Unlock()
	raw := c.convert(c.w.atomTextHTML)
	if raw == "" {
		return "", false
	}
	html := clipboardHTMLToString([]byte(raw))
	return html, html != ""
}

// convert просит сервер сконвертировать CLIPBOARD в указанную цель и ждёт
// ответ с тайм-аутом. Пустая строка — отказ, молчание владельца или пустой буфер.
func (c *x11Clipboard) convert(target uint32) string {
	c.mu.Lock()
	if c.wait != nil {
		c.mu.Unlock()
		return "" // запрос уже в пути; второй ответ всё равно будет один
	}
	ch := make(chan string, 1)
	c.wait = ch
	c.mu.Unlock()

	// Ответ придёт событием SelectionNotify в цикл событий — он и разбудит
	// этот канал (см. handleNotify).
	c.w.x11ConvertSelection(c.w.atomClipboard, target, c.w.atomClipProp, 0)

	select {
	case s := <-ch:
		return s
	case <-time.After(x11ClipboardTimeout):
		c.mu.Lock()
		c.wait = nil
		c.mu.Unlock()
		return "" // владелец молчит: буфер обмена не повод вешать интерфейс
	}
}

// handleNotify разбирает SelectionNotify, адресованный буферу обмена.
// Возвращает false, если событие не наше (тогда его разберёт XDND).
func (c *x11Clipboard) handleNotify(buf []byte) bool {
	if c == nil || c.w == nil {
		return false
	}
	selection := binary.LittleEndian.Uint32(buf[12:16])
	if selection != c.w.atomClipboard {
		return false
	}
	property := binary.LittleEndian.Uint32(buf[20:24]) // None(0) — конверсия не удалась

	text := ""
	if property != 0 {
		_, _, data := c.w.x11GetProperty(c.w.wid, property, true)
		text = string(data)
	}
	c.mu.Lock()
	ch := c.wait
	c.wait = nil
	c.mu.Unlock()
	if ch != nil {
		ch <- text
	}
	return true
}

// handleRequest отвечает на SelectionRequest: кладёт данные в свойство окна-
// просителя и сообщает ему SelectionNotify.
//
// Отказ выражается свойством None в ответе — так проситель поймёт, что
// такого формата у нас нет, и спросит другой.
func (c *x11Clipboard) handleRequest(buf []byte) {
	if c == nil || c.w == nil {
		return
	}
	// SelectionRequest: time, owner, requestor, selection, target, property
	timestamp := binary.LittleEndian.Uint32(buf[4:8])
	requestor := binary.LittleEndian.Uint32(buf[12:16])
	selection := binary.LittleEndian.Uint32(buf[16:20])
	target := binary.LittleEndian.Uint32(buf[20:24])
	property := binary.LittleEndian.Uint32(buf[24:28])
	if property == 0 {
		property = target // древние клиенты шлют None, подразумевая target
	}

	c.mu.Lock()
	text, html, owns := c.owned, c.ownedHTML, c.owns
	c.mu.Unlock()

	ok := false
	switch {
	case selection != c.w.atomClipboard || !owns:
		// Не наш буфер — отвечаем отказом.
	case target == c.w.atomTargets:
		// Список форматов, которые мы умеем отдать.
		data := make([]byte, 0, 20)
		targets := []uint32{c.w.atomTargets, c.w.atomUTF8String, 31 /*STRING*/, c.w.atomText}
		if html != "" && c.w.atomTextHTML != 0 {
			targets = append(targets, c.w.atomTextHTML)
		}
		for _, a := range targets {
			var b [4]byte
			binary.LittleEndian.PutUint32(b[:], a)
			data = append(data, b[:]...)
		}
		c.w.x11ChangeProperty(requestor, property, 4 /*ATOM*/, 32, data)
		ok = true
	case target == c.w.atomUTF8String || target == 31 /*STRING*/ || target == c.w.atomText:
		c.w.x11ChangeProperty(requestor, property, target, 8, []byte(text))
		ok = true
	case html != "" && c.w.atomTextHTML != 0 && target == c.w.atomTextHTML:
		c.w.x11ChangeProperty(requestor, property, target, 8, []byte(html))
		ok = true
	}
	if !ok {
		property = 0
	}
	c.w.x11SendSelectionNotify(requestor, timestamp, selection, target, property)
}

// handleClear — буфером завладел кто-то другой.
func (c *x11Clipboard) handleClear(buf []byte) {
	if c == nil {
		return
	}
	selection := binary.LittleEndian.Uint32(buf[12:16])
	if selection != c.w.atomClipboard {
		return
	}
	c.mu.Lock()
	c.owns, c.owned, c.ownedHTML = false, "", ""
	c.mu.Unlock()
}

// ─── Запросы протокола ──────────────────────────────────────────────────────

// x11SetSelectionOwner объявляет окно владельцем выделения (opcode 22).
func (w *X11Window) x11SetSelectionOwner(selection, owner uint32) {
	buf := make([]byte, 16)
	buf[0] = 22
	binary.LittleEndian.PutUint16(buf[2:4], 4)
	binary.LittleEndian.PutUint32(buf[4:8], owner)
	binary.LittleEndian.PutUint32(buf[8:12], selection)
	binary.LittleEndian.PutUint32(buf[12:16], 0) // CurrentTime
	w.x11Send(buf)
}

// x11SendSelectionNotify отправляет просителю ответ на SelectionRequest
// (SendEvent, opcode 25; тип события 31).
func (w *X11Window) x11SendSelectionNotify(requestor, timestamp, selection, target, property uint32) {
	ev := make([]byte, 32)
	ev[0] = 31 // SelectionNotify
	binary.LittleEndian.PutUint32(ev[4:8], timestamp)
	binary.LittleEndian.PutUint32(ev[8:12], requestor)
	binary.LittleEndian.PutUint32(ev[12:16], selection)
	binary.LittleEndian.PutUint32(ev[16:20], target)
	binary.LittleEndian.PutUint32(ev[20:24], property)

	buf := make([]byte, 44)
	buf[0] = 25 // SendEvent
	buf[1] = 0  // propagate = false
	binary.LittleEndian.PutUint16(buf[2:4], 11)
	binary.LittleEndian.PutUint32(buf[4:8], requestor)
	binary.LittleEndian.PutUint32(buf[8:12], 0) // event-mask = 0 (адресно)
	copy(buf[12:44], ev)
	w.x11Send(buf)
}
