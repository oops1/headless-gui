package datagrid

import (
	"image"
	"testing"
)

// Собственные минимум и максимум колонки — запрос GG-40.
//
// Методы SetMinWidth/SetMaxWidth были, а раскладка и ресайз мышью знали одну
// общую константу в 30 точек: col.MinWidth() не вызывался в пакете ни разу.
// В узком окне колонка «Статус» ужималась до неразличимой наравне со всеми.

func widthGrid(t *testing.T, total int, cols ...Column) *DataGrid {
	t.Helper()
	dg := New()
	dg.RowHeight = 20
	dg.HeaderHeight = 20
	for _, c := range cols {
		dg.AddColumn(c)
	}
	dg.SetBounds(image.Rect(0, 0, total, 100))
	// Ширины считаются при отрисовке (layoutColumns под флагом dirty), а не
	// при SetBounds — рисуем пустым контекстом, чтобы раскладка состоялась.
	dg.Draw(&nopDrawCtx{})
	return dg
}

func starCol(header string, weight float64) *DataGridTextColumn {
	c := NewTextColumn(header, header)
	c.SetWidth(StarWidth(weight))
	return c
}

// Колонка не ужимается ниже собственного минимума.
func TestColumnWidth_MinIsHonoured(t *testing.T) {
	status := starCol("Статус", 1)
	status.SetMinWidth(120)
	name := starCol("Ресурс", 1)

	widthGrid(t, 200, status, name)

	if got := status.ActualWidth(); got < 120 {
		t.Errorf("«Статус» ужался до %d при минимуме 120", got)
	}
	// Остаток достался соседке — она отдаёт место, а не делит поровну.
	if got := name.ActualWidth(); got > 80 {
		t.Errorf("«Ресурс» занял %d, хотя соседка забрала свой минимум", got)
	}
}

// Колонка не растягивается выше собственного максимума, а остаток уходит
// соседям — иначе после неё осталась бы пустая полоса.
func TestColumnWidth_MaxIsHonouredAndRestGoesOn(t *testing.T) {
	narrow := starCol("Флаг", 1)
	narrow.SetMaxWidth(60)
	wide := starCol("Описание", 1)

	widthGrid(t, 400, narrow, wide)

	if got := narrow.ActualWidth(); got != 60 {
		t.Errorf("«Флаг» занял %d при максимуме 60", got)
	}
	if got := wide.ActualWidth(); got < 300 {
		t.Errorf("«Описание» заняло %d — остаток от соседки не перераспределён", got)
	}
	if sum := narrow.ActualWidth() + wide.ActualWidth(); sum > 400 {
		t.Errorf("колонки заняли %d при ширине таблицы 400", sum)
	}
}

// Границы соблюдаются и у колонок фиксированной ширины.
func TestColumnWidth_PixelColumnIsClamped(t *testing.T) {
	c := NewTextColumn("Тесная", "x")
	c.SetWidth(PixelWidth(300))
	c.SetMaxWidth(100)

	widthGrid(t, 500, c)

	if got := c.ActualWidth(); got != 100 {
		t.Errorf("колонка в 300 точек с максимумом 100 заняла %d", got)
	}
}

// Максимум сильнее общего минимума: «уже некуда» — это осознанное указание.
func TestColumnWidth_MaxBeatsGlobalMinimum(t *testing.T) {
	c := starCol("Узкая", 1)
	c.SetMaxWidth(12) // меньше общего minColumnWidth = 30

	widthGrid(t, 400, c)

	if got := c.ActualWidth(); got != 12 {
		t.Errorf("колонка заняла %d, хотя максимум 12", got)
	}
}

// Ресайз мышью тоже соблюдает границы: иначе минимум держался бы только до
// первого движения.
func TestColumnWidth_ResizeRespectsBounds(t *testing.T) {
	c := NewTextColumn("Статус", "x")
	c.SetWidth(PixelWidth(150))
	c.SetMinWidth(100)
	c.SetMaxWidth(200)
	other := NewTextColumn("Прочее", "y")
	other.SetWidth(PixelWidth(150))

	dg := widthGrid(t, 400, c, other)
	dg.CanUserResizeColumns = true

	// Тянем границу первой колонки далеко влево.
	dg.OnMouseButton(150-2, 5, 0, true)
	dg.OnMouseMove(10, 5)
	if got := c.ActualWidth(); got < 100 {
		t.Errorf("после сжатия мышью ширина %d, минимум 100", got)
	}

	// И далеко вправо.
	dg.OnMouseMove(900, 5)
	if got := c.ActualWidth(); got > 200 {
		t.Errorf("после растягивания мышью ширина %d, максимум 200", got)
	}
	dg.OnMouseButton(900, 5, 0, false)
}

// Без заданных границ поведение прежнее: доли делятся по весам.
func TestColumnWidth_UnboundedSplitsByWeight(t *testing.T) {
	one := starCol("Один", 1)
	two := starCol("Два", 3)

	widthGrid(t, 400, one, two)

	if got := one.ActualWidth(); got != 100 {
		t.Errorf("колонка веса 1 заняла %d, ждали 100", got)
	}
	if got := two.ActualWidth(); got != 300 {
		t.Errorf("колонка веса 3 заняла %d, ждали 300", got)
	}
}
