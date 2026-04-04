// Package engine реализует off-screen рендеринг и детектирование изменений.
//
// Canvas использует двойную буферизацию:
//   - back — текущий рендер (виджеты рисуют сюда)
//   - front — последний отправленный потребителю кадр
//
// После каждого рендера back сравнивается с front побайтово по тайлам 64×64.
// Изменившиеся тайлы копируются во front и возвращаются как []output.DirtyTile.
package engine

import (
	"bytes"
	"image"
	"image/color"
	stdraw "image/draw"
	"math"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/oops1/headless-gui/v3/output"
)

// Canvas — off-screen RGBA-холст с двойной буферизацией.
// Реализует интерфейс widget.DrawContext.
type Canvas struct {
	front      *image.RGBA           // последний отправленный кадр
	back       *image.RGBA           // текущий рендер-таргет
	bgImage    *image.RGBA           // фоновое изображение (масштабировано под холст)
	fontCache  *FontCache            // кэш шрифта по умолчанию
	namedFonts map[string]*FontCache // именованные шрифты (FontFamily из XAML)
	clip       image.Rectangle       // активная область отсечения
	hasClip    bool                  // включено ли отсечение
	scaleTmp   *image.RGBA           // переиспользуемый буфер для DrawImageScaled
	W, H       int
	tilesX     int
	tilesY     int
}

// RegisterFont регистрирует именованный шрифт (TTF-данные) в реестре холста.
// fontName соответствует FontFamily в XAML.
func (c *Canvas) RegisterFont(fontName string, ttfData []byte) {
	if c.namedFonts == nil {
		c.namedFonts = make(map[string]*FontCache)
	}
	fc := newFontCacheFromData(ttfData, c.fontCache.dpi)
	if fc != nil {
		c.namedFonts[fontName] = fc
	}
}

// fontFor возвращает FontCache для именованного шрифта; если не найден — default.
func (c *Canvas) fontFor(fontName string) *FontCache {
	if fontName != "" && c.namedFonts != nil {
		if fc, ok := c.namedFonts[fontName]; ok {
			return fc
		}
	}
	return c.fontCache
}

func newCanvas(w, h int, fc *FontCache) *Canvas {
	ts := output.TileSize
	return &Canvas{
		front:      image.NewRGBA(image.Rect(0, 0, w, h)),
		back:       image.NewRGBA(image.Rect(0, 0, w, h)),
		fontCache:  fc,
		namedFonts: make(map[string]*FontCache),
		W:          w,
		H:          h,
		tilesX:     (w + ts - 1) / ts,
		tilesY:     (h + ts - 1) / ts,
	}
}

// ─── Background ──────────────────────────────────────────────────────────────

// setBackground масштабирует src до размера холста и сохраняет как фон.
// Фон блиттируется в back-буфер в начале каждого кадра — до отрисовки виджетов.
// Использует билинейную интерполяцию (golang.org/x/image/draw.BiLinear).
func (c *Canvas) setBackground(src image.Image) {
	dst := image.NewRGBA(image.Rect(0, 0, c.W, c.H))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), stdraw.Over, nil)
	c.bgImage = dst
}

// blitBackground очищает back-буфер и копирует фон (если задан).
// Вызывается до отрисовки виджетов — перезаписывает весь back.
// Если фонового изображения нет — заливает буфер чёрным цветом,
// чтобы при перемещении виджетов старые пиксели не оставались.
func (c *Canvas) blitBackground() {
	if c.bgImage != nil {
		copy(c.back.Pix, c.bgImage.Pix)
	} else {
		// Очищаем буфер чёрным (RGBA = 0,0,0,255)
		pix := c.back.Pix
		for i := 0; i < len(pix); i += 4 {
			pix[i+0] = 0   // R
			pix[i+1] = 0   // G
			pix[i+2] = 0   // B
			pix[i+3] = 255 // A
		}
	}
}

// ─── Clip ───────────────────────────────────────────────────────────────────

// SetClip ограничивает все последующие операции рисования прямоугольником r.
func (c *Canvas) SetClip(r image.Rectangle) {
	c.clip = r.Intersect(c.back.Bounds())
	c.hasClip = !c.clip.Empty()
}

// ClearClip снимает ограничение области рисования.
func (c *Canvas) ClearClip() {
	c.hasClip = false
}

// clampRect пересекает r с текущей областью отсечения (или bounds холста).
func (c *Canvas) clampRect(r image.Rectangle) image.Rectangle {
	if c.hasClip {
		return r.Intersect(c.clip)
	}
	return r.Intersect(c.back.Bounds())
}

// dstFor возвращает destination для font.Drawer: если clip активен —
// обёртку, ограничивающую SetRGBA до области clip; иначе back напрямую.
func (c *Canvas) dstFor() stdraw.Image {
	if c.hasClip {
		return &clippedRGBA{img: c.back, clip: c.clip}
	}
	return c.back
}

// clippedRGBA — draw.Image-обёртка над *image.RGBA с ограничением по clip.
type clippedRGBA struct {
	img  *image.RGBA
	clip image.Rectangle
}

