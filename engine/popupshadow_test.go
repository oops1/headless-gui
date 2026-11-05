package engine

import (
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// Хост, объявивший SetPopupShadow (окна-попапы с альфой точек), получает
// кадр меню с запасом под тень: тень и сглаженные скруглённые углы рисует
// движок, как в холсте. Без объявления кадр — ровно по границам оверлея, как
// прежде (хост, кладущий картинку непрозрачно, не видит лишнего).

func shadowMenuFrames(t *testing.T, shadow bool, scale float64) (PopupFrame, image.Rectangle) {
	t.Helper()
	e := New(400, 300, 60)
	t.Cleanup(e.Stop)
	if scale != 1 {
		e.SetScale(scale)
	}
	var mu sync.Mutex
	var last []PopupFrame
	e.SetPopupSink(func(f []PopupFrame) {
		mu.Lock()
		last = f
		mu.Unlock()
	})
	if shadow {
		e.SetPopupShadowMode(PopupShadowEngine)
	}
	root := widget.NewPanel(color.RGBA{R: 200, G: 200, B: 200, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 300))
	m := widget.NewPopupMenu()
	m.CornerRadius = 8
	m.Elevation = 8
	m.ShadowColor = color.RGBA{A: 90}
	m.Background = color.RGBA{R: 250, G: 250, B: 250, A: 255}
	m.SetItems([]widget.MenuItem{{Text: "Масштаб"}, {Text: "Строка состояния"}, {Text: "Перенос по словам"}})
	root.AddChild(m)
	e.SetRoot(root)
	m.Show(80, 60)
	body := m.OverlayBounds()
	e.RenderOnce()
	mu.Lock()
	defer mu.Unlock()
	if len(last) != 1 {
		t.Fatalf("кадров попапов %d, ждали 1", len(last))
	}
	return last[0], body
}

func TestPopupShadow_OffByDefault(t *testing.T) {
	testreset.Globals(t)
	f, body := shadowMenuFrames(t, false, 1)
	if f.Pad != 0 || f.Rect != body {
		t.Fatalf("без SetPopupShadow: Pad=%d Rect=%v, ждали 0 и %v", f.Pad, f.Rect, body)
	}
}

func TestPopupShadow_FrameCarriesShadowAndRoundCorners(t *testing.T) {
	testreset.Globals(t)
	for _, scale := range []float64{1, 1.5, 2} {
		f, body := shadowMenuFrames(t, true, scale)
		pad := widget.DefaultOverlayMargin()
		if f.Pad != pad || f.Rect != body.Inset(-pad) {
			t.Fatalf("scale %v: Pad=%d Rect=%v, ждали %d и %v", scale, f.Pad, f.Rect, pad, body.Inset(-pad))
		}
		ib := f.Img.Bounds()
		ph := func(v int) int { return int(float64(v)*scale + 0.5) }
		if ib.Dx() != ph(f.Rect.Dx()) || ib.Dy() != ph(f.Rect.Dy()) {
			t.Fatalf("scale %v: картинка %v не по Rect %v", scale, ib, f.Rect)
		}
		alpha := func(lx, ly int) uint8 { // логические координаты кадра
			return f.Img.RGBAAt(ib.Min.X+ph(lx), ib.Min.Y+ph(ly)).A
		}
		// Дальний угол запаса пуст — тень туда не достаёт.
		if a := alpha(0, 0); a != 0 {
			t.Errorf("scale %v: угол запаса закрашен (альфа %d)", scale, a)
		}
		// Под телом меню — тень: полупрозрачная, не пустая и не сплошная.
		if a := alpha(pad+body.Dx()/2, pad+body.Dy()+3); a == 0 || a == 255 {
			t.Errorf("scale %v: под меню альфа %d, ждали полупрозрачную тень", scale, a)
		}
		// Середина тела — сплошная, угол тела за скруглением — нет.
		if a := alpha(pad+body.Dx()/2, pad+body.Dy()/2); a != 255 {
			t.Errorf("scale %v: тело меню не сплошное (альфа %d)", scale, a)
		}
		if a := alpha(pad, pad); a == 255 {
			t.Errorf("scale %v: угол тела за скруглением сплошной — скругления нет", scale)
		}
	}
}

// Режим системы (DWM Windows 11): каскадное меню отдаётся окнами по уровням,
// без своей тени и с прямыми углами — скругляет и оттеняет ОС; радиус —
// в кадре. Прямоугольному меню ОС тени не даёт — при флаге движка её рисует
// движок, без него меню остаётся как было.
func systemMenuFrames(t *testing.T, mode PopupShadowMode, corner int) ([]PopupFrame, *widget.PopupMenu) {
	t.Helper()
	e := New(640, 420, 60)
	t.Cleanup(e.Stop)
	var mu sync.Mutex
	var last []PopupFrame
	e.SetPopupSink(func(f []PopupFrame) {
		mu.Lock()
		last = f
		mu.Unlock()
	})
	e.SetPopupShadowMode(mode)
	root := widget.NewPanel(color.RGBA{R: 200, G: 200, B: 200, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 640, 420))
	m := widget.NewPopupMenu()
	m.CornerRadius = corner
	m.Elevation = 8
	m.ShadowColor = color.RGBA{A: 90}
	m.SubMenuDelay = 0
	m.Background = color.RGBA{R: 250, G: 250, B: 250, A: 255}
	m.SetItems([]widget.MenuItem{
		{Text: "Масштаб", SubItems: []widget.MenuItem{{Text: "100%"}, {Text: "125%"}}},
		{Text: "Строка состояния"},
	})
	root.AddChild(m)
	e.SetRoot(root)
	m.Show(60, 60)
	body := m.OverlayBounds()
	m.OnMouseMove(body.Min.X+20, body.Min.Y+m.ItemHeight/2+4)
	e.RenderOnce()
	mu.Lock()
	defer mu.Unlock()
	return append([]PopupFrame(nil), last...), m
}

func TestPopupShadowSystem_CascadeByLevels(t *testing.T) {
	testreset.Globals(t)
	frames, m := systemMenuFrames(t, PopupShadowSystem|PopupShadowEngine, 8)
	parts := m.OverlayParts()
	if len(parts) != 2 {
		t.Fatalf("уровней меню %d, ждали 2 (подменю не раскрылось?)", len(parts))
	}
	if len(frames) != 2 {
		t.Fatalf("кадров %d, ждали окно на каждый уровень", len(frames))
	}
	if frames[0].ID == frames[1].ID {
		t.Fatal("у уровней один ключ окна")
	}
	for i, f := range frames {
		if !f.System || f.Corner != 8 || f.Pad != 0 || f.Rect != parts[i].Rect {
			t.Errorf("уровень %d: System=%v Corner=%d Pad=%d Rect=%v, ждали true, 8, 0, %v",
				i, f.System, f.Corner, f.Pad, f.Rect, parts[i].Rect)
		}
		// Углы кадра сплошные: скругление даст окно ОС, а окно без альфы
		// показало бы прозрачное чёрным.
		b := f.Img.Bounds()
		for _, p := range []image.Point{b.Min, {b.Max.X - 1, b.Min.Y}, {b.Min.X, b.Max.Y - 1}, b.Max.Sub(image.Pt(1, 1))} {
			if a := f.Img.RGBAAt(p.X, p.Y).A; a != 255 {
				t.Errorf("уровень %d: угол %v прозрачен (альфа %d)", i, p, a)
			}
		}
	}
}

func TestPopupShadowSystem_SquareMenuFallsBack(t *testing.T) {
	testreset.Globals(t)
	// Прямоугольное меню при «система или движок» — тень рисует движок.
	frames, _ := systemMenuFrames(t, PopupShadowSystem|PopupShadowEngine, 0)
	if len(frames) != 1 || frames[0].System || frames[0].Pad == 0 {
		t.Fatalf("прямоугольное меню: %d кадров, System=%v Pad=%d; ждали один кадр движка с запасом",
			len(frames), frames[0].System, frames[0].Pad)
	}
	// Только система — без тени и без запаса.
	frames, m := systemMenuFrames(t, PopupShadowSystem, 0)
	if len(frames) != 1 || frames[0].System || frames[0].Pad != 0 || frames[0].Rect != m.OverlayBounds() {
		t.Fatalf("только система, прямоугольное: %+v", frames[0])
	}
}
