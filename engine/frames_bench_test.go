package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// frames_bench_test.go — цена перевода координат (frames.go) на обходах, которые
// идут по ВСЕМУ дереву: на каждое движение мыши движок проходит каждый виджет, и
// лишняя проверка интерфейса ContentOffsetter на каждом из них не должна
// заметно удорожать обход.

// benchWideTree строит дерево из groups панелей по perGroup кнопок.
func benchWideTree(groups, perGroup int, inScroll bool) widget.Widget {
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 1000, 1000))
	var host widget.Widget = root
	if inScroll {
		sv := widget.NewScrollView()
		sv.SetBounds(image.Rect(0, 0, 1000, 1000))
		sv.ContentHeight = 5000
		root.AddChild(sv)
		host = sv
	}
	for g := 0; g < groups; g++ {
		p := widget.NewPanel(color.RGBA{A: 255})
		p.SetBounds(image.Rect(0, g*10, 1000, g*10+10))
		for i := 0; i < perGroup; i++ {
			b := widget.NewButton("x")
			b.SetBounds(image.Rect(i*5, g*10, i*5+5, g*10+10))
			p.AddChild(b)
		}
		host.AddChild(p)
	}
	return root
}

func BenchmarkBroadcastMouseMoveWide(b *testing.B) {
	root := benchWideTree(50, 100, false)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		broadcastMouseMove(root, 10, 10, 12+i%3, 12)
	}
}

func BenchmarkBroadcastMouseMoveWideInScroll(b *testing.B) {
	root := benchWideTree(50, 100, true)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		broadcastMouseMove(root, 10, 10, 12+i%3, 12)
	}
}

func BenchmarkHitTestWide(b *testing.B) {
	root := benchWideTree(50, 100, false)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if p := hitTestPath(root, 12, 12); len(p) == 0 {
			b.Fatal("точка вне дерева")
		}
	}
}
