package widget

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

// Запросы Go.Git к меню: ширина по замеру, отметки, правка собранного меню и
// адресация пунктов на любой глубине.

// ─── GG-21: ширина пункта считалась по байтам ──────────────────────────────

// Кириллический пункт получал вдвое большую ширину, чем нужно: len в Go
// возвращает длину в байтах. «Репозиторий» — 11 символов, 22 байта, то есть
// 176 точек вместо примерно 90, и подсветка при наведении выходила шире
// текста.
func TestMenuBar_ItemWidthMatchesTheText(t *testing.T) {
	mb := NewMenuBar()
	mb.AddMenu("Репозиторий")
	mb.AddMenu("Repositories") // столько же символов, вдвое меньше байт
	mb.SetBounds(image.Rect(0, 0, 800, 28))

	cyr, lat := mb.itemRects[0], mb.itemRects[1]
	// Ширины двух подписей одинаковой ДЛИНЫ должны быть сопоставимы. Точного
	// равенства нет — буквы разной ширины, — но двукратной разницы, которую
	// давал счёт по байтам, быть не должно.
	if cyr.Dx() > lat.Dx()*3/2 {
		t.Errorf("кириллический пункт шире латинского той же длины: %d против %d",
			cyr.Dx(), lat.Dx())
	}

	// И ширина зависит от текста, а не от числа байт: длинная подпись шире.
	mb2 := NewMenuBar()
	mb2.AddMenu("Вид")
	mb2.AddMenu("Репозиторий")
	mb2.SetBounds(image.Rect(0, 0, 800, 28))
	if mb2.itemRects[1].Dx() <= mb2.itemRects[0].Dx() {
		t.Errorf("длинная подпись не шире короткой: %d против %d",
			mb2.itemRects[1].Dx(), mb2.itemRects[0].Dx())
	}
}

// ─── GG-5: отметка у пункта меню ───────────────────────────────────────────

// checkCtx — контекст, считающий точки: галочка рисуется фигурой, а не
// символом шрифта, поэтому в тексте её не найти.
type checkCtx struct {
	DrawContext
	pixels int
	texts  []string
}

func (c *checkCtx) SetPixel(int, int, color.RGBA) { c.pixels++ }
func (c *checkCtx) DrawText(text string, _, _ int, _ color.RGBA) {
	c.texts = append(c.texts, text)
}
func (c *checkCtx) FillRect(int, int, int, int, color.RGBA)          {}
func (c *checkCtx) FillRectAlpha(int, int, int, int, color.RGBA)     {}
func (c *checkCtx) DrawHLine(int, int, int, color.RGBA)              {}
func (c *checkCtx) DrawVLine(int, int, int, color.RGBA)              {}
func (c *checkCtx) DrawBorder(int, int, int, int, color.RGBA)        {}
func (c *checkCtx) FillRoundRect(int, int, int, int, int, color.RGBA) {}

func checkedMenu(t *testing.T, checked bool) *PopupMenu {
	t.Helper()
	pm := NewPopupMenu()
	pm.SetItems([]MenuItem{
		{Text: "Тёмная", Checkable: true, Checked: checked, RadioGroup: "theme"},
		{Text: "Светлая", Checkable: true, RadioGroup: "theme"},
	})
	pm.Show(10, 10)
	return pm
}

func TestPopupMenu_CheckedItemDrawsAMark(t *testing.T) {
	plain := &checkCtx{}
	checkedMenu(t, false).DrawOverlay(plain)

	marked := &checkCtx{}
	checkedMenu(t, true).DrawOverlay(marked)

	if marked.pixels <= plain.pixels {
		t.Errorf("отмеченный пункт нарисовал %d точек против %d у неотмеченного — "+
			"галочки не видно", marked.pixels, plain.pixels)
	}
}

// Место под отметку отводится всему меню сразу: иначе подписи разъезжались бы
// в тот момент, когда пользователь ставит отметку.
func TestPopupMenu_CheckGutterDoesNotShiftOnToggle(t *testing.T) {
	pm := checkedMenu(t, false)
	before := pm.checkGutter()
	pm.SetItemChecked(0, true)
	if after := pm.checkGutter(); after != before {
		t.Errorf("поле под отметку изменилось с %d на %d — подписи дёрнутся", before, after)
	}
	if before == 0 {
		t.Error("у меню с отмечаемым пунктом нет поля под отметку")
	}

	// А у меню без отмечаемых пунктов поля нет — лишнего отступа быть не должно.
	plain := NewPopupMenu()
	plain.SetItems([]MenuItem{{Text: "Открыть"}, {Text: "Закрыть"}})
	if got := plain.checkGutter(); got != 0 {
		t.Errorf("у меню без отметок поле шириной %d", got)
	}
}

