// Package showcasedemo — демонстрации виджетов v3.27–v3.29, общие для двух
// витрин: оконной (cmd/showcase) и браузерной (cmd/webshowcase).
//
// Обе витрины грузят ОДНУ разметку (assets/ui/showcase.xaml), а новые вкладки
// — «Форматированный текст», «Код и прокрутка», «Меню» и нижние ряды вкладок
// «Деревья и таблицы» и «Компоновка» — оживляются кодом. Всё, что здесь, не
// зависит от окна ОС: RichText, TextBox для кода, прокрутка вбок, меню с
// сочетаниями и мнемониками, колонки таблицы, дерево из модели, ThemeScope.
// Поэтому код живёт в одном месте и подключается одной функцией Wire; то, что
// требует настоящего окна (печать, PDF, ссылка, тема ОС, таймер), остаётся в
// cmd/showcase.
package showcasedemo

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"unicode"

	"github.com/oops1/headless-gui/v3/widget"
	dg "github.com/oops1/headless-gui/v3/widget/datagrid"
)

// Env — то, что витрина даёт демонстрациям.
type Env struct {
	// Reg — реестр виджетов разметки (имя → виджет).
	Reg map[string]widget.Widget
	// Log пишет строку в журнал событий. Формат — КЛЮЧ перевода (как у
	// addLog витрины): строка собирается на языке интерфейса.
	Log func(format string, args ...any)
	// OnRelocalize регистрирует замыкание, которое витрина вызывает при смене
	// языка: тексты, собранные в коде, нужно перечитать (разметка обновляется
	// сама).
	OnRelocalize func(fn func())
	// Focus отдаёт фокус виджету (Engine.SetFocus): кнопка панели не должна
	// уводить каретку из редактора. nil — фокус не трогаем.
	Focus func(w widget.Widget)
	// OpenURL открывает ссылку из RichText. nil — только запись в журнал
	// (браузерная витрина: окна ОС у сервера нет).
	OpenURL func(url string)
}

// Wire оживляет вкладки. Отсутствующие в разметке виджеты пропускаются —
// витрина с урезанной разметкой не падает.
func Wire(env Env) {
	d := &demo{Env: env}
	d.richText()
	d.codeEditor()
	d.hScroll()
	d.menus()
	d.tables()
	d.themeScopes()
}

type demo struct{ Env }

// ─── Подпись, пересобираемая при смене языка ────────────────────────────────

// Live — подпись, у которой хранятся КЛЮЧ и аргументы, а не готовая
// строка: при смене языка её можно собрать заново.
type Live struct {
	lbl  *widget.Label
	key  string
	args []any
}

func (l *Live) Set(key string, args ...any) {
	l.key, l.args = key, args
	l.refresh()
}

func (l *Live) refresh() {
	if l.lbl == nil {
		return
	}
	if l.key == "" {
		l.lbl.SetText("")
		return
	}
	l.lbl.SetText(widget.Trf(l.key, l.args...))
}

// NewLive оборачивает подпись lbl (nil допустим — тогда Set ничего не делает)
// и регистрирует её перечитывание через onRelocalize.
func NewLive(lbl *widget.Label, onRelocalize func(fn func())) *Live {
	l := &Live{lbl: lbl}
	if onRelocalize != nil {
		onRelocalize(l.refresh)
	}
	return l
}

// newLive привязывает подпись по имени и подписывает её на смену языка.
func (d *demo) newLive(id string) *Live {
	lbl, _ := d.Reg[id].(*widget.Label)
	return NewLive(lbl, d.OnRelocalize)
}

func (d *demo) relocalize(fn func()) {
	if d.OnRelocalize != nil {
		d.OnRelocalize(fn)
	}
}

func (d *demo) log(format string, args ...any) {
	if d.Log != nil {
		d.Log(format, args...)
	}
}

func (d *demo) focus(w widget.Widget) {
	if d.Focus != nil {
		d.Focus(w)
	}
}

func (d *demo) button(id string, fn func()) *widget.Button {
	b, _ := d.Reg[id].(*widget.Button)
	if b != nil {
		b.OnClick = fn
	}
	return b
}

// ─── RichText: показ и редактор ─────────────────────────────────────────────

var (
	// Цвета подобраны так, чтобы читаться и на тёмной, и на светлой теме.
	colRed   = color.RGBA{R: 224, G: 90, B: 90, A: 255}
	colBlue  = color.RGBA{R: 70, G: 140, B: 230, A: 255}
	colAmber = color.RGBA{R: 255, G: 214, B: 102, A: 255}
)

