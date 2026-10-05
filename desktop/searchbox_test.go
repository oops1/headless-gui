package desktop

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Строка поиска на панели задач: режимы, ввод, подсказка, связь с меню «Пуск».

func searchFixture(t *testing.T) (*SearchBox, *FakeSearchProvider, *StartMenu, *Taskbar, *StartButton) {
	t.Helper()
	tm := managerFor(t, theme.ProfileWindows10)
	prov := NewFakeSearchProvider(
		SearchResult{ID: "edge", Title: "Microsoft Edge"},
		SearchResult{ID: "calc", Title: "Calculator"},
	)
	box := NewSearchBox(tm, prov)
	start := NewStartButton(tm)
	bar := NewTaskbar(tm)
	t.Cleanup(bar.Close)
	bar.AddItem(SlotStart, start)
	bar.AddItem(SlotStart, box)
	bar.SetBounds(image.Rect(0, tileScreenH-40, tileScreenW, tileScreenH))

	menu := NewStartMenu(tm, NewStaticAppCatalog(tileApps()...))
	menu.Screen = tileScreen()
	box.Bind(menu, start.Bounds)
	t.Cleanup(widget.StopAllAnimations)
	return box, prov, menu, bar, start
}

func TestSearchBox_ModesAndWidthsFromMetrics(t *testing.T) {
	box, _, _, bar, start := searchFixture(t)
	if got := box.PreferredSize(image.Pt(1000, 40)).X; got != 344 {
		t.Errorf("ширина поля %d, ждали 344 из метрики search.width", got)
	}
	// Поле стоит сразу после кнопки «Пуск» на всю высоту панели.
	if b := box.Bounds(); b.Min.X < start.Bounds().Max.X || b.Dx() != 344 || b.Dy() != 40 {
		t.Errorf("поле %v, ждали 344×40 правее кнопки «Пуск» %v", b, start.Bounds())
	}

	box.SetMode(SearchModeIconOnly)
	if b := box.Bounds(); b.Dx() != 48 {
		t.Errorf("значок %v, ждали ширину 48 из search.icon.width (панель переложена сама)", b)
	}
	box.SetMode(SearchModeHidden)
	if b := box.Bounds(); !b.Empty() {
		t.Errorf("скрытая строка заняла место: %v", b)
	}
	if box.TabIndex() != -1 {
		t.Error("скрытая строка осталась остановкой Tab")
	}
	box.SetMode(SearchModeBox)
	if b := box.Bounds(); b.Dx() != 344 {
		t.Errorf("после возврата поле %v", b)
	}
	_ = bar

	// Узкая панель: поле сжимается до доступного места, а не вылезает.
	if got := box.PreferredSize(image.Pt(200, 40)).X; got != 200 {
		t.Errorf("при 200 доступных поле просит %d", got)
	}
}

func TestSearchBox_TypingEditsTextAndNotifies(t *testing.T) {
	box, prov, _, _, _ := searchFixture(t)
	var changes []string
	box.OnQueryChange = func(q string) { changes = append(changes, q) }
	key := func(c widget.KeyCode, r rune) { box.OnKeyEvent(widget.KeyEvent{Code: c, Rune: r, Pressed: true}) }

	key(widget.KeyE, 'e')
	key(widget.KeyD, 'd')
	key(widget.KeyG, 'г')
	if box.Text() != "edг" {
		t.Fatalf("текст %q", box.Text())
	}
	key(widget.KeyLeft, 0)
	key(widget.KeyBackspace, 0)
	if box.Text() != "eг" {
		t.Errorf("Backspace посреди: %q, ждали «eг»", box.Text())
	}
	key(widget.KeyHome, 0)
	key(widget.KeyDelete, 0)
	if box.Text() != "г" {
		t.Errorf("Delete в начале: %q, ждали «г»", box.Text())
	}
	key(widget.KeyEnd, 0)
	key(widget.KeyA, 'a')
	if box.Text() != "гa" {
		t.Errorf("набор в конце: %q", box.Text())
	}
	if len(changes) == 0 || changes[len(changes)-1] != "гa" {
		t.Errorf("OnQueryChange %v", changes)
	}
	if n := len(prov.Queries); n == 0 || prov.Queries[n-1] != "гa" {
		t.Errorf("поставщик получил %v, ждали последний «гa»", prov.Queries)
	}
	// Служебные клавиши в текст не попадают.
	before := box.Text()
	key(widget.KeyF5, 0)
	key(widget.KeyTab, 9)
	if box.Text() != before {
		t.Errorf("служебные клавиши изменили текст: %q", box.Text())
	}
}

