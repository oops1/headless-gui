//go:build linux && !android

package core

// x11_clipboard_image_linux.go — изображение в буфере обмена X11.
//
// Копирование: мы владелец CLIPBOARD, в TARGETS у картинки одна цель —
// image/png. PNG кодируется ЛЕНИВО, на первый запрос, и отдаётся не из цикла
// событий. Маленькая картинка кладётся в свойство просителя одним куском;
// большая — по протоколу INCR (ICCCM §2.7.2): запрос ChangeProperty без
// BIG-REQUESTS не может быть больше 256 КиБ, а PNG снимка экрана — единицы
// мегабайт. Порядок передачи:
//
//  1. подписываемся на PropertyNotify окна-просителя;
//  2. кладём в его свойство тип INCR с оценкой размера и отвечаем SelectionNotify;
//  3. проситель удаляет свойство — на каждое удаление кладём следующий кусок
//     (x11IncrChunk байт); пустой кусок означает конец.
//
// Вставка: сначала TARGETS — какие цели предложил владелец; берём image/png, а
// при его отсутствии image/bmp. Большие данные владелец вправе отдать тоже по
// INCR — куски собираются в handlePropertyNotify. Срок ожидания — 300 мс на
// ответ и на каждый следующий кусок (см. convertBytes).

import (
	"encoding/binary"
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/internal/clipimage"
	"github.com/oops1/headless-gui/v3/widget"
)

var _ widget.ClipboardImageProvider = (*x11Clipboard)(nil)

// x11IncrIdle — сколько ждём, пока проситель заберёт очередной кусок INCR.
// Не короткий срок чтения: проситель — приложение, которое успевает
// отвлечься, но брошенная передача не должна висеть и держать подписку.
var x11IncrIdle = 30 * time.Second

// x11IncrKey — передача INCR: окно-проситель и свойство, в которое пишем.
type x11IncrKey struct{ window, property uint32 }

// x11IncrSend — одна ведомая нами передача INCR.
type x11IncrSend struct {
	data   []byte
	off    int
	target uint32 // тип, которым помечаются куски
	timer  *time.Timer
}

// next отдаёт очередной кусок; пустой — данные кончились (маркер конца).
func (s *x11IncrSend) next() []byte {
	if s.off >= len(s.data) {
		return nil
	}
	end := min(s.off+x11IncrChunk, len(s.data))
	chunk := s.data[s.off:end]
	s.off = end
	return chunk
}

// SetImage объявляет нас владельцем буфера с изображением.
//
// Реализует widget.ClipboardImageProvider. Картинка копируется сразу, в PNG
// превращается по запросу получателя.
func (c *x11Clipboard) SetImage(img image.Image) error {
	if img == nil || img.Bounds().Empty() {
		return widget.ErrClipboardImageEmpty
	}
	if c == nil || c.w == nil || c.w.wid == 0 || c.w.atomClipboard == 0 || c.w.atomImagePNG == 0 {
		return widget.ErrClipboardImageUnsupported
	}
	c.setOwned("", "", nil, false, newOwnedImage(img), nil)
	return nil
}

// GetImage читает изображение из буфера обмена.
//
// Реализует widget.ClipboardImageProvider. Звать не на горутине движка:
// владелец отвечает, когда готов, а картинка потом декодируется. Вызов не
// может прийти из потока событий — тот же поток ждёт ответ владельца.
func (c *x11Clipboard) GetImage() (image.Image, bool) {
	if c == nil || c.w == nil || c.w.wid == 0 || c.w.atomClipboard == 0 {
		return nil, false
	}
	c.mu.Lock()
	if c.owns {
		own := c.ownedImg
		c.mu.Unlock()
		if own == nil {
			return nil, false
		}
		return own.Image(), true // владеем сами — отвечаем из своей копии
	}
	c.mu.Unlock()

	mime, data := c.fetchImage()
	if len(data) == 0 {
		return nil, false
	}
	img, err := clipimage.DecodeMime(mime, data)
	if err != nil || img == nil || img.Bounds().Empty() {
		return nil, false
	}
	return img, true
}