// richViewParagraphs — содержимое «показа»: абзацы разного кегля, жирный,
// курсив, цвет, фон, подчёркивание, зачёркивание, ссылка.
func richViewParagraphs() []widget.RichParagraph {
	tr := widget.Tr
	return []widget.RichParagraph{
		{Runs: []widget.RichRun{{Text: tr("RichText"), Font: widget.BuiltinFontBold, Size: 22}},
			SpaceAfter: 4},
		{Runs: []widget.RichRun{{Text: tr("A paragraph is a row of runs; each run has its own font, size and colour."), Size: 10}},
			SpaceAfter: 8},
		{Runs: []widget.RichRun{
			{Text: tr("Bold"), Font: widget.BuiltinFontBold},
			{Text: ", "},
			{Text: tr("italic"), Font: widget.BuiltinFontItalic},
			{Text: ", "},
			{Text: tr("bold italic"), Font: widget.BuiltinFontBoldItalic},
			{Text: ", "},
			{Text: tr("underlined"), Underline: true},
			{Text: ", "},
			{Text: tr("struck out"), Strike: true},
		}, SpaceAfter: 6},
		{Runs: []widget.RichRun{
			{Text: tr("Colour: ")},
			{Text: tr("red"), Color: colRed},
			{Text: ", "},
			{Text: tr("blue"), Color: colBlue},
			{Text: tr(" and a marker"), Color: color.RGBA{A: 255}, BG: colAmber},
		}, SpaceAfter: 6},
		{Runs: []widget.RichRun{
			{Text: tr("Sizes: "), Size: 9},
			{Text: "9", Size: 9},
			{Text: " 12", Size: 12},
			{Text: " 16", Size: 16},
			{Text: " 22", Size: 22},
			{Text: tr(" — one baseline")},
		}, SpaceAfter: 6},
		{Runs: []widget.RichRun{
			{Text: tr("Link: ")},
			{Text: "github.com/oops1/headless-gui", Link: "https://github.com/oops1/headless-gui"},
		}, SpaceAfter: 6},
		{Runs: []widget.RichRun{
			{Text: tr("A centred, indented paragraph: words wrap by pixels, so runs of different sizes lie on one line correctly."),
				Font: widget.BuiltinFontItalic},
		}, Align: widget.TextAlignCenter, Indent: 16},
	}
}

// richEditParagraphs — начальный текст редактора.
func richEditParagraphs() []widget.RichParagraph {
	tr := widget.Tr
	return []widget.RichParagraph{
		{Runs: []widget.RichRun{{Text: tr("Edit me"), Font: widget.BuiltinFontBold, Size: 18}},
			SpaceAfter: 4},
		{Runs: []widget.RichRun{
			{Text: tr("Select a word and press ")},
			{Text: tr("B"), Font: widget.BuiltinFontBold},
			{Text: ", "},
			{Text: tr("I"), Font: widget.BuiltinFontItalic},
			{Text: " "},
			{Text: tr("or")},
			{Text: " "},
			{Text: tr("U"), Underline: true},
			{Text: "."},
		}, SpaceAfter: 4},
		{Runs: []widget.RichRun{{Text: tr("Enter starts a paragraph, Shift+Enter a new line, Ctrl+Z undoes. Paste from Word or a browser keeps the formatting.")}}},
	}
}