// Пункты одной группы ведут себя как переключатель.
func TestPopupMenu_RadioGroupIsExclusive(t *testing.T) {
	pm := NewPopupMenu()
	pm.SetItems([]MenuItem{
		{Text: "Тёмная", Checkable: true, Checked: true, RadioGroup: "theme"},
		{Text: "Светлая", Checkable: true, RadioGroup: "theme"},
		{Text: "Панель инструментов", Checkable: true, Checked: true}, // без группы
	})

	pm.SetItemChecked(1, true)
	items := pm.Items()
	if items[0].Checked {
		t.Error("отметка осталась на прежнем пункте группы — это не переключатель")
	}
	if !items[1].Checked {
		t.Error("новый пункт группы не отмечен")
	}
	if !items[2].Checked {
		t.Error("пункт БЕЗ группы потерял отметку — он от группы не зависит")
	}
}

// Отметка на пункте, не объявленном отмечаемым, всё равно видна: молча
// ничего не сделать — худший исход.
func TestPopupMenu_CheckingMakesItemCheckable(t *testing.T) {
	pm := NewPopupMenu()
	pm.SetItems([]MenuItem{{Text: "Показывать панель"}})
	pm.SetItemChecked(0, true)

	if got := pm.Items()[0]; !got.Checked || !got.Checkable {
		t.Errorf("после SetItemChecked пункт %+v — отметка не покажется", got)
	}
}

// ─── GG-4: адресация пунктов на любой глубине ──────────────────────────────

func threeLevelBar() *MenuBar {
	mb := NewMenuBar()
	mb.AddMenu("Вид",
		MenuItem{Text: "Тема", SubItems: []MenuItem{
			{Text: "Тёмная", Checkable: true, RadioGroup: "theme"},
			{Text: "Светлая", Checkable: true, RadioGroup: "theme"},
		}},
		MenuItem{Text: "Панель инструментов", Checkable: true},
	)
	mb.SetBounds(image.Rect(0, 0, 800, 28))
	return mb
}

func TestMenuBar_SetItemTextReachesThirdLevel(t *testing.T) {
	mb := threeLevelBar()

	mb.SetItemText("Dark", 0, 0, 0)
	got, ok := mb.ItemAt(0, 0, 0)
	if !ok {
		t.Fatal("пункт третьего уровня не найден по пути")
	}
	if got.Text != "Dark" {
		t.Errorf("надпись третьего уровня %q, ждали «Dark»", got.Text)
	}

	// Второй уровень и полоса — тем же методом.
	mb.SetItemText("Toolbar", 0, 1)
	if got, _ := mb.ItemAt(0, 1); got.Text != "Toolbar" {
		t.Errorf("надпись второго уровня %q", got.Text)
	}
	mb.SetItemText("View", 0)
	if items := mb.Items(); items[0].Text != "View" {
		t.Errorf("надпись полосы %q", items[0].Text)
	}
}

// Путь мимо дерева ничего не портит и не паникует.
func TestMenuBar_BadPathIsHarmless(t *testing.T) {
	mb := threeLevelBar()
	for _, path := range [][]int{nil, {}, {9}, {0, 9}, {0, 0, 9}, {-1}, {0, -1}} {
		mb.SetItemText("хлам", path...)
		mb.SetItemChecked(true, path...)
		if _, ok := mb.ItemAt(path...); ok {
			t.Errorf("путь %v найден, хотя такого пункта нет", path)
		}
	}
	if got, _ := mb.ItemAt(0, 0, 0); got.Text != "Тёмная" {
		t.Errorf("дерево испорчено неверным путём: %q", got.Text)
	}
}

// Отметка по пути работает вместе с группой.
func TestMenuBar_SetItemCheckedWithRadioGroup(t *testing.T) {
	mb := threeLevelBar()

	mb.SetItemChecked(true, 0, 0, 0)
	mb.SetItemChecked(true, 0, 0, 1)

	first, _ := mb.ItemAt(0, 0, 0)
	second, _ := mb.ItemAt(0, 0, 1)
	if first.Checked {
		t.Error("отметка осталась на первом пункте группы")
	}
	if !second.Checked {
		t.Error("второй пункт группы не отмечен")
	}
}

// ─── GG-6: правка состава собранного меню ──────────────────────────────────

