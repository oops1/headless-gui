// traylabel.go — надпись в трее панели задач.
//
// Не всё в трее — значок. Раскладку клавиатуры Windows показывает словом
// («РУС»), а не фигурой, и это читается мгновенно; тем же способом уместно
// показывать имя профиля, номер рабочего стола, что угодно короткое.
//
// До этого элемента положить в трей текст было нечем: значки сети, звука и
// питания рисуются фигурами, а своего элемента с надписью у трея не было —
// оболочка с готовыми данными о раскладке не могла их показать.
package desktop

import (
	"image"
	"sync"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ComponentTrayLabel — имя компонента для стилей темы.
const ComponentTrayLabel = "tray.label"

// KeyTrayLabelMinWidth — наименьшая ширина надписи.
//
// Нужна, чтобы соседние значки не прыгали, когда «РУС» сменяется на «ENG»:
// строки разной ширины, а место под них одно.
const KeyTrayLabelMinWidth theme.Key = "tray.label.width.min"

// TrayLabel — короткая надпись в трее.
type TrayLabel struct {
	widget.Base

	tm *theme.Manager

	// OnClick — щелчок по надписи. Оболочка вешает на него переключение
	// раскладки; nil — надпись некликабельна.
	OnClick func()

	mu   sync.RWMutex
	text string

	hovered int32
	pressed int32
}

// NewTrayLabel создаёт надпись, оформляемую темой tm.
func NewTrayLabel(tm *theme.Manager, text string) *TrayLabel {
	return &TrayLabel{tm: tm, text: text}
}

// Text возвращает текущую надпись.
func (l *TrayLabel) Text() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.text
}

// SetText меняет надпись.
//
// Зовётся из горутины оболочки — раскладка меняется по событию системы, а не
// в кадре, — поэтому текст под замком: рисует его горутина кадра.
func (l *TrayLabel) SetText(text string) {
	l.mu.Lock()
	changed := l.text != text
	l.text = text
	l.mu.Unlock()
	if changed {
		l.Invalidate()
	}
}

// PreferredSize — ширина по тексту, но не меньше tray.label.width.min:
// иначе соседние значки прыгали бы при каждой смене раскладки.
func (l *TrayLabel) PreferredSize(avail image.Point) image.Point {
	s := trayStyle(l.tm, ComponentTrayLabel, theme.StateNormal)
	text := l.Text()
	if text == "" {
		return image.Point{}
	}

	w := widget.MeasureUIText(text, fontSizeOf(s)) + 2*int(s.PadX)
	if min := l.minWidth(); w < min {
		w = min
	}
	if avail.X > 0 && w > avail.X {
		w = avail.X
	}
	return image.Pt(w, avail.Y)
}

func (l *TrayLabel) minWidth() int {
	if l.tm == nil {
		return 0
	}
	return int(l.tm.GetMetric(KeyTrayLabelMinWidth))
}

// OnMouseMove обновляет наведение — как у значков трея.
func (l *TrayLabel) OnMouseMove(x, y int) {
	trayHandleMove(&l.hovered, l.Bounds(), x, y, l.Invalidate)
}

// OnMouseButton — щелчок срабатывает на отпускании над надписью, как у всех
// элементов панели задач.
func (l *TrayLabel) OnMouseButton(e widget.MouseEvent) bool {
	if l.OnClick == nil {
		return false
	}
	return trayHandleClick(&l.pressed, l.Bounds(), e, l.OnClick, l.Invalidate)
}

func (l *TrayLabel) Draw(ctx widget.DrawContext) {
	b := l.Bounds()
	if b.Empty() {
		return
	}
	text := l.Text()
	if text == "" {
		return
	}
	s := trayStyle(l.tm, ComponentTrayLabel, trayState(&l.hovered, &l.pressed))
	PaintStyle(ctx, b, s)
	DrawTextCentered(ctx, b, text, s)
}