func (d *demo) richText() {
	view, _ := d.Reg["rtView"].(*widget.RichText)
	ed, _ := d.Reg["rtEdit"].(*widget.RichText)
	htmlBox, _ := d.Reg["rtHtml"].(*widget.TextBox)
	status := d.newLive("rtStatus")

	if view != nil {
		view.SetParagraphs(richViewParagraphs())
		// Виджет сам ничего не открывает — ссылкой распоряжается приложение.
		view.OnLinkClick = func(url string) {
			d.log("RichText: link %s", url)
			if d.OpenURL != nil {
				d.OpenURL(url)
			}
		}
		d.relocalize(func() { view.SetParagraphs(richViewParagraphs()) })
	}

	if ed != nil {
		ed.SetParagraphs(richEditParagraphs())

		// HTML() — весь документ строкой. Перенос после абзацев — только для
		// показа: так видна структура; сам HTML() остаётся прежним.
		showHTML := func() string {
			html := ed.HTML()
			if htmlBox != nil {
				htmlBox.SetText(strings.ReplaceAll(html, "</p>", "</p>\n"))
			}
			return html
		}
		showHTML() // поле не пустое с первого кадра

		// Пока человек не правил текст, демонстрационное содержимое
		// переводится вместе с интерфейсом; после правки оно его.
		edited := false
		ed.OnChange = func() { edited = true }
		ed.OnLinkClick = func(url string) {
			d.log("RichText: link %s", url)
			if d.OpenURL != nil {
				d.OpenURL(url)
			}
		}
		d.relocalize(func() {
			if !edited {
				ed.SetParagraphs(richEditParagraphs())
				showHTML()
			}
		})

		// Панель: кнопка меняет оформление и возвращает фокус в редактор —
		// иначе каретка пропала бы, а Ctrl+Z ушёл бы кнопке.
		tool := func(id string, act func()) {
			d.button(id, func() {
				act()
				d.focus(ed)
			})
		}
		tool("rtBold", ed.ToggleBold)
		tool("rtItalic", ed.ToggleItalic)
		tool("rtUnderline", ed.ToggleUnderline)
		tool("rtColor", func() {
			// Цвет уже красный у начала выделения — снимаем, иначе ставим.
			red := ed.SelectionStyle().Color == colRed
			ed.SetSelectionStyle(func(r *widget.RichRun) {
				if red {
					r.Color = color.RGBA{}
				} else {
					r.Color = colRed
				}
			})
		})
		tool("rtUndo", func() { ed.Undo() })
		tool("rtRedo", func() { ed.Redo() })

		d.button("rtShowHtml", func() {
			html := showHTML()
			status.Set("HTML: %d characters, %d paragraphs", len([]rune(html)), len(ed.Paragraphs()))
			d.log("RichText: HTML() = %d characters", len([]rune(html)))
		})
	}
}

// ─── TextBox для кода: моноширинный шрифт, Tab, подсветка ───────────────────

const codeSample = "package main\n" +
	"\n" +
	"import \"fmt\"\n" +
	"\n" +
	"// main prints a greeting five times.\n" +
	"func main() {\n" +
	"\tname := \"world\" // the string and this comment are painted by SetStyler\n" +
	"\tfor i := 1; i <= 5; i++ {\n" +
	"\t\tfmt.Println(\"Hello,\", name, \"- step\", i)\n" +
	"\t}\n" +
	"\t// This line is long on purpose: with word wrap off it runs to the right, and the horizontal scrollbar appears under the editor.\n" +
	"\tfmt.Println(\"Scroll sideways: drag the thumb, Shift+wheel or the horizontal wheel — the text does not wrap\", name)\n" +
	"}\n"

// goStyler — подсветка в духе Go: комментарии `//`, строки в кавычках,
// ключевые слова. Состояния между строками нет (строковый литерал в
// обратных кавычках на несколько строк не разбирается): Styler получает по
// одной строке, и для демонстрации этого достаточно.
type goStyler struct{ comment, str, keyword color.RGBA }

var goKeywords = map[string]bool{
	"package": true, "import": true, "func": true, "return": true, "if": true,
	"else": true, "for": true, "range": true, "var": true, "const": true,
	"type": true, "struct": true, "switch": true, "case": true, "go": true,
	"defer": true, "nil": true,
}

// LineSpans реализует widget.Styler. Границы — в рунах от начала строки.
func (s goStyler) LineSpans(_ int, text string) []widget.Span {
	rs := []rune(text)
	var spans []widget.Span
	for i := 0; i < len(rs); {
		switch {
		case rs[i] == '/' && i+1 < len(rs) && rs[i+1] == '/':
			// Комментарий — до конца строки.
			spans = append(spans, widget.Span{From: i, To: len(rs), Style: widget.Style{Color: s.comment}})
			i = len(rs)
		case rs[i] == '"':
			j := i + 1
			for j < len(rs) && rs[j] != '"' {
				if rs[j] == '\\' {
					j++ // экранированный символ пропускаем
				}
				j++
			}
			if j < len(rs) {
				j++ // закрывающая кавычка тоже строка
			}
			spans = append(spans, widget.Span{From: i, To: min(j, len(rs)), Style: widget.Style{Color: s.str}})
			i = j
		case unicode.IsLetter(rs[i]) || rs[i] == '_':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_') {
				j++
			}
			if goKeywords[string(rs[i:j])] {
				spans = append(spans, widget.Span{From: i, To: j, Style: widget.Style{Color: s.keyword}})
			}
			i = j
		default:
			i++
		}
	}
	return spans
}

