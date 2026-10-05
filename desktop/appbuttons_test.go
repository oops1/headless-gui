package desktop

import (
	"image"
	"image/color"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Кнопки приложений по-Windows 10: окна одного приложения в одной кнопке,
// список окон в предпросмотре, меню команд, подсказка, состояния и «Пуск» с
// состоянием «меню открыто».

// win10Fast — профиль Windows 10 с короткими задержками предпросмотра: тесты
// проверяют поведение, а не терпение.
func win10Fast(t *testing.T) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	p := theme.Windows10Profile()
	p.SetMetric(KeyPreviewDelayOpen, 5).SetMetric(KeyPreviewDelayClose, 5).SetMetric(KeyPreviewRefresh, 5)
	if err := m.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(p.Name); err != nil {
		t.Fatal(err)
	}
	return m
}

// groupScene — область приложений с группировкой окон: «files» закреплён и не
// запущен, «term» закреплён и имеет два окна, «mail» не закреплён и имеет два
// окна (одно свёрнуто), «calc» — одно окно.
func groupScene(t *testing.T, tm *theme.Manager) (*ApplicationArea, *StaticAppCatalog, *fakePreviews) {
	t.Helper()
	cat := NewStaticAppCatalog(
		AppInfo{ID: "files", Title: "Files"},
		AppInfo{ID: "term", Title: "Terminal"},
		AppInfo{ID: "mail", Title: "Mail"},
	)
	cat.Pin("files")
	cat.Pin("term")
	wm := newFakePreviews(
		WindowInfo{ID: 1, AppID: "term", Title: "term one", Active: true},
		WindowInfo{ID: 2, AppID: "term", Title: "term two"},
		WindowInfo{ID: 3, AppID: "mail", Title: "Inbox"},
		WindowInfo{ID: 4, AppID: "mail", Title: "Draft", Minimized: true},
		WindowInfo{ID: 5, AppID: "calc", Title: "Calc"},
	)
	a := NewApplicationArea(tm, cat, wm)
	a.SetBounds(image.Rect(100, 560, 700, 600))
	t.Cleanup(a.Close)
	return a, cat, wm
}

func titles(cells []Cell) []string {
	var out []string
	for _, c := range cells {
		out = append(out, c.Title)
	}
	return out
}

// rightClick щёлкает правой кнопкой по центру прямоугольника.
func rightClick(a *ApplicationArea, r image.Rectangle) bool {
	x, y := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	a.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseRight, Pressed: true})
	return a.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseRight})
}

// ─── Группировка ─────────────────────────────────────────────────────────────

func TestAppButtons_GroupsWindowsOfOneApp(t *testing.T) {
	a, _, _ := groupScene(t, win10Fast(t))

	cells := a.Cells()
	if got, want := titles(cells), []string{"Files", "Terminal", "Mail", "Calc"}; len(got) != len(want) {
		t.Fatalf("ячейки %v, ждали по одной на приложение %v", got, want)
	}
	if n := len(a.WindowsAt(1)); n != 2 {
		t.Errorf("у закреплённого «Terminal» окон %d, ждали 2", n)
	}
	if n := len(a.WindowsAt(2)); n != 2 {
		t.Errorf("у незакреплённой «Mail» окон %d, ждали 2", n)
	}
	if n := len(a.WindowsAt(0)); n != 0 {
		t.Errorf("у незапущенного «Files» окон %d, ждали 0", n)
	}
	// Активно любое из окон; свёрнуто только когда свёрнуты все.
	if !cells[1].Active || cells[2].Active {
		t.Errorf("активность ячеек: term=%v mail=%v", cells[1].Active, cells[2].Active)
	}
	if cells[2].Muted {
		t.Error("у «Mail» свёрнуто одно окно из двух — ячейка не должна быть приглушена")
	}
}

