//go:build linux && !android

package core

import (
	"encoding/binary"
	"image"
	"testing"
)

// XdndProxy на стороне источника (спецификация XDND): окно под курсором может
// отдавать приём другому окну. Сообщения тогда уходят посреднику, а в поле
// window события остаётся сам приёмник.

const tAtomProxy = 214

// xpProps — свойства «сервера»: окно → атом → (тип, значение CARD32/WINDOW).
type xpProp struct{ typ, val uint32 }
type xpProps map[uint32]map[uint32]xpProp

func (p xpProps) set(win, atom, typ, val uint32) {
	if p[win] == nil {
		p[win] = map[uint32]xpProp{}
	}
	p[win][atom] = xpProp{typ, val}
}

// reply отвечает на GetProperty (опкод 20) и TranslateCoordinates (40) по
// цепочке chain: потомок окна cur под точкой.
func (p xpProps) reply(chain map[uint32]uint32, getProps *int) func(req []byte) []byte {
	return func(req []byte) []byte {
		rep := make([]byte, 32)
		rep[0] = 1
		switch req[0] {
		case 40:
			binary.LittleEndian.PutUint32(rep[8:12], chain[binary.LittleEndian.Uint32(req[8:12])])
			return rep
		case 20:
			if getProps != nil {
				*getProps++
			}
			win := binary.LittleEndian.Uint32(req[4:8])
			atom := binary.LittleEndian.Uint32(req[8:12])
			v, ok := p[win][atom]
			if !ok {
				return rep // свойства нет: type = None
			}
			rep[1] = 32
			binary.LittleEndian.PutUint32(rep[4:8], 1)
			binary.LittleEndian.PutUint32(rep[8:12], v.typ)
			binary.LittleEndian.PutUint32(rep[16:20], 1)
			return binary.LittleEndian.AppendUint32(rep, v.val)
		}
		return nil
	}
}

// Поиск приёмника с XdndProxy: XdndAware читается у посредника, а приёмником
// остаётся окно под курсором.
func TestX11_FindDndTarget_Proxy(t *testing.T) {
	w, f := newX11DragWindow(t)
	w.atomXdndProxy = tAtomProxy
	chain := map[uint32]uint32{2: 10, 10: 11}
	props := xpProps{}
	f.reply = props.reply(chain, nil)

	// У окна 11 нет XdndAware, зато есть XdndProxy → окно 20; у посредника
	// XdndProxy указывает на него же и есть XdndAware v5.
	props.set(11, tAtomProxy, x11AtomWindow, 20)
	props.set(20, tAtomProxy, x11AtomWindow, 20)
	props.set(20, tAtomAware, 4, 5)
	if win, ver := w.x11FindDndTarget(10, 20); win != 11 || ver != 5 {
		t.Fatalf("приёмник (%d, v%d), ждал окно под курсором (11, v5)", win, ver)
	}
	if p := w.dndProxy(11); p != 20 {
		t.Errorf("посредник %d, ждал 20", p)
	}
}

// Посредник, не прошедший проверку, игнорируется: устаревшее свойство
// упавшей программы, не тот тип, указатель на самого себя.
func TestX11_ReadDndProxy_Validation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(p xpProps)
		want  uint32
	}{
		{"верный", func(p xpProps) {
			p.set(11, tAtomProxy, x11AtomWindow, 20)
			p.set(20, tAtomProxy, x11AtomWindow, 20)
		}, 20},
		{"у посредника нет своего XdndProxy", func(p xpProps) {
			p.set(11, tAtomProxy, x11AtomWindow, 20)
		}, 0},
		{"у посредника XdndProxy на другое окно", func(p xpProps) {
			p.set(11, tAtomProxy, x11AtomWindow, 20)
			p.set(20, tAtomProxy, x11AtomWindow, 30)
		}, 0},
		{"не тип WINDOW", func(p xpProps) {
			p.set(11, tAtomProxy, 4 /*ATOM*/, 20)
			p.set(20, tAtomProxy, x11AtomWindow, 20)
		}, 0},
		{"окно указывает на себя", func(p xpProps) {
			p.set(11, tAtomProxy, x11AtomWindow, 11)
		}, 0},
		{"свойства нет", func(p xpProps) {}, 0},
	} {
		w, f := newX11DragWindow(t)
		w.atomXdndProxy = tAtomProxy
		props := xpProps{}
		tc.setup(props)
		f.reply = props.reply(nil, nil)
		if got := w.x11ReadDndProxy(11); got != tc.want {
			t.Errorf("%s: посредник %d, ждал %d", tc.name, got, tc.want)
		}
	}
	// Без интернированного атома свойство не читается вовсе.
	w, _ := newX11DragWindow(t)
	if got := w.x11ReadDndProxy(11); got != 0 {
		t.Errorf("без атома XdndProxy посредник %d", got)
	}
}

