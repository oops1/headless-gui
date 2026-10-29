//go:build linux && !android

package core

// x11_clipboard_formats_linux.go — какие виды содержимого лежат в буфере
// обмена X11 (widget.ClipboardFormats), без чтения данных и без долгого
// ожидания чужого владельца.
//
// GetFiles на X11 спрашивает у владельца сам список и ждёт его до десяти минут
// (удалённый рабочий стол отдаёт файлы, когда они скачаны). Для вопроса «есть
// ли файлы» это не годится, поэтому здесь спрашивается только TARGETS — список
// целей, которые владелец умеет отдать, — и с коротким сроком (250 мс). Владелец
// отвечает на TARGETS сразу, не читая данные, так что честный отвечает за
// миллисекунды, а молчащий стоит не дольше срока.
//
// Ответ запоминается и пересчитывается только при смене владельца. Смену
// сообщает расширение XFixes: событие SelectionNotify (не путать с ядерным
// SelectionNotify — это ответ на ConvertSelection) приходит, когда кто-то
// объявил себя владельцем CLIPBOARD, и сбрасывает память. Подписка делается
// лениво, при первом вопросе, и в цикле событий окна — как и остальные запросы
// с ответом (курсор-картинка, касания): читать сокет вне цикла нельзя. Пока
// подписки нет (или сервер XFixes не знает), память живёт полсекунды: реже
// спрашивать мало смысла, чаще — незачем.
//
// Молчащий владелец запоминается тоже («пусто», 1 с), иначе вызов при каждом
// изменении буфера платил бы полные 250 мс за каждое.

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/widget"
)

var _ widget.ClipboardFormatsProvider = (*x11Clipboard)(nil)

const (
	// x11FormatsTimeout — сколько ждём ответ владельца на TARGETS.
	x11FormatsTimeout = 250 * time.Millisecond
	// x11FormatsTTL — срок жизни ответа, пока смену владельца никто не
	// сообщает (нет XFixes).
	x11FormatsTTL = 500 * time.Millisecond
	// x11FormatsSilentTTL — срок жизни «владелец молчит». Он же действует и при
	// XFixes: владелец может оттаять, не меняясь.
	x11FormatsSilentTTL = time.Second
	// x11FormatsWatchWait — сколько первый вопрос ждёт, пока цикл событий
	// договорится с XFixes; не дождался — спрашивает так.
	x11FormatsWatchWait = 200 * time.Millisecond
	// x11FormatsMaxTargets — предел размера ответа TARGETS: список целей
	// чужой, и десятки килобайт в нём быть не должно.
	x11FormatsMaxTargets = 1 << 20
)

// Состояние подписки на XFixes.
const (
	x11FixesOff         = iota // ещё не просили
	x11FixesRequested          // просили, цикл событий договорится
	x11FixesNegotiating        // цикл событий договаривается
	x11FixesActive             // подписаны, смену владельца сообщают события
	x11FixesUnavailable        // расширения нет или оно старое: живём по сроку
)

// XFixes: минорные коды запросов и маска подписки на смену владельца.
const (
	xfixesQueryVersion        = 0
	xfixesSelectSelectionIn   = 2
	xfixesSelectionOwnerMask  = 1 // SetSelectionOwnerNotify
	xfixesSelectionDestroyMsk = 2 // SelectionWindowDestroyNotify
	xfixesSelectionCloseMask  = 4 // SelectionClientCloseNotify
)

// x11FormatsCache — память ответов TARGETS и подписка на смену владельца.
type x11FormatsCache struct {
	// query выстраивает в очередь вопросы к владельцу: пока один ждёт ответ,
	// остальные получат его же из памяти, а не пошлют своих запросов.
	query sync.Mutex

	mu     sync.Mutex
	gen    uint64 // счётчик смен владельца: ответ, начатый до смены, не запоминается
	valid  bool
	set    widget.ClipboardFormatSet
	silent bool // владелец не ответил в срок
	at     time.Time

	ready      chan struct{} // закрывается, когда договорённость с XFixes закончилась (под mu)
	watch      atomic.Int32  // x11Fixes*
	fixesEvent atomic.Uint32 // код события XFixes SelectionNotify (firstEvent), 0 — нет
	looping    atomic.Bool   // цикл событий читает соединение и его можно будить
}

