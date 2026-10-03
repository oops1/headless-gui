//go:build linux && !android

package window

// wayland_clipboard_linux.go — буфер обмена через wl_data_device.
//
// Раньше буфер на Linux работал только через xclip/xsel: подпроцесс, X-сервер
// и внешняя утилита, которой в пакете может не быть вовсе. В сессии WinLine
// это не работает совсем — там Wayland без X-сервера, а пакет ставится без
// зависимостей, так что ни xclip, ни wl-copy взять неоткуда. При этом буфер
// сессии связан с RDP-клиентом, и wl_data_device — единственный путь обмена
// текстом с машиной пользователя.
//
// Копирование: создаём wl_data_source, объявляем ему типы и отдаём
// компоновщику как selection. Дальше он сам просит у нас данные (событие
// send с file descriptor), и так — каждому, кто вставляет.
//
// Оформленный текст (SetHTML) объявляется тем же источником дополнительным
// типом text/html: получатель выбирает сам, и Блокнот возьмёт простой текст, а
// Word или браузерный редактор — HTML. Отдельного «режима» нет: событие send
// называет тип, который запросили, и мы отвечаем именно им.
//
// Вставка: компоновщик присылает selection с готовым wl_data_offer; просим у
// него данные подходящего типа и читаем из канала. Чужой клиент может
// отвечать медленно или не ответить вовсе, поэтому чтение идёт с коротким
// тайм-аутом: буфер обмена не повод подвешивать интерфейс.

import (
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// wl_data_device_manager
	wlDataDevMgrCreateDataSource = 0

	// wl_data_device
	wlDataDeviceSetSelection = 1

	// wl_data_source requests / events
	wlDataSourceOffer       = 0
	wlDataSourceDestroy     = 1
	wlDataSourceEvTarget    = 0
	wlDataSourceEvSend      = 1
	wlDataSourceEvCancelled = 2

	// Типы, которыми объявляется и запрашивается простой текст. Первый —
	// нынешний стандарт, остальные понимают старые клиенты и тулкиты.
	mimeTextUTF8  = "text/plain;charset=utf-8"
	mimeTextPlain = "text/plain"
	mimeUTF8Str   = "UTF8_STRING"
	mimeTextStr   = "STRING"

	// clipboardReadTimeout — сколько ждём данные от владельца буфера.
	// Владелец — чужой процесс, он вправе задуматься или умереть; UI ждать
	// дольше этого не должен.
	clipboardReadTimeout = 300 * time.Millisecond

	// maxClipboardBytes — предел на вставку: буфер обмена приходит извне, и
	// доверять его размеру нельзя.
	maxClipboardBytes = 16 << 20
)

// wlClipboard — буфер обмена одного Wayland-соединения.
//
// Хранит и текст, который мы отдали в буфер (его у нас могут запросить в
// любой момент, пока мы владелец), и последний offer, присланный
// компоновщиком.
type wlClipboard struct {
	w *WaylandWindow

	mu    sync.Mutex
	owned string // текст, которым мы владеем; пусто — владеет кто-то другой
	// ownedHTML — оформленная версия нашего содержимого; пусто — только текст.
	ownedHTML string
	source    uint32 // наш wl_data_source, 0 — нет
	offer     uint32 // последний selection-offer компоновщика
	// offerText — типы, которые объявил этот offer: текст мы берём только
	// если он среди них.
	offerText bool
	// offerHTML — тип HTML из объявленных этим offer'ом; пусто — HTML нет.
	// Храним сам тип, а не флаг: запрашивать надо в точности тот, что
	// предложен (text/html или вариант с кодировкой).
	offerHTML string
}

// newWlClipboard создаёт буфер обмена для окна.
func newWlClipboard(w *WaylandWindow) *wlClipboard { return &wlClipboard{w: w} }

// SetText отдаёт текст в системный буфер обмена.
//
// Реализует widget.ClipboardProvider: окно регистрирует этот буфер глобально,
// когда Wayland-соединение поднялось.
func (c *wlClipboard) SetText(s string) { c.setSelection(s, "") }

// SetHTML отдаёт в буфер оформленный текст вместе с простым: рядом с текстовыми
// типами объявляется text/html.
//
// Реализует widget.ClipboardHTMLProvider.
func (c *wlClipboard) SetHTML(html, plain string) { c.setSelection(plain, html) }

// setSelection объявляет источник: простой текст всегда, text/html — если есть
// оформленная версия.
func (c *wlClipboard) setSelection(s, html string) {
	if c == nil || c.w == nil {
		return
	}
	w := c.w
	if w.dataDeviceID == 0 || w.dataDevMgrID == 0 {
		return // компоновщик не дал wl_data_device — обмениваться нечем
	}
	serial := w.inputSerial.Load()
	if serial == 0 {
		// Компоновщик принимает set_selection только с serial'ом недавнего
		// ввода: без него он молча откажет, и буфер остался бы чужим.
		return
	}

	c.mu.Lock()
	old := c.source
	id := w.newID()
	c.source, c.owned, c.ownedHTML = id, s, html
	c.mu.Unlock()

	if old != 0 {
		w.send(newWlMsg(old, wlDataSourceDestroy), -1)
	}
	w.send(newWlMsg(w.dataDevMgrID, wlDataDevMgrCreateDataSource).putUint(id), -1)
	mimes := []string{mimeTextUTF8, mimeTextPlain, mimeUTF8Str, mimeTextStr}
	if html != "" {
		mimes = append(mimes, mimeTextHTML)
	}
	for _, mime := range mimes {
		w.send(newWlMsg(id, wlDataSourceOffer).putString(mime), -1)
	}
	w.send(newWlMsg(w.dataDeviceID, wlDataDeviceSetSelection).putUint(id).putUint(serial), -1)
}

