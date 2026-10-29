package raster

// canvas_engine.go — то, что движок (engine) читает и задаёт у холста:
// буферы, формат, шрифты, режимы кадра и отсечения, кэши. Поля холста закрыты,
// а движок — другой пакет (задача 4.1 плана 4.0). Методы с пометкой «для
// тестов и диагностики» нужны проверкам движка, которые смотрят на холст
// после кадра.

import (
	"image"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"

	hgtext "github.com/oops1/headless-gui/v3/core/text"
	"github.com/oops1/headless-gui/v3/output"
)

// AddDefaultFonts подключает шрифты, которые движок даёт всякому холсту.
//
// Запасная цепочка — шрифты с широким покрытием символов (✓ ✗ ⚠,
// box-drawing, стрелки) к встроенному Go Regular (BUG-2): сначала системные,
// затем наши из assets/fonts — на машине без единого системного шрифта иначе
// некому нарисовать эти символы вовсе. Файлы не читаются: путь регистрируется,
// а шрифт загружается при первом символе, которого нет в основном
// (core/text/fallbackfont.go) — около 22 МБ чтения и разбора на Windows в
// обычный старт не попадают.
//
// Встроенные жирный, курсив и моноширинный Go — для FontWeight/FontStyle
// (P1) и кода (text.BuiltinFont*).
func (c *Canvas) AddDefaultFonts() {
	for _, p := range append(hgtext.SystemFallbackFontPaths(), hgtext.AssetFallbackFontPaths()...) {
		c.addFallbackFontFile(p)
	}
	c.RegisterFont(hgtext.BuiltinFontBold, gobold.TTF)
	c.RegisterFont(hgtext.BuiltinFontItalic, goitalic.TTF)
	c.RegisterFont(hgtext.BuiltinFontBoldItalic, gobolditalic.TTF)
	c.RegisterFont(hgtext.BuiltinFontMono, gomono.TTF)
}

// Back — буфер, в который рисуют: собственный или поверхность потребителя.
func (c *Canvas) Back() *image.RGBA { return c.back }

// Front — последний отданный кадр: с ним сравнивается Back.
func (c *Canvas) Front() *image.RGBA { return c.front }

// UsingOwnBack сообщает, рисует ли холст в собственный буфер, а не в
// поверхность потребителя.
func (c *Canvas) UsingOwnBack() bool { return c.back == c.backOwn }

// AttachBack делает буфером рисования img формата f — поверхность
// потребителя (Engine.SetSurfaces).
func (c *Canvas) AttachBack(img *image.RGBA, f PixelFormat) {
	c.back, c.format = img, f
	c.backClean = false // front уйдёт вперёд окна (singlebuf.go)
}

// Format — порядок каналов текущего буфера рисования.
func (c *Canvas) Format() PixelFormat { return c.format }

// SetPixelFormat задаёт порядок каналов собственного буфера и перестраивает
// под него фон bg (nil — без фона) и обои Mica: BlitBackground каждый кадр
// делает чистый memcpy заранее закодированного фона. false — текущий формат
// уже f, ничего не менялось.
func (c *Canvas) SetPixelFormat(f PixelFormat, bg image.Image) bool {
	if c.format == f {
		return false
	}
	c.setOwnFormat(f)
	if bg != nil {
		c.SetBackground(bg) // перестроить фон в новом порядке байт
	}
	if c.wallImg != nil {
		c.SetWallpaperSource(c.wallImg) // и обои Mica
	}
	c.resetMica() // размытые обои закодированы в прежнем порядке
	return true
}

// DefaultFont — шрифт по умолчанию холста.
func (c *Canvas) DefaultFont() *hgtext.FontCache { return c.fontCache }

// NamedFonts — реестр именованных шрифтов холста.
func (c *Canvas) NamedFonts() *hgtext.FontTable { return c.namedFonts }

// Fallbacks — запасная цепочка шрифтов по порядку; срез только для чтения.
func (c *Canvas) Fallbacks() []*hgtext.FallbackFont { return c.fallbacks }

// Shaper — шейпер сложного текста холста.
func (c *Canvas) Shaper() *hgtext.Shaper { return c.shaper }

// FontRev — ревизия шрифтов холста: растёт при регистрации шрифта и смене
// настроек текста. По ней сбрасывается то, что от шрифтов зависит (клоны
// оверлеев).
func (c *Canvas) FontRev() uint64 { return c.fontRev.Load() }

// SetImageInvalidator подключает журнал сбросов кэша картинок
// (Engine.InvalidateImageCache).
func (c *Canvas) SetImageInvalidator(inv *ImageInvalidator) { c.inv = inv }

// SetFrameOptions — как извлекать тайлы кадра: в переработанные буферы
// (Engine.SetFrameRecycling) и с байтами или без (Engine.SetFrameTileData).
func (c *Canvas) SetFrameOptions(recycle, tileData bool) {
	c.recycle, c.noTileData = recycle, !tileData
}

// SetSubtreeCulling включает пропуск поддеревьев вне повреждения
// (Engine.SetSubtreeCulling).
func (c *Canvas) SetSubtreeCulling(on bool) { c.cullingOn.Store(on) }

// SetOcclusionCulling включает вычитание перекрытого
// (Engine.SetOcclusionCulling).
func (c *Canvas) SetOcclusionCulling(on bool) { c.occlusionOn.Store(on) }

// SetShadowCacheEnabled включает сборку теней из шаблона; переключение
// сбрасывает собранные шаблоны.
func (c *Canvas) SetShadowCacheEnabled(on bool) { c.shadowCacheOff, c.shadowCache = !on, nil }

// ShadowCacheEnabled сообщает, собираются ли тени из шаблона.
func (c *Canvas) ShadowCacheEnabled() bool { return !c.shadowCacheOff }

// ResetForRedraw готовит клон оверлея к новому рендеру: чистый буфер, без
// отсечений.
func (c *Canvas) ResetForRedraw() {
	clear(c.back.Pix)
	c.hasClip = false
	c.hasBase = false
}

// TileGrid — число тайлов холста по горизонтали и вертикали.
func (c *Canvas) TileGrid() (cols, rows int) { return c.tilesX, c.tilesY }

// TileRegion — разметка тайла i (ряд за рядом, см. TileGrid) в текущем
// кадре: тип области и задет ли тайл рисованием. Для тестов и диагностики.
func (c *Canvas) TileRegion(i int) (kind output.RegionKind, touched bool) {
	m := c.marks[i]
	return m.kind, m.touched
}

// ShadowCacheLen — число шаблонов теней в кэше. Для тестов и диагностики.
func (c *Canvas) ShadowCacheLen() int { return len(c.shadowCache) }

// ScaledCacheLen — число масштабированных картинок в кэше. Для тестов и
// диагностики.
func (c *Canvas) ScaledCacheLen() int { return len(c.scaledCache) }
