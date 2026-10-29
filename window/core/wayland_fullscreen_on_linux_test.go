//go:build linux && !android

package core

import (
	"image"
	"testing"
)

// Полный экран на выбранном мониторе Wayland, на проводе: set_fullscreen
// получает объект wl_output ЭТОГО соединения, который соответствует монитору
// из window.Monitors (тот пришёл с другого подключения, ключ — имя глобала).

// wlTwoOutputs заводит окну два вывода: глобал 5 → объект 30 (слева) и
// глобал 9 → объект 40 (справа).
func wlTwoOutputs(c *wlTestConn) {
	c.w.outs.add(5, 30)
	c.w.outs.add(9, 40)
	for i, id := range []uint32{30, 40} {
		c.w.handleEvent(id, wlOutputEvGeometry, wlGeometry(int32(i)*1920, 0, "", "", 0))
		c.w.handleEvent(id, wlOutputEvMode, wlU32(wlOutputModeCurrent, 1920, 1080, 60000))
	}
	c.w.handleEvent(30, wlOutputEvName, wlStr("eDP-1"))
	c.w.handleEvent(40, wlOutputEvName, wlStr("HDMI-A-1"))
}

func TestWayland_SetFullscreenOn_ChoosesOutput(t *testing.T) {
	c := newWlTestWindow(t)
	wlTwoOutputs(c)

	// Монитор «со стороны»: ключ — имя глобала (одинаково у любого
	// подключения), объект wl_output в нём не известен.
	right := Monitor{Name: "HDMI-A-1", Bounds: image.Rect(1920, 0, 3840, 1080), id: 9}
	if !c.w.SetFullscreenOn(right) {
		t.Fatal("SetFullscreenOn вернул false при известном выводе")
	}
	m, ok := findWlMsg(splitWlMsgs(t, c.read(t)), 12, xdgToplevelSetFullscreen)
	if !ok {
		t.Fatal("set_fullscreen не отправлен")
	}
	if len(m.args) != 1 || m.args[0] != 40 {
		t.Errorf("set_fullscreen(%v), ждал объект wl_output 40 (правый монитор)", m.args)
	}
	if c.w.IsFullscreen() {
		t.Error("IsFullscreen true до ответа компоновщика")
	}

	// Сопоставление по имени, когда ключ устарел, и по границам без имени.
	for _, tc := range []struct {
		name string
		m    Monitor
		want uint32
	}{
		{"по имени", Monitor{Name: "eDP-1", id: 77}, 30},
		{"по границам", Monitor{Bounds: image.Rect(1920, 0, 3840, 1080)}, 40},
	} {
		c2 := newWlTestWindow(t)
		wlTwoOutputs(c2)
		if !c2.w.SetFullscreenOn(tc.m) {
			t.Errorf("%s: отказ", tc.name)
			continue
		}
		m, _ := findWlMsg(splitWlMsgs(t, c2.read(t)), 12, xdgToplevelSetFullscreen)
		if len(m.args) != 1 || m.args[0] != tc.want {
			t.Errorf("%s: set_fullscreen(%v), ждал %d", tc.name, m.args, tc.want)
		}
	}
}

// Уже в полном экране: новый set_fullscreen с другим выводом, без выхода из
// режима и без потери сохранённого размера.
func TestWayland_SetFullscreenOn_WhileFullscreen(t *testing.T) {
	c := newWlTestWindow(t)
	wlTwoOutputs(c)
	var rec fsRecorder
	c.w.SetOnFullscreenChanged(rec.fn)
	c.w.SetFullscreen(true)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateFullscreen))
	c.drain(t)
	saved := c.w.fs.saveW.Load()

	if !c.w.SetFullscreenOn(Monitor{id: 9}) {
		t.Fatal("перенос отклонён")
	}
	msgs := splitWlMsgs(t, c.read(t))
	m, ok := findWlMsg(msgs, 12, xdgToplevelSetFullscreen)
	if !ok || len(m.args) != 1 || m.args[0] != 40 {
		t.Fatalf("перенос: set_fullscreen %+v", m)
	}
	if _, ok := findWlMsg(msgs, 12, xdgToplevelUnsetFullscreen); ok {
		t.Error("перенос вышел из полного экрана")
	}
	if c.w.fs.saveW.Load() != saved {
		t.Errorf("перенос затёр сохранённый размер: %d, было %d", c.w.fs.saveW.Load(), saved)
	}
	// Компоновщик перенёс окно: configure с fullscreen — не новая смена.
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateFullscreen))
	if len(rec.got) != 1 || !rec.got[0] {
		t.Errorf("уведомления %v, ждал единственное [true]", rec.got)
	}
}

// Вывода нет среди известных (отключён) — отказ без единого сообщения.
func TestWayland_SetFullscreenOn_UnknownOutput(t *testing.T) {
	c := newWlTestWindow(t)
	wlTwoOutputs(c)
	if c.w.SetFullscreenOn(Monitor{id: 123, Name: "VGA-1", Bounds: image.Rect(9000, 0, 9800, 600)}) {
		t.Error("неизвестный вывод принят")
	}
	if c.w.SetFullscreenOn(Monitor{}) {
		t.Error("нулевой Monitor принят")
	}
	if c.w.fs.pending.Load() {
		t.Error("отказ оставил запрос «в пути»")
	}
}

// Ключ монитора — имя глобала registry, как и объявляет композитор.
func TestWaylandMonitors_KeyIsGlobalName(t *testing.T) {
	c := newWlTestWindow(t)
	wlTwoOutputs(c)
	list := c.w.monitors()
	if len(list) != 2 || list[0].id != 5 || list[1].id != 9 {
		t.Errorf("ключи %+v, ждал глобалы 5 и 9", list)
	}
}
