package native

// drag.go — действие перетаскивания DragEffect и его выбор по клавишам.

// DragEffect — что можно сделать с перетаскиваемыми файлами. Битовая маска:
// источник объявляет допустимые действия, приёмник выбирает одно из них.
// Значения совпадают с DROPEFFECT_* Windows.
type DragEffect uint32

const (
	// DragEffectCopy — скопировать файлы.
	DragEffectCopy DragEffect = 1
	// DragEffectMove — переместить файлы.
	DragEffectMove DragEffect = 2
	// DragEffectLink — создать ссылку на файлы.
	DragEffectLink DragEffect = 4

	// DragEffectAll — все известные биты.
	//
	//facadegen:skip
	DragEffectAll = DragEffectCopy | DragEffectMove | DragEffectLink
)

// String — читаемое имя маски для журналов и тестов: «copy|move», «none».
func (e DragEffect) String() string {
	if e&DragEffectAll == 0 {
		return "none"
	}
	s := ""
	add := func(bit DragEffect, name string) {
		if e&bit != 0 {
			if s != "" {
				s += "|"
			}
			s += name
		}
	}
	add(DragEffectCopy, "copy")
	add(DragEffectMove, "move")
	add(DragEffectLink, "link")
	return s
}

// DragEffectForKeys — действие по зажатым модификаторам, как в Проводнике
// Windows: Ctrl — копировать, Shift — переместить, Ctrl+Shift (или Alt) —
// ссылка; без клавиш — копировать, а если копирование недопустимо, то
// первое допустимое. Чистая функция: проверяется без ОС.
//
//facadegen:skip
func DragEffectForKeys(allowed DragEffect, ctrl, shift, alt bool) DragEffect {
	var prefer DragEffect
	switch {
	case ctrl && shift, alt:
		prefer = DragEffectLink
	case ctrl:
		prefer = DragEffectCopy
	case shift:
		prefer = DragEffectMove
	}
	return PickDragEffect(allowed, prefer)
}

// PickDragEffect выбирает действие из допустимых источником (allowed) с учётом
// пожеланий приёмника (prefer, может быть 0). Предпочтение выигрывает, если оно
// допустимо; иначе по убыванию «безопасности»: copy, move, link. 0 — выбрать
// нечего.
//
//facadegen:skip
func PickDragEffect(allowed, prefer DragEffect) DragEffect {
	allowed &= DragEffectAll
	if p := prefer & allowed; p != 0 {
		for _, e := range []DragEffect{DragEffectCopy, DragEffectMove, DragEffectLink} {
			if p&e != 0 {
				return e
			}
		}
	}
	for _, e := range []DragEffect{DragEffectCopy, DragEffectMove, DragEffectLink} {
		if allowed&e != 0 {
			return e
		}
	}
	return 0
}
