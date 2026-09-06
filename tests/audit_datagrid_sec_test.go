package tests

// audit_datagrid_sec_test.go — SEC-4 (колбэки вне мьютекса), SEC-5 (границы
// и сброс режима редактирования), SEC-11 (снятие подписки с источника).

import (
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
	dg "github.com/oops1/headless-gui/v3/widget/datagrid"
)

// ─── Инфраструктура ────────────────────────────────────────────────────────

type secRow struct {
	Name string
	Age  int
}

// newSecGrid — грид на n строк, две колонки, заголовок 20px, строка 20px.
func newSecGrid(n int) (*dg.DataGrid, *dg.ObservableCollection) {
	g := dg.New()
	g.RowHeight = 20
	g.HeaderHeight = 20

	name := dg.NewTextColumn("Name", "Name")
	name.SetActualWidth(100)
	age := dg.NewTextColumn("Age", "Age")
	age.SetActualWidth(100)
	g.AddColumn(name)
	g.AddColumn(age)

	oc := dg.NewObservableCollection()
	for i := 0; i < n; i++ {
		oc.Add(&secRow{Name: "row", Age: i})
	}
	g.SetItemsSource(oc)
	g.SetBounds(image.Rect(0, 0, 200, 20+n*20))
	return g, oc
}

// mustFinish выполняет fn в отдельной горутине и валит тест, если она не
// уложилась в отведённое время: именно так выглядит дедлок на нерекурсивном
// sync.Mutex — обработчик события зовёт публичный метод грида и встаёт
// намертво вместе со всем UI-потоком.
func mustFinish(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("%s: дедлок — обработчик под dg.mu не смог вызвать публичный метод DataGrid", what)
	}
}

// nopCtx — контекст рисования-пустышка для проверок «не паникует».
type nopCtx struct{}

func (nopCtx) FillRect(x, y, w, h int, c color.RGBA)                     {}
func (nopCtx) FillRectAlpha(x, y, w, h int, c color.RGBA)                {}
func (nopCtx) DrawBorder(x, y, w, h int, c color.RGBA)                   {}
func (nopCtx) DrawText(s string, x, y int, c color.RGBA)                 {}
func (nopCtx) DrawTextSize(s string, x, y int, sz float64, c color.RGBA) {}
func (nopCtx) MeasureText(s string, sz float64) int                      { return len(s) * 7 }
func (nopCtx) SetClip(r image.Rectangle)                                 {}
func (nopCtx) ClearClip()                                                {}
func (nopCtx) DrawHLine(x, y, length int, c color.RGBA)                  {}
func (nopCtx) DrawVLine(x, y, length int, c color.RGBA)                  {}
func (nopCtx) DrawImage(src image.Image, x, y int)                       {}
func (nopCtx) DrawImageScaled(src image.Image, x, y, w, h int)           {}
func (nopCtx) FillEllipseAA(cx, cy, rx, ry int, c color.RGBA)            {}
func (nopCtx) FillRoundRect(x, y, w, h, r int, c color.RGBA)             {}

// ─── SEC-4: колбэки не держат dg.mu ────────────────────────────────────────

// TestSEC4_OnRowActivated_NoDeadlock — обработчик двойного клика зовёт
// SelectedItem()/ScrollBy() и не вешает поток.
func TestSEC4_OnRowActivated_NoDeadlock(t *testing.T) {
	g, _ := newSecGrid(5)
	var seen interface{}
	g.OnRowActivated = func(row int, item interface{}) {
		seen = g.SelectedItem() // публичный метод под тем же dg.mu
		g.ScrollBy(1)
		_ = g.SelectedItems()
		_ = g.IsEditing()
	}
	mustFinish(t, "OnRowActivated (двойной клик)", func() {
		g.OnMouseDoubleClick(10, 20+25)
	})
	_ = seen
}

// TestSEC4_OnRowActivated_EnterKey_NoDeadlock — тот же обработчик по Enter:
// раньше он вызывался прямо из-под dg.mu в OnKeyEvent.
func TestSEC4_OnRowActivated_EnterKey_NoDeadlock(t *testing.T) {
	g, _ := newSecGrid(5)
	fired := 0
	g.OnRowActivated = func(row int, item interface{}) {
		fired++
		_ = g.SelectedItem()
		g.ScrollBy(1)
	}
	g.SetSelectedIndex(1)
	mustFinish(t, "OnRowActivated (Enter)", func() {
		g.OnKeyEvent(13, 0, true, false, false)
	})
	if fired != 1 {
		t.Fatalf("OnRowActivated по Enter сработал %d раз, ожидался 1", fired)
	}
}

