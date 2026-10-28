// scale.go — HiDPI: масштабирование логических координат в физические.
//
// Модель координат:
//   - виджеты живут в ЛОГИЧЕСКИХ пикселях (XAML Width/Height, Bounds,
//     события мыши, Measure*) — как WPF DIP;
//   - back/front-буферы канваса — ФИЗИЧЕСКИЕ пиксели (логические × scale);
//   - все операции DrawContext скейлят координаты на входе; текст
//     растеризуется в физическом размере через DPI шрифтов (96 × scale) —
//     на HiDPI-мониторе он реально чётче, а не растянут.
//
// Округление — по краям (edge-based): физическая граница = round(край × K).
// Соседние логические прямоугольники дают общую физическую границу — без
// щелей и нахлёстов на дробных масштабах (1.25, 1.5).
//
// При scale == 1 все хелперы тождественны и не меняют прежнее поведение.
package engine

import (
	"image"
	"math"
)

// sx масштабирует логическую координату в физическую (edge-based).
func (c *Canvas) sx(v int) int {
	if c.scale == 1 {
		return v
	}
	return int(math.Round(float64(v) * c.scale))
}

// sl масштабирует длину, привязанную к началу v (edge-based: физическая
// длина = граница(v+l) − граница(v), поэтому смежные отрезки стыкуются).
func (c *Canvas) sl(v, l int) int {
	if c.scale == 1 {
		return l
	}
	return c.sx(v+l) - c.sx(v)
}

// st масштабирует толщину (минимум 1 физический пиксель).
func (c *Canvas) st(t int) int {
	if c.scale == 1 {
		return t
	}
	p := int(math.Round(float64(t) * c.scale))
	if p < 1 && t > 0 {
		p = 1
	}
	return p
}

// sRect масштабирует логический прямоугольник в физический (по краям).
func (c *Canvas) sRect(r image.Rectangle) image.Rectangle {
	if c.scale == 1 {
		return r
	}
	return image.Rect(c.sx(r.Min.X), c.sx(r.Min.Y), c.sx(r.Max.X), c.sx(r.Max.Y))
}

// unRect переводит физический прямоугольник в логический (с расширением
// до целых логических пикселей — для Clip()).
func (c *Canvas) unRect(r image.Rectangle) image.Rectangle {
	if c.scale == 1 {
		return r
	}
	k := c.scale
	return image.Rect(
		int(math.Floor(float64(r.Min.X)/k)),
		int(math.Floor(float64(r.Min.Y)/k)),
		int(math.Ceil(float64(r.Max.X)/k)),
		int(math.Ceil(float64(r.Max.Y)/k)),
	)
}

// Scale возвращает текущий масштаб канваса (1.0 = без HiDPI).
func (c *Canvas) Scale() float64 { return c.scale }

// LogicalSize возвращает логический размер холста.
func (c *Canvas) LogicalSize() (w, h int) { return c.logicalW, c.logicalH }

// cloneForSize создаёт канвас нового логического размера/масштаба, сохраняя
// реестры шрифтов (именованные, fallback-цепочку) и исходный фон.
// Используется SetResolution/SetScale: раньше пересоздание канваса теряло
// зарегистрированные шрифты.
func (c *Canvas) cloneForSize(w, h int, scale float64, bgSrc image.Image) *Canvas {
	return c.adopt(newCanvasScaled(w, h, scale, c.fontCache), bgSrc)
}

// cloneForPhys — cloneForSize с явным физическим размером (scalewithin.go).
func (c *Canvas) cloneForPhys(pw, ph, w, h int, scale float64, bgSrc image.Image) *Canvas {
	return c.adopt(newCanvasPhys(pw, ph, w, h, scale, c.fontCache), bgSrc)
}

// adopt переносит на новый холст nc реестры шрифтов, настройки и фон.
func (c *Canvas) adopt(nc *Canvas, bgSrc image.Image) *Canvas {
	nc.namedFonts = c.namedFonts
	nc.families = c.families
	nc.fallbacks = c.fallbacks
	// Настройки движка переезжают вместе с холстом: смена разрешения не
	// повод молча вернуть пропуск поддеревьев или порядок каналов к
	// значениям по умолчанию.
	nc.cullingOn.Store(c.cullingOn.Load())
	nc.occlusionOn.Store(c.occlusionOn.Load())
	// Журнал сбросов кэша изображений общий на движок: новый холст начинает с
	// пустого кэша, поэтому прежние записи журнала ему применять незачем.
	if c.inv != nil {
		nc.inv, nc.invSeen = c.inv, c.inv.seq.Load()
	}
	// Формат — только СОБСТВЕННОГО буфера. Новый холст рисует в свою память:
	// буфер потребителя, отданный через SetSurface, размера прежнего экрана и
	// после смены разрешения не годится, а его порядок каналов относится
	// только к нему. Потребитель отдаёт буфер заново.
	nc.formatOwn = c.formatOwn
	nc.format = c.formatOwn
	if bgSrc != nil {
		nc.setBackground(bgSrc)
	}
	// Отдельный источник обоев Mica переживает смену разрешения так же, как фон.
	if c.wallImg != nil {
		nc.setWallpaperSource(c.wallImg)
	}
	return nc
}

// setDPIAll задаёт DPI всем шрифтам канваса (default, именованные, fallback)
// и сбрасывает кэши глифов, метрик и раскладок шейпера.
func (c *Canvas) setDPIAll(dpi float64) {
	c.fontCache.SetDPI(dpi)
	for _, fc := range c.namedFonts.All() {
		fc.SetDPI(dpi)
	}
	for _, fb := range c.fallbacks {
		fb.Apply(func(fc *FontCache) { fc.SetDPI(dpi) }) // незагруженные возьмут DPI при загрузке
	}
	c.shaper.DropLayouts()
}
