package desktop

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Переход по буквам: сетка всех букв на месте списка приложений.

// letterVP — окно списка меню.
func letterVP(m *StartMenu) image.Rectangle {
	return m.listViewport(m.startGeometry(m.contentRect()))
}

func gridCellOf(t *testing.T, m *StartMenu, letter string) gridCell {
	t.Helper()
	c, ok := m.gridCellByLetter(letterVP(m), letter)
	if !ok {
		t.Fatalf("в сетке нет буквы %q", letter)
	}
	return c
}

// openGridByClick открывает сетку щелчком по заголовку буквы.
func openGridByClick(t *testing.T, m *StartMenu, letter string) {
	t.Helper()
	r := m.keyRect(prefRow + "l:" + letter)
	if r.Empty() {
		t.Fatalf("заголовок %q не виден", letter)
	}
	clickMenu(m, r)
	if !m.LetterGridOpen() {
		t.Fatalf("щелчок по заголовку %q не открыл сетку", letter)
	}
}

func TestLetters_AlphabetBuiltFromPresentGroups(t *testing.T) {
	lat := startAlphabet([]string{"A", "C"})
	if lat[0] != "#" || len(lat) != 1+26 || lat[1] != "A" || lat[26] != "Z" {
		t.Errorf("латинский алфавит: %v", lat)
	}
	cyr := startAlphabet([]string{"A", "Б", "Я"})
	if len(cyr) != 1+26+33 {
		t.Errorf("с кириллицей букв %d, ждали 60", len(cyr))
	}
	idx := func(l string) int {
		for i, x := range cyr {
			if x == l {
				return i
			}
		}
		return -1
	}
	if !(idx("Е") < idx("Ё") && idx("Ё") < idx("Ж")) {
		t.Error("Ё должна стоять между Е и Ж")
	}
	// Буква вне известных алфавитов (иероглиф) попадает в конец.
	other := startAlphabet([]string{"A", "漢"})
	if other[len(other)-1] != "漢" {
		t.Errorf("прочая буква не добавлена: %v", other[len(other)-3:])
	}
}

// Щелчок по заголовку буквы открывает сетку на месте списка; доступны только
// буквы с группами.
func TestLetters_ClickHeaderOpensGrid(t *testing.T) {
	m, _ := tiledMenu(t)
	openGridByClick(t, m, "D")

	if c := gridCellOf(t, m, "D"); !c.active {
		t.Error("D есть в списке, а в сетке недоступна")
	}
	for _, l := range []string{"#", "U", "Z"} {
		if gridCellOf(t, m, l).active {
			t.Errorf("буква %q без группы должна быть недоступна", l)
		}
	}
	// Выбор стоит на букве, с которой открыли.
	if got := m.v.sel[areaList]; got != prefGrid+"D" {
		t.Errorf("выбор %q, ждали g:D", got)
	}
	// Сетка заняла место списка: ни одна ячейка не вылезла за окно.
	vp := letterVP(m)
	for _, c := range m.gridCells(vp) {
		if !c.rect.In(vp) {
			t.Errorf("ячейка %q %v вне окна списка %v", c.letter, c.rect, vp)
		}
	}
}

// Выбор доступной буквы прокручивает список к её группе и закрывает сетку;
// недоступная буква не реагирует.
func TestLetters_PickJumpsAndClosesGrid(t *testing.T) {
	m, cat := tiledMenu(t)
	openGridByClick(t, m, "D")

	clickMenu(m, gridCellOf(t, m, "Z").rect) // недоступная
	if !m.LetterGridOpen() {
		t.Fatal("щелчок по недоступной букве закрыл сетку")
	}
	if m.v.listScroll != 0 {
		t.Errorf("щелчок по недоступной букве прокрутил список: %d", m.v.listScroll)
	}

	clickMenu(m, gridCellOf(t, m, "H").rect)
	if m.LetterGridOpen() {
		t.Error("после выбора буквы сетка осталась открытой")
	}
	rows, _ := m.listRows()
	h := rows[rowIndex(rows, "l:H")]
	if m.v.listScroll != h.y {
		t.Errorf("прокрутка %d, ждали верх группы H (%d)", m.v.listScroll, h.y)
	}
	if got := m.v.sel[areaList]; got != prefRow+"l:H" {
		t.Errorf("выбор %q, ждали заголовок H", got)
	}
	if len(cat.Launched) != 0 {
		t.Errorf("переход по буквам что-то запустил: %v", cat.Launched)
	}
	if !m.IsOpen() {
		t.Error("меню закрылось")
	}
}

// Группа в конце списка не может встать наверх (прокрутка упирается в конец):
// список остаётся на пределе прокрутки.
func TestLetters_JumpToLastGroupClampsScroll(t *testing.T) {
	m, _ := tiledMenu(t)
	openGridByClick(t, m, "A")
	if !m.JumpToLetter("T") {
		t.Fatal("T не найдена")
	}
	_, contentH := m.listRows()
	if want := contentH - letterVP(m).Dy(); m.v.listScroll != want {
		t.Errorf("прокрутка %d, ждали предел %d", m.v.listScroll, want)
	}
	if m.JumpToLetter("Z") {
		t.Error("переход к букве без группы вернул true")
	}
}

