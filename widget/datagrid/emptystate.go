// emptystate.go — что видно, когда строк нет.
//
// При пустом источнике таблица рисовала только заголовки, и человек видел
// пустой прямоугольник без единого слова. Для таблиц, у которых пусто — это
// обычное первое состояние (учётные данные, ключи, история), объяснение нужнее
// всего именно там. Приложение обходилось своим Label поверх таблицы и
// переключало его видимость вручную.
package datagrid

import "image"

// drawEmptyState рисует объяснение поверх пустой области данных.
//
// Только когда строк НЕТ: непустая таблица объясняет себя сама. Заголовки при
// этом остаются на месте — по ним видно, что за таблица перед человеком, и
// прятать их значило бы прятать половину ответа.
func (dg *DataGrid) drawEmptyState(ctx DrawContextBridge, s *drawSnapshot) {
	if len(s.rows) > 0 || s.rowCount > 0 {
		return
	}
	dr := s.dataRect
	if dr.Empty() {
		return
	}

	if s.emptyDraw != nil {
		s.emptyDraw(EmptyStateContext{Rect: dr, DrawCtx: ctx, FontSize: s.fontSize})
		return
	}
	if s.emptyText == "" {
		return
	}

	// По центру области данных: пустой таблице нечего прижимать к краю, а
	// текст посередине читается как состояние, а не как первая строка.
	w := ctx.MeasureText(s.emptyText, s.fontSize)
	x := dr.Min.X + (dr.Dx()-w)/2
	y := dr.Min.Y + (dr.Dy()-int(s.fontSize*1.6))/2
	if x < dr.Min.X {
		x = dr.Min.X
	}
	ctx.DrawTextSize(s.emptyText, x, y, s.fontSize, s.emptyColor)
}

// EmptyStateContext — что известно отрисовщику пустого состояния.
type EmptyStateContext struct {
	// Rect — область данных (без заголовка и полосы прокрутки).
	Rect     image.Rectangle
	DrawCtx  DrawContextBridge
	FontSize float64
}
