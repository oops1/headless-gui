package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Измерение содержимого — запрос GG-47.
//
// Ни StackPanel, ни ScrollView не считали свой размер по детям: высоту
// прокрутки выставляло приложение, а панель раскладывала по уже заданным
// размерам. Больнее всего это било по сворачиваемой группе внутри прокрутки:
// каждое раскрытие меняло высоту содержимого, и её пересчитывали руками.

// sizedPanel — виджет заданной высоты (роль «блока настроек»).
func sizedPanel(h int) *widget.Panel {
	p := widget.NewPanel(widget.Theme{}.PanelBG)
	p.ShowHeader = false
	p.SetBounds(image.Rect(0, 0, 100, h))
	return p
}

// Панель складывает высоты детей, промежутки и отступы.
func TestStackPanel_DesiredSize(t *testing.T) {
	sp := widget.NewStackPanel(widget.OrientationVertical)
	sp.Spacing = 4
	sp.Padding = 6
	sp.SetBounds(image.Rect(0, 0, 200, 500))
	sp.AddChild(sizedPanel(30))
	sp.AddChild(sizedPanel(50))

	_, h := sp.DesiredSize()
	// 30 + 4 + 50 + 6*2 = 96
	if h != 96 {
		t.Errorf("панель просит %d по высоте, ждали 96", h)
	}
}

// Скрытый ребёнок не занимает ни своего места, ни промежутка после себя.
func TestStackPanel_DesiredSizeSkipsHidden(t *testing.T) {
	sp := widget.NewStackPanel(widget.OrientationVertical)
	sp.Spacing = 4
	sp.SetBounds(image.Rect(0, 0, 200, 500))
	a, b := sizedPanel(30), sizedPanel(50)
	sp.AddChild(a)
	sp.AddChild(b)

	_, full := sp.DesiredSize()
	b.SetVisible(false)
	_, hidden := sp.DesiredSize()

	if hidden != 30 {
		t.Errorf("со скрытым вторым панель просит %d, ждали 30", hidden)
	}
	if full <= hidden {
		t.Errorf("скрытие ничего не изменило: было %d, стало %d", full, hidden)
	}
}

// Свёрнутая раскрывашка просит высоту одного заголовка.
func TestExpander_DesiredSizeFollowsState(t *testing.T) {
	ex := widget.NewExpander("Дополнительно")
	ex.HeaderHeight = 30
	ex.SetBounds(image.Rect(0, 0, 200, 200))
	ex.AddChild(sizedPanel(120))

	_, collapsed := ex.DesiredSize()
	if collapsed != 30 {
		t.Errorf("свёрнутая просит %d, ждали высоту заголовка 30", collapsed)
	}

	ex.SetExpanded(true)
	_, expanded := ex.DesiredSize()
	if expanded <= collapsed {
		t.Errorf("развёрнутая просит %d, свёрнутая — %d", expanded, collapsed)
	}
	if expanded < 150 {
		t.Errorf("развёрнутая просит %d — содержимое в 120 точек не учтено", expanded)
	}
}

// Столбик пересчитывается после сворачивания: то, ради чего всё затевалось.
func TestStackPanel_RelayoutAfterCollapse(t *testing.T) {
	sp := widget.NewStackPanel(widget.OrientationVertical)
	sp.SetBounds(image.Rect(0, 0, 200, 500))

	ex := widget.NewExpander("Группа")
	ex.HeaderHeight = 30
	ex.SetBounds(image.Rect(0, 0, 200, 200))
	ex.AddChild(sizedPanel(120))
	ex.SetExpanded(true)
	sp.AddChild(ex)

	after := sizedPanel(40)
	sp.AddChild(after)
	sp.Relayout()
	yExpanded := after.Bounds().Min.Y

	ex.SetExpanded(false)
	sp.Relayout()
	yCollapsed := after.Bounds().Min.Y

	if yCollapsed >= yExpanded {
		t.Errorf("после сворачивания сосед стоит на %d, было %d — пустота осталась",
			yCollapsed, yExpanded)
	}
	if yCollapsed != 30 {
		t.Errorf("сосед встал на %d, ждали сразу под заголовком (30)", yCollapsed)
	}
}

// Прокрутка сама считает высоту содержимого.
func TestScrollView_FitContent(t *testing.T) {
	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 200, 100))

	tall := sizedPanel(50)
	tall.SetBounds(image.Rect(0, 0, 200, 300))
	sv.AddChild(tall)

	sv.FitContent()
	if sv.ContentHeight != 300 {
		t.Errorf("высота содержимого %d, ждали 300", sv.ContentHeight)
	}

	// Содержимое сжалось — прокрутка не должна остаться за его концом.
	sv.SetScrollY(150)
	tall.SetBounds(image.Rect(0, 0, 200, 120))
	sv.FitContent()

	if sv.ContentHeight != 120 {
		t.Errorf("после сжатия высота %d, ждали 120", sv.ContentHeight)
	}
	if got := sv.ScrollY(); got > 20 {
		t.Errorf("прокрутка осталась на %d при содержимом 120 и окне 100", got)
	}
}

// Разделитель сообщает свою толщину — контейнеру не нужно её угадывать.
func TestStackPanel_MeasuresSeparator(t *testing.T) {
	sp := widget.NewStackPanel(widget.OrientationVertical)
	sp.SetBounds(image.Rect(0, 0, 200, 300))
	sp.AddChild(sizedPanel(20))
	sep := widget.NewSeparator()
	sep.Thickness = 2
	sp.AddChild(sep)
	sp.AddChild(sizedPanel(20))
	sp.Relayout()

	_, h := sp.DesiredSize()
	if h != 42 {
		t.Errorf("панель с разделителем просит %d, ждали 20+2+20", h)
	}
}
