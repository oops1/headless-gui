// culling.go — пропуск поддеревьев, не пересекающихся с изменившейся областью.
//
// Кадр рисуется по damage: движок ставит ножницы, и лишние пиксели
// отбрасываются. Но обход дерева и вызовы Draw происходили всё равно — для
// рабочего стола это постоянные затраты, не зависящие от того, изменился один
// пиксель или весь экран. Тикающие часы на панели раз в минуту обходили всё
// дерево ради двух тайлов.
//
// Здесь обход учится пропускать целые ветки: если охватывающий прямоугольник
// поддерева не задевает ни одной изменившейся области, его Draw не зовут.
//
// Отсюда требование к виджетам, записанное в GUIDE: Draw не гарантирован
// каждый кадр. Виджет, который в Draw что-то считает и запоминает, сломается
// молча — потому и есть выключатель.
package widget

import (
	"image"
	"sync"
	"sync/atomic"
)

// CullScope — то, что контекст отрисовки знает о текущем кадре: какие области
// перерисовываются и разрешён ли пропуск.
//
// Спрашивается у КОНТЕКСТА, а не у пакета. Раньше это были переменные на
// процесс, и два движка в одном процессе (окно и вынесенный в своё окно
// попап — обычное дело) перетирали их друг другу: движок мог пропустить
// поддерево по чужому damage и отдать недорисованный кадр.
//
// Реализуется engine.Canvas. Контекст без этих методов ограничений не
// ставит — обход идёт целиком, как до появления пропуска.
type CullScope interface {
	// DrawDamage — области кадра в ЛОГИЧЕСКИХ координатах. Пустой список
	// означает «рисуем всё».
	DrawDamage() []image.Rectangle
	// SubtreeCullingEnabled — разрешён ли пропуск поддеревьев.
	SubtreeCullingEnabled() bool
}

var (
	// treeGen растёт при каждом изменении геометрии или состава дерева.
	// Охватывающие прямоугольники поддеревьев кэшируются по нему: пока
	// дерево не трогали, пересчитывать нечего. Счётчик общий на процесс
	// намеренно — он лишь помечает кэш устаревшим, и лишний пересчёт из-за
	// чужого дерева безвреден.
	treeGen atomic.Uint64
)

// bumpTreeGen помечает кэш охватывающих прямоугольников устаревшим.
// Зовётся при изменении границ и состава дерева.
func bumpTreeGen() { treeGen.Add(1) }

// subtreeCarrier — виджет, хранящий кэш прямоугольника своего поддерева.
//
// Интерфейс с приватным методом: его может реализовать только тип,
// встраивающий Base, — то есть любой виджет этого движка. Чужая реализация
// Widget кэша не имеет и просто не пропускается.
type subtreeCarrier interface {
	subtreeCache() *subtreeInfo
}

// subtreeInfo — запомненный прямоугольник поддерева и поколение дерева, на
// котором он посчитан.
type subtreeInfo struct {
	mu   sync.Mutex
	rect image.Rectangle
	gen  uint64
	set  bool
}

// SubtreeRect возвращает прямоугольник, накрывающий виджет и всё его
// поддерево, включая поля, которые виджет просит на отрисовку за своими
// границами (DrawMarginer).
//
// Результат кэшируется до ближайшего изменения дерева: обход указателей
// дёшев, но делать его дважды за кадр незачем.
func SubtreeRect(w Widget) image.Rectangle {
	gen := treeGen.Load()
	if c, ok := w.(subtreeCarrier); ok {
		info := c.subtreeCache()
		info.mu.Lock()
		if info.set && info.gen == gen {
			r := info.rect
			info.mu.Unlock()
			return r
		}
		info.mu.Unlock()

		r := computeSubtreeRect(w, 0)
		info.mu.Lock()
		info.rect, info.gen, info.set = r, gen, true
		info.mu.Unlock()
		return r
	}
	return computeSubtreeRect(w, 0)
}

// DrawMarginer — виджет, рисующий ЗА своими границами: мягкая тень, свечение,
// подсветка. Поле входит в прямоугольник поддерева, иначе пропуск обрежет
// то, что виджет рисует снаружи.
type DrawMarginer interface {
	DrawMargin() int
}

// computeSubtreeRect считает прямоугольник поддерева обходом.
func computeSubtreeRect(w Widget, depth int) image.Rectangle {
	if w == nil || depth > 64 {
		return image.Rectangle{}
	}
	r := w.Bounds()
	if m, ok := w.(DrawMarginer); ok {
		if d := m.DrawMargin(); d > 0 {
			r = r.Inset(-d)
		}
	}
	for _, child := range w.Children() {
		r = r.Union(computeSubtreeRect(child, depth+1))
	}
	return r
}

// SkipSubtree сообщает, можно ли не рисовать поддерево виджета в этом кадре.
//
// Решение принимается по damage ЭТОГО кадра, который знает контекст
// отрисовки: у каждого движка он свой.
//
// Нельзя пропускать: виджет с открытым оверлеем (оверлей рисуется отдельным
// проходом, но его область в границы виджета не входит) и виджет с пустым
// поддеревом — пустой прямоугольник означает «не знаю, где это», а не
// «нигде».
func SkipSubtree(ctx DrawContext, w Widget) bool {
	scope, ok := ctx.(CullScope)
	if !ok || !scope.SubtreeCullingEnabled() {
		return false
	}
	damage := scope.DrawDamage()
	if len(damage) == 0 {
		return false
	}
	if od, ok := w.(OverlayDrawer); ok && od.HasOverlay() {
		return false
	}

	r := SubtreeRect(w)
	if r.Empty() {
		return false
	}
	for _, d := range damage {
		if r.Overlaps(d) {
			return false
		}
	}
	return true
}
