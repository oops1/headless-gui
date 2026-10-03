package showcasedemo

import (
	"os"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/cmd/internal/showcasestrings"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Вкладки версий 3.27–3.29 оживляются кодом, поэтому проверяем их тем, чем
// пользуется человек: настоящим вводом в движок. Разметка та же, что у обеих
// витрин.

// xamlPath — разметка витрины относительно каталога пакета.
const xamlPath = "../../../assets/ui/showcase.xaml"

type rig struct {
	t      *testing.T
	eng    *engine.Engine
	reg    map[string]widget.Widget
	logs   []string
	relocs []func()
}

func newRig(t *testing.T) *rig {
	t.Helper()
	if _, err := os.Stat(xamlPath); err != nil {
		t.Skipf("разметки нет: %v", err)
	}
	showcasestrings.Register()
	widget.SetLanguage("EN")
	t.Cleanup(func() { widget.SetLanguage("RU") })

	root, reg, err := widget.LoadUIFromXAMLFile(xamlPath)
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	t.Cleanup(func() { widget.ReleaseXAML(root) })

	eng := engine.New(1280, 900, 30)
	eng.SetRoot(root)
	r := &rig{t: t, eng: eng, reg: reg}
	Wire(Env{
		Reg:          reg,
		Log:          func(f string, a ...any) { r.logs = append(r.logs, widget.Trf(f, a...)) },
		OnRelocalize: func(fn func()) { r.relocs = append(r.relocs, fn) },
		Focus:        eng.SetFocus,
	})
	return r
}

// tab открывает вкладку, на которой лежит виджет id.
func (r *rig) tab(id string) {
	r.t.Helper()
	tabs := r.reg["mainTabs"].(*widget.TabControl)
	i := TabOf(tabs, r.reg[id])
	if i < 0 {
		r.t.Fatalf("виджет %q не лежит ни на одной вкладке", id)
	}
	tabs.SetActive(i)
	r.eng.RenderOnce()
}

func (r *rig) click(x, y int) {
	r.eng.SendMouseMove(x, y)
	r.eng.SendMouseButton(x, y, widget.MouseLeft, true)
	r.eng.SendMouseButton(x, y, widget.MouseLeft, false)
	r.eng.RenderOnce()
}

func (r *rig) clickWidget(id string) {
	b := r.reg[id].Bounds()
	r.click((b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2)
}

func (r *rig) key(code widget.KeyCode, mod widget.KeyMod) {
	r.eng.SendKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: true})
	r.eng.SendKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: false})
	r.eng.RenderOnce()
}

func (r *rig) label(id string) string { return r.reg[id].(*widget.Label).Text() }

// Кнопки панели меняют оформление выделенного, и Ctrl+Z их откатывает; HTML()
// показывает результат в поле.
func TestRichTextToolbar(t *testing.T) {
	r := newRig(t)
	r.tab("rtEdit")
	ed := r.reg["rtEdit"].(*widget.RichText)
	if !ed.Editable {
		t.Fatal("rtEdit должен быть редактором (Editable)")
	}

	orig := ed.HTML()
	r.eng.SetFocus(ed)
	ed.SelectAll()
	r.clickWidget("rtBold")
	if !strings.Contains(ed.HTML(), "<span style=\"font-weight:bold\">Select a word") {
		t.Errorf("после «Ж» обычный текст не стал жирным: %s", ed.HTML())
	}
	if !ed.CanUndo() {
		t.Error("правка оформления не попала в историю")
	}
	r.clickWidget("rtUndo")
	if ed.HTML() != orig {
		t.Errorf("отмена не вернула документ:\n%s\nбыло:\n%s", ed.HTML(), orig)
	}

	r.clickWidget("rtColor")
	r.clickWidget("rtShowHtml")
	box := r.reg["rtHtml"].(*widget.TextBox)
	if !strings.Contains(box.GetText(), "color:#e05a5a") {
		t.Errorf("поле HTML не показывает цвет: %q", box.GetText())
	}
	if !strings.HasPrefix(r.label("rtStatus"), "HTML:") {
		t.Errorf("подпись состояния: %q", r.label("rtStatus"))
	}
}

