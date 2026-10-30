package engine

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
)

// Окно, переехавшее с детьми, заявляет одно место несколько раз: в дифф
// оно идёт одной областью.
func TestMergeOverlapsFoldsOneWindowsRegions(t *testing.T) {
	testreset.Globals(t)
	win := image.Rect(100, 100, 600, 400)
	moved := win.Add(image.Pt(6, 3))
	rs := []image.Rectangle{win.Union(moved), win, moved, win.Inset(-24).Union(moved.Inset(-24)), image.Rect(1500, 900, 1600, 950)}
	got := mergeOverlaps(rs)
	if len(got) != 2 {
		t.Fatalf("%d regions, want the window's one and the far one apart: %v", len(got), got)
	}
	area := 0
	for _, r := range got {
		area += r.Dx() * r.Dy()
	}
	want := win.Inset(-24).Union(moved.Inset(-24))
	if area != want.Dx()*want.Dy()+100*50 {
		t.Fatalf("area %d, want %d", area, want.Dx()*want.Dy()+100*50)
	}
	if len(rs) != 5 || rs[1] != win {
		t.Fatal("the regions given were changed")
	}
}

// Касающиеся области — не пересекающиеся: они остаются раздельными.
func TestMergeOverlapsKeepsTouchingApart(t *testing.T) {
	testreset.Globals(t)
	got := mergeOverlaps([]image.Rectangle{image.Rect(0, 0, 10, 10), image.Rect(10, 0, 20, 10)})
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}
