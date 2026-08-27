package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Всплывающая панель рабочего стола — оверлей движка, а не обычный ребёнок.
//
// Это не деталь реализации: оверлеи движок умеет выносить в отдельные окна ОС
// (SetPopupSink), и меню «Пуск» тогда может вылезать за границы окна
// оболочки, как настоящее системное меню. Если панель перестанет быть
// оверлеем, она молча начнёт обрезаться по окну — картинка останется
// «почти правильной», и заметить это по тестам поведения нельзя.
func TestDesktopFlyout_DrawsAsEngineOverlay(t *testing.T) {
	const w, h = 400, 300

	tm := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(tm); err != nil {
		t.Fatal(err)
	}
	if err := tm.SetTheme(theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}

	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	fl := desktop.NewFlyout(tm, "menu")
	fl.Screen = image.Rect(0, 0, w, h)
	fl.Size = func() image.Point { return image.Pt(120, 90) }
	// Содержимое заливает своё место — так его видно на снимке.
	painted := 0
	fl.Content = func(ctx widget.DrawContext, r image.Rectangle) {
		painted++
		ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{R: 255, G: 255, B: 255, A: 255})
	}
	fl.SetBounds(image.Rect(0, h-40, 60, h)) // «значок» на панели задач
	root.AddChild(fl)

	eng := engine.New(w, h, 30)
	eng.SetRoot(root)

	eng.RenderOnce()
	if painted != 0 {
		t.Fatalf("закрытая панель нарисовала содержимое (%d раз)", painted)
	}

	fl.Open(image.Rect(0, h-40, 60, h))
	img := eng.RenderOnce()
	if painted == 0 {
		t.Fatal("открытая панель не попала в отрисовку оверлеев движка")
	}
	if img == nil {
		t.Fatal("кадр не отрисован")
	}

	// Панель раскрылась ВВЕРХ от значка: пиксель над значком стал светлым.
	r := fl.OverlayBounds()
	if r.Empty() {
		t.Fatal("OverlayBounds пуст у открытой панели")
	}
	if r.Max.Y > h-40 {
		t.Errorf("панель залезла на значок: %v", r)
	}
	got := img.RGBAAt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
	if got.R < 200 || got.G < 200 || got.B < 200 {
		t.Errorf("в середине панели %v цвет %v — содержимое не нарисовано", r, got)
	}

	// Клик мимо закрывает — и оверлей исчезает из кадра.
	fl.OnMouseButton(widget.MouseEvent{X: w - 1, Y: 0, Button: widget.MouseLeft, Pressed: true})
	if fl.IsOpen() {
		t.Error("клик мимо не закрыл панель")
	}
}

// Панель не уезжает за экран, даже если значок стоит у самого края.
func TestDesktopFlyout_FitsIntoScreen(t *testing.T) {
	const w, h = 400, 300
	screen := image.Rect(0, 0, w, h)

	tm := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(tm); err != nil {
		t.Fatal(err)
	}
	if err := tm.SetTheme(theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}

	fl := desktop.NewFlyout(tm, "menu")
	fl.Screen = screen
	fl.Size = func() image.Point { return image.Pt(200, 150) }
	fl.Align = desktop.AlignStart

	// Значок в правом нижнем углу: панель шире оставшегося места справа.
	fl.Open(image.Rect(w-30, h-40, w, h))
	r := fl.OverlayBounds()
	if !r.In(screen) {
		t.Errorf("панель %v вышла за экран %v", r, screen)
	}
	if r.Dx() != 200 || r.Dy() != 150 {
		t.Errorf("панель %v изменила размер вместо сдвига", r)
	}
}
