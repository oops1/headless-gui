package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Окно и диалог без штатной полосы заголовка — запрос GG-51.
//
// Диалог всегда рисовал свою полосу во всю ширину, значит верхняя полоса
// принадлежала движку, а любая боковая панель начиналась только под ней —
// левый верхний угол приложению был недоступен. Панель навигации во всю левую
// сторону с заголовком внутри неё воспроизвести было нельзя.

// Без полосы содержимое начинается от верха диалога.
func TestChromeless_ContentStartsAtTop(t *testing.T) {
	d := widget.NewDialog("Настройки", 400, 300)
	withBar := d.ContentBounds()

	d.SetChromeless(true)
	without := d.ContentBounds()

	if without.Min.Y >= withBar.Min.Y {
		t.Errorf("без полосы содержимое начинается на %d, с полосой — на %d",
			without.Min.Y, withBar.Min.Y)
	}
	if got := without.Min.Y - d.Bounds().Min.Y; got > 14 {
		t.Errorf("содержимое отступает от верха на %d — полоса всё ещё занимает место", got)
	}
}

// Кнопка ✕ убирается вместе с полосой: рисовать её поверх чужой шапки движок
// не вправе.
func TestChromeless_HidesCloseButton(t *testing.T) {
	d := widget.NewDialog("Настройки", 400, 300)
	if !d.ShowCloseButton {
		t.Fatal("у обычного диалога нет ✕ — проверять нечего")
	}

	d.SetChromeless(true)
	if d.ShowCloseButton {
		t.Error("✕ остался при выключенной полосе заголовка")
	}

	d.SetChromeless(false)
	if !d.ShowCloseButton {
		t.Error("✕ не вернулся вместе с полосой")
	}
}

// Полоса заголовка перестаёт рисоваться.
func TestChromeless_TitleBarIsGone(t *testing.T) {
	withBar := widget.NewDialog("Заголовок", 300, 200)
	a := widgetScene(t, withBar, image.Rect(40, 20, 340, 110))

	without := widget.NewDialog("Заголовок", 300, 200)
	without.SetChromeless(true)
	b := widgetScene(t, without, image.Rect(40, 20, 340, 110))

	same := true
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("диалог без полосы нарисован так же, как с полосой")
	}
}

// Объявленная область тащит диалог так же, как раньше полоса заголовка.
func TestChromeless_DragAreaMovesDialog(t *testing.T) {
	d := widget.NewDialog("Настройки", 400, 300)
	d.SetChromeless(true)
	d.SetBounds(image.Rect(100, 100, 500, 400))
	d.AddDragArea(image.Rect(0, 0, 240, 48)) // шапка боковой панели

	before := d.Bounds()
	inside := image.Pt(before.Min.X+60, before.Min.Y+20)
	d.OnMouseButton(widget.MouseEvent{X: inside.X, Y: inside.Y, Button: widget.MouseLeft, Pressed: true})
	d.OnMouseMove(inside.X+70, inside.Y+50)
	d.OnMouseButton(widget.MouseEvent{X: inside.X + 70, Y: inside.Y + 50, Button: widget.MouseLeft, Pressed: false})

	after := d.Bounds()
	if after.Min.X != before.Min.X+70 || after.Min.Y != before.Min.Y+50 {
		t.Errorf("диалог не сдвинулся за объявленную область: %v → %v", before, after)
	}
}

// Вне объявленной области диалог не тащится: остальное содержимое остаётся
// содержимым.
func TestChromeless_OutsideDragAreaDoesNotMove(t *testing.T) {
	d := widget.NewDialog("Настройки", 400, 300)
	d.SetChromeless(true)
	d.SetBounds(image.Rect(100, 100, 500, 400))
	d.AddDragArea(image.Rect(0, 0, 240, 48))

	before := d.Bounds()
	// Ниже объявленной области.
	pt := image.Pt(before.Min.X+60, before.Min.Y+200)
	d.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true})
	d.OnMouseMove(pt.X+70, pt.Y+50)
	d.OnMouseButton(widget.MouseEvent{X: pt.X + 70, Y: pt.Y + 50, Button: widget.MouseLeft, Pressed: false})

	if d.Bounds() != before {
		t.Errorf("диалог сдвинулся из-за нажатия вне области: %v", d.Bounds())
	}
}

// Объявленная область сильнее лежащих в ней виджетов: шапку целиком составляют
// виджеты приложения, и проверка «под курсором чей-то ребёнок» отменяла бы
// перетаскивание всегда.
func TestChromeless_DragAreaBeatsChildren(t *testing.T) {
	d := widget.NewDialog("Настройки", 400, 300)
	d.SetChromeless(true)
	d.SetBounds(image.Rect(0, 0, 400, 300))

	header := widget.NewPanel(widget.Theme{}.PanelBG)
	header.ShowHeader = false
	d.SetContent(header) // содержимое накрывает весь диалог
	d.AddDragArea(image.Rect(0, 0, 240, 48))

	before := d.Bounds()
	d.OnMouseButton(widget.MouseEvent{X: 60, Y: 20, Button: widget.MouseLeft, Pressed: true})
	d.OnMouseMove(90, 40)
	d.OnMouseButton(widget.MouseEvent{X: 90, Y: 40, Button: widget.MouseLeft, Pressed: false})

	if d.Bounds() == before {
		t.Error("виджет содержимого отменил перетаскивание за объявленную область")
	}
}

// Borderless-окно: своей полосы нет, и без объявленной области его было не
// сдвинуть — WantsCapture отвечал «нет» безусловно.
func TestChromeless_BorderlessWindowDrag(t *testing.T) {
	w := widget.NewWindow("Настройки", 400, 300)
	w.Style = widget.WindowStyleNone
	w.SetBounds(image.Rect(0, 0, 400, 300))

	press := widget.MouseEvent{X: 60, Y: 20, Button: widget.MouseLeft, Pressed: true}
	if w.WantsCapture(press) {
		t.Fatal("borderless-окно просит захват без объявленной области")
	}

	w.AddDragArea(image.Rect(0, 0, 240, 48))
	if !w.WantsCapture(press) {
		t.Error("объявленная область не даёт окну захватить мышь для перетаскивания")
	}

	w.ClearDragAreas()
	if w.WantsCapture(press) {
		t.Error("после ClearDragAreas окно всё ещё просит захват")
	}
}
