//go:build linux && !android

package window

import (
	"image"
	"testing"
)

// Масштаб интерфейса на Wayland движок не спрашивал вовсе: на мониторе с
// удвоением всё выходило вдвое мельче положенного, и поправить это можно
// было только переменной HEADLESS_GUI_SCALE — то есть человек должен был
// знать о ней и выставлять руками.

// Дробный масштаб — прямое указание компоновщика для этой поверхности:
// 180 сто двадцатых долей это 1.5.
func TestWaylandScale_FractionalPreferred(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.scale.fracID = 55
	c.w.scale.viewportID = 56

	var got float64
	c.w.SetOnDpiChanged(func(k float64) { got = k })
	c.w.handleEvent(55, wpFracEvPreferredScale, wlU32(180))

	if got != 1.5 {
		t.Errorf("окну сообщили масштаб %v, ждал 1.5", got)
	}
	if k := c.w.scaleFactor(); k != 1.5 {
		t.Errorf("масштаб поверхности %v", k)
	}

	msgs := splitWlMsgs(t, c.drain(t))
	// Размер кадра в поверхностных единицах: 800×600 пикселей при 1.5 — это
	// 533×400. Без этого картинка заняла бы столько единиц, сколько в ней
	// пикселей, и окно стало бы в полтора раза больше заказанного.
	dst, ok := findWlMsg(msgs, 56, wpViewportSetDestination)
	if !ok || len(dst.args) != 2 {
		t.Fatalf("область просмотра не задана: %+v", msgs)
	}
	if int32(dst.args[0]) != 533 || int32(dst.args[1]) != 400 {
		t.Errorf("set_destination %v, ждал [533 400]", dst.args)
	}
	// При дробном масштабе буферный обязан остаться единицей.
	if _, ok := findWlMsg(msgs, c.w.surfaceID, wlSurfaceSetBufferScale); ok {
		t.Error("отправлен set_buffer_scale при дробном масштабе")
	}
}

// Без расширения дробного масштаба остаётся целый — свойство монитора, на
// который попало окно.
func TestWaylandScale_IntegerFromOutput(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.scale.outputs = map[uint32]float64{9: 0}

	var got float64
	c.w.SetOnDpiChanged(func(k float64) { got = k })
	c.w.handleEvent(9, wlOutputEvScale, wlU32(2))
	// Пока окно не показалось на этом мониторе, его масштаб ничего не значит.
	if got != 0 {
		t.Errorf("масштаб применён до wl_surface.enter: %v", got)
	}

	c.w.handleEvent(c.w.surfaceID, wlSurfaceEvEnter, wlU32(9))
	if got != 2 {
		t.Errorf("окну сообщили масштаб %v, ждал 2", got)
	}

	msgs := splitWlMsgs(t, c.drain(t))
	m, ok := findWlMsg(msgs, c.w.surfaceID, wlSurfaceSetBufferScale)
	if !ok || len(m.args) != 1 || int32(m.args[0]) != 2 {
		t.Errorf("set_buffer_scale %v, ждал 2", m.args)
	}
}

// Размер в configure приходит в поверхностных единицах, а движок считает в
// пикселях буфера: на HiDPI это разные числа, и без перевода окно рисовало
// бы кадр вчетверо меньше нужного.
func TestWaylandScale_ConfigureToPixels(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.scale.current = 2

	var gotW, gotH int
	c.w.onResize = func(w, h int) { gotW, gotH = w, h }
	body := append(wlU32(500, 400), wlU32(0)...) // states: пустой массив
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, body)

	if gotW != 1000 || gotH != 800 {
		t.Errorf("движку сообщили %dx%d, ждал 1000x800 пикселей", gotW, gotH)
	}
	if c.w.width != 1000 || c.w.height != 800 {
		t.Errorf("окно запомнило %dx%d", c.w.width, c.w.height)
	}
}

// Фиксация размера и минимум уходят компоновщику в поверхностных единицах:
// физические пиксели завысили бы окно в scale раз.
func TestWaylandScale_SizeLimitsInSurfaceUnits(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.scale.current = 2
	c.w.width, c.w.height = 800, 600

	c.w.applySizeLimits(false)
	msgs := splitWlMsgs(t, c.drain(t))

	m, ok := findWlMsg(msgs, c.w.toplevelID, xdgToplevelSetMaxSize)
	if !ok || len(m.args) != 2 {
		t.Fatalf("set_max_size не отправлен: %+v", msgs)
	}
	if int32(m.args[0]) != 400 || int32(m.args[1]) != 300 {
		t.Errorf("set_max_size %v, ждал [400 300]", m.args)
	}
}

// Повреждённая область приходит в пикселях буфера. Пока масштаб единица, это
// те же поверхностные единицы; на HiDPI они расходятся, и нужен
// damage_buffer — он считает ровно в пикселях.
func TestWaylandScale_DamageInBufferPixels(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.scale.current = 2
	c.w.gCompositorVer = wlCompositorVersionBufferDamage

	c.w.damageSurface(image.Rect(10, 20, 110, 220))
	msgs := splitWlMsgs(t, c.drain(t))

	m, ok := findWlMsg(msgs, c.w.surfaceID, wlSurfaceDamageBuffer)
	if !ok || len(m.args) != 4 {
		t.Fatalf("damage_buffer не отправлен: %+v", msgs)
	}
	if int32(m.args[0]) != 10 || int32(m.args[2]) != 100 {
		t.Errorf("damage_buffer %v, ждал область в пикселях буфера", m.args)
	}
	if _, ok := findWlMsg(msgs, c.w.surfaceID, wlSurfaceDamage); ok {
		t.Error("отправлен и обычный damage — область была бы помечена дважды")
	}
}

// Старый компоновщик (wl_compositor младше четвёртой версии) damage_buffer не
// знает: область переводится в поверхностные единицы с запасом, лучше
// перерисовать лишнее, чем оставить полоску старого кадра.
func TestWaylandScale_DamageFallback(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.scale.current = 2
	c.w.gCompositorVer = 1

	c.w.damageSurface(image.Rect(10, 20, 110, 220))
	msgs := splitWlMsgs(t, c.drain(t))

	m, ok := findWlMsg(msgs, c.w.surfaceID, wlSurfaceDamage)
	if !ok || len(m.args) != 4 {
		t.Fatalf("damage не отправлен: %+v", msgs)
	}
	if int32(m.args[0]) != 4 || int32(m.args[2]) != 52 {
		t.Errorf("damage %v, ждал поверхностные единицы с запасом", m.args)
	}
}

// Масштаб единица — ни одного лишнего запроса: окно ведёт себя ровно так,
// как до появления HiDPI.
func TestWaylandScale_NoChangeAtOne(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.scale.outputs = map[uint32]float64{9: 1}

	called := false
	c.w.SetOnDpiChanged(func(k float64) { called = true })
	c.w.handleEvent(c.w.surfaceID, wlSurfaceEvEnter, wlU32(9))

	if called {
		t.Error("окну сообщили о смене масштаба, хотя он остался единицей")
	}
	if k := c.w.scaleFactor(); k != 1 {
		t.Errorf("масштаб %v", k)
	}
}