func (w *clippedRGBA) ColorModel() color.Model { return w.img.ColorModel() }
func (w *clippedRGBA) Bounds() image.Rectangle { return w.clip }
func (w *clippedRGBA) At(x, y int) color.Color { return w.img.At(x, y) }
func (w *clippedRGBA) Set(x, y int, col color.Color) {
	if image.Pt(x, y).In(w.clip) {
		w.img.Set(x, y, col)
	}
}

// ─── DrawContext ────────────────────────────────────────────────────────────

// FillRect заливает прямоугольник сплошным цветом.
func (c *Canvas) FillRect(x, y, w, h int, col color.RGBA) {
	if col.A == 0 {
		return
	}
	r := c.clampRect(image.Rect(x, y, x+w, y+h))
	if r.Empty() {
		return
	}
	stdraw.Draw(c.back, r, &image.Uniform{C: col}, image.Point{}, stdraw.Src)
}

// FillRectAlpha заливает прямоугольник с альфа-смешиванием (Over).
func (c *Canvas) FillRectAlpha(x, y, w, h int, col color.RGBA) {
	r := c.clampRect(image.Rect(x, y, x+w, y+h))
	if r.Empty() {
		return
	}
	stdraw.Draw(c.back, r, &image.Uniform{C: col}, image.Point{}, stdraw.Over)
}

// FillRoundRect заливает прямоугольник со скруглёнными углами радиуса r.
func (c *Canvas) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	if r <= 0 {
		c.FillRect(x, y, w, h, col)
		return
	}
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	// Центральная полоса без скруглений
	c.FillRect(x, y+r, w, h-2*r, col)
	// Верхняя и нижняя полосы со скруглёнными углами
	rf := float64(r)
	for i := 0; i < r; i++ {
		dy := float64(r - i - 1)
		inset := r - int(math.Round(math.Sqrt(rf*rf-dy*dy)))
		lineW := w - 2*inset
		if lineW > 0 {
			c.FillRect(x+inset, y+i, lineW, 1, col)     // верх
			c.FillRect(x+inset, y+h-1-i, lineW, 1, col) // низ
		}
	}
}

// DrawRoundBorder рисует 1-пиксельный контур со скруглёнными углами.
func (c *Canvas) DrawRoundBorder(x, y, w, h, r int, col color.RGBA) {
	if r <= 0 {
		c.DrawBorder(x, y, w, h, col)
		return
	}
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	// Прямые стороны
	c.DrawHLine(x+r, y, w-2*r, col)     // верх
	c.DrawHLine(x+r, y+h-1, w-2*r, col) // низ
	c.DrawVLine(x, y+r, h-2*r, col)     // лево
	c.DrawVLine(x+w-1, y+r, h-2*r, col) // право
	// Углы: четверти окружности
	rf := float64(r)
	for i := 0; i <= r; i++ {
		dy := float64(r - i)
		dx := int(math.Round(math.Sqrt(rf*rf - dy*dy)))
		// Верхний левый угол
		c.SetPixel(x+r-dx, y+i, col)
		// Верхний правый угол
		c.SetPixel(x+w-1-r+dx, y+i, col)
		// Нижний левый угол
		c.SetPixel(x+r-dx, y+h-1-i, col)
		// Нижний правый угол
		c.SetPixel(x+w-1-r+dx, y+h-1-i, col)
	}
}

// DrawBorder рисует 1-пиксельный контур прямоугольника.
func (c *Canvas) DrawBorder(x, y, w, h int, col color.RGBA) {
	c.FillRect(x, y, w, 1, col)     // верх
	c.FillRect(x, y+h-1, w, 1, col) // низ
	c.FillRect(x, y, 1, h, col)     // лево
	c.FillRect(x+w-1, y, 1, h, col) // право
}

// DrawText рисует строку TTF-шрифтом (Go Regular) размером DefaultFontSize.
func (c *Canvas) DrawText(text string, x, y int, col color.RGBA) {
	c.DrawTextSize(text, x, y, DefaultFontSize, col)
}

// DrawTextSize рисует строку шрифтом по умолчанию произвольного размера (в пунктах).
func (c *Canvas) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	c.drawTextWithFont(c.fontCache, text, x, y, sizePt, col)
}

// DrawTextFont рисует строку именованным шрифтом (fontName="") → шрифт по умолчанию.
func (c *Canvas) DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA) {
	c.drawTextWithFont(c.fontFor(fontName), text, x, y, sizePt, col)
}

func (c *Canvas) drawTextWithFont(fc *FontCache, text string, x, y int, sizePt float64, col color.RGBA) {
	face := fc.Face(sizePt)
	ascent := face.Metrics().Ascent.Round()
	d := font.Drawer{
		Dst:  c.dstFor(),
		Src:  &image.Uniform{C: col},
		Face: face,
		Dot:  fixed.P(x, y+ascent),
	}
	d.DrawString(text)
}

// MeasureText возвращает ширину строки в пикселях (шрифт по умолчанию, sizePt).
func (c *Canvas) MeasureText(text string, sizePt float64) int {
	return c.fontCache.Measure(text, sizePt)
}

