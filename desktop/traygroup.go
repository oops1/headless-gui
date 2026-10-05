// traygroup.go — группа значков трея одной кнопкой: «сеть + звук + питание» в
// Windows 11 не три кнопки, а одна плашка с общей подсветкой и одним щелчком
// (открывает быстрые настройки).
//
// Группа держит значки (NetworkItem, VolumeItem, PowerItem, TrayIcon — любой
// Item), раскладывает их в ряд внутри своей плашки и рисует сама. Значки не
// дети группы в дереве виджетов: мышь и клавиши принимает только группа,
// поэтому наведение подсвечивает всю плашку, щелчок по любому значку — один
// OnClick, а в обходе Tab группа — одна остановка. Значок без своей плашки
// (Normal-заливка прозрачна) на плашке группы читается как часть кнопки.
//
// Подсказка у каждого значка своя: ToolTipAt отдаёт подсказку значка под
// курсором («Сеть: дом», «Звук: 50 %»).
package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ComponentTrayGroup — имя компонента группы в стилях темы (плашка).
const ComponentTrayGroup = "tray.group"

const (
	// KeyTrayGroupPad — поле плашки слева и справа от значков.
	KeyTrayGroupPad theme.Key = "tray.group.pad"
	// KeyTrayGroupGap — зазор между значками группы (сверх их собственных
	// отступов tray.*.PadX).
	KeyTrayGroupGap theme.Key = "tray.group.gap"
	// KeyTrayGroupHeight — высота плашки; 0 — решает панель (taskbar.item.height).
	KeyTrayGroupHeight theme.Key = "tray.group.height"
)

// TrayGroup — группа значков трея с общей подсветкой.
//
//	grp := desktop.NewTrayGroup(tm,
//	    desktop.NewNetworkStatus(tm, st),
//	    desktop.NewVolumeStatus(tm, st),
//	    desktop.NewPowerStatus(tm, st))
//	grp.OnClick = func() { quick.Toggle(grp.Bounds()) }
//	grp.TrackManager(flyouts, "quick")       // горит, пока панель открыта
//	tray.AddItem(grp)
type TrayGroup struct {
	iconButton
	items []Item
}

var (
	_ Item             = (*TrayGroup)(nil)
	_ widget.Focusable = (*TrayGroup)(nil)
	_ FocusNavigable   = (*TrayGroup)(nil)
	_ FocusRinger      = (*TrayGroup)(nil)
)

// NewTrayGroup создаёт группу из значков items (по порядку слева направо).
func NewTrayGroup(tm *theme.Manager, items ...Item) *TrayGroup {
	g := &TrayGroup{}
	g.init(tm, ComponentTrayGroup)
	for _, it := range items {
		g.AddItem(it)
	}
	return g
}

// AddItem дописывает значок в конец группы.
func (g *TrayGroup) AddItem(it Item) {
	if it == nil {
		return
	}
	g.items = append(g.items, it)
	g.layout()
	g.Invalidate()
}

// Items возвращает значки группы.
func (g *TrayGroup) Items() []Item { return append([]Item(nil), g.items...) }

// Close закрывает значки группы (отписывает их от источников).
func (g *TrayGroup) Close() {
	for _, it := range g.items {
		if c, ok := it.(interface{ Close() }); ok {
			c.Close()
		}
	}
}

// PreferredSize — значки в ряд с зазорами и полями плашки. Значок нулевой
// ширины (питание без батареи) места не занимает.
func (g *TrayGroup) PreferredSize(avail image.Point) image.Point {
	gap, pad := g.metric(KeyTrayGroupGap), g.metric(KeyTrayGroupPad)
	w, n := 0, 0
	for _, it := range g.items {
		iw := it.PreferredSize(avail).X
		if iw <= 0 {
			continue
		}
		if n > 0 {
			w += gap
		}
		w += iw
		n++
	}
	if n == 0 {
		return image.Point{}
	}
	w += 2 * pad
	if avail.X > 0 && w > avail.X {
		w = avail.X
	}
	return image.Pt(w, g.metric(KeyTrayGroupHeight))
}

// SetBounds задаёт границы плашки и раскладывает значки.
func (g *TrayGroup) SetBounds(r image.Rectangle) {
	g.Base.SetBounds(r)
	g.layout()
}

// layout ставит значки в ряд внутри плашки на всю её высоту: значок сам
// центрирует глиф по высоте (trayInner).
func (g *TrayGroup) layout() {
	b := g.Bounds()
	gap, pad := g.metric(KeyTrayGroupGap), g.metric(KeyTrayGroupPad)
	x := b.Min.X + pad
	avail := image.Pt(b.Dx(), b.Dy())
	for _, it := range g.items {
		w := it.PreferredSize(avail).X
		if b.Empty() || w <= 0 {
			it.SetBounds(image.Rectangle{})
			continue
		}
		it.SetBounds(image.Rect(x, b.Min.Y, x+w, b.Max.Y).Intersect(b))
		x += w + gap
	}
}

// OnKeyEvent: Enter и Space — как щелчок, стрелки — к соседу по области.
func (g *TrayGroup) OnKeyEvent(e widget.KeyEvent) {
	g.HandleKey(g, e, click(g.OnClick), g.Invalidate)
}

// ToolTipAt возвращает подсказку значка под точкой (реализует интерфейс
// подсказок движка): у группы нет общей подсказки, у каждого значка своя.
func (g *TrayGroup) ToolTipAt(x, y int) string {
	pt := image.Pt(x, y)
	for _, it := range g.items {
		if !pt.In(it.Bounds()) {
			continue
		}
		if t, ok := it.(interface{ GetToolTip() string }); ok {
			return t.GetToolTip()
		}
	}
	return ""
}

// Draw рисует общую плашку, затем значки.
func (g *TrayGroup) Draw(ctx widget.DrawContext) {
	if _, s := g.paintPlate(ctx); s == nil {
		return
	}
	for _, it := range g.items {
		if !it.Bounds().Empty() {
			it.Draw(ctx)
		}
	}
}
