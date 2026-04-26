package tests

import (
	"image"
	"image/color"
	"sync/atomic"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	dgridPkg "github.com/oops1/headless-gui/v3/widget/datagrid"
)

var (
	tcLabel = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	tcPanel = color.RGBA{R: 30, G: 30, B: 30, A: 255}
)

// ─── A1: Grid Star=0 сворачивает столбец ───────────────────────────────────

// TestGrid_StarZero_CollapsesColumn — A1: <ColumnDefinition Width="0*"/>
// должен реально получить 0px, а не быть приведён к 1*.
func TestGrid_StarZero_CollapsesColumn(t *testing.T) {
	g := widget.NewGrid()
	g.ColDefs = []widget.GridDefinition{
		{Mode: widget.GridSizeStar, Value: 0}, // свёрнут
		{Mode: widget.GridSizeStar, Value: 1},
		{Mode: widget.GridSizeStar, Value: 1},
	}
	g.RowDefs = []widget.GridDefinition{
		{Mode: widget.GridSizeStar, Value: 1},
	}
	g.SetBounds(image.Rect(0, 0, 300, 100))

	// Расставим маркеры (Label) по колонкам, чтобы померить cellRect через
	// ребёнка. Это самый простой способ инспектировать cell width без
	// доступа к приватным rowOffsets/colOffsets.
	for col := 0; col < 3; col++ {
		l := widget.NewLabel("·", tcLabel)
		l.SetGridProps(0, col, 1, 1)
		g.AddChild(l)
	}
	g.SetBounds(image.Rect(0, 0, 300, 100)) // принудительный re-layout

	gotWidths := make([]int, 3)
	for i, ch := range g.Children() {
		gotWidths[i] = ch.Bounds().Dx()
	}

	if gotWidths[0] != 0 {
		t.Errorf("Star=0 column should collapse to 0px, got %d", gotWidths[0])
	}
	// Оставшиеся 1* + 1* делят 300px поровну.
	if gotWidths[1] != 150 || gotWidths[2] != 150 {
		t.Errorf("remaining stars should split 300px 50/50, got %d, %d", gotWidths[1], gotWidths[2])
	}
}

// TestGrid_StarNegative_StillDefaultsToOne проверяет, что отрицательное
// значение Star по-прежнему трактуется как 1* (защита от мусора).
func TestGrid_StarNegative_StillDefaultsToOne(t *testing.T) {
	g := widget.NewGrid()
	g.ColDefs = []widget.GridDefinition{
		{Mode: widget.GridSizeStar, Value: -5}, // мусор → 1*
		{Mode: widget.GridSizeStar, Value: 1},
	}
	g.RowDefs = []widget.GridDefinition{{Mode: widget.GridSizeStar, Value: 1}}
	g.SetBounds(image.Rect(0, 0, 200, 100))

	for col := 0; col < 2; col++ {
		l := widget.NewLabel("·", tcLabel)
		l.SetGridProps(0, col, 1, 1)
		g.AddChild(l)
	}
	g.SetBounds(image.Rect(0, 0, 200, 100))

	w0 := g.Children()[0].Bounds().Dx()
	w1 := g.Children()[1].Bounds().Dx()
	if w0 != 100 || w1 != 100 {
		t.Errorf("negative Star should default to 1*, expected 100/100, got %d/%d", w0, w1)
	}
}

// ─── A5: Button.OnClick keyboard path синхронный ───────────────────────────

// TestButton_OnClick_KeyboardSynchronous проверяет, что Enter вызывает
// OnClick синхронно (без go-обёртки) — счётчик должен быть инкрементирован
// ДО возврата OnKeyEvent.
func TestButton_OnClick_KeyboardSynchronous(t *testing.T) {
	btn := widget.NewButton("ok")
	var counter int32
	btn.OnClick = func() { atomic.AddInt32(&counter, 1) }

	// Press
	btn.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if got := atomic.LoadInt32(&counter); got != 1 {
		t.Fatalf("expected synchronous OnClick on Enter, counter=%d", got)
	}

	// Space — тоже должен сработать
	btn.OnKeyEvent(widget.KeyEvent{Code: widget.KeySpace, Pressed: true})
	if got := atomic.LoadInt32(&counter); got != 2 {
		t.Fatalf("expected synchronous OnClick on Space, counter=%d", got)
	}

	// Release не должен дёргать OnClick
	btn.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: false})
	if got := atomic.LoadInt32(&counter); got != 2 {
		t.Fatalf("release must not fire OnClick, counter=%d", got)
	}
}

// ─── A7: Button — несколько обработчиков клика ─────────────────────────────

// TestButton_AddClickHandler_Multiple подтверждает, что несколько подписчиков
// получают вызов в порядке регистрации, а OnClick (поле) идёт первым.
func TestButton_AddClickHandler_Multiple(t *testing.T) {
	btn := widget.NewButton("multi")
	var order []string

	btn.OnClick = func() { order = append(order, "field") }
	btn.AddClickHandler(func() { order = append(order, "h1") })
	btn.AddClickHandler(func() { order = append(order, "h2") })

	btn.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})

	if len(order) != 3 || order[0] != "field" || order[1] != "h1" || order[2] != "h2" {
		t.Fatalf("expected [field, h1, h2], got %v", order)
	}
}

