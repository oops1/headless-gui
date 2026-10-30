//go:build linux && !android

package x11

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
// Список файлов (SetFiles) — так же, целями text/uri-list и
// x-special/gnome-copied-files, как у GTK и Qt.
//
// Виртуальные файлы (SetVirtualFiles) — те же цели, но пути берутся у
// приложения в момент запроса (platform/native/clipboard_virtual.go) и ответ уходит из
// горутины: приложение вправе скачивать файлы, а цикл событий этого ждать не
// должен.
//
// Вставка: просим сервер сконвертировать CLIPBOARD в UTF8_STRING; ответ
// приходит событием в цикле событий, поэтому GetText ждёт его по каналу с
// коротким тайм-аутом. Владелец — чужой процесс и вправе не ответить вовсе.

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/widget/base"
)

// x11ClipboardTimeout — сколько ждём ответ владельца буфера.
const x11ClipboardTimeout = 300 * time.Millisecond

// x11ClipboardFilesTimeout — сколько ждём список файлов: владелец вправе
// отдать его, только когда файлы готовы (см. clipboardFilesTimeout).
const x11ClipboardFilesTimeout = 10 * time.Minute

// x11Clipboard — буфер обмена одного X11-соединения.
type x11Clipboard struct {
	w *X11Window

	mu    sync.Mutex
	owned string // наш текст, пока владеем CLIPBOARD
	// ownedHTML — оформленная версия нашего содержимого; пусто — только текст.
	ownedHTML string
	// ownedFiles — наш список файлов; nil — отдаём не файлы.
	ownedFiles []string
	ownedCut   bool
	owns       bool     // владеем ли мы буфером
	wait       *x11Wait // ждёт ответа на ConvertSelection; nil — не ждём
	// ownedImg — наша картинка; nil — отдаём не изображение.
	ownedImg *native.OwnedImage
	// ownedVirt — обещанные виртуальные файлы; nil — не отдаём.
	ownedVirt *native.ClipVirtual
	// incrSends — передачи INCR, которые мы ведём получателям (x11_clipboard_image_linux.go).
	incrSends map[x11IncrKey]*x11IncrSend
	// pending считает ответы, готовящиеся в горутинах (кодирование PNG); нужен
	// тестам, чтобы дождаться их без пауз.
	pending sync.WaitGroup
	// fmts — память ответов «какие виды содержимого в буфере» и подписка на
	// смену владельца (x11_clipboard_formats_linux.go).
	fmts x11FormatsCache
}

func newX11Clipboard(w *X11Window) *x11Clipboard { return &x11Clipboard{w: w} }

// SetText объявляет нас владельцем буфера обмена.
//
// Реализует base.ClipboardProvider.
func (c *x11Clipboard) SetText(s string) { c.setOwned(s, "", nil, false, nil, nil) }

// SetHTML объявляет нас владельцем буфера и готовит к выдаче и текст, и HTML.
//
// Реализует base.ClipboardHTMLProvider.
func (c *x11Clipboard) SetHTML(html, plain string) { c.setOwned(plain, html, nil, false, nil, nil) }

// SetFiles объявляет нас владельцем буфера со списком файлов и путями текстом.
//
// Реализует base.ClipboardFilesProvider.
func (c *x11Clipboard) SetFiles(paths []string, cut bool) {
	c.setOwned(native.FilesPlainText(paths), "", append([]string(nil), paths...), cut, nil, nil)
}

var _ base.ClipboardVirtualFilesProvider = (*x11Clipboard)(nil)

// SetVirtualFiles объявляет нас владельцем буфера с обещанными файлами: цели
// списка файлов объявлены, а пути приложение даёт (opts.Materialize), когда
// их запросят.
//
// Реализует base.ClipboardVirtualFilesProvider. Без Materialize файлов не
// предложить — отказ, и буфер не меняется.
func (c *x11Clipboard) SetVirtualFiles(files []base.VirtualFile, open func(int) (io.ReadCloser, error), opts base.VirtualFilesOptions) error {
	if c == nil || c.w == nil || c.w.wid == 0 || c.w.atomClipboard == 0 {
		return base.ErrClipboardVirtualUnsupported
	}
	v, err := native.NewClipVirtual(files, open, opts)
	if err != nil {
		return err
	}
	if c.w.atomTextUriList == 0 && c.w.atomGnomeCopied == 0 {
		return fmt.Errorf("window: сервер не дал атомов списка файлов: %w", base.ErrClipboardVirtualUnsupported)
	}
	c.setOwned(v.Names(), "", nil, false, nil, v)
	return nil
}

