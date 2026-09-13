package tests

import (
	"image"
	"image/color"
	"testing"
	"testing/fstest"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// GG-71: отметку «только чтение» в шапке стороны можно убрать. GG-72: шапки
// сторон можно скрыть — код начинается от верхнего края.

func diffHeadersScene(t *testing.T) (*engine.Engine, *widget.DiffView) {
	t.Helper()
	dv := widget.NewDiffView("", "")
	dv.SetText(widget.DiffLeft, "old.go", "рабочая копия", "a\nb\nc\n")
	dv.SetText(widget.DiffRight, "new.go", "индекс", "a\nB\nc\n")
	dv.SetReadOnly(widget.DiffLeft, true)
	dv.SetReadOnly(widget.DiffRight, true)
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 800, 300))
	dv.SetBounds(image.Rect(0, 0, 800, 300))
	root.AddChild(dv)
	eng := engine.New(800, 300, 20)
	eng.SetRoot(root)
	return eng, dv
}

func diffPaneTop(t *testing.T, dv *widget.DiffView) int {
	t.Helper()
	for _, c := range widget.BuildAccessTree(dv, nil).Children {
		if c.Role == widget.RoleTextInput {
			return c.Bounds.Min.Y
		}
	}
	t.Fatal("панель стороны не найдена в дереве доступности")
	return 0
}

// Без шапок код начинается от верхнего края контрола, а область шапок не
// рисуется вовсе.
func TestDiffView_ShowHeaders(t *testing.T) {
	eng, dv := diffHeadersScene(t)
	if !dv.ShowHeaders() {
		t.Fatal("шапки по умолчанию скрыты")
	}
	withHeaders := diffPaneTop(t, dv)
	before := eng.RenderOnce()

	dv.SetShowHeaders(false)
	if got := diffPaneTop(t, dv); got != 0 || got >= withHeaders {
		t.Fatalf("без шапок панель начинается с %d (с шапками — %d), ждал 0", got, withHeaders)
	}
	after := eng.RenderOnce()
	if sameStrip(before, after, image.Rect(0, 0, 800, withHeaders)) {
		t.Fatal("полоса шапок не изменилась — карточки всё ещё рисуются")
	}

	dv.SetShowHeaders(true)
	if got := diffPaneTop(t, dv); got != withHeaders {
		t.Fatalf("возврат шапок: панель с %d, ждал %d", got, withHeaders)
	}
}

// Отметка «только чтение» убирается из шапки, а сторона остаётся только для
// чтения.
func TestDiffView_ShowReadOnlyMark(t *testing.T) {
	eng, dv := diffHeadersScene(t)
	if !dv.ShowReadOnlyMark() {
		t.Fatal("отметка по умолчанию скрыта")
	}
	top := diffPaneTop(t, dv)
	withMark := eng.RenderOnce()

	dv.SetShowReadOnlyMark(false)
	withoutMark := eng.RenderOnce()
	if sameStrip(withMark, withoutMark, image.Rect(0, 0, 800, top)) {
		t.Fatal("шапка не изменилась — отметка «только чтение» всё ещё рисуется")
	}
	if !sameStrip(withMark, withoutMark, image.Rect(0, top, 800, 300)) {
		t.Fatal("убрав отметку, изменился и код — должна меняться только шапка")
	}
	if !dv.IsReadOnly(widget.DiffLeft) {
		t.Fatal("убрав отметку, сторона перестала быть только для чтения")
	}
}

// Разметка: оба атрибута.
func TestDiffView_HeadersXAML(t *testing.T) {
	_, reg, err := widget.LoadUIFromXAMLFS([]byte(`<Canvas Width="400" Height="300">
  <DiffView x:Name="d" ShowHeaders="False" ShowReadOnlyMark="False"/>
</Canvas>`), fstest.MapFS{})
	if err != nil {
		t.Fatalf("разметка: %v", err)
	}
	dv := reg["d"].(*widget.DiffView)
	if dv.ShowHeaders() || dv.ShowReadOnlyMark() {
		t.Fatalf("атрибуты не применились: ShowHeaders=%v ShowReadOnlyMark=%v", dv.ShowHeaders(), dv.ShowReadOnlyMark())
	}
}

// sameStrip сравнивает прямоугольник двух кадров (в физических точках при
// масштабе 1).
func sameStrip(a, b *image.RGBA, r image.Rectangle) bool {
	r = r.Intersect(a.Bounds()).Intersect(b.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return false
			}
		}
	}
	return true
}