// TestSEC4_OnSorting_NoDeadlock — обработчик сортировки зовёт публичные
// методы грида; при Handled=false грид всё равно должен отсортировать данные.
func TestSEC4_OnSorting_NoDeadlock(t *testing.T) {
	g, _ := newSecGrid(5)
	fired := 0
	g.OnSorting = func(e *dg.SortingEvent) {
		fired++
		_ = g.SelectedItem()
		g.ScrollBy(0)
		_ = g.Columns()
	}
	mustFinish(t, "OnSorting", func() {
		g.OnMouseButton(10, 5, 0, true) // клик по заголовку первой колонки
	})
	if fired != 1 {
		t.Fatalf("OnSorting сработал %d раз, ожидался 1", fired)
	}
}

// TestSEC4_OnSorting_HandledSkipsSort — Handled=true по-прежнему отменяет
// штатную сортировку (семантика не должна была измениться от переноса
// колбэка за пределы лока).
func TestSEC4_OnSorting_HandledSkipsSort(t *testing.T) {
	g, oc := newSecGrid(3)
	oc.Set(0, &secRow{Name: "c", Age: 0})
	oc.Set(1, &secRow{Name: "a", Age: 1})
	oc.Set(2, &secRow{Name: "b", Age: 2})

	g.OnSorting = func(e *dg.SortingEvent) { e.Handled = true }
	g.OnMouseButton(10, 5, 0, true)

	g.SetSelectedIndex(0)
	first, _ := g.SelectedItem().(*secRow)
	if first == nil || first.Name != "c" {
		t.Fatalf("Handled=true не отменил сортировку: первая строка %#v", first)
	}
}

// TestSEC4_OnCellEditEnding_NoDeadlock — обработчик завершения правки зовёт
// публичные методы и может отменить запись.
func TestSEC4_OnCellEditEnding_NoDeadlock(t *testing.T) {
	g, oc := newSecGrid(3)
	fired := 0
	g.OnCellEditEnding = func(e *dg.CellEditEndingEvent) {
		fired++
		_ = g.SelectedItem()
		_ = g.ScrollY()
		g.ScrollBy(0)
	}

	mustFinish(t, "OnCellEditEnding", func() {
		g.OnMouseDoubleClick(10, 20+5) // начать правку строки 0
		g.OnKeyEvent(88, 'X', true, false, false)
		g.OnKeyEvent(13, 0, true, false, false) // Enter — commit
	})
	if fired != 1 {
		t.Fatalf("OnCellEditEnding сработал %d раз, ожидался 1", fired)
	}
	if got := oc.Get(0).(*secRow).Name; got == "row" {
		t.Fatalf("значение не записано в модель после коммита: %q", got)
	}
}

// TestSEC4_OnCellEditEnding_CancelKeepsModel — Cancel=true не пишет в модель.
func TestSEC4_OnCellEditEnding_CancelKeepsModel(t *testing.T) {
	g, oc := newSecGrid(3)
	g.OnCellEditEnding = func(e *dg.CellEditEndingEvent) { e.Cancel = true }

	g.OnMouseDoubleClick(10, 20+5)
	g.OnKeyEvent(88, 'X', true, false, false)
	g.OnKeyEvent(13, 0, true, false, false)

	if got := oc.Get(0).(*secRow).Name; got != "row" {
		t.Fatalf("Cancel=true всё равно записал значение: %q", got)
	}
	if g.IsEditing() {
		t.Fatal("после Cancel грид остался в режиме редактирования")
	}
}

// TestSEC4_OnSelectionChanged_NoDeadlock — регресс на уже починенный путь.
func TestSEC4_OnSelectionChanged_NoDeadlock(t *testing.T) {
	g, _ := newSecGrid(5)
	g.OnSelectionChanged = func(e dg.SelectionChangedEvent) {
		_ = g.SelectedItem()
		g.ScrollBy(0)
	}
	mustFinish(t, "OnSelectionChanged", func() {
		g.OnMouseButton(10, 20+25, 0, true)
	})
}

