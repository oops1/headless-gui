package desktop_test

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/widget"
)

// Снимки быстрых настроек Windows 11: светлый и тёмный, главная страница,
// вложенная, режим правки. Кадры пишутся при QS_OUT.

func TestVisual_QuickSettingsWin11(t *testing.T) {
	cases := []struct {
		name string
		o    qsOpts
	}{
		{"light_main", qsOpts{}},
		{"dark_main", qsOpts{dark: true}},
		{"light_hover_focus", qsOpts{after: func(q *desktop.QuickSettings) {
			q.SetFocused(true)
			qsKey(q, widget.KeyTab, 0)
			qsKey(q, widget.KeyRight, 0)
			qsKey(q, widget.KeyRight, 0)
			t := desktop.QSTileRect(q, "bt")
			q.OnMouseMove(t.Min.X+10, t.Min.Y+10)
		}}},
		{"dark_chevron_hover", qsOpts{dark: true, after: func(q *desktop.QuickSettings) {
			c := desktop.QSChevronRect(q, "wifi")
			q.OnMouseMove(c.Min.X+5, c.Min.Y+5)
		}}},
		{"light_details", qsOpts{after: func(q *desktop.QuickSettings) {
			qsClick(q, centreOf(desktop.QSChevronRect(q, "wifi")))
			qsFinish()
		}}},
		{"dark_details", qsOpts{dark: true, after: func(q *desktop.QuickSettings) {
			qsClick(q, centreOf(desktop.QSZoneRect(q, "volchev")))
			qsFinish()
		}}},
		{"light_edit", qsOpts{after: func(q *desktop.QuickSettings) {
			qsClick(q, centreOf(desktop.QSZoneRect(q, "edit")))
		}}},
		{"dark_edit", qsOpts{dark: true, after: func(q *desktop.QuickSettings) {
			qsClick(q, centreOf(desktop.QSZoneRect(q, "edit")))
		}}},
		{"light_no_bright_3", qsOpts{noBright: true, model: func(m *desktop.QuickActionList) {
			m.Replace(m.List()[:3])
		}}},
		{"dark_200", qsOpts{dark: true, scale: 2, w: 640, h: 560}},
		{"light_150", qsOpts{scale: 1.5, w: 800, h: 600}},
		{"light_mica", qsOpts{mica: true}},
		{"dark_mica", qsOpts{mica: true, dark: true}},
		{"light_dragging", qsOpts{after: func(q *desktop.QuickSettings) {
			q.SetEditing(true)
			from := centreOf(desktop.QSTileRect(q, "wifi"))
			q.OnMouseButton(widget.MouseEvent{X: from.X, Y: from.Y, Button: widget.MouseLeft, Pressed: true})
			to := centreOf(desktop.QSTileRect(q, "saver"))
			for i := 1; i <= 8; i++ {
				q.OnMouseMove(from.X+(to.X-from.X)*i/8+6, from.Y+(to.Y-from.Y)*i/8+4)
			}
		}}},
		{"dark_transition", qsOpts{dark: true, after: func(q *desktop.QuickSettings) {
			qsClick(q, centreOf(desktop.QSChevronRect(q, "bt")))
			t0 := time.Now()
			widget.StepAnimations(t0)
			widget.StepAnimations(t0.Add(90 * time.Millisecond))
		}}},
		{"light_scrolled", qsOpts{after: func(q *desktop.QuickSettings) {
			g := desktop.QSZoneRect(q, "grid")
			q.OnMouseWheelPixels(g.Min.X+10, g.Min.Y+10, 0, 60)
		}}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			s := qsBuild(t, c.o)
			img := s.render()
			qsSave(t, img, "qs_"+c.name)
			if r := desktop.QSPanelRect(s.q); r.Empty() {
				t.Fatal("панель не показана")
			}
		})
	}
}

func centreOf(r image.Rectangle) image.Point {
	return image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
}

var _ = time.Now
