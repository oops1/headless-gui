package engine

import (
	"image"
	"image/color"

	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

var _ widget.RotatedTextDrawer = (*Canvas)(nil)

// rotatedTextPad — запас вокруг строки в промежуточном растре (физические
// пиксели): свесы глифов за ascent/descent и кернинг не должны обрезаться.
const rotatedTextPad = 4

// maxRotatedTextSide — предельная сторона промежуточного растра. Строка длиннее
// (патологический вход) не рисуется: растр с таким размером съел бы память.
const maxRotatedTextSide = 8192

// DrawTextRotated рисует строку, повёрнутую на 0/90/180/270° вокруг точки
// (x, y) (логические координаты); подробности и расположение надписи —
// в widget.RotatedTextDrawer. Реализует его.
//
// Строка раскладывается и растрируется обычным путём (шейпинг, запасные
// шрифты, кернинг) в промежуточный холст физического размера, а затем её
// альфа-маска переставляется по пикселям на прямой угол — без
// передискретизации. Поэтому на HiDPI глифы остаются такими же чёткими, как у
// горизонтального текста: растр, повёрнутый приложением на масштаб 1× и
// растянутый движком, выходил мягким.
func (c *Canvas) DrawTextRotated(text string, x, y int, sizePt float64, fontName string, angle int, col color.RGBA) bool {
	turns := ((angle % 360) + 360) % 360
	switch turns {
	case 0:
		c.DrawTextFont(text, x, y, sizePt, fontName, col)
		return true
	case 90, 180, 270:
	default:
		return false
	}
	if text == "" || col.A == 0 {
		return true
	}

	fc := c.fontFor(fontName)
	ascent, descent := fc.vMetrics(sizePt)
	tw := c.measureWithFallback(fc, text, sizePt)
	const pad = rotatedTextPad
	sw, sh := tw+2*pad, ascent+descent+2*pad // размер промежуточного растра
	if tw <= 0 || sw > maxRotatedTextSide || sh > maxRotatedTextSide {
		return true // нечего рисовать либо слишком велико
	}

	// Куда ляжет повёрнутый растр: (u, v) — точка исходного растра относительно
	// точки (px, py), поворот переводит её так, как описано в RotatedTextDrawer.
	px, py := c.sx(x), c.sx(y)
	var dst image.Rectangle
	switch turns {
	case 90: // (u, v) → (v, -u)
		dst = image.Rect(px-pad, py+pad-sw, px-pad+sh, py+pad)
	case 270: // (u, v) → (-v, u)
		dst = image.Rect(px+pad-sh, py-pad, px+pad, py-pad+sw)
	default: // 180: (u, v) → (-u, -v)
		dst = image.Rect(px+pad-sw, py+pad-sh, px+pad, py+pad)
	}
	// Целиком вне отсечения — не тратим растр.
	if c.clampRect(dst).Empty() {
		return true
	}

	// Холст масштаба 1 с физическими размерами: шрифты общие и уже
	// настроены на физический DPI, метрики те же, что у основного холста.
	tmp := newCanvasScaled(sw, sh, 1, c.fontCache)
	tmp.namedFonts = c.namedFonts
	tmp.families = c.families
	tmp.fallbacks = c.fallbacks
	tmp.DrawTextFont(text, pad, pad, sizePt, fontName, color.RGBA{255, 255, 255, 255})
	src := tmp.back
	// Холст tmp размером в сам растр: его размер мог быть зажат
	// (clampCanvasSize) — читаем фактический.
	rw, rh := src.Rect.Dx(), src.Rect.Dy()
	if rw != sw || rh != sh {
		return true
	}

	mask := image.NewAlpha(dst)
	for my := 0; my < sh; my++ {
		row := src.Pix[my*src.Stride : my*src.Stride+sw*4]
		for mx := 0; mx < sw; mx++ {
			a := row[mx*4+3]
			if a == 0 {
				continue
			}
			var ix, iy int // индекс в повёрнутой маске
			switch turns {
			case 90:
				ix, iy = my, sw-1-mx
			case 270:
				ix, iy = sh-1-my, mx
			default:
				ix, iy = sw-1-mx, sh-1-my
			}
			mask.Pix[iy*mask.Stride+ix] = a
		}
	}
	c.withMaskKind(output.RegionText, func() { c.drawAlphaMask(mask, dst.Min.X, dst.Min.Y, col) })
	return true
}