// TestButton_RemoveClickHandler — снятая подписка не вызывается.
func TestButton_RemoveClickHandler(t *testing.T) {
	btn := widget.NewButton("rm")
	var keptCalls, removedCalls int32

	id := btn.AddClickHandler(func() { atomic.AddInt32(&removedCalls, 1) })
	btn.AddClickHandler(func() { atomic.AddInt32(&keptCalls, 1) })

	if !btn.RemoveClickHandler(id) {
		t.Fatal("RemoveClickHandler should report success")
	}
	if btn.RemoveClickHandler(id) {
		t.Fatal("second RemoveClickHandler with same id should fail")
	}

	btn.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})

	if atomic.LoadInt32(&removedCalls) != 0 {
		t.Errorf("removed handler must not fire")
	}
	if atomic.LoadInt32(&keptCalls) != 1 {
		t.Errorf("kept handler should fire once, got %d", keptCalls)
	}
}

// TestButton_ClearClickHandlers сбрасывает все добавленные обработчики,
// но сохраняет поле OnClick.
func TestButton_ClearClickHandlers(t *testing.T) {
	btn := widget.NewButton("clr")
	var fieldCalls, handlerCalls int32

	btn.OnClick = func() { atomic.AddInt32(&fieldCalls, 1) }
	btn.AddClickHandler(func() { atomic.AddInt32(&handlerCalls, 1) })

	btn.ClearClickHandlers()
	btn.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})

	if atomic.LoadInt32(&fieldCalls) != 1 {
		t.Errorf("OnClick field must still fire after ClearClickHandlers, got %d", fieldCalls)
	}
	if atomic.LoadInt32(&handlerCalls) != 0 {
		t.Errorf("AddClickHandler subscribers must be cleared, got %d", handlerCalls)
	}
}

// ─── A6: ListView — KeepScrollAtBottom / PreserveScroll ────────────────────

// TestListView_AutoScrollToBottom_OnSetItems — при добавлении новых
// элементов в режиме live-tail прокрутка должна оставаться у конца.
func TestListView_AutoScrollToBottom_OnSetItems(t *testing.T) {
	lv := widget.NewListView()
	lv.AutoScrollToBottom = true
	lv.SetBounds(image.Rect(0, 0, 200, 100)) // ~3 элемента видно при ItemHeight=28

	// Заполняем 30 элементами и сразу скроллим в конец.
	items := make([]string, 30)
	for i := range items {
		items[i] = "log line"
	}
	lv.SetItems(items)
	lv.ScrollToBottom()
	bottomScroll := scrollY(lv)
	if bottomScroll == 0 {
		t.Fatal("ScrollToBottom should move scrollY > 0")
	}

	// Добавляем ещё 30 — пользователь был у конца, должен «прилипнуть» к концу.
	items2 := append(items, items...)
	lv.SetItems(items2)

	if scrollY(lv) <= bottomScroll {
		t.Errorf("AutoScrollToBottom expected new scrollY > previous bottom (%d), got %d",
			bottomScroll, scrollY(lv))
	}
}

// TestListView_DefaultSetItemsResetsScroll — старое поведение по умолчанию.
func TestListView_DefaultSetItemsResetsScroll(t *testing.T) {
	lv := widget.NewListView()
	lv.SetBounds(image.Rect(0, 0, 200, 100))

	items := make([]string, 30)
	for i := range items {
		items[i] = "x"
	}
	lv.SetItems(items)
	lv.ScrollToBottom()

	if scrollY(lv) == 0 {
		t.Fatal("scroll should move to bottom for setup")
	}

	// SetItems без флагов → scrollY должно стать 0.
	lv.SetItems(items)
	if scrollY(lv) != 0 {
		t.Errorf("default SetItems should reset scrollY to 0, got %d", scrollY(lv))
	}
}

// TestListView_PreserveScroll — при выставленном PreserveScrollOnSetItems
// scrollY сохраняется (с учётом нового maxScroll).
func TestListView_PreserveScroll(t *testing.T) {
	lv := widget.NewListView()
	lv.PreserveScrollOnSetItems = true
	lv.SetBounds(image.Rect(0, 0, 200, 100))

	items := make([]string, 50)
	for i := range items {
		items[i] = "x"
	}
	lv.SetItems(items)
	lv.ScrollBy(100)
	prev := scrollY(lv)
	if prev == 0 {
		t.Fatal("setup: scroll should be > 0")
	}

	lv.SetItems(items) // тот же объём
	if scrollY(lv) != prev {
		t.Errorf("PreserveScroll: expected scrollY=%d, got %d", prev, scrollY(lv))
	}
}

// ─── A9: Engine.SetRoot сохраняет bounds корня ────────────────────────────

