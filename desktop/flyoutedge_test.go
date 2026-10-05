package desktop

import (
	"image"
	"testing"
)

// Положение окна у бокового края и при прижатии к краю монитора считается без
// движка: только геометрия Flyout.

func sideFlyout(edge Edge, align Align, anchor, screen image.Rectangle) *Flyout {
	f := NewFlyout(nil, "x")
	f.Margin = 4
	f.Edge, f.Align = edge, align
	f.Anchor, f.Screen = anchor, screen
	f.Size = func() image.Point { return image.Pt(200, 300) }
	return f
}

func TestFlyoutEdge_Vertical(t *testing.T) {
	for e, want := range map[Edge]bool{EdgeBottom: false, EdgeTop: false, EdgeLeft: true, EdgeRight: true} {
		if e.Vertical() != want {
			t.Errorf("Edge(%d).Vertical() = %v", e, !want)
		}
	}
}

func TestFlyoutEdge_LeftBarOpensToTheRight(t *testing.T) {
	screen := image.Rect(0, 0, 800, 600)
	anchor := image.Rect(0, 10, 40, 50)
	f := sideFlyout(EdgeLeft, AlignStart, anchor, screen)
	got := f.restRect()
	if want := image.Rect(44, 10, 244, 310); got != want {
		t.Errorf("Left/Start: %v, ждали %v", got, want)
	}
	// Низ окна на уровне низа значка: окно растёт вверх, но не выше экрана.
	f = sideFlyout(EdgeLeft, AlignEnd, image.Rect(0, 100, 40, 140), screen)
	if got, want := f.restRect(), image.Rect(44, 0, 244, 300); got != want {
		t.Errorf("Left/End: %v, ждали %v (прижато к верху экрана)", got, want)
	}
	f = sideFlyout(EdgeLeft, AlignEnd, image.Rect(0, 500, 40, 540), screen)
	if got, want := f.restRect(), image.Rect(44, 240, 244, 540); got != want {
		t.Errorf("Left/End у низа: %v, ждали %v", got, want)
	}
}

func TestFlyoutEdge_RightBarOpensToTheLeft(t *testing.T) {
	screen := image.Rect(0, 0, 800, 600)
	f := sideFlyout(EdgeRight, AlignStart, image.Rect(760, 560, 800, 600), screen)
	// Низ значка у низа экрана: окно сдвинуто вверх.
	if got, want := f.restRect(), image.Rect(556, 300, 756, 600); got != want {
		t.Errorf("Right/Start: %v, ждали %v", got, want)
	}
}

func TestFlyoutEdge_SlideFollowsEdge(t *testing.T) {
	for e, want := range map[Edge]SlideFrom{
		EdgeBottom: SlideBottom, EdgeTop: SlideTop, EdgeLeft: SlideLeft, EdgeRight: SlideRight,
	} {
		f := NewFlyout(nil, "x")
		f.Edge = e
		if got := f.slideFrom(); got != want {
			t.Errorf("Edge %d: slideFrom = %v, ждали %v", e, got, want)
		}
	}
}

// Окно у левой панели вырастает из-за её края: область движения упирается в
// панель, а не тянется до края экрана и не рисуется поверх панели.
func TestFlyoutEdge_MotionRegionStopsAtBar(t *testing.T) {
	screen := image.Rect(0, 0, 800, 600)
	f := sideFlyout(EdgeLeft, AlignStart, image.Rect(0, 10, 40, 50), screen)
	rest := f.restRect()
	f.setPresence(0.4)
	if r := f.motionRegion(rest); r.Min.X != 40 {
		t.Errorf("область движения %v должна начинаться у края панели (x=40)", r)
	}
	f = sideFlyout(EdgeRight, AlignStart, image.Rect(760, 10, 800, 50), screen)
	rest = f.restRect()
	f.setPresence(0.4)
	if r := f.motionRegion(rest); r.Max.X != 760 {
		t.Errorf("область движения %v должна кончаться у края панели (x=760)", r)
	}
	// Смещение при выезде идёт к панели.
	f.SlideDistance = 30
	f.setPresence(0)
	if off := f.slideOffset(); off.X <= 0 || off.Y != 0 {
		t.Errorf("смещение %v: окно справа выезжает вправо", off)
	}
}

func TestFlyoutEdge_PinToEdge(t *testing.T) {
	screen := image.Rect(1000, 0, 1800, 600)
	f := sideFlyout(EdgeBottom, AlignStart, image.Rect(1700, 560, 1740, 600), screen)
	f.Margin = 0
	f.SetMonitor(Monitor{ID: "b", Bounds: screen, WorkArea: image.Rect(1000, 0, 1800, 560)})
	f.PinToEdge(EdgeRight)

	if e, ok := f.Pinned(); !ok || e != EdgeRight {
		t.Fatalf("Pinned = %v %v", e, ok)
	}
	// Правый край рабочей области, верх рабочей области.
	if got, want := f.restRect(), image.Rect(1600, 0, 1800, 300); got != want {
		t.Errorf("Right/Start: %v, ждали %v", got, want)
	}
	f.Align = AlignEnd
	if got, want := f.restRect(), image.Rect(1600, 260, 1800, 560); got != want {
		t.Errorf("Right/End: %v, ждали %v (низ рабочей области)", got, want)
	}
	if f.slideFrom() != SlideRight {
		t.Errorf("прижатая к правому краю выезжает справа, а не %v", f.slideFrom())
	}

	f.PinToEdge(EdgeLeft)
	f.Align = AlignStart
	if got, want := f.restRect(), image.Rect(1000, 0, 1200, 300); got != want {
		t.Errorf("Left: %v, ждали %v", got, want)
	}
	f.PinToEdge(EdgeBottom)
	f.Align = AlignEnd
	if got, want := f.restRect(), image.Rect(1600, 260, 1800, 560); got != want {
		t.Errorf("Bottom/End: %v, ждали %v", got, want)
	}

	f.Unpin()
	if _, ok := f.Pinned(); ok {
		t.Error("Unpin не снял прижатие")
	}
	// Снова от значка: над ним, по правому краю значка (Align = End).
	if got, want := f.restRect(), image.Rect(1540, 260, 1740, 560); got != want {
		t.Errorf("после Unpin окно %v, ждали %v", got, want)
	}
}

// Окно, прижатое к монитору, вписывается в границы монитора, даже если рабочая
// область не задана.
func TestFlyoutEdge_PinWithoutWorkAreaUsesScreen(t *testing.T) {
	screen := image.Rect(0, 0, 500, 400)
	f := sideFlyout(EdgeBottom, AlignStart, image.Rect(0, 360, 40, 400), screen)
	f.Margin = 0
	f.PinToEdge(EdgeRight)
	if got, want := f.restRect(), image.Rect(300, 0, 500, 300); got != want {
		t.Errorf("%v, ждали %v", got, want)
	}
}

func TestFlyoutEdge_SetMonitorSetsScreen(t *testing.T) {
	f := NewFlyout(nil, "x")
	m := Monitor{ID: "m", Bounds: image.Rect(10, 20, 810, 620)}
	f.SetMonitor(m)
	if f.Screen != m.Bounds || f.Monitor().ID != "m" {
		t.Errorf("Screen = %v, Monitor = %+v", f.Screen, f.Monitor())
	}
	if w := m.Work(); w != m.Bounds {
		t.Errorf("Work без WorkArea = %v", w)
	}
}
