package geom

import "testing"

// Имена фильтров — часть журнала и отладочного вывода: менять их нельзя без
// причины.
func TestImageFilterNames(t *testing.T) {
	want := map[ImageFilter]string{
		FilterNearest:    "nearest",
		FilterBilinear:   "bilinear",
		FilterCatmullRom: "catmullrom",
		ImageFilter(99):  "unknown",
	}
	for f, name := range want {
		if got := f.String(); got != name {
			t.Errorf("ImageFilter(%d).String() = %q, ждали %q", uint8(f), got, name)
		}
	}
}

// Порядок значений фильтров — часть бинарного контракта с сохранёнными
// настройками: nearest — ноль, как у переменной, не заданной явно.
func TestImageFilterValues(t *testing.T) {
	if FilterNearest != 0 || FilterBilinear != 1 || FilterCatmullRom != 2 {
		t.Fatalf("значения фильтров сдвинуты: %d %d %d", FilterNearest, FilterBilinear, FilterCatmullRom)
	}
}

func TestPoint2FZeroValue(t *testing.T) {
	var p Point2F
	if p.X != 0 || p.Y != 0 {
		t.Fatalf("нулевая точка не нулевая: %+v", p)
	}
}