func TestMenuBar_MenusCanBeReplaced(t *testing.T) {
	mb := threeLevelBar()

	mb.SetMenuItems(0,
		MenuItem{Text: "Русский"},
		MenuItem{Text: "English"},
		MenuItem{Text: "Deutsch"}, // язык, положенный пользователем
	)
	got, ok := mb.ItemAt(0, 2)
	if !ok || got.Text != "Deutsch" {
		t.Errorf("подменю не заменено: %+v (найдено=%v)", got, ok)
	}

	mb.InsertMenu(0, MenuBarItem{Text: "Файл"})
	if items := mb.Items(); len(items) != 2 || items[0].Text != "Файл" {
		t.Errorf("после вставки полоса: %+v", items)
	}

	mb.RemoveMenu(0)
	if items := mb.Items(); len(items) != 1 || items[0].Text != "Вид" {
		t.Errorf("после удаления полоса: %+v", items)
	}

	mb.ClearMenus()
	if got := mb.MenuCount(); got != 0 {
		t.Errorf("после ClearMenus осталось %d пунктов", got)
	}
}

// Вставка за пределами списка кладёт в конец — так удобнее строить в цикле.
func TestMenuBar_InsertBeyondEndAppends(t *testing.T) {
	mb := NewMenuBar()
	mb.AddMenu("Файл")
	mb.InsertMenu(99, MenuBarItem{Text: "Справка"})
	mb.InsertMenu(-5, MenuBarItem{Text: "Первый"})

	items := mb.Items()
	if len(items) != 3 || items[0].Text != "Первый" || items[2].Text != "Справка" {
		t.Errorf("порядок после вставок: %+v", items)
	}
}

// Правка состава закрывает раскрытое подменю: открытое показывает СНИМОК
// пунктов, и оставить его поверх нового состава значит показывать то, чего
// уже нет, и звать его обработчики.
func TestMenuBar_EditingClosesTheOpenSubmenu(t *testing.T) {
	mb := threeLevelBar()
	mb.setActiveIdx(0)
	mb.popup.SetItems(mb.Items()[0].Items)
	mb.popup.Show(0, 28)
	if !mb.popup.IsOpen() {
		t.Fatal("подменю не открылось — тест ничего не проверяет")
	}

	mb.SetMenuItems(0, MenuItem{Text: "Другое"})

	if mb.popup.IsOpen() {
		t.Error("правка состава оставила открытым подменю со старыми пунктами")
	}
}

// ─── XAML ──────────────────────────────────────────────────────────────────

func TestXAML_MenuItemChecked(t *testing.T) {
	const src = `<Window Width="400" Height="300">
  <Menu Name="bar">
    <MenuItem Header="Вид">
      <MenuItem Header="Тёмная" IsChecked="True" GroupName="theme"/>
      <MenuItem Header="Светлая" GroupName="theme"/>
      <MenuItem Header="Панель" IsCheckable="True"/>
      <MenuItem Header="Обычный"/>
    </MenuItem>
  </Menu>
</Window>`

	_, reg, err := LoadUIFromXAML([]byte(src))
	if err != nil {
		t.Fatalf("разбор XAML: %v", err)
	}
	mb, ok := reg["bar"].(*MenuBar)
	if !ok {
		t.Fatalf("в разметке нет полосы меню: %T", reg["bar"])
	}

	dark, ok := mb.ItemAt(0, 0)
	if !ok {
		t.Fatal("пункт «Тёмная» не найден")
	}
	if !dark.Checked || !dark.Checkable || dark.RadioGroup != "theme" {
		t.Errorf("IsChecked/GroupName не дошли из разметки: %+v", dark)
	}

	panel, _ := mb.ItemAt(0, 2)
	if !panel.Checkable || panel.Checked {
		t.Errorf("IsCheckable без IsChecked дал %+v", panel)
	}

	plain, _ := mb.ItemAt(0, 3)
	if plain.Checkable || plain.Checked {
		t.Errorf("обычный пункт объявлен отмечаемым: %+v", plain)
	}
}

// ─── GG-2: функциональные клавиши в KeyBinding ─────────────────────────────

func TestParseKeyName_FunctionAndFriends(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want KeyCode
	}{
		{"F1", KeyF1}, {"f5", KeyF5}, {"F12", KeyF12}, {"F24", KeyF24},
		{"Esc", KeyEscape}, {"Escape", KeyEscape},
		{"Del", KeyDelete}, {"Ins", KeyInsert},
		{"Home", KeyHome}, {"End", KeyEnd},
		{"PgUp", KeyPageUp}, {"PageDown", KeyPageDown},
		{"Enter", KeyEnter}, {"Tab", KeyTab}, {"Space", KeySpace},
		{"A", KeyA}, {"z", KeyZ},
		{"5", KeyCode('5')},
	} {
		if got := parseKeyName(tc.in); got != tc.want {
			t.Errorf("parseKeyName(%q) = %d, ждали %d", tc.in, got, tc.want)
		}
	}
}

