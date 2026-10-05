package desktop

import (
	"image"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Кнопки панели Windows 11: виджеты, Task View, поиск, группа трея, колокольчик.

func w11key(code widget.KeyCode) widget.KeyEvent { return widget.KeyEvent{Code: code, Pressed: true} }

func center(r image.Rectangle) (int, int) { return r.Min.X + r.Dx()/2, r.Min.Y + r.Dy()/2 }

// click щёлкает левой кнопкой по центру прямоугольника r элемента w.
func click11(w interface {
	OnMouseButton(widget.MouseEvent) bool
}, x, y int) {
	w.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	w.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
}

// ─── Порядок обхода и клавиатура ─────────────────────────────────────────────

// Tab идёт по порядку на панели слева направо: виджеты, «Пуск», поиск, Task
// View, кнопки окон, группа трея, колокольчик — и колокольчик Windows 11 в нём
// есть, а у Windows 10 его по-прежнему нет.
func TestWin11Focus_TabOrderLeftToRight(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	var got []widget.Widget
	for _, w := range widget.CollectFocusables(s.bar) {
		got = append(got, w)
	}
	want := []widget.Widget{s.widgets, s.start, s.search, s.taskview, s.area, s.group, s.bell}
	if len(got) != len(want) {
		t.Fatalf("остановок Tab %d, ждали %d: %T", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("остановка %d: %T, ждали %T", i, got[i], want[i])
		}
	}

	// Windows 10: колокольчик не входит в обход (как и раньше).
	tm10 := win10Fast(t)
	nb := NewNotificationButton(tm10, nil)
	nb.SetBounds(image.Rect(0, 0, 40, 40))
	if nb.TabIndex() >= 0 {
		t.Error("кнопка уведомлений Windows 10 стала остановкой Tab")
	}
}

// Enter и Space нажимают кнопки с клавиатуры, стрелки идут по области, рамка
// фокуса видна и гаснет от щелчка мыши.
func TestWin11Focus_KeysActivateAndArrowsMove(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)

	s.start.SetFocused(true)
	s.start.OnKeyEvent(w11key(widget.KeyRight))
	if !s.search.IsFocused() || s.start.IsFocused() {
		t.Fatal("стрелка вправо не перенесла фокус от «Пуска» к поиску")
	}
	s.search.OnKeyEvent(w11key(widget.KeyRight))
	if !s.taskview.IsFocused() {
		t.Fatal("стрелка вправо не перенесла фокус к Task View")
	}
	s.taskview.OnKeyEvent(w11key(widget.KeyEnter))
	s.taskview.OnKeyEvent(w11key(widget.KeySpace))
	if s.clicks["taskview"] != 2 {
		t.Errorf("Enter и Space нажали Task View %d раз, ждали 2", s.clicks["taskview"])
	}
	s.taskview.OnKeyEvent(w11key(widget.KeyLeft))
	if !s.search.IsFocused() {
		t.Error("стрелка влево не вернула фокус к поиску")
	}

	for name, f := range map[string]interface {
		SetFocused(bool)
		OnKeyEvent(widget.KeyEvent)
	}{"widgets": s.widgets, "group": s.group, "bell": s.bell} {
		f.SetFocused(true)
		f.OnKeyEvent(w11key(widget.KeyEnter))
		if s.clicks[name] != 1 {
			t.Errorf("%s: Enter нажал %d раз, ждали 1", name, s.clicks[name])
		}
		f.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true, Repeat: true})
		if s.clicks[name] != 1 {
			t.Errorf("%s: автоповтор Enter нажал снова", name)
		}
	}
}

// Рамка фокуса Task View видна с клавиатуры и убирается щелчком мыши.
func TestWin11Focus_RingShowsForKeyboardOnly(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	plain := s.render(1)
	s.taskview.SetFocused(true)
	ring := s.render(1)
	if imagesEqualRGBA(plain, ring) {
		t.Fatal("рамка фокуса Task View не нарисована")
	}
	// Всё отличие внутри кнопки (рамка не вылезает за её границы).
	b := s.taskview.Bounds()
	for y := 0; y < w11H; y++ {
		for x := 0; x < w11W; x++ {
			if plain.RGBAAt(x, y) != ring.RGBAAt(x, y) && !image.Pt(x, y).In(b) {
				t.Fatalf("рамка вышла за границы кнопки %v: пиксель %d,%d", b, x, y)
			}
		}
	}
	cx, cy := center(b)
	click11(s.taskview, cx, cy)
	if after := s.render(1); !imagesEqualRGBA(after, plain) {
		t.Error("после щелчка мышью рамка фокуса осталась")
	}
}

