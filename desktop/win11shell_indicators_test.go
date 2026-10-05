package desktop

import (
	"fmt"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Индикаторы кнопок окон Windows 11: пилюля, прогресс, счётчик, «внимание».

// areaFills рисует область приложений на записывающем контексте.
func areaFills(a *ApplicationArea) *recCtx {
	ctx := &recCtx{}
	a.Draw(ctx)
	return ctx
}

// pillWidth — ширина пилюли в ячейке cell по записанным заливкам (0 — нет).
func pillWidth(c *recCtx, cell image.Rectangle) int {
	for _, f := range c.fills {
		if f.h == 3 && f.x >= cell.Min.X && f.x+f.w <= cell.Max.X && f.y >= cell.Max.Y-4 {
			return f.w
		}
	}
	return 0
}

// Запущено — 6 серым, активно — 16 акцентом, закреплённое незапущенное — без
// пилюли; цвет активной — акцент темы.
func TestWin11Pill_WidthsAndColor(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	s.area.layout()
	c := areaFills(s.area)
	want := []int{16, 6, 6, 6, 6, 6, 6, 0}
	for i, w := range want {
		if got := pillWidth(c, s.area.ButtonRect(i)); got != w {
			t.Errorf("кнопка %d: пилюля %d, ждали %d", i, got, w)
		}
	}
	accent := s.tm.GetStyle(ComponentTaskButton, PartTaskPill, theme.StateActive).Fill
	for _, f := range c.fills {
		if f.h == 3 && f.w == 16 && f.col != accent {
			t.Errorf("пилюля активного окна %v, ждали акцент %v", f.col, accent)
		}
		if f.h == 3 && f.w == 6 && f.col.A >= 255 {
			t.Errorf("пилюля запущенного непрозрачна (%v): должна быть серой", f.col)
		}
	}
}

// Смена активного окна плавно меняет ширину пилюль; каждый шаг перерисовывает
// только две затронутые кнопки.
func TestWin11Pill_AnimatesAndRepaintsOnlyItsButtons(t *testing.T) {
	defer widget.StopAllAnimations()
	s := newW11Scene(t, theme.ProfileWindows11)
	s.area.layout()
	areaFills(s.area) // ячейки запомнены в покое

	var rects []image.Rectangle
	fulls := 0
	h := widget.RegisterUINotifierWake(func() { fulls++ }, func(r image.Rectangle) { rects = append(rects, r) }, func() {})
	defer widget.UnregisterUINotifier(h)

	s.wm.Activate(2) // активной становится вторая кнопка («Files»)
	rects, fulls = nil, 0
	areaFills(s.area) // рисование заводит анимации
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(75 * time.Millisecond))
	c := areaFills(s.area)
	w1, w0 := pillWidth(c, s.area.ButtonRect(1)), pillWidth(c, s.area.ButtonRect(0))
	if w1 <= 6 || w1 >= 16 {
		t.Errorf("ширина растущей пилюли на середине перехода %d, ждали между 6 и 16", w1)
	}
	if w0 <= 6 || w0 >= 16 {
		t.Errorf("ширина сжимающейся пилюли на середине перехода %d, ждали между 6 и 16", w0)
	}
	if fulls != 0 {
		t.Errorf("шаги анимации вызвали %d полных перерисовок", fulls)
	}
	// Шаги перерисовывают только свои кнопки: две пилюли и мигающее «внимание»
	// шестой кнопки сцены; остальные не трогаются.
	inButton := func(r image.Rectangle) bool {
		for _, i := range []int{0, 1, 5} {
			if r.In(s.area.ButtonRect(i).Inset(-4)) {
				return true
			}
		}
		return false
	}
	for _, r := range rects {
		if !r.Empty() && !inButton(r) {
			t.Errorf("шаг анимации заявил область %v вне кнопок 0, 1 и 5", r)
		}
	}
	finishAnimations()
	c = areaFills(s.area)
	if got := pillWidth(c, s.area.ButtonRect(1)); got != 16 {
		t.Errorf("после перехода пилюля активной %d, ждали 16", got)
	}
	if got := pillWidth(c, s.area.ButtonRect(0)); got != 6 {
		t.Errorf("после перехода пилюля прежней активной %d, ждали 6", got)
	}
}

