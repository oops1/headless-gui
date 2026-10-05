// showdesktop.go — узкая полоска «Показать рабочий стол» у края панели.
package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ComponentTrayShowDesktop — имя компонента полоски в стилях темы.
const ComponentTrayShowDesktop = "tray.showdesktop"

// KeyTrayShowDesktopWidth — ширина полоски. Если тема её не задала (или задала
// неположительной), берётся четверть размера значка трея: полоска узкая по
// своей природе.
const KeyTrayShowDesktopWidth theme.Key = "tray.showdesktop.width"

// ShowDesktopButton — полоска во всю высоту панели у её края (кладётся ПОСЛЕДНИМ
// в область трея панели, чтобы встать к правому краю). Клик вызывает OnClick:
// оболочка сворачивает все окна или возвращает их обратно. Наведение
// подсвечивает полоску по стилю «tray.showdesktop»; подсказка — строка
// widget.Tr("ShowDesktop").
type ShowDesktopButton struct {
	widget.Base

	tm *theme.Manager

	// OnClick — колбэк клика (отпускание над полоской).
	OnClick func()

	hovered int32
	pressed int32
}

// NewShowDesktopButton создаёт полоску, оформляемую темой tm.
func NewShowDesktopButton(tm *theme.Manager) *ShowDesktopButton {
	return &ShowDesktopButton{tm: tm}
}

// GetToolTip перекрывает промоутнутый из widget.Base: строка берётся при каждом
// показе и следует за языком.
func (b *ShowDesktopButton) GetToolTip() string { return widget.Tr(StrShowDesktop) }

// width — ширина полоски в логических пикселях.
func (b *ShowDesktopButton) width() int {
	if b.tm != nil {
		if w := int(b.tm.GetMetric(KeyTrayShowDesktopWidth)); w > 0 {
			return w
		}
	}
	if w := trayIconSize(b.tm) / 4; w > 0 {
		return w
	}
	return 1
}

// PreferredSize — ширина из метрики, высота — вся доступная.
func (b *ShowDesktopButton) PreferredSize(avail image.Point) image.Point {
	return image.Point{X: b.width(), Y: avail.Y}
}

// OnMouseMove обновляет наведение.
func (b *ShowDesktopButton) OnMouseMove(x, y int) {
	trayHandleMove(&b.hovered, b.Bounds(), x, y, b.Invalidate)
}

// OnMouseButton реализует клик (отпускание над полоской).
func (b *ShowDesktopButton) OnMouseButton(e widget.MouseEvent) bool {
	return trayHandleClick(&b.pressed, b.Bounds(), e, b.OnClick, b.Invalidate)
}

// Draw рисует подложку по стилю состояния.
func (b *ShowDesktopButton) Draw(ctx widget.DrawContext) {
	r := b.Bounds()
	if r.Empty() {
		return
	}
	PaintStyle(ctx, r, trayStyle(b.tm, ComponentTrayShowDesktop, trayState(&b.hovered, &b.pressed)))
}
