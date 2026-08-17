//go:build linux && !android

package window

import "encoding/binary"

// XDND (X Drag-and-Drop, протокол v5) — приём файлов, перетащенных из ОС в
// окно. Реализовано поверх сырых Xlib-запросов бэкенда (native_linux.go):
// окно объявляет XdndAware=5, отвечает на XdndEnter/Position/Leave/Drop
// сообщениями XdndStatus/XdndFinished, а данные (text/uri-list) получает
// АСИНХРОННО через XConvertSelection → SelectionNotify (без блокирующего
// ожидания ответа в цикле событий — как того требует однопоточная модель
// соединения). Спецификация: freedesktop.org/wiki/Specifications/XDND.
//
// Все обработчики вызываются из горутины насоса событий (handleX11Event),
// поэтому синхронное чтение reply в x11GetProperty безопасно — тем же приёмом,
// что x11ReloadKeyboardMapping (промежуточные события диспатчатся на месте).

// handleXdndEnter запоминает окно-источник и определяет, предложен ли
// text/uri-list среди типов данных (иначе сброс не принимаем).
func (w *X11Window) handleXdndEnter(buf []byte) {
	w.dndSource = binary.LittleEndian.Uint32(buf[12:16]) // data.l[0]
	l1 := binary.LittleEndian.Uint32(buf[16:20])          // data.l[1]
	w.dndVersion = int(l1 >> 24)
	w.dndAccept = false
	w.dndDropPending = false

	if l1&1 != 0 {
		// Более 3 типов — полный список в свойстве XdndTypeList источника.
		// Свойство источника НЕ удаляем (delete=false).
		_, _, data := w.x11GetProperty(w.dndSource, w.atomXdndTypeList, false)
		for i := 0; i+4 <= len(data); i += 4 {
			if binary.LittleEndian.Uint32(data[i:i+4]) == w.atomTextUriList {
				w.dndAccept = true
				break
			}
		}
		return
	}
	// До 3 типов — инлайн в data.l[2..4].
	for _, off := range []int{20, 24, 28} {
		if binary.LittleEndian.Uint32(buf[off:off+4]) == w.atomTextUriList {
			w.dndAccept = true
			break
		}
	}
}

// handleXdndPosition запоминает позицию курсора (корневые координаты) и
// отвечает источнику XdndStatus (принимаем/не принимаем).
func (w *X11Window) handleXdndPosition(buf []byte) {
	src := binary.LittleEndian.Uint32(buf[12:16]) // data.l[0]
	if src == 0 {
		src = w.dndSource
	}
	packed := binary.LittleEndian.Uint32(buf[20:24]) // data.l[2] = (x<<16)|y
	w.dndX = int(packed >> 16 & 0xFFFF)
	w.dndY = int(packed & 0xFFFF)
	w.sendXdndStatus(src, w.dndAccept)
}

// handleXdndDrop запрашивает данные выделения (если принимаем) — ответ придёт
// событием SelectionNotify. Иначе сразу сообщает источнику XdndFinished(отказ).
func (w *X11Window) handleXdndDrop(buf []byte) {
	src := binary.LittleEndian.Uint32(buf[12:16]) // data.l[0]
	if src == 0 {
		src = w.dndSource
	}
	w.dndTime = binary.LittleEndian.Uint32(buf[20:24]) // data.l[2] = timestamp
	if !w.dndAccept || w.onFilesDropped == nil {
		w.sendXdndFinished(src, false)
		w.dndReset()
		return
	}
	// XConvertSelection(XdndSelection, text/uri-list) → свойство XdndSelection
	// на нашем окне; получение — в handleSelectionNotify.
	w.dndDropPending = true
	w.x11ConvertSelection(w.atomXdndSelection, w.atomTextUriList, w.atomXdndSelection, w.dndTime)
}

// handleSelectionNotify читает доставленные данные (text/uri-list), парсит
// пути, вызывает колбэк и завершает сессию XdndFinished.
func (w *X11Window) handleSelectionNotify(buf []byte) {
	if !w.dndDropPending {
		return
	}
	w.dndDropPending = false
	selection := binary.LittleEndian.Uint32(buf[12:16])
	property := binary.LittleEndian.Uint32(buf[20:24]) // None(0) — конверсия не удалась
	src := w.dndSource

	accepted := false
	if selection == w.atomXdndSelection && property != 0 {
		_, _, data := w.x11GetProperty(w.wid, property, true) // читаем и удаляем
		paths := parseURIList(string(data))
		if len(paths) > 0 && w.onFilesDropped != nil {
			lx, ly := w.dndLocalPos()
			w.onFilesDropped(paths, lx, ly)
			accepted = true
		}
	}
	w.sendXdndFinished(src, accepted)
	w.dndReset()
}

// dndLocalPos переводит корневые координаты сброса в клиентские (по кэшу позиции).
func (w *X11Window) dndLocalPos() (int, int) {
	px, py := w.GetPosition()
	return w.dndX - px, w.dndY - py
}

// dndReset сбрасывает состояние DnD-сессии.
func (w *X11Window) dndReset() {
	w.dndSource = 0
	w.dndVersion = 0
	w.dndAccept = false
	w.dndDropPending = false
}

// ─── XDND: низкоуровневые запросы ────────────────────────────────────────────

// sendXdndStatus сообщает источнику, принимаем ли мы сброс (data.l[1] бит 0).
func (w *X11Window) sendXdndStatus(dest uint32, accept bool) {
	var data [5]uint32
	data[0] = w.wid // target
	if accept {
		data[1] = 1              // бит 0: accept
		data[4] = w.atomXdndActionCopy
	}
	// data[2]=data[3]=0 — пустой прямоугольник: источник шлёт XdndPosition на
	// каждое движение курсора.
	w.x11SendClientMessage(dest, w.atomXdndStatus, data)
}