// fetchImage достаёт у владельца байты изображения: спрашивает, какие цели он
// предложил (TARGETS), и берёт image/png, а при его отсутствии image/bmp.
// Пустой результат — картинки нет.
//
// Владелец, ответивший на TARGETS отказом или пустым списком (такие
// встречаются), пробуется вслепую: сначала PNG, затем BMP. Молчащий владелец
// — нет: он не ответит и на остальное, а вызов не должен длиться дольше срока
// одного ответа. У владельца с непустым списком без картинки лишних вопросов
// нет.
func (c *x11Clipboard) fetchImage() (mime string, data []byte) {
	w := c.w
	if w.atomTargets == 0 || (w.atomImagePNG == 0 && w.atomImageBMP == 0) {
		return "", nil
	}
	get := func(atom uint32) []byte {
		return c.convertBytes(atom, x11ClipboardTimeout, maxClipboardImageBytes)
	}
	raw, answered := c.convertAnswer(w.atomTargets, x11ClipboardTimeout, 1<<20)
	if !answered {
		return "", nil
	}
	if len(raw) < 4 {
		if w.atomImagePNG != 0 {
			if d := get(w.atomImagePNG); len(d) > 0 {
				return clipimage.MimePNG, d
			}
		}
		if w.atomImageBMP != 0 {
			if d := get(w.atomImageBMP); len(d) > 0 {
				return clipimage.MimeBMP, d
			}
		}
		return "", nil
	}
	var havePNG, haveBMP bool
	for i := 0; i+4 <= len(raw); i += 4 {
		switch a := binary.LittleEndian.Uint32(raw[i:]); {
		case a == 0:
		case a == w.atomImagePNG:
			havePNG = true
		case a == w.atomImageBMP:
			haveBMP = true
		}
	}
	switch {
	case havePNG:
		return clipimage.MimePNG, get(w.atomImagePNG)
	case haveBMP:
		return clipimage.MimeBMP, get(w.atomImageBMP)
	}
	return "", nil
}

// serveImage отвечает на запрос image/png: кодирует PNG (один раз за владение)
// и отдаёт его целиком или по INCR. Зовётся из горутины, не из цикла событий.
func (c *x11Clipboard) serveImage(img *ownedImage, requestor, property, timestamp, selection, target uint32) {
	w := c.w
	data, err := img.PNG()
	if err != nil || len(data) == 0 {
		w.x11SendSelectionNotify(requestor, timestamp, selection, target, 0)
		return
	}
	if len(data) <= x11IncrChunk {
		w.x11ChangeProperty(requestor, property, target, 8, data)
		w.x11SendSelectionNotify(requestor, timestamp, selection, target, property)
		return
	}
	// Подписка на PropertyNotify собственного окна затёрла бы его маску
	// событий, а в буфер обмена через сервер мы сами себя не просим — отказ.
	if w.atomIncr == 0 || requestor == w.wid {
		w.x11SendSelectionNotify(requestor, timestamp, selection, target, 0)
		return
	}

	key := x11IncrKey{requestor, property}
	st := &x11IncrSend{data: data, target: target}
	c.mu.Lock()
	if c.incrSends == nil {
		c.incrSends = map[x11IncrKey]*x11IncrSend{}
	}
	if old := c.incrSends[key]; old != nil {
		old.timer.Stop() // тот же проситель начал заново — прежняя передача брошена
	}
	c.incrSends[key] = st
	st.timer = time.AfterFunc(x11IncrIdle, func() { c.abortIncr(key, st) })
	c.mu.Unlock()

	// Подписка — до того, как свойство появится: удаление его просителем может
	// прийти сразу за ответом.
	w.x11SelectPropertyEvents(requestor, true)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(data)))
	w.x11ChangeProperty(requestor, property, w.atomIncr, 32, size[:])
	w.x11SendSelectionNotify(requestor, timestamp, selection, target, property)
}