// ─── SEC-5: границы и режим редактирования ─────────────────────────────────

// TestSEC5_EditThenClearCollection — начали правку, коллекция очищена из
// фоновой горутины, дальше клик/коммит/отрисовка. Раньше здесь была паника
// на dg.sortedIdx[dg.editingRow].
func TestSEC5_EditThenClearCollection(t *testing.T) {
	g, oc := newSecGrid(5)

	g.OnMouseDoubleClick(10, 20+25) // правка строки 1
	if !g.IsEditing() {
		t.Fatal("редактирование не началось — тест ничего не проверяет")
	}

	oc.Clear()

	if g.IsEditing() {
		t.Error("после Clear() грид остался в режиме редактирования исчезнувшей строки")
	}
	g.OnKeyEvent(13, 0, true, false, false) // commit по пустой коллекции
	g.OnMouseButton(10, 20+25, 0, true)
	g.Draw(nopCtx{})
}

// TestSEC5_EditThenRemoveRows — коллекция сжалась, редактируемая строка
// исчезла: правка отменяется БЕЗ коммита, паники нет.
func TestSEC5_EditThenRemoveRows(t *testing.T) {
	g, oc := newSecGrid(5)

	g.OnMouseDoubleClick(10, 20+4*20+5) // правка последней строки (4)
	if !g.IsEditing() {
		t.Fatal("редактирование не началось")
	}
	g.OnKeyEvent(88, 'X', true, false, false)

	// Фоновая горутина сносит хвост коллекции.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		oc.RemoveAt(4)
		oc.RemoveAt(3)
	}()
	wg.Wait()

	if g.IsEditing() {
		t.Error("редактирование исчезнувшей строки не отменено")
	}
	g.OnKeyEvent(13, 0, true, false, false)
	g.Draw(nopCtx{})

	for i := 0; i < oc.Count(); i++ {
		if got := oc.Get(i).(*secRow).Name; got != "row" {
			t.Fatalf("правка исчезнувшей строки утекла в строку %d: %q", i, got)
		}
	}
}

// TestSEC5_SetColumnsDuringEdit — смена набора колонок посреди правки:
// editingCol обязан быть сброшен, иначе dg.columns[editingCol] за границей.
func TestSEC5_SetColumnsDuringEdit(t *testing.T) {
	g, _ := newSecGrid(4)

	g.OnMouseDoubleClick(150, 20+25) // правка второй колонки
	if !g.IsEditing() {
		t.Fatal("редактирование не началось")
	}

	one := dg.NewTextColumn("Only", "Name")
	one.SetActualWidth(100)
	g.SetColumns([]dg.Column{one})

	if g.IsEditing() {
		t.Error("SetColumns не сбросил режим редактирования")
	}
	g.OnKeyEvent(13, 0, true, false, false)
	g.OnMouseButton(10, 20+25, 0, true)
	g.OnMouseMove(99, 5) // resize по протухшему индексу колонки
	g.Draw(nopCtx{})
}

// TestSEC5_DrawEditCell_CursorClamp — длинное значение, курсор в конце,
// затем модель под редактором меняется: срез рун по старой позиции не должен
// ронять кадр.
func TestSEC5_DrawEditCell_CursorClamp(t *testing.T) {
	g, oc := newSecGrid(3)
	oc.Set(0, &secRow{Name: "очень длинное значение ячейки", Age: 0})

	g.SetFocused(true)
	g.OnMouseDoubleClick(10, 20+5)
	for i := 0; i < 40; i++ {
		g.OnKeyEvent(39, 0, true, false, false) // End-ward
	}
	g.OnKeyEvent(35, 0, true, false, false) // End
	oc.Set(0, &secRow{Name: "", Age: 0})    // значение «схлопнулось»
	g.Draw(nopCtx{})
	g.OnKeyEvent(8, 0, true, false, false) // Backspace по клэмпнутой позиции
	g.Draw(nopCtx{})
}

