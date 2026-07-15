// Package tests — маршрутизация Drag&Drop файлов из ОС через движок
// (engine.SendFilesDropped): доставка виджету под точкой, всплытие (bubbling)
// и поглощение, логические координаты.
package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// dropTarget — тестовый виджет, реализующий widget.FileDropTarget.
type dropTarget struct {
	widget.Base
	consume    bool     // возвращать true (поглощать) из OnFilesDropped
	called     int      // сколько раз вызван
	gotPaths   []string // последние полученные пути
	gotX, gotY int      // последние координаты (логические)
}

func (d *dropTarget) Draw(ctx widget.DrawContext) {}

func (d *dropTarget) OnFilesDropped(x, y int, paths []string) bool {
	d.called++
	d.gotX, d.gotY = x, y
	d.gotPaths = paths
	return d.consume
}

// newDropTree строит дерево parent→child с заданными bounds.
func newDropTree() (parent, child *dropTarget) {
	parent = &dropTarget{}
	parent.SetBounds(image.Rect(0, 0, 400, 300))
	child = &dropTarget{}
	child.SetBounds(image.Rect(50, 50, 150, 150))
	parent.AddChild(child)
	return
}

func TestSendFilesDropped_DeliversToWidgetUnderPoint(t *testing.T) {
	eng := newTestEngine()
	parent, child := newDropTree()
	child.consume = true
	eng.SetRoot(parent)

	eng.SendFilesDropped(100, 100, []string{"/tmp/a.txt", "/tmp/b.txt"})

	if child.called != 1 {
		t.Fatalf("child.called = %d, want 1", child.called)
	}
	if parent.called != 0 {
		t.Fatalf("parent.called = %d, want 0 (child поглотил)", parent.called)
	}
	if len(child.gotPaths) != 2 || child.gotPaths[0] != "/tmp/a.txt" {
		t.Fatalf("child.gotPaths = %v, want [/tmp/a.txt /tmp/b.txt]", child.gotPaths)
	}
	if child.gotX != 100 || child.gotY != 100 {
		t.Fatalf("child coords = (%d,%d), want (100,100)", child.gotX, child.gotY)
	}
}

func TestSendFilesDropped_BubblesWhenNotConsumed(t *testing.T) {
	eng := newTestEngine()
	parent, child := newDropTree()
	child.consume = false // не поглощает — всплывает к родителю
	parent.consume = true
	eng.SetRoot(parent)

	eng.SendFilesDropped(100, 100, []string{"/x"})

	if child.called != 1 {
		t.Fatalf("child.called = %d, want 1", child.called)
	}
	if parent.called != 1 {
		t.Fatalf("parent.called = %d, want 1 (всплытие)", parent.called)
	}
}

func TestSendFilesDropped_OutsideChild_OnlyParent(t *testing.T) {
	eng := newTestEngine()
	parent, child := newDropTree()
	eng.SetRoot(parent)

	// Точка (300,250) вне child (50..150) — попадает только в parent.
	eng.SendFilesDropped(300, 250, []string{"/y"})

	if child.called != 0 {
		t.Fatalf("child.called = %d, want 0 (точка вне child)", child.called)
	}
	if parent.called != 1 {
		t.Fatalf("parent.called = %d, want 1", parent.called)
	}
}

func TestSendFilesDropped_EmptyPaths_NoOp(t *testing.T) {
	eng := newTestEngine()
	parent, child := newDropTree()
	eng.SetRoot(parent)

	eng.SendFilesDropped(100, 100, nil)
	eng.SendFilesDropped(100, 100, []string{})

	if child.called != 0 || parent.called != 0 {
		t.Fatalf("пустой список файлов не должен вызывать OnFilesDropped: child=%d parent=%d",
			child.called, parent.called)
	}
}

// TestSendFilesDropped_LogicalCoords — координаты, передаваемые виджету, уже
// логические: при HiDPI-масштабе физические пиксели делятся на scale.
func TestSendFilesDropped_LogicalCoords(t *testing.T) {
	eng := newTestEngine()
	parent, child := newDropTree()
	child.consume = true
	eng.SetRoot(parent)
	eng.SetScale(2)

	// Физическая точка (200,200) → логическая (100,100) — внутри child.
	eng.SendFilesDropped(200, 200, []string{"/z"})

	if child.called != 1 {
		t.Fatalf("child.called = %d, want 1", child.called)
	}
	if child.gotX != 100 || child.gotY != 100 {
		t.Fatalf("child coords = (%d,%d), want логические (100,100)", child.gotX, child.gotY)
	}
}

// TestSendFilesDropped_SkipsNonTargets — виджет в пути, НЕ реализующий
// FileDropTarget (обычная Panel), пропускается; событие доходит до целевого
// предка.
func TestSendFilesDropped_SkipsNonTargets(t *testing.T) {
	eng := newTestEngine()

	parent := &dropTarget{}
	parent.SetBounds(image.Rect(0, 0, 400, 300))
	parent.consume = true

	mid := widget.NewPanel(widget.DarkTheme().PanelBG) // не FileDropTarget
	mid.SetBounds(image.Rect(20, 20, 380, 280))
	parent.AddChild(mid)

	eng.SetRoot(parent)
	eng.SendFilesDropped(200, 150, []string{"/w"})

	if parent.called != 1 {
		t.Fatalf("parent.called = %d, want 1 (Panel пропущена, событие дошло до parent)", parent.called)
	}
}
