package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Изменяемый размер диалога — запрос GG-39.
//
// Размер задавался один раз в NewDialog и больше не менялся: ни Resizable, ни
// MinWidth/MinHeight, а SetBounds диалог не переопределял вовсе — кнопка ✕
// оставалась на прежнем месте, содержимое сохраняло прежний размер. Окно
// настроек, которое обязано тянуться, приходилось делать отдельным окном,
// теряя модальность.

// closeButtonOf — кнопка ✕ диалога (последний ребёнок, добавленный
// конструктором первым; ищем по правому верхнему углу).
func closeButtonOf(d *widget.Dialog) image.Rectangle {
	best := image.Rectangle{}
	for _, c := range d.Children() {
		b := c.Bounds()
		if b.Empty() {
			continue
		}
		if best.Empty() || (b.Max.X >= best.Max.X && b.Min.Y <= best.Min.Y) {
			best = b
		}
	}
	return best
}

// Кнопка ✕ переезжает в новый правый верхний угол.
func TestDialogResize_CloseButtonFollowsWidth(t *testing.T) {
	d := widget.NewDialog("Настройки", 400, 300)
	d.SetResizable(true)
	before := closeButtonOf(d)

	d.Resize(700, 300)
	after := closeButtonOf(d)

	if after.Max.X <= before.Max.X {
		t.Errorf("кнопка ✕ осталась на %v при расширении с 400 до 700 (была %v)", after, before)
	}
	if got := d.Bounds().Max.X - after.Max.X; got != 6 {
		t.Errorf("кнопка ✕ отстоит от правого края на %d, ждали 6", got)
	}
}

// Содержимое растягивается на всю область.
func TestDialogResize_ContentFills(t *testing.T) {
	d := widget.NewDialog("Настройки", 400, 300)
	d.SetResizable(true)

	content := widget.NewStackPanel(widget.OrientationVertical)
	d.SetContent(content)

	if got := content.Bounds(); got != d.ContentBounds() {
		t.Fatalf("содержимое заняло %v, область %v", got, d.ContentBounds())
	}

	d.Resize(700, 500)
	if got := content.Bounds(); got != d.ContentBounds() {
		t.Errorf("после растягивания содержимое заняло %v, область %v", got, d.ContentBounds())
	}
	if content.Bounds().Dx() < 600 {
		t.Errorf("содержимое шириной %d при диалоге 700", content.Bounds().Dx())
	}
}

// Обычные дети сдвигаются, а не растягиваются: диалоги расставляют кнопки
// абсолютными координатами, и растяжение каждого превратило бы MessageBox в
// стопку виджетов друг поверх друга.
func TestDialogResize_PlainChildrenAreNotStretched(t *testing.T) {
	d := widget.NewDialog("Вопрос", 400, 300)
	btn := widget.NewButton("OK")
	btn.SetBounds(image.Rect(300, 250, 380, 280))
	d.AddChild(btn)

	sizeBefore := btn.Bounds().Size()
	d.SetBounds(image.Rect(100, 100, 500, 400)) // перенос без изменения размера

	if got := btn.Bounds().Size(); got != sizeBefore {
		t.Errorf("кнопка изменила размер при переносе: %v → %v", sizeBefore, got)
	}
	if got := btn.Bounds().Min; got.X != 400 || got.Y != 350 {
		t.Errorf("кнопка сдвинулась в %v, ждали (400,350)", got)
	}
}

// Минимум не даёт сжать диалог до неуправляемого состояния.
func TestDialogResize_MinSize(t *testing.T) {
	d := widget.NewDialog("Настройки", 400, 300)
	d.SetResizable(true)

	d.Resize(10, 10)
	if w, h := d.Bounds().Dx(), d.Bounds().Dy(); w < 200 || h < 120 {
		t.Errorf("диалог сжался до %dx%d — умолчательный минимум не сработал", w, h)
	}

	d.SetMinSize(500, 400)
	if w, h := d.Bounds().Dx(), d.Bounds().Dy(); w != 500 || h != 400 {
		t.Errorf("после SetMinSize размер %dx%d, ждали не меньше 500x400", w, h)
	}
	d.Resize(300, 200)
	if w, h := d.Bounds().Dx(), d.Bounds().Dy(); w != 500 || h != 400 {
		t.Errorf("диалог сжат до %dx%d ниже заданного минимума 500x400", w, h)
	}
}

// Растяжимость по умолчанию выключена: диалог с вопросом тянуть незачем.
func TestDialogResize_OffByDefault(t *testing.T) {
	d := widget.NewDialog("Вопрос", 400, 300)
	if d.IsResizable() {
		t.Error("новый диалог оказался растяжимым по умолчанию")
	}
}

// Смена содержимого убирает прежнее из дерева — иначе старое осталось бы
// висеть под новым, принимая щелчки.
func TestDialogResize_SetContentReplaces(t *testing.T) {
	d := widget.NewDialog("Настройки", 400, 300)
	first := widget.NewStackPanel(widget.OrientationVertical)
	second := widget.NewStackPanel(widget.OrientationVertical)

	d.SetContent(first)
	d.SetContent(second)

	for _, c := range d.Children() {
		if c == widget.Widget(first) {
			t.Error("прежнее содержимое осталось в дереве диалога")
		}
	}
	if d.Content() != widget.Widget(second) {
		t.Error("новое содержимое не запомнено")
	}
}