// Без признака темы окна не склеиваются: прежнее поведение.
func TestAppButtons_NoGroupingWithoutThemeFlag(t *testing.T) {
	for _, name := range []string{theme.ProfileWindows2000, theme.ProfileWindows11, theme.ProfileMacOS} {
		tm := managerFor(t, name)
		if tm.GetFlag(KeyTaskButtonGroup, false) {
			t.Errorf("%s группирует окна: прежний вид сломан", name)
		}
		if tm.GetMetric(KeyTaskButtonStack) != 0 || tm.GetMetric(KeyTaskButtonUnderlineIdleLen) != 0 {
			t.Errorf("%s получила метрики стопки или короткой линии", name)
		}
		if !taskButtonMuted(tm) {
			t.Errorf("%s перестала приглушать свёрнутые окна", name)
		}
	}
	a, _, _ := groupScene(t, managerFor(t, theme.ProfileWindows11))
	// files, term (первое окно), mail ×2, calc.
	if got := len(a.Cells()); got != 5 {
		t.Errorf("Windows 11: %d ячеек, ждали 5 (каждое окно незакреплённого — своя кнопка)", got)
	}
}

// Тема сменилась на ту, что группирует: область пересобирается сама.
func TestAppButtons_FollowsThemeSwitch(t *testing.T) {
	tm := managerFor(t, theme.ProfileWindows11)
	a, _, _ := groupScene(t, tm)
	if got := len(a.Cells()); got != 5 {
		t.Fatalf("до смены темы %d ячеек, ждали 5", got)
	}
	if err := tm.SetTheme(theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	a.PreferredSize(image.Pt(600, 40))
	if got := len(a.Cells()); got != 4 {
		t.Errorf("после смены темы %d ячеек, ждали 4: окна не склеились", got)
	}
}

// Ширина кнопки Windows 10 — 48 при значке 24, кнопки вплотную.
func TestAppButtons_Windows10ButtonGeometry(t *testing.T) {
	a, _, _ := groupScene(t, managerFor(t, theme.ProfileWindows10))
	if got, want := a.PreferredSize(image.Pt(600, 40)).X, 4*48; got != want {
		t.Errorf("ширина области %d, ждали %d (4 кнопки по 48)", got, want)
	}
	a.layout()
	if r0, r1 := cellRect(t, a, 0), cellRect(t, a, 1); r0.Dx() != 48 || r1.Min.X != r0.Max.X {
		t.Errorf("кнопки %v и %v: ждали ширину 48 и отсутствие зазора", r0, r1)
	}
}

// ─── Щелчок по стопке ────────────────────────────────────────────────────────

// Без предпросмотра окна стопки переключаются по кругу.
func TestAppButtons_StackClickCyclesWithoutPreview(t *testing.T) {
	a, _, wm := groupScene(t, win10Fast(t))
	r := cellRect(t, a, 1) // term: активно окно 1

	clickAt(a, r)
	if got := wm.Activated; len(got) != 1 || got[0] != 2 {
		t.Fatalf("первый щелчок активировал %v, ждали [2]", got)
	}
	clickAt(a, r) // теперь активно окно 2
	if got := wm.Activated; len(got) != 2 || got[1] != 1 {
		t.Errorf("второй щелчок активировал %v, ждали возврат к окну 1", got)
	}
	if len(wm.Minimized) != 0 {
		t.Error("щелчок по стопке свернул окно: он должен переключать")
	}
}

// С клавиатуры Enter у стопки листает окна: в список некуда перенести фокус.
func TestAppButtons_StackEnterCycles(t *testing.T) {
	a, _, wm := groupScene(t, win10Fast(t))
	a.SetFocused(true)
	a.FocusState.SetCell(1)
	a.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if got := wm.Activated; len(got) != 1 || got[0] != 2 {
		t.Errorf("Enter активировал %v, ждали [2]", got)
	}
}

func TestAppButtons_StackClickShowsWindowList(t *testing.T) {
	tm := win10Fast(t)
	a, _, wm := groupScene(t, tm)
	p := NewWindowPreview(tm, wm)
	p.Screen = image.Rect(0, 0, 800, 600)
	p.Track(a)
	t.Cleanup(p.Close)

	r := cellRect(t, a, 2) // mail: окна 3 и 4
	clickAt(a, r)
	if !p.IsOpen() {
		t.Fatal("щелчок по стопке не открыл список окон")
	}
	wins := p.ListedWindows()
	if len(wins) != 2 || wins[0].ID != 3 || wins[1].ID != 4 {
		t.Fatalf("в списке %+v, ждали окна 3 и 4", wins)
	}
	if len(wm.Activated) != 0 {
		t.Error("щелчок по стопке с предпросмотром переключил окно — он должен только показать список")
	}
	if r := p.Bounds(); r.Max.Y > a.ButtonRect(2).Min.Y {
		t.Errorf("список %v налез на кнопку %v", r, a.ButtonRect(2))
	}
	// Миниатюра у каждого окна своя, источник опрошен для обоих.
	if p.thumbs[3] == nil || p.thumbs[4] == nil {
		t.Errorf("миниатюры списка: %v", p.thumbs)
	}

	// Щелчок по второму окну поднимает его и закрывает список.
	items, _ := p.listItems(p.contentRect(), 2)
	pressList(p, items[1].Min.Add(image.Pt(3, 3)))
	if got := wm.Activated; len(got) != 1 || got[0] != 4 {
		t.Errorf("активировано %v, ждали [4]", got)
	}
	if p.IsOpen() {
		t.Error("после выбора окна список остался открыт")
	}
}

func pressList(p *WindowPreview, pt image.Point) {
	p.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true})
}