// Ссылка в показе уходит приложению, а не открывается виджетом.
func TestRichTextLinkGoesToApp(t *testing.T) {
	r := newRig(t)
	view := r.reg["rtView"].(*widget.RichText)
	var got string
	Wire(Env{Reg: r.reg, OpenURL: func(u string) { got = u }})
	view.OnLinkClick("https://example.com/x")
	if got != "https://example.com/x" {
		t.Errorf("OpenURL получил %q", got)
	}
}

// Прокрутка вбок: после SetScrollX щелчок попадает в кнопку, которую видно.
func TestHScrollClickReachesVisibleButton(t *testing.T) {
	r := newRig(t)
	r.tab("hScroll")
	sv := r.reg["hScroll"].(*widget.ScrollView)
	if sv.ContentWidth <= sv.Bounds().Dx() {
		t.Fatalf("содержимое %d не шире области %d", sv.ContentWidth, sv.Bounds().Dx())
	}
	sv.SetScrollX(500)
	r.eng.RenderOnce()

	// Дети остаются в координатах содержимого; на экране кнопка сдвинута.
	b := r.reg["hsBtn5"].Bounds()
	r.click(b.Min.X-sv.ScrollX()+b.Dx()/2, (b.Min.Y+b.Max.Y)/2)
	if got := r.label("hsStatus"); !strings.HasPrefix(got, "Clicked button 6") {
		t.Errorf("щелчок после прокрутки: %q", got)
	}

	r.clickWidget("hsRight")
	if sv.ScrollX() != sv.ContentWidth-sv.Bounds().Dx() {
		t.Errorf("«в конец»: ScrollX=%d", sv.ScrollX())
	}
}

// TextBox для кода: моноширинный, Tab — символ, подсветка, перенос переключается.
func TestCodeEditor(t *testing.T) {
	r := newRig(t)
	tb := r.reg["codeBox"].(*widget.TextBox)
	if tb.FontName != widget.BuiltinFontMono || !tb.AcceptTab || tb.Wrap {
		t.Fatalf("настройки редактора кода: font=%q tab=%v wrap=%v", tb.FontName, tb.AcceptTab, tb.Wrap)
	}

	// Подсветка: комментарий и строка получают свои цвета, ключевое слово тоже.
	s := goStyler{
		comment: colAmber, str: colRed, keyword: colBlue,
	}
	spans := s.LineSpans(0, "\tname := \"w\" // c")
	var gotStr, gotComment bool
	for _, sp := range spans {
		switch sp.Style.Color {
		case colRed:
			gotStr = sp.From == 9 && sp.To == 12
		case colAmber:
			gotComment = sp.From == 13
		}
	}
	if !gotStr || !gotComment {
		t.Errorf("подсветка: строка=%v комментарий=%v, spans=%v", gotStr, gotComment, spans)
	}
	// Кавычка внутри комментария строкой не считается.
	for _, sp := range s.LineSpans(0, "// say \"hi\"") {
		if sp.Style.Color == colRed {
			t.Errorf("строка внутри комментария подсвечена: %v", sp)
		}
	}

	r.tab("codeBox")
	r.clickWidget("codeWrap")
	if !tb.Wrap {
		t.Error("кнопка «Перенос» не включила Wrap")
	}
	if !strings.HasSuffix(r.label("codeStatus"), "on") {
		t.Errorf("подпись после включения переноса: %q", r.label("codeStatus"))
	}
	r.clickWidget("codeWrap")
	if tb.Wrap {
		t.Error("повторное нажатие не выключило Wrap")
	}

	// Tab вставляется символом, а не уводит фокус.
	r.eng.SetFocus(tb)
	before := tb.GetText()
	r.key(widget.KeyTab, 0)
	if tb.GetText() == before || !strings.Contains(tb.GetText(), "\t") {
		t.Errorf("Tab не вставил табуляцию: %q", tb.GetText()[:12])
	}
}

