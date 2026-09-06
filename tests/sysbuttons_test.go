package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Штатные кнопки окна у диалога и свободное место, за которое окно тащат.
//
// В полосе заголовка диалога была одна ✕: диалогу-вопросу больше и не нужно.
// Диалог, показанный в собственном окне ОС и растянутый на пол-экрана, ведёт
// себя как окно — и человек ищет в его полосе привычные три кнопки. А поле
// поиска, растянутое до самых кнопок, не оставляло в полосе ни одной точки,
// за которую окно можно было бы схватить мышью.

// clickChildAt нажимает и отпускает левую кнопку в точке, доставляя события
// ребёнку под ней (движок в этих тестах не участвует).
func clickChildAt(t *testing.T, parent widget.Widget, pt image.Point) {
	t.Helper()
	children := parent.Children()
	for i := len(children) - 1; i >= 0; i-- {
		c := children[i]
		if !widget.IsWidgetVisible(c) || !pt.In(c.Bounds()) {
			continue
		}
		mb, ok := c.(interface{ OnMouseButton(widget.MouseEvent) bool })
		if !ok {
			continue
		}
		mb.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true})
		mb.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: false})
		return
	}
	t.Fatalf("под точкой %v нет виджета, принимающего нажатие", pt)
}

func sysBtnDialog(t *testing.T) *widget.Dialog {
	t.Helper()
	d := widget.NewDialog("Настройки", 700, 400)
	d.SetBounds(image.Rect(0, 0, 700, 400))
	d.SetWindowButtons(true)
	return d
}

// Три кнопки стоят в полосе справа налево и не налезают друг на друга.
func TestWindowButtons_ThreeInARow(t *testing.T) {
	d := sysBtnDialog(t)
	if !d.HasWindowButtons() {
		t.Fatal("кнопки не появились")
	}

	var rects []image.Rectangle
	for _, c := range d.Children() {
		r := c.Bounds()
		if r.Empty() || r.Max.Y > d.Bounds().Min.Y+d.TitleHeight {
			continue
		}
		rects = append(rects, r)
	}
	if len(rects) != 3 {
		t.Fatalf("в полосе %d кнопок, ожидалось 3: %v", len(rects), rects)
	}
	for i := range rects {
		for j := i + 1; j < len(rects); j++ {
			if rects[i].Overlaps(rects[j]) {
				t.Errorf("кнопки перекрываются: %v и %v", rects[i], rects[j])
			}
		}
		if rects[i].Max.X > d.Bounds().Max.X {
			t.Errorf("кнопка вылезла за диалог: %v", rects[i])
		}
	}
}

// Кнопки зовут свои действия — их ставит нативный хост окна.
func TestWindowButtons_CallHooks(t *testing.T) {
	d := sysBtnDialog(t)

	min, maxRestore := 0, 0
	d.OnMinimize = func() { min++ }
	d.OnMaximizeRestore = func() { maxRestore++ }

	b := d.Bounds()
	y := b.Min.Y + d.TitleHeight/2
	// Справа налево: ✕, «развернуть», «свернуть» — шаг 26 (24 + зазор).
	clickChildAt(t, d, image.Pt(b.Max.X-6-12-26, y))
	clickChildAt(t, d, image.Pt(b.Max.X-6-12-52, y))

	if maxRestore != 1 {
		t.Errorf("«развернуть» вызвано %d раз", maxRestore)
	}
	if min != 1 {
		t.Errorf("«свернуть» вызвано %d раз", min)
	}
}

// Закрытие осталось на ✕: кнопки добавились рядом, а не вместо неё.
func TestWindowButtons_CloseStillCloses(t *testing.T) {
	eng := newDialogEngine()
	d := widget.NewDialog("Настройки", 700, 400)
	d.SetWindowButtons(true)
	eng.ShowModal(d)

	b := d.Bounds()
	clickChildAt(t, d, image.Pt(b.Max.X-6-12, b.Min.Y+d.TitleHeight/2))

	if d.IsModal() {
		t.Error("нажатие на ✕ не закрыло диалог")
	}
}

// Развёрнутое окно рисует «восстановить» вместо «развернуть».
func TestWindowButtons_MaximizedGlyph(t *testing.T) {
	scene := func(maximized bool) *image.RGBA {
		d := widget.NewDialog("Настройки", 300, 200)
		d.SetWindowButtons(true)
		d.SetMaximized(maximized)
		return widgetScene(t, d, image.Rect(20, 10, 320, 210))
	}

	a, b := scene(false), scene(true)
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return
		}
	}
	t.Error("развёрнутое окно нарисовало ту же кнопку, что и обычное")
}

// ─── Место, за которое тащат окно ───────────────────────────────────────────

// Между начинкой приложения и кнопками остаётся свободная полоска.
func TestTitleBar_LeavesDragHandle(t *testing.T) {
	d := sysBtnDialog(t)
	search := widget.NewTextInput("Поиск настроек...")
	d.SetTitleBarContent(search)

	gap := d.Bounds().Max.X - search.Bounds().Max.X
	if gap < 60 {
		t.Errorf("между поиском и краем диалога всего %d точек", gap)
	}
}

