package widget

import (
	"image"
	"image/color"
	"testing"
)

// Механика перекрытия: что копится, что пропускается и что не пропускается
// никогда.

func opaquePanelAt(r image.Rectangle) *Panel {
	p := NewPanel(color.RGBA{R: 40, G: 120, B: 220, A: 255})
	p.ShowHeader = false
	p.SetBounds(r)
	return p
}

// Виджет с открытым оверлеем не пропускается, даже если сам он закрыт целиком.
//
// Оверлей — раскрытый список, меню — рисуется ОТДЕЛЬНЫМ проходом, и его
// площадь в границы виджета не входит. Пропустить такой виджет значит
// потерять открытое меню: пользователь щёлкнул, а список не появился.
func TestOccluders_OverlayIsNeverSkipped(t *testing.T) {
	cover := opaquePanelAt(image.Rect(0, 0, 400, 300))

	var occ occluders
	occ.add(cover)
	if occ.n != 1 {
		t.Fatalf("непрозрачная панель объявила %d областей", occ.n)
	}

	dd := NewDropdown("раз", "два", "три")
	dd.SetBounds(image.Rect(60, 60, 200, 92))

	// Закрытый список — обычный виджет, его пропустить можно.
	if !occ.Occluded(dd) {
		t.Error("закрытый список под непрозрачной панелью не пропущен — лишняя отрисовка")
	}

	// Раскрытый — нельзя ни в коем случае.
	dd.SetOpen(true)
	if occ.Occluded(dd) {
		t.Error("список с раскрытым оверлеем пропущен — меню не нарисуется")
	}
}

// Виджет с пустым поддеревом не пропускается: пустой прямоугольник означает
// «не знаю, где это», а не «нигде».
func TestOccluders_EmptySubtreeIsNotSkipped(t *testing.T) {
	var occ occluders
	occ.add(opaquePanelAt(image.Rect(0, 0, 400, 300)))

	empty := NewPanel(color.RGBA{A: 255})
	empty.ShowHeader = false
	// Границы не заданы — поддерево пустое.

	if occ.Occluded(empty) {
		t.Error("виджет без границ пропущен — а движок не знает, где он рисует")
	}
}

// Вложенность проверяется в ОДИН прямоугольник, а не в объединение.
//
// Поддерево, закрытое двумя панелями вскладчину, будет нарисовано зря — и это
// сознательный выбор: хитрая форма объединения даёт куда больше поводов
// ошибиться, а ошибка здесь оставляет дыру на экране.
func TestOccluders_TwoHalvesDoNotAddUp(t *testing.T) {
	var occ occluders
	occ.add(opaquePanelAt(image.Rect(0, 0, 200, 300)))
	occ.add(opaquePanelAt(image.Rect(200, 0, 400, 300)))

	across := NewPanel(color.RGBA{R: 200, A: 255})
	across.ShowHeader = false
	across.SetBounds(image.Rect(150, 100, 250, 150)) // лежит на обеих

	if occ.Occluded(across) {
		t.Error("виджет засчитан закрытым по объединению двух областей")
	}

	// А внутри одной — пропускается.
	inside := NewPanel(color.RGBA{R: 200, A: 255})
	inside.ShowHeader = false
	inside.SetBounds(image.Rect(20, 100, 120, 150))
	if !occ.Occluded(inside) {
		t.Error("виджет внутри одной области не пропущен")
	}
}

// Список закрывающих областей ограничен и от переполнения не портится.
func TestOccluders_ListIsBounded(t *testing.T) {
	var occ occluders
	for i := 0; i < 4*maxOccluders; i++ {
		occ.add(opaquePanelAt(image.Rect(i, 0, i+50, 50)))
	}
	if occ.n > maxOccluders {
		t.Errorf("накоплено %d областей при пределе %d", occ.n, maxOccluders)
	}

	// Переполнение теряет экономию, но не правильность: то, что попало в
	// список, по-прежнему закрывает.
	inside := NewPanel(color.RGBA{R: 200, A: 255})
	inside.ShowHeader = false
	inside.SetBounds(image.Rect(5, 5, 40, 40))
	if !occ.Occluded(inside) {
		t.Error("после переполнения перестала работать даже первая область")
	}
}

// Скруглённая заливка не обещает своих углов, прямая обещает всё.
func TestOpaqueRect_RoundedLosesOnlyTheCorners(t *testing.T) {
	b := image.Rect(10, 20, 210, 170)

	if got := opaqueRect(b, 0); got != b {
		t.Errorf("без скругления объявлено %v вместо %v", got, b)
	}

	got := opaqueRect(b, 12)
	if want := image.Rect(10, 32, 210, 158); got != want {
		t.Errorf("со скруглением 12 объявлено %v, ждали %v", got, want)
	}

	// Скругление больше половины высоты не оставляет ничего — и это верно:
	// такая фигура почти круг, ручаться за прямоугольник в ней нельзя.
	if got := opaqueRect(image.Rect(0, 0, 100, 20), 15); !got.Empty() {
		t.Errorf("при скруглении больше половины высоты объявлено %v", got)
	}
	if got := opaqueRect(image.Rectangle{}, 0); !got.Empty() {
		t.Errorf("пустая область объявила %v", got)
	}
}

// Узкая фигура со скруглением не объявляет ничего.
//
// image.Rect при перевёрнутых координатах не отдаёт пустой прямоугольник, а
// переставляет их местами. Полоса высотой 20 со скруглением 15 давала из-за
// этого не «ничего не закрываю», а бодрое «закрываю вот эту чужую полосу» —
// и под ней осталась бы дыра.
func TestOpaqueRect_TooRoundToPromiseAnything(t *testing.T) {
	for _, tc := range []struct {
		name string
		b    image.Rectangle
		cr   int
	}{
		{"скругление во всю высоту", image.Rect(0, 0, 100, 20), 15},
		{"скругление ровно в половину высоты", image.Rect(0, 0, 100, 20), 10},
		{"скругление шире фигуры", image.Rect(0, 0, 20, 100), 15},
	} {
		if got := opaqueRect(tc.b, tc.cr); !got.Empty() {
			t.Errorf("%s: объявлено %v, ждали пустоту", tc.name, got)
		}
	}
}
