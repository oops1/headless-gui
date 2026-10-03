// Прокрутка полосы вкладок заголовка и перестановка вкладок
// перетаскиванием.
//
// Вкладки сжимались до предела, а дальше просто не рисовались: при десятке
// открытых документов часть становилась невидимой И недостижимой — ни
// щелчком до неё дотянуться, ни узнать, что она есть. Переставить вкладки
// местами было нечем вовсе.
package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// manyTabsWindow — окно, в заголовок которого вкладки заведомо не влезают.
func manyTabsWindow(t *testing.T, width, n int) *widget.Window {
	t.Helper()
	w := widget.NewWindow("term", width, 300)
	w.ShowLocaleIndicator = false
	w.EnableTitleTabs()
	for i := 0; i < n; i++ {
		p := widget.NewPanel(widget.DarkTheme().PanelBG)
		p.ShowHeader = false
		w.AddTitleTab(tabName(i), p)
	}
	w.Draw(&recCtx{})
	return w
}

func tabName(i int) string {
	names := []string{"первый", "второй", "третий", "четвёртый", "пятый",
		"шестой", "седьмой", "восьмой", "девятый", "десятый"}
	return names[i%len(names)]
}

// headers — подписи вкладок окна в нынешнем порядке.
func headers(w *widget.Window) []string {
	out := make([]string, w.TitleTabCount())
	for i := range out {
		out[i] = w.TitleTabHeader(i)
	}
	return out
}

func TestTitleTabs_MoveReorders(t *testing.T) {
	w := manyTabsWindow(t, 800, 4)
	w.SetActiveTitleTab(1)

	var movedFrom, movedTo = -1, -1
	w.OnTitleTabMoved = func(from, to int) { movedFrom, movedTo = from, to }

	if !w.MoveTitleTab(0, 2) {
		t.Fatal("перестановка отклонена")
	}
	want := []string{"второй", "третий", "первый", "четвёртый"}
	for i, h := range headers(w) {
		if h != want[i] {
			t.Fatalf("порядок %v, ждал %v", headers(w), want)
		}
	}
	// Активной остаётся та же вкладка, её номер сдвинулся вслед за ней.
	if got := w.TitleTabHeader(w.ActiveTitleTab()); got != "второй" {
		t.Errorf("активна %q, ждал «второй»", got)
	}
	if movedFrom != 0 || movedTo != 2 {
		t.Errorf("о перестановке сообщили как %d→%d", movedFrom, movedTo)
	}

	// Бессмысленные перестановки отклоняются молча.
	if w.MoveTitleTab(1, 1) || w.MoveTitleTab(-1, 0) || w.MoveTitleTab(0, 99) {
		t.Error("принята неверная перестановка")
	}
}

// Перетаскивание меняет вкладки местами на лету: соседи расступаются до
// того, как кнопку отпустили.
func TestTitleTabs_DragReorders(t *testing.T) {
	w := manyTabsWindow(t, 800, 4)
	first := w.TitleTabRect(0)
	second := w.TitleTabRect(1)
	if first.Empty() || second.Empty() {
		t.Fatal("вкладки не размечены")
	}
	y := (first.Min.Y + first.Max.Y) / 2

	// Нажали на первой и повели вправо, за середину второй.
	w.OnMouseButton(widget.MouseEvent{X: first.Min.X + 10, Y: y, Button: widget.MouseLeft, Pressed: true})
	w.OnMouseMove(second.Min.X+second.Dx()/2+5, y)

	if got := headers(w)[0]; got != "второй" {
		t.Errorf("порядок %v — перетаскивание не переставило вкладки", headers(w))
	}
	// Отпускание кнопки израсходовано перетаскиванием.
	if !w.OnMouseButton(widget.MouseEvent{X: second.Max.X, Y: y, Button: widget.MouseLeft}) {
		t.Error("отпускание после перетаскивания не обработано")
	}
}

