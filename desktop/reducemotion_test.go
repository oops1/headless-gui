package desktop

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Режим «меньше движения» (theme.FlagMotionReduce): единая точка —
// Manager.GetAnimation, и все потребители токенов анимации получают нулевую
// длительность.

func reducedManager(t *testing.T, profile string) *theme.Manager {
	t.Helper()
	tm := managerFor(t, profile)
	tm.SetFlag(theme.FlagMotionReduce, true)
	return tm
}

func TestReduceMotion_AnimationHelperReturnsZero(t *testing.T) {
	tm := managerFor(t, theme.ProfileWindows11)
	if d, _ := animation(tm, AnimMenuOpen); d <= 0 {
		t.Fatal("без флага у Windows 11 есть выезд панели")
	}
	tm.SetFlag(theme.FlagMotionReduce, true)
	for _, k := range []theme.Key{AnimMenuOpen, AnimWindowOpen, AnimHover, AnimTaskbarItem} {
		if d, _ := animation(tm, k); d != 0 {
			t.Errorf("%s: длительность %v при «меньше движения»", k, d)
		}
	}
	if d, _ := itemAnimation(tm); d != 0 {
		t.Errorf("itemAnimation = %v", d)
	}
}

// Панель открывается и закрывается разом: ни движения, ни анимаций в реестре.
func TestReduceMotion_FlyoutIsInstant(t *testing.T) {
	defer widget.StopAllAnimations()
	f := motionFlyout(reducedManager(t, theme.ProfileWindows11))
	f.Open(motionAnchor)
	if f.Presence() != 1 || f.rect() != f.restRect() {
		t.Errorf("панель выезжает при «меньше движения»: presence=%v", f.Presence())
	}
	if widget.AnimationsActive() {
		t.Error("открытие панели завело анимацию")
	}
	f.Close()
	if f.HasOverlay() || f.Presence() != 0 {
		t.Error("панель закрывается не разом")
	}
}

// Число (боковая панель «Пуска») меняется сразу.
func TestReduceMotion_TweenIsInstant(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := reducedManager(t, theme.ProfileWindows11)
	changed := 0
	tw := NewTween(tm, AnimMenuOpen, 0, func() { changed++ })
	tw.To(1)
	if tw.Value() != 1 || tw.Animating() || changed != 1 {
		t.Errorf("Tween: значение %v, идёт %v, onChange %d", tw.Value(), tw.Animating(), changed)
	}
}

// Наведение: цвет меняется скачком, переход не заводится.
func TestReduceMotion_HoverHasNoTransition(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := reducedManager(t, theme.ProfileWindows11)
	get := buttonStyle(tm)
	normal, hover := get(theme.StateNormal), get(theme.StateHover)
	if normal.Fill == hover.Fill {
		t.Skip("в теме у наведения та же заливка")
	}
	var m motion
	r := image.Rect(10, 10, 50, 50)
	m.Style(tm, 1, r, theme.StateNormal, get)
	if got := m.Style(tm, 1, r, theme.StateHover, get); got.Fill != hover.Fill {
		t.Errorf("заливка %v, ждали сразу %v", got.Fill, hover.Fill)
	}
	if widget.AnimationsActive() {
		t.Error("наведение завело анимацию")
	}
}

// Автоскрытие: панель выезжает разом, а не за умолчание в 140 мс.
func TestReduceMotion_AutoHideIsInstant(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := reducedManager(t, theme.ProfileWindows10)
	bar := NewTaskbar(tm)
	t.Cleanup(bar.Close)
	bar.AddItem(SlotStart, NewStartButton(tm))
	h := bar.Height()
	bar.SetBounds(image.Rect(0, screenBottom-h, 800, screenBottom))
	bar.SetAutoHide(true)
	bar.OnMouseMove(400, screenBottom-1)
	// Один шаг часов — и всё: время вперёд на кадр, а не на 140 мс.
	widget.StepAnimations(time.Now())
	widget.StepAnimations(time.Now().Add(time.Millisecond))
	if !bar.IsRevealed() || !bar.Bounds().Overlaps(image.Rect(0, screenBottom-h, 800, screenBottom-1)) {
		t.Errorf("панель не на месте после одного шага: %v", bar.Bounds())
	}
}

// Раскрытие карточки уведомления — функциональная анимация: по умолчанию
// мгновенная, а профиль может оставить её короткой.
func TestReduceMotion_NotificationExpandIsFunctional(t *testing.T) {
	m := managerFor(t, theme.ProfileWindows10)
	if d, _ := expandAnimation(m); d <= 0 {
		t.Fatal("без флага раскрытие анимировано")
	}
	m.SetFlag(theme.FlagMotionReduce, true)
	if d, _ := expandAnimation(m); d != 0 {
		t.Errorf("раскрытие при «меньше движения»: %v", d)
	}

	// Профиль без токена, но с метрикой: берётся menu.open, укороченная как
	// функциональная.
	short := theme.NewProfile("short")
	short.Parent = theme.ProfileWindows11
	short.SetMetric(theme.KeyMotionReduceFunctionalMS, 40)
	if err := m.RegisterTheme(short); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("short"); err != nil {
		t.Fatal(err)
	}
	if d, _ := expandAnimation(m); d != 40*time.Millisecond {
		t.Errorf("раскрытие по метрике: %v, ждали 40 мс", d)
	}
	if d, _ := animation(m, AnimMenuOpen); d != 0 {
		t.Errorf("украшение menu.open не обнулено: %v", d)
	}
}

