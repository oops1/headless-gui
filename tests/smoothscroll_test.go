package tests

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// smoothscroll_test.go — «Плавный скролл»: пиксельные дельты колеса/тачпада
// доходят до виджетов, инерция ScrollView затухает/останавливается на часах
// движка (StepAnimations с руками заданными моментами — как в anim_test.go),
// и прерывается новым вводом. Тиковый путь при этом остаётся рабочим (фолбэк).

// stepInertia продвигает глобальные анимации от t0 с шагом 16 мс n раз.
func stepInertia(t0 time.Time, n int) {
	widget.StepAnimations(t0) // ленивый старт: t=0
	for i := 1; i <= n; i++ {
		widget.StepAnimations(t0.Add(time.Duration(i) * 16 * time.Millisecond))
	}
}

// ─── ScrollView: пиксельная дельта доходит ──────────────────────────────────

// TestSmoothScroll_PixelDeltaReachesScrollView: одиночная пиксельная дельта
// доезжает целиком (маховик интегрирует импульс: путь ≈ dy), плавно.
func TestSmoothScroll_PixelDeltaReachesScrollView(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 200, 200))
	sv.ContentHeight = 5000

	if !sv.OnMouseWheelPixels(100, 100, 0, 50) {
		t.Fatalf("дельта должна быть поглощена ScrollView")
	}
	// До первого тика движения ещё нет (плавный скролл, а не мгновенный прыжок).
	if sv.ScrollY() != 0 {
		t.Fatalf("до StepAnimations скролл не должен двигаться, got %d", sv.ScrollY())
	}

	stepInertia(time.Now(), 400)

	got := sv.ScrollY()
	if got < 40 || got > 60 {
		t.Fatalf("пиксельная дельта 50 должна доехать ≈50 px, got %d", got)
	}
	if widget.AnimationsActive() {
		t.Fatalf("после затухания активных анимаций быть не должно")
	}
}

// ─── ScrollView: инерция затухает и останавливается ─────────────────────────

func TestSmoothScroll_InertiaDecaysAndStops(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 200, 200))
	sv.ContentHeight = 100000 // большой запас, чтобы не упереться в край

	// «Бросок»: быстрая серия дельт вниз.
	for i := 0; i < 6; i++ {
		sv.OnMouseWheelPixels(100, 100, 0, 40)
	}

	t0 := time.Now()
	widget.StepAnimations(t0)

	// Инерция продолжает движение ПОСЛЕ окончания ввода.
	prev := sv.ScrollY()
	moved := false
	for i := 1; i <= 20; i++ {
		widget.StepAnimations(t0.Add(time.Duration(i) * 16 * time.Millisecond))
		if sv.ScrollY() > prev {
			moved = true
		}
		prev = sv.ScrollY()
	}
	if !moved {
		t.Fatalf("инерция должна двигать содержимое после броска")
	}

	// Доводим до полной остановки.
	for i := 21; i <= 500; i++ {
		widget.StepAnimations(t0.Add(time.Duration(i) * 16 * time.Millisecond))
	}
	if widget.AnimationsActive() {
		t.Fatalf("инерция должна затухнуть и остановиться")
	}
	settled := sv.ScrollY()
	// Путь ≈ сумма дельт (6×40=240), в пределах допуска.
	if settled < 200 || settled > 300 {
		t.Fatalf("итоговый скролл ≈240 px, got %d", settled)
	}
	// После остановки скролл не «ползёт».
	for i := 501; i <= 520; i++ {
		widget.StepAnimations(t0.Add(time.Duration(i) * 16 * time.Millisecond))
	}
	if sv.ScrollY() != settled {
		t.Fatalf("после остановки скролл не должен меняться: было %d, стало %d", settled, sv.ScrollY())
	}
}

// ─── ScrollView: инерция прерывается новым вводом/кликом ─────────────────────

func TestSmoothScroll_InertiaInterruptedByClick(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 200, 200))
	sv.ContentHeight = 100000

	for i := 0; i < 6; i++ {
		sv.OnMouseWheelPixels(100, 100, 0, 40)
	}
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(16 * time.Millisecond))
	widget.StepAnimations(t0.Add(32 * time.Millisecond))

	mid := sv.ScrollY()
	if mid <= 0 {
		t.Fatalf("инерция должна была сдвинуть скролл, got %d", mid)
	}

	// Клик ЛКМ (не по скроллбару — левее трека) прерывает инерцию: анимация
	// помечается снятой (её вычистит ближайший StepAnimations).
	sv.OnMouseButton(widget.MouseEvent{X: 100, Y: 100, Button: widget.MouseLeft, Pressed: true})

	// Дальнейшие тики не двигают скролл (снятая инерция не тикает).
	for i := 3; i <= 200; i++ {
		widget.StepAnimations(t0.Add(time.Duration(i) * 16 * time.Millisecond))
	}
	if sv.ScrollY() != mid {
		t.Fatalf("после прерывания скролл не должен двигаться: было %d, стало %d", mid, sv.ScrollY())
	}
	if widget.AnimationsActive() {
		t.Fatalf("после прерывания и прогонки анимаций активных быть не должно")
	}
}