// ─── Группа значков трея ─────────────────────────────────────────────────────

// Один щелчок по любому значку группы, общая подсветка при наведении, подсказка
// у каждого значка своя.
func TestWin11TrayGroup_OneButton(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	items := s.group.Items()
	for i, it := range items {
		cx, cy := center(it.Bounds())
		click11(s.group, cx, cy)
		if s.clicks["group"] != i+1 {
			t.Errorf("щелчок по значку %d: кнопка нажата %d раз", i, s.clicks["group"])
		}
	}

	// Подсветка — одна плашка на всю группу.
	g := s.group.Bounds()
	s.group.OnMouseMove(g.Min.X+2, g.Min.Y+2)
	ctx := &recCtx{}
	s.group.Draw(ctx)
	whole := false
	for _, f := range ctx.fills {
		if f.w == g.Dx() && f.h == g.Dy() {
			whole = true
		}
	}
	if !whole {
		t.Errorf("подсветка не на всю группу %v: %+v", g, ctx.fills[:min(4, len(ctx.fills))])
	}
	// Наведение на другой значок группы — та же плашка, а не отдельная.
	nx, ny := center(items[0].Bounds())
	s.group.OnMouseMove(nx, ny)
	ctx = &recCtx{}
	s.group.Draw(ctx)
	for _, f := range ctx.fills {
		if f.h == g.Dy() && f.w != g.Dx() && f.w > 20 {
			t.Errorf("отдельная плашка значка %+v вместо общей", f)
		}
	}

	// Подсказки.
	if tip := s.group.ToolTipAt(nx, ny); !strings.Contains(tip, "home") {
		t.Errorf("подсказка сети %q", tip)
	}
	vx, vy := center(items[1].Bounds())
	if tip := s.group.ToolTipAt(vx, vy); !strings.Contains(tip, "60") {
		t.Errorf("подсказка звука %q", tip)
	}
	if tip := s.group.ToolTipAt(g.Min.X-50, g.Min.Y); tip != "" {
		t.Errorf("подсказка вне группы %q", tip)
	}
}

// Значок без батареи места не занимает.
func TestWin11TrayGroup_NoBatteryIsNarrower(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	full := s.group.Bounds().Dx()
	s.status.SetPower(PowerState{NoBattery: true})
	if got := s.group.PreferredSize(image.Pt(400, 48)).X; got != full-24 {
		t.Errorf("ширина группы без батареи %d, ждали %d", got, full-24)
	}
}

// Группа горит, пока открыта панель, за которой она следит.
func TestWin11TrayGroup_TracksPanel(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	f := NewFlyout(s.tm, ComponentQuickSettings)
	f.Size = func() image.Point { return image.Pt(300, 300) }
	f.Screen = image.Rect(0, 0, w11W, w11H+400)
	untrack := s.group.Track(f)
	defer untrack()
	if s.group.Active() {
		t.Fatal("группа горит при закрытой панели")
	}
	f.Open(s.group.Bounds())
	if !s.group.Active() {
		t.Error("группа не загорелась при открытии панели")
	}
	f.Close()
	if s.group.Active() {
		t.Error("группа не погасла при закрытии панели")
	}
}

// ─── Колокольчик: «Не беспокоить» ────────────────────────────────────────────

type dndSource struct {
	*FakeNotifications
	on bool
}

func (d *dndSource) DoNotDisturb() bool { return d.on }

func TestWin11Bell_DoNotDisturb(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	normal := &recCtx{}
	s.bell.Draw(normal)
	if !containsText(normal.texts, "4") {
		t.Fatal("счётчик 4 не нарисован")
	}
	frame := s.render(2)

	s.bell.SetDoNotDisturb(true)
	if !s.bell.DoNotDisturb() {
		t.Fatal("SetDoNotDisturb не включил режим")
	}
	dnd := &recCtx{}
	s.bell.Draw(dnd)
	if containsText(dnd.texts, "4") {
		t.Error("счётчик остался при «Не беспокоить»")
	}
	if tip := s.bell.GetToolTip(); tip != tr(StrTrayDND) || tip == "" {
		t.Errorf("подсказка в «Не беспокоить» %q", tip)
	}
	if imagesEqualRGBA(frame, s.render(2)) {
		t.Error("«Не беспокоить» не изменил значок")
	}

	// Источник, сообщающий режим сам, главнее ручного флага.
	src := &dndSource{FakeNotifications: NewFakeNotifications()}
	b := NewNotificationButton(s.tm, src)
	if b.DoNotDisturb() {
		t.Error("режим включён без источника")
	}
	src.on = true
	if !b.DoNotDisturb() {
		t.Error("режим источника не прочитан")
	}
}