// GetVirtualFiles возвращает обещанные нами файлы, пока мы владеем буфером.
func (c *x11Clipboard) GetVirtualFiles() ([]base.VirtualFile, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	v, owns := c.ownedVirt, c.owns
	c.mu.Unlock()
	if v == nil || !owns {
		return nil, false
	}
	return append([]base.VirtualFile(nil), v.Files...), true
}

// OpenVirtualFile открывает содержимое нашего обещанного файла (open приложения).
func (c *x11Clipboard) OpenVirtualFile(index int) (io.ReadCloser, error) {
	if c == nil {
		return nil, base.ErrVirtualFileNotFound
	}
	c.mu.Lock()
	v := c.ownedVirt
	c.mu.Unlock()
	return v.OpenFile(index)
}

// Cleanup снимает обещание виртуальных файлов, если оно ещё в буфере: владение
// выделением отдаётся (никому), приложению сообщается через WithRelease. Чужое
// содержимое и наши текст и файлы не трогает.
func (c *x11Clipboard) Cleanup() {
	if c == nil || c.w == nil {
		return
	}
	c.mu.Lock()
	v := c.ownedVirt
	if v != nil {
		c.owns, c.owned, c.ownedHTML = false, "", ""
		c.ownedFiles, c.ownedCut, c.ownedImg, c.ownedVirt = nil, false, nil, nil
	}
	c.mu.Unlock()
	if v == nil {
		return
	}
	if c.w.wid != 0 && c.w.atomClipboard != 0 {
		c.w.x11SetSelectionOwner(c.w.atomClipboard, 0)
	}
	v.Drop()
}

// setOwned запоминает содержимое и становится владельцем CLIPBOARD. Прежнее
// оформление затирается всегда: иначе после SetText проситель text/html
// получил бы разметку от прошлого копирования.
func (c *x11Clipboard) setOwned(text, html string, files []string, cut bool, img *native.OwnedImage, virt *native.ClipVirtual) {
	if c == nil || c.w == nil || c.w.wid == 0 || c.w.atomClipboard == 0 {
		return
	}
	c.mu.Lock()
	c.owned, c.ownedHTML, c.owns = text, html, true
	c.ownedFiles, c.ownedCut, c.ownedImg = files, cut, img
	old := c.ownedVirt
	c.ownedVirt = virt
	c.mu.Unlock()
	old.Drop()
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
		return s // владеем сами — незачем ходить через сервер (у картинки текста нет)
	}
	c.mu.Unlock()
	return c.convert(c.w.atomUTF8String)
}

// GetHTML читает оформленный текст из буфера обмена.
//
// Реализует base.ClipboardHTMLProvider. Запрашиваем text/html напрямую: если
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
	html := native.ClipboardHTMLToString([]byte(raw))
	return html, html != ""
}

// GetFiles читает список файлов из буфера обмена.
//
// Реализует base.ClipboardFilesProvider. Сначала x-special/gnome-copied-files
// — только он знает про cut, — при отказе text/uri-list. Владелец без такой
// цели отвечает отказом сразу, а с ней вправе думать долго: звать не на
// горутине движка.
func (c *x11Clipboard) GetFiles() ([]string, bool, bool) {
	if c == nil || c.w == nil || c.w.wid == 0 || c.w.atomClipboard == 0 {
		return nil, false, false
	}
	c.mu.Lock()
	if c.owns {
		files, cut, virt := c.ownedFiles, c.ownedCut, c.ownedVirt
		c.mu.Unlock()
		if virt != nil {
			// Вставка внутри программы: те же пути, что получил бы чужой вставляющий.
			paths, err := virt.Paths()
			return paths, false, err == nil && len(paths) > 0
		}
		if len(files) == 0 {
			return nil, false, false
		}
		return append([]string(nil), files...), cut, true
	}
	c.mu.Unlock()
	if a := c.w.atomGnomeCopied; a != 0 {
		if p, cut, ok := native.ParseGnomeCopiedFiles(c.convertTimeout(a, x11ClipboardFilesTimeout)); ok {
			return p, cut, true
		}
	}
	if a := c.w.atomTextUriList; a != 0 {
		if p, ok := native.ParseClipboardURIList(c.convertTimeout(a, x11ClipboardFilesTimeout)); ok {
			return p, false, true
		}
	}
	return nil, false, false
}