// Набор в строке открывает меню «Пуск» с результатами; Enter открывает первый
// результат, стрелки ходят по списку.
func TestSearchBox_TypingOpensStartMenuWithResults(t *testing.T) {
	box, prov, menu, _, _ := searchFixture(t)
	box.OnKeyEvent(widget.KeyEvent{Code: widget.KeyC, Rune: 'c', Pressed: true})
	menu.Settle()
	if !menu.IsOpen() {
		t.Fatal("набор в строке не открыл меню")
	}
	if menu.Query() != "c" {
		t.Errorf("запрос меню %q", menu.Query())
	}
	rows, _ := menu.listRows()
	if len(rows) == 0 || rows[len(rows)-1].kind != rowResult {
		t.Fatalf("в меню нет результатов: %v", rows)
	}
	if got := len(prov.Queries); got != 1 {
		t.Errorf("поставщик получил запрос %d раз (%v), ждали один", got, prov.Queries)
	}

	box.OnKeyEvent(widget.KeyEvent{Code: widget.KeyDown, Pressed: true})
	box.OnKeyEvent(widget.KeyEvent{Code: widget.KeyDown, Pressed: true})
	box.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if len(prov.Activated) != 1 || prov.Activated[0] != "calc" {
		t.Errorf("открыто %v, ждали второй результат calc", prov.Activated)
	}
	if menu.IsOpen() {
		t.Error("меню осталось открытым после открытия результата")
	}
	if box.Text() != "" {
		t.Errorf("текст %q пережил закрытие меню", box.Text())
	}
}

func TestSearchBox_EscapeClosesMenuThenClearsText(t *testing.T) {
	box, _, menu, _, _ := searchFixture(t)
	box.OnKeyEvent(widget.KeyEvent{Code: widget.KeyE, Rune: 'e', Pressed: true})
	menu.Settle()
	box.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	if menu.IsOpen() {
		t.Error("Esc не закрыл меню")
	}
	if box.Text() != "" {
		t.Errorf("текст %q остался после закрытия меню", box.Text())
	}
	box2, _, _, _, _ := searchFixture(t)
	box2.SetText("abc")
	box2.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	if box2.Text() != "" {
		t.Errorf("Esc без меню не очистил поле: %q", box2.Text())
	}
}

// Нажатие на поле открывает меню в режиме поиска; значок режима IconOnly — тоже.
func TestSearchBox_ClickOpensStartMenu(t *testing.T) {
	box, _, menu, _, _ := searchFixture(t)
	x, y := pointIn(box.Bounds())
	box.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	if menu.IsOpen() {
		t.Error("меню открылось на нажатии, а не на отпускании")
	}
	box.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
	menu.Settle()
	if !menu.IsOpen() {
		t.Fatal("клик по строке не открыл меню")
	}
	// Фокус клавиатуры остаётся у строки: набор продолжается в ней.
	if menu.IsFocused() {
		t.Error("меню забрало фокус у строки поиска")
	}

	var activated int
	box.OnActivate = func() { activated++ }
	menu.Close()
	box.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	box.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
	if activated != 1 {
		t.Errorf("OnActivate вызван %d раз, ждали 1", activated)
	}
}

// Каретка встаёт туда, куда щёлкнули.
func TestSearchBox_ClickPlacesCaret(t *testing.T) {
	box, _, _, _, _ := searchFixture(t)
	box.SetText("abcdef")
	field, _ := box.fieldRect()
	// Щелчок в начало поля — каретка перед первой буквой.
	box.OnMouseButton(widget.MouseEvent{X: field.Min.X + 1, Y: field.Min.Y + 20, Button: widget.MouseLeft, Pressed: true})
	box.OnKeyEvent(widget.KeyEvent{Code: widget.KeyX, Rune: 'X', Pressed: true})
	if box.Text() != "Xabcdef" {
		t.Errorf("после щелчка в начало и набора: %q", box.Text())
	}
}

func TestSearchBox_PlaceholderFollowsLanguage(t *testing.T) {
	box, _, _, _, _ := searchFixture(t)
	box.SetBounds(image.Rect(40, 680, 384, 720))
	useLanguage(t, "RU")
	ctx := &recCtx{}
	box.Draw(ctx)
	if !containsText(ctx.texts, "Чтобы начать поиск, введите здесь запрос") {
		t.Fatalf("русская подсказка не нарисована: %v", ctx.texts)
	}
	widget.SetLanguage("EN")
	ctx = &recCtx{}
	box.Draw(ctx)
	if !containsText(ctx.texts, "Type here to search") {
		t.Errorf("английская подсказка не нарисована: %v", ctx.texts)
	}
	box.SetText("запрос")
	ctx = &recCtx{}
	box.Draw(ctx)
	if containsText(ctx.texts, "Type here to search") || !containsText(ctx.texts, "запрос") {
		t.Errorf("с текстом подсказка должна уйти: %v", ctx.texts)
	}
}

