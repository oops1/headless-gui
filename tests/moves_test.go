package tests

import (
	"image"
	"image/color"
	"sync"
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

// recordMoves подписывается на объявления о переносе на время теста —
// ровно так же, как это делает движок. Возвращает функцию, отдающую
// накопленное.
//
// Своя подписка на тест, а не общий список пакета: объявления идут всем
// подписчикам сразу, и тест видит только те, что случились при нём.
func recordMoves(t *testing.T) func() []widget.MoveNotice {
	t.Helper()
	var mu sync.Mutex
	var got []widget.MoveNotice
	h := widget.RegisterMoveSink(func(n widget.MoveNotice) {
		mu.Lock()
		got = append(got, n)
		mu.Unlock()
	})
	t.Cleanup(func() { widget.UnregisterMoveSink(h) })
	return func() []widget.MoveNotice {
		mu.Lock()
		defer mu.Unlock()
		out := got
		got = nil
		return out
	}
}

func TestMoves_WindowDragDeclaresMove(t *testing.T) {
	win := widget.NewWindow("Окно", 300, 200)
	from := image.Rect(100, 100, 400, 300)
	win.SetBounds(from)

	// Подписываемся ПОСЛЕ SetBounds: это не перетаскивание.
	taken := recordMoves(t)

	// Тащим за заголовок.
	win.OnMouseButton(widget.MouseEvent{X: 200, Y: 110, Button: widget.MouseLeft, Pressed: true})
	win.OnMouseMove(230, 140)

	moves := taken()
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
	win := widget.NewWindow("Окно", 200, 150)
	win.SetBounds(image.Rect(50, 50, 250, 200))
	win.OnMouseButton(widget.MouseEvent{X: 100, Y: 60, Button: widget.MouseLeft, Pressed: true})

	rects := collectDamage(func() { win.OnMouseMove(120, 80) })
	if len(rects) == 0 {
		t.Error("перетаскивание объявило перенос, но не заявило ни одной изменившейся области")
	}
}

// Масштабирование переносом не считается: потребитель выполнит его блитом и
// получит искажение.
func TestMoves_RejectsResize(t *testing.T) {
	taken := recordMoves(t)
	widget.NotifyMove(image.Rect(0, 0, 100, 100), image.Rect(200, 200, 340, 340))
	if got := taken(); len(got) != 0 {
		t.Errorf("объявлен перенос с изменением размера: %v", got)
	}
}

// Кадр несёт объявленные переносы наружу.
//
// Объявление берётся напрямую: то, что перетаскивание окна его порождает,
// проверено выше, а здесь важен путь «объявили — попало в кадр».
func TestMoves_ReachTheFrame(t *testing.T) {
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

// Объявленный перенос ЭКОНОМИТ трафик, а не только описывает его.
//
// Дифф сравнивает новый задний буфер со старым передним: на старом месте
// окна ещё нет, на новом уже есть — и оба места уезжают тайлами. Пока
// перенос не применялся к переднему буферу, потребитель получал и команду
// копирования, и все пиксели, то есть платил как без переноса.
func TestMoves_SaveTraffic(t *testing.T) {
	const w, h = 600, 400

	// Одна и та же сцена и один и тот же сдвиг: с объявлением и без.
	tilesFor := func(declare bool) (int, int) {
		root := widget.NewPanel(color.RGBA{R: 20, G: 24, B: 30, A: 255})
		root.ShowHeader = false
		root.SetBounds(image.Rect(0, 0, w, h))

		// Пёстрая «картинка окна»: одноцветную дифф не заметил бы и без
		// переноса — экономия оказалась бы мнимой.
		win := widget.NewPanel(color.RGBA{R: 200, G: 90, B: 40, A: 255})
		win.ShowHeader = false
		from := image.Rect(60, 60, 300, 260)
		win.SetBounds(from)
		root.AddChild(win)
		for i := 0; i < 8; i++ {
			stripe := widget.NewPanel(color.RGBA{R: uint8(30 + i*25), G: 70, B: 160, A: 255})
			stripe.ShowHeader = false
			stripe.SetBounds(image.Rect(70+i*28, 70, 90+i*28, 250))
			root.AddChild(stripe)
		}

		eng := engine.New(w, h, 60)
		eng.SetRenderOnDemand(true)
		eng.SetRoot(root)
		eng.RenderOnce()
		eng.RenderOnce() // съедаем полную инвалидацию от SetRoot

		// Двигаем окно вместе с содержимым.
		delta := image.Pt(120, 90)
		to := from.Add(delta)
		win.SetBounds(to)
		for _, child := range root.Children() {
			if child == win {
				continue
			}
			b := child.Bounds()
			if b.In(from) {
				child.SetBounds(b.Add(delta))
			}
		}
		if declare {
			widget.NotifyMove(from, to)
		}

		frame := eng.RenderFrameNow()

		bytes := 0
		for _, tile := range frame.Tiles {
			bytes += len(tile.Data)
		}
		return len(frame.Tiles), bytes
	}

	withTiles, withBytes := tilesFor(true)
	plainTiles, plainBytes := tilesFor(false)

	if withBytes >= plainBytes {
		t.Errorf("с объявленным переносом ушло %d байт в %d тайлах, без него — %d в %d: экономии нет",
			withBytes, withTiles, plainBytes, plainTiles)
	}
	t.Logf("перенос: %d тайлов / %d байт; без переноса: %d тайлов / %d байт (%.0f%% от прежнего)",
		withTiles, withBytes, plainTiles, plainBytes,
		100*float64(withBytes)/float64(plainBytes))
}

// Переносы в одном кадре не пересекаются.
//
// Движок применяет их к своему буферу подряд, и второй читает пиксели, уже
// переложенные первым. Потребитель повторить этот порядок не обязан: команда
// копии поверхности в RDP выполняется когда угодно относительно соседних, а
// локальный потребитель вправе применить их разом. Разошедшийся порядок даёт
// копию с неверным источником — прямоугольник чужого содержимого с резким
// швом, ровно как в незакрытом наблюдении заказчика.
//
// Поэтому кадр несёт только непересекающиеся переносы; пересекающиеся уезжают
// обычными тайлами.
func TestMoves_FrameCarriesNoOverlappingMoves(t *testing.T) {
	const w, h = 600, 400

	root := widget.NewPanel(color.RGBA{R: 20, G: 24, B: 30, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	eng := engine.New(w, h, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.RenderOnce()
	eng.RenderOnce()

	// Второй перенос забирает пиксели оттуда, куда только что приехал первый.
	a := image.Rect(50, 50, 250, 200)
	aTo := a.Add(image.Pt(100, 0))
	b := aTo.Add(image.Pt(20, 20)) // накрывает место приземления первого
	bTo := b.Add(image.Pt(0, 120))

	widget.NotifyMove(a, aTo)
	widget.NotifyMove(b, bTo)
	// Кадр обязан быть частичным: в полном переносы бессмысленны.
	eng.InvalidateRect(a.Union(aTo))
	eng.InvalidateRect(b.Union(bTo))

	frame := eng.RenderFrameNow()
	if len(frame.Moves) == 0 {
		t.Fatal("кадр не принёс ни одного переноса")
	}
	for i := 0; i < len(frame.Moves); i++ {
		for j := i + 1; j < len(frame.Moves); j++ {
			for _, x := range []image.Rectangle{frame.Moves[i].Src(), frame.Moves[i].Rect} {
				for _, y := range []image.Rectangle{frame.Moves[j].Src(), frame.Moves[j].Rect} {
					if x.Overlaps(y) {
						t.Errorf("переносы %d и %d пересекаются (%v и %v): "+
							"потребитель применит их в своём порядке и скопирует не то",
							i, j, x, y)
					}
				}
			}
		}
	}
}

// Непересекающиеся переносы кадр несёт все.
func TestMoves_IndependentMovesAllSurvive(t *testing.T) {
	const w, h = 600, 400

	root := widget.NewPanel(color.RGBA{R: 20, G: 24, B: 30, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	eng := engine.New(w, h, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.RenderOnce()
	eng.RenderOnce()

	a := image.Rect(10, 10, 110, 80)
	bb := image.Rect(300, 250, 400, 320)
	widget.NotifyMove(a, a.Add(image.Pt(120, 0)))
	widget.NotifyMove(bb, bb.Add(image.Pt(0, 60)))
	eng.InvalidateRect(a.Union(a.Add(image.Pt(120, 0))))
	eng.InvalidateRect(bb.Union(bb.Add(image.Pt(0, 60))))

	if got := len(eng.RenderFrameNow().Moves); got != 2 {
		t.Errorf("кадр принёс %d переносов из двух непересекающихся", got)
	}
}
