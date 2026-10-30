package raster

// canvas_record.go — то, что холст отдаёт рекордеру команд (core/draw,
// ADR-0004): верхняя оценка охвата строки без измерения, округление клипа и
// признак обоев. Всё — чтение; пиксели холста здесь не трогаются.

import (
	"image"
	"math"
	"unicode/utf8"

	hgtext "github.com/oops1/headless-gui/v4/core/text"
)

// TextBounds — верхняя оценка логического охвата строки text у точки (x, y)
// кеглем sizePt шрифтом fontName, без измерения строки: число рун ×
// максимальный шаг глифа по основному шрифту и уже загруженным запасным,
// запас по горизонтали как у отсечки отрисовки (clipPenSlackX), по
// вертикали — em сверху и снизу (глифы запасных шрифтов и цветные эмодзи
// выходят за метрики основного). Завышенный охват стоит лишнего пропуска,
// заниженный был бы дефектом.
func (c *Canvas) TextBounds(text string, x, y int, sizePt float64, fontName string) image.Rectangle {
	fc := c.FontFor(fontName)
	n := utf8.RuneCountInString(text)
	maxAdv := fc.MaxAdvance(sizePt).Ceil()
	asc, desc := fc.VMetrics(sizePt)
	for _, fb := range c.fallbacks {
		f := fb.Loaded()
		if f == nil {
			continue
		}
		if a := f.MaxAdvance(sizePt).Ceil(); a > maxAdv {
			maxAdv = a
		}
		a, d := f.VMetrics(sizePt)
		asc, desc = max(asc, a), max(desc, d)
	}
	em := int(math.Ceil(sizePt * fc.DPI() / 72))
	// Запасной шрифт, который загрузится только при отрисовке (эмодзи, CJK),
	// в оценке не участвует: шаг не уже em — обычная ширина таких глифов.
	if maxAdv < em {
		maxAdv = em
	}
	k := c.scale
	up := func(v int) int { return int(math.Ceil(float64(v)/k)) + 1 }
	return image.Rect(x-up(clipPenSlackX), y-up(em), x+up(n*maxAdv+clipPenSlackX), y+up(asc+desc+em))
}

// LogicalClip возвращает логический прямоугольник, который Clip() отдал бы
// после SetClip(r), без базового клипа кадра: округление на дробном
// масштабе то же, что у холста.
func (c *Canvas) LogicalClip(r image.Rectangle) image.Rectangle {
	return c.unRect(c.PhysRect(r).Intersect(c.Frame()))
}

// HasWallpaper сообщает, есть ли у холста (или его владельца обоев)
// источник для Mica: то, что MicaBehind вернул бы без отрисовки.
func (c *Canvas) HasWallpaper() bool {
	owner := c
	if c.wallParent != nil {
		owner = c.wallParent
	}
	return owner.wallpaper() != nil
}

// RegisterFontData — проверка типа: холст — измеритель рекордера.
var _ interface {
	TextBounds(text string, x, y int, sizePt float64, fontName string) image.Rectangle
	LogicalClip(r image.Rectangle) image.Rectangle
	HasWallpaper() bool
	MeasureText(text string, sizePt float64) int
	FontMetrics(fontName string, sizePt float64) (int, int, int)
	HasGlyph(r rune) bool
	OcclusionEnabled() bool
	Scale() float64
	LogicalSize() (int, int)
	InvalidateImageCache(src image.Image)
} = (*Canvas)(nil)

// maxAdvanceOf — для тестов: шаг в логических пикселях.
func maxAdvanceOf(fc *hgtext.FontCache, sizePt float64) int { return fc.MaxAdvance(sizePt).Ceil() }
