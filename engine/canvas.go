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
	"runtime"
	"sort"
	"sync"

	"golang.org/x/image/draw"
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
	fallbacks  []*FontCache          // fallback-шрифты для отсутствующих глифов (BUG-2)
	clip       image.Rectangle       // активная область отсечения
	hasClip    bool                  // включено ли отсечение
	baseClip   image.Rectangle       // базовый клип кадра (damage-область частичной перерисовки)
	hasBase    bool                  // активен ли базовый клип
	scaleTmp   *image.RGBA           // переиспользуемый буфер для DrawImageScaled
	shaper     textShaper            // шейпинг сложного текста (RTL, лигатуры; см. shaper.go)
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

// hasFont сообщает, зарегистрирован ли именованный шрифт.
func (c *Canvas) hasFont(name string) bool {
	_, ok := c.namedFonts[name]
	return ok
}

// SetDefaultFont делает зарегистрированный именованный шрифт шрифтом по умолчанию
// (используется DrawText/DrawTextSize и как primary для fallback). Возвращает
// false, если шрифт с таким именем не зарегистрирован.
func (c *Canvas) SetDefaultFont(name string) bool {
	if name == "" {
		return false
	}
	if fc, ok := c.namedFonts[name]; ok && fc != nil {
		c.fontCache = fc
		return true
	}
	return false
}

