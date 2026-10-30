package core

import (
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/oops1/headless-gui/v4/widget"
)

// Захват указателя (Window.SetPointerLock): разбор RAWINPUT, склейка сдвигов,
// подавление абсолютных движений и путь от бэкенда до приложения.

// plRawMouse собирает RAWINPUT мыши с заголовком под указатель размером ptrSize.
func plRawMouse(ptrSize int, typ uint32, flags uint16, x, y int32) []byte {
	hdr := 8 + 2*ptrSize
	b := make([]byte, hdr+24)
	binary.LittleEndian.PutUint32(b[0:], typ)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)))
	binary.LittleEndian.PutUint16(b[hdr:], flags)
	binary.LittleEndian.PutUint32(b[hdr+12:], uint32(x))
	binary.LittleEndian.PutUint32(b[hdr+16:], uint32(y))
	return b
}

func TestRelMotionQueue_CoalescesIntoOneDelivery(t *testing.T) {
	var q relMotionQueue
	var runs []func()
	run := func(f func()) { runs = append(runs, f) }
	var got [][2]float64
	send := func(dx, dy float64) { got = append(got, [2]float64{dx, dy}) }

	q.add(run, 1, 2, send)
	q.add(run, 3, -1, send)
	q.add(run, 0.5, 0.5, send)
	if len(runs) != 1 {
		t.Fatalf("в очередь поставлено %d сбросов, ждал 1", len(runs))
	}
	runs[0]()
	if !reflect.DeepEqual(got, [][2]float64{{4.5, 1.5}}) {
		t.Fatalf("доставлено %v, ждал одну сумму [4.5 1.5]", got)
	}
	// После сброса следующий сдвиг ставит новый.
	q.add(run, 1, 0, send)
	if len(runs) != 2 {
		t.Fatalf("после сброса новый сдвиг не поставлен в очередь: %d", len(runs))
	}
	runs[1]()
	// Взаимно гасящиеся сдвиги не доставляются пустышкой.
	q.add(run, 2, 0, send)
	q.add(run, -2, 0, send)
	n := len(got)
	runs[2]()
	if len(got) != n {
		t.Errorf("нулевая сумма доставлена: %v", got[n:])
	}
}

// plNative — окно ОС с захватом указателя.
type plNative struct {
	inputNative
	motion  func(dx, dy float64)
	changed func(active bool)
	calls   []bool
	refuse  bool
}

func (f *plNative) SetPointerLock(on bool) bool {
	f.calls = append(f.calls, on)
	return !on || !f.refuse
}
func (f *plNative) SetOnRelativeMotion(fn func(dx, dy float64))  { f.motion = fn }
func (f *plNative) SetOnPointerLockChanged(fn func(active bool)) { f.changed = fn }

func plWindow(t *testing.T) (*Window, *plNative, *queueEngine) {
	t.Helper()
	eng := &queueEngine{}
	fn := &plNative{}
	win := &Window{}
	win.eng, win.native, win.scale = eng, fn, 1
	win.setupInput()
	if fn.motion == nil || fn.changed == nil {
		t.Fatal("окно не подключило приём сдвигов и смены захвата")
	}
	return win, fn, eng
}

func plPump(eng *queueEngine, fn *plNative) {
	for {
		n := eng.run()
		inv := fn.invoked
		fn.invoked = nil
		for _, f := range inv {
			f()
		}
		if n == 0 && len(inv) == 0 {
			return
		}
	}
}

// Сдвиги от бэкенда доходят до приложения на горутине движка суммой.
func TestPointerLock_RelativeMotionDelivered(t *testing.T) {
	win, fn, eng := plWindow(t)
	var got []widget.RelativeMotion
	win.SetOnRelativeMotion(func(m widget.RelativeMotion) { got = append(got, m) })

	fn.motion(2, 3)
	fn.motion(-1, 4)
	if len(got) != 0 {
		t.Fatal("сдвиг выполнен на потоке бэкенда, а не в очереди движка")
	}
	plPump(eng, fn)
	if !reflect.DeepEqual(got, []widget.RelativeMotion{{DX: 1, DY: 7}}) {
		t.Errorf("доставлено %v, ждал [{1 7}]", got)
	}
	// Без приёмника сдвиги теряются без паники.
	win.SetOnRelativeMotion(nil)
	fn.motion(1, 1)
	plPump(eng, fn)
	if len(got) != 1 {
		t.Errorf("снятый приёмник получил сдвиг: %v", got)
	}
}

// Пока захват действует, абсолютные движения мыши не доходят до движка; после
// снятия — снова доходят.
func TestPointerLock_AbsoluteMoveSuppressedWhileActive(t *testing.T) {
	_, fn, eng := plWindow(t)
	fn.onMove(10, 10)
	plPump(eng, fn)
	wantLog(t, eng, "move 10,10")

	fn.changed(true)
	fn.onMove(20, 20)
	plPump(eng, fn)
	wantLog(t, eng) // тишина

	fn.changed(false)
	fn.onMove(30, 30)
	plPump(eng, fn)
	wantLog(t, eng, "move 30,30")
}

// Приложение узнаёт о начале и конце захвата; PointerLocked отражает состояние.
func TestPointerLock_ChangedNotified(t *testing.T) {
	win, fn, eng := plWindow(t)
	var seen []bool
	win.SetOnPointerLockChanged(func(a bool) { seen = append(seen, a) })
	fn.changed(true)
	if !win.PointerLocked() {
		t.Error("PointerLocked=false при действующем захвате")
	}
	fn.changed(false)
	if win.PointerLocked() {
		t.Error("PointerLocked=true после снятия")
	}
	plPump(eng, fn)
	if !reflect.DeepEqual(seen, []bool{true, false}) {
		t.Errorf("уведомления %v, ждал [true false]", seen)
	}
}

func TestPointerLock_SetPointerLockResult(t *testing.T) {
	win, fn, _ := plWindow(t)
	if !win.SetPointerLock(true) || !win.lock.wanted.Load() {
		t.Error("захват не принят")
	}
	if !win.SetPointerLock(false) || win.lock.wanted.Load() {
		t.Error("снятие не прошло")
	}
	if !reflect.DeepEqual(fn.calls, []bool{true, false}) {
		t.Errorf("вызовы бэкенда %v", fn.calls)
	}
	// Бэкенд отказал: false, приложение не считается захватившим.
	fn.refuse = true
	if win.SetPointerLock(true) || win.lock.wanted.Load() {
		t.Error("отказ бэкенда принят за успех")
	}
}

// Бэкенд без захвата (macOS): включить нельзя, выключить — всегда можно.
func TestPointerLock_UnsupportedBackend(t *testing.T) {
	win := &Window{}
	win.native = &inputNative{}
	if win.SetPointerLock(true) {
		t.Error("захват включён на бэкенде без поддержки")
	}
	if !win.SetPointerLock(false) {
		t.Error("выключение на бэкенде без поддержки вернуло false")
	}
}
