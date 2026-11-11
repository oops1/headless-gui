package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/engine"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// Рамка окна от ОС — запрос GG-99.
//
// На Windows 11 углы окна срезает DWM и она же кладёт рамку в 1 px. Окно
// движка рисовало поверх свой скруглённый контур: на прямых краях он лежал
// под рамкой DWM, а на углах, где радиусы и сглаживание не совпадают, был
// виден второй дугой — тёмной или акцентной. С SystemFrame окно и диалог
// своего наружного контура не рисуют, корпус прямоугольный, цвет рамки уходит
// ОС.

// systemFrameWindow рисует главное окно 320×200 со скруглением темы и
// возвращает кадр.
func systemFrameWindow(t *testing.T, setup func(w *widget.Window)) (*image.RGBA, *widget.Window) {
	t.Helper()
	w := widget.NewWindow("Go.Git", 320, 200)
	w.MainWindow = true
	w.TitleStyle = widget.WindowTitleWin
	w.CornerRadius = 8
	if setup != nil {
		setup(w)
	}
	w.SetBounds(image.Rect(0, 0, 320, 200))
	eng := engine.New(320, 200, 30)
	t.Cleanup(eng.Stop)
	eng.SetRoot(w)
	eng.RenderOnce()
	return snapshotRGBA(eng.RenderOnce()), w
}

// countColor считает точки цвета c в прямоугольнике.
func countColor(img *image.RGBA, r image.Rectangle, c color.RGBA) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if img.RGBAAt(x, y) == c {
				n++
			}
		}
	}
	return n
}

// Окно, у которого рамку рисует ОС, своего контура не рисует ни на краях, ни
// в угловых квадратах, а корпус доходит до самых углов. Без SystemFrame контур
// на месте — тест различает режимы.
func TestSystemFrame_WindowDrawsNoContour(t *testing.T) {
	testreset.Globals(t)
	own, _ := systemFrameWindow(t, func(w *widget.Window) { w.SetFrameColor(accent) })
	if n := countColor(own, own.Rect, accent); n < 600 {
		t.Fatalf("без SystemFrame точек контура %d — контур не нарисован, тест ничего не проверяет", n)
	}

	sys, w := systemFrameWindow(t, func(w *widget.Window) {
		w.SetFrameColor(accent)
		w.SystemFrame = true
	})
	if n := countColor(sys, sys.Rect, accent); n != 0 {
		t.Errorf("с SystemFrame нарисовано %d точек контура цветом рамки", n)
	}
	// Корпус прямоугольный: в угловой точке — цвет корпуса, как рядом с углом,
	// а не фон холста за скруглением. Нижний левый угол — там нет заголовка.
	corner, inside := sys.RGBAAt(0, 199), sys.RGBAAt(12, 190)
	if corner != inside {
		t.Errorf("угол корпуса %v, внутри рядом %v — корпус скруглён, за ним виден холст", corner, inside)
	}
	if w.CornerRadius != 8 {
		t.Errorf("SystemFrame изменил CornerRadius окна: %d", w.CornerRadius)
	}

	// Без своего цвета у главного окна была XOR-рамка — её тоже нет.
	plain, _ := systemFrameWindow(t, func(w *widget.Window) { w.SystemFrame = true })
	if left, in := plain.RGBAAt(0, 120), plain.RGBAAt(3, 120); left != in {
		t.Errorf("с SystemFrame на левом краю осталась рамка: %v против %v внутри", left, in)
	}
}

// Развёрнутому окну рамку не рисует никто, кроме движка: SystemFrame её не
// снимает.
func TestSystemFrame_MaximizedKeepsOwnContour(t *testing.T) {
	testreset.Globals(t)
	img, _ := systemFrameWindow(t, func(w *widget.Window) {
		w.SetFrameColor(accent)
		w.SystemFrame = true
		w.SetMaximized(true)
	})
	if n := countColor(img, img.Rect, accent); n == 0 {
		t.Error("у развёрнутого окна с SystemFrame пропал свой контур")
	}
}