// Меню: сочетание написано, Ctrl+N работает, Alt+буква открывает строку меню,
// а подчёркнутая буква выбирает пункт — и по-английски, и по-русски (по
// физической клавише).
func TestMenusShortcutsAndMnemonics(t *testing.T) {
	r := newRig(t)
	r.tab("menuDemo")

	r.key(widget.KeyN, widget.ModCtrl)
	if got := r.label("menuStatus"); got != "Shortcut: Ctrl+N" {
		t.Errorf("Ctrl+N: %q", got)
	}

	r.key(widget.KeyF, widget.ModAlt) // «_File»
	r.key(widget.KeyO, 0)             // «_Open…»
	if got := r.label("menuStatus"); got != "Menu command: Open…" {
		t.Errorf("Alt+F, O: %q", got)
	}

	// По-русски «_Файл» открывается Alt+Ф — это физическая клавиша A,
	// «Сох_ранить» выбирается буквой «р» — клавиша H.
	widget.SetLanguage("RU")
	r.eng.RenderOnce()
	r.key(widget.KeyA, widget.ModAlt)
	r.key(widget.KeyH, 0)
	if got := r.label("menuStatus"); got != "Команда меню: Сохранить" {
		t.Errorf("Alt+Ф, р: %q", got)
	}
}

// Alt+буква на чужой вкладке не должен открывать невидимое меню и не должен
// съедать нажатие.
func TestAltMnemonicIgnoredOnOtherTab(t *testing.T) {
	r := newRig(t)
	r.tab("rtEdit")
	bar := r.reg["menuDemo"].(*widget.MenuBar)
	r.key(widget.KeyF, widget.ModAlt)
	if bar.IsFocused() {
		t.Error("строка меню с другой вкладки открылась по Alt+F")
	}
}

// Нижние ряды «Деревьев и таблиц» и «Компоновки»: колонки таблицы, дерево из
// модели, четыре области темы.
func TestTablesTreeAndThemeScopes(t *testing.T) {
	r := newRig(t)

	g := r.reg["gridCheck"].(*widget.DataGridWidget)
	if n := g.Grid.ItemsSource().Count(); n == 0 {
		t.Error("таблица с флажками пуста")
	}
	var template bool
	for _, c := range g.Grid.Columns() {
		if tc, ok := c.(interface{ UsesCachedText() bool }); ok && !tc.UsesCachedText() {
			template = true
		}
	}
	if len(g.Grid.Columns()) != 3 || !template {
		t.Errorf("колонок %d, шаблонная колонка найдена: %v", len(g.Grid.Columns()), template)
	}

	tw := r.reg["treeHier"].(*widget.TreeViewWidget)
	roots := tw.Tree.Roots()
	if len(roots) != 3 || roots[0].Header != "widget" || len(roots[0].Children) != 3 {
		t.Errorf("дерево из модели: %d корней", len(roots))
	}

	mount := r.reg["themeScopeMount"].(*widget.Panel)
	var scopes int
	for _, ch := range mount.Children() {
		if s, ok := ch.(*widget.ThemeScope); ok && s.HasOwnTheme() {
			scopes++
		}
	}
	if scopes != 4 {
		t.Errorf("областей темы %d, ждали 4", scopes)
	}
}

// Смена языка перечитывает то, что собрано в коде.
func TestRelocalize(t *testing.T) {
	r := newRig(t)
	ed := r.reg["rtEdit"].(*widget.RichText)
	if !strings.HasPrefix(ed.Text(), "Edit me") {
		t.Fatalf("EN: %q", ed.Text())
	}
	widget.SetLanguage("RU")
	for _, fn := range r.relocs {
		fn()
	}
	if !strings.HasPrefix(ed.Text(), "Правь меня") {
		t.Errorf("RU: %q", ed.Text())
	}
	if got := r.label("menuStatus"); got != "Пока ничего не выбрано" {
		t.Errorf("подпись меню после смены языка: %q", got)
	}

	// Если человек правил текст, перевод его не затирает.
	ed.SelectAll()
	r.eng.SetFocus(ed)
	r.eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyX, Rune: 'x', Pressed: true})
	widget.SetLanguage("EN")
	for _, fn := range r.relocs {
		fn()
	}
	if strings.HasPrefix(ed.Text(), "Edit me") {
		t.Errorf("перевод затёр правку человека: %q", ed.Text())
	}
}