// «Меньше движения»: ширина меняется сразу, анимаций нет.
func TestWin11Pill_ReducedMotionIsInstant(t *testing.T) {
	defer widget.StopAllAnimations()
	s := newW11Scene(t, theme.ProfileWindows11)
	s.tm.SetFlag(theme.FlagMotionReduce, true)
	s.area.layout()
	areaFills(s.area)
	s.wm.Activate(2)
	c := areaFills(s.area)
	if got := pillWidth(c, s.area.ButtonRect(1)); got != 16 {
		t.Errorf("при «меньше движения» пилюля %d, ждали сразу 16", got)
	}
	if got := pillWidth(c, s.area.ButtonRect(0)); got != 6 {
		t.Errorf("при «меньше движения» прежняя пилюля %d, ждали сразу 6", got)
	}
	if widget.AnimationsActive() {
		t.Error("смена активного окна завела анимацию при «меньше движения»")
	}
}

// Наложение прогресса: дорожка во всю ширину значка и полоса долей; цвет по
// состоянию — акцент, жёлтый, красный.
func TestWin11Progress_StatesAndShare(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	s.area.layout()
	c := areaFills(s.area)
	bars := func(cell int) []recFill {
		r := s.area.ButtonRect(cell)
		var out []recFill
		for _, f := range c.fills {
			if f.h == 4 && f.x >= r.Min.X && f.x+f.w <= r.Max.X && f.y >= r.Min.Y {
				out = append(out, f)
			}
		}
		return out
	}
	part := func(p string) color.RGBA { return s.tm.GetStyle(ComponentTaskButton, p, theme.StateNormal).Fill }
	cases := []struct {
		cell  int
		share float64
		fill  color.RGBA
	}{
		{1, 0.45, part(PartTaskProgressFill)},
		{2, 0.70, part(PartTaskProgressPaused)},
		{4, 0.30, part(PartTaskProgressError)},
	}
	for _, tc := range cases {
		got := bars(tc.cell)
		if len(got) != 2 {
			t.Errorf("кнопка %d: заливок прогресса %d, ждали дорожку и полосу", tc.cell, len(got))
			continue
		}
		track, bar := got[0], got[1]
		if track.w != 24 {
			t.Errorf("кнопка %d: дорожка %d, ждали ширину значка 24", tc.cell, track.w)
		}
		if want := int(float64(24)*tc.share + 0.5); bar.w != want {
			t.Errorf("кнопка %d: полоса %d, ждали %d", tc.cell, bar.w, want)
		}
		if bar.col != tc.fill {
			t.Errorf("кнопка %d: цвет %v, ждали %v", tc.cell, bar.col, tc.fill)
		}
	}
	if len(bars(0)) != 0 || len(bars(3)) != 0 {
		t.Error("кнопки без прогресса получили полосу")
	}
}

// Счётчик: число в кружке; больше 99 — «99+»; у кнопки без счётчика его нет.
func TestWin11Badge_CountsAndCap(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	s.area.layout()
	c := areaFills(s.area)
	if !containsText(c.texts, "3") || !containsText(c.texts, "99+") {
		t.Errorf("счётчики «3» и «99+» не нарисованы: %+v", c.texts)
	}
	if containsText(c.texts, "120") {
		t.Error("счётчик 120 не усечён до «99+»")
	}
	// Кружок лежит внутри своей кнопки.
	accent := s.tm.GetStyle(ComponentTaskButton, PartTaskBadge, theme.StateNormal).Fill
	cells := map[int]string{3: "3", 6: "99+"}
	for cell := range cells {
		r := s.area.ButtonRect(cell)
		found := false
		for _, f := range c.fills {
			if f.col == accent && f.h == 14 && f.x >= r.Min.X && f.x+f.w <= r.Max.X && f.y >= r.Min.Y {
				found = true
			}
		}
		if !found {
			t.Errorf("кнопка %d: кружок счётчика не найден внутри кнопки %v", cell, r)
		}
	}
	s.wm.SetWindows([]WindowInfo{{ID: 4, AppID: "mail", Title: "Mail", Badge: 0}})
	if containsText(areaFills(s.area).texts, "3") {
		t.Error("счётчик остался после обнуления")
	}
}