type sysFrameCall struct {
	c   color.RGBA
	set bool
}

// Цвет рамки уходит ОС: заданный — как есть, у неактивного окна —
// приглушённый, снятый — set=false. Повторный кадр без изменений ничего не
// шлёт.
func TestSystemFrame_ColorGoesToOS(t *testing.T) {
	testreset.Globals(t)
	var calls []sysFrameCall
	w := widget.NewWindow("Go.Git", 320, 200)
	w.MainWindow = true
	w.TitleStyle = widget.WindowTitleWin
	w.CornerRadius = 8
	w.SystemFrame = true
	w.SystemFrameColor = func(c color.RGBA, set bool) { calls = append(calls, sysFrameCall{c, set}) }
	w.SetBounds(image.Rect(0, 0, 320, 200))
	eng := engine.New(320, 200, 30)
	defer eng.Stop()
	eng.SetRoot(w)

	eng.RenderOnce()
	if len(calls) != 1 || calls[0].set {
		t.Fatalf("окно без цвета рамки: вызовы %+v, ждал один с set=false", calls)
	}
	w.Invalidate()
	eng.RenderOnce()
	if len(calls) != 1 {
		t.Fatalf("кадр без изменений послал цвет ещё раз: %+v", calls)
	}

	w.SetFrameColor(accent)
	eng.RenderOnce()
	if len(calls) != 2 || !calls[1].set || calls[1].c != accent {
		t.Fatalf("SetFrameColor: вызовы %+v", calls)
	}

	w.SetActive(false)
	eng.RenderOnce()
	if len(calls) != 3 || !calls[2].set || calls[2].c == accent {
		t.Fatalf("неактивное окно: вызовы %+v — цвет должен приглушиться", calls)
	}

	w.SetActive(true)
	w.SetFrameColor(color.RGBA{})
	eng.RenderOnce()
	if last := calls[len(calls)-1]; last.set {
		t.Fatalf("снятый цвет рамки: последний вызов %+v, ждал set=false", last)
	}
}

// Диалог в окне ОС с её рамкой рисует корпус прямоугольным и без контура.
func TestSystemFrame_DialogDrawsNoContour(t *testing.T) {
	testreset.Globals(t)
	border := color.RGBA{R: 200, G: 30, B: 30, A: 255}
	render := func(sys bool) *image.RGBA {
		d := widget.NewDialog("Сравнение", 300, 180)
		d.BorderColor = border
		d.Shadow = color.RGBA{}
		d.CornerRadius = 8
		d.SystemFrame = sys
		d.SetBounds(image.Rect(0, 0, 300, 180))
		eng := engine.New(300, 180, 30)
		defer eng.Stop()
		eng.SetRoot(d)
		eng.RenderOnce()
		return snapshotRGBA(eng.RenderOnce())
	}
	own := render(false)
	if n := countColor(own, own.Rect, border); n < 500 {
		t.Fatalf("без SystemFrame точек контура диалога %d — тест ничего не проверяет", n)
	}
	sys := render(true)
	// Разделитель под заголовком того же цвета остаётся — он внутренний.
	// Проверяем края: левый, правый и нижний столбцы и строки без контура.
	edges := []image.Rectangle{
		image.Rect(0, 60, 1, 180), image.Rect(299, 60, 300, 180), image.Rect(0, 179, 300, 180),
	}
	for _, r := range edges {
		if n := countColor(sys, r, border); n != 0 {
			t.Errorf("с SystemFrame на краю %v осталось %d точек контура диалога", r, n)
		}
	}
	if corner, inside := sys.RGBAAt(0, 179), sys.RGBAAt(12, 170); corner != inside {
		t.Errorf("угол диалога %v, внутри рядом %v — корпус скруглён", corner, inside)
	}
}
