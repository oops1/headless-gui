package raster

// window.go — холст-окно: холст воркера тайлового растра (ADR-0005).
//
// Буфер окна — тайл (или ореол) кадра: image.RGBA с Rect в ФИЗИЧЕСКИХ
// координатах кадра, не от нуля. Сдвига координат нет нигде: PhysX, пути
// AA-фигур, фаза подпикселя текста, зерно шума, положение в обоях и буферы
// теней считаются ровно как у холста движка, а окно лишь зажимает запись
// (clampRect по back.Bounds()). Границы кадра (frame) хранятся отдельно: по
// ним считаются клип, Clip() и область чтения размытой подложки.
//
// У окна личные виды шрифтов (FontCache.NewView, FallbackFont.View) над
// общим кэшем растров: замок одного вида между воркерами съедал бы
// параллелизм. Таблицы именованных шрифтов и семейств общие, личный вид
// заводится лениво на каждый общий. Признак тайла — один, центрального тайла
// кадра (regions.go). Из окна нельзя звать пул воркеров (noPool).

import (
	"image"

	hgtext "github.com/oops1/headless-gui/v3/core/text"
	"github.com/oops1/headless-gui/v3/output"
)

// NewWindowCanvas создаёт холст-окно над холстом движка src: тот же масштаб,
// формат байтов, обои (по наследованию), журнал сбросов картинок; виды
// шрифтов — личные. Окно задаётся Retarget.
func NewWindowCanvas(src *Canvas) *Canvas { return newWindowCanvas(src, false) }

// NewWindowCanvasShared — окно для горутины кадра (последовательный путь,
// снимки рекордера): шрифты и шейпер холста движка без личных видов —
// замок вида ни с кем не делится, а личный вид стоил бы второго кэша глифов
// на каждый кадр из пары тайлов.
func NewWindowCanvasShared(src *Canvas) *Canvas { return newWindowCanvas(src, true) }

func newWindowCanvas(src *Canvas, shared bool) *Canvas {
	c := &Canvas{
		isWindow:    true,
		noPool:      true,
		sharedFonts: shared,
		frame:       src.Frame(),
		fontCache:   src.fontCache.NewView(),
		namedFonts:  src.namedFonts,
		families:    src.families,
		shaper:      &hgtext.Shaper{},
		views:       make(map[*hgtext.FontCache]*hgtext.FontCache),
		viewRev:     src.fontRev.Load(),
		W:          src.W,
		H:          src.H,
		scale:      src.scale,
		logicalW:   src.logicalW,
		logicalH:   src.logicalH,
		format:     src.format,
		formatOwn:  src.format,
		bgImage:    src.bgImage,
	}
	c.fallbacks = make([]*hgtext.FallbackFont, len(src.fallbacks))
	for i, fb := range src.fallbacks {
		c.fallbacks[i] = fb.View()
	}
	if shared {
		c.fontCache, c.fallbacks, c.shaper, c.views = src.fontCache, src.fallbacks, src.shaper, nil
	}
	c.InheritWallpaper(src)
	if src.inv != nil {
		c.inv, c.invSeen = src.inv, src.inv.seq.Load()
	}
	c.mica = &micaCache{levels: map[int]*micaLevel{}}
	c.cullingOn.Store(src.cullingOn.Load())
	c.occlusionOn.Store(src.occlusionOn.Load())
	c.shadowCacheOff = src.shadowCacheOff
	c.back = &image.RGBA{Rect: image.Rectangle{}}
	c.backOwn = c.back
	return c
}

// IsWindow сообщает, что холст — окно кадра.
func (c *Canvas) IsWindow() bool { return c.isWindow }

// Frame — границы кадра (физические): у окна — кадр холста движка, у
// обычного холста — его размер (back холста движка — окно над front,
// ADR-0007, и его границы кадром не являются).
func (c *Canvas) Frame() image.Rectangle {
	if c.isWindow {
		return c.frame
	}
	return image.Rect(0, 0, c.W, c.H)
}

// Retarget переводит окно на прямоугольник window (физические координаты
// кадра): буфер растёт до нужного размера и остаётся, состояние холста
// сбрасывается (клип, скруглённый и базовый клипы, признак). Пиксели буфера
// после этого не определены — их задаёт префилл.
func (c *Canvas) Retarget(window image.Rectangle) {
	window = window.Intersect(c.frame)
	w, h := window.Dx(), window.Dy()
	if w <= 0 || h <= 0 {
		w, h = 1, 1
	}
	need := w * h * 4
	if cap(c.back.Pix) < need {
		c.back.Pix = make([]byte, need)
	}
	c.back.Pix = c.back.Pix[:need]
	c.back.Stride = w * 4
	c.back.Rect = window
	c.hasClip, c.hasBase = false, false
	c.round.active = false
	c.centerMark = tileMark{}
	c.center = window
}

// SyncFrame подтягивает у окна то, что у холста движка могло смениться между
// кадрами: формат байтов back (чужая память потребителя), границы кадра,
// ревизию шрифтов (новые личные виды), выключатель кэша теней.
func (c *Canvas) SyncFrame(src *Canvas) {
	c.frame = src.Frame()
	c.wallParent = src.wallOwner()
	c.bgImage = src.bgImage
	c.inv = src.inv
	c.W, c.H = src.W, src.H
	c.scale, c.logicalW, c.logicalH = src.scale, src.logicalW, src.logicalH
	if c.format != src.format {
		c.setOwnFormat(src.format)
		c.scaledCache, c.scaledPixels = nil, 0
	}
	c.shadowCacheOff = src.shadowCacheOff
	if c.sharedFonts {
		c.fontCache, c.fallbacks, c.shaper = src.fontCache, src.fallbacks, src.shaper
		return
	}
	if rev := src.fontRev.Load(); rev != c.viewRev || !c.fontCache.SameParams(src.fontCache) {
		c.viewRev = rev
		c.fontCache = src.fontCache.NewView()
		clear(c.views)
		c.fallbacks = c.fallbacks[:0]
		for _, fb := range src.fallbacks {
			c.fallbacks = append(c.fallbacks, fb.View())
		}
		c.shaper = &hgtext.Shaper{}
	}
}

