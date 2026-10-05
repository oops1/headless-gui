package svg

import (
	"image"
	"image/color"
	"sync/atomic"
)

// Options — переключатели «точного» режима растеризации. Нулевое значение —
// прежнее поведение пакета, побитно (хеш-тест TestFlatRasterUnchanged и
// golden-тесты движка держатся на нём).
//
// Режим выбирается при РАСТЕРИЗАЦИИ, а не при разборе: один и тот же
// *Document можно нарисовать и так, и этак. Уровней три, от общего к частному:
//
//   - SetDefaultOptions — для всего процесса (в main, до создания окон);
//   - Document.SetOptions — для одного документа (перекрывает общий);
//   - RasterizeWith / RasterizeCachedWith / RenderWith — для одного вызова.
//
// Опции входят в ключ кэша RasterizeCached, поэтому переключение режима не
// отдаёт старую картинку.
type Options struct {
	// StrokeJoins включает настоящую обводку: stroke-linejoin (miter с
	// stroke-miterlimit, round, bevel), stroke-linecap (butt, round, square),
	// stroke-dasharray/stroke-dashoffset и толщину ровно как задана (без
	// «минимальной видимой» 0,75 px). Без флага обводка — упрощённые квады по
	// сегментам, на скруглённых кривых дающие «гребень».
	StrokeJoins bool
	// GroupLayers включает групповую непрозрачность слоем: группа (g, svg,
	// use, а также фигура с заливкой И обводкой) с opacity<1 рисуется в
	// отдельный слой, который затем накладывается целиком. Перекрывающиеся
	// потомки перестают просвечивать друг сквозь друга. Без флага opacity
	// группы по-прежнему умножается в прозрачность каждой её фигуры.
	//
	// Групповые mask и filter слоем рисуются ВСЕГДА (флаг на них не влияет):
	// в плоских значках их нет, а у значков с масками слой даёт верный
	// результат.
	GroupLayers bool
}

// PreciseOptions — все точные режимы разом.
var PreciseOptions = Options{StrokeJoins: true, GroupLayers: true}

var (
	defaultOpts atomic.Uint32
)

func (o Options) bits() uint8 {
	var b uint8
	if o.StrokeJoins {
		b |= 1
	}
	if o.GroupLayers {
		b |= 2
	}
	return b
}

func optionsFromBits(b uint8) Options {
	return Options{StrokeJoins: b&1 != 0, GroupLayers: b&2 != 0}
}

// SetDefaultOptions задаёт режим растеризации для всех документов, у которых
// нет своего (Document.SetOptions). Потокобезопасно; вызывать лучше один раз
// при старте приложения.
func SetDefaultOptions(o Options) { defaultOpts.Store(uint32(o.bits())) }

// DefaultOptions возвращает общий режим растеризации (по умолчанию — нулевой).
func DefaultOptions() Options { return optionsFromBits(uint8(defaultOpts.Load())) }

// SetOptions задаёт режим растеризации этого документа поверх общего
// (SetDefaultOptions). Кэш растеризаций не сбрасывается — опции входят в ключ.
func (d *Document) SetOptions(o Options) {
	d.mu.Lock()
	d.optSet, d.opt = true, o
	d.mu.Unlock()
}

// UseDefaultOptions возвращает документ к общему режиму (SetDefaultOptions).
func (d *Document) UseDefaultOptions() {
	d.mu.Lock()
	d.optSet, d.opt = false, Options{}
	d.mu.Unlock()
}

// Options возвращает действующий для документа режим: собственный, если задан,
// иначе общий.
func (d *Document) Options() Options {
	d.mu.Lock()
	set, o := d.optSet, d.opt
	d.mu.Unlock()
	if set {
		return o
	}
	return DefaultOptions()
}

// RasterizeWith — Rasterize с явным режимом (документные и общие опции не
// учитываются).
func (d *Document) RasterizeWith(w, h int, current color.RGBA, tint bool, o Options) *image.RGBA {
	return d.rasterize(w, h, current, tint, o)
}

// RasterizeCachedWith — RasterizeCached с явным режимом; кэш общий, режим в
// ключе.
func (d *Document) RasterizeCachedWith(w, h int, current color.RGBA, tint bool, o Options) *image.RGBA {
	return d.rasterizeCached(w, h, current, tint, o)
}

// RenderWith — Render с явным режимом.
func RenderWith(doc *Document, w, h int, tint color.RGBA, o Options) *image.RGBA {
	if doc == nil || w <= 0 || h <= 0 {
		return nil
	}
	if tint.A == 0 {
		return doc.RasterizeCachedWith(w, h, defaultCurrentColor, false, o)
	}
	return doc.RasterizeCachedWith(w, h, tint, true, o)
}
