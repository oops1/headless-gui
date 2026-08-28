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

var (
	// cullMu защищает damage кадра: движок ставит его перед отрисовкой,
	// обход читает во время неё.
	cullMu sync.RWMutex
	// drawDamage — области, которые движок собирается перерисовать.
	// Пустой список означает «рисуем всё» — так ведут себя полный кадр и
	// любой потребитель, не сообщивший damage.
	drawDamage []image.Rectangle

	// cullingOn — включён ли пропуск. По умолчанию да: контракт объявлен, а
	// приложение с нарушенным вернёт прежнее поведение одной строкой.
	cullingOn atomic.Bool

	// treeGen растёт при каждом изменении геометрии или состава дерева.
	// Охватывающие прямоугольники поддеревьев кэшируются по нему: пока
	// дерево не трогали, пересчитывать нечего.
	treeGen atomic.Uint64
)

func init() { cullingOn.Store(true) }

// SetSubtreeCulling включает или выключает пропуск поддеревьев.
func SetSubtreeCulling(v bool) { cullingOn.Store(v) }

// SubtreeCulling сообщает, включён ли пропуск.
func SubtreeCulling() bool { return cullingOn.Load() }

// SetDrawDamage сообщает области, которые будут перерисованы в этом кадре.
// Вызывается движком перед обходом; пустой список снимает ограничение.
func SetDrawDamage(rects []image.Rectangle) {
	cullMu.Lock()
	drawDamage = append(drawDamage[:0], rects...)
	cullMu.Unlock()
}

// ClearDrawDamage снимает ограничение: следующий обход пройдёт целиком.
func ClearDrawDamage() {
	cullMu.Lock()
	drawDamage = drawDamage[:0]
	cullMu.Unlock()
}

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
// Нельзя пропускать: виджет с открытым оверлеем (оверлей рисуется отдельным
// проходом, но его область в границы виджета не входит) и виджет с пустым
// поддеревом — пустой прямоугольник означает «не знаю, где это», а не
// «нигде».
func SkipSubtree(w Widget) bool {
	if !cullingOn.Load() {
		return false
	}
	cullMu.RLock()
	n := len(drawDamage)
	cullMu.RUnlock()
	if n == 0 {
		return false
	}
	if od, ok := w.(OverlayDrawer); ok && od.HasOverlay() {
		return false
	}

	r := SubtreeRect(w)
	if r.Empty() {
		return false
	}

	cullMu.RLock()
	defer cullMu.RUnlock()
	for _, d := range drawDamage {
		if r.Overlaps(d) {
			return false
		}
	}
	return true
}
