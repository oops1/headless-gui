//go:build linux && !android

package x11

// x11_drag_linux.go — окно как ИСТОЧНИК перетаскивания файлов по XDND v5
// (v3.34). Приёмная сторона — dnd_linux.go.
//
// Порядок:
//  1. BeginDragFiles (любая горутина) только запоминает запрос и шлёт окну
//     служебное ClientMessage. Всё остальное выполняет насос событий окна:
//     только он вправе читать ответы сервера (запрос с ответом из другой
//     горутины отнял бы чужой reply).
//  2. Насос получает сообщение: объявляет окно владельцем XdndSelection и
//     захватывает указатель (GrabPointer) и клавиатуру (Esc).
//  3. На каждое движение мыши ищется окно под курсором с XdndAware (спуск от
//     корня TranslateCoordinates'ом). Смена окна — XdndLeave старому и
//     XdndEnter новому; затем XdndPosition, а следующий уходит после
//     XdndStatus приёмника.
//  4. Отпускание кнопки над принявшим окном — XdndDrop; приёмник забирает
//     данные запросом SelectionRequest к XdndSelection (отдаём text/uri-list)
//     и сообщает XdndFinished. Иначе — XdndLeave и отмена.
//
// Итог в done: действие из XdndFinished (или последнего XdndStatus для
// приёмников старше версии 2); 0 — отмена, отказ, Esc, захват не удался,
// приёмник не ответил за 10 секунд.
//
// XdndProxy: окно под курсором может отдавать приём другому окну (свойство
// XdndProxy, проверенное по спецификации); XdndAware тогда ищется на посреднике,
// а сообщения уходят ему же, с окном-приёмником в поле window (xdndSend).
//
// Чего нет: значка и курсора перетаскивания (остаётся курсор окна под
// указателем): XDND значка не определяет, а окно-призрак без композитора и
// расширения SHAPE нечем сделать прозрачным и неуловимым для поиска приёмника
// (он спускается по окнам под указателем и нашёл бы само призрачное окно), —
// поэтому X11Window не реализует fileDragImageSource, а BeginDragFilesImage
// начинает обычное перетаскивание (DragImageSupported() == false). Списка
// типов длиннее одного (text/uri-list) тоже нет.

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// Фазы перетаскивания.
const (
	dragNone     = iota
	dragPending  // запрос принят, насос ещё не начал
	dragDragging // указатель захвачен, ведём приёмник
	dragDropped  // XdndDrop отправлен, ждём XdndFinished
)

// x11DragFinishTimeout — сколько ждём XdndFinished после сброса (переменная
// ради теста).
var x11DragFinishTimeout = 10 * time.Second

const (
	// x11DragStatusTimeout — сколько ждём XdndStatus, прежде чем слать
	// следующую позицию: приёмник мог зависнуть.
	x11DragStatusTimeout = 300 * time.Millisecond
)

// x11DragPos — последнее положение указателя при перетаскивании.
type x11DragPos struct {
	x, y  int // корневые координаты
	time  uint32
	state uint16 // модификаторы события (Shift 1, Control 4, Mod1 8)
}

// x11Drag — состояние перетаскивания, начатого окном.
type x11Drag struct {
	active atomic.Bool // фаза dragging/dropped: насос отдаёт события источнику

	mu     sync.Mutex
	phase  int
	body   string // text/uri-list
	effect native.DragEffect
	done   func(native.DragEffect)
	button byte // кнопка X (1..3), с которой начали
	grab   bool // указатель захвачен нами

	target        uint32 // окно-приёмник; 0 — нет
	proxy         uint32 // его XdndProxy (кому слать сообщения); 0 — прокси нет
	targetVer     int
	sentEnter     bool
	accepted      bool              // последний XdndStatus: приёмник принимает
	action        native.DragEffect // действие из последнего XdndStatus
	statusPending bool              // XdndPosition отправлен, XdndStatus ещё нет
	posSentAt     time.Time
	queued        bool // позиция ждёт XdndStatus
	queuedPos     x11DragPos

	busy    bool // насос уже разбирает движение (вложенные события лишь обновляют pos)
	havePos bool
	pos     x11DragPos

	timer *time.Timer

	// find ищет приёмник под точкой: окно и версию XDND. nil — настоящий
	// поиск по серверу; подменяется тестом.
	find func(rx, ry int) (win uint32, ver int)
	// proxyOf возвращает окно-посредник приёмника win (XdndProxy) или 0. nil —
	// настоящее чтение свойства; подменяется тестом. proxies — его кэш на время
	// перетаскивания: свойство читается по разу на окно, а не на каждое
	// движение мыши.
	proxyOf func(win uint32) uint32
	proxies map[uint32]uint32
}