// И за эту полоску диалог действительно тащится.
func TestTitleBar_DragHandleMovesDialog(t *testing.T) {
	d := sysBtnDialog(t)
	search := widget.NewTextInput("Поиск настроек...")
	d.SetTitleBarContent(search)

	before := d.Bounds()
	// Середина между правым краем поиска и левым краем кнопок.
	x := (search.Bounds().Max.X + before.Max.X - 6 - 3*24) / 2
	pt := image.Pt(x, before.Min.Y+d.TitleHeight/2)

	d.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true})
	d.OnMouseMove(pt.X+40, pt.Y+25)
	d.OnMouseButton(widget.MouseEvent{X: pt.X + 40, Y: pt.Y + 25, Button: widget.MouseLeft, Pressed: false})

	if d.Bounds().Min.X != before.Min.X+40 || d.Bounds().Min.Y != before.Min.Y+25 {
		t.Errorf("диалог не сдвинулся за свободную часть полосы: %v → %v", before, d.Bounds())
	}
}

// Кнопки занимают место: начинка ужимается, а не залезает под них.
func TestTitleBar_ContentShrinksForButtons(t *testing.T) {
	d := widget.NewDialog("Настройки", 700, 400)
	d.SetBounds(image.Rect(0, 0, 700, 400))
	search := widget.NewTextInput("Поиск...")
	d.SetTitleBarContent(search)
	wide := search.Bounds().Max.X

	d.SetWindowButtons(true)

	if narrow := search.Bounds().Max.X; narrow >= wide {
		t.Errorf("поиск не ужался: было до %d, стало до %d", wide, narrow)
	}
	// И не пересекается с кнопками.
	for _, c := range d.Children() {
		if c == widget.Widget(search) {
			continue
		}
		r := c.Bounds()
		if r.Empty() || r.Max.Y > d.TitleHeight {
			continue
		}
		if r.Overlaps(search.Bounds()) {
			t.Errorf("поиск %v налезает на кнопку %v", search.Bounds(), r)
		}
	}
}

// У окна (widget.Window) все три кнопки были и остаются — начинка полосы их не
// вытесняет.
//
// Стиль заголовка задан ЯВНО: он зависит от ОС, а раскладка полосы в них
// зеркальная — в Windows кнопки справа, в macOS «светофор» слева. Тест на одну
// раскладку, положившийся на автоопределение, ловил бы ошибку только на одной
// платформе (и падал на другой).
func titleBarWindow(t *testing.T, style widget.WindowTitleStyle) (*widget.Window, *widget.TextInput) {
	t.Helper()
	w := widget.NewWindow("Настройки", 800, 600)
	w.TitleStyle = style
	w.SetBounds(image.Rect(0, 0, 800, 600))
	search := widget.NewTextInput("Поиск...")
	w.SetTitleBarContent(search)
	return w, search
}

func TestWindowTitleBar_KeepsThreeButtons(t *testing.T) {
	w, search := titleBarWindow(t, widget.WindowTitleWin)

	closeR, minR, maxR := w.CloseBtnRect(), w.MinBtnRect(), w.MaxBtnRect()
	if closeR.Empty() || minR.Empty() || maxR.Empty() {
		t.Fatalf("не все кнопки на месте: ✕=%v ─=%v □=%v", closeR, minR, maxR)
	}
	for _, r := range []image.Rectangle{closeR, minR, maxR} {
		if r.Overlaps(search.Bounds()) {
			t.Errorf("поиск %v налезает на кнопку %v", search.Bounds(), r)
		}
	}
	if gap := minR.Min.X - search.Bounds().Max.X; gap < 60 {
		t.Errorf("между поиском и кнопками всего %d точек — окно не за что схватить", gap)
	}
}

// В mac-раскладке режима «свои контролы в полосе» нет: кнопки окна стоят слева,
// подпись центрирована, и виджет приложения пришлось бы втискивать между ними —
// такой полосы в macOS нет ни у одного окна.
func TestWindowTitleBar_MacHasNoBarContent(t *testing.T) {
	w, search := titleBarWindow(t, widget.WindowTitleMac)

	if r := w.TitleBarContentBounds(); !r.Empty() {
		t.Errorf("в mac-полосе нашлось место для начинки: %v", r)
	}
	if r := search.Bounds(); !r.Empty() {
		t.Errorf("виджет приложения разложен в mac-полосе: %v", r)
	}
	if widget.IsWidgetVisible(search) {
		t.Error("виджет в mac-полосе остался видимым")
	}

	// Кнопка сворачивания — часть того же режима, её там тоже нет.
	w.SetNavButton(true)
	for _, c := range w.Children() {
		if c == widget.Widget(search) {
			continue
		}
		if r := c.Bounds(); !r.Empty() && r.Max.Y <= w.ContentBounds().Min.Y {
			t.Errorf("в mac-полосе разложен виджет движка: %v", r)
		}
	}
}

// Та же полоса в Windows-раскладке начинку принимает — проверка, что запрет
// касается именно mac, а не выключил режим целиком.
func TestWindowTitleBar_WinHasBarContent(t *testing.T) {
	_, search := titleBarWindow(t, widget.WindowTitleWin)

	if search.Bounds().Empty() {
		t.Error("в Windows-раскладке начинке не отвели места")
	}
}