// Сборка наложений стопки: ошибка важнее паузы и обычного, счётчики складываются,
// «внимание» не активного окна.
func TestOverlayOf_Aggregation(t *testing.T) {
	o := overlayOf([]WindowInfo{
		{ProgressState: ProgressNormal, Progress: 0.9, Badge: 2},
		{ProgressState: ProgressPaused, Progress: 0.3, Badge: 5},
		{ProgressState: ProgressPaused, Progress: 0.6},
		{Active: true, Attention: true},
	})
	if o.pstate != ProgressPaused || o.progress != 0.6 {
		t.Errorf("прогресс стопки %v/%v, ждали пауза 0.6", o.pstate, o.progress)
	}
	if o.badge != 7 {
		t.Errorf("счётчик стопки %d, ждали 7", o.badge)
	}
	if o.attention {
		t.Error("активное окно не вправе просить внимания")
	}
	o = overlayOf([]WindowInfo{{ProgressState: ProgressNormal, Progress: 7}, {ProgressState: ProgressError, Progress: -1}, {Attention: true}})
	if o.pstate != ProgressError || o.progress != 0 || !o.attention {
		t.Errorf("%+v: ждали ошибку с долей 0 (обрезка) и внимание", o)
	}
}

// attentionAlpha — сила подложки «внимания» по записанной заливке кнопки: её
// цвет оранжевый (красный канал сильнее зелёного, синего нет), ослабленный
// мерцанием; 0 — подложки нет.
func attentionAlpha(c *recCtx, cell image.Rectangle) int {
	for _, f := range c.fills {
		if f.w == cell.Dx() && f.h == cell.Dy() && f.col.B == 0 && f.col.R > f.col.G {
			return int(f.col.A)
		}
	}
	return 0
}

// «Внимание»: подложка мигает, затем остаётся постоянной; в «меньше движения»
// сразу постоянная; пропадает, когда окно просить перестаёт или становится
// активным.
func TestWin11Attention_BlinksThenSteady(t *testing.T) {
	defer widget.StopAllAnimations()
	s := newW11Scene(t, theme.ProfileWindows11)
	s.area.layout()
	cell := s.area.ButtonRect(5) // «Calc»
	steady := attentionAlpha(areaFills(s.area), cell)
	if steady <= 0 {
		t.Fatalf("подложка «внимания» не нарисована (%d)", steady)
	}
	// Начало мигания: сила полная; к четверти периода — спадает.
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(225 * time.Millisecond)) // полпериода первого мигания
	mid := attentionAlpha(areaFills(s.area), cell)
	if mid >= steady {
		t.Errorf("на середине мигания подложка %d, ждали слабее постоянной %d", mid, steady)
	}
	finishAnimations()
	if end := attentionAlpha(areaFills(s.area), cell); end != steady {
		t.Errorf("после мигания подложка %d, ждали постоянную %d", end, steady)
	}

	// Окно стало активным — подложки нет.
	s.wm.Activate(6)
	if got := attentionAlpha(areaFills(s.area), s.area.ButtonRect(5)); got != 0 {
		t.Errorf("у активного окна осталась подложка внимания (%d)", got)
	}

	// «Меньше движения»: сразу постоянная, анимаций нет.
	s2 := newW11Scene(t, theme.ProfileWindows11)
	s2.tm.SetFlag(theme.FlagMotionReduce, true)
	s2.area.layout()
	widget.StopAllAnimations()
	if got := attentionAlpha(areaFills(s2.area), s2.area.ButtonRect(5)); got != steady {
		t.Errorf("при «меньше движения» подложка %d, ждали сразу постоянную %d", got, steady)
	}
	if widget.AnimationsActive() {
		t.Error("«внимание» завело анимацию при «меньше движения»")
	}
}

