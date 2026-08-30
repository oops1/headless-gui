package desktop

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Две панели, стоящие одна над другой.
//
// Windows показывает уведомления НАД календарём. Для каждой панели площадь
// соседки — «мимо», и клик по числу календаря считался для центра уведомлений
// поводом закрыться; закрываясь, он ещё и съедал событие, и календарь не
// видел ни чисел, ни стрелок месяца. Оболочке пришлось разносить события по
// панелям вручную.

// stackedPanels — календарь и над ним вторая панель, обе в дереве движка.
func stackedPanels(t *testing.T) (*engine.Engine, *CalendarFlyout, *Flyout) {
	t.Helper()

	m := calendarHoverTheme(t)

	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 800, 600))

	cal := NewCalendarFlyout(m, NewFakeClock(time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)))
	cal.Screen = image.Rect(0, 0, 800, 600)
	root.AddChild(cal)

	// Вторая панель — над календарём, как центр уведомлений в Windows.
	above := NewFlyout(m, ComponentCalendar)
	above.Screen = image.Rect(0, 0, 800, 600)
	above.Size = func() image.Point { return image.Pt(148, 120) }
	above.Content = func(widget.DrawContext, image.Rectangle) {}
	root.AddChild(above)

	eng := engine.New(800, 600, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.RenderOnce()

	anchor := image.Rect(700, 560, 740, 600)
	cal.Open(anchor)
	above.Anchor = anchor
	// Отступ считаем от УЖЕ открытого календаря: тема могла добавить свой
	// зазор, и «высота календаря» сама по себе оставила бы панели внахлёст.
	above.Margin = anchor.Min.Y - cal.Bounds().Min.Y
	above.Open(anchor)

	if cal.Bounds().Empty() || above.Bounds().Empty() {
		t.Fatalf("панели без геометрии: календарь %v, верхняя %v", cal.Bounds(), above.Bounds())
	}
	if cal.Bounds().Overlaps(above.Bounds()) {
		t.Fatalf("панели наложились: %v и %v", cal.Bounds(), above.Bounds())
	}
	return eng, cal, above
}

// dayPoint — середина любой ячейки своего месяца.
func dayPoint(t *testing.T, c *CalendarFlyout) image.Point {
	t.Helper()
	cell := anyDayCell(t, c)
	return cell.rect.Min.Add(image.Pt(cell.rect.Dx()/2, cell.rect.Dy()/2))
}

// Без группы соседка закрывается — это по-прежнему верно: две несвязанные
// панели друг другу никто.
func TestFlyoutGroup_WithoutGroupNeighbourStillCloses(t *testing.T) {
	eng, cal, above := stackedPanels(t)

	pt := dayPoint(t, cal)
	eng.SendMouseButton(pt.X, pt.Y, widget.MouseLeft, true)

	if above.IsOpen() {
		t.Error("панель вне группы осталась открытой при клике мимо неё")
	}
	// Но клик всё равно ДОШЁЛ до календаря: съедать его было нельзя.
	if cal.Selected().IsZero() {
		t.Error("клик по числу не дошёл до календаря — его съела соседняя панель")
	}
}

// В группе клик по соседке не закрывает панель и не пропадает.
func TestFlyoutGroup_ClickInNeighbourKeepsBothOpen(t *testing.T) {
	eng, cal, above := stackedPanels(t)
	NewFlyoutGroup(cal.Flyout, above)

	pt := dayPoint(t, cal)
	eng.SendMouseButton(pt.X, pt.Y, widget.MouseLeft, true)

	if !above.IsOpen() {
		t.Error("клик по числу календаря закрыл соседнюю панель группы")
	}
	if !cal.IsOpen() {
		t.Error("клик по собственному числу закрыл сам календарь")
	}
	if cal.Selected().IsZero() {
		t.Error("клик по числу не дошёл до календаря")
	}
}

// Клик мимо ВСЕЙ группы закрывает её целиком.
func TestFlyoutGroup_ClickOutsideClosesTheWholeGroup(t *testing.T) {
	eng, cal, above := stackedPanels(t)
	NewFlyoutGroup(cal.Flyout, above)

	// Точка заведомо вне обеих панелей.
	outside := image.Pt(20, 20)
	if outside.In(cal.Bounds()) || outside.In(above.Bounds()) {
		t.Fatal("точка для клика мимо оказалась внутри панели")
	}
	eng.SendMouseButton(outside.X, outside.Y, widget.MouseLeft, true)

	if cal.IsOpen() || above.IsOpen() {
		t.Errorf("клик мимо группы не закрыл её: календарь=%v, верхняя=%v",
			cal.IsOpen(), above.IsOpen())
	}
}

// Панель, вынутая из группы, снова считает соседку чужой.
func TestFlyoutGroup_LeavingTheGroupRestoresOldBehaviour(t *testing.T) {
	eng, cal, above := stackedPanels(t)
	g := NewFlyoutGroup(cal.Flyout, above)

	above.SetGroup(nil)
	if g.covers(dayPoint(t, cal)) && above.Group() != nil {
		t.Fatal("панель осталась в группе после SetGroup(nil)")
	}

	pt := dayPoint(t, cal)
	eng.SendMouseButton(pt.X, pt.Y, widget.MouseLeft, true)
	if above.IsOpen() {
		t.Error("вынутая из группы панель не закрылась при клике мимо неё")
	}
}

// CloseAll закрывает всю группу разом.
func TestFlyoutGroup_CloseAll(t *testing.T) {
	_, cal, above := stackedPanels(t)
	g := NewFlyoutGroup(cal.Flyout, above)

	g.CloseAll()
	if cal.IsOpen() || above.IsOpen() {
		t.Errorf("CloseAll оставил панели открытыми: календарь=%v, верхняя=%v",
			cal.IsOpen(), above.IsOpen())
	}
}

// Нулевая группа безопасна: панель без группы не требует проверок на месте
// вызова.
func TestFlyoutGroup_NilGroupIsSafe(t *testing.T) {
	var g *FlyoutGroup
	g.CloseAll()
	g.OpenAll(image.Rect(0, 0, 10, 10))
	if g.covers(image.Pt(1, 1)) {
		t.Error("нулевая группа объявила точку своей")
	}
}

// Панель получает события от движка вообще: до этой правки Bounds объявляли
// только меню «Пуск» и быстрые настройки, а календарь и центр уведомлений —
// нет, и движок их попросту не видел.
func TestFlyout_IsReachableByTheEngine(t *testing.T) {
	m := calendarHoverTheme(t)
	for _, tc := range []struct {
		name string
		make func() *Flyout
	}{
		{"общая основа", func() *Flyout {
			f := NewFlyout(m, ComponentCalendar)
			f.Size = func() image.Point { return image.Pt(148, 120) }
			f.Content = func(widget.DrawContext, image.Rectangle) {}
			return f
		}},
		{"календарь", func() *Flyout {
			c := NewCalendarFlyout(m, NewFakeClock(time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)))
			return c.Flyout
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.make()
			f.Screen = image.Rect(0, 0, 800, 600)
			f.Open(image.Rect(700, 560, 740, 600))
			if f.Bounds().Empty() {
				t.Fatal("у открытой панели пустые границы — движок её не найдёт")
			}
			if !f.Bounds().Overlaps(f.OverlayBounds()) {
				t.Errorf("границы %v не совпадают с областью оверлея %v",
					f.Bounds(), f.OverlayBounds())
			}
			f.Close()
			if !f.Bounds().Empty() {
				t.Errorf("закрытая панель занимает %v", f.Bounds())
			}
		})
	}
}
