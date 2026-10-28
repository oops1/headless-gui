package draw

// record_review_test.go — случаи из состязательного ревью 4а.1: клип на
// выходе из Draw ребёнка, охват блита со сдвигом внутри буфера, кегль
// DrawText, группа прозрачности с долей ≥ 1 и ≤ 0.

import (
	"image"
	"testing"
)

func TestRecorder_KeepRestoresChildExitClip(t *testing.T) {
	m := stubMeasurer{200, 200}
	r := NewRecorder()
	// Ребёнок снимает клип в конце Draw; родитель пишет после него.
	child := &stubHolder{name: "child", draw: func(ctx DrawContext) {
		ctx.FillRect(10, 10, 10, 10, red)
		ctx.ClearClip()
	}}
	root := &stubHolder{name: "root"}
	root.draw = func(ctx DrawContext) {
		ctx.SetClip(image.Rect(0, 0, 50, 50))
		recordChild(r, child, image.Point{})
		ctx.FillRect(0, 0, 200, 200, red) // на прямом пути — уже без клипа
	}
	r.BeginFrame(m, true, nil)
	recordChild(r, root, image.Point{})
	l, exit := r.EndFrame()
	if !child.list.Exit().NoClip {
		t.Fatalf("клип на выходе ребёнка %+v, ждали снятый", child.list.Exit())
	}
	if got := l.Cmds[1]; !got.Clip.NoClip || got.BBox != image.Rect(0, 0, 200, 200) {
		t.Errorf("заливка после ребёнка: клип %+v, охват %v", *got.Clip, got.BBox)
	}
	if !exit.NoClip {
		t.Errorf("клип на выходе кадра %+v", exit)
	}

	// Родитель перезаписывается, ребёнок удержан: после Keep клип — его
	// выходной, заливка родителя снова без клипа.
	r.BeginFrame(m, true, nil)
	root.draw = func(ctx DrawContext) {
		ctx.SetClip(image.Rect(0, 0, 50, 50))
		if !r.CanReuse(child, image.Rect(10, 10, 20, 20), image.Point{}) {
			t.Fatal("ребёнок должен быть удержан")
		}
		r.Keep(child, image.Point{})
		ctx.FillRect(0, 0, 200, 200, red)
	}
	recordChild(r, root, image.Point{})
	l2, exit2 := r.EndFrame()
	if got := l2.Cmds[1]; !got.Clip.NoClip || got.BBox != image.Rect(0, 0, 200, 200) {
		t.Errorf("после Keep: клип %+v, охват %v", *got.Clip, got.BBox)
	}
	if !exit2.NoClip {
		t.Errorf("клип на выходе кадра после Keep %+v", exit2)
	}

	// Удержанный корень: ребёнок перестал снимать клип — обход сообщает, что
	// список корня устарел.
	child.draw = func(ctx DrawContext) { ctx.FillRect(10, 10, 10, 10, red) }
	r.BeginFrame(m, true, nil)
	stale := r.WalkRetained(root, func(c ListHolder, off image.Point) { recordChild(r, c.(*stubHolder), off) })
	r.EndFrame()
	if !stale {
		t.Error("ребёнок оставляет другой клип на выходе, список родителя должен устареть")
	}
	// И наоборот: перезапись с тем же выходом не старит.
	r.BeginFrame(m, true, nil)
	stale = r.WalkRetained(root, func(c ListHolder, off image.Point) { recordChild(r, c.(*stubHolder), off) })
	r.EndFrame()
	if stale {
		t.Error("тот же клип на выходе — список родителя верен")
	}
}

func TestRecorder_BlitBoundsOffsetInsideBuffer(t *testing.T) {
	m := stubMeasurer{200, 200}
	r := NewRecorder()
	h := &stubHolder{name: "h"}
	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	r.BeginFrame(m, true, nil)
	r.BeginRecord(h, image.Point{})
	r.BlitPixels(src, image.Rect(40, 30, 60, 50), 10, 10, true)     // кусок буфера со сдвигом
	r.BlitPixels(src, image.Rect(150, 150, 160, 160), 10, 10, true) // вне буфера — отбрасывается
	r.BlitPixels(src, image.Rect(90, 90, 120, 120), 0, 0, true)     // обрезается буфером
	r.EndRecord(h)
	l, _ := r.EndFrame()
	if l.Len() != 2 {
		t.Fatalf("команд %d, ждали 2", l.Len())
	}
	// Пиксель (0,0) буфера ложится в (10,10): кусок 40..60 × 30..50 лежит в
	// 50..70 × 40..60 (+1 запас по краю).
	if got, want := l.Cmds[0].BBox, image.Rect(50, 40, 71, 61); got != want {
		t.Errorf("охват блита со сдвигом %v, ждали %v", got, want)
	}
	if got := l.Cmds[1].Ext.Rect; got != image.Rect(90, 90, 100, 100) {
		t.Errorf("область источника не обрезана буфером: %v", got)
	}
}

func TestRecorder_DrawTextUsesDefaultFontSize(t *testing.T) {
	m := stubMeasurer{200, 200}
	r := NewRecorder()
	h := &stubHolder{name: "h"}
	r.BeginFrame(m, true, nil)
	r.BeginRecord(h, image.Point{})
	r.DrawText("a", 5, 20, red)
	r.EndRecord(h)
	l, _ := r.EndFrame()
	if l.Len() != 1 || l.Cmds[0].F[0] != DefaultFontSize() {
		t.Errorf("кегль DrawText %v, ждали кегль по умолчанию %v", l.Cmds[0].F[0], DefaultFontSize())
	}
}

func TestRecorder_OpacityEdges(t *testing.T) {
	m := stubMeasurer{200, 200}
	r := NewRecorder()
	h := &stubHolder{name: "h"}
	r.BeginFrame(m, true, nil)
	r.BeginRecord(h, image.Point{})
	r.DrawWithOpacity(image.Rect(0, 0, 50, 50), 1, func() { r.FillRect(0, 0, 10, 10, red) })
	r.DrawWithOpacity(image.Rect(0, 0, 50, 50), 0, func() { r.FillRect(20, 20, 10, 10, red) })
	r.DrawWithOpacity(image.Rect(0, 0, 50, 50), 0.5, nil)
	r.EndRecord(h)
	l, _ := r.EndFrame()
	if l.Len() != 1 || l.Cmds[0].Op != OpFillRect {
		t.Errorf("ждали одну заливку без группы (доля 1), получили %d команд", l.Len())
	}
}
