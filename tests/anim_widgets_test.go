package tests

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// anim_widgets_test.go — проверки трёх пилотов анимационного фреймворка:
// ToggleSwitch (скольжение ручки), ProgressBar.AnimateValue, Dialog (fade-in
// затемнения). Стиль — по образцу tests/anim_test.go и tests/modal_test.go:
// widget.StopAllAnimations() в начале/defer (глобальный реестр анимаций общий
// на процесс), waitCount/поллинг с таймаутом вместо фиксированного sleep.

// waitAnimSettle ждёт, пока AnimationsActive() не станет false (анимация
// завершилась естественным путём), либо до истечения таймаута. Использует
// короткий поллинг вместо фиксированного долгого sleep.
func waitAnimSettle(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !widget.AnimationsActive() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return !widget.AnimationsActive()
}

// newAnimEngine создаёт движок 60fps с пустой панелью-корнем — используется
// во всех сценариях "с работающим движком" этого файла.
func newAnimEngine(w, h int) *engine.Engine {
	eng := engine.New(w, h, 60)
	eng.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.SetBounds(image.Rect(0, 0, w, h))
	eng.SetRoot(root)
	return eng
}

// ─── ToggleSwitch ────────────────────────────────────────────────────────────

// TestAnimToggleSwitch_NoEngine_InstantCanonical: SetChecked/SetOn без
// работающего движка (ни одного StepAnimations) должен немедленно отражать
// каноническое состояние — ручка не должна "зависнуть" в позиции по
// умолчанию, ожидая несуществующих тиков (инвариант (а) задачи).
func TestAnimToggleSwitch_NoEngine_InstantCanonical(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	ts := widget.NewToggleSwitch("T")
	ts.SetBounds(image.Rect(0, 0, 100, 30))

	// on=true без единого тика анимации — рисуем мгновенно на canvas и
	// проверяем позицию кружка (правый край капсулы).
	ts.SetOn(true)
	cxOn := knobCenterX(t, ts)
	capRight := ts.Bounds().Min.X + toggleCapsuleWidth() - togglePadConst() - toggleThumbRConst()
	if cxOn != capRight {
		t.Fatalf("SetOn(true) без движка: ручка должна быть мгновенно в положении on=%d, got %d", capRight, cxOn)
	}

	ts.SetOn(false)
	cxOff := knobCenterX(t, ts)
	capLeft := ts.Bounds().Min.X + togglePadConst() + toggleThumbRConst()
	if cxOff != capLeft {
		t.Fatalf("SetOn(false) без движка: ручка должна быть мгновенно в положении off=%d, got %d", capLeft, cxOff)
	}
}

// TestAnimToggleSwitch_WithEngine_SettlesToCanonical: с работающим движком
// SetOn запускает плавную анимацию (AnimateOwned), но по её завершении
// визуальная позиция ручки обязана совпасть с канонической (checked=true→1,
// false→0). Ждём завершения через AnimationsActive() с коротким поллингом.
func TestAnimToggleSwitch_WithEngine_SettlesToCanonical(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	eng := newAnimEngine(200, 100)
	ts := widget.NewToggleSwitch("T")
	ts.SetBounds(image.Rect(10, 10, 110, 40))
	eng.Root().(*widget.Panel).AddChild(ts)
	eng.Start()
	defer eng.Stop()

	if !waitCount(eng, 1) {
		t.Fatal("первый кадр не отрендерился")
	}

	ts.SetOn(true)
	if !waitAnimSettle(2 * time.Second) {
		t.Fatal("анимация ручки не завершилась за отведённое время")
	}
	// Небольшая пауза, чтобы движок успел отрисовать финальный кадр
	// после того, как тик выставил каноническую позицию.
	time.Sleep(30 * time.Millisecond)

	if !ts.IsOn() {
		t.Fatal("IsOn() должен быть true после SetOn(true)")
	}
	cx := knobCenterX(t, ts)
	capRight := ts.Bounds().Min.X + toggleCapsuleWidth() - togglePadConst() - toggleThumbRConst()
	if cx != capRight {
		t.Fatalf("после завершения анимации ручка должна быть в каноническом положении on=%d, got %d", capRight, cx)
	}
}

