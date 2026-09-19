// stretch.go — распорка: пустой элемент, забирающий свободное место в
// StackPanel и ToolBar (GG-84).
//
// Панель инструментов клиента git повторяет SmartGit: левая группа кнопок
// прижата к левому краю, правая — к правому, середина пуста. Собрать такое
// было нечем: обе панели ставят детей подряд по их собственной ширине,
// HAlign они не смотрят, а звёздочные доли есть только у Grid. Приложению
// оставалось считать остаток самому и подсовывать виджет нужной ширины.
package widget

// Stretch — распорка. Сама ничего не рисует и не принимает ввод: её работа —
// занять место, которое осталось от прочих элементов по оси раскладки.
//
//	tb.AddStretch()                 // в разметке: <ToolBarStretch/> или <Stretch/>
//	sp.AddStretch()
//
// Несколько распорок делят остаток по весам (Weight): по умолчанию поровну.
// Места не осталось — распорка получает нулевой размер и просто не видна.
type Stretch struct {
	Base

	// Weight — доля остатка. 0 и меньше считается единицей, так что
	// <Stretch/> и <Stretch Weight="1"/> — одно и то же.
	Weight int
}

// NewStretch создаёт распорку.
func NewStretch() *Stretch { return &Stretch{} }

// NewStretchWeighted создаёт распорку с заданным весом.
func NewStretchWeighted(weight int) *Stretch { return &Stretch{Weight: weight} }

// weight — вес распорки, не меньше единицы.
func (s *Stretch) weight() int {
	if s.Weight > 0 {
		return s.Weight
	}
	return 1
}

// Draw ничего не рисует: распорка — это пустое место.
func (s *Stretch) Draw(ctx DrawContext) {}

// spreadStretch раздаёт free между распорками с весами weights.
//
// Остаток от деления достаётся последней: иначе сумма долей не сходится с
// остатком, и панель из двух распорок оставляла бы у правого края щель в
// пару точек. Отрицательный остаток (места не хватило) — все нули.
func spreadStretch(free int, weights []int) []int {
	out := make([]int, len(weights))
	if len(weights) == 0 || free <= 0 {
		return out
	}
	total := 0
	for _, w := range weights {
		total += w
	}
	if total <= 0 {
		return out
	}
	given := 0
	for i, w := range weights {
		if i == len(weights)-1 {
			out[i] = free - given
			break
		}
		part := free * w / total
		out[i] = part
		given += part
	}
	return out
}

// stretchIn собирает распорки среди детей: их индексы и веса.
func stretchIn(children []Widget) (idx []int, weights []int) {
	for i, c := range children {
		if s, ok := c.(*Stretch); ok {
			idx = append(idx, i)
			weights = append(weights, s.weight())
		}
	}
	return idx, weights
}

// buildXAMLStretch строит распорку из <Stretch/> или <ToolBarStretch/>.
func buildXAMLStretch(el xElement) Widget {
	s := NewStretch()
	if w := xatoi(el.attr("Weight")); w > 0 {
		s.Weight = w
	}
	return s
}
