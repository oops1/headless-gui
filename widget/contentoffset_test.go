package widget

import (
	"image"
	"testing"
)

// contentoffset_test.go — перевод областей перерисовки и границ для доступности
// у детей контейнера со сдвигом (contentoffset.go). Хит-тест, события и кадр —
// через настоящий движок: engine/frames_test.go и tests/scrollview_coords_test.go.

// captureRects подменяет внешний приёмник точечных уведомлений и возвращает то,
// что виджеты успели заявить; приёмник снимается по завершении теста.
func captureRects(t *testing.T) *[]image.Rectangle {
	t.Helper()
	var got []image.Rectangle
	SetUIRectChangeNotifier(func(r image.Rectangle) { got = append(got, r) })
	t.Cleanup(func() { SetUIRectChangeNotifier(nil) })
	return &got
}

// scrolledScene — прокрутка 300×100 на scrollY, в ней кнопка на y=200..230.
func scrolledScene(t *testing.T, scrollY int) (*ScrollView, *Button) {
	t.Helper()
	sv := NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 300, 100))
	sv.ContentHeight = 400
	btn := NewButton("B")
	btn.SetBounds(image.Rect(10, 200, 200, 230))
	sv.AddChild(btn)
	sv.SetScrollY(scrollY)
	if sv.ScrollY() != scrollY {
		t.Fatalf("ScrollY = %d, ждали %d", sv.ScrollY(), scrollY)
	}
	return sv, btn
}

func TestScreenRect_InvalidateTranslatedByScroll(t *testing.T) {
	_, btn := scrolledScene(t, 200)
	got := captureRects(t)

	btn.Invalidate()

	want := image.Rect(10, 0, 200, 30) // место кнопки на экране
	if len(*got) != 1 || (*got)[0] != want {
		t.Fatalf("заявлено %v, ждали [%v]", *got, want)
	}
}

// Пока смещение нулевое, поведение прежнее до пикселя: область как есть, без
// обрезки по прокрутке.
func TestScreenRect_ZeroOffsetIsUntouched(t *testing.T) {
	_, btn := scrolledScene(t, 0)
	got := captureRects(t)

	btn.Invalidate()

	if want := btn.Bounds(); len(*got) != 1 || (*got)[0] != want {
		t.Fatalf("заявлено %v, ждали [%v]", *got, want)
	}
}

// Прокрученное за край скрыто клипом — заявлять его незачем.
func TestScreenRect_ScrolledOutIsDropped(t *testing.T) {
	// Кнопка на 200..230 при сдвиге 50 стоит на экране в 150..180 — ниже
	// видимой части прокрутки (0..100).
	_, far := scrolledScene(t, 50)
	got := captureRects(t)

	far.Invalidate()

	if len(*got) != 0 {
		t.Fatalf("за краем прокрутки заявлено %v, ждали пусто", *got)
	}
}

// SetBounds ребёнка заявляет объединение старой и новой области — тоже в
// экранных координатах.
func TestScreenRect_SetBoundsUnionTranslated(t *testing.T) {
	_, btn := scrolledScene(t, 200)
	got := captureRects(t)

	btn.SetBounds(image.Rect(10, 210, 200, 240))

	want := image.Rect(10, 0, 200, 40) // объединение 200..230 и 210..240, минус 200
	if len(*got) != 1 || (*got)[0] != want {
		t.Fatalf("заявлено %v, ждали [%v]", *got, want)
	}
}

// Новый ребёнок в уже прокрученное содержимое: его область переводится с учётом
// смещения самой прокрутки (она — контейнер, а не только чей-то родитель).
func TestScreenRect_AddChildUsesOwnOffset(t *testing.T) {
	sv, _ := scrolledScene(t, 200)
	second := NewButton("2")
	second.SetBounds(image.Rect(10, 260, 200, 290))
	got := captureRects(t)

	sv.AddChild(second)

	want := image.Rect(10, 60, 200, 90)
	if len(*got) != 1 || (*got)[0] != want {
		t.Fatalf("заявлено %v, ждали [%v]", *got, want)
	}
}

// Вложенные прокрутки складывают смещения.
func TestScreenRect_NestedScrolls(t *testing.T) {
	outer := NewScrollView()
	outer.SetBounds(image.Rect(0, 0, 300, 150))
	outer.ContentHeight = 600
	inner := NewScrollView()
	inner.SetBounds(image.Rect(10, 300, 280, 400))
	inner.ContentHeight = 220
	outer.AddChild(inner)
	btn := NewButton("N")
	btn.SetBounds(image.Rect(20, 450, 200, 480))
	inner.AddChild(btn)
	outer.SetScrollY(300)
	inner.SetScrollY(120)
	got := captureRects(t)

	btn.Invalidate()

	want := image.Rect(20, 30, 200, 60) // 450 - 120 - 300 = 30
	if len(*got) != 1 || (*got)[0] != want {
		t.Fatalf("заявлено %v, ждали [%v]", *got, want)
	}
}

