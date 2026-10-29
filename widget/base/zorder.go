package base

// zorder.go — перестановка детей по Z без пересоздания.
//
// Порядок детей и есть Z-порядок: позже в списке — выше на экране, раньше
// получает щелчок при обходе сверху вниз. Поднять окно по щелчку раньше
// значило RemoveChild + AddChild: два уведомления, сброс родителя у ребёнка и
// у Canvas — потеря его attached-свойств. Здесь ребёнок переезжает внутри
// списка, остаётся тем же ребёнком того же родителя, а заявляется ровно та
// площадь, что меняется на экране, — его поддерево с полями (DrawMargin):
// поднятое окно легло поверх соседей там, где лежит само, и больше нигде.
//
// Как и AddChild, вызывается на горутине движка (обработчик ввода, Post).

// ChildIndex возвращает позицию ребёнка w в списке детей, -1 — не ребёнок.
func (b *Base) ChildIndex(w Widget) int {
	for i, c := range b.children {
		if c == w {
			return i
		}
	}
	return -1
}

// RaiseChild поднимает ребёнка w над всеми соседями. false — w не ребёнок.
func (b *Base) RaiseChild(w Widget) bool {
	return b.moveChildTo(w, len(b.children)-1)
}

// LowerChild опускает ребёнка w под всех соседей. false — w не ребёнок.
func (b *Base) LowerChild(w Widget) bool {
	return b.moveChildTo(w, 0)
}

// InsertChildAt добавляет w на позицию i (0 — под всеми, len — над всеми,
// как AddChild); выходящее за границы i прижимается к ним.
func (b *Base) InsertChildAt(i int, w Widget) {
	i = clampChildIndex(i, len(b.children))
	b.children = append(b.children, nil)
	copy(b.children[i+1:], b.children[i:])
	b.children[i] = w
	AdoptChild(b, w)
	b.markChildAdded(w)
	bumpTreeGen()
	b.notifyOwnedRect(screenRect(b, SubtreeRect(w)))
}

// moveChildTo переставляет ребёнка w на позицию to.
func (b *Base) moveChildTo(w Widget, to int) bool {
	from := b.ChildIndex(w)
	if from < 0 {
		return false
	}
	if moveInSlice(b.children, from, to) {
		// Геометрия поддеревьев не изменилась, но кэш охватывающих
		// прямоугольников живёт по поколению дерева, и вычитание перекрытого
		// идёт по новому порядку: поколение поднимается, как при любой
		// перестройке.
		bumpTreeGen()
		b.drawDirty.Store(true)
		b.markUp()
		b.notifyOwnedRect(screenRect(b, SubtreeRect(w)))
	}
	return true
}

// moveInSlice переставляет элемент from на позицию to, сдвигая соседей.
// false — переставлять нечего.
func moveInSlice[T any](s []T, from, to int) bool {
	to = clampChildIndex(to, len(s)-1)
	if from == to {
		return false
	}
	v := s[from]
	if from < to {
		copy(s[from:to], s[from+1:to+1])
	} else {
		copy(s[to+1:from+1], s[to:from])
	}
	s[to] = v
	return true
}

func clampChildIndex(i, max int) int {
	if i < 0 {
		return 0
	}
	if i > max {
		return max
	}
	return i
}
