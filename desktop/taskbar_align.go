// taskbar_align.go — выравнивание группы «пуск + приложения» на панели и высота
// кнопок панели (Windows 11).
//
// Windows 11 ставит «Пуск», поиск, Task View и кнопки окон ОДНОЙ группой по
// центру панели, а пользователь вправе прижать её влево. Группа — это слоты
// SlotStart и SlotApps вместе: потребитель кладёт «Пуск», поиск и Task View в
// SlotStart, кнопки окон — в SlotApps, и они едут вместе. Слот SlotWidgets
// (кнопка виджетов) от выравнивания не зависит и стоит у левого края.
//
// Выравнивание меняется на лету, ничего не пересоздавая: панель переложит
// элементы и перерисует свою полосу. Два способа, оба живые:
//
//   - флаг темы taskbar.centered через theme.Manager.SetFlag — панель
//     подписана на менеджер, и смена флага перекладывает её сама;
//   - Taskbar.SetAlignment — назначение мимо темы (как SetEdge для края): не
//     зависит от профиля и переживает смену темы. ResetAlignment отдаёт решение
//     обратно флагу.
package desktop

import "github.com/oops1/headless-gui/v3/theme"

// BarAlign — как группа «пуск + приложения» стоит на панели.
type BarAlign int

const (
	// BarAlignLeft — группа прижата к левому краю (Windows 10, классические).
	BarAlignLeft BarAlign = iota
	// BarAlignCenter — группа по центру панели (Windows 11).
	BarAlignCenter
)

// KeyTaskbarItemHeight — высота кнопок панели (Пуск, поиск, Task View, виджеты,
// подсветка трея): по центру высоты панели. 0 — во всю высоту, как раньше;
// Windows 11 объявляет 40 в панели 48.
const KeyTaskbarItemHeight theme.Key = "taskbar.item.height"

// Alignment возвращает действующее выравнивание: назначенное явно, а без
// назначения — выведенное из флага темы taskbar.centered.
func (t *Taskbar) Alignment() BarAlign {
	if t.centeredGroup() {
		return BarAlignCenter
	}
	return BarAlignLeft
}

// SetAlignment назначает выравнивание явно, мимо темы. Панель перекладывает
// элементы и перерисовывает свою полосу; повтор того же значения ничего не
// делает. Боковая панель (столбец) выравнивание не использует: группа там всегда
// сверху, как и раньше.
func (t *Taskbar) SetAlignment(a BarAlign) {
	if t.alignSet && t.align == a {
		return
	}
	t.align, t.alignSet = a, true
	t.alignmentChanged()
}

// ResetAlignment возвращает выравнивание, выводимое из флага taskbar.centered.
func (t *Taskbar) ResetAlignment() {
	if !t.alignSet {
		return
	}
	t.alignSet = false
	t.alignmentChanged()
}

func (t *Taskbar) alignmentChanged() {
	t.relayout()
	t.Invalidate()
}

// centeredGroup — стоит ли группа по центру: решение SetAlignment, а без него
// флаг темы.
func (t *Taskbar) centeredGroup() bool {
	if t.alignSet {
		return t.align == BarAlignCenter
	}
	return t.flag(KeyTaskbarCentered)
}