// Удалённый ребёнок больше не числится за прокруткой: его область не
// переводится, как у любого виджета вне неё.
func TestScreenRect_RemovedChildIsReleased(t *testing.T) {
	sv, btn := scrolledScene(t, 200)
	if !sv.RemoveChild(btn) {
		t.Fatalf("RemoveChild не нашёл ребёнка")
	}
	got := captureRects(t)

	btn.Invalidate()

	if want := btn.Bounds(); len(*got) != 1 || (*got)[0] != want {
		t.Fatalf("заявлено %v, ждали [%v]", *got, want)
	}
}

// Прокрутка, созданная литералом структуры, объявляет себя контейнером со
// сдвигом при первом же AddChild.
func TestScreenRect_ZeroValueScrollView(t *testing.T) {
	sv := &ScrollView{scrollbarWidth: 10}
	sv.SetBounds(image.Rect(0, 0, 300, 100))
	sv.ContentHeight = 400
	btn := NewButton("B")
	btn.SetBounds(image.Rect(10, 200, 200, 230))
	sv.AddChild(btn)
	sv.SetScrollY(200)
	got := captureRects(t)

	btn.Invalidate()

	if want := image.Rect(10, 0, 200, 30); len(*got) != 1 || (*got)[0] != want {
		t.Fatalf("заявлено %v, ждали [%v]", *got, want)
	}
}

func TestContentOffset_MatchesScroll(t *testing.T) {
	sv := NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 200, 100))
	sv.ContentHeight = 400
	sv.ContentWidth = 600
	sv.SetScrollY(120)
	sv.SetScrollX(70)
	if got, want := sv.ContentOffset(), image.Pt(70, 120); got != want {
		t.Fatalf("ContentOffset = %v, ждали %v", got, want)
	}
	var _ ContentOffsetter = sv
}

// Границы для скринридера — экранные, и у вложенных тоже.
func TestAccessTree_BoundsAreScreen(t *testing.T) {
	sv, btn := scrolledScene(t, 200)
	tree := BuildAccessTree(sv, nil)
	var find func(n *AccessNode) *AccessNode
	find = func(n *AccessNode) *AccessNode {
		if n.Widget == Widget(btn) {
			return n
		}
		for _, c := range n.Children {
			if r := find(c); r != nil {
				return r
			}
		}
		return nil
	}
	n := find(tree)
	if n == nil {
		t.Fatalf("кнопки нет в дереве")
	}
	if want := image.Rect(10, 0, 200, 30); n.Bounds != want {
		t.Fatalf("границы %v, ждали %v", n.Bounds, want)
	}
	// Сама прокрутка описана своими (экранными) границами.
	if tree.Bounds != sv.Bounds() {
		t.Fatalf("границы прокрутки %v, ждали %v", tree.Bounds, sv.Bounds())
	}
}

// Меню прижимается к краям холста В КАДРЕ виджета: у поля, прокрученного далеко
// вниз, координаты содержимого заведомо больше холста, и без поправки меню
// улетало бы за экран вместо того, чтобы открыться у курсора.
func TestPopupMenuShow_ClampsInEventFrame(t *testing.T) {
	SetScreenBounds(300, 300)
	t.Cleanup(func() { SetScreenBounds(0, 0) })

	open := func(frame image.Point, x, y int) image.Point {
		m := NewPopupMenu()
		m.SetItems([]MenuItem{{Text: "a"}, {Text: "b"}, {Text: "c"}})
		prev := SetEventFrame(frame)
		m.Show(x, y)
		SetEventFrame(prev)
		defer m.Close()
		return m.OverlayBounds().Min
	}

	// Без кадра — прежнее поведение: прижимается к краю холста.
	if got := open(image.Point{}, 100, 5000); got.Y >= 300 {
		t.Fatalf("без кадра меню вне холста: %v", got)
	}
	// С кадром (0,4900) точка (100,5000) — это экранные (100,100): меню у курсора.
	if got, want := open(image.Pt(0, 4900), 100, 5000), image.Pt(100, 5000); got != want {
		t.Fatalf("в кадре меню открыто в %v, ждали %v", got, want)
	}
	// У нижнего края экрана внутри кадра всё равно прижимается — к экрану.
	got := open(image.Pt(0, 4900), 100, 5190) // экранная y=290
	if got.Y >= 4900+300 || got.Y < 4900 {
		t.Fatalf("меню не прижато к краю экрана в кадре: %v", got)
	}
	if prev := SetEventFrame(image.Point{}); prev != (image.Point{}) {
		t.Fatalf("кадр события остался взведённым: %v", prev)
	}
}