func (d *demo) codeEditor() {
	tb, _ := d.Reg["codeBox"].(*widget.TextBox)
	if tb == nil {
		return
	}
	status := d.newLive("codeStatus")

	// Свойства, которые разметка тоже умеет (FontFamily, AcceptsTab,
	// TextWrapping="NoWrap"), задаём в коде явно: показать, как это делает
	// приложение без XAML.
	tb.FontName = widget.BuiltinFontMono
	tb.AcceptTab = true
	tb.TabSize = 4
	tb.Wrap = false
	tb.SetStyler(goStyler{
		comment: color.RGBA{R: 87, G: 166, B: 74, A: 255},
		str:     color.RGBA{R: 214, G: 157, B: 133, A: 255},
		keyword: color.RGBA{R: 86, G: 156, B: 214, A: 255},
	})
	tb.SetText(codeSample)

	refresh := func() {
		// Два ключа вместо подстановки «on/off»: подпись хранится ключом и
		// аргументами, а вложенный перевод остался бы на прежнем языке.
		key := "Lines: %d · characters: %d · word wrap: off"
		if tb.Wrap {
			key = "Lines: %d · characters: %d · word wrap: on"
		}
		status.Set(key, len(strings.Split(tb.GetText(), "\n")), len([]rune(tb.GetText())))
	}
	tb.OnChange = func(string) { refresh() }
	refresh()

	if b, ok := d.Reg["codeWrap"].(*widget.Button); ok {
		b.OnCheckedChanged = func(on bool) {
			tb.Wrap = on
			tb.Invalidate()
			refresh()
			d.log("TextBox: word wrap = %v", on)
		}
	}
}

// ─── ScrollView вбок: кнопки внутри, щелчок после прокрутки ─────────────────

func (d *demo) hScroll() {
	sv, _ := d.Reg["hScroll"].(*widget.ScrollView)
	if sv == nil {
		return
	}
	// Содержимое — десять кнопок по 160 точек с отступом: шире области.
	const contentW = 1620
	sv.ContentWidth = contentW

	status := d.newLive("hsStatus")
	status.Set("Scroll position: %d of %d", sv.ScrollX(), contentW-sv.Bounds().Dx())

	for i := 0; i < 10; i++ {
		n := i + 1
		d.button(fmt.Sprintf("hsBtn%d", i), func() {
			// Клик дошёл именно до той кнопки, что видна под курсором.
			status.Set("Clicked button %d (scrolled by %d px)", n, sv.ScrollX())
			d.log("ScrollView: button %d (scrolled by %d px)", n, sv.ScrollX())
		})
	}
	d.button("hsLeft", func() {
		sv.SetScrollX(0)
		status.Set("Scroll position: %d of %d", sv.ScrollX(), contentW-sv.Bounds().Dx())
	})
	d.button("hsRight", func() {
		sv.SetScrollX(contentW) // зажмётся в допустимое
		status.Set("Scroll position: %d of %d", sv.ScrollX(), contentW-sv.Bounds().Dx())
	})
}

// ─── Меню: сочетания и мнемоники ────────────────────────────────────────────

// plainMnemonic убирает из подписи пункта разметку мнемоники: «_Сохранить» →
// «Сохранить», «a__b» → «a_b».
func plainMnemonic(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] == '_' {
			if i+1 < len(rs) && rs[i+1] == '_' {
				b.WriteRune('_')
				i++
			}
			continue
		}
		b.WriteRune(rs[i])
	}
	return b.String()
}

// contains — есть ли target в поддереве root.
func contains(root, target widget.Widget) bool {
	if root == nil {
		return false
	}
	if root == target {
		return true
	}
	for _, ch := range root.Children() {
		if contains(ch, target) {
			return true
		}
	}
	return false
}

// TabOf возвращает индекс вкладки, внутри которой лежит виджет w, или −1.
// Витрине это нужно, чтобы скрыть вкладку целиком (браузерная витрина прячет
// «Платформу»), не полагаясь на её номер в разметке.
func TabOf(tabs *widget.TabControl, w widget.Widget) int {
	if tabs == nil || w == nil {
		return -1
	}
	for i := 0; i < tabs.TabCount(); i++ {
		if contains(tabs.TabContent(i), w) {
			return i
		}
	}
	return -1
}

