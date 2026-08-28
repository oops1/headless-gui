package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Перетаскивание контура заявляет полосы, а не их объединение.
//
// Заказчик измерил: контур экономил площадь отрисовки, но не трафик — за
// кадр перетаскивания уходило ~71 КБ там, где рамка вокруг окна 720×440 это
// несколько килобайт. Причина была в одной строке: две рамки, разошедшиеся
// на пять точек, объединяются в прямоугольник во всё окно, и потребитель
// перерисовывал эту площадь на каждый шаг мыши.

// collectDamage запоминает области, заявленные во время f.
func collectDamage(f func()) []image.Rectangle {
	var got []image.Rectangle
	h := widget.RegisterUINotifier(nil, func(r image.Rectangle) {
		got = append(got, r)
	})
	defer widget.UnregisterUINotifier(h)
	f()
	return got
}

// dragOutlineOneStep — окно в режиме контура, сдвинутое мышью на шаг.
func dragOutlineOneStep(t *testing.T, style widget.OutlineDragStyle) []image.Rectangle {
	t.Helper()

	win := widget.NewWindow("Окно", 720, 440)
	win.OutlineDrag = true
	win.OutlineDragStyle = style
	win.SetBounds(image.Rect(100, 100, 820, 540)) // 720×440, как в замерах

	// Нажатие на заголовке начинает перетаскивание контуром.
	win.OnMouseButton(widget.MouseEvent{X: 400, Y: 110, Button: widget.MouseLeft, Pressed: true})

	return collectDamage(func() { win.OnMouseMove(405, 110) })
}

func TestOutlineDrag_ClaimsStripsNotUnion(t *testing.T) {
	rects := dragOutlineOneStep(t, widget.OutlineDragBorder)
	if len(rects) == 0 {
		t.Fatal("шаг перетаскивания не заявил ни одной области — контур не двигался")
	}

	// Площадь заявленного против площади объединения: полосы вокруг окна
	// 720×440 занимают около четырёх процентов его площади.
	var claimed int
	union := image.Rectangle{}
	for _, r := range rects {
		claimed += r.Dx() * r.Dy()
		union = union.Union(r)
	}
	full := union.Dx() * union.Dy()
	if full == 0 {
		t.Fatal("заявленные области вырождены")
	}
	if claimed > full/4 {
		t.Errorf("заявлено %d точек из %d в объединении — это объединение, а не полосы",
			claimed, full)
	}
	if len(rects) < 8 {
		t.Errorf("областей %d, ждали не меньше восьми: по четыре полосы на старый и новый контур",
			len(rects))
	}
}

// Залитый контур — другое дело: под заливкой меняется каждый пиксель, и
// дробить её на полосы значило бы соврать потребителю.
func TestOutlineDrag_FilledClaimsWholeRect(t *testing.T) {
	rects := dragOutlineOneStep(t, widget.OutlineDragFilled)
	if len(rects) == 0 {
		t.Fatal("шаг перетаскивания не заявил ни одной области")
	}
	for _, r := range rects {
		if r.Dx() < 100 || r.Dy() < 100 {
			t.Errorf("залитый контур заявил полосу %v — под заливкой меняется всё", r)
		}
	}
}
