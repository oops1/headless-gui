//go:build linux && !android

package window

// x11_moveresize_linux.go — системное перемещение и изменение размера окна
// под X11, через _NET_WM_MOVERESIZE.
//
// Окна движка создаются без рамки (borderless): заголовок и края рисует само
// приложение, и оконный менеджер по умолчанию не даёт их тянуть. Под Win32
// края обслуживает WM_NCHITTEST, под Wayland — xdg_toplevel.resize, а на X11
// не было ничего: SetResizable была пустой функцией, и окно нельзя было ни
// растянуть за край, ни перетащить силами WM — только переставить
// SetPosition, без прилипания к краям экрана.
//
// EWMH даёт для этого одно сообщение: клиент просит менеджер начать
// перетаскивание или ресайз, и дальше всё делает менеджер — ровно как со
// своими рамками.

import (
	"encoding/binary"

	"github.com/oops1/headless-gui/v3/widget"
)

// Направления _NET_WM_MOVERESIZE (EWMH): углы и стороны считаются по часовой
// стрелке от левого верхнего, затем перемещение.
const (
	netWMMoveResizeSizeTopLeft     = 0
	netWMMoveResizeSizeTop         = 1
	netWMMoveResizeSizeTopRight    = 2
	netWMMoveResizeSizeRight       = 3
	netWMMoveResizeSizeBottomRight = 4
	netWMMoveResizeSizeBottom      = 5
	netWMMoveResizeSizeBottomLeft  = 6
	netWMMoveResizeSizeLeft        = 7
	netWMMoveResizeMove            = 8
	netWMMoveResizeCancel          = 11
)

// SetResizable запоминает, разрешено ли пользователю менять размер окна.
//
// Само ограничение накладывает минимальный и максимальный размер
// (WM_NORMAL_HINTS), а здесь — только признак: за край тянуть можно лишь
// тогда, когда ресайз вообще разрешён.
func (w *X11Window) SetResizable(v bool) { w.resizable.Store(v) }

// BeginMove просит оконный менеджер перетащить окно за курсором.
// Реализует interactiveMover.
func (w *X11Window) BeginMove() bool {
	return w.startMoveResize(netWMMoveResizeMove)
}

// BeginResize просит оконный менеджер тянуть край окна.
// edges — биты widget.NativeEdge*. Реализует interactiveMover.
func (w *X11Window) BeginResize(edges int) bool {
	if !w.resizable.Load() {
		return false
	}
	dir, ok := netWMResizeDirection(edges)
	if !ok {
		return false
	}
	return w.startMoveResize(dir)
}

// netWMResizeDirection переводит биты краёв в направление EWMH.
//
// Противоположные края разом — бессмыслица (тянуть окно и влево, и вправо
// одновременно нельзя), такой запрос отклоняется.
func netWMResizeDirection(edges int) (int, bool) {
	top := edges&widget.NativeEdgeTop != 0
	bottom := edges&widget.NativeEdgeBottom != 0
	left := edges&widget.NativeEdgeLeft != 0
	right := edges&widget.NativeEdgeRight != 0
	if (top && bottom) || (left && right) {
		return 0, false
	}
	switch {
	case top && left:
		return netWMMoveResizeSizeTopLeft, true
	case top && right:
		return netWMMoveResizeSizeTopRight, true
	case bottom && left:
		return netWMMoveResizeSizeBottomLeft, true
	case bottom && right:
		return netWMMoveResizeSizeBottomRight, true
	case top:
		return netWMMoveResizeSizeTop, true
	case bottom:
		return netWMMoveResizeSizeBottom, true
	case left:
		return netWMMoveResizeSizeLeft, true
	case right:
		return netWMMoveResizeSizeRight, true
	}
	return 0, false
}

// startMoveResize отправляет менеджеру _NET_WM_MOVERESIZE.
//
// Перед этим кнопку нужно отпустить на уровне X: менеджер берёт указатель
// себе, и наш захват мешал бы ему вести окно.
func (w *X11Window) startMoveResize(direction int) bool {
	if w.wid == 0 || w.atomNetWMMoveResize == 0 {
		return false // менеджер не объявил поддержку
	}
	w.x11UngrabPointer()

	px, py := w.pointerRoot()
	var data [5]uint32
	data[0] = uint32(px)
	data[1] = uint32(py)
	data[2] = uint32(direction)
	data[3] = 1 // кнопка мыши: левая
	data[4] = 1 // источник: обычное приложение (EWMH source indication)
	w.x11SendClientMessageTo(w.screen.Root, w.atomNetWMMoveResize, data)
	return true
}

// rememberRoot запоминает позицию указателя в координатах экрана из события
// мыши: _NET_WM_MOVERESIZE просит её, а отдельный запрос к серверу стоил бы
// обращения туда-обратно на каждое нажатие.
func (w *X11Window) rememberRoot(buf []byte) {
	if len(buf) < 24 {
		return
	}
	w.rootX.Store(int32(int16(binary.LittleEndian.Uint16(buf[20:22]))))
	w.rootY.Store(int32(int16(binary.LittleEndian.Uint16(buf[22:24]))))
}

// pointerRoot возвращает запомненную позицию указателя на экране.
func (w *X11Window) pointerRoot() (int, int) {
	return int(w.rootX.Load()), int(w.rootY.Load())
}

// x11UngrabPointer отпускает захват указателя (opcode 27).
//
// Менеджер окон, начиная перетаскивание, берёт указатель себе; наш захват
// ему помешал бы. Вызов безвреден и когда захвата не было.
func (w *X11Window) x11UngrabPointer() {
	buf := make([]byte, 8)
	buf[0] = 27 // UngrabPointer
	binary.LittleEndian.PutUint16(buf[2:4], 2)
	binary.LittleEndian.PutUint32(buf[4:8], 0) // CurrentTime
	w.x11Send(buf)
}

// x11SendClientMessageTo отправляет ClientMessage корневому окну от имени
// нашего: именно так EWMH принимает просьбы к менеджеру окон (событие
// адресовано корню, а window в нём — наше).
func (w *X11Window) x11SendClientMessageTo(dest, msgType uint32, data [5]uint32) {
	ev := make([]byte, 32)
	ev[0] = 33 // ClientMessage
	ev[1] = 32 // format
	binary.LittleEndian.PutUint32(ev[4:8], w.wid)
	binary.LittleEndian.PutUint32(ev[8:12], msgType)
	for i := 0; i < 5; i++ {
		binary.LittleEndian.PutUint32(ev[12+i*4:16+i*4], data[i])
	}

	buf := make([]byte, 44)
	buf[0] = 25 // SendEvent
	buf[1] = 0  // propagate = false
	binary.LittleEndian.PutUint16(buf[2:4], 11)
	binary.LittleEndian.PutUint32(buf[4:8], dest)
	// SubstructureNotify | SubstructureRedirect — маска, которую слушает
	// менеджер окон на корневом окне.
	binary.LittleEndian.PutUint32(buf[8:12], 0x00180000)
	copy(buf[12:], ev)
	w.x11Send(buf)
}
