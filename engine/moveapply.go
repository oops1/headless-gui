// moveapply.go — применение объявленных переносов к переднему буферу.
//
// Перетаскивание окна не меняет пиксели, а переносит их. Виджет объявляет
// это (widget.NotifyMove), движок кладёт объявление в кадр, и потребитель
// выполняет копирование вместо приёма картинки заново.
//
// Но одного объявления мало. Дифф сравнивает НОВЫЙ задний буфер со СТАРЫМ
// передним: на старом месте окна ещё нет, на новом — уже есть, и оба места
// уезжают тайлами. Потребитель получал и команду переноса, и все пиксели —
// то есть платил ровно столько же, сколько без переноса.
//
// Поэтому перенос применяется к переднему буферу ДО диффа: передний буфер
// приводится в то состояние, в котором окажется буфер потребителя после
// копирования. Дальше дифф сравнивает уже с ним и оставляет только то, что
// действительно отличается, — кайму, тень, изменившееся содержимое.
package engine

import (
	"image"

	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// applyMovesToFront выполняет переносы внутри переднего буфера.
//
// Координаты объявлений — ЛОГИЧЕСКИЕ (виджеты живут в них), буфер —
// физический, поэтому каждая область масштабируется. Перенос, выходящий за
// холст, обрезается по нему: копировать из-за края нечего.
//
// Возвращает применённые переносы в логических координатах — в кадр уходят
// только они. Отброшенное объявление в кадре было бы ложью: потребитель
// скопировал бы то, чего движок у себя не копировал.
func (c *Canvas) applyMovesToFront(moves []widgetMove) []output.MoveRegion {
	if len(moves) == 0 || c.front == nil {
		return nil
	}
	out := make([]output.MoveRegion, 0, len(moves))
	bounds := image.Rect(0, 0, c.W, c.H)
	// taken — области, уже занятые применёнными переносами (и источники, и
	// назначения) в физических координатах. См. проверку пересечения ниже.
	taken := make([]image.Rectangle, 0, 2*len(moves))

	for _, m := range moves {
		dst := c.sRect(m.Rect).Intersect(bounds)
		if dst.Empty() {
			continue
		}
		// Источник сдвигается ровно на ту же величину, что и назначение:
		// перенос не меняет размера, иначе это уже масштабирование.
		delta := dst.Min.Sub(c.sRect(m.Rect).Min)
		src := c.sRect(image.Rect(m.From.X, m.From.Y,
			m.From.X+m.Rect.Dx(), m.From.Y+m.Rect.Dy())).Add(delta)
		src = src.Intersect(bounds)
		if src.Empty() || src.Dx() != dst.Dx() || src.Dy() != dst.Dy() {
			// Обрезка развела источник и назначение — переносить нечего:
			// пусть область уедет обычными тайлами.
			continue
		}
		if src.Min == dst.Min {
			continue
		}
		// Переносы, задевающие уже перенесённое, дальше не идут.
		//
		// Движок применяет их к своему буферу подряд, поэтому второй читает
		// пиксели, которые первый уже переложил. Потребитель обязан повторить
		// ровно тот же порядок — а он этого не обязан уметь: команда копии
		// поверхности в RDP выполняется когда угодно относительно соседних, и
		// локальный потребитель вправе применить их разом. Разошедшийся
		// порядок даёт копию с неверным источником — прямоугольник чужого
		// содержимого с резким швом.
		//
		// Поэтому в кадр уходят только непересекающиеся переносы: их можно
		// применять в любом порядке и получить один и тот же результат.
		// Остальное уезжает обычными тайлами — дороже, но верно.
		if overlapsAny(taken, src) || overlapsAny(taken, dst) {
			continue
		}
		blitWithin(c.front, src, dst.Min)
		taken = append(taken, src, dst)
		out = append(out, output.MoveRegion{From: m.From, Rect: m.Rect})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// overlapsAny сообщает, задевает ли r хоть одну из областей.
func overlapsAny(rects []image.Rectangle, r image.Rectangle) bool {
	for _, o := range rects {
		if r.Overlaps(o) {
			return true
		}
	}
	return false
}

// widgetMove — объявление переноса в логических координатах (копия
// widget.MoveNotice, чтобы engine не зависел от его формы).
type widgetMove struct {
	From image.Point
	Rect image.Rectangle
}

// noteMove принимает объявление от дерева виджетов. Регистрируется в New.
//
// Приёмник получает объявления ВСЕГО процесса, поэтому чужие надо отсеять:
// движок берёт только то, что попадает на его холст. Иначе перенос из
// соседнего движка заставил бы этот скопировать у себя пиксели с места, где
// ничего не переезжало, — и потребитель увидел бы смазанный след.
func (e *Engine) noteMove(n widget.MoveNotice) {
	if n.Rect.Empty() {
		return
	}
	e.mu.RLock()
	canvas := e.canvas
	e.mu.RUnlock()
	if canvas == nil {
		return
	}
	// Границы холста в логических координатах — тех же, в которых живут
	// виджеты и приходит объявление.
	screen := image.Rect(0, 0, canvas.logicalW, canvas.logicalH)
	src := image.Rect(n.From.X, n.From.Y, n.From.X+n.Rect.Dx(), n.From.Y+n.Rect.Dy())
	if !n.Rect.Overlaps(screen) && !src.Overlaps(screen) {
		return
	}

	e.moveMu.Lock()
	e.pendingMoves = append(e.pendingMoves, widgetMove{From: n.From, Rect: n.Rect})
	e.moveMu.Unlock()
}

// takeMoves забирает накопленные объявления и очищает список.
func (e *Engine) takeMoves() []widgetMove {
	e.moveMu.Lock()
	out := e.pendingMoves
	e.pendingMoves = nil
	e.moveMu.Unlock()
	return out
}

// blitWithin копирует прямоугольник внутри одного изображения.
//
// Строки идут в порядке, зависящем от направления сдвига: при перекрытии
// областей копирование сверху вниз затёрло бы ещё не прочитанные строки —
// та же осторожность, что у memmove.
func blitWithin(img *image.RGBA, src image.Rectangle, dstMin image.Point) {
	w := src.Dx() * 4
	if w <= 0 {
		return
	}
	rows := src.Dy()
	down := dstMin.Y > src.Min.Y

	for i := 0; i < rows; i++ {
		y := i
		if down {
			y = rows - 1 - i // сверху вниз: идём с конца, чтобы не затереть
		}
		so := img.PixOffset(src.Min.X, src.Min.Y+y)
		do := img.PixOffset(dstMin.X, dstMin.Y+y)
		copy(img.Pix[do:do+w], img.Pix[so:so+w])
	}
}