func TestEngine_SetRoot_PreservesNonEmptyBounds(t *testing.T) {
	eng := engine.New(1000, 800, 20)

	root := widget.NewPanel(tcPanel)
	root.SetBounds(image.Rect(50, 50, 450, 350))
	eng.SetRoot(root)

	got := root.Bounds()
	want := image.Rect(50, 50, 450, 350)
	if got != want {
		t.Errorf("SetRoot must preserve non-empty bounds, got %v want %v", got, want)
	}
}

func TestEngine_SetRoot_FillsCanvasIfEmpty(t *testing.T) {
	eng := engine.New(640, 480, 20)
	root := widget.NewPanel(tcPanel) // bounds пустые → должен растянуться

	eng.SetRoot(root)

	if root.Bounds() != image.Rect(0, 0, 640, 480) {
		t.Errorf("empty root should be stretched to canvas, got %v", root.Bounds())
	}
}

func TestEngine_SetRootFullCanvas_AlwaysFills(t *testing.T) {
	eng := engine.New(640, 480, 20)
	root := widget.NewPanel(tcPanel)
	root.SetBounds(image.Rect(10, 10, 100, 100))

	eng.SetRootFullCanvas(root)

	if root.Bounds() != image.Rect(0, 0, 640, 480) {
		t.Errorf("SetRootFullCanvas must override bounds, got %v", root.Bounds())
	}
}

// ─── A4: per-column IsReadOnly tri-state ──────────────────────────────────

// ─── A3: OnRowActivated стреляет на double-click ──────────────────────────

type rowItem struct {
	Name  string
	Value int
}

// TestDataGrid_OnRowActivated_DoubleClick проверяет, что callback
// OnRowActivated вызывается при двойном клике, причём он срабатывает
// даже если грид read-only (типичный сценарий — toggle breakpoint
// в дизассемблере).
func TestDataGrid_OnRowActivated_DoubleClick(t *testing.T) {
	dg := dgridPkg.New()
	dg.IsReadOnly = true // read-only грид
	dg.RowHeight = 20
	dg.HeaderHeight = 0 // упростим расчёт Y
	dg.SetBounds(image.Rect(0, 0, 200, 200))

	col := dgridPkg.NewTextColumn("Name", "Name")
	col.SetActualWidth(150)
	dg.AddColumn(col)

	src := dgridPkg.NewObservableCollection()
	src.Add(&rowItem{Name: "a"})
	src.Add(&rowItem{Name: "b"})
	src.Add(&rowItem{Name: "c"})
	dg.SetItemsSource(src)

	var (
		gotRow  int = -1
		gotItem interface{}
		fired   int32
	)
	dg.OnRowActivated = func(row int, item interface{}) {
		atomic.AddInt32(&fired, 1)
		gotRow = row
		gotItem = item
	}

	// Двойной клик по второй строке (y=20..40).
	dg.OnMouseDoubleClick(10, 25)

	if atomic.LoadInt32(&fired) != 1 {
		t.Fatalf("OnRowActivated must fire even on read-only grid, fired=%d", fired)
	}
	if gotRow != 1 {
		t.Errorf("expected row=1, got %d", gotRow)
	}
	if it, ok := gotItem.(*rowItem); !ok || it.Name != "b" {
		t.Errorf("expected item Name=b, got %#v", gotItem)
	}
}

// ─── A4: per-column IsReadOnly tri-state ──────────────────────────────────

// TestColumn_ReadOnly_Tristate проверяет три состояния:
//   - не выставлено → IsReadOnlyExplicit=false, IsReadOnly()=false
//   - SetReadOnly(true) → explicit=true, value=true
//   - SetReadOnly(false) после true → explicit=true, value=false
//   - ResetReadOnly() → explicit=false снова.
func TestColumn_ReadOnly_Tristate(t *testing.T) {
	col := dgridPkg.NewTextColumn("h", "Path")

	if col.IsReadOnlyExplicit() {
		t.Fatal("fresh column must not be explicit")
	}
	if col.IsReadOnly() {
		t.Fatal("fresh column must not be RO by default")
	}

	col.SetReadOnly(true)
	if !col.IsReadOnlyExplicit() || !col.IsReadOnly() {
		t.Fatal("SetReadOnly(true) must set both explicit and value")
	}

	col.SetReadOnly(false)
	if !col.IsReadOnlyExplicit() {
		t.Fatal("SetReadOnly(false) is still explicit override")
	}
	if col.IsReadOnly() {
		t.Fatal("SetReadOnly(false) must zero the value")
	}

	col.ResetReadOnly()
	if col.IsReadOnlyExplicit() || col.IsReadOnly() {
		t.Fatal("ResetReadOnly must clear both flags")
	}
}

// scrollY извлекает приватное поле scrollY через ScrollBy(0)
// scrollY возвращает текущее смещение прокрутки ListView через
// внутренний test-only хелпер ListViewScrollY.
func scrollY(lv *widget.ListView) int {
	return widget.ListViewScrollY(lv)
}