// ─── Виджеты ─────────────────────────────────────────────────────────────────

// Содержимое от потребителя: текст меняет ширину кнопки, панель перекладывается
// сама; без текста — один значок; подсказка потребителя главнее умолчания.
func TestWin11Widgets_ContentChangesWidth(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	withText := s.widgets.Bounds().Dx()
	if withText <= 40 {
		t.Fatalf("кнопка с погодой %d шириной, ждали больше значка", withText)
	}
	s.widgets.SetContent(WidgetsContent{})
	if got := s.widgets.Bounds().Dx(); got != 40 {
		t.Errorf("кнопка без текста %d, ждали 40", got)
	}
	if tip := s.widgets.GetToolTip(); tip != tr(StrWidgets) {
		t.Errorf("подсказка по умолчанию %q", tip)
	}
	s.widgets.SetContent(WidgetsContent{Temperature: "-3°", Caption: "Снег", ToolTip: "Погода: снег"})
	if got := s.widgets.Bounds().Dx(); got <= 40 {
		t.Errorf("кнопка с новым текстом %d", got)
	}
	if tip := s.widgets.GetToolTip(); tip != "Погода: снег" {
		t.Errorf("подсказка потребителя %q", tip)
	}
	// Слот стоит у левого края: группа центруется правее него.
	if s.widgets.Bounds().Min.X > s.bar.Bounds().Min.X+8 {
		t.Errorf("кнопка виджетов не у левого края: %v", s.widgets.Bounds())
	}
	if s.start.Bounds().Min.X < s.widgets.Bounds().Max.X {
		t.Errorf("«Пуск» %v наехал на виджеты %v", s.start.Bounds(), s.widgets.Bounds())
	}
}

// Длинный текст не раздувает кнопку выше предела widgets.width.max.
func TestWin11Widgets_WidthIsCapped(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	s.widgets.SetContent(WidgetsContent{Temperature: strings.Repeat("W", 60), Caption: strings.Repeat("w", 80)})
	limit := int(s.tm.GetMetric("widgets.width.max"))
	if got := s.widgets.Bounds().Dx(); got > limit {
		t.Errorf("ширина %d больше предела %d", got, limit)
	}
	_ = s.render(1) // усечённый текст рисуется без паники
}

// ─── Task View ───────────────────────────────────────────────────────────────

func TestWin11TaskView_ClickTooltipAndState(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	cx, cy := center(s.taskview.Bounds())
	click11(s.taskview, cx, cy)
	if s.clicks["taskview"] != 1 {
		t.Errorf("щелчок нажал %d раз", s.clicks["taskview"])
	}
	// Отпускание в стороне — отмена.
	s.taskview.OnMouseButton(widget.MouseEvent{X: cx, Y: cy, Button: widget.MouseLeft, Pressed: true})
	s.taskview.OnMouseButton(widget.MouseEvent{X: cx + 100, Y: cy, Button: widget.MouseLeft})
	if s.clicks["taskview"] != 1 {
		t.Error("отпускание вне кнопки сработало")
	}
	if tip := s.taskview.GetToolTip(); tip != tr(StrTaskView) || tip == "" {
		t.Errorf("подсказка %q", tip)
	}
	// Состояния: наведение, нажатие и «обзор открыт» рисуются по-разному.
	draw := func() string {
		s.taskview.Draw(&recCtx{}) // рисование заводит переход цвета
		finishAnimations()
		c := &recCtx{}
		s.taskview.Draw(c)
		return frameKey(c)
	}
	rest := draw()
	s.taskview.OnMouseMove(cx, cy)
	hover := draw()
	s.taskview.SetActive(true)
	active := draw()
	if rest == hover || hover == active {
		t.Error("состояния Task View не различаются")
	}
	// Смена языка меняет подсказку без пересоздания.
	widget.SetLanguage("EN")
	defer widget.SetLanguage("RU")
	if tip := s.taskview.GetToolTip(); tip != "Task View" {
		t.Errorf("подсказка на английском %q", tip)
	}
}

// ─── Поиск ───────────────────────────────────────────────────────────────────

