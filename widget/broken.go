package widget

// broken.go — виджеты, которые сломались, и заглушка на их месте.
//
// Движок, поймавший панику в Draw (политика сбоев, docs/adr/0001), помечает
// виновника здесь. Дальше его не рисуют: на его месте заглушка, а остальной
// интерфейс живёт. Учёт общий на процесс, как и учёт рисующихся виджетов
// (drawtrace.go): сломанный виджет сломан, в каком бы движке его ни рисовали.
//
// Стоимость, пока сломанных нет, — та же атомарная проверка на ребёнка, что
// у учёта рисующихся: оба держат один счётчик медленного пути.

import (
	"image/color"
	"sync"
)

var (
	brokenMu sync.Mutex
	broken   map[Widget]struct{}
)

// MarkBroken помечает виджет сломанным: Draw у него больше не вызывается, на
// его месте рисуется заглушка. Повторная пометка ничего не меняет.
func MarkBroken(w Widget) {
	if w == nil {
		return
	}
	brokenMu.Lock()
	if broken == nil {
		broken = make(map[Widget]struct{})
	}
	if _, ok := broken[w]; !ok {
		broken[w] = struct{}{}
		drawSlowPath.Add(1)
	}
	brokenMu.Unlock()
	markWidgetDirty(w) // на месте его списка — заглушка
}

// ClearBroken снимает пометку: виджет снова рисуется сам. Для программы,
// которая починила его состояние.
func ClearBroken(w Widget) {
	brokenMu.Lock()
	if _, ok := broken[w]; ok {
		delete(broken, w)
		drawSlowPath.Add(-1)
	}
	brokenMu.Unlock()
	markWidgetDirty(w) // заглушка уступает место самому виджету
}

// ClearAllBroken снимает все пометки. Для тестов: учёт общий на процесс.
func ClearAllBroken() {
	brokenMu.Lock()
	drawSlowPath.Add(-int32(len(broken)))
	broken = nil
	brokenMu.Unlock()
}

// IsBroken сообщает, помечен ли виджет сломанным.
func IsBroken(w Widget) bool {
	if w == nil {
		return false
	}
	brokenMu.Lock()
	_, ok := broken[w]
	brokenMu.Unlock()
	return ok
}

// BrokenWidgets перечисляет сломанные виджеты. Порядок не определён.
func BrokenWidgets() []Widget {
	brokenMu.Lock()
	defer brokenMu.Unlock()
	if len(broken) == 0 {
		return nil
	}
	out := make([]Widget, 0, len(broken))
	for w := range broken {
		out = append(out, w)
	}
	return out
}

// brokenFill и brokenEdge — цвета заглушки: нейтральный серый с рамкой,
// заметный на любом фоне и не похожий ни на один виджет.
var (
	brokenFill = color.RGBA{R: 96, G: 96, B: 96, A: 255}
	brokenEdge = color.RGBA{R: 200, G: 60, B: 60, A: 255}
)

// drawBrokenPlaceholder рисует заглушку на месте сломанного виджета: только
// через обязательные методы DrawContext, чтобы заглушка не могла упасть там
// же, где упал виджет.
func drawBrokenPlaceholder(w Widget, ctx DrawContext) {
	r := w.Bounds()
	if r.Empty() {
		return
	}
	ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), brokenFill)
	ctx.DrawBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), brokenEdge)
	// Крест из горизонтальных и вертикальных отрезков — лесенкой, без AA:
	// заглушке важно быть узнаваемой, а не красивой.
	n := r.Dx()
	if r.Dy() < n {
		n = r.Dy()
	}
	for i := 0; i < n; i++ {
		ctx.SetPixel(r.Min.X+i, r.Min.Y+i, brokenEdge)
		ctx.SetPixel(r.Max.X-1-i, r.Min.Y+i, brokenEdge)
	}
}