// Смена прогресса и счётчика перерисовывает только их кнопку; смена заголовка —
// всю область.
func TestWin11Overlay_RepaintsOnlyChangedButton(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	s.area.layout()
	areaFills(s.area)
	var rects []image.Rectangle
	fulls := 0
	h := widget.RegisterUINotifierWake(func() { fulls++ }, func(r image.Rectangle) { rects = append(rects, r) }, func() {})
	defer widget.UnregisterUINotifier(h)

	wins := w11Windows()
	wins[1].Progress = 0.46 // «Files»: на процент дальше
	s.wm.SetWindows(wins)
	cell := s.area.ButtonRect(1)
	if fulls != 0 {
		t.Errorf("смена прогресса вызвала %d полных перерисовок", fulls)
	}
	if len(rects) != 1 || !rects[0].In(cell) {
		t.Errorf("заявлено %v, ждали только кнопку %v", rects, cell)
	}

	rects = nil
	wins[3].Badge = 4 // «Mail»
	s.wm.SetWindows(wins)
	if len(rects) != 1 || !rects[0].In(s.area.ButtonRect(3)) {
		t.Errorf("смена счётчика заявила %v, ждали кнопку %v", rects, s.area.ButtonRect(3))
	}

	rects = nil
	wins[2].Title = "Editor — other file" // состав видимого меняется — область целиком
	s.wm.SetWindows(wins)
	whole := false
	for _, r := range rects {
		whole = whole || r == s.area.Bounds()
	}
	if !whole {
		t.Errorf("смена заголовка заявила %v, ждали область целиком %v", rects, s.area.Bounds())
	}
}

// Профили без индикаторов (Windows 10, Windows 2000, macOS) не меняются, даже
// если модель заполняет прогресс, счётчик и «внимание»: рисунок кнопок тот же,
// что и без этих полей.
func TestOverlay_OtherProfilesIgnoreModelFields(t *testing.T) {
	plain := []WindowInfo{
		{ID: 1, AppID: "web", Title: "Browser", Active: true},
		{ID: 2, AppID: "mail", Title: "Inbox"},
		{ID: 3, AppID: "mail", Title: "Draft", Minimized: true},
		{ID: 4, AppID: "term", Title: "bash"},
	}
	rich := append([]WindowInfo(nil), plain...)
	rich[0].ProgressState, rich[0].Progress, rich[0].Badge, rich[0].Attention = ProgressError, 0.5, 7, true
	rich[1].Badge, rich[1].Attention = 3, true
	rich[3].ProgressState, rich[3].Progress = ProgressNormal, 0.2

	for _, name := range []string{theme.ProfileWindows10, theme.ProfileWindows2000, theme.ProfileMacOS} {
		var keys [2]string
		for i, wins := range [][]WindowInfo{plain, rich} {
			tm := managerFor(t, name)
			root := frozenBar(t, tm, 640, 120)
			bar := root.Children()[len(root.Children())-1].(*Taskbar)
			var area *ApplicationArea
			for _, it := range bar.Items(SlotApps) {
				area = it.(*ApplicationArea)
			}
			area.wm.(*FakeWindowModel).SetWindows(wins)
			area.layout()
			keys[i] = frameKey(areaFills(area))
		}
		if keys[0] != keys[1] {
			t.Errorf("%s: поля прогресса, счётчика и внимания изменили рисунок кнопок", name)
		}
	}
}

// frameKey — записанные заливки и тексты одной строкой для сравнения.
func frameKey(c *recCtx) string {
	var b []byte
	for _, f := range c.fills {
		b = append(b, fmt.Sprintf("%v;", f)...)
	}
	for _, tx := range c.texts {
		b = append(b, fmt.Sprintf("%v;", tx)...)
	}
	return string(b)
}