// MeasureTextFont возвращает ширину строки именованным шрифтом.
func (c *Canvas) MeasureTextFont(text string, sizePt float64, fontName string) int {
	return c.fontFor(fontName).Measure(text, sizePt)
}

// MeasureRunePositions возвращает накопленную ширину после каждого символа.
// Результат: len(runes)+1 элементов; positions[0]==0, positions[n] — ширина text[:n].
func (c *Canvas) MeasureRunePositions(text string, sizePt float64) []int {
	return c.fontCache.MeasureRunes(text, sizePt)
}

// SetPixel устанавливает цвет одного пикселя (с учётом clip).
func (c *Canvas) SetPixel(x, y int, col color.RGBA) {
	if c.hasClip {
		if !image.Pt(x, y).In(c.clip) {
			return
		}
	}
	if x >= 0 && x < c.W && y >= 0 && y < c.H {
		c.back.SetRGBA(x, y, col)
	}
}

// DrawHLine рисует горизонтальную линию длиной length пикселей.
func (c *Canvas) DrawHLine(x, y, length int, col color.RGBA) {
	c.FillRect(x, y, length, 1, col)
}

// DrawVLine рисует вертикальную линию длиной length пикселей.
func (c *Canvas) DrawVLine(x, y, length int, col color.RGBA) {
	c.FillRect(x, y, 1, length, col)
}

// DrawImage рисует произвольное изображение в позицию (x, y) в оригинальном размере.
func (c *Canvas) DrawImage(src image.Image, x, y int) {
	r := c.clampRect(image.Rect(x, y, x+src.Bounds().Dx(), y+src.Bounds().Dy()))
	if r.Empty() {
		return
	}
	offset := src.Bounds().Min.Add(image.Pt(r.Min.X-x, r.Min.Y-y))
	stdraw.Draw(c.back, r, src, offset, stdraw.Over)
}

// DrawImageScaled рисует изображение масштабированным до (w × h) в позицию (x, y).
// Промежуточный буфер переиспользуется между вызовами если размер совпадает.
func (c *Canvas) DrawImageScaled(src image.Image, x, y, w, h int) {
	dstRect := c.clampRect(image.Rect(x, y, x+w, y+h))
	if dstRect.Empty() {
		return
	}
	// Переиспользуем буфер если размер подходит.
	need := image.Rect(0, 0, w, h)
	tmp := c.scaleTmp
	if tmp == nil || tmp.Bounds() != need {
		tmp = image.NewRGBA(need)
		c.scaleTmp = tmp
	} else {
		// Очищаем буфер для нового масштабирования.
		for i := range tmp.Pix {
			tmp.Pix[i] = 0
		}
	}
	draw.BiLinear.Scale(tmp, tmp.Bounds(), src, src.Bounds(), stdraw.Src, nil)
	offset := image.Pt(dstRect.Min.X-x, dstRect.Min.Y-y)
	stdraw.Draw(c.back, dstRect, tmp, offset, stdraw.Over)
}

// ─── Tile diffing ───────────────────────────────────────────────────────────

// diffAndSync сравнивает back с front по тайлам и возвращает изменившиеся.
func (c *Canvas) diffAndSync() []output.DirtyTile {
	ts := output.TileSize
	var tiles []output.DirtyTile

	for ty := 0; ty < c.tilesY; ty++ {
		for tx := 0; tx < c.tilesX; tx++ {
			px := tx * ts
			py := ty * ts
			pw := min(ts, c.W-px)
			ph := min(ts, c.H-py)

			if !c.tilesEqual(px, py, pw, ph) {
				data := c.extractTile(px, py, pw, ph)
				tiles = append(tiles, output.DirtyTile{
					X: px, Y: py,
					W: pw, H: ph,
					Data: data,
				})
				c.syncTile(px, py, pw, ph)
			}
		}
	}
	return tiles
}

func (c *Canvas) tilesEqual(x, y, w, h int) bool {
	rowBytes := w * 4
	for row := 0; row < h; row++ {
		fOff := c.front.PixOffset(x, y+row)
		bOff := c.back.PixOffset(x, y+row)
		if !bytes.Equal(
			c.front.Pix[fOff:fOff+rowBytes],
			c.back.Pix[bOff:bOff+rowBytes],
		) {
			return false
		}
	}
	return true
}

func (c *Canvas) extractTile(x, y, w, h int) []byte {
	data := make([]byte, w*h*4)
	rowBytes := w * 4
	for row := 0; row < h; row++ {
		src := c.back.PixOffset(x, y+row)
		dst := row * rowBytes
		copy(data[dst:dst+rowBytes], c.back.Pix[src:src+rowBytes])
	}
	return data
}

func (c *Canvas) syncTile(x, y, w, h int) {
	rowBytes := w * 4
	for row := 0; row < h; row++ {
		src := c.back.PixOffset(x, y+row)
		dst := c.front.PixOffset(x, y+row)
		copy(c.front.Pix[dst:dst+rowBytes], c.back.Pix[src:src+rowBytes])
	}
}

