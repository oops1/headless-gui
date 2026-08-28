package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Переехавшее содержимое.
//
// Перетаскивание окна не меняет пиксели — оно переносит их. Замер показал,
// что это самый дорогой сценарий рабочего стола: 1,85 мс и два мегабайта на
// кадр. Объявленный перенос позволяет потребителю скопировать картинку у
// себя вместо того, чтобы принимать её заново.

func TestMoves_WindowDragDeclaresMove(t *testing.T) {
	widget.DropMoves() // прошлые объявления к этому тесту не относятся

	win := widget.NewWindow("Окно", 300, 200)
	from := image.Rect(100, 100, 400, 300)
	win.SetBounds(from)
	widget.DropMoves() // SetBounds — это не перетаскивание

	// Тащим за заголовок.
	win.OnMouseButton(widget.MouseEvent{X: 200, Y: 110, Button: widget.MouseLeft, Pressed: true})
	win.OnMouseMove(230, 140)

	moves := widget.TakeMoves()
	if len(moves) == 0 {
		t.Fatal("перетаскивание окна не объявило переноса")
	}
	m := moves[len(moves)-1]
	if m.Rect.Dx() != from.Dx() || m.Rect.Dy() != from.Dy() {
		t.Errorf("перенос %v изменил размер окна %v — это уже не перенос", m.Rect, from)
	}
	if m.From == m.Rect.Min {
		t.Error("перенос никуда не переехал")
	}
	if dx, dy := m.Rect.Min.X-m.From.X, m.Rect.Min.Y-m.From.Y; dx != 30 || dy != 30 {
		t.Errorf("сдвиг (%d,%d), ждали (30,30) — столько прошла мышь", dx, dy)
	}
}

// Перенос не отменяет инвалидацию: он подсказка, как дешевле получить тот же
// результат, а не замена damage.
func TestMoves_StillReportsDamage(t *testing.T) {
	widget.DropMoves()

	win := widget.NewWindow("Окно", 200, 150)
	win.SetBounds(image.Rect(50, 50, 250, 200))
	win.OnMouseButton(widget.MouseEvent{X: 100, Y: 60, Button: widget.MouseLeft, Pressed: true})

	rects := collectDamage(func() { win.OnMouseMove(120, 80) })
	if len(rects) == 0 {
		t.Error("перетаскивание объявило перенос, но не заявило ни одной изменившейся области")
	}
	widget.DropMoves()
}

// Масштабирование переносом не считается: потребитель выполнит его блитом и
// получит искажение.
func TestMoves_RejectsResize(t *testing.T) {
	widget.DropMoves()
	widget.NotifyMove(image.Rect(0, 0, 100, 100), image.Rect(200, 200, 340, 340))
	if got := widget.TakeMoves(); len(got) != 0 {
		t.Errorf("объявлен перенос с изменением размера: %v", got)
	}
}

// Кадр несёт объявленные переносы наружу.
//
// Объявление берётся напрямую: то, что перетаскивание окна его порождает,
// проверено выше, а здесь важен путь «объявили — попало в кадр».
func TestMoves_ReachTheFrame(t *testing.T) {
	widget.DropMoves()

	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 600, 400))

	patch := widget.NewPanel(color.RGBA{R: 200, G: 80, B: 40, A: 255})
	patch.ShowHeader = false
	patch.SetBounds(image.Rect(50, 50, 250, 200))
	root.AddChild(patch)

	eng := engine.New(600, 400, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.RenderOnce()
	// Второй кадр съедает полную инвалидацию от SetRoot: в полном кадре
	// переносы бессмысленны, потребитель и так получает всё заново.
	eng.RenderOnce()

	from := patch.Bounds()
	to := from.Add(image.Pt(40, 30))
	patch.SetBounds(to)
	widget.NotifyMove(from, to)

	frame := eng.RenderFrameNow()
	if len(frame.Moves) == 0 {
		t.Fatal("кадр не принёс объявленного переноса")
	}
	m := frame.Moves[0]
	if m.Rect != to || m.From != from.Min {
		t.Errorf("перенос в кадре: из %v в %v, ждали из %v в %v",
			m.From, m.Rect, from.Min, to)
	}

	// Полный кадр переносов не несёт: они там ни к чему.
	widget.NotifyMove(to, to.Add(image.Pt(10, 10)))
	eng.Invalidate()
	if full := eng.RenderFrameNow(); len(full.Moves) != 0 {
		t.Errorf("полный кадр принёс %d переносов", len(full.Moves))
	}
}