// ─── ListView: пиксельная дельта прокручивает попиксельно ───────────────────

func TestSmoothScroll_ListViewPixelDelta(t *testing.T) {
	items := make([]string, 100)
	for i := range items {
		items[i] = "item"
	}
	lv := widget.NewListView(items...)
	lv.SetBounds(image.Rect(0, 0, 200, 200)) // видно ~7 из 100 → есть куда скроллить

	// Прокрутка вниз пиксельной дельтой — поглощается и двигает.
	if !lv.OnMouseWheelPixels(50, 50, 0, 5000) {
		t.Fatalf("ListView должен поглотить пиксельную дельту")
	}
	// Теперь у нижнего края — дальнейшая прокрутка вниз всплывает (false).
	if lv.OnMouseWheelPixels(50, 50, 0, 10) {
		t.Fatalf("у нижнего края дельта вниз должна всплыть к родителю (false)")
	}
	// Прокрутка вверх — снова к верхнему краю.
	lv.OnMouseWheelPixels(50, 50, 0, -5000)
	if lv.OnMouseWheelPixels(50, 50, 0, -10) {
		t.Fatalf("у верхнего края дельта вверх должна всплыть к родителю (false)")
	}
}

// ─── TextBox: пиксельная дельта + субпиксельное накопление ──────────────────

func TestSmoothScroll_TextBoxPixelDelta(t *testing.T) {
	tb := widget.NewTextBox("")
	tb.SetText(strings.Repeat("line\n", 60))
	tb.SetBounds(image.Rect(0, 0, 200, 120))

	before := tb.ScrollTop()
	if !tb.OnMouseWheelPixels(50, 50, 0, 30) {
		t.Fatalf("TextBox должен поглотить пиксельную дельту")
	}
	if tb.ScrollTop() <= before {
		t.Fatalf("пиксельная дельта должна прокрутить TextBox вниз: было %d, стало %d", before, tb.ScrollTop())
	}

	// Субпиксельное накопление: 0.4+0.4 (<1) не двигает, третья 0.4 (=1.2) — на 1 px.
	base := tb.ScrollTop()
	tb.OnMouseWheelPixels(50, 50, 0, 0.4)
	tb.OnMouseWheelPixels(50, 50, 0, 0.4)
	if tb.ScrollTop() != base {
		t.Fatalf("субпиксельные дельты <1 px не должны двигать скролл: было %d, стало %d", base, tb.ScrollTop())
	}
	tb.OnMouseWheelPixels(50, 50, 0, 0.4)
	if tb.ScrollTop() != base+1 {
		t.Fatalf("накопленная субпиксельная дельта должна сдвинуть на 1 px: ожидалось %d, got %d", base+1, tb.ScrollTop())
	}
}

// ─── Engine: маршрутизация + фолбэк на тики ─────────────────────────────────

// wheelTickSpy — виджет, знающий ТОЛЬКО тиковое колесо (без OnMouseWheelPixels).
type wheelTickSpy struct {
	widget.Base
	up, down int
}

func (s *wheelTickSpy) Draw(ctx widget.DrawContext) {}

func (s *wheelTickSpy) OnMouseButton(e widget.MouseEvent) bool {
	if e.Pressed && e.Button == widget.MouseWheelUp {
		s.up++
		return true
	}
	if e.Pressed && e.Button == widget.MouseWheelDown {
		s.down++
		return true
	}
	return false
}

// TestSmoothScroll_EngineRoutesToScrollView: engine.SendMouseWheelPixels
// доставляет точную дельту виджету под курсором (ScrollView).
func TestSmoothScroll_EngineRoutesToScrollView(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 300, 300))
	sv.ContentHeight = 5000

	eng := engine.New(300, 300, 60)
	eng.SetRoot(sv)

	eng.SendMouseWheelPixels(150, 150, 0, 60)
	stepInertia(time.Now(), 400)

	got := sv.ScrollY()
	if got < 45 || got > 75 {
		t.Fatalf("дельта 60 через движок должна доехать ≈60 px, got %d", got)
	}
}

// TestSmoothScroll_EngineFallbackToTicks: если под курсором виджет без точного
// обработчика — движок синтезирует эквивалентные тики (старый путь не ломается).
func TestSmoothScroll_EngineFallbackToTicks(t *testing.T) {
	spy := &wheelTickSpy{}
	spy.SetBounds(image.Rect(0, 0, 300, 300))

	eng := engine.New(300, 300, 60)
	eng.SetRoot(spy)

	// 120 px вниз при шаге 40 px/тик → 3 тика вниз.
	eng.SendMouseWheelPixels(150, 150, 0, 120)
	if spy.down != 3 || spy.up != 0 {
		t.Fatalf("фолбэк должен дать 3 тика вниз, got down=%d up=%d", spy.down, spy.up)
	}

	// 40 px вверх → 1 тик вверх.
	eng.SendMouseWheelPixels(150, 150, 0, -40)
	if spy.up != 1 {
		t.Fatalf("фолбэк вверх должен дать 1 тик, got up=%d", spy.up)
	}
}