// TestAnimToggleSwitch_FastDoubleClick_NoStuckMidway: быстрое переключение
// туда-обратно (эмуляция двойного клика) не должно оставить ручку в
// промежуточном положении — AnimateOwned(ts,"knob",...) должен остановить
// предыдущую анимацию и не "тянуть" стрелку к устаревшей цели.
func TestAnimToggleSwitch_FastDoubleClick_NoStuckMidway(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	eng := newAnimEngine(200, 100)
	ts := widget.NewToggleSwitch("T")
	ts.SetBounds(image.Rect(10, 10, 110, 40))
	eng.Root().(*widget.Panel).AddChild(ts)
	eng.Start()
	defer eng.Stop()

	if !waitCount(eng, 1) {
		t.Fatal("первый кадр не отрендерился")
	}

	// Туда-обратно быстро, без ожидания завершения между вызовами.
	ts.SetOn(true)
	time.Sleep(5 * time.Millisecond) // даём первому тику стартовать (не более)
	ts.SetOn(false)

	if !waitAnimSettle(2 * time.Second) {
		t.Fatal("анимация не устоялась за отведённое время")
	}
	time.Sleep(30 * time.Millisecond)

	if ts.IsOn() {
		t.Fatal("итоговое состояние должно быть off после туда-обратно (последний вызов SetOn(false))")
	}
	cx := knobCenterX(t, ts)
	capLeft := ts.Bounds().Min.X + togglePadConst() + toggleThumbRConst()
	if cx != capLeft {
		t.Fatalf("после туда-обратно ручка должна встать РОВНО в каноническое off=%d (без 'хвоста' старой анимации), got %d", capLeft, cx)
	}
}

// ─── ProgressBar.AnimateValue ────────────────────────────────────────────────

// TestAnimProgressBar_SetValue_StillInstant: SetValue остаётся мгновенным
// (обратная совместимость) — не создаёт анимацию и не требует тиков движка.
func TestAnimProgressBar_SetValue_StillInstant(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	pb := widget.NewProgressBar()
	pb.SetValue(0.3)
	pb.SetValue(0.9)
	if v := pb.Value(); v < 0.899 || v > 0.901 {
		t.Fatalf("SetValue должен быть мгновенным, got %v", v)
	}
	if widget.AnimationsActive() {
		t.Fatal("SetValue не должен создавать анимацию")
	}
}

// TestAnimProgressBar_AnimateValue_ReachesTarget: AnimateValue плавно доходит
// до newValue за отведённое время (с работающим движком).
func TestAnimProgressBar_AnimateValue_ReachesTarget(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	eng := newAnimEngine(200, 100)
	pb := widget.NewProgressBar()
	pb.SetBounds(image.Rect(10, 10, 190, 30))
	eng.Root().(*widget.Panel).AddChild(pb)
	eng.Start()
	defer eng.Stop()

	if !waitCount(eng, 1) {
		t.Fatal("первый кадр не отрендерился")
	}

	pb.AnimateValue(0.75)
	if !waitAnimSettle(2 * time.Second) {
		t.Fatal("AnimateValue не завершился за отведённое время")
	}
	if v := pb.Value(); v < 0.74 || v > 0.76 {
		t.Fatalf("после AnimateValue значение должно достичь ~0.75, got %v", v)
	}
}

// ─── Dialog fade-in ──────────────────────────────────────────────────────────

// TestAnimDialog_ShowModal_FadeReachesTarget: после показа модального диалога
// и завершения fade-in DimColor().A равен целевому (Dim.A из темы).
func TestAnimDialog_ShowModal_FadeReachesTarget(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	eng := newAnimEngine(300, 200)
	eng.Start()
	defer eng.Stop()

	if !waitCount(eng, 1) {
		t.Fatal("первый кадр не отрендерился")
	}

	dlg := widget.NewDialog("T", 200, 100)
	target := dlg.Dim.A // канонический таргет ДО показа
	eng.ShowModal(dlg)
	defer eng.CloseModal(dlg)

	if !waitAnimSettle(2 * time.Second) {
		t.Fatal("fade-анимация не завершилась за отведённое время")
	}
	time.Sleep(30 * time.Millisecond)

	if got := dlg.DimColor().A; got != target {
		t.Fatalf("после fade-in DimColor().A должен быть %d, got %d", target, got)
	}
}

// TestAnimDialog_EscapeDuringFade_NoPanicNoRace: закрытие диалога через
// Escape ПОСРЕДИ fade-анимации не должно паниковать и не должно оставлять
// "мусорных" тиков, продолжающих дёргать уже закрытый диалог.
func TestAnimDialog_EscapeDuringFade_NoPanicNoRace(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("паника при закрытии диалога посреди fade-in: %v", r)
		}
	}()

	eng := newAnimEngine(300, 200)
	eng.Start()
	defer eng.Stop()

	if !waitCount(eng, 1) {
		t.Fatal("первый кадр не отрендерился")
	}

	dlg := widget.NewDialog("T", 200, 100)
	eng.ShowModal(dlg)

	// Закрываем СРАЗУ, не дожидаясь окончания fade (140ms) — Escape должен
	// закрыть диалог немедленно и безопасно остановить fade-анимацию.
	time.Sleep(10 * time.Millisecond)
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})

	// Даём движку ещё несколько кадров потикать: если анимация не была
	// остановлена, она "дотикивала" бы ещё ~130ms, дёргая setDimAlpha на уже
	// закрытом диалоге. Явной проверки состояния здесь недостаточно (диалог
	// валиден и без Stop), поэтому проверяем ГЛАВНОЕ: активных анимаций
	// после закрытия быть не должно (Stop реально снял анимацию).
	time.Sleep(200 * time.Millisecond)
	if widget.AnimationsActive() {
		t.Fatal("после закрытия диалога посреди fade не должно оставаться активных анимаций")
	}
}