// Свойство читается один раз на окно за перетаскивание, а не на каждое
// движение мыши.
func TestX11_DndProxy_Cached(t *testing.T) {
	w, f := newX11DragWindow(t)
	w.atomXdndProxy = tAtomProxy
	props := xpProps{}
	props.set(11, tAtomProxy, x11AtomWindow, 20)
	props.set(20, tAtomProxy, x11AtomWindow, 20)
	reads := 0
	f.reply = props.reply(nil, &reads)
	for i := 0; i < 5; i++ {
		if p := w.dndProxy(11); p != 20 {
			t.Fatalf("посредник %d", p)
		}
	}
	if reads != 2 { // XdndProxy окна и проверочное чтение у посредника
		t.Errorf("чтений свойства %d, ждал 2 (кэш на окно)", reads)
	}
	w.drag.proxies = nil // кэш живёт до конца перетаскивания
	w.dndProxy(11)
	if reads != 4 {
		t.Errorf("после сброса кэша чтений %d, ждал 4", reads)
	}
}

// Источник шлёт Enter/Position/Drop/Leave посреднику, а в поле window события
// — приёмник.
func TestX11_DragSource_MessagesViaProxy(t *testing.T) {
	// SendEvent: получатель @4, а событие с @12; его поле window — @16.
	via := func(atom, to, window uint32) func(r []byte) bool {
		return func(r []byte) bool {
			return r[0] == 25 && len(r) >= 44 && binary.LittleEndian.Uint32(r[4:8]) == to &&
				r[12] == 33 && binary.LittleEndian.Uint32(r[16:20]) == window &&
				binary.LittleEndian.Uint32(r[20:24]) == atom
		}
	}

	w, f := newX11DragWindow(t)
	got := startDrag(t, w, f, DragEffectCopy, 50, 5)
	w.drag.proxyOf = func(win uint32) uint32 {
		if win == 50 {
			return 99
		}
		return 0
	}

	w.handleX11Event(motion(300, 200, 0))
	for _, atom := range []uint32{tAtomEnter, tAtomPosition} {
		if f.find(via(atom, 99, 50)) == nil {
			t.Fatalf("сообщение %d не ушло посреднику 99 с window=50", atom)
		}
		if f.find(clientMsg(atom, 50)) != nil {
			t.Errorf("сообщение %d ушло самому приёмнику, а не посреднику", atom)
		}
	}
	w.handleX11Event(cm(tAtomStatus, [5]uint32{50, 1, 0, 0, tAtomCopy}))

	// Сброс — тоже через посредника.
	w.handleX11Event(buttonEv(5, 1))
	if f.find(via(tAtomDrop, 99, 50)) == nil {
		t.Fatal("XdndDrop не ушёл посреднику")
	}
	w.dragFinish(0)
	_ = got

	// Отмена Esc: XdndLeave посреднику.
	w2, f2 := newX11DragWindow(t)
	startDrag(t, w2, f2, DragEffectCopy, 50, 5)
	w2.drag.proxyOf = w.drag.proxyOf
	w2.handleX11Event(motion(300, 200, 0))
	w2.dragCancel()
	if f2.find(via(tAtomLeave, 99, 50)) == nil {
		t.Error("XdndLeave не ушёл посреднику")
	}

	// Смена окна: Leave старому — его посреднику, Enter новому — без посредника.
	w3, f3 := newX11DragWindow(t)
	cur := uint32(50)
	startDrag(t, w3, f3, DragEffectCopy, 50, 5)
	w3.drag.find = func(rx, ry int) (uint32, int) { return cur, 5 }
	w3.drag.proxyOf = w.drag.proxyOf
	w3.handleX11Event(motion(1, 1, 0))
	cur = 60
	w3.handleX11Event(motion(2, 2, 0))
	if f3.find(via(tAtomLeave, 99, 50)) == nil {
		t.Error("Leave старому окну не ушёл его посреднику")
	}
	if f3.find(via(tAtomEnter, 60, 60)) == nil {
		t.Error("Enter окну без посредника ушёл не ему самому")
	}
}

// X11 значка не рисует: окно не объявляет fileDragImageSource, и Window честно
// отвечает DragImageSupported() == false, а перетаскивание всё равно идёт.
func TestX11_NoDragImage(t *testing.T) {
	w, f := newX11DragWindow(t)
	if _, ok := any(w).(fileDragImageSource); ok {
		t.Fatal("X11Window объявил fileDragImageSource, хотя значок не рисует")
	}
	win := &Window{surface: surface{native: w, scale: 1}}
	if win.DragImageSupported() {
		t.Error("DragImageSupported true на X11")
	}
	w.btnDown.Store(1)
	if !win.BeginDragFilesImage([]string{"/tmp/a"}, DragEffectCopy, image.NewRGBA(image.Rect(0, 0, 8, 8)), image.Pt(1, 1)) {
		t.Fatal("BeginDragFilesImage на X11 не начал перетаскивание без значка")
	}
	if f.find(clientMsg(tAtomBegin, 1)) == nil {
		t.Error("служебное сообщение насосу не отправлено")
	}
}

// Окно без XdndProxy: поведение прежнее — получатель и window совпадают.
func TestX11_DragSource_NoProxyUnchanged(t *testing.T) {
	w, f := newX11DragWindow(t)
	startDrag(t, w, f, DragEffectCopy, 50, 5)
	w.handleX11Event(motion(300, 200, 0))
	enter := f.find(clientMsg(tAtomEnter, 50))
	if enter == nil || binary.LittleEndian.Uint32(enter[16:20]) != 50 {
		t.Fatalf("Enter без посредника: %v", enter)
	}
}