// invalidate сбрасывает память: владелец сменился. Ответ, который в этот момент
// ещё в пути, устарел и запомнен не будет (см. store).
func (f *x11FormatsCache) invalidate() {
	f.mu.Lock()
	f.valid = false
	f.gen++
	f.mu.Unlock()
}

// lookup отдаёт запомненный ответ, если он ещё годен.
func (f *x11FormatsCache) lookup(now time.Time) (widget.ClipboardFormatSet, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.valid {
		return 0, false
	}
	ttl := x11FormatsTTL
	if f.silent {
		ttl = x11FormatsSilentTTL
	} else if f.watch.Load() == x11FixesActive {
		return f.set, true // до следующей смены владельца
	}
	if now.Sub(f.at) > ttl {
		return 0, false
	}
	return f.set, true
}

// stale отдаёт последний запомненный ответ независимо от возраста.
func (f *x11FormatsCache) stale() widget.ClipboardFormatSet {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.valid {
		return 0
	}
	return f.set
}

// store запоминает ответ. gen — значение счётчика смен до вопроса: если за время
// вопроса владелец сменился, ответ уже устарел и не запоминается.
func (f *x11FormatsCache) store(gen uint64, s widget.ClipboardFormatSet, silent bool, now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gen != gen {
		return
	}
	f.valid, f.set, f.silent, f.at = true, s, silent, now
}

// ClipboardFormats возвращает виды содержимого буфера обмена, не читая данные.
//
// Реализует widget.ClipboardFormatsProvider. Свой буфер — из своей копии,
// мгновенно. Чужой — из памяти, а при её отсутствии одним вопросом TARGETS с
// сроком x11FormatsTimeout. Если вопрос уже в пути (другой вызов или чтение
// файлов), возвращается последний известный ответ, а не ожидание: пусто, если
// такого нет.
func (c *x11Clipboard) ClipboardFormats() widget.ClipboardFormatSet {
	if c == nil || c.w == nil || c.w.wid == 0 || c.w.atomClipboard == 0 {
		return 0
	}
	c.mu.Lock()
	if c.owns {
		s := native.OwnedFormats(c.owned, c.ownedHTML, len(c.ownedFiles) > 0 || c.ownedVirt != nil, c.ownedImg != nil)
		c.mu.Unlock()
		c.fmts.invalidate() // чужой ответ, запомненный до нашего владения, не годится
		return s
	}
	c.mu.Unlock()
	if c.w.atomTargets == 0 {
		return 0
	}

	st := &c.fmts
	st.query.Lock()
	defer st.query.Unlock()
	// Договариваться с XFixes и допрашивать владельца одновременно нельзя: ответ
	// владельца, пришедший посреди договорённости, разбирался бы вложенным
	// запросом и подхватил бы чужой ответ сервера. Поэтому первый вопрос
	// сначала дожидается договорённости (миллисекунды).
	c.requestFormatsWatch()
	st.awaitWatch(x11FormatsWatchWait)
	if s, ok := st.lookup(time.Now()); ok {
		return s
	}
	c.mu.Lock()
	busy := c.wait != nil
	c.mu.Unlock()
	if busy {
		return st.stale() // владелец уже допрашивается (например, GetFiles)
	}

	gen := st.generation()
	raw, answered := c.convertAnswer(c.w.atomTargets, x11FormatsTimeout, x11FormatsMaxTargets)
	var s widget.ClipboardFormatSet
	if answered {
		s = x11FormatsFromTargets(raw, c.formatAtoms())
	}
	st.store(gen, s, !answered, time.Now())
	return s
}

// generation — счётчик смен владельца; см. store.
func (f *x11FormatsCache) generation() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gen
}

