package desktop

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ─── Записывающий DrawContext для тестов пакета desktop ─────────────────────
//
// Свой, не общий с tests/titlebadge_test.go: тот recCtx живёт в пакете
// tests и отсюда недоступен. Измеритель детерминирован (число рун × testCharW),
// заливки и тексты записываются для последующих проверок.

const testCharW = 7

type recFill struct {
	x, y, w, h int
	col        color.RGBA
}

type recText struct {
	text string
	x, y int
	size float64
	col  color.RGBA
}

type recCtx struct {
	fills []recFill
	texts []recText
}

func (c *recCtx) FillRect(x, y, w, h int, col color.RGBA) {
	c.fills = append(c.fills, recFill{x, y, w, h, col})
}
func (c *recCtx) FillRectAlpha(x, y, w, h int, col color.RGBA) { c.FillRect(x, y, w, h, col) }
func (c *recCtx) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	c.FillRect(x, y, w, h, col)
}
func (c *recCtx) DrawBorder(x, y, w, h int, col color.RGBA)         {}
func (c *recCtx) DrawRoundBorder(x, y, w, h, r int, col color.RGBA) {}
func (c *recCtx) SetPixel(x, y int, col color.RGBA)                 {}
func (c *recCtx) DrawHLine(x, y, length int, col color.RGBA)        {}
func (c *recCtx) DrawVLine(x, y, length int, col color.RGBA)        {}
func (c *recCtx) DrawImage(src image.Image, x, y int)               {}
func (c *recCtx) DrawImageScaled(src image.Image, x, y, w, h int)   {}
func (c *recCtx) DrawText(text string, x, y int, col color.RGBA) {
	c.texts = append(c.texts, recText{text, x, y, 0, col})
}
func (c *recCtx) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	c.texts = append(c.texts, recText{text, x, y, sizePt, col})
}
func (c *recCtx) DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA) {
	c.texts = append(c.texts, recText{text, x, y, sizePt, col})
}
func (c *recCtx) MeasureText(text string, sizePt float64) int {
	return len([]rune(text)) * testCharW
}
func (c *recCtx) MeasureTextFont(text string, sizePt float64, fontName string) int {
	return len([]rune(text)) * testCharW
}
func (c *recCtx) MeasureRunePositions(text string, sizePt float64) []int { return nil }
func (c *recCtx) SetClip(r image.Rectangle)                              {}
func (c *recCtx) ClearClip()                                             {}
func (c *recCtx) Clip() image.Rectangle                                  { return image.Rect(0, 0, 1<<15, 1<<15) }

func containsText(texts []recText, want string) bool {
	for _, tx := range texts {
		if tx.text == want {
			return true
		}
	}
	return false
}

// ─── Тема для тестов ────────────────────────────────────────────────────────

// testThemeManager собирает минимальную тему со стилями всех компонентов
// этого пакета (часы + трей) и метрикой KeyTrayIconSize — достаточно, чтобы
// Draw/PreferredSize не падали на пустой теме и было что сравнивать.
func testThemeManager(t *testing.T) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	p := theme.NewProfile("Test")

	p.SetStyle(ComponentClock, "", theme.StateNormal, theme.StyleDelta{
		Text: theme.C(theme.RGB(240, 240, 240)),
		PadX: theme.N(4),
		Font: &theme.FontSpec{Size: 10},
	})
	p.SetStyle(ComponentClock, "date", theme.StateNormal, theme.StyleDelta{
		Text: theme.C(theme.RGB(160, 160, 160)),
		PadY: theme.N(2),
		Font: &theme.FontSpec{Size: 8},
	})

	p.SetStyle(ComponentNetwork, "", theme.StateNormal, theme.StyleDelta{
		Fill:   theme.C(theme.RGB(240, 240, 240)),
		Border: theme.C(theme.RGB(90, 90, 90)),
		PadX:   theme.N(2),
		PadY:   theme.N(2),
	})
	p.SetStyle(ComponentVolume, "", theme.StateNormal, theme.StyleDelta{
		Fill:   theme.C(theme.RGB(240, 240, 240)),
		Border: theme.C(theme.RGB(90, 90, 90)),
		PadX:   theme.N(2),
		PadY:   theme.N(2),
	})
	p.SetStyle(ComponentPower, "", theme.StateNormal, theme.StyleDelta{
		Fill:   theme.C(theme.RGB(60, 200, 90)),
		Border: theme.C(theme.RGB(220, 220, 220)),
		Text:   theme.C(theme.RGB(255, 210, 60)),
		PadX:   theme.N(2),
		PadY:   theme.N(2),
		Corner: theme.N(2),
	})
	p.SetMetric(KeyTrayIconSize, 16)

	if err := m.RegisterTheme(p); err != nil {
		t.Fatalf("RegisterTheme: %v", err)
	}
	if err := m.SetTheme("Test"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}
	return m
}

// ─── Часы ────────────────────────────────────────────────────────────────────