// «Значок и подпись»: Enter и Space открывают поиск, как щелчок; набор текста
// уходит в запрос; в поле Enter по-прежнему отправляет запрос.
func TestWin11Search_IconAndLabelActivates(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	s.search.SetMode(SearchModeIconAndLabel)
	s.search.OnKeyEvent(w11key(widget.KeyEnter))
	s.search.OnKeyEvent(w11key(widget.KeySpace))
	if s.clicks["search"] != 2 {
		t.Errorf("Enter и Space открыли поиск %d раз, ждали 2", s.clicks["search"])
	}
	cx, cy := center(s.search.Bounds())
	click11(s.search, cx, cy)
	if s.clicks["search"] != 3 {
		t.Errorf("щелчок открыл поиск %d раз всего, ждали 3", s.clicks["search"])
	}
	s.search.OnKeyEvent(widget.KeyEvent{Code: widget.KeyA, Rune: 'a', Pressed: true})
	if s.search.Text() != "a" {
		t.Errorf("набор текста не дошёл до запроса: %q", s.search.Text())
	}

	// Поле: Enter отправляет запрос, а не открывает поиск.
	s.search.SetText("")
	s.search.SetMode(SearchModeBox)
	submitted := ""
	s.search.OnSubmit = func(q string) { submitted = q + "!" }
	before := s.clicks["search"]
	s.search.OnKeyEvent(w11key(widget.KeyEnter))
	if s.clicks["search"] != before || submitted != "!" {
		t.Errorf("Enter в поле: открыто %d→%d, отправлено %q", before, s.clicks["search"], submitted)
	}
	// Подпись и короткая подсказка берутся из строк интерфейса.
	if hint := s.search.placeholder(); hint != tr(StrSearchLabel) {
		t.Errorf("подсказка пустого поля %q, ждали короткую %q", hint, tr(StrSearchLabel))
	}
}

// Windows 10: подсказка поля прежняя, длинная.
func TestWin10Search_PlaceholderUnchanged(t *testing.T) {
	b := NewSearchBox(win10Fast(t), nil)
	if got := b.placeholder(); got != tr(StrSearchPlaceholder) {
		t.Errorf("подсказка поля Windows 10 %q", got)
	}
}

// ─── Края, слот виджетов, масштабы ───────────────────────────────────────────

// У бокового края слот виджетов — самый верх столбца, над «Пуском».
func TestWin11Bar_WidgetsSlotAtTopOfColumn(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	s.bar.SetEdge(EdgeLeft)
	s.bar.SetBounds(image.Rect(0, 0, 62, 400))
	w, st := s.widgets.Bounds(), s.start.Bounds()
	if w.Empty() || st.Empty() || w.Min.Y >= st.Min.Y {
		t.Errorf("виджеты %v не над «Пуском» %v в столбце", w, st)
	}
}

// 100–200 %: пилюля активной кнопки, счётчик и полоса прогресса видны на кадре
// целиком, ничем не обрезаны.
func TestWin11Indicators_VisibleAtScales(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 1.5, 1.75, 2} {
		s := newW11Scene(t, theme.ProfileWindows11)
		img := s.render(scale)
		px := func(x, y int) (r, g, b uint8) {
			c := img.RGBAAt(int(float64(x)*scale), int(float64(y)*scale))
			return c.R, c.G, c.B
		}
		accent := s.tm.GetStyle(ComponentTaskButton, PartTaskPill, theme.StateActive).Fill
		isAccent := func(x, y int) bool {
			r, g, b := px(x, y)
			d := func(a, b uint8) int {
				if a > b {
					return int(a - b)
				}
				return int(b - a)
			}
			return d(r, accent.R) < 24 && d(g, accent.G) < 24 && d(b, accent.B) < 24
		}
		cell := s.area.ButtonRect(0)
		pill := pillRect(s.tm, cell, 16)
		if x, y := center(pill); !isAccent(x, y) {
			t.Errorf("масштаб %v: пилюля активной кнопки не видна в %d,%d", scale, x, y)
		}
		// Её края не обрезаны: крайние пиксели пилюли акцентные.
		if !isAccent(pill.Min.X+1, pill.Min.Y+1) || !isAccent(pill.Max.X-2, pill.Min.Y+1) {
			t.Errorf("масштаб %v: пилюля обрезана по краям %v", scale, pill)
		}
		// Счётчик «3» на кнопке Mail (кружок цвета акцента внутри кнопки).
		mail := s.area.ButtonRect(3)
		if !isAccent(mail.Max.X-8, mail.Min.Y+9) {
			t.Errorf("масштаб %v: кружок счётчика не виден", scale)
		}
	}
}

// Смена масштаба холста на живой панели не ломает раскладку: логические границы
// те же.
func TestWin11Bar_LayoutIndependentOfScale(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	before := s.area.ButtonRect(2)
	s.render(1.5)
	s.render(2)
	if got := s.area.ButtonRect(2); got != before {
		t.Errorf("кнопка сдвинулась при смене масштаба: %v → %v", before, got)
	}
}
