// taskview.go — кнопка Task View (Windows 11): обзор окон.
//
// Что кнопка открывает, решает потребитель: в Windows это виртуальные рабочие
// столы, у оболочки без столов — обзор окон. Компонент только значок с плашкой,
// состояниями (наведение, нажатие, «обзор открыт»), рамкой фокуса и колбэком
// OnClick; рисунок значка — иконка темы taskview.icon, а без неё встроенный
// векторный глиф (два окна).
package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ComponentTaskView — имя компонента кнопки Task View в стилях темы.
const ComponentTaskView = "taskview"

const (
	// KeyTaskViewIconSize — сторона значка кнопки (логические пиксели).
	KeyTaskViewIconSize theme.Key = "taskview.icon.size"
	// KeyTaskViewIcon — иконка кнопки в наборе иконок темы; без неё рисуется
	// встроенный глиф.
	KeyTaskViewIcon theme.Key = "taskview.icon"
)

// TaskViewButton — кнопка Task View. Кладётся в SlotStart после поиска, чтобы
// стоять в центрированной группе Windows 11.
//
//	tv := desktop.NewTaskViewButton(tm)
//	tv.OnClick = func() { overview.Toggle() }
//	tv.TrackManager(flyouts, "overview")   // горит, пока обзор открыт
//	bar.AddItem(desktop.SlotStart, tv)
type TaskViewButton struct {
	iconButton

	// Icon — своя картинка вместо значка темы; IconAt — источник по размеру в
	// физических пикселях (как AppInfo.IconAt). Побеждают и иконку темы, и
	// встроенный глиф.
	Icon   image.Image
	IconAt func(size int) image.Image

	glyph glyphMemo
}

var (
	_ Item             = (*TaskViewButton)(nil)
	_ widget.Focusable = (*TaskViewButton)(nil)
	_ FocusNavigable   = (*TaskViewButton)(nil)
	_ FocusRinger      = (*TaskViewButton)(nil)
)

// NewTaskViewButton создаёт кнопку Task View, оформляемую темой tm.
func NewTaskViewButton(tm *theme.Manager) *TaskViewButton {
	b := &TaskViewButton{}
	b.init(tm, ComponentTaskView)
	return b
}

// PreferredSize — значок плюс отступы стиля; высоту решает панель (taskbar.item.height).
func (b *TaskViewButton) PreferredSize(avail image.Point) image.Point {
	st := styleOf(b.tm, ComponentTaskView, "", theme.StateNormal)
	w := b.metric(KeyTaskViewIconSize) + 2*int(st.PadX)
	if w < 0 {
		w = 0
	}
	if avail.X > 0 && w > avail.X {
		w = avail.X
	}
	return image.Pt(w, 0)
}

// GetToolTip — «Представление задач»: строка берётся при каждом показе, поэтому
// следует за языком.
func (b *TaskViewButton) GetToolTip() string { return tr(StrTaskView) }

// OnKeyEvent: Enter и Space — как щелчок, стрелки — к соседу по области.
func (b *TaskViewButton) OnKeyEvent(e widget.KeyEvent) {
	b.HandleKey(b, e, click(b.OnClick), b.Invalidate)
}

// Draw рисует плашку и значок.
func (b *TaskViewButton) Draw(ctx widget.DrawContext) {
	r, s := b.paintPlate(ctx)
	if s == nil {
		return
	}
	drawIconIn(ctx, b.tm, r, b.metric(KeyTaskViewIconSize), s, b.Icon, b.IconAt,
		[]theme.Key{KeyTaskViewIcon}, glyphTaskView, &b.glyph)
}
