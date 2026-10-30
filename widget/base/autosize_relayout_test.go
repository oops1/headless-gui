package base

import (
	"image"
	"testing"
)

// GG-74: ширина по содержимому пересчитывается, когда меняется подпись.
//
// DockPanel и WrapPanel мерили ребёнка без явного размера один раз — пока его
// размер был нулевым. Подпись потом менялась, ширина оставалась от старой.

// То же в WrapPanel.
func TestWrapPanel_AutoWidthFollowsText(t *testing.T) {
	wp := NewWrapPanel(OrientationHorizontal)
	btn := NewButton("OK")
	wp.AddChild(btn)
	wp.SetBounds(image.Rect(0, 0, 600, 100))
	before := btn.Bounds().Dx()

	btn.SetText("Применить и закрыть окно")
	wp.Relayout()

	want := max(80, buttonContentWidth(btn, DesiredHeight(btn)))
	if got := btn.Bounds().Dx(); got != want || got <= before {
		t.Fatalf("ширина кнопки после смены подписи %d (до — %d), ждал %d", got, before, want)
	}
}