// abortIncr бросает передачу, которую проситель не забирает.
func (c *x11Clipboard) abortIncr(key x11IncrKey, st *x11IncrSend) {
	c.mu.Lock()
	if c.incrSends[key] != st {
		c.mu.Unlock()
		return
	}
	delete(c.incrSends, key)
	last := !c.incrWindowBusyLocked(key.window)
	c.mu.Unlock()
	if last {
		c.w.x11SelectPropertyEvents(key.window, false)
	}
}

// incrWindowBusyLocked — на окне ещё есть наши передачи (подписку снимать рано).
func (c *x11Clipboard) incrWindowBusyLocked(window uint32) bool {
	for k := range c.incrSends {
		if k.window == window {
			return true
		}
	}
	return false
}

// handlePropertyNotify разбирает PropertyNotify. Возвращает true, если событие
// относилось к буферу обмена.
//
// Событие значимо в двух случаях:
//   - на нашем окне появился новый кусок INCR, который нам отдаёт владелец, —
//     читаем и удаляем его (удаление и есть просьба следующего);
//   - проситель удалил кусок, который мы ему положили, — кладём следующий.
func (c *x11Clipboard) handlePropertyNotify(buf []byte) bool {
	if c == nil || c.w == nil {
		return false
	}
	window := binary.LittleEndian.Uint32(buf[4:8])
	atom := binary.LittleEndian.Uint32(buf[8:12])
	state := buf[16] // 0 — NewValue, 1 — Deleted

	switch {
	case window == c.w.wid && atom == c.w.atomClipProp && state == 0:
		c.mu.Lock()
		wt := c.wait
		reading := wt != nil && wt.incr
		c.mu.Unlock()
		if !reading {
			return true // обычный ответ: его заберёт handleNotify
		}
		_, _, chunk := c.w.x11GetProperty(c.w.wid, c.w.atomClipProp, true)
		c.mu.Lock()
		if c.wait != wt {
			c.mu.Unlock()
			return true // ожидающий уже сдался
		}
		switch {
		case len(chunk) == 0: // пустой кусок — конец передачи
			c.wait = nil
			res := wt.buf
			c.mu.Unlock()
			wt.ch <- res
		case len(wt.buf)+len(chunk) > wt.limit: // слишком большое — отказ
			c.wait = nil
			c.mu.Unlock()
			wt.ch <- nil
		default:
			wt.buf = append(wt.buf, chunk...)
			c.mu.Unlock()
			wt.notifyProgress()
		}
		return true

	case state == 1:
		key := x11IncrKey{window, atom}
		c.mu.Lock()
		st := c.incrSends[key]
		if st == nil {
			c.mu.Unlock()
			return false
		}
		chunk := st.next()
		done := len(chunk) == 0
		last := false
		if done {
			st.timer.Stop()
			delete(c.incrSends, key)
			last = !c.incrWindowBusyLocked(window)
		} else {
			st.timer.Reset(x11IncrIdle)
		}
		c.mu.Unlock()
		// Пустой кусок кладём тоже: это маркер конца для просителя.
		c.w.x11ChangeProperty(window, atom, st.target, 8, chunk)
		if last {
			c.w.x11SelectPropertyEvents(window, false)
		}
		return true
	}
	return false
}

// x11SelectPropertyEvents подписывает нас на PropertyNotify чужого окна
// (ChangeWindowAttributes, opcode 2, маска CWEventMask) или снимает подписку.
// Маска у каждого клиента своя, чужие подписки окна не затрагиваются.
func (w *X11Window) x11SelectPropertyEvents(window uint32, on bool) {
	const cwEventMask = 0x00000800
	const propertyChangeMask = 0x00400000
	buf := make([]byte, 16)
	buf[0] = 2
	binary.LittleEndian.PutUint16(buf[2:4], 4)
	binary.LittleEndian.PutUint32(buf[4:8], window)
	binary.LittleEndian.PutUint32(buf[8:12], cwEventMask)
	if on {
		binary.LittleEndian.PutUint32(buf[12:16], propertyChangeMask)
	}
	w.x11Send(buf)
}
