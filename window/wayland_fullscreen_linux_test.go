//go:build linux && !android

package window

import (
	"encoding/binary"
	"testing"
)

// Полный экран на Wayland, на проводе: запрос уходит опкодами xdg_toplevel
// set_fullscreen(11) / unset_fullscreen(12), а состояние берётся из configure.
// Отказ компоновщика (как у WinLine без WINLINE_FULLSCREEN) — configure без
// состояния fullscreen.

// wlConfigure собирает тело xdg_toplevel.configure: размер и массив состояний.
func wlConfigure(w, h int32, states ...uint32) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b[0:4], uint32(w))
	binary.LittleEndian.PutUint32(b[4:8], uint32(h))
	return append(b, wlStatesArray(states...)...)
}

// fsRecorder собирает уведомления о смене полного экрана.
type fsRecorder struct{ got []bool }

func (r *fsRecorder) fn(on bool) { r.got = append(r.got, on) }

func TestWayland_SetFullscreen_SendsRequest(t *testing.T) {
	c := newWlTestWindow(t)
	if !c.w.SetFullscreen(true) {
		t.Fatal("SetFullscreen вернул false: запрос отправлен, ответ должен быть true")
	}
	msgs := splitWlMsgs(t, c.read(t))
	m, ok := findWlMsg(msgs, 12, xdgToplevelSetFullscreen)
	if !ok {
		t.Fatalf("set_fullscreen не отправлен: %+v", msgs)
	}
	if len(m.args) != 1 || m.args[0] != 0 {
		t.Errorf("set_fullscreen %v, ждал output = 0 (выбор за компоновщиком)", m.args)
	}
	if xdgToplevelSetFullscreen != 11 || xdgToplevelUnsetFullscreen != 12 {
		t.Errorf("опкоды %d/%d, по протоколу xdg-shell 11/12",
			xdgToplevelSetFullscreen, xdgToplevelUnsetFullscreen)
	}
	// Фиксация размера окна снимается, как перед разворачиванием: иначе
	// компоновщик не растянет окно с min == max.
	if mx, ok := findWlMsg(msgs, 12, xdgToplevelSetMaxSize); !ok || mx.args[0] != 0 || mx.args[1] != 0 {
		t.Errorf("максимум размера не снят: %+v", msgs)
	}
	if c.w.IsFullscreen() {
		t.Error("IsFullscreen true до ответа компоновщика")
	}
}

// Компоновщик согласился: configure с fullscreen — состояние и уведомление.
func TestWayland_Fullscreen_Accepted(t *testing.T) {
	c := newWlTestWindow(t)
	var rec fsRecorder
	c.w.SetOnFullscreenChanged(rec.fn)
	var sizes [][2]int
	c.w.onResize = func(w, h int) { sizes = append(sizes, [2]int{w, h}) }

	c.w.SetFullscreen(true)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateFullscreen, xdgStateActivated))

	if !c.w.IsFullscreen() {
		t.Error("IsFullscreen false после configure с fullscreen")
	}
	if len(rec.got) != 1 || !rec.got[0] {
		t.Errorf("уведомления %v, ждал [true]", rec.got)
	}
	if len(sizes) != 1 || sizes[0] != [2]int{1920, 1080} {
		t.Errorf("размер окна %v, ждал одно изменение до 1920×1080", sizes)
	}

	// Повторный configure с тем же состоянием не плодит уведомления.
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateFullscreen))
	if len(rec.got) != 1 {
		t.Errorf("повторное состояние дало уведомление: %v", rec.got)
	}
}

// Компоновщик отказал: запрос ушёл (SetFullscreen вернул true), configure
// пришёл без fullscreen. Приложение получает false и остаётся в обычном
// окне — это сценарий WinLine без WINLINE_FULLSCREEN.
func TestWayland_Fullscreen_RefusedByCompositor(t *testing.T) {
	c := newWlTestWindow(t)
	var rec fsRecorder
	c.w.SetOnFullscreenChanged(rec.fn)

	if !c.w.SetFullscreen(true) {
		t.Fatal("SetFullscreen должен вернуть true: запрос отправлен")
	}
	// Отказ WinLine: размер и состояния окна как были, без fullscreen.
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(800, 600, xdgStateActivated))

	if c.w.IsFullscreen() {
		t.Error("IsFullscreen true при отказе компоновщика")
	}
	if len(rec.got) != 1 || rec.got[0] {
		t.Errorf("уведомления %v, ждал [false]", rec.got)
	}

	// Следующий configure (например, смена фокуса) отказом уже не считается.
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(800, 600, xdgStateActivated))
	if len(rec.got) != 1 {
		t.Errorf("лишнее уведомление без запроса: %v", rec.got)
	}

	// После отказа можно спросить снова.
	c.drain(t)
	c.w.SetFullscreen(true)
	if _, ok := findWlMsg(splitWlMsgs(t, c.drain(t)), 12, xdgToplevelSetFullscreen); !ok {
		t.Error("повторный запрос после отказа не отправлен")
	}
}