// Наведение и фокус меняют вид поля через стили темы, а не через литералы.
func TestSearchBox_HoverAndFocusStyles(t *testing.T) {
	box, _, _, _, _ := searchFixture(t)
	box.SetBounds(image.Rect(40, 680, 384, 720))
	fill := func() uint8 {
		ctx := &recCtx{}
		box.Draw(ctx)
		for _, f := range ctx.fills {
			if f.w == 344 && f.h == 40 {
				return f.col.R
			}
		}
		t.Fatal("подложка поля не нарисована")
		return 0
	}
	rest := fill()
	box.OnMouseMove(100, 700)
	fill() // кадр запускает переход цвета
	finishAnimations()
	hover := fill()
	if hover <= rest {
		t.Errorf("наведение не высветлило поле: покой %d, наведение %d", rest, hover)
	}
	box.SetFocused(true)
	if !box.IsFocused() {
		t.Error("SetFocused не сработал")
	}
	if r, _ := box.FocusRing(); r != box.Bounds() {
		t.Errorf("рамка фокуса %v, ждали границы %v", r, box.Bounds())
	}
}

// Набор буквы на меню, к которому привязана строка, переносит ввод в строку и
// отдаёт ей фокус.
func TestSearchBox_TypingOnMenuMovesInputToBox(t *testing.T) {
	box, _, menu, _, start := searchFixture(t)
	menu.Open(start.Bounds())
	menu.Settle()
	menu.SetFocused(true)

	menu.OnKeyEvent(widget.KeyEvent{Code: widget.KeyP, Rune: 'p', Pressed: true})
	if box.Text() != "p" {
		t.Errorf("текст строки %q, ждали «p»", box.Text())
	}
	if !box.IsFocused() || menu.IsFocused() {
		t.Errorf("фокус: строка %v, меню %v — ждали переход к строке", box.IsFocused(), menu.IsFocused())
	}
	if menu.Query() != "p" {
		t.Errorf("запрос меню %q", menu.Query())
	}
}

// Без привязанной строки меню показывает запрос само.
func TestSearchBox_UnboundMenuShowsInlineQuery(t *testing.T) {
	m, _ := tiledMenu(t)
	m.OnKeyEvent(widget.KeyEvent{Code: widget.KeyX, Rune: 'x', Pressed: true})
	ctx := &recCtx{}
	m.drawContent(ctx, m.contentRect())
	if !containsText(ctx.texts, "x") {
		t.Errorf("запрос не показан в списке: %v", ctx.texts)
	}
	if !m.inlineQueryBar() {
		t.Error("встроенная строка запроса не включилась")
	}
}

// Нажатие на привязанную строку поиска при открытом меню не закрывает его (иначе
// оно закрывалось бы на нажатии и открывалось заново на отпускании); нажатие
// куда-либо ещё — закрывает.
func TestSearchBox_ClickOnBoxDoesNotDismissMenu(t *testing.T) {
	box, _, menu, _, start := searchFixture(t)
	menu.Open(start.Bounds())
	menu.Settle()
	x, y := pointIn(box.Bounds())
	menu.DismissAt(x, y)
	if !menu.IsOpen() {
		t.Error("клик по строке поиска закрыл меню")
	}
	menu.DismissAt(tileScreenW-5, 5)
	if menu.IsOpen() {
		t.Error("клик мимо не закрыл меню")
	}
}

// Плоское меню (Windows 11) тоже отписывается от темы при закрытии.
func TestStartMenu_FlatMenuReleasesSubscriptionsOnClose(t *testing.T) {
	tm := managerFor(t, theme.ProfileWindows11)
	base := tm.ObserverCount()
	m := NewStartMenu(tm, NewStaticAppCatalog(tileApps()...))
	m.Screen = tileScreen()
	m.Open(tileAnchor())
	if tm.ObserverCount() <= base {
		t.Error("открытое меню не подписалось на тему")
	}
	m.Close()
	m.Settle()
	if got := tm.ObserverCount(); got != base {
		t.Errorf("после закрытия наблюдателей %d, было %d", got, base)
	}
	widget.StopAllAnimations()
}

// Подписи без видимого текста — подсказка кнопки-значка поиска и гамбургера —
// берутся из строк интерфейса и следуют за языком.
func TestStartMenu_UnlabelledControlsHaveLocalisedLabels(t *testing.T) {
	box, _, menu, _, _ := searchFixture(t)
	useLanguage(t, "RU")
	if box.GetToolTip() != "" {
		t.Errorf("у поля поиска подсказка %q, ждали пустую (есть заполнитель)", box.GetToolTip())
	}
	box.SetMode(SearchModeIconOnly)
	if got := box.GetToolTip(); got != "Поиск" {
		t.Errorf("подсказка значка %q, ждали «Поиск»", got)
	}
	widget.SetLanguage("EN")
	if got := box.GetToolTip(); got != "Search" {
		t.Errorf("подсказка значка %q, ждали «Search»", got)
	}
	if got := menu.SidebarToggleLabel(); got != "Expand" {
		t.Errorf("гамбургер свёрнутой панели %q, ждали Expand", got)
	}
	menu.SetSidebarExpanded(true)
	if got := menu.SidebarToggleLabel(); got != "Collapse" {
		t.Errorf("гамбургер развёрнутой панели %q, ждали Collapse", got)
	}
}
