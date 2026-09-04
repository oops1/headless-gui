package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Виджет, лежащий МЕЖДУ двумя далёкими заявленными областями.
//
// Частичный кадр стирает фон и рисует по ОБЪЕДИНЕНИЮ областей: клип у холста
// один прямоугольник, двух он держать не умеет. А решение «это поддерево
// перерисовывать не надо» принималось по КАЖДОЙ области отдельно — и виджет в
// разрыве между ними попадал под стирание, но не под отрисовку.
//
// Само по себе это ещё не видно: дифф идёт по областям, и испорченное место
// потребителю не уходит. Уходит оно потому, что дифф работает ТАЙЛАМИ по 64
// точки: тайл, зацепивший заявленную область краем, сравнивается целиком — и
// уносит потребителю затёртого соседа, который в область не попал.
//
// На экране это пустой прямоугольник на месте кнопок или всплывающего меню,
// живущий до ближайшего полного кадра. Именно это владелец и увидел в showcase
// как мигание интерфейса.
func TestPartialFrame_WidgetBetweenTwoDamageRects(t *testing.T) {
	const w, h = 640, 480

	root := widget.NewPanel(color.RGBA{R: 24, G: 28, B: 36, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	// Жертва: в область не попадает, но делит тайл с заявленным уголком —
	// значит попадает в дифф.
	victim := widget.NewWindow("Жертва", 180, 100)
	victim.SetBounds(image.Rect(200, 200, 380, 300))
	root.AddChild(victim)

	eng := engine.New(w, h, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.RenderOnce()
	before := snapshotRGBA(eng.RenderOnce())
	if before == nil {
		t.Fatal("кадр не отрисован")
	}

	// Уголок в тайле (192,192)-(256,256), где начинается жертва, — и вторая
	// область далеко внизу справа, чтобы объединение накрыло жертву целиком.
	// Ничего на самом деле не менялось: правильный движок обязан вернуть ту
	// же картинку.
	eng.InvalidateRect(image.Rect(193, 193, 198, 198))
	eng.InvalidateRect(image.Rect(500, 400, 560, 440))
	after := snapshotRGBA(eng.RenderOnce())
	if after == nil {
		t.Fatal("частичный кадр не отрисован")
	}

	box := victim.Bounds()
	n := 0
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			i := before.PixOffset(x, y)
			if before.Pix[i] != after.Pix[i] ||
				before.Pix[i+1] != after.Pix[i+1] ||
				before.Pix[i+2] != after.Pix[i+2] {
				n++
			}
		}
	}
	if n != 0 {
		t.Errorf("виджет в разрыве между заявленными областями потерял %d точек "+
			"из %d — его затёрло фоном и не перерисовало", n, box.Dx()*box.Dy())
	}
}