func TestLetters_EscClosesGridThenMenu(t *testing.T) {
	m, _ := tiledMenu(t)
	openGridByClick(t, m, "C")
	pressKey(m, widget.KeyEscape)
	if m.LetterGridOpen() {
		t.Error("Esc не закрыл сетку")
	}
	if !m.IsOpen() {
		t.Fatal("первый Esc закрыл ещё и меню")
	}
	if got := m.v.sel[areaList]; got != prefRow+"l:C" {
		t.Errorf("после Esc выбор %q, ждали возврата на заголовок C", got)
	}
	if m.v.listScroll != 0 {
		t.Error("Esc прокрутил список")
	}
	pressKey(m, widget.KeyEscape)
	if m.IsOpen() {
		t.Error("второй Esc не закрыл меню")
	}
}

// С клавиатуры: стрелки доходят до заголовка буквы, Enter открывает сетку,
// стрелки ходят по её геометрии, Enter выбирает.
func TestLetters_KeyboardOpensAndNavigatesGrid(t *testing.T) {
	m, _ := tiledMenu(t)
	pressKey(m, widget.KeyDown) // первое приложение
	pressKey(m, widget.KeyUp)   // заголовок буквы над ним
	if got := m.v.sel[areaList]; got != prefRow+"l:A" {
		t.Fatalf("Вверх с первого приложения: %q, ждали заголовок A", got)
	}
	pressKey(m, widget.KeyEnter)
	if !m.LetterGridOpen() {
		t.Fatal("Enter на заголовке не открыл сетку")
	}
	sel := func() string { return strings.TrimPrefix(m.v.sel[areaList], prefGrid) }
	if sel() != "A" {
		t.Fatalf("в сетке выбрана %q, ждали A", sel())
	}
	pressKey(m, widget.KeyRight)
	if sel() != "B" {
		t.Errorf("Вправо: %q, ждали B", sel())
	}
	pressKey(m, widget.KeyDown) // четыре столбца: под B лежит F
	if sel() != "F" {
		t.Errorf("Вниз: %q, ждали F", sel())
	}
	pressKey(m, widget.KeyLeft)
	if sel() != "E" {
		t.Errorf("Влево: %q, ждали E", sel())
	}
	pressKey(m, widget.KeyUp)
	if sel() != "A" {
		t.Errorf("Вверх: %q, ждали A", sel())
	}
	pressKey(m, widget.KeyEnd)
	if sel() != "T" {
		t.Errorf("End: %q, ждали последнюю доступную T", sel())
	}
	pressKey(m, widget.KeyHome)
	if sel() != "A" {
		t.Errorf("Home: %q, ждали первую доступную A", sel())
	}
	// Недоступные буквы стрелки перешагивают: вверх от A идти некуда.
	pressKey(m, widget.KeyUp)
	if sel() != "A" {
		t.Errorf("Вверх от A: %q, ждали A (над ней только «#» без группы)", sel())
	}
	pressKey(m, widget.KeyEnd)
	pressKey(m, widget.KeySpace) // Пробел тоже выбирает
	if m.LetterGridOpen() {
		t.Error("Пробел не выбрал букву")
	}
	rows, _ := m.listRows()
	if m.v.sel[areaList] != prefRow+"l:T" || m.v.listScroll == 0 {
		t.Errorf("выбор %q, прокрутка %d: ждали переход к T", m.v.sel[areaList], m.v.listScroll)
	}
	_ = rows
}

// Home и End в списке по-прежнему встают на приложения, а не на заголовки букв.
func TestLetters_HomeEndInListStayOnApps(t *testing.T) {
	m, _ := tiledMenu(t)
	pressKey(m, widget.KeyEnd)
	if got := m.v.sel[areaList]; strings.HasPrefix(got, prefRow+"l:") {
		t.Errorf("End встал на заголовок %q", got)
	}
	pressKey(m, widget.KeyHome)
	if got := m.v.sel[areaList]; strings.HasPrefix(got, prefRow+"l:") {
		t.Errorf("Home встал на заголовок %q", got)
	}
}

// Наведение на ячейку перерисовывает только её, а недоступная букву не
// подсвечивает.
func TestLetters_HoverInvalidatesOnlyItsCell(t *testing.T) {
	m, _ := tiledMenu(t)
	openGridByClick(t, m, "A")
	var rects []image.Rectangle
	fulls := 0
	h := widget.RegisterUINotifierWake(func() { fulls++ }, func(r image.Rectangle) { rects = append(rects, r) }, func() {})
	defer widget.UnregisterUINotifier(h)

	x, y := pointIn(gridCellOf(t, m, "C").rect)
	m.OnMouseMove(x, y) // уход с заголовка, с которого открывали, — его строка
	rects = nil
	x, y = pointIn(gridCellOf(t, m, "D").rect)
	m.OnMouseMove(x, y)
	if fulls != 0 {
		t.Errorf("наведение вызвало %d полных перерисовок", fulls)
	}
	if len(rects) == 0 {
		t.Fatal("наведение ничего не заявило")
	}
	cell := gridCellOf(t, m, "C").rect
	for _, r := range rects {
		if r.Dx() > cell.Dx()+2 || r.Dy() > cell.Dy()+2 {
			t.Errorf("заявлена область %v — больше ячейки %v", r, cell)
		}
	}
	rects = nil
	x, y = pointIn(gridCellOf(t, m, "Z").rect)
	m.OnMouseMove(x, y)
	if m.v.hover != "" {
		t.Errorf("наведение на недоступную букву %q", m.v.hover)
	}
}