func (d *demo) menus() {
	bar, _ := d.Reg["menuDemo"].(*widget.MenuBar)
	pm, _ := d.Reg["menuPopup"].(*widget.PopupMenu)
	status := d.newLive("menuStatus")
	status.Set("Nothing chosen yet")

	if bar != nil {
		bar.OnSelect = func(_, _ int, text string) {
			status.Set("Menu command: %s", plainMnemonic(text))
			d.log("Menu: %s", plainMnemonic(text))
		}
	}
	if pm != nil {
		pm.OnSelect = func(_ int, text string) {
			status.Set("Popup command: %s", plainMnemonic(text))
			d.log("PopupMenu: «%s»", plainMnemonic(text))
		}
		if b, ok := d.Reg["menuShowPopup"].(*widget.Button); ok {
			b.OnClick = func() { pm.ShowBelow(b) }
		}
	}

	// Сочетания клавиш — это InputBindings окна: меню их только ПОКАЗЫВАЕТ.
	// Ctrl+S уже занят привязкой витрины (SaveCommand), её не трогаем.
	win, _ := d.Reg["rootWin"].(*widget.Window)
	if win == nil {
		return
	}
	for _, sc := range []struct {
		key  widget.KeyCode
		name string
	}{{widget.KeyN, "Ctrl+N"}, {widget.KeyO, "Ctrl+O"}} {
		name := sc.name
		win.InputBindings = append(win.InputBindings, widget.InputBinding{
			Key: sc.key, Mods: widget.ModCtrl,
			Command: widget.NewRelayCommand(func() {
				status.Set("Shortcut: %s", name)
				d.log("Shortcut: %s", name)
			}),
		})
	}

	// Alt+буква открывает строку меню: полоса фокуса не держит, поэтому Alt+Ф
	// приложение получает само и передаёт в ActivateMnemonic. Кириллическую
	// мнемонику движок ловит по физической клавише, так что одна привязка на
	// латинскую клавишу покрывает обе раскладки. Привязываем все буквы, а
	// клавишу «съедаем» только когда мнемоника нашлась: это решает
	// CanExecute (ActivateMnemonic сам открывает меню и отвечает, нашлась ли
	// буква), иначе чужие Alt+буква пропадали бы.
	if bar == nil {
		return
	}
	tabs, _ := d.Reg["mainTabs"].(*widget.TabControl)
	menuTab := TabOf(tabs, bar)
	for c := 'A'; c <= 'Z'; c++ {
		code := widget.KeyCode(c)
		win.InputBindings = append(win.InputBindings, widget.InputBinding{
			Key: code, Mods: widget.ModAlt,
			Command: &widget.RelayCommand{
				CanExecuteFn: func(any) bool {
					// Строка меню видна только на своей вкладке.
					if tabs == nil || tabs.Active() != menuTab {
						return false
					}
					// Фокус строка меню берёт сама и, закрывшись, возвращает:
					// стрелки, буква пункта и Esc дойдут до неё.
					return bar.ActivateMnemonic(widget.KeyEvent{Code: code, Mod: widget.ModAlt, Pressed: true})
				},
			},
		})
	}
}

// ─── Таблица с колонками-флажком и шаблонной, дерево из модели ──────────────

// service — строка таблицы: имя, флажок, нагрузка 0..1.
type service struct {
	Name    string
	Enabled bool
	Load    float64
}

// node — узел дерева из модели; HierarchicalDataTemplate берёт заголовок из
// Name и дочерние узлы из Children.
type node struct {
	Name     string
	Children []*node
}

func (d *demo) tables() {
	if g, ok := d.Reg["gridCheck"].(*widget.DataGridWidget); ok {
		items := dg.NewObservableCollection()
		for _, s := range []*service{
			{"nginx", true, 0.62}, {"postgres", true, 0.35}, {"redis", false, 0.08},
			{"worker", true, 0.91}, {"cron", false, 0.02},
		} {
			items.Add(s)
		}
		g.Grid.SetItemsSource(items)

		// Разметка объявляет колонку тегом DataGridTemplateColumn, а вот
		// отрисовку ячейки задаёт приложение: полоса нагрузки с подписью.
		for _, col := range g.Grid.Columns() {
			if tc, ok := col.(*dg.DataGridTemplateColumn); ok {
				tc.CellTemplate = drawLoadCell
			}
		}
		g.Grid.OnCellEditEnding = func(e *dg.CellEditEndingEvent) {
			d.log("DataGrid: cell edited")
		}
	}

	if tw, ok := d.Reg["treeHier"].(*widget.TreeViewWidget); ok {
		roots := dg.NewObservableCollection()
		roots.Add(&node{"widget", []*node{{"RichText", nil}, {"ScrollView", nil}, {"TextBox", nil}}})
		roots.Add(&node{"window", []*node{{"OpenURL", nil}, {"DetectSystemTheme", nil}}})
		roots.Add(&node{"printing", []*node{{"SavePDF", nil}, {"PrintDialog", nil}}})
		// Шаблон — в разметке (<HierarchicalDataTemplate ItemsSource=
		// "{Binding Children}">); модель у дерева своя, не из DataContext
		// окна, поэтому источник подаём кодом.
		tw.Tree.SetItemsSource(roots)
		// Раскрыты первые два корня, третий свёрнут: так все строки помещаются
		// в окно дерева целиком, а раскрытие видно и без прокрутки.
		for i, r := range tw.Tree.Roots() {
			if i < 2 {
				tw.Tree.ExpandItem(r)
			}
		}
	}
}