// Раскрытие карточки в панели: при флаге высота меняется с первого же шага.
func TestReduceMotion_CardExpandsAtOnce(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	m.SetFlag(theme.FlagMotionReduce, true)

	_, ns, ids := slideFixture(t)
	nc := NewNotificationCenter(m, ns)
	nc.Clock = NewFakeClock(sampleTime().Add(2 * time.Hour))
	nc.Screen = panelScreen()
	nc.Open(panelAnchor())
	nc.Settle()
	defer nc.Close()

	c0, _ := nc.testLayout().cardOf(ids[2])
	nc.clickZone(t, zoneKey{kind: zoneCardToggle, note: ids[2]})
	c1, _ := nc.testLayout().cardOf(ids[2])
	if c1.rect.Dy() <= c0.rect.Dy() || c1.anim {
		t.Errorf("карточка раскрывается постепенно: %d -> %d, anim=%v", c0.rect.Dy(), c1.rect.Dy(), c1.anim)
	}
}

// ─── Тень и материал в PaintStyle ───────────────────────────────────────────

// paintRec — контекст, записывающий вызовы PaintStyle.
type paintRec struct {
	widget.DrawContext
	shadow, param, mica, fills int
	lastParam                  [3]float64
}

func (c *paintRec) FillRect(x, y, w, h int, col color.RGBA)      { c.fills++ }
func (c *paintRec) FillRectAlpha(x, y, w, h int, col color.RGBA) { c.fills++ }
func (c *paintRec) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	c.fills++
}
func (c *paintRec) DrawBorder(x, y, w, h int, col color.RGBA)         {}
func (c *paintRec) DrawRoundBorder(x, y, w, h, r int, col color.RGBA) {}
func (c *paintRec) DrawSoftShadow(r image.Rectangle, corner int, elevation float64, col color.RGBA) {
	c.shadow++
}
func (c *paintRec) DrawShadow(r image.Rectangle, corner int, blur, ox, oy float64, col color.RGBA) {
	c.param++
	c.lastParam = [3]float64{blur, ox, oy}
}
func (c *paintRec) MicaBehind(r image.Rectangle, radius int, tint color.RGBA) bool {
	c.mica++
	return true
}

// Без токенов тень — прежняя (DrawSoftShadow от Elevation), с токенами — с
// явными размытием и смещением.
func TestPaintStyle_ShadowTokens(t *testing.T) {
	r := image.Rect(0, 0, 100, 100)
	legacy := &theme.Style{Corner: 8, Elevation: 12, Shadow: theme.RGBA(0, 0, 0, 70), Fill: theme.RGB(240, 240, 240)}
	rec := &paintRec{}
	PaintStyle(rec, r, legacy)
	if rec.shadow != 1 || rec.param != 0 {
		t.Errorf("прежний путь: soft=%d param=%d", rec.shadow, rec.param)
	}

	tok := *legacy
	tok.ShadowBlur, tok.ShadowOffsetX, tok.ShadowOffsetY = 20, 2, 8
	rec = &paintRec{}
	PaintStyle(rec, r, &tok)
	if rec.param != 1 || rec.shadow != 0 || rec.lastParam != [3]float64{20, 2, 8} {
		t.Errorf("токены: soft=%d param=%d %v", rec.shadow, rec.param, rec.lastParam)
	}
}

// Материал Mica стиля доходит до контекста; заливка стиля поверх не кладётся.
func TestPaintStyle_Mica(t *testing.T) {
	s := &theme.Style{Fill: theme.RGB(243, 243, 243), Backdrop: theme.BackdropSpec{
		Mode: theme.BackdropBlur, Material: theme.MaterialMica, Radius: 80,
		Tint: theme.RGBA(243, 243, 243, 190), Fallback: theme.RGB(243, 243, 243)}}
	rec := &paintRec{}
	PaintStyle(rec, image.Rect(0, 0, 100, 100), s)
	if rec.mica != 1 {
		t.Errorf("MicaBehind вызван %d раз", rec.mica)
	}
	if rec.fills != 0 {
		t.Errorf("поверх Mica положена заливка (%d)", rec.fills)
	}
}

// Запас движения панели растёт вместе с мягкой тенью: тень не обрезается.
func TestFlyout_MotionRegionCoversSoftShadow(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows11)
	f := motionFlyout(tm)
	f.Open(motionAnchor)
	rest := f.restRect()
	before := f.motionRegion(rest)

	tm.SetFlag(theme.FlagShadowSoft, true)
	after := f.motionRegion(rest)
	if after.Dx() <= before.Dx() {
		t.Errorf("область движения %v не шире прежней %v при мягкой тени", after, before)
	}
	sp, _ := tm.GetStyle(ComponentStartMenu, "", theme.StateNormal).ResolveShadow()
	// Панель выезжает снизу, поэтому тень запасается сверху и с боков.
	if after.Min.Y > rest.Min.Y-sp.Extent() {
		t.Errorf("запас сверху меньше тени: %v при месте %v и ширине тени %d", after, rest, sp.Extent())
	}
}
