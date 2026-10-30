package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// Перетаскивание окна с тенью над однородным фоном: один перенос вместе с
// полями тени, и ни одного тайла целиком внутри перенесённой области — тень
// уехала копированием, а не тайлами.
func TestSceneShadowMove_WindowDragMovesShadow(t *testing.T) {
	testreset.Globals(t)
	eng := New(640, 480, 30)
	defer eng.Stop()
	eng.SetRenderOnDemand(true)
	root := widget.NewPanel(color.RGBA{R: 200, G: 210, B: 220, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 640, 480))
	win := widget.NewWindow("Окно", 256, 192)
	// Стиль заголовка — явно: по умолчанию он по ОС, и на macOS точка захвата
	// пришлась бы на кнопки заголовка слева.
	win.TitleStyle = widget.WindowTitleWin
	win.SetShadow(testShadow)
	win.SetBounds(image.Rect(128, 128, 384, 320))
	root.AddChild(win)
	eng.SetRoot(root)
	eng.RenderOnce()

	m := win.DrawMargin()
	if m == 0 {
		t.Fatal("у окна нет полей тени — тест не о том")
	}
	// Тянем за заголовок ровно на два тайла вправо и один вниз.
	eng.SendMouseButton(160, 135, widget.MouseLeft, true)
	// Нажатие перерисовывает кадр целиком (SendMouseButton), а в полном кадре
	// переносы не применяются — как и в жизни, перетаскивание идёт уже
	// следующими кадрами.
	eng.RenderFrameNow()
	eng.SendMouseMove(160+128, 135+64)
	fr := eng.RenderFrameNow()
	eng.SendMouseButton(160+128, 135+64, widget.MouseLeft, false)

	moved := win.Bounds()
	if moved.Min != image.Pt(256, 192) {
		t.Fatalf("окно не переехало: %v", moved)
	}
	want := moved.Inset(-m)
	if len(fr.Moves) != 1 || fr.Moves[0].Rect != want {
		t.Fatalf("переносы %+v, ждали один с Rect %v", fr.Moves, want)
	}
	for _, tl := range fr.Tiles {
		r := image.Rect(tl.X, tl.Y, tl.X+tl.W, tl.Y+tl.H)
		if r.In(want) {
			t.Fatalf("тайл %v целиком внутри перенесённой области ушёл тайлом", r)
		}
	}
}