// noteButton ведёт маску зажатых кнопок мыши.
func (w *X11Window) noteButton(btn byte, down bool) {
	if btn < 1 || btn > 3 {
		return
	}
	bit := uint32(1) << (btn - 1)
	for {
		old := w.btnDown.Load()
		next := old | bit
		if !down {
			next = old &^ bit
		}
		if w.btnDown.CompareAndSwap(old, next) {
			return
		}
	}
}

// BeginDragFiles начинает перетаскивание файлов (fileDragSource). Кнопка мыши
// должна быть зажата. Возврат true означает «запрос принят»: захват указателя
// происходит позже, в насосе событий, и его неудача придёт как done(0).
func (w *X11Window) BeginDragFiles(paths []string, effect native.DragEffect, done func(native.DragEffect)) bool {
	if w.wid == 0 || w.conn == nil || done == nil || len(paths) == 0 || effect&native.DragEffectAll == 0 ||
		w.atomXdndSelection == 0 || w.atomTextUriList == 0 || w.atomDragBegin == 0 {
		return false
	}
	held := w.btnDown.Load() & 7
	if held == 0 {
		return false
	}
	button := byte(1)
	for held&1 == 0 {
		held >>= 1
		button++
	}
	d := &w.drag
	d.mu.Lock()
	if d.phase != dragNone {
		d.mu.Unlock()
		return false // одно перетаскивание за раз
	}
	d.phase = dragPending
	d.body, d.effect, d.done, d.button = native.BuildURIList(paths), effect&native.DragEffectAll, done, button
	d.target, d.sentEnter, d.accepted, d.statusPending, d.queued = 0, false, false, false, false
	d.busy, d.havePos, d.grab = false, false, false
	d.proxy, d.proxies = 0, nil
	d.mu.Unlock()
	// Самому себе: событие придёт в насос, и тот начнёт перетаскивание.
	w.x11SendClientMessage(w.wid, w.atomDragBegin, [5]uint32{})
	return true
}