func TestAppButtons_ListCloseButtonClosesWindow(t *testing.T) {
	tm := win10Fast(t)
	a, _, wm := groupScene(t, tm)
	p := NewWindowPreview(tm, wm)
	p.Screen = image.Rect(0, 0, 800, 600)
	p.Track(a)
	t.Cleanup(p.Close)
	clickAt(a, cellRect(t, a, 1)) // term: окна 1 и 2

	items, vertical := p.listItems(p.contentRect(), 2)
	cr := p.closeRect(items[1], vertical)
	pressList(p, cr.Min.Add(image.Pt(2, 2)))

	if got := wm.Closed; len(got) != 1 || got[0] != 2 {
		t.Fatalf("закрыто %v, ждали [2]", got)
	}
	if len(wm.Activated) != 0 {
		t.Error("крестик ещё и активировал окно")
	}
	// Осталось одно окно — список не нужен.
	if p.IsOpen() {
		t.Error("в списке осталось одно окно, а панель открыта")
	}
}

// Наведение на стопку показывает список после задержки.
func TestAppButtons_HoverShowsWindowList(t *testing.T) {
	tm := win10Fast(t)
	a, _, wm := groupScene(t, tm)
	p := NewWindowPreview(tm, wm)
	p.Screen = image.Rect(0, 0, 800, 600)
	p.Track(a)
	t.Cleanup(p.Close)

	r := cellRect(t, a, 2)
	a.OnMouseMove(r.Min.X+5, r.Min.Y+5)
	if p.IsOpen() {
		t.Fatal("список открылся мгновенно, без задержки")
	}
	waitFor(t, "открытие списка", p.IsOpen)
	if n := len(p.ListedWindows()); n != 2 {
		t.Errorf("в списке %d окон, ждали 2", n)
	}

	// Одиночное окно по-прежнему показывается одно.
	r = cellRect(t, a, 3)
	a.OnMouseMove(r.Min.X+5, r.Min.Y+5)
	if n := len(p.ListedWindows()); n != 0 {
		t.Errorf("у «Calc» одно окно, а режим списка включён (%d)", n)
	}
	if info, ok := p.Window(); !ok || info.ID != 5 {
		t.Errorf("показано окно %+v, ждали 5", info)
	}
}

// Список следит за моделью: закрытое снаружи окно уходит из него, последнее —
// закрывает панель.
func TestAppButtons_ListFollowsWindowModel(t *testing.T) {
	tm := win10Fast(t)
	a, _, wm := groupScene(t, tm)
	p := NewWindowPreview(tm, wm)
	p.Screen = image.Rect(0, 0, 800, 600)
	p.Track(a)
	t.Cleanup(p.Close)
	clickAt(a, cellRect(t, a, 2))

	wm.SetWindows(append(wm.Windows(), WindowInfo{ID: 6, AppID: "mail", Title: "Third"}))
	if p.syncList() || len(p.ListedWindows()) != 3 {
		t.Fatalf("после нового окна в списке %d, ждали 3", len(p.ListedWindows()))
	}
	wm.Close(3)
	wm.Close(6)
	if !p.syncList() || p.IsOpen() {
		t.Error("осталось одно окно, а список не закрылся")
	}
}

