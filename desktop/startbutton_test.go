package desktop

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ─── Записывающий DrawContext для тестов пакета desktop ────────────────────
//
// Свой, а не общий с tests/titlebadge_test.go: itemRecCtx там объявлен в другом
// пакете (tests) и не экспортирован — переиспользовать нельзя.

type itemRecFill struct {
	x, y, w, h int
	col        color.RGBA
}

type itemRecImage struct {
	x, y, w, h int
}

type itemRecText struct {
	text string
	x, y int
}

// itemRecCtx — записывающий widget.DrawContext с детерминированным измерителем
// текста (ширина = число рун × itemRecCharW). Позволяет проверить, что и каким
// цветом было нарисовано, без движка и настоящих шрифтов.
type itemRecCtx struct {
	fills  []itemRecFill
	images []itemRecImage
	texts  []itemRecText
	clip   image.Rectangle
}

const itemRecCharW = 7

func newItemRecCtx() *itemRecCtx {
	return &itemRecCtx{clip: image.Rect(0, 0, 1<<15, 1<<15)}
}

func (c *itemRecCtx) FillRect(x, y, w, h int, col color.RGBA) {
	c.fills = append(c.fills, itemRecFill{x, y, w, h, col})
}
func (c *itemRecCtx) FillRectAlpha(x, y, w, h int, col color.RGBA) {
	c.fills = append(c.fills, itemRecFill{x, y, w, h, col})
}
func (c *itemRecCtx) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	c.fills = append(c.fills, itemRecFill{x, y, w, h, col})
}
func (c *itemRecCtx) DrawBorder(x, y, w, h int, col color.RGBA)           {}
func (c *itemRecCtx) DrawRoundBorder(x, y, w, h, r int, col color.RGBA)   {}
func (c *itemRecCtx) SetPixel(x, y int, col color.RGBA)                   {}
func (c *itemRecCtx) DrawHLine(x, y, length int, col color.RGBA)          {}
func (c *itemRecCtx) DrawVLine(x, y, length int, col color.RGBA)          {}
func (c *itemRecCtx) DrawImage(src image.Image, x, y int) {
	c.images = append(c.images, itemRecImage{x, y, 0, 0})
}
func (c *itemRecCtx) DrawImageScaled(src image.Image, x, y, w, h int) {
	c.images = append(c.images, itemRecImage{x, y, w, h})
}
func (c *itemRecCtx) DrawText(text string, x, y int, col color.RGBA) {
	c.texts = append(c.texts, itemRecText{text, x, y})
}
func (c *itemRecCtx) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	c.texts = append(c.texts, itemRecText{text, x, y})
}
func (c *itemRecCtx) DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA) {
	c.texts = append(c.texts, itemRecText{text, x, y})
}
func (c *itemRecCtx) MeasureText(text string, sizePt float64) int { return len([]rune(text)) * itemRecCharW }
func (c *itemRecCtx) MeasureTextFont(text string, sizePt float64, fontName string) int {
	return len([]rune(text)) * itemRecCharW
}
func (c *itemRecCtx) MeasureRunePositions(text string, sizePt float64) []int { return nil }
func (c *itemRecCtx) SetClip(r image.Rectangle)                              { c.clip = r }
func (c *itemRecCtx) ClearClip()                                             { c.clip = image.Rect(0, 0, 1<<15, 1<<15) }
func (c *itemRecCtx) Clip() image.Rectangle                                  { return c.clip }

// backgroundFill возвращает заливку, покрывающую целиком прямоугольник r
// (подложка компонента, а не значок/квадратик поверх неё), если такая была
// нарисована.
func (c *itemRecCtx) backgroundFill(r image.Rectangle) (itemRecFill, bool) {
	for _, f := range c.fills {
		if f.x == r.Min.X && f.y == r.Min.Y && f.w == r.Dx() && f.h == r.Dy() {
			return f, true
		}
	}
	return itemRecFill{}, false
}

// ─── Тестовая тема ──────────────────────────────────────────────────────────

// buildTestTheme собирает менеджер тем с профилем "test", объявляющим все
// токены startbutton/taskbutton, которые нужны компонентам из этого пакета.
// Заливка различается по состояниям — тесты на наведение/нажатие/активность
// проверяют это различие напрямую.
func buildTestTheme() *theme.Manager {
	tm := theme.NewManager()

	p := theme.NewProfile("test")
	p.SetMetric(KeyStartButtonIconSize, 16)
	p.SetMetric(KeyStartButtonIconGap, 2)
	p.SetMetric(KeyStartButtonLabelGap, 4)
	p.SetMetric(KeyStartButtonLabelWidth, 40)
	p.SetMetric(KeyStartButtonLabel, 0)

	p.SetMetric(KeyTaskButtonWidth, 150)
	p.SetMetric(KeyTaskButtonMinWidth, 70)
	p.SetMetric(KeyTaskButtonIconSize, 16)
	p.SetMetric(KeyTaskButtonGap, 4)
	p.SetMetric(KeyTaskButtonLabelGap, 4)

	p.SetStyle(ComponentStartButton, "", theme.StateNormal, theme.StyleDelta{
		Fill: theme.C(theme.RGB(30, 30, 30)),
		Text: theme.C(theme.RGB(255, 255, 255)),
		PadX: theme.N(4),
		PadY: theme.N(4),
	})
	p.SetStyle(ComponentStartButton, "", theme.StateHover, theme.StyleDelta{
		Fill: theme.C(theme.RGB(60, 60, 60)),
	})
	p.SetStyle(ComponentStartButton, "", theme.StatePressed, theme.StyleDelta{
		Fill: theme.C(theme.RGB(90, 90, 90)),
	})

	p.SetStyle(ComponentTaskButton, "", theme.StateNormal, theme.StyleDelta{
		Fill: theme.C(theme.RGB(20, 20, 20)),
		Text: theme.C(theme.RGB(255, 255, 255)),
		PadX: theme.N(4),
		PadY: theme.N(4),
	})
	p.SetStyle(ComponentTaskButton, "", theme.StateHover, theme.StyleDelta{
		Fill: theme.C(theme.RGB(50, 50, 50)),
	})
	p.SetStyle(ComponentTaskButton, "", theme.StatePressed, theme.StyleDelta{
		Fill: theme.C(theme.RGB(70, 70, 70)),
	})
	p.SetStyle(ComponentTaskButton, "", theme.StateActive, theme.StyleDelta{
		Fill: theme.C(theme.RGB(0, 90, 200)),
	})
	p.SetStyle(ComponentTaskButton, "", theme.StateDisabled, theme.StyleDelta{
		Fill: theme.C(theme.RGB(10, 10, 10)),
	})

	if err := tm.RegisterTheme(p); err != nil {
		panic(err)
	}
	if err := tm.SetTheme("test"); err != nil {
		panic(err)
	}
	return tm
}

