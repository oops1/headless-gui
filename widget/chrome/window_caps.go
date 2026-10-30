package chrome

import "github.com/oops1/headless-gui/v4/widget/base"

// window_caps.go — Window как контейнер со своей раскладкой (ChildLayouter)
// и его семантика для доступности (AccessDescriber): вместо веток по типу в
// ядре (ADR-0012, 8.5).

// LaysOutChildren — SetBounds окна сам перекладывает детей (ChildLayouter).
func (w *Window) LaysOutChildren() bool { return true }

// DescribeAccess — семантика окна (AccessDescriber): роль окна, имя —
// заголовок, неактивное окно — состояние.
func (w *Window) DescribeAccess(info *base.AccessInfo) {
	info.Role = base.RoleWindow
	info.Name = w.Title
	if !w.IsActive() {
		info.States = append(info.States, base.StateInactive)
	}
}

var (
	_ base.ChildLayouter   = (*Window)(nil)
	_ base.AccessDescriber = (*Window)(nil)
)

// InputBindingList — привязки клавиш окна (поле InputBindings): по ним
// загрузчик XAML связывает CommandPath с командами DataContext.
func (w *Window) InputBindingList() []base.InputBinding { return w.InputBindings }
