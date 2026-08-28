package tests

import (
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Два движка в одном процессе.
//
// Оболочка удалённого стола держит их именно так: свой движок на окно.
// Раньше состояние конвейера — damage обхода, выключатель пропуска, список
// объявленных переносов — лежало в переменных пакета, одних на процесс.
// Второй движок затирал их первому: тот пропускал поддерево по чужому damage
// и отдавал недорисованный кадр, а перенос из соседнего окна заставлял его
// копировать пиксели с места, где ничего не переезжало.
//
// Теперь всё это принадлежит движку, и тесты ниже проверяют именно
// разделение, а не то, что «оба как-то работают».

// buildScene2 собирает одинаковую сцену: фон, полоска и кнопка внизу.
func buildScene2(w, h int) (*widget.Panel, *widget.Button) {
	root := widget.NewPanel(color.RGBA{R: 30, G: 40, B: 60, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	for i := 0; i < 4; i++ {
		p := widget.NewPanel(color.RGBA{R: uint8(40 + 40*i), G: 120, B: 200, A: 255})
		p.ShowHeader = false
		p.SetBounds(image.Rect(20+i*70, 40, 80+i*70, 150))
		root.AddChild(p)
	}
	btn := widget.NewButton("Кнопка")
	btn.SetBounds(image.Rect(40, 200, 200, 240))
	root.AddChild(btn)
	return root, btn
}

// Выключатель пропуска — свой у каждого движка.
func TestTwoEngines_CullingSwitchIsPerEngine(t *testing.T) {
	a := engine.New(400, 300, 60)
	b := engine.New(400, 300, 60)

	a.SetSubtreeCulling(false)
	if a.SubtreeCulling() {
		t.Error("движок A не выключил пропуск у себя")
	}
	if !b.SubtreeCulling() {
		t.Error("выключение у движка A погасило пропуск и у движка B")
	}

	b.SetSubtreeCulling(false)
	a.SetSubtreeCulling(true)
	if !a.SubtreeCulling() || b.SubtreeCulling() {
		t.Errorf("состояния перепутались: A=%v, B=%v (ждали true и false)",
			a.SubtreeCulling(), b.SubtreeCulling())
	}
}

// Перенос, объявленный деревом одного движка, не уносит пиксели у другого.
//
// Оба движка получают объявление — реестр широковещательный, — но берёт его
// только тот, на чьём холсте оно лежит. Здесь холсты разного размера, и
// перенос помещается лишь в больший.
func TestTwoEngines_MoveGoesToTheRightOne(t *testing.T) {
	big, _ := buildScene2(800, 600)
	engBig := engine.New(800, 600, 60)
	engBig.SetRenderOnDemand(true)
	engBig.SetRoot(big)
	engBig.RenderOnce()
	engBig.RenderOnce()

	small, _ := buildScene2(200, 150)
	engSmall := engine.New(200, 150, 60)
	engSmall.SetRenderOnDemand(true)
	engSmall.SetRoot(small)
	engSmall.RenderOnce()
	engSmall.RenderOnce()

	// Область целиком за пределами маленького холста.
	from := image.Rect(400, 300, 600, 450)
	to := from.Add(image.Pt(40, 30))
	widget.NotifyMove(from, to)

	fSmall := engSmall.RenderFrameNow()
	if len(fSmall.Moves) != 0 {
		t.Errorf("маленький движок принял чужой перенос: %v", fSmall.Moves)
	}
	fBig := engBig.RenderFrameNow()
	if len(fBig.Moves) != 1 {
		t.Fatalf("большой движок получил %d переносов, ждали 1", len(fBig.Moves))
	}
	if fBig.Moves[0].Rect != to || fBig.Moves[0].From != from.Min {
		t.Errorf("перенос искажён: из %v в %v", fBig.Moves[0].From, fBig.Moves[0].Rect)
	}
}

// Два движка рисуют одновременно и получают каждый свой кадр.
//
// Здесь и жил исходный дефект. Damage обхода лежал в переменной пакета: один
// движок ставил её перед обходом, второй в этот же момент затирал своей — и
// первый пропускал поддеревья по чужой области, отдавая недорисованный кадр.
// Порча возможна только при наложении обходов во времени, поэтому проверка
// одна: движки крутятся в двух горутинах, а кадр каждого сверяется с кадром
// его же сцены, отрисованной в одиночестве.
//
// Тест ещё и под детектором гонок (`go test -race`), которому раньше было на
// что ругаться: к переменным пакета ходили без всякой синхронизации.
func TestTwoEngines_ConcurrentRendersStayApart(t *testing.T) {
	const rounds = 40

	// solo — эталонная последовательность кадров сцены без соседей.
	solo := func(w, h int) []*image.RGBA {
		root, btn := buildScene2(w, h)
		eng := engine.New(w, h, 60)
		eng.SetRenderOnDemand(true)
		eng.SetRoot(root)
		eng.RenderOnce()

		out := make([]*image.RGBA, 0, rounds)
		for i := 0; i < rounds; i++ {
			b := btn.Bounds()
			nb := b.Add(image.Pt(0, 1))
			btn.SetBounds(nb)
			eng.InvalidateRect(b.Union(nb))
			out = append(out, snapshotRGBA(eng.RenderOnce()))
		}
		return out
	}
	wantSmall := solo(400, 300)
	wantBig := solo(640, 480)

	// Кадры складываются в срез по указателю: горутина дописывает его, а
	// сравнение идёт после wg.Wait().
	run := func(w, h int, out *[]*image.RGBA, wg *sync.WaitGroup) {
		go func() {
			defer wg.Done()
			root, btn := buildScene2(w, h)
			eng := engine.New(w, h, 60)
			eng.SetRenderOnDemand(true)
			eng.SetRoot(root)
			eng.RenderOnce()

			for i := 0; i < rounds; i++ {
				b := btn.Bounds()
				nb := b.Add(image.Pt(0, 1))
				btn.SetBounds(nb)
				eng.InvalidateRect(b.Union(nb))
				*out = append(*out, snapshotRGBA(eng.RenderOnce()))
			}
		}()
	}

	var wg sync.WaitGroup
	var gotSmall, gotBig []*image.RGBA
	wg.Add(2)
	run(400, 300, &gotSmall, &wg)
	run(640, 480, &gotBig, &wg)
	wg.Wait()

	compare := func(name string, want, got []*image.RGBA) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: %d кадров вместо %d", name, len(got), len(want))
		}
		for i := range want {
			if !samePix(want[i], got[i]) {
				t.Fatalf("%s: кадр %d отличается от отрисованного в одиночестве — "+
					"обход пошёл по чужому damage", name, i)
			}
		}
	}
	compare("малый движок", wantSmall, gotSmall)
	compare("большой движок", wantBig, gotBig)
}

// Переносы двух движков не перемешиваются под нагрузкой.
func TestTwoEngines_ConcurrentMovesStayApart(t *testing.T) {
	const rounds = 30

	run := func(w, h int, wg *sync.WaitGroup) {
		defer wg.Done()
		root, btn := buildScene2(w, h)
		eng := engine.New(w, h, 60)
		eng.SetRenderOnDemand(true)
		eng.SetRoot(root)
		eng.RenderOnce()
		eng.RenderOnce()

		for i := 0; i < rounds; i++ {
			b := btn.Bounds()
			nb := b.Add(image.Pt(0, 1))
			btn.SetBounds(nb)
			widget.NotifyMove(b, nb)
			eng.InvalidateRect(b.Union(nb))

			// Каждый перенос лежит на обоих холстах, так что оба движка его
			// примут — но каждый ровно по одному разу за кадр. Раньше список
			// был общий: кто отрисовался первым, тот и забирал чужие.
			for _, m := range eng.RenderFrameNow().Moves {
				if !m.Rect.In(image.Rect(0, 0, w, h)) {
					t.Errorf("движок %dx%d получил перенос вне своего холста: %v", w, h, m.Rect)
					return
				}
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go run(400, 300, &wg)
	go run(640, 480, &wg)
	wg.Wait()
}

// samePix сравнивает кадры попиксельно.
func samePix(a, b *image.RGBA) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Bounds() != b.Bounds() {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}

// snapshotRGBA копирует кадр: движок отдаёт свой буфер, и следующий кадр
// перепишет его на месте.
func snapshotRGBA(src *image.RGBA) *image.RGBA {
	if src == nil {
		return nil
	}
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}

// Перетаскивание в одном движке не двигает пиксели в другом ТАКОГО ЖЕ размера.
//
// Две сессии одного разрешения в одном процессе — обычный случай для
// оболочки удалённого стола, и по координатам их объявления не различить:
// прямоугольник (100,100)-(400,300) лежит на обоих холстах. Различает только
// принадлежность дереву, которую объявление несёт с собой.
//
// Чужой перенос дорог не тем, что «лишний»: движок скопировал бы у себя
// пиксели с места, где ничего не переезжало, и потребитель показал бы чужой
// прямоугольник до следующего диффа.
func TestTwoEngines_SameSizeDoNotStealEachOthersMoves(t *testing.T) {
	const w, h = 800, 600

	build := func() (*widget.Panel, *widget.Window, *engine.Engine) {
		root := widget.NewPanel(color.RGBA{R: 24, G: 28, B: 36, A: 255})
		root.ShowHeader = false
		root.SetBounds(image.Rect(0, 0, w, h))

		win := widget.NewWindow("Окно", 300, 200)
		win.SetBounds(image.Rect(100, 100, 400, 300))
		root.AddChild(win)

		eng := engine.New(w, h, 60)
		eng.SetRenderOnDemand(true)
		eng.SetRoot(root)
		eng.RenderOnce()
		eng.RenderOnce() // съедаем полную инвалидацию от SetRoot
		return root, win, eng
	}

	_, _, engA := build()
	_, _, engB := build()

	// Тащим окно ТОЛЬКО в движке A — через сам движок, а не в обход:
	// объявление о переносе рождается внутри обработки ввода, и путь через
	// диспетчеризацию проверяет заодно, что движок не берёт при этом своих
	// замков (иначе приёмник объявления встал бы намертво).
	engA.SendMouseButton(200, 110, widget.MouseLeft, true)
	// Кадр после нажатия: клик движок инвалидирует целиком (он может открыть
	// меню, сместить фокус — задеть что угодно), а в полном кадре переносы
	// бессмысленны. В работе этот кадр уходит потребителю сразу же, и шаг
	// мыши приходит уже на чистое состояние.
	engA.RenderFrameNow()
	engB.RenderFrameNow()

	engA.SendMouseMove(240, 150)

	if got := len(engA.RenderFrameNow().Moves); got != 1 {
		t.Errorf("движок, в чьём дереве тащили окно, получил %d переносов вместо одного", got)
	}
	if got := engB.RenderFrameNow().Moves; len(got) != 0 {
		t.Errorf("соседний движок принял чужой перенос %v — он скопирует у себя "+
			"пиксели с места, где ничего не переезжало", got)
	}
}

// Объявление без виджета по-прежнему доходит: потребитель вправе звать
// NotifyMove сам, и тогда отличить своё от чужого можно только по холсту.
func TestTwoEngines_BareNotifyMoveStillReaches(t *testing.T) {
	root := widget.NewPanel(color.RGBA{R: 24, G: 28, B: 36, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 600, 400))

	eng := engine.New(600, 400, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.RenderOnce()
	eng.RenderOnce()

	from := image.Rect(50, 50, 250, 200)
	to := from.Add(image.Pt(40, 30))
	widget.NotifyMove(from, to)
	eng.InvalidateRect(from.Union(to))

	if got := len(eng.RenderFrameNow().Moves); got != 1 {
		t.Errorf("объявление без виджета дало %d переносов, ждали 1", got)
	}
}