// Окон больше, чем влезает в ряд, — столбец заголовков без миниатюр.
func TestAppButtons_ManyWindowsBecomeAColumn(t *testing.T) {
	tm := win10Fast(t)
	cat := NewStaticAppCatalog()
	var wins []WindowInfo
	for i := 1; i <= 9; i++ {
		wins = append(wins, WindowInfo{ID: WindowID(i), AppID: "term", Title: "t"})
	}
	wm := newFakePreviews(wins...)
	a := NewApplicationArea(tm, cat, wm)
	a.SetBounds(image.Rect(0, 560, 300, 600))
	t.Cleanup(a.Close)
	p := NewWindowPreview(tm, wm)
	p.Screen = image.Rect(0, 0, 1280, 600)
	p.Track(a)
	t.Cleanup(p.Close)

	clickAt(a, cellRect(t, a, 0))
	if !p.IsOpen() || len(p.ListedWindows()) != 9 {
		t.Fatalf("список не открылся: %v", p.ListedWindows())
	}
	if !p.listVertical(9) {
		t.Error("девять окон в ряд не помещаются, а список горизонтальный")
	}
	if wm.calls != 0 {
		t.Errorf("столбцу миниатюры не нужны, а источник опрошен %d раз", wm.calls)
	}
	if sz := p.size(); sz.X > p.thumbMax().X+2*p.metric(KeyPreviewPad, previewDefPad) {
		t.Errorf("столбец шире одной миниатюры: %v", sz)
	}
}

// ─── Меню команд ─────────────────────────────────────────────────────────────

