//go:build linux && !android

package window

import (
	"testing"
	"time"
)

// Смена мониторов на Wayland, на проводе: новый и исчезнувший wl_output,
// перенастройка и логические размеры будят подписчика.

func wlMonChgAnnounce(c *wlTestConn, name, ver uint32) {
	c.w.registryID = 2
	body := append(wlU32(name), wlStr("wl_output")...)
	c.w.handleEvent(2, wlRegistryEvGlobal, append(body, wlU32(ver)...))
}

type wlMonCounter struct{ n int }

func (m *wlMonCounter) fn() { m.n++ }

// Монитор объявлен после старта: привязывается на ходу (bind wl_output) и будит.
func TestWaylandMonitorChange_NewOutputBoundLive(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.outs.live.Store(true) // стартовая привязка выводов выполнена
	var cnt wlMonCounter
	c.w.watchMonitors(cnt.fn)

	wlMonChgAnnounce(c, 50, 4)
	if cnt.n != 1 {
		t.Errorf("новый wl_output разбудил %d раз, ждал 1", cnt.n)
	}
	msgs := splitWlMsgs(t, c.read(t))
	bind, ok := findWlMsg(msgs, 2, wlRegistryBind)
	if !ok || bind.args[0] != 50 {
		t.Fatalf("wl_output не привязан: %+v", msgs)
	}
	id := bind.args[len(bind.args)-1]
	if !c.w.outs.isOutput(id) {
		t.Error("привязанный wl_output не занесён в список мониторов")
	}

	// Повтор того же глобала второй привязки не даёт.
	wlMonChgAnnounce(c, 50, 4)
	if cnt.n != 1 {
		t.Errorf("повторное объявление дало вызов: %d", cnt.n)
	}

	// Его события тоже будят: геометрия, режим, done.
	c.w.handleEvent(id, wlOutputEvGeometry, wlGeometry(0, 0, "A", "B", 0))
	c.w.handleEvent(id, wlOutputEvMode, wlU32(wlOutputModeCurrent, 1920, 1080, 60000))
	c.w.handleEvent(id, 2 /* wl_output.done */, nil)
	if cnt.n != 4 {
		t.Errorf("события вывода дали %d вызовов, ждал 4", cnt.n)
	}
	if len(c.w.monitors()) != 1 {
		t.Errorf("мониторов %d, ждал 1", len(c.w.monitors()))
	}
}

// До стартовой привязки объявленные глобалы не привязываются на ходу: их
// подберёт bindOutputs вместе со всеми.
func TestWaylandMonitorChange_BeforeInitialBindNotLive(t *testing.T) {
	c := newWlTestWindow(t)
	var cnt wlMonCounter
	c.w.watchMonitors(cnt.fn)
	wlMonChgAnnounce(c, 50, 4)
	if cnt.n != 0 {
		t.Errorf("до стартовой привязки вызван подписчик: %d", cnt.n)
	}
	if _, ok := c.w.gOutputs[50]; !ok {
		t.Error("глобал не записан для стартовой привязки")
	}
}

// Монитор отключили: глобал исчез — вывод убран из списка, объекты отпущены.
func TestWaylandMonitorChange_OutputRemoved(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.registryID = 2
	c.w.gOutputs = map[uint32]uint32{50: 4}
	info := c.w.outs.add(50, 30)
	info.ver = 4
	info.xdgID = 31
	c.w.scale.outputs = map[uint32]float64{30: 1}
	c.w.scale.entered = 30
	var cnt wlMonCounter
	c.w.watchMonitors(cnt.fn)

	c.w.handleEvent(2, wlRegistryEvGlobalRemove, wlU32(50))
	if cnt.n != 1 {
		t.Errorf("исчезновение wl_output разбудило %d раз, ждал 1", cnt.n)
	}
	if c.w.outs.isOutput(30) {
		t.Error("вывод остался в списке мониторов")
	}
	if c.w.isOutputObject(30) || c.w.scale.entered != 0 {
		t.Error("масштабные таблицы окна хранят исчезнувший вывод")
	}
	msgs := splitWlMsgs(t, c.read(t))
	if _, ok := findWlMsg(msgs, 31, xdgOutputDestroy); !ok {
		t.Errorf("zxdg_output_v1.destroy не отправлен: %+v", msgs)
	}
	if _, ok := findWlMsg(msgs, 30, wlOutputRelease); !ok {
		t.Errorf("wl_output.release не отправлен: %+v", msgs)
	}

	// Чужой глобал не трогает ничего.
	c.w.handleEvent(2, wlRegistryEvGlobalRemove, wlU32(99))
	if cnt.n != 1 {
		t.Errorf("удаление чужого глобала разбудило подписчика: %d", cnt.n)
	}
}

// wl_output.release есть только с версии 3: на старом объекте запрос — ошибка
// протокола, соединение бы разорвали.
func TestWaylandMonitorChange_NoReleaseOnOldOutput(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.registryID = 2
	info := c.w.outs.add(50, 30)
	info.ver = 2
	c.w.handleEvent(2, wlRegistryEvGlobalRemove, wlU32(50))
	c.peer.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	buf := make([]byte, 64)
	if n, _ := c.peer.Read(buf); n > 0 {
		t.Errorf("на wl_output v2 отправлен запрос: %v", buf[:n])
	}
}

// Логические размер и положение (xdg_output) тоже перемена.
func TestWaylandMonitorChange_XdgOutputEvents(t *testing.T) {
	c := newWlTestWindow(t)
	info := c.w.outs.add(1, 30)
	info.xdgID = 31
	var cnt wlMonCounter
	c.w.watchMonitors(cnt.fn)
	c.w.handleEvent(31, xdgOutEvLogicalPosition, wlU32(0, 0))
	c.w.handleEvent(31, xdgOutEvLogicalSize, wlU32(1500, 1000))
	if cnt.n != 2 {
		t.Errorf("события xdg_output дали %d вызовов, ждал 2", cnt.n)
	}
}

// Без подписки и после её снятия события вывода никого не будят.
func TestWaylandMonitorChange_StopAndNilNotify(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.outs.add(1, 30)
	c.w.handleEvent(30, wlOutputEvGeometry, wlGeometry(0, 0, "A", "B", 0)) // без подписки — не падает

	var cnt wlMonCounter
	stop := c.w.watchMonitors(cnt.fn)
	stop()
	c.w.handleEvent(30, wlOutputEvGeometry, wlGeometry(0, 0, "A", "B", 0))
	if cnt.n != 0 {
		t.Errorf("подписка снята, но вызов был: %d", cnt.n)
	}
	if c.w.watchMonitors(nil) != nil {
		t.Error("watchMonitors(nil) должен вернуть nil")
	}
}