// GetText читает текст из системного буфера обмена.
func (c *wlClipboard) GetText() string {
	if c == nil || c.w == nil {
		return ""
	}
	c.mu.Lock()
	owned, offer, hasText := c.owned, c.offer, c.offerText
	src := c.source
	c.mu.Unlock()

	// Владеем сами — отвечаем из своей копии, не гоняя данные через
	// компоновщика и собственный канал.
	if src != 0 && owned != "" {
		return owned
	}
	if offer == 0 || !hasText {
		return ""
	}
	return string(c.receive(offer, mimeTextUTF8))
}

// GetHTML читает оформленный текст из системного буфера обмена.
//
// Реализует widget.ClipboardHTMLProvider. Берёт text/html, только если
// владелец его объявил: иначе ok == false, и вызывающий возьмёт простой текст
// через GetText.
func (c *wlClipboard) GetHTML() (string, bool) {
	if c == nil || c.w == nil {
		return "", false
	}
	c.mu.Lock()
	html, offer, mime := c.ownedHTML, c.offer, c.offerHTML
	src := c.source
	c.mu.Unlock()

	// Владеем сами — отвечаем из своей копии.
	if src != 0 && html != "" {
		return html, true
	}
	if offer == 0 || mime == "" {
		return "", false
	}
	data := c.receive(offer, mime)
	if len(data) == 0 {
		return "", false
	}
	html = clipboardHTMLToString(data)
	return html, html != ""
}

// receive просит у владельца данные указанного типа и читает их с тайм-аутом.
func (c *wlClipboard) receive(offer uint32, mime string) []byte {
	r, wr, err := os.Pipe()
	if err != nil {
		return nil
	}
	// receive(mime, fd): владелец пишет в наш write-конец, мы читаем read-конец.
	c.w.send(newWlMsg(offer, wlDataOfferReceive).putString(mime), int(wr.Fd()))
	wr.Close() // свой конец закрываем сразу: иначе EOF не придёт никогда

	done := make(chan []byte, 1)
	go func() {
		defer r.Close()
		data, _ := io.ReadAll(io.LimitReader(r, maxClipboardBytes))
		done <- data
	}()
	select {
	case data := <-done:
		return data
	case <-time.After(clipboardReadTimeout):
		// Владелец не ответил. Канал дочитает горутина, и файл закроется
		// там же; подвешивать на это интерфейс незачем.
		r.SetReadDeadline(time.Now())
		return nil
	}
}

// handleSelection запоминает offer, присланный компоновщиком как содержимое
// буфера обмена. id == 0 означает «буфер пуст».
func (c *wlClipboard) handleSelection(id uint32) {
	c.mu.Lock()
	old := c.offer
	c.offer, c.offerText, c.offerHTML = id, false, ""
	if id != 0 {
		// Типы этого offer пришли раньше, событиями offer: берём их из
		// общего списка предложений окна.
		c.offerText = c.w.offerHasText(id)
		c.offerHTML = c.w.offerHTMLMime(id)
		// Чужой selection означает, что владелец теперь не мы.
		if c.source != 0 {
			c.owned, c.ownedHTML = "", ""
		}
	}
	c.mu.Unlock()

	if old != 0 && old != id {
		c.w.send(newWlMsg(old, wlDataOfferDestroy), -1)
		c.w.offerDelete(old)
	}
}

// handleSourceSend отдаёт владеющее содержимое запросившему: компоновщик
// передал нам file descriptor, в который нужно записать данные, и назвал тип,
// который запросили. На text/html отвечаем разметкой, на остальное — текстом:
// если бы отвечали всегда текстом, Word получил бы простую строку под видом
// HTML и вставил её без оформления.
func (c *wlClipboard) handleSourceSend(source uint32, mime string, fd int) {
	c.mu.Lock()
	ours, text := c.source == source, c.owned
	if isHTMLMime(mime) && c.ownedHTML != "" {
		text = c.ownedHTML
	}
	c.mu.Unlock()
	if fd < 0 {
		return
	}
	f := os.NewFile(uintptr(fd), "wl-clipboard-send")
	if !ours {
		f.Close()
		return
	}
	// Пишем в своей горутине: получатель читает не торопясь, а канал имеет
	// размер буфера — запись блокируется, и цикл событий встал бы вместе с
	// ней.
	go func() {
		defer f.Close()
		io.Copy(f, strings.NewReader(text))
	}()
}

// handleSourceCancelled — наш источник заменён чужим: буфером владеет другой.
func (c *wlClipboard) handleSourceCancelled(source uint32) {
	c.mu.Lock()
	ours := c.source == source
	if ours {
		c.source, c.owned, c.ownedHTML = 0, "", ""
	}
	c.mu.Unlock()
	if ours {
		c.w.send(newWlMsg(source, wlDataSourceDestroy), -1)
	}
}

// isSource сообщает, наш ли это wl_data_source (для разбора событий).
func (c *wlClipboard) isSource(id uint32) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return id != 0 && c.source == id
}