// Колесо над сеткой её не прокручивает и не сдвигает список под ней; нажатие на
// плитку закрывает сетку.
func TestLetters_WheelIgnoredAndPressElsewhereCloses(t *testing.T) {
	m, _ := tiledMenu(t)
	openGridByClick(t, m, "A")
	pt := image.Pt(letterVP(m).Min.X+50, letterVP(m).Min.Y+100)
	m.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseWheelDown, Pressed: true})
	if m.v.listScroll != 0 || !m.LetterGridOpen() {
		t.Errorf("колесо над сеткой: прокрутка %d, открыта %v", m.v.listScroll, m.LetterGridOpen())
	}
	x, y := pointIn(m.tileRectAbs("t0"))
	m.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	if m.LetterGridOpen() {
		t.Error("нажатие на плитку не закрыло сетку")
	}
	m.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
}

// Набор текста переводит в поиск: у результатов нет букв, сетка закрывается.
func TestLetters_TypingClosesGrid(t *testing.T) {
	m, _ := tiledMenu(t)
	openGridByClick(t, m, "A")
	m.OnKeyEvent(widget.KeyEvent{Code: widget.KeyUnknown, Rune: 'x', Pressed: true})
	if m.LetterGridOpen() {
		t.Error("набор текста не закрыл сетку")
	}
	if m.Query() != "x" {
		t.Errorf("запрос %q, ждали x", m.Query())
	}
	if m.OpenLetterGrid() {
		t.Error("во время поиска сетка открылась")
	}
}

// Закрытие и переоткрытие меню сбрасывает сетку.
func TestLetters_ResetOnReopen(t *testing.T) {
	m, _ := tiledMenu(t)
	openGridByClick(t, m, "A")
	a := m.Anchor
	m.Close()
	m.Open(a)
	m.Settle()
	if m.LetterGridOpen() {
		t.Error("сетка пережила закрытие меню")
	}
}

// Сетка появляется по анимации темы, а не рывком; по Tab из списка закрывается.
func TestLetters_GridFadesIn(t *testing.T) {
	m, _ := tiledMenu(t)
	openGridByClick(t, m, "A")
	if !m.v.gridFade.Animating() {
		t.Fatal("сетка появилась без перехода")
	}
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(50 * time.Millisecond))
	if v := m.v.gridFade.Value(); v <= 0 || v >= 1 {
		t.Errorf("посреди перехода прозрачность %v, ждали между 0 и 1", v)
	}
	widget.StepAnimations(t0.Add(time.Second))
	if v := m.v.gridFade.Value(); v != 1 {
		t.Errorf("после перехода %v, ждали 1", v)
	}
	pressKey(m, widget.KeyTab)
	if m.LetterGridOpen() {
		t.Error("Tab не закрыл сетку")
	}
}

// Кадр с сеткой: буквы нарисованы вместо списка (приложений в кадре нет).
func TestLetters_GridDrawnInsteadOfList(t *testing.T) {
	m, _ := tiledMenu(t)
	ctx := &recCtx{}
	m.drawContent(ctx, m.contentRect())
	if !containsPrefixText(ctx.texts, "A приложение") {
		t.Fatal("в обычном кадре нет приложений")
	}
	openGridByClick(t, m, "A")
	finishAnimations()
	ctx = &recCtx{}
	m.drawContent(ctx, m.contentRect())
	if containsPrefixText(ctx.texts, "A приложение") {
		t.Error("при открытой сетке список всё ещё нарисован")
	}
	letters := map[string]bool{}
	for _, tx := range ctx.texts {
		letters[tx.text] = true
	}
	for _, l := range []string{"#", "A", "Z"} {
		if !letters[l] {
			t.Errorf("буква %q не нарисована", l)
		}
	}
}

// Другие темы не знают сетки: в плоском меню заголовков-букв нет вовсе, а
// OpenLetterGrid ничего не делает.
func TestLetters_NotInFlatMenu(t *testing.T) {
	defer widget.StopAllAnimations()
	m := NewStartMenu(managerFor(t, theme.ProfileWindows2000), NewStaticAppCatalog(tileApps()...))
	m.Screen = tileScreen()
	m.Open(tileAnchor())
	m.Settle()
	if m.OpenLetterGrid() || m.LetterGridOpen() {
		t.Error("плоское меню открыло сетку букв")
	}
}