// Выход: unset_fullscreen на провод, уведомление по ответу компоновщика.
func TestWayland_Fullscreen_Leave(t *testing.T) {
	c := newWlTestWindow(t)
	var rec fsRecorder
	c.w.SetOnFullscreenChanged(rec.fn)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateFullscreen))

	c.w.SetFullscreen(false)
	if _, ok := findWlMsg(splitWlMsgs(t, c.read(t)), 12, xdgToplevelUnsetFullscreen); !ok {
		t.Fatal("unset_fullscreen не отправлен")
	}
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(800, 600, xdgStateActivated))
	if c.w.IsFullscreen() {
		t.Error("IsFullscreen true после выхода")
	}
	if want := []bool{true, false}; len(rec.got) != 2 || rec.got[0] != want[0] || rec.got[1] != want[1] {
		t.Errorf("уведомления %v, ждал %v", rec.got, want)
	}
}

// Выход по воле компоновщика (Esc/F11 на его стороне): уведомление приходит,
// хотя приложение ничего не просило.
func TestWayland_Fullscreen_LeftByCompositor(t *testing.T) {
	c := newWlTestWindow(t)
	var rec fsRecorder
	c.w.SetOnFullscreenChanged(rec.fn)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateFullscreen))
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(800, 600))
	if len(rec.got) != 2 || !rec.got[0] || rec.got[1] {
		t.Errorf("уведомления %v, ждал [true false]", rec.got)
	}
}

// Запрос не нужен, если окно уже в нужном состоянии.
func TestWayland_Fullscreen_NoRedundantRequests(t *testing.T) {
	c := newWlTestWindow(t)
	if !c.w.SetFullscreen(false) {
		t.Error("выход из невключённого полного экрана вернул false")
	}
	c.w.SetFullscreen(true)
	c.w.SetFullscreen(true) // запрос уже в пути
	msgs := splitWlMsgs(t, c.read(t))
	n := 0
	for _, m := range msgs {
		if m.obj == 12 && m.opcode == xdgToplevelSetFullscreen {
			n++
		}
	}
	if n != 1 {
		t.Errorf("set_fullscreen отправлен %d раз, ждал 1", n)
	}
	if _, ok := findWlMsg(msgs, 12, xdgToplevelUnsetFullscreen); ok {
		t.Error("unset_fullscreen отправлен без полноэкранного состояния")
	}
}

// IsMaximized на время полного экрана хранит прежнее значение, а не true: кнопка
// □ и меню окна знают, развёрнуто ли оно на самом деле.
func TestWayland_Fullscreen_KeepsMaximizedValue(t *testing.T) {
	// Окно не было развёрнуто.
	c := newWlTestWindow(t)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateFullscreen))
	if c.w.IsMaximized() {
		t.Error("полноэкранное окно считается развёрнутым")
	}

	// Окно было развёрнуто; компоновщик на время полного экрана состояние
	// maximized не называет — значение остаётся.
	c = newWlTestWindow(t)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1024, 768, xdgStateMaximized))
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateFullscreen))
	if !c.w.IsMaximized() {
		t.Error("развёрнутое окно потеряло «развёрнуто» в полном экране")
	}
	if !c.w.IsFullscreen() {
		t.Error("IsFullscreen false")
	}
	// Выход: компоновщик возвращает окно развёрнутым.
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1024, 768, xdgStateMaximized))
	if !c.w.IsMaximized() || c.w.IsFullscreen() {
		t.Errorf("после выхода: развёрнуто=%v, полный экран=%v", c.w.IsMaximized(), c.w.IsFullscreen())
	}
}

// Полноэкранное окно не скругляется (cornermask.go), обычное — скругляется.
func TestWayland_Fullscreen_NoRoundedCorners(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.SetCornerRadius(8)
	if c.w.maskRadius() == 0 {
		t.Fatal("обычное окно не скругляется")
	}
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateFullscreen))
	if r := c.w.maskRadius(); r != 0 {
		t.Errorf("радиус маски в полном экране %d, ждал 0", r)
	}
}

// Окно с запретом ресайза после выхода снова фиксируется на размере ДО полного
// экрана, а не на размере экрана.
func TestWayland_Fullscreen_FixedSizeRestoredOnLeave(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.resizable.Store(false)
	c.w.SetFullscreen(true)
	c.drain(t)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateFullscreen))

	c.w.SetFullscreen(false)
	msgs := splitWlMsgs(t, c.read(t))
	mn, ok1 := findWlMsg(msgs, 12, xdgToplevelSetMinSize)
	mx, ok2 := findWlMsg(msgs, 12, xdgToplevelSetMaxSize)
	if !ok1 || !ok2 {
		t.Fatalf("фиксация размера не восстановлена: %+v", msgs)
	}
	if mn.args[0] != 800 || mn.args[1] != 600 || mx.args[0] != 800 || mx.args[1] != 600 {
		t.Errorf("min %v max %v, ждал по 800×600 (размер до полного экрана)", mn.args, mx.args)
	}
}

// Отказ компоновщика у окна с запретом ресайза тоже возвращает фиксацию.
func TestWayland_Fullscreen_RefusalRestoresFixedSize(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.resizable.Store(false)
	c.w.SetFullscreen(true)
	c.drain(t)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(800, 600))
	msgs := splitWlMsgs(t, c.read(t))
	if mx, ok := findWlMsg(msgs, 12, xdgToplevelSetMaxSize); !ok || mx.args[0] != 800 || mx.args[1] != 600 {
		t.Errorf("после отказа размер не зафиксирован обратно: %+v", msgs)
	}
}