func TestClockItem_ShowsFakeTime(t *testing.T) {
	tm := testThemeManager(t)
	clk := NewFakeClock(time.Date(2026, 8, 27, 14, 5, 0, 0, time.UTC))
	c := NewClock(tm, clk)
	defer c.Close()
	c.SetBounds(image.Rect(0, 0, 200, 44))

	ctx := &recCtx{}
	c.Draw(ctx)

	if !containsText(ctx.texts, "14:05") {
		t.Errorf("не нашли отрисованную строку времени %q среди %+v", "14:05", ctx.texts)
	}
}

func TestClockItem_StringChangesOnMinuteAdvance(t *testing.T) {
	tm := testThemeManager(t)
	clk := NewFakeClock(time.Date(2026, 8, 27, 14, 5, 30, 0, time.UTC))
	c := NewClock(tm, clk)
	defer c.Close()
	// Высота под одну строку — дата не помещается, сравнение проще.
	c.SetBounds(image.Rect(0, 0, 200, 20))

	first := &recCtx{}
	c.Draw(first)
	if !containsText(first.texts, "14:05") {
		t.Fatalf("не нашли 14:05 в %+v", first.texts)
	}

	clk.Advance(time.Minute)
	second := &recCtx{}
	c.Draw(second)
	if !containsText(second.texts, "14:06") {
		t.Errorf("после перевода на минуту не нашли 14:06 в %+v", second.texts)
	}
}

// TestClockItem_TickInvalidatesOnlyWhenStringChanges — требование №8: тик раз
// в секунду не должен будить рендер, пока отображаемая строка не изменилась.
func TestClockItem_TickInvalidatesOnlyWhenStringChanges(t *testing.T) {
	tm := testThemeManager(t)
	clk := NewFakeClock(time.Date(2026, 8, 27, 14, 5, 30, 0, time.UTC))
	c := NewClock(tm, clk)
	defer c.Close()
	c.SetBounds(image.Rect(0, 0, 200, 20))

	// Первая отрисовка запускает секундный Animate (ensureTick).
	c.Draw(&recCtx{})

	var rectCalls int
	handle := widget.RegisterUINotifier(nil, func(image.Rectangle) { rectCalls++ })
	defer widget.UnregisterUINotifier(handle)

	base := time.Now()
	// Тик внутри той же секунды — строка не меняется, уведомлений быть не должно.
	widget.StepAnimations(base.Add(400 * time.Millisecond))
	widget.StepAnimations(base.Add(700 * time.Millisecond))
	if rectCalls != 0 {
		t.Fatalf("тик без смены отображаемой строки вызвал %d уведомлений, ждали 0", rectCalls)
	}

	// Минута сменилась — ближайший тик обязан перерисовать.
	clk.Advance(time.Minute)
	widget.StepAnimations(base.Add(900 * time.Millisecond))
	if rectCalls == 0 {
		t.Error("смена отображаемой строки не вызвала Invalidate")
	}
}

func TestClockItem_CloseStopsTicking(t *testing.T) {
	tm := testThemeManager(t)
	clk := NewFakeClock(time.Date(2026, 8, 27, 14, 5, 30, 0, time.UTC))
	c := NewClock(tm, clk)
	c.SetBounds(image.Rect(0, 0, 200, 20))
	c.Draw(&recCtx{})
	c.Close()

	var rectCalls int
	handle := widget.RegisterUINotifier(nil, func(image.Rectangle) { rectCalls++ })
	defer widget.UnregisterUINotifier(handle)

	clk.Advance(time.Minute)
	widget.StepAnimations(time.Now().Add(2 * time.Second))
	if rectCalls != 0 {
		t.Errorf("после Close тик всё ещё перерисовывает: %d уведомлений", rectCalls)
	}
}

func TestClockItem_PreferredSizeGrowsWithLength(t *testing.T) {
	tm := testThemeManager(t)
	clk := NewFakeClock(time.Date(2026, 8, 27, 14, 5, 0, 0, time.UTC))

	short := NewClock(tm, clk)
	short.TimeFormat = "15:04"
	defer short.Close()

	long := NewClock(tm, clk)
	long.TimeFormat = "15:04:05 -0700 MST"
	defer long.Close()

	// Высота мала — однострочный режим, сравниваем только ширину времени.
	avail := image.Pt(400, 10)
	wShort := short.PreferredSize(avail).X
	wLong := long.PreferredSize(avail).X
	if wLong <= wShort {
		t.Errorf("PreferredSize не растёт с длиной строки: short=%d long=%d", wShort, wLong)
	}
}

func TestClockItem_TwoLineWhenTallEnough(t *testing.T) {
	tm := testThemeManager(t)
	clk := NewFakeClock(time.Date(2026, 8, 27, 14, 5, 0, 0, time.UTC))
	c := NewClock(tm, clk)
	defer c.Close()

	shortH := c.PreferredSize(image.Pt(200, 10)).Y
	tallH := c.PreferredSize(image.Pt(200, 60)).Y
	if tallH <= shortH {
		t.Errorf("высота не выросла при появлении даты: short=%d tall=%d", shortH, tallH)
	}
}