// sendXdndFinished завершает DnD-сессию (accepted — был ли сброс принят).
func (w *X11Window) sendXdndFinished(dest uint32, accepted bool) {
	if dest == 0 {
		return
	}
	var data [5]uint32
	data[0] = w.wid
	if accepted {
		data[1] = 1 // бит 0: данные приняты (XDND v5)
		data[2] = w.atomXdndActionCopy
	}
	w.x11SendClientMessage(dest, w.atomXdndFinished, data)
}

// x11SendClientMessage отправляет ClientMessage (format 32, 5×CARD32) окну dest
// через SendEvent (mask 0 — доставляется владельцу окна dest).
func (w *X11Window) x11SendClientMessage(dest, msgType uint32, data [5]uint32) {
	ev := make([]byte, 32)
	ev[0] = 33 // ClientMessage
	ev[1] = 32 // format
	binary.LittleEndian.PutUint32(ev[4:8], dest) // window = получатель
	binary.LittleEndian.PutUint32(ev[8:12], msgType)
	for i := 0; i < 5; i++ {
		binary.LittleEndian.PutUint32(ev[12+i*4:16+i*4], data[i])
	}

	sendBuf := make([]byte, 44)
	sendBuf[0] = 25 // SendEvent
	sendBuf[1] = 0  // propagate = false
	binary.LittleEndian.PutUint16(sendBuf[2:4], 11)
	binary.LittleEndian.PutUint32(sendBuf[4:8], dest) // destination
	binary.LittleEndian.PutUint32(sendBuf[8:12], 0)   // event-mask = 0
	copy(sendBuf[12:], ev)
	w.x11Send(sendBuf)
}

// x11ConvertSelection просит владельца выделения selection преобразовать его
// к типу target и записать в property окна-запросчика (нашего). Ответ — событие
// SelectionNotify (без reply).
func (w *X11Window) x11ConvertSelection(selection, target, property, time uint32) {
	buf := make([]byte, 24)
	buf[0] = 24 // ConvertSelection
	binary.LittleEndian.PutUint16(buf[2:4], 6)
	binary.LittleEndian.PutUint32(buf[4:8], w.wid) // requestor
	binary.LittleEndian.PutUint32(buf[8:12], selection)
	binary.LittleEndian.PutUint32(buf[12:16], target)
	binary.LittleEndian.PutUint32(buf[16:20], property)
	binary.LittleEndian.PutUint32(buf[20:24], time)
	w.x11Send(buf)
}

// propertyTooBig — заявленная длина свойства (в 4-байтовых словах) вне лимита.
func propertyTooBig(words int) bool {
	return words < 0 || words > maxDnDBytes/4
}

// x11Discard дочитывает и выбрасывает n байт тела ответа.
func (w *X11Window) x11Discard(n int) {
	buf := make([]byte, 64*1024)
	for n > 0 {
		k := min(n, len(buf))
		if !w.readFull(buf[:k]) {
			return
		}
		n -= k
	}
}

// x11GetProperty читает свойство property окна window (тип AnyPropertyType).
// del — удалить свойство после чтения. Возвращает (тип, формат в битах, данные).
// Вызывается из горутины насоса событий: reply читается синхронно, промежуточные
// события диспатчатся на месте (как в x11ReloadKeyboardMapping).
func (w *X11Window) x11GetProperty(window, property uint32, del bool) (uint32, int, []byte) {
	delFlag := byte(0)
	if del {
		delFlag = 1
	}
	req := make([]byte, 24)
	req[0] = 20 // GetProperty
	req[1] = delFlag
	binary.LittleEndian.PutUint16(req[2:4], 6)
	binary.LittleEndian.PutUint32(req[4:8], window)
	binary.LittleEndian.PutUint32(req[8:12], property)
	binary.LittleEndian.PutUint32(req[12:16], 0)          // type = AnyPropertyType
	binary.LittleEndian.PutUint32(req[16:20], 0)          // long-offset
	binary.LittleEndian.PutUint32(req[20:24], 0x1FFFFFFF) // long-length (4-байтовых единиц)

	w.mu.Lock()
	w.conn.Write(req)
	w.seqNum++
	w.mu.Unlock()

	hdr := make([]byte, 32)
	for i := 0; i < 4096; i++ { // предохранитель от бесконечного цикла
		if !w.readFull(hdr) {
			return 0, 0, nil
		}
		switch hdr[0] {
		case 1: // reply
			format := int(hdr[1])
			n := int(binary.LittleEndian.Uint32(hdr[4:8])) // длина значения в 4-байтовых единицах
			typeAtom := binary.LittleEndian.Uint32(hdr[8:12])
			nitems := int(binary.LittleEndian.Uint32(hdr[16:20]))
			if n == 0 {
				return typeAtom, format, nil
			}
			if propertyTooBig(n) { // пир заявил неправдоподобный размер
				w.x11Discard(n * 4)
				return 0, 0, nil
			}
			raw := make([]byte, n*4)
			if !w.readFull(raw) {
				return 0, 0, nil
			}
			valLen := nitems
			switch format {
			case 16:
				valLen = nitems * 2
			case 32:
				valLen = nitems * 4
			}
			if valLen > len(raw) {
				valLen = len(raw)
			}
			return typeAtom, format, raw[:valLen]
		case 0: // error-пакет — запрос отвергнут
			return 0, 0, nil
		default: // обычное событие — обрабатываем на месте
			w.handleX11Event(hdr)
		}
	}
	return 0, 0, nil
}
