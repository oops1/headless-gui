// docksplit.go — несколько панелей на одной стороне: вкладками или в столбик.
//
// До этого несколько панелей одной стороны показывались ТОЛЬКО вкладками, и
// разделить их по длине стороны было нечем — «Репозитории» над «Ветками» на
// макете заказчика собрать было невозможно.
//
// Здесь второй режим: панели делят сторону между собой, между ними — кромка,
// за которую их можно перетаскивать. Режим задаётся на сторону, а не на
// менеджер: слева удобен столбик, снизу — вкладки, и это разные решения.
package widget

import "image"

// DockStack — как показываются несколько панелей одной стороны.
type DockStack int

const (
	// DockStackTabs — вкладками: видна одна панель, остальные корешками.
	// Прежнее и умолчательное поведение.
	DockStackTabs DockStack = iota
	// DockStackSplit — в столбик: панели делят сторону, между ними кромка.
	DockStackSplit
)

// dockSplitMinPane — наименьшая доля стороны, до которой можно сжать панель
// в режиме столбика.
//
// Доля, а не пиксели: сторона бывает и в двести точек, и во всю высоту
// экрана, и «шестьдесят точек» означало бы в этих случаях совсем разное.
const dockSplitMinRatio = 0.1

// SideStack сообщает, как показываются панели стороны.
func (m *DockManager) SideStack(s DockSide) DockStack {
	if !validSide(s) {
		return DockStackTabs
	}
	return m.stacks[int(s)]
}

// SetSideStack задаёт режим показа нескольких панелей стороны.
func (m *DockManager) SetSideStack(s DockSide, mode DockStack) {
	if !validSide(s) || m.stacks[int(s)] == mode {
		return
	}
	m.stacks[int(s)] = mode
	m.layout()
	m.Invalidate()
}

// SplitSide переводит сторону в столбик и задаёт долю ПЕРВОЙ панели.
//
// Названо так, как просил заказчик. Доля первой, а не всех: разделить надвое —
// обычный случай, и требовать список долей ради него незачем. Остальные
// панели делят остаток поровну.
func (m *DockManager) SplitSide(s DockSide, ratio float64) {
	if !validSide(s) {
		return
	}
	m.stacks[int(s)] = DockStackSplit
	docked := m.dockedPanes(s)
	if len(docked) < 2 {
		m.layout()
		m.Invalidate()
		return
	}
	ratio = clampRatio(ratio, 1-float64(len(docked)-1)*dockSplitMinRatio)

	rest := (1 - ratio) / float64(len(docked)-1)
	ratios := make([]float64, len(docked))
	ratios[0] = ratio
	for i := 1; i < len(ratios); i++ {
		ratios[i] = rest
	}
	m.splitRatios[int(s)] = ratios
	m.layout()
	m.Invalidate()
}

// clampRatio держит долю в разумных пределах.
func clampRatio(v, max float64) float64 {
	if v < dockSplitMinRatio {
		return dockSplitMinRatio
	}
	if max < dockSplitMinRatio {
		max = dockSplitMinRatio
	}
	if v > max {
		return max
	}
	return v
}