// fontNames возвращает отсортированный список зарегистрированных именованных шрифтов.
func (c *Canvas) fontNames() []string {
	out := make([]string, 0, len(c.namedFonts))
	for k := range c.namedFonts {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// AddFallbackFont добавляет fallback-шрифт из TTF/OTF-данных (BUG-2).
// Используется для рун, отсутствующих в основном шрифте.
func (c *Canvas) AddFallbackFont(ttfData []byte) bool {
	fc := newFontCacheFromData(ttfData, c.fontCache.dpi)
	if fc == nil {
		return false
	}
	c.fallbacks = append(c.fallbacks, fc)
	return true
}

// fcForRune возвращает FontCache, содержащий глиф для руны r: сначала primary,
// затем fallback-цепочку. Второе значение — найден ли глиф вообще: если false,
// ни один шрифт не покрывает руну (вызывающий код пропускает отрисовку, чтобы
// не рисовать уродливый .notdef-квадрат «тофу»).
func (c *Canvas) fcForRune(primary *FontCache, r rune) (*FontCache, bool) {
	if primary.HasGlyph(r) {
		return primary, true
	}
	for _, fb := range c.fallbacks {
		if fb.HasGlyph(r) {
			return fb, true
		}
	}
	return primary, false
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

// blitBackgroundIn — как blitBackground, но только в области r (частичная
// перерисовка): вне r back-буфер сохраняет прошлый кадр.
func (c *Canvas) blitBackgroundIn(r image.Rectangle) {
	r = r.Intersect(c.back.Bounds())
	if r.Empty() {
		return
	}
	if c.bgImage != nil {
		// bgImage создаётся размером с холст — stride совпадает с back.
		rowBytes := r.Dx() * 4
		for y := r.Min.Y; y < r.Max.Y; y++ {
			off := c.back.PixOffset(r.Min.X, y)
			copy(c.back.Pix[off:off+rowBytes], c.bgImage.Pix[off:off+rowBytes])
		}
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		off := c.back.PixOffset(r.Min.X, y)
		row := c.back.Pix[off : off+r.Dx()*4]
		for i := 0; i < len(row); i += 4 {
			row[i+0] = 0
			row[i+1] = 0
			row[i+2] = 0
			row[i+3] = 255
		}
	}
}

// ─── Clip ───────────────────────────────────────────────────────────────────

// SetClip ограничивает все последующие операции рисования прямоугольником r.
// При активном базовом клипе (частичная перерисовка) итоговая область —
// пересечение r с базовым клипом: виджеты не могут рисовать вне damage.
func (c *Canvas) SetClip(r image.Rectangle) {
	if c.hasBase {
		// hasClip остаётся true даже при пустом пересечении — рисовать
		// вне damage нельзя (пустой clip == «не рисовать ничего»).
		c.clip = r.Intersect(c.baseClip)
		c.hasClip = true
		return
	}
	c.clip = r.Intersect(c.back.Bounds())
	c.hasClip = !c.clip.Empty()
}

// ClearClip снимает ограничение области рисования (до базового клипа,
// если идёт частичная перерисовка).
func (c *Canvas) ClearClip() {
	if c.hasBase {
		c.clip = c.baseClip
		c.hasClip = true
		return
	}
	c.hasClip = false
}

// setBaseClip включает базовый клип кадра: вся отрисовка (включая
// SetClip/ClearClip виджетов) ограничивается прямоугольником r.
// Используется движком для частичной перерисовки по damage-области.
func (c *Canvas) setBaseClip(r image.Rectangle) {
	c.baseClip = r.Intersect(c.back.Bounds())
	c.hasBase = true
	c.clip = c.baseClip
	c.hasClip = true
}

// clearBaseClip выключает базовый клип (конец частичного кадра).
func (c *Canvas) clearBaseClip() {
	c.hasBase = false
	c.hasClip = false
}

// Clip возвращает текущую область отсечения (или полные границы холста).
func (c *Canvas) Clip() image.Rectangle {
	if c.hasClip {
		return c.clip
	}
	return c.back.Bounds()
}

// clampRect пересекает r с текущей областью отсечения (или bounds холста).
func (c *Canvas) clampRect(r image.Rectangle) image.Rectangle {
	if c.hasClip {
		return r.Intersect(c.clip)
	}
	return r.Intersect(c.back.Bounds())
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
// Углы сглажены (AA-маски, см. aa.go); прямоугольное тело заливается
// быстрым FillRect. Полупрозрачные цвета идут по старому не-AA пути,
// чтобы не смешивать семантики Src (тело) и Over (углы).
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
	if col.A < 255 {
		c.fillRoundRectLegacy(x, y, w, h, r, col)
		return
	}
	// Тело: средняя полоса на всю ширину + верх/низ между углами.
	c.FillRect(x, y+r, w, h-2*r, col)
	c.FillRect(x+r, y, w-2*r, r, col)
	c.FillRect(x+r, y+h-r, w-2*r, r, col)
	// Сглаженные углы.
	c.drawCorners(cornersFor(r, cornerFill), x, y, w, h, r, col)
}

// fillRoundRectLegacy — прежняя ступенчатая заливка (для A<255).
func (c *Canvas) fillRoundRectLegacy(x, y, w, h, r int, col color.RGBA) {
	c.FillRect(x, y+r, w, h-2*r, col)
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
// Дуги углов сглажены (AA-маски четверть-кольца, см. aa.go).
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
	if col.A < 255 {
		c.drawRoundBorderCornersLegacy(x, y, w, h, r, col)
		return
	}
	// Сглаженные дуги углов.
	c.drawCorners(cornersFor(r, cornerRing), x, y, w, h, r, col)
}

// drawRoundBorderCornersLegacy — прежние ступенчатые дуги (для A<255).
func (c *Canvas) drawRoundBorderCornersLegacy(x, y, w, h, r int, col color.RGBA) {
	rf := float64(r)
	for i := 0; i <= r; i++ {
		dy := float64(r - i)
		dx := int(math.Round(math.Sqrt(rf*rf - dy*dy)))
		c.SetPixel(x+r-dx, y+i, col)       // верхний левый
		c.SetPixel(x+w-1-r+dx, y+i, col)   // верхний правый
		c.SetPixel(x+r-dx, y+h-1-i, col)   // нижний левый
		c.SetPixel(x+w-1-r+dx, y+h-1-i, col) // нижний правый
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
	// Обе ветки блиттируют кэшированные альфа-маски глифов (см. FontCache.Glyph)
	// вместо повторной растеризации контуров через font.Drawer.
	ascent, descent := fc.vMetrics(sizePt)
	baseline := y + ascent

	// Вертикальный отсев: строка целиком вне клипа — не итерируем руны вовсе
	// (важно при частичной перерисовке, когда клип — небольшая damage-область).
	// Запас 4px на глифы, выходящие за ascent/descent (акценты и т.п.).
	if c.hasClip {
		if y-4 >= c.clip.Max.Y || y+ascent+descent+4 <= c.clip.Min.Y {
			return
		}
	}

	// Сложный текст (RTL, арабский, индийские скрипты, комбинируемые знаки) —
	// через шейпер (см. shaper.go). При недоступности шейпинга (шрифт не
	// распарсился typesetting'ом) — деградация на per-rune путь ниже.
	if needsShaping(text) && c.drawShapedText(fc, text, x, baseline, sizePt, col) {
		return
	}

	// Быстрый путь: нет fallback-шрифтов — один шрифт, с кернингом
	// (поведение прежнего font.Drawer.DrawString; отсутствующий глиф
	// пропускается без продвижения пера — как делал Drawer с opentype).
	if len(c.fallbacks) == 0 {
		pen := fixed.I(x)
		prev := rune(-1)
		for _, r := range text {
			if prev >= 0 {
				pen += fc.Kern(sizePt, prev, r)
			}
			g := fc.Glyph(sizePt, r)
			if !g.ok {
				continue
			}
			c.drawGlyphMask(g, pen.Round()+g.offX, baseline+g.offY, col)
			pen += g.advance
			prev = r
		}
		return
	}

	// Fallback-путь: по рунам, выбирая шрифт с нужным глифом.
	// Baseline от основного шрифта, кернинг не применяется (прежнее поведение).
	pen := fixed.I(x)
	for _, r := range text {
		chosen, found := c.fcForRune(fc, r)
		g := chosen.Glyph(sizePt, r)
		if found && g.ok {
			c.drawGlyphMask(g, pen.Round()+g.offX, baseline+g.offY, col)
		}
		// Отсутствующий глиф не рисуем (без .notdef-квадрата), но сохраняем
		// интервал по ширине пробела, чтобы текст не «слипался».
		adv := g.advance
		if !found || !g.ok {
			if sp := fc.Glyph(sizePt, ' '); sp.ok {
				adv = sp.advance
			}
		}
		pen += adv
	}
}

// drawGlyphMask альфа-блендит маску глифа из per-rune кэша.
func (c *Canvas) drawGlyphMask(g cachedGlyph, gx, gy int, col color.RGBA) {
	if g.mask == nil {
		return
	}
	c.drawAlphaMask(g.mask, gx, gy, col)
}

// drawAlphaMask альфа-блендит альфа-маску цветом col в back-буфер (Over,
// premultiplied — как image/draw для font.Drawer). Учитывает clip.
// (gx, gy) — позиция левого верхнего угла маски на холсте.
func (c *Canvas) drawAlphaMask(alpha *image.Alpha, gx, gy int, col color.RGBA) {
	mw, mh := alpha.Rect.Dx(), alpha.Rect.Dy()
	r := c.clampRect(image.Rect(gx, gy, gx+mw, gy+mh))
	if r.Empty() {
		return
	}
	sr := uint32(col.R) * 0x101 // 16-бит premultiplied компоненты
	sg := uint32(col.G) * 0x101
	sb := uint32(col.B) * 0x101
	sa := uint32(col.A) * 0x101
	const m16 = 1<<16 - 1
	mask := alpha.Pix
	mStride := alpha.Stride
	dst := c.back.Pix
	for yy := r.Min.Y; yy < r.Max.Y; yy++ {
		mRow := (yy - gy) * mStride
		dOff := c.back.PixOffset(r.Min.X, yy)
		for xx := r.Min.X; xx < r.Max.X; xx++ {
			ma := uint32(mask[mRow+(xx-gx)])
			if ma == 0 {
				dOff += 4
				continue
			}
			ma |= ma << 8 // 0..0xffff
			a := sa * ma / m16
			inv := m16 - a
			p := dst[dOff : dOff+4 : dOff+4]
			p[0] = uint8((uint32(p[0])*0x101*inv/m16 + sr*ma/m16) >> 8)
			p[1] = uint8((uint32(p[1])*0x101*inv/m16 + sg*ma/m16) >> 8)
			p[2] = uint8((uint32(p[2])*0x101*inv/m16 + sb*ma/m16) >> 8)
			p[3] = uint8((uint32(p[3])*0x101*inv/m16 + sa*ma/m16) >> 8)
			dOff += 4
		}
	}
}

// runeAdvance возвращает ширину руны в выбранном (с учётом fallback) шрифте.
// Для непокрытых рун использует ширину пробела (соответствует drawTextWithFont).
func (c *Canvas) runeAdvance(fc *FontCache, r rune, sizePt float64) fixed.Int26_6 {
	chosen, found := c.fcForRune(fc, r)
	face := chosen.Face(sizePt)
	a, ok := face.GlyphAdvance(r)
	if !ok || !found {
		if sp, ok2 := fc.Face(sizePt).GlyphAdvance(' '); ok2 {
			return sp
		}
	}
	return a
}

// measureWithFallback измеряет ширину строки с учётом fallback-шрифтов.
func (c *Canvas) measureWithFallback(fc *FontCache, text string, sizePt float64) int {
	// Сложный текст: ширина после шейпинга (лигатуры меняют ширину строки).
	if needsShaping(text) {
		if l := c.shaper.layout(fc, c.fallbacks, text, sizePt); l != nil {
			return l.width.Round()
		}
	}
	if len(c.fallbacks) == 0 {
		return fc.Measure(text, sizePt)
	}
	var w fixed.Int26_6
	for _, r := range text {
		w += c.runeAdvance(fc, r, sizePt)
	}
	return w.Round()
}

// drawShapedText рисует строку через шейпер: глифы в видимом порядке,
// маски растеризуются по GID и кэшируются. baseline — Y базовой линии.
// false — шейпинг недоступен, вызывающий код рисует per-rune путём.
func (c *Canvas) drawShapedText(fc *FontCache, text string, x, baseline int, sizePt float64, col color.RGBA) bool {
	l := c.shaper.layout(fc, c.fallbacks, text, sizePt)
	if l == nil {
		return false
	}
	sizePx := fixed.Int26_6(sizePt * fc.dpi / 72.0 * 64.0)
	pen := fixed.I(x)
	for _, g := range l.glyphs {
		// GID 0 = .notdef: руна не покрыта ни одним шрифтом — не рисуем
		// «тофу»-квадрат (философия BUG-2), но перо продвигаем.
		if g.gid == 0 {
			pen += g.adv
			continue
		}
		m := c.shaper.glyphMaskFor(g.face, g.gid, sizePx)
		if m.mask != nil {
			// Точка отрисовки: перо + XOffset; YOffset положителен вверх.
			gx := (pen + g.xOff).Round() + m.offX
			gy := baseline - g.yOff.Round() + m.offY
			c.drawAlphaMask(m.mask, gx, gy, col)
		}
		pen += g.adv
	}
	return true
}

// shapedRunePositions — накопленные ширины по логическим рунам для сложного
// текста: advance каждого кластера распределяется поровну между его рунами.
// Для RTL это ЛОГИЧЕСКИЕ позиции (для каретки TextInput), не визуальные.
func (c *Canvas) shapedRunePositions(fc *FontCache, text string, sizePt float64) []int {
	l := c.shaper.layout(fc, c.fallbacks, text, sizePt)
	if l == nil {
		return nil
	}
	runes := []rune(text)
	n := len(runes)
	// Суммарный advance по кластерам (cluster = индекс первой руны кластера).
	clusterAdv := make(map[int]fixed.Int26_6, n)
	for _, g := range l.glyphs {
		clusterAdv[g.cluster] += g.adv
	}
	// Границы кластеров в логическом порядке.
	starts := make([]int, 0, len(clusterAdv))
	for s := range clusterAdv {
		starts = append(starts, s)
	}
	sort.Ints(starts)

	pos := make([]int, n+1)
	var acc fixed.Int26_6
	for ci, s := range starts {
		end := n
		if ci+1 < len(starts) {
			end = starts[ci+1]
		}
		adv := clusterAdv[s]
		cnt := end - s
		if cnt <= 0 {
			continue
		}
		for k := 1; k <= cnt; k++ {
			pos[s+k] = (acc + adv*fixed.Int26_6(k)/fixed.Int26_6(cnt)).Round()
		}
		acc += adv
	}
	return pos
}

// MeasureText возвращает ширину строки в пикселях (шрифт по умолчанию, sizePt).
func (c *Canvas) MeasureText(text string, sizePt float64) int {
	return c.measureWithFallback(c.fontCache, text, sizePt)
}

// MeasureTextFont возвращает ширину строки именованным шрифтом.
func (c *Canvas) MeasureTextFont(text string, sizePt float64, fontName string) int {
	return c.measureWithFallback(c.fontFor(fontName), text, sizePt)
}

// MeasureRunePositions возвращает накопленную ширину после каждого символа.
// Результат: len(runes)+1 элементов; positions[0]==0, positions[n] — ширина text[:n].
// Для сложного текста (шейпинг) позиции считаются по кластерам в ЛОГИЧЕСКОМ
// порядке рун — визуальное позиционирование каретки в RTL не поддержано (v1).
func (c *Canvas) MeasureRunePositions(text string, sizePt float64) []int {
	if needsShaping(text) {
		if pos := c.shapedRunePositions(c.fontCache, text, sizePt); pos != nil {
			return pos
		}
	}
	if len(c.fallbacks) == 0 {
		return c.fontCache.MeasureRunes(text, sizePt)
	}
	runes := []rune(text)
	pos := make([]int, len(runes)+1)
	var w fixed.Int26_6
	for i, r := range runes {
		w += c.runeAdvance(c.fontCache, r, sizePt)
		pos[i+1] = w.Round()
	}
	return pos
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
	return c.diffTiles(0, 0, c.tilesX-1, c.tilesY-1)
}

// diffAndSyncIn — как diffAndSync, но сравнивает только тайлы, пересекающие
// region (damage-область из InvalidateRect). Тайлы вне region не сравниваются
// и НЕ синхронизируются — контракт: вызывающий заявил все изменённые области.
func (c *Canvas) diffAndSyncIn(region image.Rectangle) []output.DirtyTile {
	region = region.Intersect(image.Rect(0, 0, c.W, c.H))
	if region.Empty() {
		return nil
	}
	ts := output.TileSize
	tx0 := region.Min.X / ts
	ty0 := region.Min.Y / ts
	tx1 := (region.Max.X - 1) / ts
	ty1 := (region.Max.Y - 1) / ts
	return c.diffTiles(tx0, ty0, tx1, ty1)
}

// diffTiles сравнивает тайлы в диапазоне индексов [tx0..tx1]×[ty0..ty1].
// При достаточном объёме работа распараллеливается по рядам тайлов: каждый
// воркер обрабатывает непересекающийся диапазон памяти (сравнение, извлечение
// и синхронизация front — всё в границах своих рядов), поэтому гонок нет.
func (c *Canvas) diffTiles(tx0, ty0, tx1, ty1 int) []output.DirtyTile {
	if ty1 >= c.tilesY {
		ty1 = c.tilesY - 1
	}
	if tx1 >= c.tilesX {
		tx1 = c.tilesX - 1
	}
	if ty0 > ty1 || tx0 > tx1 {
		return nil
	}

	rows := ty1 - ty0 + 1
	total := rows * (tx1 - tx0 + 1)
	workers := runtime.GOMAXPROCS(0)
	if workers > rows {
		workers = rows
	}
	// Порог: параллелить только заметный объём — сравнение одного тайла
	// это ~16 КБ memcmp, на мелких диффах горутины дороже выигрыша.
	if workers <= 1 || total < 64 {
		return c.diffTileRows(tx0, ty0, tx1, ty1)
	}

	results := make([][]output.DirtyTile, workers)
	var wg sync.WaitGroup
	chunk := (rows + workers - 1) / workers
	for i := 0; i < workers; i++ {
		r0 := ty0 + i*chunk
		r1 := r0 + chunk - 1
		if r1 > ty1 {
			r1 = ty1
		}
		if r0 > r1 {
			break
		}
		wg.Add(1)
		go func(idx, a, b int) {
			defer wg.Done()
			results[idx] = c.diffTileRows(tx0, a, tx1, b)
		}(i, r0, r1)
	}
	wg.Wait()

	var tiles []output.DirtyTile
	for _, part := range results {
		tiles = append(tiles, part...)
	}
	return tiles
}

// diffTileRows — последовательный diff тайлов в диапазоне рядов [ty0..ty1].
func (c *Canvas) diffTileRows(tx0, ty0, tx1, ty1 int) []output.DirtyTile {
	ts := output.TileSize
	var tiles []output.DirtyTile

	for ty := ty0; ty <= ty1; ty++ {
		for tx := tx0; tx <= tx1; tx++ {
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