// x11FormatAtoms — атомы целей, по которым TARGETS переводится в виды.
type x11FormatAtoms struct {
	text     []uint32 // UTF8_STRING, STRING, TEXT
	html     uint32
	png, bmp uint32
	uriList  uint32
	gnome    uint32
}

// formatAtoms собирает атомы целей окна.
func (c *x11Clipboard) formatAtoms() x11FormatAtoms {
	w := c.w
	return x11FormatAtoms{
		text: []uint32{w.atomUTF8String, 31 /*STRING*/, w.atomText},
		html: w.atomTextHTML, png: w.atomImagePNG, bmp: w.atomImageBMP,
		uriList: w.atomTextUriList, gnome: w.atomGnomeCopied,
	}
}

// x11FormatsFromTargets переводит ответ TARGETS (список атомов, по 4 байта) в
// виды содержимого. Нулевой атом не совпадает ни с чем: цель, которую окно не
// сумело получить у сервера (atoms == 0), не должна «находиться» в пустоте.
func x11FormatsFromTargets(raw []byte, a x11FormatAtoms) widget.ClipboardFormatSet {
	var s widget.ClipboardFormatSet
	for i := 0; i+4 <= len(raw); i += 4 {
		t := binary.LittleEndian.Uint32(raw[i:])
		if t == 0 {
			continue
		}
		switch {
		case t == a.html:
			s |= widget.ClipboardFormatHTML
		case t == a.png, t == a.bmp:
			s |= widget.ClipboardFormatImage
		case t == a.uriList, t == a.gnome:
			s |= widget.ClipboardFormatFiles
		default:
			for _, ta := range a.text {
				if t == ta {
					s |= widget.ClipboardFormatText
					break
				}
			}
		}
	}
	return s
}

// ─── Подписка на смену владельца (XFixes) ───────────────────────────────────

// requestFormatsWatch просит цикл событий подписаться на смену владельца
// CLIPBOARD. Один раз за жизнь окна; безопасно из любой горутины.
func (c *x11Clipboard) requestFormatsWatch() {
	st := &c.fmts
	if !st.watch.CompareAndSwap(x11FixesOff, x11FixesRequested) {
		return
	}
	st.mu.Lock()
	st.ready = make(chan struct{})
	st.mu.Unlock()
	if st.looping.Load() {
		c.w.x11Send(x11WakeRequest(c.w.wid)) // разбудить цикл: он договорится
	}
}

// setFormatsLooping отмечает, что цикл событий окна запущен (и его можно
// будить) либо остановлен.
func (c *x11Clipboard) setFormatsLooping(on bool) {
	if c != nil {
		c.fmts.looping.Store(on)
	}
}

// x11SyncClipboardWatch исполняет отложенную просьбу о подписке на смену
// владельца. Зовётся циклом событий между событиями, где запрос с ответом
// безопасен. Любая неудача — x11FixesUnavailable: ответы живут по сроку.
func (w *X11Window) x11SyncClipboardWatch() {
	c := w.clip
	if c == nil || w.atomClipboard == 0 {
		return
	}
	st := &c.fmts
	if !st.watch.CompareAndSwap(x11FixesRequested, x11FixesNegotiating) {
		return
	}
	if first, ok := w.x11NegotiateFixes(); ok {
		st.fixesEvent.Store(uint32(first))
		st.watch.Store(x11FixesActive)
		st.invalidate() // всё запомненное до подписки нельзя считать вечным
	} else {
		st.watch.Store(x11FixesUnavailable)
	}
	st.mu.Lock()
	if st.ready != nil {
		close(st.ready)
		st.ready = nil
	}
	st.mu.Unlock()
}

// awaitWatch ждёт конца договорённости с XFixes, если она идёт, но не дольше
// timeout. Без цикла событий договориться некому — не ждёт вовсе.
func (f *x11FormatsCache) awaitWatch(timeout time.Duration) {
	if s := f.watch.Load(); (s != x11FixesRequested && s != x11FixesNegotiating) || !f.looping.Load() {
		return
	}
	f.mu.Lock()
	ch := f.ready
	f.mu.Unlock()
	if ch == nil {
		return
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-ch:
	case <-t.C:
	}
}

