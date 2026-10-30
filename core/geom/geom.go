// Package geom — геометрические и фильтровые типы, которыми обмениваются
// растеризатор и виджеты.
//
// Пакет не зависит ни от движка, ни от виджетов: его могут использовать
// растеризатор (core/raster), печать и внешние инструменты без всего
// остального. В widget эти типы доступны через алиасы с теми же именами, так
// что код приложений не меняется.
package geom

// Point2F — точка с дробными координатами в логических единицах.
//
// Координаты те же, что у целочисленных примитивов: целое значение — центр
// точки. Иначе путь, собранный из дробных точек, и отрезок DrawLineAA между
// теми же целыми точками разошлись бы на полпикселя.
type Point2F struct{ X, Y float64 }

// ImageFilter — ядро передискретизации DrawImageRect.
type ImageFilter uint8

const (
	// FilterNearest — ближайший пиксель: без сглаживания, быстрее всех. Для
	// увеличения «до пикселей» (просмотр картинки в масштабе 800 %) и пиксель-арта.
	FilterNearest ImageFilter = iota
	// FilterBilinear — билинейное ядро (как у DrawImageScaled). При уменьшении
	// усредняет все накрытые пиксели источника, а не только соседние.
	FilterBilinear
	// FilterCatmullRom — бикубическое ядро Кэтмулла-Рома: чётче билинейного,
	// заметно медленнее; даёт небольшой «звон» у резких границ.
	FilterCatmullRom
)

// String возвращает имя фильтра.
func (f ImageFilter) String() string {
	switch f {
	case FilterNearest:
		return "nearest"
	case FilterBilinear:
		return "bilinear"
	case FilterCatmullRom:
		return "catmullrom"
	}
	return "unknown"
}
