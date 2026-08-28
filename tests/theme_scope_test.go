package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Тема на поддерево: две темы в одном кадре.
//
// До этого тема была одна на всё приложение — `ApplyGlobalTheme` пишет в общие
// переменные, а `Engine.SetTheme` обходит всё дерево. Оболочке удалённого
// стола нужно другое: окно гостя в его теме рядом со своим интерфейсом.

// scopeScene — кадр с двумя областями: слева тёмная тема, справа классическая.
func scopeScene(t *testing.T, w, h int) (*engine.Engine, *widget.ThemeScope, *widget.ThemeScope, *widget.Button, *widget.Button) {
	t.Helper()

	root := widget.NewPanel(color.RGBA{R: 10, G: 10, B: 10, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	left := widget.NewThemeScope(widget.DarkTheme())
	left.SetBounds(image.Rect(0, 0, w/2, h))
	leftBtn := widget.NewButton("Тёмная")
	leftBtn.SetBounds(image.Rect(20, 40, w/2-20, 80))
	left.AddChild(leftBtn)
	root.AddChild(left)

	right := widget.NewThemeScope(widget.Win2000Theme())
	right.SetBounds(image.Rect(w/2, 0, w, h))
	rightBtn := widget.NewButton("Классика")
	rightBtn.SetBounds(image.Rect(w/2+20, 40, w-20, 80))
	right.AddChild(rightBtn)
	root.AddChild(right)

	eng := engine.New(w, h, 30)
	eng.SetRoot(root)
	return eng, left, right, leftBtn, rightBtn
}

func TestThemeScope_TwoThemesInOneFrame(t *testing.T) {
	eng, _, _, leftBtn, rightBtn := scopeScene(t, 400, 160)

	if leftBtn.Background == rightBtn.Background {
		t.Errorf("кнопки в разных областях получили одинаковый фон %v — тема области не применилась",
			leftBtn.Background)
	}

	img := eng.RenderOnce()
	if img == nil {
		t.Fatal("кадр не отрисован")
	}
	lb, rb := leftBtn.Bounds(), rightBtn.Bounds()
	l := img.RGBAAt(lb.Min.X+lb.Dx()/2, lb.Min.Y+lb.Dy()/2)
	r := img.RGBAAt(rb.Min.X+rb.Dx()/2, rb.Min.Y+rb.Dy()/2)
	if l == r {
		t.Errorf("на кадре обе кнопки одного цвета %v — темы областей не дошли до отрисовки", l)
	}
}

// Классическая тема даёт фаску, тёмная — нет: форма тоже своя у каждой
// области, а не только цвет. Форма читается из общей переменной, поэтому
// именно её потерять проще всего.
func TestThemeScope_ShapeIsPerScope(t *testing.T) {
	eng, _, _, leftBtn, rightBtn := scopeScene(t, 400, 160)
	img := eng.RenderOnce()
	if img == nil {
		t.Fatal("кадр не отрисован")
	}

	// Фаску отличает объём: верхняя кромка кнопки светлая, нижняя тёмная.
	// Проверять «край отличается от середины» бесполезно — у плоской кнопки
	// тоже есть рамка, и такая проверка проходит всегда.
	bevelled := func(b image.Rectangle) bool {
		x := b.Min.X + b.Dx()/2
		top := img.RGBAAt(x, b.Min.Y)
		bottom := img.RGBAAt(x, b.Max.Y-1)
		return top != bottom
	}

	if !bevelled(rightBtn.Bounds()) {
		t.Error("классическая область осталась без фаски — форма взята у общей темы")
	}
	if bevelled(leftBtn.Bounds()) {
		t.Error("в тёмной области появилась фаска — на неё повлияла соседняя тема")
	}
}

// Глобальная смена темы не трогает область с собственной темой.
func TestThemeScope_SurvivesGlobalThemeChange(t *testing.T) {
	eng, _, right, _, rightBtn := scopeScene(t, 400, 160)

	before := rightBtn.Background
	eng.SetTheme(widget.DarkTheme()) // «весь интерфейс — тёмный»

	if rightBtn.Background != before {
		t.Errorf("область со своей темой перекрасилась глобальной сменой: %v → %v",
			before, rightBtn.Background)
	}
	if right.Theme() == nil {
		t.Error("область потеряла свою тему")
	}
}

// Область без темы ведёт себя как обычный контейнер: глобальная тема доходит
// до её детей.
func TestThemeScope_WithoutThemeIsTransparent(t *testing.T) {
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 200, 100))

	scope := widget.NewThemeScope(nil)
	btn := widget.NewButton("Обычная")
	btn.SetBounds(image.Rect(10, 10, 190, 50))
	scope.AddChild(btn)
	root.AddChild(scope)

	eng := engine.New(200, 100, 30)
	eng.SetRoot(root)

	before := btn.Background
	eng.SetTheme(widget.Win2000Theme())
	if btn.Background == before {
		t.Error("область без своей темы не пропустила глобальную смену к детям")
	}
}