// sideRatios возвращает доли панелей стороны, дополняя их до нужной длины.
//
// Дополняет, а не пересоздаёт: панель могли добавить или закрыть уже после
// того, как пользователь развёл кромку, и терять его настройку из-за этого
// не надо.
func (m *DockManager) sideRatios(s DockSide, n int) []float64 {
	if n <= 0 {
		return nil
	}
	cur := m.splitRatios[int(s)]
	out := make([]float64, n)
	copy(out, cur)

	var known float64
	missing := 0
	for i, v := range out {
		if v <= 0 {
			missing++
			out[i] = 0
			continue
		}
		known += v
	}
	if missing > 0 {
		share := (1 - known) / float64(missing)
		if share < dockSplitMinRatio {
			share = dockSplitMinRatio
		}
		for i := range out {
			if out[i] == 0 {
				out[i] = share
			}
		}
	}

	// Нормируем: доли могли не сойтись после добавления или закрытия панели.
	var sum float64
	for _, v := range out {
		sum += v
	}
	if sum <= 0 {
		for i := range out {
			out[i] = 1 / float64(n)
		}
		return out
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

// layoutSideSplit раскладывает панели стороны в столбик и возвращает кромки
// между ними.
//
// Кромки нужны наружу: за них тянут мышью, и хит-тест ищет их в том же
// списке, что и кромки сторон.
func (m *DockManager) layoutSideSplit(s DockSide, region image.Rectangle, docked []*DockPane) []image.Rectangle {
	ratios := m.sideRatios(s, len(docked))
	m.splitRatios[int(s)] = ratios

	gut := m.gutterSize()
	// Панели делят сторону вдоль ДЛИННОЙ оси: у левой и правой стороны это
	// высота, у верхней и нижней — ширина. Иначе столбик получился бы
	// поперёк, и панели сплющило бы в вертикальные полоски.
	vertical := s == DockLeft || s == DockRight

	total := region.Dy()
	if !vertical {
		total = region.Dx()
	}
	free := total - gut*(len(docked)-1)
	if free <= 0 {
		// Место кончилось: раскладывать нечего, отдаём всё первой панели —
		// пустой экран хуже, чем одна видимая панель.
		setDockChildBounds(docked[0], region)
		for _, p := range docked[1:] {
			setDockChildBounds(p, image.Rectangle{})
		}
		return nil
	}

	var gutters []image.Rectangle
	pos := region.Min.Y
	if !vertical {
		pos = region.Min.X
	}
	for i, p := range docked {
		size := int(float64(free) * ratios[i])
		if i == len(docked)-1 {
			// Последней — весь остаток: округление долей иначе оставляло бы
			// незакрашенную полоску у края.
			end := region.Max.Y
			if !vertical {
				end = region.Max.X
			}
			size = end - pos
		}
		if vertical {
			setDockChildBounds(p, image.Rect(region.Min.X, pos, region.Max.X, pos+size))
		} else {
			setDockChildBounds(p, image.Rect(pos, region.Min.Y, pos+size, region.Max.Y))
		}
		pos += size
		if i < len(docked)-1 {
			if vertical {
				gutters = append(gutters, image.Rect(region.Min.X, pos, region.Max.X, pos+gut))
			} else {
				gutters = append(gutters, image.Rect(pos, region.Min.Y, pos+gut, region.Max.Y))
			}
			pos += gut
		}
	}
	return gutters
}

// SplitGutterAt ищет кромку между панелями столбика под точкой.
//
// Возвращает сторону и номер кромки (i означает «между панелями i и i+1»).
func (m *DockManager) SplitGutterAt(x, y int) (DockSide, int, bool) {
	pt := image.Pt(x, y)
	for s := 0; s < 4; s++ {
		for i, g := range m.splitGutters[s] {
			if pt.In(g) {
				return DockSide(s), i, true
			}
		}
	}
	return DockLeft, 0, false
}

// DragSplitGutter двигает кромку i стороны s в точку (x, y).
//
// Соседние панели меняют доли: одна растёт, другая уменьшается, сумма долей
// стороны не меняется. Ни одна не сжимается ниже dockSplitMinRatio — иначе
// панель можно было бы схлопнуть в ноль и уже не найти.
func (m *DockManager) DragSplitGutter(s DockSide, i, x, y int) {
	if !validSide(s) {
		return
	}
	region := m.regions[int(s)]
	if region.Empty() {
		return
	}
	docked := m.dockedPanes(s)
	if i < 0 || i+1 >= len(docked) {
		return
	}
	ratios := m.sideRatios(s, len(docked))

	vertical := s == DockLeft || s == DockRight
	total := region.Dy()
	pos := y - region.Min.Y
	if !vertical {
		total = region.Dx()
		pos = x - region.Min.X
	}
	free := total - m.gutterSize()*(len(docked)-1)
	if free <= 0 {
		return
	}

	// Доля до кромки — это сумма долей панелей левее (выше) неё.
	var before float64
	for k := 0; k <= i; k++ {
		before += ratios[k]
	}
	want := float64(pos) / float64(free)
	delta := want - before

	pair := ratios[i] + ratios[i+1]
	first := clampRatio(ratios[i]+delta, pair-dockSplitMinRatio)
	ratios[i] = first
	ratios[i+1] = pair - first

	m.splitRatios[int(s)] = ratios
	m.layout()
	m.Invalidate()
}