// ─── StartButton ─────────────────────────────────────────────────────────────

func TestStartButton_ClickFiresOnce(t *testing.T) {
	tm := buildTestTheme()
	sb := NewStartButton(tm)
	sb.SetBounds(image.Rect(0, 0, 60, 32))

	clicks := 0
	sb.OnClick = func() { clicks++ }

	if !sb.OnMouseButton(widget.MouseEvent{X: 10, Y: 10, Button: widget.MouseLeft, Pressed: true}) {
		t.Fatalf("press над кнопкой должен быть поглощён")
	}
	if !sb.armed {
		t.Fatalf("press над кнопкой должен взвести её")
	}
	if !sb.OnMouseButton(widget.MouseEvent{X: 10, Y: 10, Button: widget.MouseLeft, Pressed: false}) {
		t.Fatalf("release над взведённой кнопкой должен быть поглощён")
	}
	if clicks != 1 {
		t.Fatalf("OnClick должен сработать ровно один раз, сработал %d раз(а)", clicks)
	}
	if sb.armed {
		t.Fatalf("после release кнопка не должна оставаться взведённой")
	}
}

func TestStartButton_ReleaseAwayDoesNotFire(t *testing.T) {
	tm := buildTestTheme()
	sb := NewStartButton(tm)
	sb.SetBounds(image.Rect(0, 0, 60, 32))

	clicks := 0
	sb.OnClick = func() { clicks++ }

	sb.OnMouseButton(widget.MouseEvent{X: 10, Y: 10, Button: widget.MouseLeft, Pressed: true})
	sb.OnMouseButton(widget.MouseEvent{X: 500, Y: 500, Button: widget.MouseLeft, Pressed: false})

	if clicks != 0 {
		t.Fatalf("release в стороне не должен вызывать OnClick, вызван %d раз(а)", clicks)
	}
}

func TestStartButton_PressOutsideBoundsIgnored(t *testing.T) {
	tm := buildTestTheme()
	sb := NewStartButton(tm)
	sb.SetBounds(image.Rect(0, 0, 60, 32))

	if sb.OnMouseButton(widget.MouseEvent{X: 500, Y: 500, Button: widget.MouseLeft, Pressed: true}) {
		t.Fatalf("press вне кнопки не должен ею поглощаться")
	}
	if sb.armed {
		t.Fatalf("press вне кнопки не должен её взводить")
	}
}

func TestStartButton_HoverChangesStyle(t *testing.T) {
	tm := buildTestTheme()
	bounds := image.Rect(0, 0, 60, 32)

	sbNormal := NewStartButton(tm)
	sbNormal.SetBounds(bounds)
	ctxNormal := newItemRecCtx()
	sbNormal.Draw(ctxNormal)
	fillNormal, ok := ctxNormal.backgroundFill(bounds)
	if !ok {
		t.Fatalf("не найдена заливка подложки в обычном состоянии")
	}

	sbHover := NewStartButton(tm)
	sbHover.SetBounds(bounds)
	sbHover.OnMouseMove(10, 10) // внутри границ — наведение
	if !sbHover.hovered {
		t.Fatalf("OnMouseMove внутри границ должен выставить hovered")
	}
	ctxHover := newItemRecCtx()
	sbHover.Draw(ctxHover)
	fillHover, ok := ctxHover.backgroundFill(bounds)
	if !ok {
		t.Fatalf("не найдена заливка подложки в наведённом состоянии")
	}

	if fillNormal.col == fillHover.col {
		t.Fatalf("заливка при наведении должна отличаться от обычной, обе %v", fillNormal.col)
	}
	wantHover := theme.RGB(60, 60, 60)
	if fillHover.col != wantHover {
		t.Fatalf("заливка при наведении = %v, хотим %v (стиль StateHover из темы)", fillHover.col, wantHover)
	}

	// Курсор ушёл за границы — наведение снимается.
	sbHover.OnMouseMove(500, 500)
	if sbHover.hovered {
		t.Fatalf("OnMouseMove вне границ должен снять hovered")
	}
}