// ─── Вспомогательные функции для чтения визуальной геометрии ToggleSwitch ───
//
// ToggleSwitch не экспортирует позицию ручки напрямую. Кружок рисуется через
// drawFilledCircle(ctx, cx, cy, r, col), который при поддержке ctx интерфейса
// widget.AAShapes (см. widget/context.go) вызывает ОДИН РАЗ ctx.FillEllipseAA
// (cx, cy, r, r, col) — этого достаточно, чтобы фиктивный DrawContext поймал
// точный X центра ручки без растровой развёртки.

// toggleCapsuleWidth/togglePadConst/toggleThumbRConst — публичные копии
// приватных геометрических констант ToggleSwitch (44/4/7), захардкожены
// здесь, т.к. пакет widget их не экспортирует; используются только для
// вычисления ожидаемых координат в тесте (не влияют на сам виджет).
func toggleCapsuleWidth() int { return 44 }
func togglePadConst() int     { return 4 }
func toggleThumbRConst() int  { return 7 }

// knobCenterX рендерит ToggleSwitch в фиктивный DrawContext (fakeDrawCtx,
// implements widget.AAShapes) и возвращает X центра нарисованного кружка
// ручки (последний вызов FillEllipseAA — единственный круг в Draw).
func knobCenterX(t *testing.T, ts *widget.ToggleSwitch) int {
	t.Helper()
	rec := &fakeDrawCtx{}
	ts.Draw(rec)
	if rec.lastEllipseCX == nil {
		t.Fatal("ToggleSwitch.Draw не нарисовал кружок ручки (FillEllipseAA не вызван)")
	}
	return *rec.lastEllipseCX
}

// fakeDrawCtx — минимальная реализация widget.DrawContext + widget.AAShapes
// для юнит-тестов вне engine: все примитивы — no-op, кроме FillEllipseAA,
// которая запоминает X последнего вызова (кружок ручки ToggleSwitch).
type fakeDrawCtx struct {
	lastEllipseCX *int
}

func (f *fakeDrawCtx) FillRect(x, y, w, h int, col color.RGBA)      {}
func (f *fakeDrawCtx) FillRectAlpha(x, y, w, h int, col color.RGBA) {}
func (f *fakeDrawCtx) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
}
func (f *fakeDrawCtx) DrawBorder(x, y, w, h int, col color.RGBA)          {}
func (f *fakeDrawCtx) DrawRoundBorder(x, y, w, h, r int, col color.RGBA) {}
func (f *fakeDrawCtx) SetPixel(x, y int, col color.RGBA)                 {}
func (f *fakeDrawCtx) DrawHLine(x, y, length int, col color.RGBA)        {}
func (f *fakeDrawCtx) DrawVLine(x, y, length int, col color.RGBA)        {}
func (f *fakeDrawCtx) DrawImage(src image.Image, x, y int)               {}
func (f *fakeDrawCtx) DrawImageScaled(src image.Image, x, y, w, h int)   {}
func (f *fakeDrawCtx) DrawText(text string, x, y int, col color.RGBA)    {}
func (f *fakeDrawCtx) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
}
func (f *fakeDrawCtx) DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA) {
}
func (f *fakeDrawCtx) MeasureText(text string, sizePt float64) int { return 0 }
func (f *fakeDrawCtx) MeasureTextFont(text string, sizePt float64, fontName string) int {
	return 0
}
func (f *fakeDrawCtx) MeasureRunePositions(text string, sizePt float64) []int { return nil }
func (f *fakeDrawCtx) SetClip(r image.Rectangle)                             {}
func (f *fakeDrawCtx) ClearClip()                                            {}
func (f *fakeDrawCtx) Clip() image.Rectangle                                 { return image.Rectangle{} }

// FillEllipseAA — единственный примитив, который реально интересует тест:
// drawFilledCircle(ctx,...) вызывает его ровно один раз для кружка ручки.
func (f *fakeDrawCtx) FillEllipseAA(cx, cy, rx, ry int, col color.RGBA) {
	x := cx
	f.lastEllipseCX = &x
}
func (f *fakeDrawCtx) StrokeEllipseAA(cx, cy, rx, ry int, thickness float64, col color.RGBA) {
}
func (f *fakeDrawCtx) FillPolygonAA(pts []image.Point, col color.RGBA) {}
func (f *fakeDrawCtx) StrokePolylineAA(pts []image.Point, thickness float64, closed bool, col color.RGBA) {
}
func (f *fakeDrawCtx) DrawLineAA(x1, y1, x2, y2 int, thickness float64, col color.RGBA) {}