// x11NegotiateFixes узнаёт у сервера, есть ли XFixes ≥ 2.0, и подписывается на
// смену владельца CLIPBOARD. Вызывать в цикле событий. first — код события
// SelectionNotify расширения.
func (w *X11Window) x11NegotiateFixes() (first byte, ok bool) {
	reply, got := w.x11Reply(x11QueryExtensionRequest(x11FixesExtensionName))
	if !got {
		return 0, false
	}
	major, first, present := parseQueryExtensionReply(reply)
	if !present || first == 0 {
		return 0, false
	}
	reply, got = w.x11Reply(x11FixesQueryVersionRequest(major))
	if !got {
		return 0, false
	}
	if vmaj, _, ok := parseFixesVersion(reply); !ok || vmaj < 2 {
		return 0, false // подписка на выделения появилась в XFixes 2.0
	}
	w.x11Send(x11FixesSelectSelectionInputRequest(major, w.wid, w.atomClipboard,
		xfixesSelectionOwnerMask|xfixesSelectionDestroyMsk|xfixesSelectionCloseMask))
	return first, true
}

const x11FixesExtensionName = "XFIXES"

// x11FixesQueryVersionRequest — XFixesQueryVersion(5.0): запрос с ответом;
// сервер отвечает своей версией. Без него остальные запросы расширения
// недопустимы.
func x11FixesQueryVersionRequest(major byte) []byte {
	req := make([]byte, 12)
	req[0], req[1] = major, xfixesQueryVersion
	binary.LittleEndian.PutUint16(req[2:4], 3)
	binary.LittleEndian.PutUint32(req[4:8], 5)
	binary.LittleEndian.PutUint32(req[8:12], 0)
	return req
}

// parseFixesVersion достаёт версию сервера из ответа XFixesQueryVersion.
func parseFixesVersion(reply []byte) (major, minor uint32, ok bool) {
	if len(reply) < 16 || reply[0] != 1 {
		return 0, 0, false
	}
	return binary.LittleEndian.Uint32(reply[8:12]), binary.LittleEndian.Uint32(reply[12:16]), true
}

// x11FixesSelectSelectionInputRequest — XFixesSelectSelectionInput(window,
// selection, mask): запрос без ответа.
func x11FixesSelectSelectionInputRequest(major byte, window, selection, mask uint32) []byte {
	req := make([]byte, 16)
	req[0], req[1] = major, xfixesSelectSelectionIn
	binary.LittleEndian.PutUint16(req[2:4], 4)
	binary.LittleEndian.PutUint32(req[4:8], window)
	binary.LittleEndian.PutUint32(req[8:12], selection)
	binary.LittleEndian.PutUint32(req[12:16], mask)
	return req
}

// x11FixesSelectionEvent разбирает событие XFixes SelectionNotify: type (код
// расширения + 0), подтип, окно, владелец, выделение, времена. isFixes — это оно
// и есть; selection — выделение, которого оно касается. Чистая функция:
// раскладку проверяет тест.
func x11FixesSelectionEvent(buf []byte, firstEvent byte) (isFixes bool, selection uint32) {
	if firstEvent == 0 || len(buf) < 16 || buf[0]&0x7F != firstEvent {
		return false, 0
	}
	return true, binary.LittleEndian.Uint32(buf[12:16])
}

// handleFixesEvent разбирает событие XFixes о смене владельца. true — событие
// наше и дальше его разбирать не нужно.
func (c *x11Clipboard) handleFixesEvent(buf []byte) bool {
	if c == nil || c.w == nil {
		return false
	}
	first := byte(c.fmts.fixesEvent.Load())
	isFixes, sel := x11FixesSelectionEvent(buf, first)
	if !isFixes {
		return false
	}
	if sel == c.w.atomClipboard {
		c.fmts.invalidate()
	}
	return true
}