// convert просит сервер сконвертировать CLIPBOARD в указанную цель и ждёт
// ответ с коротким тайм-аутом. Пустая строка — отказ, молчание владельца или
// пустой буфер.
func (c *x11Clipboard) convert(target uint32) string {
	return c.convertTimeout(target, x11ClipboardTimeout)
}

// convertTimeout — convert с заданным сроком ожидания.
func (c *x11Clipboard) convertTimeout(target uint32, timeout time.Duration) string {
	return string(c.convertBytes(target, timeout, native.MaxClipboardBytes))
}

// x11Wait — ожидание ответа на ConvertSelection.
type x11Wait struct {
	ch       chan []byte   // готовый ответ; nil — отказ
	progress chan struct{} // INCR: пришёл очередной кусок, срок ожидания начинается заново
	// Состояние передачи INCR: владелец отдаёт большие данные кусками, и вместо
	// ответа приходит свойство типа INCR (ICCCM §2.7.2).
	incr  bool
	buf   []byte
	limit int // предел размера ответа
}

// notifyProgress будит ожидающего: владелец жив, срок отсчитывается заново.
func (wt *x11Wait) notifyProgress() {
	select {
	case wt.progress <- struct{}{}:
	default:
	}
}

// convertBytes — convert с заданным сроком и пределом размера, отдающий байты.
// Срок относится к ответу владельца и, при передаче INCR, к каждому следующему
// куску: цельный срок на всю большую картинку обрубил бы честного владельца,
// а срок на кусок не даёт зависшему подвесить вызов.
func (c *x11Clipboard) convertBytes(target uint32, timeout time.Duration, limit int) []byte {
	data, _ := c.convertAnswer(target, timeout, limit)
	return data
}

