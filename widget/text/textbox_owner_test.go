package text

// textbox_owner_test.go — кэш раскладки TextBox при смене владельца дерева
// (ревью 5.2, owner_services_review_test.go) на поддельном владельце.

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget/base"
)

// tbOwner — поддельный владелец с измерителем «k px на руну».
type tbOwner struct {
	set *base.MeasureSet
}

func (*tbOwner) TreeOwner() {}

func (o *tbOwner) OwnerMeasurers() *base.MeasureSet { return o.set }

func newTBOwner(k int) *tbOwner {
	text := func(s string, _ float64) int { return k * len([]rune(s)) }
	return &tbOwner{set: base.NewMeasureSet(base.Measurers{
		Text:     text,
		TextFont: func(s string, pt float64, _ string) int { return text(s, pt) },
	})}
}

func tbRoot(o base.Owner) *base.Panel {
	root := base.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 800, 600))
	base.BindOwner(root, o)
	return root
}

// Поле ввода, перенесённое в дерево другого владельца, держит перенос строк,
// посчитанный прежним (ключ раскладки — только ревизия и шрифт).
func TestOwnerServicesReview_TextBoxLayoutStaleAfterOwnerChange(t *testing.T) {
	useTestMeasurer(t)
	oa, ob := newTBOwner(5), newTBOwner(15)
	ra, rb := tbRoot(oa), tbRoot(ob)
	tb := NewTextBox("")
	tb.Wrap = true
	tb.SetBounds(image.Rect(0, 0, 300, 200))
	tb.SetText("один два три четыре пять шесть семь восемь девять десять")
	ra.AddChild(tb)
	tb.mu.Lock()
	tb.ensureLayout()
	nA := len(tb.lines)
	tb.mu.Unlock()

	ra.RemoveChild(tb)
	rb.AddChild(tb)
	if base.OwnerOf(tb) != base.Owner(ob) {
		t.Fatal("поле не перешло к B")
	}
	tb.mu.Lock()
	tb.ensureLayout()
	nB := len(tb.lines)
	tb.mu.Unlock()

	fresh := NewTextBox("")
	fresh.Wrap = true
	fresh.SetBounds(image.Rect(0, 0, 300, 200))
	fresh.SetText("один два три четыре пять шесть семь восемь девять десять")
	rb.AddChild(fresh)
	fresh.mu.Lock()
	fresh.ensureLayout()
	nFresh := len(fresh.lines)
	fresh.mu.Unlock()
	t.Logf("строк: в A %d, после переноса в B %d, свежее в B %d", nA, nB, nFresh)
	if nB != nFresh {
		t.Errorf("после переноса в B раскладка осталась от A: %d строк, свежая — %d", nB, nFresh)
	}
}
