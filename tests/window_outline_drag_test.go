// Тесты перетаскивания окна контуром (ENGINE_ISSUES winline, v3.13.4):
// окно стоит на месте, пока виден контур, и переезжает один раз — при
// отпускании кнопки.
package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// TestWindow_OutlineDrag_MovesOnce — окно на канвасе: во время drag bounds
// не меняются, контур едет за мышью, при отпускании окно встаёт на его место.
func TestWindow_OutlineDrag_MovesOnce(t *testing.T) {
	w := widget.NewWindow("win", 300, 200)
	w.SetBounds(image.Rect(100, 50, 400, 250))
	w.OutlineDrag = true
	start := w.Bounds()

	pressAt(w, 200, 60) // заголовок
	if !w.HasOverlay() {
		t.Error("контур не показан после начала drag")
	}

	w.OnMouseMove(260, 110) // +60, +50
	if got := w.Bounds(); got != start {
		t.Errorf("окно сдвинулось во время drag: %v, ждали %v", got, start)
	}

	w.OnMouseMove(240, 90) // назад: суммарно +40, +30
	if got := w.Bounds(); got != start {
		t.Errorf("окно сдвинулось во время drag: %v", got)
	}

	releaseAt(w, 240, 90)
	want := start.Add(image.Pt(40, 30))
	if got := w.Bounds(); got != want {
		t.Errorf("после отпускания окно в %v, ждали %v", got, want)
	}
	if w.HasOverlay() {
		t.Error("контур остался виден после отпускания")
	}
}

// TestWindow_OutlineDrag_NativeCallsDragMoveOnce — нативный режим: OnDragMove
// зовётся ровно один раз, с итоговым смещением.
func TestWindow_OutlineDrag_NativeCallsDragMoveOnce(t *testing.T) {
	w := widget.NewWindow("win", 300, 200)
	w.SetBounds(image.Rect(0, 0, 300, 200))
	w.OutlineDrag = true
	calls, dx, dy := 0, 0, 0
	w.OnDragMove = func(mx, my int) { calls++; dx, dy = mx, my }

	pressAt(w, 150, 10)
	w.OnMouseMove(200, 40)
	w.OnMouseMove(230, 70)
	if calls != 0 {
		t.Errorf("OnDragMove вызван %d раз во время drag, ждали 0", calls)
	}
	releaseAt(w, 230, 70)
	if calls != 1 || dx != 80 || dy != 60 {
		t.Errorf("OnDragMove: calls=%d dx=%d dy=%d, ждали 1/80/60", calls, dx, dy)
	}
}

// TestWindow_OutlineDrag_Off — без флага поведение прежнее: окно едет сразу.
func TestWindow_OutlineDrag_Off(t *testing.T) {
	w := widget.NewWindow("win", 300, 200)
	w.SetBounds(image.Rect(100, 50, 400, 250))
	start := w.Bounds()

	pressAt(w, 200, 60)
	w.OnMouseMove(240, 90)
	want := start.Add(image.Pt(40, 30))
	if got := w.Bounds(); got != want {
		t.Errorf("окно в %v, ждали %v (обычное перетаскивание)", got, want)
	}
	if w.HasOverlay() {
		t.Error("контур не должен показываться без OutlineDrag")
	}
	releaseAt(w, 240, 90)
}

// TestWindow_OutlineDrag_XAML — атрибуты OutlineDrag/OutlineDragFill.
func TestWindow_OutlineDrag_XAML(t *testing.T) {
	const xaml = `<Window Title="w" Width="400" Height="300"
		OutlineDrag="True" OutlineDragFill="#20303030"/>`
	root, _, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatal(err)
	}
	w, ok := root.(*widget.Window)
	if !ok {
		t.Fatalf("корень %T", root)
	}
	if !w.OutlineDrag {
		t.Error("OutlineDrag=\"True\" не применился")
	}
	if w.OutlineDragFill.A == 0 {
		t.Error("OutlineDragFill не разобран")
	}
}

// ─── Стоимость контура (ENGINE_ISSUES winline, v3.13.5) ─────────────────────

// countingCtx — DrawContext, считающий залитые пиксели: заливка контура
// перекрашивает всё под ним, рамка — лишь тонкие полосы.
type countingCtx struct {
	recCtx
	filledPixels int
}

func (c *countingCtx) FillRectAlpha(x, y, w, h int, col color.RGBA) { c.filledPixels += w * h }

// TestWindow_OutlineDrag_BorderOnlyByDefault — по умолчанию контур не
// заливается: заливка перекрашивает каждый пиксель под окном и по сети
// обходится дороже, чем перемещение самого окна.
func TestWindow_OutlineDrag_BorderOnlyByDefault(t *testing.T) {
	w := widget.NewWindow("win", 300, 200)
	w.SetBounds(image.Rect(0, 0, 300, 200))
	w.OutlineDrag = true
	pressAt(w, 150, 10)
	w.OnMouseMove(200, 60)

	ctx := &countingCtx{}
	w.DrawOverlay(ctx)
	if ctx.filledPixels != 0 {
		t.Errorf("по умолчанию залито %d пикселей, ждали 0 (только рамка)", ctx.filledPixels)
	}

	// Явно запрошенная заливка рисуется.
	w.OutlineDragStyle = widget.OutlineDragFilled
	w.OutlineDragFill = color.RGBA{R: 20, G: 20, B: 20, A: 60}
	ctx2 := &countingCtx{}
	w.DrawOverlay(ctx2)
	if ctx2.filledPixels != 300*200 {
		t.Errorf("при OutlineDragFilled залито %d пикселей, ждали %d", ctx2.filledPixels, 300*200)
	}
	releaseAt(w, 200, 60)
}

// TestWindow_OutlineDrag_DamageStaysLocal — перемещение и посадка контура
// не требуют полного кадра: хост получает только затронутые области.
// Полная инвалидация скрыла бы от него, что окно просто переехало, — а по
// этому RDP-оболочка решает, можно ли обойтись командой копирования.
func TestWindow_OutlineDrag_DamageStaysLocal(t *testing.T) {
	w := widget.NewWindow("win", 300, 200)
	w.SetBounds(image.Rect(100, 100, 400, 300))
	w.OutlineDrag = true

	var fullCalls int
	var rects []image.Rectangle
	h := widget.RegisterUINotifier(
		func() { fullCalls++ },
		func(r image.Rectangle) { rects = append(rects, r) },
	)
	defer widget.UnregisterUINotifier(h)

	step := func(name string, fn func()) {
		t.Helper()
		fullCalls, rects = 0, nil
		fn()
		if fullCalls != 0 {
			t.Errorf("%s: %d полных инвалидаций, ждали 0", name, fullCalls)
		}
		if len(rects) == 0 {
			t.Errorf("%s: об области не сообщили вовсе", name)
		}
		screen := image.Rect(0, 0, 1200, 800)
		for _, r := range rects {
			// Затронутое — окрестность окна, а не весь экран.
			if r.Dx() > 700 || r.Dy() > 500 {
				t.Errorf("%s: заявлена область %v — почти весь экран %v", name, r, screen)
			}
		}
	}

	step("движение контура", func() {
		pressAt(w, 200, 110)
		w.OnMouseMove(260, 160)
	})
	step("посадка окна", func() { releaseAt(w, 260, 160) })

	if got := w.Bounds(); got != image.Rect(160, 150, 460, 350) {
		t.Errorf("окно после посадки в %v", got)
	}
}