// view — личный вид общего шрифта fc (таблица имён и семейств отдаёт общие
// виды холста движка).
func (c *Canvas) view(fc *hgtext.FontCache) *hgtext.FontCache {
	if c.views == nil || fc == nil {
		return fc
	}
	if v, ok := c.views[fc]; ok {
		return v
	}
	v := fc.NewView()
	c.views[fc] = v
	return v
}

// SetCenter задаёт центральный тайл окна (физические координаты кадра);
// признак тайла начинается пустым — его ставят фон окна и команды.
func (c *Canvas) SetCenter(tile image.Rectangle) {
	c.center = tile
	c.centerMark = tileMark{}
}

// StoreCenterMark записывает признак центрального тайла в холст движка.
func (c *Canvas) StoreCenterMark(main *Canvas, tileIndex int) {
	if tileIndex >= 0 && tileIndex < len(main.marks) {
		main.marks[tileIndex] = c.centerMark
	}
}

// PrefillFrom копирует в окно строки области r (⊆ окно) из буфера src,
// лежащего в тех же координатах кадра (back холста движка или снимок).
func (c *Canvas) PrefillFrom(src *image.RGBA, r image.Rectangle) {
	r = r.Intersect(c.back.Rect).Intersect(src.Rect)
	if r.Empty() {
		return
	}
	n := r.Dx() * 4
	for y := r.Min.Y; y < r.Max.Y; y++ {
		d := c.back.PixOffset(r.Min.X, y)
		s := src.PixOffset(r.Min.X, y)
		copy(c.back.Pix[d:d+n], src.Pix[s:s+n])
	}
}

// CopyTo копирует строки области r (⊆ окно) из окна в буфер dst тех же
// координат (back холста движка, растр кэша размытия).
func (c *Canvas) CopyTo(dst *image.RGBA, r image.Rectangle) {
	r = r.Intersect(c.back.Rect).Intersect(dst.Rect)
	if r.Empty() {
		return
	}
	n := r.Dx() * 4
	for y := r.Min.Y; y < r.Max.Y; y++ {
		s := c.back.PixOffset(r.Min.X, y)
		d := dst.PixOffset(r.Min.X, y)
		copy(dst.Pix[d:d+n], c.back.Pix[s:s+n])
	}
}

// SetScaledOverride задаёт источник готовых растров крупных картинок
// (ADR-0005: масштабирование на горутине кадра, а не в каждом воркере):
// scaledFor спрашивает его первым.
func (c *Canvas) SetScaledOverride(fn func(src image.Image, pw, ph int) *image.RGBA) {
	c.scaledOverride = fn
}

// SetScaledBudget задаёт бюджет пикселей кэша масштабирования холста.
func (c *Canvas) SetScaledBudget(pixels int) {
	if pixels < 1 {
		pixels = 1
	}
	c.scaledBudget = pixels
}

// BlitBackdrop кладёт готовый растр слоя размытой подложки (попадание кэша,
// ADR-0005): строки raster (Rect — физические координаты кадра) копируются
// как есть в формате back внутри клипа и скругления, признаки тайлов — как у
// самого слоя: картинка по области и заливка тинтом (без самой заливки:
// тинт уже в растре). r — логическая область слоя (для признака тинта).
func (c *Canvas) BlitBackdrop(raster *image.RGBA, r image.Rectangle, tint colorRGBA) {
	area := c.clampRect(raster.Rect)
	if !area.Empty() {
		c.MarkImage(area)
		for y := area.Min.Y; y < area.Max.Y; y++ {
			x0, x1, ok := c.round.spanX(y, area.Min.X, area.Max.X)
			if !ok {
				continue
			}
			d := c.back.PixOffset(x0, y)
			s := raster.PixOffset(x0, y)
			copy(c.back.Pix[d:d+(x1-x0)*4], raster.Pix[s:s+(x1-x0)*4])
		}
	}
	if tint.A > 0 {
		c.markSolid(c.PhysRect(r), c.enc(tint), tint.A == 255)
	}
}

// TileRect — прямоугольник тайла i холста движка (физический).
func (c *Canvas) TileRect(i int) image.Rectangle {
	ts := output.TileSize
	tx, ty := i%c.tilesX, i/c.tilesX
	return image.Rect(tx*ts, ty*ts, min(tx*ts+ts, c.W), min(ty*ts+ts, c.H))
}

// BackImage — буфер back холста: окно воркера, чужая память потребителя
// или окно back холста движка (singlebuf.go).
func (c *Canvas) BackImage() *image.RGBA { return c.back }

// ScaledFor — растр картинки src, масштабированной до pw×ph физических
// пикселей, из кэша холста (горутина кадра готовит крупные картинки для
// воркеров).
func (c *Canvas) ScaledFor(src image.Image, pw, ph int) *image.RGBA { return c.scaledFor(src, pw, ph) }

// ScaleThickness — физическая толщина/радиус логической величины t.
func (c *Canvas) ScaleThickness(t int) int { return c.st(t) }

// NoteBackdrop запоминает область слоя размытия r (логическая) текущего
// кадра для расширения следующего частичного кадра — как это делает сам
// BlurBehind; горутина кадра зовёт его за слои, нарисованные в окнах.
func (c *Canvas) NoteBackdrop(r image.Rectangle) { c.noteBackdrop(c.PhysRect(r)) }