// Короткое движение — не перетаскивание: дрожание руки при щелчке не должно
// менять порядок вкладок.
func TestTitleTabs_DragThreshold(t *testing.T) {
	w := manyTabsWindow(t, 800, 3)
	before := headers(w)
	r := w.TitleTabRect(0)
	y := (r.Min.Y + r.Max.Y) / 2

	w.OnMouseButton(widget.MouseEvent{X: r.Min.X + 10, Y: y, Button: widget.MouseLeft, Pressed: true})
	w.OnMouseMove(r.Min.X+12, y)
	w.OnMouseButton(widget.MouseEvent{X: r.Min.X + 12, Y: y, Button: widget.MouseLeft})

	for i, h := range headers(w) {
		if h != before[i] {
			t.Fatalf("порядок изменился от дрожания: %v → %v", before, headers(w))
		}
	}
}

// Когда вкладки не помещаются, полоса прокручивается: раньше лишние просто
// не рисовались и становились недостижимыми.
func TestTitleTabs_ScrollsWhenOverflowing(t *testing.T) {
	w := manyTabsWindow(t, 420, 10)

	last := w.TitleTabCount() - 1
	if !w.TitleTabRect(last).Empty() {
		t.Skip("вкладки уместились — прокручивать нечего")
	}

	// Листаем полосу вправо: последняя вкладка обязана показаться.
	for i := 0; i < 40 && w.TitleTabRect(last).Empty(); i++ {
		w.ScrollTitleTabs(60)
		w.Draw(&recCtx{})
	}
	r := w.TitleTabRect(last)
	if r.Empty() {
		t.Fatal("последняя вкладка недостижима даже после прокрутки")
	}
	// И она действительно под курсором, а не просто «размечена».
	part, idx := w.TitleTabHitTest((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	if part != widget.TitleTabPartTab || idx != last {
		t.Errorf("хит-тест последней вкладки = (%v, %d)", part, idx)
	}
}

// Выбор вкладки показывает её целиком: её могли выбрать с клавиатуры, когда
// она уехала за край.
func TestTitleTabs_ScrollsToActive(t *testing.T) {
	w := manyTabsWindow(t, 420, 10)
	last := w.TitleTabCount() - 1
	if !w.TitleTabRect(last).Empty() {
		t.Skip("вкладки уместились")
	}

	w.SetActiveTitleTab(last)
	w.Draw(&recCtx{})

	r := w.TitleTabRect(last)
	if r.Empty() {
		t.Fatal("активная вкладка осталась за краем полосы")
	}
}

// Колесо над заголовком листает вкладки, над содержимым — нет: там оно
// принадлежит содержимому.
func TestTitleTabs_WheelScrollsOnlyTitleBar(t *testing.T) {
	w := manyTabsWindow(t, 420, 10)
	if w.TitleTabRect(w.TitleTabCount()-1).Empty() == false {
		t.Skip("вкладки уместились")
	}
	r := w.TitleTabRect(0)
	y := (r.Min.Y + r.Max.Y) / 2

	if !w.OnMouseWheelPixels(r.Min.X+5, y, 0, -40) {
		t.Error("колесо над полосой вкладок не прокрутило её")
	}
	content := w.ContentBounds()
	if w.OnMouseWheelPixels(content.Min.X+10, content.Min.Y+10, 0, -40) {
		t.Error("колесо над содержимым перехвачено заголовком")
	}
}

// Прокрутка к вкладке — чистая арифметика: видимую не трогаем, иначе полоса
// дёргалась бы на каждом кадре.
func TestTitleTabs_ScrollMath(t *testing.T) {
	w := manyTabsWindow(t, 420, 6)
	// Косвенная проверка через поведение: двойной вызов ScrollTitleTabs не
	// уводит полосу дальше предела.
	for i := 0; i < 50; i++ {
		w.ScrollTitleTabs(100)
	}
	w.Draw(&recCtx{})
	first := w.TitleTabRect(0)
	lastIdx := w.TitleTabCount() - 1
	lastR := w.TitleTabRect(lastIdx)
	if lastR.Empty() {
		t.Fatal("после прокрутки до упора последняя вкладка не видна")
	}
	if !first.Empty() && first.Min.X > lastR.Min.X {
		t.Errorf("порядок вкладок нарушен прокруткой: первая %v, последняя %v", first, lastR)
	}
}
