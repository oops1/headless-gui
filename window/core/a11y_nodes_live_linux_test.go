//go:build linux && !android

package core

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/platform/dbus"
	"github.com/oops1/headless-gui/v3/widget"
)

// Живая шина доступности: действия по узлам и состояние радиокнопки через
// настоящий D-Bus. Тесты пропускаются там, где шины доступности нет.

// newATSPILiveBridge поднимает мост на шине поверх произвольного корня.
func newATSPILiveBridge(t *testing.T, root widget.Widget) (*atspiBridge, *engine.Engine) {
	t.Helper()
	t.Setenv("HEADLESS_GUI_A11Y", "1")
	if _, err := atspiBusAddress(); err != nil {
		t.Skipf("нет шины доступности: %v", err)
	}
	eng := engine.New(500, 400, 20)
	eng.SetRoot(root)
	win := New(eng, "Живое окно")
	win.scale = 1
	b := &atspiBridge{win: win}
	if err := b.start(); err != nil {
		t.Skipf("мост не поднялся (нет реестра AT-SPI?): %v", err)
	}
	t.Cleanup(b.stop)
	return b, eng
}

// dialAT открывает клиента шины доступности.
func dialAT(t *testing.T) *dbus.Conn {
	t.Helper()
	addr, err := atspiBusAddress()
	if err != nil {
		t.Skipf("нет шины доступности: %v", err)
	}
	cl, err := dbus.Dial(addr)
	if err != nil {
		t.Fatalf("клиент шины доступности: %v", err)
	}
	t.Cleanup(cl.Close)
	return cl
}

// DoAction и GrabFocus на узле владельца, посланные клиентом по шине, доходят
// до обработчика владельца с идентификатором узла.
func TestATSPILive_NodeActionsOnWire(t *testing.T) {
	root := widget.NewWindow("Живые узлы", 500, 400)
	o := &nodeOwner{reply: true}
	o.SetBounds(image.Rect(10, 50, 210, 150))
	root.AddChild(o)
	b, _ := newATSPILiveBridge(t, root)
	cl := dialAT(t)

	call := func(path, iface, member, sig string, args ...any) *dbus.Message {
		t.Helper()
		reply, err := cl.Call(b.appName, path, iface, member, sig, args)
		if err != nil {
			t.Fatalf("%s %s.%s: %v", path, iface, member, err)
		}
		return reply
	}
	right := atspiPath(idOfNamed(t, b, "правая"))

	// Узел владельца объявляет Action.
	var hasAction bool
	for _, s := range call(right, ifaceAccessible, "GetInterfaces", "").Body[0].([]string) {
		hasAction = hasAction || s == ifaceAction
	}
	if !hasAction {
		t.Fatal("узел владельца не объявил интерфейс Action")
	}
	if n := call(right, dbus.IfaceProperties, "Get", "ss", ifaceAction, "NActions").Body[0].(dbus.Variant).Val; n != int32(1) {
		t.Errorf("NActions = %v", n)
	}

	if ok := call(right, ifaceAction, "DoAction", "i", int32(0)).Body[0]; ok != true {
		t.Errorf("DoAction по шине вернул %v", ok)
	}
	if ok := call(right, ifaceComponent, "GrabFocus", "").Body[0]; ok != true {
		t.Errorf("GrabFocus по шине вернул %v", ok)
	}
	if len(o.calls) != 2 || o.calls[0] != "R:invoke" || o.calls[1] != "R:focus" {
		t.Fatalf("владелец получил %v, ожидалось [R:invoke R:focus]", o.calls)
	}

	// Отказ владельца доходит клиенту как false.
	o.reply = false
	if ok := call(right, ifaceAction, "DoAction", "i", int32(0)).Body[0]; ok != false {
		t.Errorf("отказ владельца: DoAction вернул %v", ok)
	}
}

// Радиокнопка: по шине читается checked, а смена выбора шлёт событие
// StateChanged(checked), а не selected.
func TestATSPILive_RadioCheckedStateAndEvent(t *testing.T) {
	root := widget.NewWindow("Живое радио", 500, 400)
	a := widget.NewRadioButton("Первая", "atspi-live-radio")
	a.SetBounds(image.Rect(10, 50, 160, 80))
	c := widget.NewRadioButton("Вторая", "atspi-live-radio")
	c.SetBounds(image.Rect(10, 90, 160, 120))
	root.AddChild(a)
	root.AddChild(c)
	a.SetSelected(true)
	b, _ := newATSPILiveBridge(t, root)
	cl := dialAT(t)

	type ev struct {
		path, detail string
		on           int32
	}
	events := make(chan ev, 32)
	cl.OnSignal(func(msg *dbus.Message) {
		if msg.Interface != ifaceEventObject || msg.Member != "StateChanged" || msg.Sender != b.appName {
			return
		}
		e := ev{path: msg.Path}
		e.detail, _ = msg.Body[0].(string)
		e.on, _ = msg.Body[1].(int32)
		select {
		case events <- e:
		default:
		}
	})
	if err := cl.AddMatch("type='signal',interface='" + ifaceEventObject + "'"); err != nil {
		t.Fatalf("AddMatch: %v", err)
	}

	state := func(name string) []uint32 {
		t.Helper()
		reply, err := cl.Call(b.appName, atspiPath(idOfNamed(t, b, name)), ifaceAccessible, "GetState", "", nil)
		if err != nil {
			t.Fatalf("GetState %q: %v", name, err)
		}
		var set []uint32
		for _, s := range reply.Body[0].([]any) {
			set = append(set, s.(uint32))
		}
		return set
	}
	if s := state("Первая"); !hasBit(s, atspiStateChecked) || hasBit(s, atspiStateSelected) {
		t.Errorf("выбранная радиокнопка по шине: %v", s)
	}
	if s := state("Вторая"); hasBit(s, atspiStateChecked) {
		t.Errorf("невыбранная радиокнопка по шине отмечена: %v", s)
	}

	// Выбор второй: событие checked=1 у неё и checked=0 у первой. Пути берём
	// ДО выбора: idOfNamed пересобирает снимок, и разницу, которую должен
	// заметить цикл событий моста, он съел бы сам.
	secondPath := atspiPath(idOfNamed(t, b, "Вторая"))
	firstPath := atspiPath(idOfNamed(t, b, "Первая"))
	c.SetSelected(true)
	deadline := time.After(5 * time.Second)
	var gotOn, gotOff bool
	for !gotOn || !gotOff {
		select {
		case e := <-events:
			if e.detail == "selected" {
				t.Fatalf("радиокнопка прислала selected вместо checked: %+v", e)
			}
			if e.detail != "checked" {
				continue
			}
			if e.path == secondPath && e.on == 1 {
				gotOn = true
			}
			if e.path == firstPath && e.on == 0 {
				gotOff = true
			}
		case <-deadline:
			t.Fatalf("события checked не пришли (включили=%v выключили=%v)", gotOn, gotOff)
		}
	}
	if s := state("Вторая"); !hasBit(s, atspiStateChecked) {
		t.Errorf("после выбора вторая не отмечена: %v", s)
	}
}