// TestSEC5_ConcurrentMutationWhileDrawing — фоновая горутина перетряхивает
// коллекцию (Add/RemoveAt/Clear), пока UI-поток рисует кадры и обрабатывает
// ввод, в том числе входит в редактирование. Ни паники, ни гонок (-race).
//
// Ввод и отрисовка нарочно в ОДНОЙ горутине: это и есть UI-поток движка.
// Поля самих элементов модели ничем не защищены (как и в WPF) — их правит
// только UI-поток, а фоновая горутина меняет лишь состав коллекции, где
// синхронизацию обеспечивает сама ObservableCollection.
func TestSEC5_ConcurrentMutationWhileDrawing(t *testing.T) {
	g, oc := newSecGrid(50)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	go func() { // фоновый мутатор коллекции
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			oc.Add(&secRow{Name: "new", Age: i})
			oc.RemoveAt(0)
			if i%50 == 49 {
				oc.Clear()
				for j := 0; j < 20; j++ {
					oc.Add(&secRow{Name: "again", Age: j})
				}
			}
		}
	}()
	go func() { // UI-поток: ввод + отрисовка
		defer wg.Done()
		ctx := nopCtx{}
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			y := 20 + (i%40)*20
			g.OnMouseMove(50, y)
			g.OnMouseButton(50, y, 0, true)
			g.OnMouseDoubleClick(50, y) // вход в редактирование
			g.Draw(ctx)
			g.OnKeyEvent(13, 0, true, false, false) // commit по исчезнувшей строке
			g.OnKeyEvent(27, 0, true, false, false)
			g.Draw(ctx)
		}
	}()

	time.Sleep(300 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// ─── SEC-11: снятие подписки с источника ───────────────────────────────────

// TestSEC11_DataGridUnsubscribesOldSource — N перебиндовок не оставляют живых
// обработчиков на брошенных коллекциях.
func TestSEC11_DataGridUnsubscribesOldSource(t *testing.T) {
	g, first := newSecGrid(3)
	if n := first.HandlerCount(); n != 1 {
		t.Fatalf("после SetItemsSource у источника %d подписчиков, ожидался 1", n)
	}

	prev := first
	for i := 0; i < 20; i++ {
		next := dg.NewObservableCollection()
		next.Add(&secRow{Name: "x", Age: i})
		g.SetItemsSource(next)

		if n := prev.HandlerCount(); n != 0 {
			t.Fatalf("итерация %d: у прежнего источника осталось %d подписчиков", i, n)
		}
		if n := next.HandlerCount(); n != 1 {
			t.Fatalf("итерация %d: у нового источника %d подписчиков, ожидался 1", i, n)
		}
		prev = next
	}

	g.Dispose()
	if n := prev.HandlerCount(); n != 0 {
		t.Fatalf("после Dispose осталось %d подписчиков", n)
	}
}

// TestSEC11_CollectionViewUnsubscribesOldSource — то же для CollectionView.
func TestSEC11_CollectionViewUnsubscribesOldSource(t *testing.T) {
	first := sampleOC()
	v := widget.NewCollectionView(first)
	if n := first.HandlerCount(); n != 1 {
		t.Fatalf("после NewCollectionView у источника %d подписчиков, ожидался 1", n)
	}

	prev := first
	for i := 0; i < 20; i++ {
		next := sampleOC()
		v.SetSource(next)
		if n := prev.HandlerCount(); n != 0 {
			t.Fatalf("итерация %d: у прежнего источника осталось %d подписчиков", i, n)
		}
		if n := next.HandlerCount(); n != 1 {
			t.Fatalf("итерация %d: у нового источника %d подписчиков", i, n)
		}
		prev = next
	}

	v.Dispose()
	if n := prev.HandlerCount(); n != 0 {
		t.Fatalf("после Dispose осталось %d подписчиков", n)
	}
}

// TestSEC11_OldSourceNoLongerDrivesGrid — брошенная коллекция больше не
// перестраивает грид: после перебиндовки её изменения не меняют число строк.
func TestSEC11_OldSourceNoLongerDrivesGrid(t *testing.T) {
	g, old := newSecGrid(3)

	fresh := dg.NewObservableCollection()
	fresh.Add(&secRow{Name: "only", Age: 0})
	g.SetItemsSource(fresh)

	for i := 0; i < 100; i++ {
		old.Add(&secRow{Name: "ghost", Age: i})
	}

	g.SetSelectedIndex(0)
	if it, _ := g.SelectedItem().(*secRow); it == nil || it.Name != "only" {
		t.Fatalf("грид смотрит не на новый источник: %#v", it)
	}
	g.SetSelectedIndex(1)
	if g.SelectedItem() != nil {
		t.Fatal("грид видит строки из брошенной коллекции")
	}
}
