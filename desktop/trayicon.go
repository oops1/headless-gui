// trayicon.go — обобщённый значок трея: картинка или SVG с подсказкой и кликом.
//
// Значки состояния (сеть, звук, питание) знают свои данные сами. Всё прочее —
// значок приложения в трее, индикатор раскладки, облачная синхронизация —
// потребитель описывает картинкой: достаточно image.Image, SVG-данных или
// функции, отдающей картинку нужного размера, плюс подсказка и колбэк клика.
package desktop

import (
	"image"
	"sync"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/svg"
)

// ComponentTrayIcon — имя компонента обобщённого значка трея в стилях темы.
const ComponentTrayIcon = "tray.icon"

// TrayIcon — значок в трее панели задач из произвольного источника.
//
// Источник один из трёх, последний заданный побеждает:
//   - SetSVG — векторный значок: растеризуется в ФИЗИЧЕСКОМ размере
//     (логический × масштаб), поэтому на 125–200 % остаётся чётким;
//   - SetImageSource — функция «размер в физических пикселях → картинка»: для
//     значков, у которых есть растры под несколько размеров (см. AppInfo.IconAt);
//   - SetImage — одна готовая картинка (растягивается движком).
//
// Размер значка — метрика темы tray.icon.size, как у остальных значков трея.
// Цвета и отступы — стиль «tray.icon». currentColor в SVG — цвет текста стиля;
// при флаге темы tray.icon.tint весь значок перекрашивается в него.
type TrayIcon struct {
	widget.Base

	tm *theme.Manager

	// OnClick — колбэк клика (отпускание над значком).
	OnClick func()

	hovered int32
	pressed int32

	tt    trayTooltip
	ttKey atomic.Value // string: ключ widget.Tr подсказки; пусто — берётся tt

	mu   sync.Mutex
	img  image.Image
	src  func(size int) image.Image
	doc  *svg.Document
	memo glyphMemo
}

// NewTrayIcon создаёт пустой значок трея, оформляемый темой tm.
func NewTrayIcon(tm *theme.Manager) *TrayIcon {
	return &TrayIcon{tm: tm}
}

// NewTrayImageIcon создаёт значок из готовой картинки.
func NewTrayImageIcon(tm *theme.Manager, img image.Image) *TrayIcon {
	t := NewTrayIcon(tm)
	t.SetImage(img)
	return t
}

// NewTraySVGIcon создаёт значок из SVG-данных. Ошибка разбора возвращается
// вместе с (пустым) значком: оболочка вправе показать его и так.
func NewTraySVGIcon(tm *theme.Manager, data []byte) (*TrayIcon, error) {
	t := NewTrayIcon(tm)
	return t, t.SetSVG(data)
}

// SetImage задаёт значок готовой картинкой (nil убирает значок).
func (t *TrayIcon) SetImage(img image.Image) {
	t.mu.Lock()
	t.img, t.src, t.doc = img, nil, nil
	t.mu.Unlock()
	t.Invalidate()
}

// SetImageSource задаёт значок функцией: она получает сторону квадрата в
// ФИЗИЧЕСКИХ пикселях и отдаёт картинку подходящего размера (nil — значок не
// рисуется). Вызывается из Draw на каждый кадр, поэтому обязана быть быстрой
// (кэшировать по размеру).
func (t *TrayIcon) SetImageSource(src func(size int) image.Image) {
	t.mu.Lock()
	t.img, t.src, t.doc = nil, src, nil
	t.mu.Unlock()
	t.Invalidate()
}

// SetSVG задаёт значок SVG-данными. При ошибке разбора прежний значок
// сохраняется, ошибка возвращается.
func (t *TrayIcon) SetSVG(data []byte) error {
	doc, err := svg.Parse(data)
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.img, t.src, t.doc = nil, nil, doc
	t.mu.Unlock()
	t.Invalidate()
	return nil
}

// SetToolTip задаёт подсказку готовой строкой.
func (t *TrayIcon) SetToolTip(s string) {
	t.ttKey.Store("")
	t.tt.set(s)
}

// SetToolTipKey задаёт подсказку ключом перевода (widget.Tr): строка берётся при
// каждом показе, поэтому следует за сменой языка без пересоздания значка.
func (t *TrayIcon) SetToolTipKey(key string) { t.ttKey.Store(key) }

// GetToolTip перекрывает промоутнутый из widget.Base — см. trayTooltip.
func (t *TrayIcon) GetToolTip() string {
	if k, _ := t.ttKey.Load().(string); k != "" {
		return widget.Tr(k)
	}
	return t.tt.get()
}

// style читает стиль значка из темы (пустой стиль, если темы нет).
func (t *TrayIcon) style(st theme.State) *theme.Style {
	return trayStyle(t.tm, ComponentTrayIcon, st)
}

// fillsStrip — подсветка на всю высоту полосы, если тема просит.
func (t *TrayIcon) fillsStrip() bool { return trayFillsStrip(t.tm) }

// PreferredSize — квадрат стороной из темы плюс отступы стиля по бокам.
func (t *TrayIcon) PreferredSize(image.Point) image.Point {
	size := trayIconSize(t.tm)
	return image.Point{X: size + 2*int(t.style(theme.StateNormal).PadX), Y: size}
}

// OnMouseMove обновляет наведение.
func (t *TrayIcon) OnMouseMove(x, y int) {
	trayHandleMove(&t.hovered, t.Bounds(), x, y, t.Invalidate)
}

// OnMouseButton реализует клик (отпускание над значком).
func (t *TrayIcon) OnMouseButton(e widget.MouseEvent) bool {
	return trayHandleClick(&t.pressed, t.Bounds(), e, t.OnClick, t.Invalidate)
}

// Draw рисует подложку по стилю и значок по центру.
func (t *TrayIcon) Draw(ctx widget.DrawContext) {
	b := t.Bounds()
	if b.Empty() {
		return
	}
	st := trayState(&t.hovered, &t.pressed)
	s := t.style(st)
	PaintStyle(ctx, b, s)

	box := glyphSquare(shrinkByPad(b, s))
	if box.Empty() {
		return
	}
	tint := t.tm != nil && t.tm.GetFlag(KeyTrayIconTint, false)

	t.mu.Lock()
	img, src, doc := t.img, t.src, t.doc
	t.mu.Unlock()

	switch {
	case doc != nil:
		widget.DrawSVG(ctx, doc, box, ink(s), tint)
	case src != nil:
		p := widget.PhysicalRect(ctx, box)
		side := p.X
		if p.Y < side {
			side = p.Y
		}
		img = src(side)
		fallthrough
	case img != nil:
		if img == nil {
			return
		}
		if tint {
			img = t.memo.tinted(img, ink(s))
		}
		ctx.DrawImageScaled(img, box.Min.X, box.Min.Y, box.Dx(), box.Dy())
	}
}