func menuTexts(a *ApplicationArea) []string {
	m := a.menu.get()
	if m == nil {
		return nil
	}
	var out []string
	for _, it := range m.Items() {
		if it.Separator {
			out = append(out, "-")
		} else {
			out = append(out, it.Text)
		}
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAppButtons_RightClickOpensDefaultCommands(t *testing.T) {
	a, cat, wm := groupScene(t, win10Fast(t))

	// Закреплённое незапущенное: запуск и «Открепить».
	r := cellRect(t, a, 0)
	if !rightClick(a, r) || !a.HasOverlay() {
		t.Fatal("правый щелчок не открыл меню")
	}
	if want := []string{"Files", "-", tr(StrAppUnpin)}; !eq(menuTexts(a), want) {
		t.Errorf("меню %v, ждали %v", menuTexts(a), want)
	}
	if ob := a.OverlayBounds(); ob.Empty() || ob.Max.Y > r.Min.Y {
		t.Errorf("меню %v должно стоять над кнопкой %v", ob, r)
	}
	a.menu.get().Items()[2].OnClick()
	for _, id := range cat.Pinned() {
		if id == "files" {
			t.Error("«Открепить» не открепило")
		}
	}
	a.Dismiss()

	// Запущенное незакреплённое: «Закрепить» и «Закрыть все окна».
	a.layout()
	rightClick(a, cellRect(t, a, 2))
	want := []string{"Mail", "-", tr(StrAppPin), "-", tr(StrAppCloseAll)}
	if !eq(menuTexts(a), want) {
		t.Fatalf("меню %v, ждали %v", menuTexts(a), want)
	}
	items := a.menu.get().Items()
	items[2].OnClick()
	items[4].OnClick()
	if got := cat.Pinned(); got[len(got)-1] != "mail" {
		t.Errorf("закреплены %v, ждали «mail» в конце", got)
	}
	if len(wm.Closed) != 2 {
		t.Errorf("закрыто %v, ждали оба окна «Mail»", wm.Closed)
	}
}

func TestAppButtons_ConsumerCommandsReplaceDefaults(t *testing.T) {
	a, _, _ := groupScene(t, win10Fast(t))
	var seen AppButton
	ran := false
	a.SetCommands(AppCommandsFunc(func(b AppButton) []AppCommand {
		seen = b
		return []AppCommand{
			{Separator: true}, // лишние разделители по краям убираются
			{Title: "Мой пункт", Run: func() { ran = true }},
			{Separator: true},
		}
	}))
	rightClick(a, cellRect(t, a, 1))
	if seen.App != "term" || !seen.Pinned || len(seen.Windows) != 2 {
		t.Errorf("потребитель получил %+v", seen)
	}
	if want := []string{"Мой пункт"}; !eq(menuTexts(a), want) {
		t.Fatalf("меню %v, ждали %v", menuTexts(a), want)
	}
	a.menu.get().Items()[0].OnClick()
	if !ran {
		t.Error("команда потребителя не выполнилась")
	}

	// Пустой набор — меню нет.
	a.Dismiss()
	a.SetCommands(AppCommandsFunc(func(AppButton) []AppCommand { return nil }))
	rightClick(a, cellRect(t, a, 1))
	if a.HasOverlay() {
		t.Error("пустой набор команд открыл пустую рамку")
	}
}

// Меню рисует и разбирает сама область: пункт выбирается мышью, Esc и клик
// мимо закрывают, а меню не попадает в обход Tab.
func TestAppButtons_MenuMouseAndDismiss(t *testing.T) {
	a, cat, _ := groupScene(t, win10Fast(t))
	rightClick(a, cellRect(t, a, 0))
	ob := a.OverlayBounds()
	if ob.Empty() {
		t.Fatal("меню не открыто")
	}
	// Идём по высоте меню, пока какое-то нажатие не попадёт в первый пункт.
	x := ob.Min.X + ob.Dx()/2
	for y := ob.Min.Y; y < ob.Max.Y && a.HasOverlay(); y += 2 {
		a.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
		a.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
	}
	if len(cat.Launched) != 1 || cat.Launched[0] != "files" {
		t.Errorf("запущено %v, ждали [files] — пункт меню не нажался мышью", cat.Launched)
	}
	if a.HasOverlay() {
		t.Error("меню осталось открытым после выбора пункта")
	}

	rightClick(a, cellRect(t, a, 0))
	if !a.DismissOnEscape() || a.HasOverlay() {
		t.Error("Esc не закрыл меню")
	}
	rightClick(a, cellRect(t, a, 0))
	a.OnMouseButton(widget.MouseEvent{X: 5, Y: 5, Button: widget.MouseLeft, Pressed: true})
	if a.HasOverlay() {
		t.Error("клик мимо не закрыл меню")
	}
	for _, c := range widget.CollectFocusables(a) {
		if c != widget.Widget(a) {
			t.Errorf("в обход Tab попал посторонний виджет %T", c)
		}
	}
}

func TestAppButtons_ContextKeyOpensMenuOfFocusedCell(t *testing.T) {
	a, _, _ := groupScene(t, win10Fast(t))
	a.SetFocused(true)
	a.FocusState.SetCell(2)
	a.OnKeyEvent(widget.KeyEvent{Code: widget.KeyMenu, Pressed: true})
	if !a.HasOverlay() || menuTexts(a)[0] != "Mail" {
		t.Errorf("клавиша меню: открыто=%v пункты=%v", a.HasOverlay(), menuTexts(a))
	}
	a.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	if a.HasOverlay() {
		t.Error("Esc не закрыл меню, открытое с клавиатуры")
	}
}

// Строки команд — widget.Tr: переопределяются и следуют за языком.
func TestAppButtons_DefaultCommandStringsAreTranslatable(t *testing.T) {
	for _, key := range []string{StrAppPin, StrAppUnpin, StrAppCloseWindow, StrAppCloseAll, StrAppWindows, StrAppCloseTip} {
		for _, lang := range []string{"RU", "EN"} {
			if v := widget.TrIn(lang, key); v == key || v == "" {
				t.Errorf("ключ %s не переведён на %s", key, lang)
			}
		}
	}
	cmds := DefaultAppCommands(nil, nil, AppButton{App: "x", Title: "X"})
	if len(cmds) != 0 {
		t.Errorf("без каталога и модели команд быть не должно: %+v", cmds)
	}
}

// ─── Подсказка ───────────────────────────────────────────────────────────────

func TestAppButtons_ToolTips(t *testing.T) {
	tm := win10Fast(t)
	a, _, wm := groupScene(t, tm)
	at := func(i int) string {
		r := cellRect(t, a, i)
		return a.ToolTipAt(r.Min.X+3, r.Min.Y+3)
	}

	if got := at(0); got != "Files" {
		t.Errorf("подсказка незапущенного: %q, ждали «Files»", got)
	}
	// Без слушателя наведения предпросмотра нет — подсказка нужна и живым.
	if got := at(3); got != "Calc" {
		t.Errorf("подсказка живой кнопки без предпросмотра: %q", got)
	}
	if got, want := at(1), widget.TrIn(DefaultLanguage, StrAppWindows); got == "" || got == want {
		t.Errorf("подсказка стопки: %q", got)
	}

	// Предпросмотр подключён и умеет миниатюры — живым кнопкам подсказка не
	// нужна, незапущенным по-прежнему нужна.
	p := NewWindowPreview(tm, wm)
	p.Track(a)
	t.Cleanup(p.Close)
	if got := at(3); got != "" {
		t.Errorf("у кнопки с предпросмотром подсказка %q, ждали пусто", got)
	}
	if got := at(0); got != "Files" {
		t.Errorf("у незапущенного при предпросмотре подсказка %q, ждали «Files»", got)
	}
	if got := a.ToolTipAt(5, 5); got != "" {
		t.Errorf("подсказка вне кнопок: %q", got)
	}
}

// Трей: у каждого значка подсказка уже есть — проверка, что не потеряна.
func TestAppButtons_TrayItemsHaveToolTips(t *testing.T) {
	tm := win10Fast(t)
	st := NewFakeSystemStatus()
	for name, tip := range map[string]string{
		"сеть":    NewNetworkStatus(tm, st).GetToolTip(),
		"звук":    NewVolumeStatus(tm, st).GetToolTip(),
		"питание": NewPowerStatus(tm, st).GetToolTip(),
		"часы":    NewClock(tm, NewFakeClock(time.Date(2026, 3, 14, 15, 9, 26, 0, time.UTC))).GetToolTip(),
	} {
		if tip == "" {
			t.Errorf("у значка трея %q нет подсказки", name)
		}
	}
}

// ─── Состояния на кадре ──────────────────────────────────────────────────────

// renderArea рисует область на тёмном фоне панели.
func renderArea(t *testing.T, a *ApplicationArea, w, h int) *image.RGBA {
	t.Helper()
	root := widget.NewPanel(color.RGBA{R: 31, G: 31, B: 31, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	root.AddChild(a)
	eng := engine.New(w, h, 30)
	eng.SetRoot(root)
	return eng.RenderOnce()
}

func areaAt(t *testing.T, tm *theme.Manager, wins ...WindowInfo) (*ApplicationArea, *StaticAppCatalog) {
	t.Helper()
	cat := NewStaticAppCatalog(AppInfo{ID: "files", Title: "Files"}, AppInfo{ID: "term", Title: "Term"},
		AppInfo{ID: "mail", Title: "Mail"})
	cat.Pin("files")
	cat.Pin("term")
	a := NewApplicationArea(tm, cat, NewFakeWindowModel(wins...))
	a.SetBounds(image.Rect(0, 0, 400, 40))
	t.Cleanup(a.Close)
	return a, cat
}

var panelBG = color.RGBA{R: 31, G: 31, B: 31, A: 255}

func TestAppButtons_Windows10StateMarks(t *testing.T) {
	tm := managerFor(t, theme.ProfileWindows10)
	a, _ := areaAt(t, tm,
		WindowInfo{ID: 1, AppID: "term", Title: "t", Active: false},
		WindowInfo{ID: 2, AppID: "mail", Title: "m", Active: true},
		WindowInfo{ID: 3, AppID: "mail", Title: "m2"},
	)
	// files (не запущен), term (запущен), mail (стопка, активна).
	img := renderArea(t, a, 400, 40)
	files, term, mail := cellRect(t, a, 0), cellRect(t, a, 1), cellRect(t, a, 2)
	row := func(r image.Rectangle) int { return r.Max.Y - 1 }
	cx := func(r image.Rectangle) int { return r.Min.X + r.Dx()/2 }

	if got := img.RGBAAt(cx(files), row(files)); got != panelBG {
		t.Errorf("у закреплённого незапущенного есть линия: %v", got)
	}
	// Запущено: короткая линия по центру, по краям кнопки фона.
	if got := img.RGBAAt(cx(term), row(term)); got == panelBG {
		t.Error("у запущенного нет линии снизу")
	}
	if got := img.RGBAAt(term.Min.X+3, row(term)); got != panelBG {
		t.Errorf("линия запущенного идёт до края кнопки: %v", got)
	}
	// Активно: линия на всю ширину акцентом и подсветка фона.
	accent := tm.GetStyle(ComponentTaskButton, "", theme.StateActive).Border
	for _, x := range []int{mail.Min.X, cx(mail), mail.Max.X - 1} {
		if got := img.RGBAAt(x, row(mail)); got != accent {
			t.Errorf("линия активного в x=%d: %v, ждали акцент %v", x, got, accent)
		}
	}
	if got := img.RGBAAt(mail.Min.X+2, mail.Min.Y+4); got == panelBG {
		t.Error("у активной кнопки нет подсветки фона")
	}
	if got := img.RGBAAt(term.Min.X+2, term.Min.Y+4); got != panelBG {
		t.Errorf("у неактивной подсветка фона: %v", got)
	}
}

// Наведение подсвечивает и закреплённое незапущенное, и свёрнутое окно.
func TestAppButtons_Windows10HoverHighlightsEverything(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows10)
	a, _ := areaAt(t, tm, WindowInfo{ID: 1, AppID: "term", Title: "t", Minimized: true})
	for i := 0; i < 2; i++ { // files — не запущен, term — свёрнут
		r := cellRect(t, a, i)
		a.OnMouseMove(r.Min.X+4, r.Min.Y+4)
		renderArea(t, a, 400, 40)
		finishAnimations() // переход цвета доведён до конца
		img := renderArea(t, a, 400, 40)
		if got := img.RGBAAt(r.Min.X+4, r.Min.Y+4); got == panelBG {
			t.Errorf("ячейка %d: наведение не подсвечено", i)
		}
		// Подсветка прямоугольная: угол кнопки закрашен, как и середина.
		if got := img.RGBAAt(r.Min.X, r.Min.Y); got == panelBG {
			t.Errorf("ячейка %d: угол подсветки не закрашен — скругление", i)
		}
		a.OnMouseMove(300, 20)
	}
}

func TestAppButtons_StackMarkOnlyOnStacks(t *testing.T) {
	tm := managerFor(t, theme.ProfileWindows10)
	a, _ := areaAt(t, tm,
		WindowInfo{ID: 1, AppID: "term", Title: "t"},
		WindowInfo{ID: 2, AppID: "mail", Title: "m"},
		WindowInfo{ID: 3, AppID: "mail", Title: "m2"},
	)
	img := renderArea(t, a, 400, 40)
	term, mail := cellRect(t, a, 1), cellRect(t, a, 2)
	// Торцы правее значка (24 px по центру кнопки в 48 px).
	probe := func(r image.Rectangle) color.RGBA {
		return img.RGBAAt(r.Min.X+r.Dx()/2+int(tm.GetMetric(KeyTaskButtonIconSize))/2+int(tm.GetMetric(KeyTaskButtonStackOffset)), r.Min.Y+r.Dy()/2)
	}
	if probe(term) != panelBG {
		t.Error("у одиночного окна нарисована стопка")
	}
	if probe(mail) == panelBG {
		t.Error("у стопки торцы не нарисованы")
	}
}

// Метки прежних тем не изменились: Windows 11 рисует короткую чёрточку у
// активного, а у запущенного неактивного — вдвое короче, как и раньше.
func TestAppButtons_Windows11MarksUnchanged(t *testing.T) {
	tm := managerFor(t, theme.ProfileWindows11)
	a, _ := areaAt(t, tm,
		WindowInfo{ID: 1, AppID: "term", Title: "t", Active: true},
		WindowInfo{ID: 2, AppID: "mail", Title: "m"},
	)
	img := renderArea(t, a, 400, 40)
	term, mail := cellRect(t, a, 1), cellRect(t, a, 2)
	if got := img.RGBAAt(term.Min.X+term.Dx()/2, term.Max.Y-1); got == panelBG {
		t.Error("Windows 11: чёрточка активного исчезла")
	}
	if got := img.RGBAAt(mail.Min.X+mail.Dx()/2, mail.Max.Y-1); got == panelBG {
		t.Error("Windows 11: чёрточка запущенного исчезла")
	}
	if tm.GetMetric(KeyTaskButtonUnderlineIdleLen) != 0 {
		t.Error("Windows 11 получила длину линии неактивного")
	}
}

// ─── «Пуск» горит, пока меню открыто ────────────────────────────────────────

func TestStartButton_ActiveWhileFlyoutOpen(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows10)
	sb := NewStartButton(tm)
	sb.SetBounds(image.Rect(0, 0, 48, 40))
	f := NewFlyout(tm, ComponentStartMenu)
	f.Size = func() image.Point { return image.Pt(100, 100) }
	untrack := sb.Track(f)

	if sb.Active() {
		t.Fatal("кнопка горит при закрытом меню")
	}
	before := renderStart(t, sb)
	f.Open(image.Rect(0, 560, 48, 600))
	if !sb.Active() {
		t.Fatal("меню открыто, а кнопка не горит")
	}
	renderStart(t, sb)
	finishAnimations() // переход цвета доведён до конца
	if renderStart(t, sb).RGBAAt(40, 4) == before.RGBAAt(40, 4) {
		t.Error("состояние Active не видно на кадре: у кнопки нет стиля")
	}
	// Закрыто чем угодно — не только кнопкой.
	f.Close()
	if sb.Active() {
		t.Error("меню закрыто, а кнопка горит")
	}
	untrack()
	f.Open(image.Rect(0, 560, 48, 600))
	if sb.Active() {
		t.Error("после разрыва связи кнопка следит за меню")
	}
}

func TestStartButton_TrackManager(t *testing.T) {
	tm := managerFor(t, theme.ProfileWindows10)
	sb := NewStartButton(tm)
	mgr := NewFlyoutManager()
	start, other := NewFlyout(tm, ComponentStartMenu), NewFlyout(tm, ComponentStartMenu)
	for _, f := range []*Flyout{start, other} {
		f.Size = func() image.Point { return image.Pt(100, 100) }
	}
	mgr.Register("start", start)
	mgr.Register("other", other)
	sb.TrackManager(mgr, "start")

	anchor := image.Rect(0, 560, 48, 600)
	mgr.Open("start", anchor)
	if !sb.Active() {
		t.Fatal("«Пуск» открыт, кнопка не горит")
	}
	// Открыли другую панель — менеджер закрыл «Пуск», кнопка погасла.
	mgr.Open("other", anchor)
	if sb.Active() {
		t.Error("«Пуск» закрыт открытием соседней панели, а кнопка горит")
	}
	mgr.Open("start", anchor)
	start.DismissAt(500, 100) // клик мимо
	if sb.Active() {
		t.Error("«Пуск» закрыт кликом мимо, а кнопка горит")
	}
}

// Кнопка, у которой меню уже открыто к моменту связывания, горит сразу.
func TestStartButton_TrackSyncsInitialState(t *testing.T) {
	tm := managerFor(t, theme.ProfileWindows10)
	f := NewFlyout(tm, ComponentStartMenu)
	f.Size = func() image.Point { return image.Pt(100, 100) }
	f.Open(image.Rect(0, 560, 48, 600))
	sb := NewStartButton(tm)
	sb.Track(f)
	if !sb.Active() {
		t.Error("меню было открыто до связывания, а кнопка не горит")
	}
	sb.Track(nil) // не должно падать
}

func renderStart(t *testing.T, sb *StartButton) *image.RGBA {
	t.Helper()
	root := widget.NewPanel(color.RGBA{R: 31, G: 31, B: 31, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 100, 40))
	root.AddChild(sb)
	eng := engine.New(100, 40, 30)
	eng.SetRoot(root)
	return eng.RenderOnce()
}

// Другие темы не получили «горящую» кнопку молча: у Windows 2000 она
// вдавлена, как и при нажатии.
func TestStartButton_ActiveStyleExistsInClassicThemes(t *testing.T) {
	for _, name := range []string{theme.ProfileWindows2000, theme.ProfileWindows11, theme.ProfileWindows10} {
		tm := managerFor(t, name)
		act := tm.GetStyle(ComponentStartButton, "", theme.StateActive)
		norm := tm.GetStyle(ComponentStartButton, "", theme.StateNormal)
		if reflect.DeepEqual(act, norm) {
			t.Errorf("%s: у кнопки «Пуск» нет стиля Active", name)
		}
	}
}

// Подсказка часов — дата словами; заданная оболочкой побеждает.
func TestClockToolTip_LongDateUnlessSet(t *testing.T) {
	c := NewClock(win10Fast(t), NewFakeClock(time.Date(2026, 3, 14, 15, 9, 26, 0, time.UTC)))
	if got := c.GetToolTip(); !strings.Contains(got, "2026") || !strings.Contains(got, "14") {
		t.Errorf("подсказка часов %q: ждали дату словами", got)
	}
	c.SetToolTip("свой текст")
	if got := c.GetToolTip(); got != "свой текст" {
		t.Errorf("заданная подсказка потеряна: %q", got)
	}
}