// convertAnswer — convertBytes, различающий отказ и молчание: answered == false
// значит, владелец не ответил в срок (или запрос уже был в пути), а пустой
// data при answered == true — он ответил отказом.
func (c *x11Clipboard) convertAnswer(target uint32, timeout time.Duration, limit int) (data []byte, answered bool) {
	c.mu.Lock()
	if c.wait != nil {
		c.mu.Unlock()
		return nil, false // запрос уже в пути; второй ответ всё равно будет один
	}
	wt := &x11Wait{ch: make(chan []byte, 1), progress: make(chan struct{}, 1), limit: limit}
	c.wait = wt
	c.mu.Unlock()

	// Ответ придёт событием SelectionNotify в цикл событий — он и разбудит
	// этот канал (см. handleNotify).
	c.w.x11ConvertSelection(c.w.atomClipboard, target, c.w.atomClipProp, 0)

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case res := <-wt.ch:
			return res, true
		case <-wt.progress:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(timeout)
		case <-timer.C:
			c.mu.Lock()
			if c.wait == wt {
				c.wait = nil
			}
			c.mu.Unlock()
			return nil, false // владелец молчит: буфер обмена не повод вешать интерфейс
		}
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

	var data []byte
	if property != 0 {
		var typ uint32
		typ, _, data = c.w.x11GetProperty(c.w.wid, property, true)
		if c.w.atomIncr != 0 && typ == c.w.atomIncr {
			// Владелец отдаёт данные кусками. Свойство мы только что удалили —
			// это и есть сигнал «присылай первый кусок»; дальше куски приходят
			// событиями PropertyNotify (handlePropertyNotify).
			c.mu.Lock()
			wt := c.wait
			if wt != nil {
				wt.incr, wt.buf = true, nil
			}
			c.mu.Unlock()
			if wt != nil {
				wt.notifyProgress()
			}
			return true
		}
	}
	c.mu.Lock()
	wt := c.wait
	c.wait = nil
	c.mu.Unlock()
	if wt != nil {
		wt.ch <- data
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
	files, cut := c.ownedFiles, c.ownedCut
	img := c.ownedImg
	virt := c.ownedVirt
	c.mu.Unlock()
	uriList, gnome := c.w.atomTextUriList, c.w.atomGnomeCopied

	ok := false
	switch {
	case selection != c.w.atomClipboard || !owns:
		// Не наш буфер — отвечаем отказом.
	case target == c.w.atomTargets:
		// Список форматов, которые мы умеем отдать.
		data := make([]byte, 0, 20)
		targets := []uint32{c.w.atomTargets, c.w.atomUTF8String, 31 /*STRING*/, c.w.atomText}
		if img != nil {
			// Картинка: текстовых целей нет, иначе редактор вставил бы пустую
			// строку вместо того, чтобы понять, что вставлять нечего.
			targets = []uint32{c.w.atomTargets}
			if c.w.atomImagePNG != 0 {
				targets = append(targets, c.w.atomImagePNG)
			}
		}
		if html != "" && c.w.atomTextHTML != 0 {
			targets = append(targets, c.w.atomTextHTML)
		}
		if len(files) > 0 || virt != nil {
			for _, a := range []uint32{uriList, gnome} {
				if a != 0 {
					targets = append(targets, a)
				}
			}
		}
		for _, a := range targets {
			var b [4]byte
			binary.LittleEndian.PutUint32(b[:], a)
			data = append(data, b[:]...)
		}
		c.w.x11ChangeProperty(requestor, property, 4 /*ATOM*/, 32, data)
		ok = true
	case img != nil && c.w.atomImagePNG != 0 && target == c.w.atomImagePNG:
		// PNG кодируется при первом запросе и не в цикле событий: на большом
		// снимке это десятки миллисекунд. Ответ уйдёт из горутины.
		c.pending.Add(1)
		go func() {
			defer c.pending.Done()
			c.serveImage(img, requestor, property, timestamp, selection, target)
		}()
		return
	case img != nil:
		// Остальные цели у картинки — отказ.
	case virt != nil && ((uriList != 0 && target == uriList) || (gnome != 0 && target == gnome)):
		// Пути даёт приложение, и это может длиться долго: ответ уйдёт из
		// горутины, цикл событий не ждёт.
		c.pending.Add(1)
		go func() {
			defer c.pending.Done()
			c.serveVirtual(virt, target == gnome, requestor, property, timestamp, selection, target)
		}()
		return
	case target == c.w.atomUTF8String || target == 31 /*STRING*/ || target == c.w.atomText:
		c.w.x11ChangeProperty(requestor, property, target, 8, []byte(text))
		ok = true
	case html != "" && c.w.atomTextHTML != 0 && target == c.w.atomTextHTML:
		c.w.x11ChangeProperty(requestor, property, target, 8, []byte(html))
		ok = true
	case len(files) > 0 && uriList != 0 && target == uriList:
		c.w.x11ChangeProperty(requestor, property, target, 8, []byte(native.BuildURIList(files)))
		ok = true
	case len(files) > 0 && gnome != 0 && target == gnome:
		c.w.x11ChangeProperty(requestor, property, target, 8, []byte(native.BuildGnomeCopiedFiles(files, cut)))
		ok = true
	}
	if !ok {
		property = 0
	}
	c.w.x11SendSelectionNotify(requestor, timestamp, selection, target, property)
}

// serveVirtual отвечает на запрос списка файлов у виртуальных файлов: просит
// пути у приложения и отдаёт их в нужном формате. Отказ приложения (или тело,
// которое не влезает в один запрос) — свойство None. Зовётся из горутины.
func (c *x11Clipboard) serveVirtual(v *native.ClipVirtual, gnome bool, requestor, property, timestamp, selection, target uint32) {
	w := c.w
	paths, err := v.Paths()
	if err != nil || len(paths) == 0 {
		w.x11SendSelectionNotify(requestor, timestamp, selection, target, 0)
		return
	}
	body := native.BuildURIList(paths)
	if gnome {
		body = native.BuildGnomeCopiedFiles(paths, false)
	}
	if len(body) > native.X11IncrChunk {
		// Для INCR список путей слишком мал, а ChangeProperty больше запроса не
		// сделать: честный отказ лучше обрыва соединения.
		w.x11SendSelectionNotify(requestor, timestamp, selection, target, 0)
		return
	}
	w.x11ChangeProperty(requestor, property, target, 8, []byte(body))
	w.x11SendSelectionNotify(requestor, timestamp, selection, target, property)
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
	c.ownedFiles, c.ownedCut, c.ownedImg = nil, false, nil
	virt := c.ownedVirt
	c.ownedVirt = nil
	c.mu.Unlock()
	c.fmts.invalidate() // у буфера новый владелец: прежний ответ о видах не годится
	virt.Drop()
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