// «f» с мусором дальше не должно становиться F-чем-нибудь.
//
// Одиночная «f» в список не входит: это законная буквенная клавиша, и
// Key="F" обязан означать именно её, а не начало F-ряда.
func TestParseKeyName_RejectsNonsense(t *testing.T) {
	if got := parseKeyName("f"); got != KeyCode('F') {
		t.Errorf("parseKeyName(\"f\") = %d, ждали буквенную клавишу F", got)
	}
	for _, in := range []string{"", "f0", "f25", "f100", "file", "fx", "f1x", " "} {
		if got := parseKeyName(in); got != KeyUnknown {
			t.Errorf("parseKeyName(%q) = %d, ждали KeyUnknown", in, got)
		}
	}
}

// Горячая клавиша из разметки доходит до привязки — ради этого запрос и подан.
func TestXAML_KeyBindingUnderstandsFunctionKeys(t *testing.T) {
	const src = `<Window Width="400" Height="300">
  <Window.InputBindings>
    <KeyBinding Key="F5" Command="Refresh"/>
    <KeyBinding Key="F12" Modifiers="Ctrl" Command="Tools"/>
  </Window.InputBindings>
  <StackPanel/>
</Window>`

	root, _, err := LoadUIFromXAML([]byte(src))
	if err != nil {
		t.Fatalf("разбор XAML: %v", err)
	}
	win, ok := root.(*Window)
	if !ok {
		t.Fatalf("корень разметки — %T, ждали окно", root)
	}
	if len(win.InputBindings) != 2 {
		t.Fatalf("разобрано %d привязок, ждали две", len(win.InputBindings))
	}
	if got := win.InputBindings[0].Key; got != KeyF5 {
		t.Errorf("Key=\"F5\" разобран как %d, ждали %d", got, KeyF5)
	}
	if got := win.InputBindings[1].Key; got != KeyF12 {
		t.Errorf("Key=\"F12\" разобран как %d, ждали %d", got, KeyF12)
	}
	if win.InputBindings[1].Mods&ModCtrl == 0 {
		t.Error("модификатор Ctrl потерян")
	}
}

// ─── GG-3: скрытый ребёнок не держит строку Auto ───────────────────────────

func TestGrid_HiddenChildDoesNotHoldAnAutoRow(t *testing.T) {
	g := NewGrid()
	g.RowDefs = []GridDefinition{
		{Mode: GridSizeAuto},
		{Mode: GridSizeStar, Value: 1},
	}
	g.ColDefs = []GridDefinition{{Mode: GridSizeStar, Value: 1}}

	bar := NewPanel(color.RGBA{R: 40, G: 40, B: 40, A: 255})
	bar.ShowHeader = false
	bar.SetBounds(image.Rect(0, 0, 100, 40))
	bar.SetGridProps(0, 0, 1, 1)
	g.AddChild(bar)

	body := NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	body.ShowHeader = false
	body.SetGridProps(1, 0, 1, 1)
	g.AddChild(body)

	g.SetBounds(image.Rect(0, 0, 300, 300))
	withBar := g.rowOffsets[1] - g.rowOffsets[0]
	if withBar <= 0 {
		t.Fatalf("строка Auto с видимым содержимым нулевая: %v", g.rowOffsets)
	}

	bar.SetVisible(false)
	g.SetBounds(image.Rect(0, 0, 300, 300))
	if got := g.rowOffsets[1] - g.rowOffsets[0]; got != 0 {
		t.Errorf("строка Auto держит %d точек под скрытым виджетом (было %d)", got, withBar)
	}
}

// Строка ФИКСИРОВАННОЙ высоты не схлопывается — и это верно: её размер задан
// числом, а не содержимым, ровно как в WPF. Проверяется, чтобы правка выше не
// расползлась на этот случай.
func TestGrid_FixedRowKeepsItsHeightWhenChildHides(t *testing.T) {
	g := NewGrid()
	g.RowDefs = []GridDefinition{
		{Mode: GridSizePixel, Value: 40},
		{Mode: GridSizeStar, Value: 1},
	}
	g.ColDefs = []GridDefinition{{Mode: GridSizeStar, Value: 1}}

	bar := NewPanel(color.RGBA{R: 40, G: 40, B: 40, A: 255})
	bar.ShowHeader = false
	bar.SetGridProps(0, 0, 1, 1)
	g.AddChild(bar)
	g.SetBounds(image.Rect(0, 0, 300, 300))

	bar.SetVisible(false)
	g.SetBounds(image.Rect(0, 0, 300, 300))
	if got := g.rowOffsets[1] - g.rowOffsets[0]; got != 40 {
		t.Errorf("строка фиксированной высоты стала %d вместо 40", got)
	}
}

// Вспомогательное: имена клавиш разбираются без учёта регистра.
func TestParseKeyName_CaseInsensitive(t *testing.T) {
	for _, in := range []string{"F5", "f5", "F5 ", " f5"} {
		if got := parseKeyName(in); got != KeyF5 {
			t.Errorf("parseKeyName(%q) = %d", strings.TrimSpace(in), got)
		}
	}
}
