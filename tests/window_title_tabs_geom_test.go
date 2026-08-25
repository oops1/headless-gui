// Тесты экспортированной геометрии полосы вкладок заголовка и переключения
// меню шеврона (ENGINE_ISSUES winline, v3.13.4).
package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// newTabbedWindow — окно с тремя вкладками, «+» и меню шеврона; геометрия
// посчитана одним проходом Draw (recCtx меряет текст как рун×7).
func newTabbedWindow(t *testing.T, width int) (*widget.Window, *widget.PopupMenu) {
	t.Helper()
	w := widget.NewWindow("term", width, 300)
	w.ShowLocaleIndicator = false
	w.EnableTitleTabs()
	for _, name := range []string{"PowerShell", "cmd", "bash"} {
		p := widget.NewPanel(widget.DarkTheme().PanelBG)
		p.ShowHeader = false
		w.AddTitleTab(name, p)
	}
	menu := widget.NewPopupMenu()
	menu.AddItem("Windows PowerShell", func() {})
	menu.AddItem("Ubuntu", func() {})
	w.SetTitleTabsMenu(menu)
	w.OnTitleTabNew = func() {}
	w.Draw(&recCtx{})
	return w, menu
}

// TestTitleTabs_ExportedGeometry — прямоугольники вкладок, «×», «+» и «v»
// видны снаружи и согласованы с хит-тестом.
func TestTitleTabs_ExportedGeometry(t *testing.T) {
	w, _ := newTabbedWindow(t, 800)

	prev := image.Rectangle{}
	for i := 0; i < w.TitleTabCount(); i++ {
		r := w.TitleTabRect(i)
		if r.Empty() {
			t.Fatalf("вкладка %d: пустой прямоугольник", i)
		}
		if !prev.Empty() && r.Min.X < prev.Max.X {
			t.Errorf("вкладка %d начинается левее конца предыдущей: %v после %v", i, r, prev)
		}
		prev = r

		// Хит-тест центра вкладки указывает на неё же.
		part, idx := w.TitleTabHitTest((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
		if part != widget.TitleTabPartTab || idx != i {
			t.Errorf("хит-тест центра вкладки %d = (%v, %d)", i, part, idx)
		}
	}

	// «×» есть у активной вкладки и лежит внутри неё.
	act := w.ActiveTitleTab()
	cr := w.TitleTabCloseRect(act)
	if cr.Empty() {
		t.Fatalf("у активной вкладки %d нет «×»", act)
	}
	if !cr.In(w.TitleTabRect(act)) {
		t.Errorf("«×» %v вне вкладки %v", cr, w.TitleTabRect(act))
	}
	if part, idx := w.TitleTabHitTest((cr.Min.X+cr.Max.X)/2, (cr.Min.Y+cr.Max.Y)/2); part != widget.TitleTabPartClose || idx != act {
		t.Errorf("хит-тест «×» = (%v, %d), ждали (Close, %d)", part, idx, act)
	}

	// Кнопки «+» и «v» — правее вкладок, не пересекаются.
	plus, menu := w.TitleTabPlusRect(), w.TitleTabMenuRect()
	if plus.Empty() || menu.Empty() {
		t.Fatalf("plus=%v menu=%v — обе кнопки должны быть", plus, menu)
	}
	if plus.Min.X < prev.Max.X {
		t.Errorf("«+» %v левее конца последней вкладки %v", plus, prev)
	}
	if plus.Overlaps(menu) {
		t.Errorf("«+» %v и «v» %v пересекаются", plus, menu)
	}
	if part, _ := w.TitleTabHitTest((plus.Min.X+plus.Max.X)/2, (plus.Min.Y+plus.Max.Y)/2); part != widget.TitleTabPartPlus {
		t.Errorf("хит-тест «+» = %v", part)
	}
	if part, _ := w.TitleTabHitTest((menu.Min.X+menu.Max.X)/2, (menu.Min.Y+menu.Max.Y)/2); part != widget.TitleTabPartMenu {
		t.Errorf("хит-тест «v» = %v", part)
	}

	// Свободное место полосы — ничего (там работает перетаскивание окна).
	if part, _ := w.TitleTabHitTest(menu.Max.X+20, (menu.Min.Y+menu.Max.Y)/2); part != widget.TitleTabPartNone {
		t.Errorf("хит-тест пустого места = %v, ждали None", part)
	}
}

// TestTitleTabs_CloseCrossSurvivesNarrowWindow — сценарий WinLine: на
// минимальной ширине окна «×» активной вкладки ещё отрисован.
func TestTitleTabs_CloseCrossSurvivesNarrowWindow(t *testing.T) {
	w, _ := newTabbedWindow(t, 420)
	act := w.ActiveTitleTab()
	if w.TitleTabCloseRect(act).Empty() {
		t.Errorf("на ширине 420 «×» активной вкладки пропал (вкладка %v)", w.TitleTabRect(act))
	}
}

// TestTitleTabs_ChevronTogglesMenu — репро WinLine: повторный клик по
// шеврону закрывает меню, а не открывает его заново.
func TestTitleTabs_ChevronTogglesMenu(t *testing.T) {
	w, menu := newTabbedWindow(t, 800)
	eng := engine.New(800, 300, 30)
	eng.SetRoot(w)
	w.Draw(&recCtx{}) // SetRoot переложил окно — пересчитываем геометрию

	mr := w.TitleTabMenuRect()
	if mr.Empty() {
		t.Fatal("кнопка «v» не найдена")
	}
	cx, cy := (mr.Min.X+mr.Max.X)/2, (mr.Min.Y+mr.Max.Y)/2

	eng.SendMouseButton(cx, cy, widget.MouseLeft, true)
	eng.SendMouseButton(cx, cy, widget.MouseLeft, false)
	if !menu.IsOpen() {
		t.Fatal("первый клик по шеврону не открыл меню")
	}

	eng.SendMouseButton(cx, cy, widget.MouseLeft, true)
	eng.SendMouseButton(cx, cy, widget.MouseLeft, false)
	if menu.IsOpen() {
		t.Error("второй клик по шеврону не закрыл меню (нужно два клика)")
	}

	// Третий клик снова открывает — состояние не залипло.
	eng.SendMouseButton(cx, cy, widget.MouseLeft, true)
	eng.SendMouseButton(cx, cy, widget.MouseLeft, false)
	if !menu.IsOpen() {
		t.Error("третий клик не открыл меню заново")
	}
}
