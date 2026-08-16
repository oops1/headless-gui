package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// hittest_bench_test.go — стоимость построения пути hit-test (PERF-14).
// Путь строится на КАЖДОЕ нажатие/движение с адресной доставкой, а глубина
// дерева в реальном UI (окно → док → панель → грид → строка → ячейка → текст)
// легко доходит до десятка уровней.

// benchDeepTree строит цепочку вложенных панелей глубиной depth;
// все они содержат точку (5, 5).
func benchDeepTree(depth int) widget.Widget {
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 400, 400))
	cur := widget.Widget(root)
	for i := 0; i < depth; i++ {
		p := widget.NewPanel(color.RGBA{A: 255})
		p.SetBounds(image.Rect(0, 0, 400-i, 400-i))
		cur.(*widget.Panel).AddChild(p)
		cur = p
	}
	return root
}

func benchHitTestPath(b *testing.B, depth int) {
	root := benchDeepTree(depth)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if p := hitTestPath(root, 5, 5); len(p) != depth+1 {
			b.Fatalf("глубина пути %d, ожидалось %d", len(p), depth+1)
		}
	}
}

func BenchmarkHitTestPathDepth4(b *testing.B)  { benchHitTestPath(b, 4) }
func BenchmarkHitTestPathDepth12(b *testing.B) { benchHitTestPath(b, 12) }
func BenchmarkHitTestPathDepth24(b *testing.B) { benchHitTestPath(b, 24) }

// TestHitTestPathOrder — путь идёт от корня к самому глубокому виджету.
func TestHitTestPathOrder(t *testing.T) {
	root := benchDeepTree(5)
	path := hitTestPath(root, 5, 5)
	if len(path) != 6 {
		t.Fatalf("длина пути %d, ожидалось 6", len(path))
	}
	if path[0] != root {
		t.Error("путь должен начинаться с корня")
	}
	for i := 1; i < len(path); i++ {
		kids := path[i-1].Children()
		found := false
		for _, k := range kids {
			if k == path[i] {
				found = true
			}
		}
		if !found {
			t.Fatalf("элемент %d не является потомком предыдущего", i)
		}
	}
	// Точка вне дерева — пустой путь.
	if p := hitTestPath(root, 1000, 1000); p != nil {
		t.Errorf("вне дерева ожидался nil, получено %v", p)
	}
}