// dragBegin — на насосе: объявляем владение XdndSelection и захватываем
// указатель.
func (w *X11Window) dragBegin() {
	d := &w.drag
	d.mu.Lock()
	if d.phase != dragPending {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()

	w.x11SetSelectionOwner(w.atomXdndSelection, w.wid)
	if !w.x11GrabPointerSync() {
		w.dragFinish(0)
		return
	}
	w.x11GrabKeyboardSync() // для Esc; без клавиатуры отмена — отпусканием вне приёмника

	d.mu.Lock()
	d.phase, d.grab = dragDragging, true
	d.mu.Unlock()
	d.active.Store(true)
	// Приёмник ищется с первого движения мыши: на захвате оно уже в пути.
}

// dragHandleEvent — на насосе, пока идёт перетаскивание: забирает события,
// которые принадлежат источнику. false — событие обычное.
func (w *X11Window) dragHandleEvent(buf []byte) bool {
	d := &w.drag
	d.mu.Lock()
	phase, button := d.phase, d.button
	d.mu.Unlock()

	evType := buf[0] & 0x7F
	switch evType {
	case 33: // ClientMessage: ответы приёмника
		switch binary.LittleEndian.Uint32(buf[8:12]) {
		case w.atomXdndStatus:
			w.dragOnStatus(buf)
			return true
		case w.atomXdndFinished:
			w.dragOnFinished(buf)
			return true
		}
		return false
	case 30: // SelectionRequest: приёмник просит данные
		if binary.LittleEndian.Uint32(buf[16:20]) == w.atomXdndSelection {
			w.dragServeSelection(buf)
			return true
		}
		return false
	}
	if phase != dragDragging {
		return false
	}
	switch evType {
	case 6: // MotionNotify: root_x@20, root_y@22
		p := x11DragPos{
			x:     int(int16(binary.LittleEndian.Uint16(buf[20:22]))),
			y:     int(int16(binary.LittleEndian.Uint16(buf[22:24]))),
			time:  binary.LittleEndian.Uint32(buf[4:8]),
			state: binary.LittleEndian.Uint16(buf[28:30]),
		}
		w.rootX.Store(int32(p.x))
		w.rootY.Store(int32(p.y))
		w.dragMotion(p)
		return true
	case 5: // ButtonRelease
		w.noteButton(buf[1], false)
		if buf[1] == button {
			w.dragRelease(binary.LittleEndian.Uint32(buf[4:8]))
		}
		return true
	case 4: // ButtonPress
		w.noteButton(buf[1], true)
		return true
	case 2: // KeyPress: Esc отменяет
		if w.x11VKForKey(buf[1]) == native.VK_ESCAPE {
			w.dragCancel()
		}
		return true
	case 3: // KeyRelease
		return true
	}
	return false
}

// dragMotion обрабатывает движение: поиск приёмника и сообщения ему. События,
// пришедшие во время поиска (запросы с ответом читают поток), лишь обновляют
// позицию — разбор идёт одним циклом.
func (w *X11Window) dragMotion(p x11DragPos) {
	d := &w.drag
	d.mu.Lock()
	d.pos, d.havePos = p, true
	if d.busy {
		d.mu.Unlock()
		return
	}
	d.busy = true
	for {
		cur := d.pos
		d.havePos = false
		d.mu.Unlock()
		w.dragStep(cur)
		d.mu.Lock()
		if !d.havePos || d.phase != dragDragging {
			d.busy = false
			d.mu.Unlock()
			return
		}
	}
}

// dragStep — один шаг: приёмник под точкой, enter/leave при смене, position.
func (w *X11Window) dragStep(p x11DragPos) {
	d := &w.drag
	find := d.find
	if find == nil {
		find = w.x11FindDndTarget
	}
	win, ver := find(p.x, p.y)

	d.mu.Lock()
	changed := win != d.target
	d.mu.Unlock()
	var proxy uint32
	if changed && win != 0 {
		proxy = w.dndProxy(win) // чтение свойства — вне замка
	}

	d.mu.Lock()
	prev, prevProxy, prevSent := d.target, d.proxy, d.sentEnter
	if changed {
		d.target, d.proxy, d.targetVer, d.sentEnter = win, proxy, ver, false
		d.accepted, d.statusPending, d.queued, d.action = false, false, false, 0
	}
	proxy = d.proxy
	d.mu.Unlock()

	if changed {
		if prev != 0 && prevSent {
			w.sendXdndLeave(prev, prevProxy)
		}
		if win != 0 {
			w.sendXdndEnter(win, proxy, ver)
			d.mu.Lock()
			d.sentEnter = true
			d.mu.Unlock()
		}
	}
	if win == 0 {
		return
	}
	d.mu.Lock()
	if d.statusPending && time.Since(d.posSentAt) < x11DragStatusTimeout {
		d.queued, d.queuedPos = true, p
		d.mu.Unlock()
		return
	}
	d.statusPending, d.posSentAt = true, time.Now()
	d.mu.Unlock()
	w.sendXdndPosition(win, proxy, p)
}

// dragOnStatus — XdndStatus от приёмника: принимает ли он и какое действие.
func (w *X11Window) dragOnStatus(buf []byte) {
	d := &w.drag
	from := binary.LittleEndian.Uint32(buf[12:16])
	flags := binary.LittleEndian.Uint32(buf[16:20])
	action := binary.LittleEndian.Uint32(buf[28:32])

	d.mu.Lock()
	if from != d.target || d.phase != dragDragging {
		d.mu.Unlock()
		return
	}
	d.accepted = flags&1 != 0
	d.action = w.dndActionEffect(action)
	d.statusPending = false
	q, qp, win, proxy := d.queued, d.queuedPos, d.target, d.proxy
	d.queued = false
	if q {
		d.statusPending, d.posSentAt = true, time.Now()
	}
	d.mu.Unlock()
	if q {
		w.sendXdndPosition(win, proxy, qp)
	}
}

// dragOnFinished — XdndFinished: приёмник забрал данные.
func (w *X11Window) dragOnFinished(buf []byte) {
	d := &w.drag
	from := binary.LittleEndian.Uint32(buf[12:16])
	flags := binary.LittleEndian.Uint32(buf[16:20])
	action := binary.LittleEndian.Uint32(buf[20:24])

	d.mu.Lock()
	if from != d.target || d.phase != dragDropped {
		d.mu.Unlock()
		return
	}
	eff := d.action
	if d.targetVer >= 5 {
		// В v5 приёмник сам сообщает, принял ли он данные и каким действием.
		eff = 0
		if flags&1 != 0 {
			eff = w.dndActionEffect(action)
		}
	}
	d.mu.Unlock()
	w.dragFinish(eff)
}

// dragRelease — кнопку отпустили: сброс на принявшее окно или отмена.
func (w *X11Window) dragRelease(time32 uint32) {
	d := &w.drag
	d.mu.Lock()
	if d.phase != dragDragging {
		d.mu.Unlock()
		return
	}
	target, proxy, ver, sent, accepted := d.target, d.proxy, d.targetVer, d.sentEnter, d.accepted
	action := d.action
	d.mu.Unlock()
	w.dragUngrab()

	if target == 0 || !accepted {
		if target != 0 && sent {
			w.sendXdndLeave(target, proxy)
		}
		w.dragFinish(0)
		return
	}
	d.mu.Lock()
	d.phase = dragDropped
	if ver >= 2 {
		d.timer = time.AfterFunc(x11DragFinishTimeout, func() { w.dragTimeout() })
	}
	d.mu.Unlock()
	w.sendXdndDrop(target, proxy, time32)
	if ver < 2 {
		// Приёмник старше v2 XdndFinished не шлёт: итог — последнее согласованное.
		w.dragFinish(action)
	}
}

// dragTimeout — приёмник не прислал XdndFinished.
func (w *X11Window) dragTimeout() {
	d := &w.drag
	d.mu.Lock()
	ok := d.phase == dragDropped
	d.mu.Unlock()
	if ok {
		w.dragFinish(0)
	}
}

// dragCancel — Esc: приёмнику XdndLeave, итог 0.
func (w *X11Window) dragCancel() {
	d := &w.drag
	d.mu.Lock()
	if d.phase != dragDragging {
		d.mu.Unlock()
		return
	}
	target, proxy, sent := d.target, d.proxy, d.sentEnter
	d.mu.Unlock()
	if target != 0 && sent {
		w.sendXdndLeave(target, proxy)
	}
	w.dragFinish(0)
}

// dragUngrab отпускает захваты указателя и клавиатуры.
func (w *X11Window) dragUngrab() {
	d := &w.drag
	d.mu.Lock()
	grabbed := d.grab
	d.grab = false
	d.mu.Unlock()
	if !grabbed {
		return
	}
	req := make([]byte, 8)
	req[0] = 27 // UngrabPointer
	binary.LittleEndian.PutUint16(req[2:4], 2)
	w.x11Send(req)
	req = make([]byte, 8)
	req[0] = 32 // UngrabKeyboard
	binary.LittleEndian.PutUint16(req[2:4], 2)
	w.x11Send(req)
}

// dragFinish заканчивает перетаскивание: захваты и владение снимаются, кнопка
// мыши в движке отпускается (отпускание забрал захват), done получает итог.
// Повторный вызов — no-op.
func (w *X11Window) dragFinish(effect native.DragEffect) {
	d := &w.drag
	d.mu.Lock()
	if d.phase == dragNone {
		d.mu.Unlock()
		return
	}
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	done, button := d.done, d.button
	owned := d.phase != dragPending
	d.phase, d.done, d.body = dragNone, nil, ""
	d.target, d.sentEnter, d.accepted, d.queued, d.statusPending = 0, false, false, false, false
	d.proxy, d.proxies = 0, nil
	d.mu.Unlock()
	d.active.Store(false)
	w.dragUngrab()
	if owned {
		w.x11SetSelectionOwner(w.atomXdndSelection, 0)
	}
	// Кнопка X 1/2/3 — левая/средняя/правая; id движка 0/2/1.
	if w.onMouseButton != nil && button >= 1 && button <= 3 {
		id := map[byte]int{1: 0, 2: 2, 3: 1}[button]
		px, py := w.GetPosition()
		w.btnDown.Store(0)
		w.onMouseButton(int(w.rootX.Load())-px, int(w.rootY.Load())-py, id, false)
	}
	if done != nil {
		done(effect)
	}
}

// dragServeSelection отвечает на SelectionRequest по XdndSelection: список
// типов (TARGETS) или text/uri-list.
func (w *X11Window) dragServeSelection(buf []byte) {
	timestamp := binary.LittleEndian.Uint32(buf[4:8])
	requestor := binary.LittleEndian.Uint32(buf[12:16])
	selection := binary.LittleEndian.Uint32(buf[16:20])
	target := binary.LittleEndian.Uint32(buf[20:24])
	property := binary.LittleEndian.Uint32(buf[24:28])
	if property == 0 {
		property = target
	}
	d := &w.drag
	d.mu.Lock()
	body, live := d.body, d.phase == dragDragging || d.phase == dragDropped
	d.mu.Unlock()

	ok := false
	switch {
	case !live:
	case target == w.atomTargets:
		var data []byte
		for _, a := range []uint32{w.atomTargets, w.atomTextUriList} {
			data = binary.LittleEndian.AppendUint32(data, a)
		}
		w.x11ChangeProperty(requestor, property, 4 /*ATOM*/, 32, data)
		ok = true
	case target == w.atomTextUriList:
		w.x11ChangeProperty(requestor, property, target, 8, []byte(body))
		ok = true
	}
	if !ok {
		property = 0
	}
	w.x11SendSelectionNotify(requestor, timestamp, selection, target, property)
}

// ─── Сообщения XDND источника ────────────────────────────────────────────────

func (w *X11Window) sendXdndEnter(dest, proxy uint32, ver int) {
	if ver > 5 {
		ver = 5
	}
	var data [5]uint32
	data[0] = w.wid
	data[1] = uint32(ver) << 24 // бит 0 снят: типы — в data[2..4]
	data[2] = w.atomTextUriList
	w.xdndSend(dest, proxy, w.atomXdndEnter, data)
}

func (w *X11Window) sendXdndPosition(dest, proxy uint32, p x11DragPos) {
	d := &w.drag
	d.mu.Lock()
	allowed := d.effect
	d.mu.Unlock()
	eff := native.DragEffectForKeys(allowed, p.state&4 != 0, p.state&1 != 0, p.state&8 != 0)
	var data [5]uint32
	data[0] = w.wid
	data[2] = uint32(p.x&0xFFFF)<<16 | uint32(p.y&0xFFFF)
	data[3] = p.time
	data[4] = w.dndEffectAtom(eff)
	w.xdndSend(dest, proxy, w.atomXdndPosition, data)
}

func (w *X11Window) sendXdndLeave(dest, proxy uint32) {
	var data [5]uint32
	data[0] = w.wid
	w.xdndSend(dest, proxy, w.atomXdndLeave, data)
}

func (w *X11Window) sendXdndDrop(dest, proxy, time32 uint32) {
	var data [5]uint32
	data[0] = w.wid
	data[2] = time32
	w.xdndSend(dest, proxy, w.atomXdndDrop, data)
}

// xdndSend отправляет сообщение XDND приёмнику dest. Если у приёмника есть
// XdndProxy (proxy != 0), событие уходит окну-посреднику, а в поле window
// остаётся сам приёмник (так требует спецификация XDND, раздел о прокси).
func (w *X11Window) xdndSend(dest, proxy, msgType uint32, data [5]uint32) {
	to := dest
	if proxy != 0 {
		to = proxy
	}
	w.x11SendClientMessageVia(to, dest, msgType, data)
}

// dndProxy возвращает окно-посредник приёмника win (XdndProxy) с проверкой по
// спецификации: свойство должно быть типа WINDOW, а у самого посредника свой
// XdndProxy обязан указывать на него же — иначе это остаток от упавшей
// программы, и посредник игнорируется. 0 — прокси нет. Результат кэшируется на
// время перетаскивания. Только на насосе событий (читает свойства).
func (w *X11Window) dndProxy(win uint32) uint32 {
	d := &w.drag
	d.mu.Lock()
	if p, ok := d.proxies[win]; ok {
		d.mu.Unlock()
		return p
	}
	of := d.proxyOf
	d.mu.Unlock()
	if of == nil {
		of = w.x11ReadDndProxy
	}
	p := of(win)
	d.mu.Lock()
	if d.proxies == nil {
		d.proxies = map[uint32]uint32{}
	}
	d.proxies[win] = p
	d.mu.Unlock()
	return p
}

// x11ReadDndProxy читает и проверяет XdndProxy окна win (см. dndProxy).
func (w *X11Window) x11ReadDndProxy(win uint32) uint32 {
	if w.atomXdndProxy == 0 || win == 0 {
		return 0
	}
	p := w.x11WindowProp(win, w.atomXdndProxy)
	if p == 0 || p == win {
		return 0
	}
	if w.x11WindowProp(p, w.atomXdndProxy) != p {
		return 0
	}
	return p
}

// x11WindowProp — значение свойства типа WINDOW (format 32) или 0.
func (w *X11Window) x11WindowProp(win, prop uint32) uint32 {
	typ, format, data := w.x11GetProperty(win, prop, false)
	if typ != x11AtomWindow || format != 32 || len(data) < 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(data[:4])
}

// x11AtomWindow — предопределённый атом типа WINDOW.
const x11AtomWindow = 33

// dndEffectAtom — атом действия XDND для DragEffect.
func (w *X11Window) dndEffectAtom(e native.DragEffect) uint32 {
	switch e {
	case native.DragEffectMove:
		return w.atomXdndActionMove
	case native.DragEffectLink:
		return w.atomXdndActionLink
	}
	return w.atomXdndActionCopy
}

// dndActionEffect — обратный перевод атома действия.
func (w *X11Window) dndActionEffect(a uint32) native.DragEffect {
	switch {
	case a == 0:
		return 0
	case a == w.atomXdndActionCopy:
		return native.DragEffectCopy
	case a == w.atomXdndActionMove:
		return native.DragEffectMove
	case a == w.atomXdndActionLink:
		return native.DragEffectLink
	}
	return 0
}

// ─── Запросы с ответом (только на насосе событий) ────────────────────────────

// x11RoundTrip отправляет запрос и читает ответ; события, пришедшие раньше
// ответа, разбираются на месте. nil — сервер вернул ошибку или соединение
// закрыто. Вызывать только из горутины насоса.
func (w *X11Window) x11RoundTrip(req []byte) []byte {
	w.mu.Lock()
	if w.conn == nil {
		w.mu.Unlock()
		return nil
	}
	w.conn.Write(req)
	w.seqNum++
	w.mu.Unlock()

	hdr := make([]byte, 32)
	for i := 0; i < 4096; i++ { // предохранитель от бесконечного цикла
		if !w.readFull(hdr) {
			return nil
		}
		switch hdr[0] {
		case 1:
			extra := int(binary.LittleEndian.Uint32(hdr[4:8])) * 4
			if extra == 0 {
				return hdr
			}
			if extra > native.MaxDnDBytes {
				w.x11Discard(extra)
				return nil
			}
			tail := make([]byte, extra)
			if !w.readFull(tail) {
				return nil
			}
			return append(hdr, tail...)
		case 0:
			return nil
		default:
			w.handleX11Event(append([]byte(nil), hdr...))
		}
	}
	return nil
}

// x11GrabPointerSync захватывает указатель за окном (кнопки и движение —
// только нам). true — захват получен.
func (w *X11Window) x11GrabPointerSync() bool {
	req := make([]byte, 24)
	req[0] = 26 // GrabPointer
	req[1] = 0  // owner-events = false
	binary.LittleEndian.PutUint16(req[2:4], 6)
	binary.LittleEndian.PutUint32(req[4:8], w.wid)
	binary.LittleEndian.PutUint16(req[8:10], 0x4|0x8|0x40) // ButtonPress|ButtonRelease|PointerMotion
	req[10], req[11] = 1, 1                                // оба режима асинхронные
	// confine-to, cursor, time = None / None / CurrentTime
	r := w.x11RoundTrip(req)
	return r != nil && r[1] == 0 // GrabSuccess
}

// x11GrabKeyboardSync захватывает клавиатуру, чтобы поймать Esc.
func (w *X11Window) x11GrabKeyboardSync() bool {
	req := make([]byte, 16)
	req[0] = 31 // GrabKeyboard
	binary.LittleEndian.PutUint16(req[2:4], 4)
	binary.LittleEndian.PutUint32(req[4:8], w.wid)
	req[12], req[13] = 1, 1
	r := w.x11RoundTrip(req)
	return r != nil && r[1] == 0
}

// x11DndAwareVersion — версия XDND окна win (свойство XdndAware) или 0.
func (w *X11Window) x11DndAwareVersion(win uint32) int {
	typ, _, data := w.x11GetProperty(win, w.atomXdndAware, false)
	if typ == 0 || len(data) < 4 {
		return 0
	}
	return int(binary.LittleEndian.Uint32(data[:4]))
}

// x11FindDndTarget ищет под точкой (корневые координаты) окно с XdndAware:
// спуск от корня, на каждом уровне — потомок под точкой и свойство. Берётся
// первое подходящее окно сверху: у оконных менеджеров с рамкой это окно
// клиента. win == 0 — приёмника нет.
func (w *X11Window) x11FindDndTarget(rx, ry int) (uint32, int) {
	root := w.rootWin
	if root == 0 {
		root = w.screen.Root
	}
	cur := root
	for depth := 0; depth < 16; depth++ {
		req := make([]byte, 16)
		req[0] = 40 // TranslateCoordinates
		binary.LittleEndian.PutUint16(req[2:4], 4)
		binary.LittleEndian.PutUint32(req[4:8], root)
		binary.LittleEndian.PutUint32(req[8:12], cur)
		binary.LittleEndian.PutUint16(req[12:14], uint16(int16(rx)))
		binary.LittleEndian.PutUint16(req[14:16], uint16(int16(ry)))
		r := w.x11RoundTrip(req)
		if r == nil {
			return 0, 0
		}
		child := binary.LittleEndian.Uint32(r[8:12])
		if child == 0 {
			return 0, 0
		}
		cur = child
		if w.atomXdndAware == 0 {
			continue
		}
		// Окно с XdndProxy: приёмником отвечает посредник, и XdndAware ищется
		// на нём; у самого окна свойства может не быть вовсе.
		if proxy := w.dndProxy(cur); proxy != 0 {
			if ver := w.x11DndAwareVersion(proxy); ver >= 3 {
				return cur, ver
			}
		}
		if ver := w.x11DndAwareVersion(cur); ver >= 3 {
			return cur, ver
		}
	}
	return 0, 0
}