// drawLoadCell рисует ячейку шаблонной колонки: дорожка, заполнение и процент.
func drawLoadCell(c dg.CellDrawContext) {
	s, ok := c.Item.(*service)
	if !ok {
		return
	}
	r := c.Rect
	barH := 8
	track := image.Rect(r.Min.X+8, r.Min.Y+(r.Dy()-barH)/2, r.Max.X-52, r.Min.Y+(r.Dy()-barH)/2+barH)
	// Дорожка — цвет текста темы с малой прозрачностью, поэтому читается и на
	// тёмной, и на светлой теме. Цвета в движке предумножены на альфу.
	const a = 48
	tc := c.TextColor
	c.DrawCtx.FillRectAlpha(track.Min.X, track.Min.Y, track.Dx(), track.Dy(),
		color.RGBA{R: uint8(int(tc.R) * a / 255), G: uint8(int(tc.G) * a / 255), B: uint8(int(tc.B) * a / 255), A: a})
	fill := color.RGBA{R: 76, G: 175, B: 80, A: 255}
	if s.Load > 0.8 {
		fill = color.RGBA{R: 224, G: 90, B: 90, A: 255}
	}
	c.DrawCtx.FillRect(track.Min.X, track.Min.Y, int(float64(track.Dx())*s.Load), track.Dy(), fill)
	c.DrawCtx.DrawText(fmt.Sprintf("%.0f%%", s.Load*100), track.Max.X+8, r.Min.Y+(r.Dy()-16)/2, c.TextColor)
}

// ─── ThemeScope: четыре облика одновременно ─────────────────────────────────

func (d *demo) themeScopes() {
	mount, _ := d.Reg["themeScopeMount"].(*widget.Panel)
	if mount == nil {
		return
	}
	mb := mount.Bounds()
	const gap = 10
	names := []string{"Win10 Dark", "Win11 Light", "Win2000", "Mac"}
	w := (mb.Dx() - gap*(len(names)-1)) / len(names)
	for i, name := range names {
		th := widget.ThemeByName(name)
		if th == nil {
			continue
		}
		r := image.Rect(mb.Min.X+i*(w+gap), mb.Min.Y, mb.Min.X+i*(w+gap)+w, mb.Max.Y)
		scope := widget.NewThemeScope(th)
		scope.SetBounds(r)

		// Подложка области — Panel: её цвет берётся из темы области, а не из
		// темы приложения.
		bg := widget.NewPanel(th.WindowBG)
		bg.ShowHeader = false
		bg.SetBounds(r)

		title := widget.NewLabel(name, th.LabelText)
		title.SetBounds(image.Rect(r.Min.X+8, r.Min.Y+4, r.Max.X-8, r.Min.Y+22))
		bg.AddChild(title)
		ok := widget.NewButton("OK")
		ok.SetBounds(image.Rect(r.Min.X+8, r.Min.Y+30, r.Min.X+88, r.Min.Y+58))
		bg.AddChild(ok)
		cb := widget.NewCheckBox("CheckBox")
		cb.SetBounds(image.Rect(r.Min.X+98, r.Min.Y+32, r.Max.X-6, r.Min.Y+56))
		cb.SetChecked(true)
		bg.AddChild(cb)

		// Область темизирует поддерево в момент AddChild, поэтому подложку
		// отдаём ей уже С детьми: добавленные позже остались бы в теме
		// приложения.
		scope.AddChild(bg)
		mount.AddChild(scope)
	}
}
