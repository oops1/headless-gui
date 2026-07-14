package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// ─── Recording DrawContext ──────────────────────────────────────────────────

type recTextDraw struct {
	text string
	x    int
}

// recCtx — записывающий widget.DrawContext с детерминированным измерителем
// текста (ширина = число рун × charW). Позволяет проверить геометрию
// отрисовки титлбара без движка/шрифтов.
type recCtx struct {
	texts []recTextDraw
}

const recCharW = 7

func (c *recCtx) measure(s string) int { return len([]rune(s)) * recCharW }

func (c *recCtx) FillRect(x, y, w, h int, col color.RGBA)              {}
func (c *recCtx) FillRectAlpha(x, y, w, h int, col color.RGBA)         {}
func (c *recCtx) FillRoundRect(x, y, w, h, r int, col color.RGBA)      {}
func (c *recCtx) DrawBorder(x, y, w, h int, col color.RGBA)            {}
func (c *recCtx) DrawRoundBorder(x, y, w, h, r int, col color.RGBA)    {}
func (c *recCtx) SetPixel(x, y int, col color.RGBA)                    {}
func (c *recCtx) DrawHLine(x, y, length int, col color.RGBA)           {}
func (c *recCtx) DrawVLine(x, y, length int, col color.RGBA)           {}
func (c *recCtx) DrawImage(src image.Image, x, y int)                  {}
func (c *recCtx) DrawImageScaled(src image.Image, x, y, w, h int)      {}
func (c *recCtx) DrawText(text string, x, y int, col color.RGBA) {
	c.texts = append(c.texts, recTextDraw{text, x})
}
func (c *recCtx) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	c.texts = append(c.texts, recTextDraw{text, x})
}
func (c *recCtx) DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA) {
	c.texts = append(c.texts, recTextDraw{text, x})
}
func (c *recCtx) MeasureText(text string, sizePt float64) int { return c.measure(text) }
func (c *recCtx) MeasureTextFont(text string, sizePt float64, fontName string) int {
	return c.measure(text)
}
func (c *recCtx) MeasureRunePositions(text string, sizePt float64) []int { return nil }
func (c *recCtx) SetClip(r image.Rectangle)                              {}
func (c *recCtx) ClearClip()                                             {}
func (c *recCtx) Clip() image.Rectangle                                  { return image.Rect(0, 0, 1<<15, 1<<15) }

// findBadgeAndTitle разбивает записанные текстовые отрисовки на бейдж локали
// (текст == locale) и заголовок (всё остальное непустое).
func (c *recCtx) findBadgeAndTitle(locale string) (badgeLeft int, haveBadge bool, title *recTextDraw) {
	const padX = 6 // drawLocaleBadge рисует текст в bx+padX
	for i := range c.texts {
		td := c.texts[i]
		if td.text == locale {
			badgeLeft = td.x - padX
			haveBadge = true
			continue
		}
		if td.text != "" {
			t := td
			title = &t
		}
	}
	return
}

// ─── Тесты ──────────────────────────────────────────────────────────────────

const longTitle = "Очень длинный заголовок окна который заведомо шире узкого окна двести пикселей"

func TestTitleBadge_WinEllipsized(t *testing.T) {
	prev := widget.Locale()
	widget.SetLocale("EN")
	defer widget.SetLocale(prev)

	w := widget.NewWindow(longTitle, 200, 120)
	w.SetBounds(image.Rect(0, 0, 200, 120))
	w.TitleStyle = widget.WindowTitleWin
	w.ShowLocaleIndicator = true

	rec := &recCtx{}
	w.Draw(rec)

	badgeLeft, haveBadge, title := rec.findBadgeAndTitle(widget.Locale())
	if !haveBadge {
		t.Fatal("бейдж локали не отрисован")
	}
	if title == nil {
		// Заголовок полностью обрезан (окно совсем узкое) — тоже допустимо:
		// он гарантированно не заходит на бейдж.
		return
	}
	right := title.x + rec.measure(title.text)
	if right > badgeLeft {
		t.Errorf("Win: заголовок нарисован до x=%d, правее левого края бейджа x=%d", right, badgeLeft)
	}
	if title.text == longTitle {
		t.Errorf("Win: заголовок не обрезан эллипсисом (%q целиком)", title.text)
	}
}

func TestTitleBadge_MacEllipsized(t *testing.T) {
	prev := widget.Locale()
	widget.SetLocale("EN")
	defer widget.SetLocale(prev)

	w := widget.NewWindow(longTitle, 200, 120)
	w.SetBounds(image.Rect(0, 0, 200, 120))
	w.TitleStyle = widget.WindowTitleMac
	w.ShowLocaleIndicator = true

	rec := &recCtx{}
	w.Draw(rec)

	badgeLeft, haveBadge, title := rec.findBadgeAndTitle(widget.Locale())
	if !haveBadge {
		t.Fatal("Mac: бейдж локали не отрисован")
	}
	if title == nil {
		return
	}
	right := title.x + rec.measure(title.text)
	if right > badgeLeft {
		t.Errorf("Mac: центрированный заголовок доходит до x=%d, правее левого края бейджа x=%d", right, badgeLeft)
	}
}

// TestTitleBadge_ShortTitleNotClipped — короткий заголовок в широком окне НЕ
// обрезается (эллипсис не срабатывает без необходимости).
func TestTitleBadge_ShortTitleNotClipped(t *testing.T) {
	prev := widget.Locale()
	widget.SetLocale("EN")
	defer widget.SetLocale(prev)

	w := widget.NewWindow("OK", 400, 120)
	w.SetBounds(image.Rect(0, 0, 400, 120))
	w.TitleStyle = widget.WindowTitleWin

	rec := &recCtx{}
	w.Draw(rec)

	_, _, title := rec.findBadgeAndTitle(widget.Locale())
	if title == nil || title.text != "OK" {
		t.Errorf("короткий заголовок изменён/не отрисован: %+v", title)
	}
}
