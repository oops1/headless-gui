package desktop_test

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/theme"
)

// Элемент нулевой ширины (скрытое поле поиска) не оставляет двойного зазора,
// когда тема просит об этом флагом (Windows 11). Без флага — прежняя
// раскладка: в других темах такие элементы есть, и их кадры не меняются.
func TestTaskbar_EmptyItemAddsNoGap(t *testing.T) {
	for _, tc := range []struct {
		profile string
		skip    bool
	}{
		{theme.ProfileWindows11, true},
		{theme.ProfileWindows2000, false},
	} {
		tm, _ := edgeManager(t, tc.profile)
		bar := desktop.NewTaskbar(tm)
		a := newRowOnly(image.Pt(40, 40))
		hidden := newRowOnly(image.Pt(0, 40))
		b := newRowOnly(image.Pt(40, 40))
		bar.AddItem(desktop.SlotStart, a)
		bar.AddItem(desktop.SlotStart, hidden)
		bar.AddItem(desktop.SlotStart, b)
		bar.SetBounds(image.Rect(0, 0, 1000, 48))

		gap := b.Bounds().Min.X - a.Bounds().Max.X
		single := int(tm.GetMetric(desktop.KeyTaskbarGap))
		want := single
		if !tc.skip {
			want = 2 * single
		}
		if gap != want {
			t.Errorf("%s: промежуток между соседями %d, ждали %d (зазор %d)", tc.profile, gap, want, single)
		}
		bar.Close()
	}
}
