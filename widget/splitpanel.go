// Package widget — SplitPanel: контейнер из двух панелей с перетаскиваемым
// разделителем (WPF-аналог: два региона GridSplitter'а без явного Grid).
//
// В отличие от GridSplitter (который двигает столбцы/строки родительского Grid),
// SplitPanel сам является контейнером: он держит ДВУХ детей (First/Second —
// первые два ребёнка, добавленные через AddChild) и раскладывает их по обе
// стороны полосы-разделителя. Позиция разделителя хранится как доля 0..1
// доступного места, поэтому при ресайзе контейнера соотношение сохраняется.
//
// Взаимодействие:
//   - hover над полосой → курсор SizeWE (Horizontal) / SizeNS (Vertical);
//   - перетаскивание полосы ЛКМ (через CaptureManager) двигает границу с
//     клэмпом по MinFirst/MinSecond;
//   - двойной клик по полосе сворачивает First (Position→0, прежняя позиция
//     запоминается) / повторный двойной клик — разворачивает обратно.
//
// ВНИМАНИЕ для интеграции: чтобы вложение SplitPanel в Canvas/DockPanel и т.п.
// не «двоило» сдвиги, тип *SplitPanel нужно добавить в widget.HasOwnLayout
// (widget/canvas.go) — SplitPanel сам перекладывает детей в SetBounds.
package widget

import (
	"image"
	"image/color"
	"math"
	"time"
)

// splitDoubleClickMs — окно (мс) для распознавания двойного клика по полосе.
const splitDoubleClickMs = 400

// splitGrabPad — расширение hit-зоны полосы по перпендикуляру (px), чтобы
// тонкую полосу было легче «схватить» мышью.
const splitGrabPad = 3

// SplitPanel — контейнер из двух панелей с перетаскиваемым разделителем.
type SplitPanel struct {
	Base

	// Orientation задаёт направление разделения:
	//   OrientationHorizontal — панели слева/справа, полоса вертикальная;
	//   OrientationVertical   — панели сверху/снизу, полоса горизонтальная.
	Orientation Orientation

	// SplitterSize — толщина полосы-разделителя в пикселях (по умолчанию 6).
	SplitterSize int

	// Position — позиция разделителя как доля 0..1 доступного места
	// (ширины/высоты за вычетом полосы). Определяет размер First-панели.
	Position float64

	// MinFirst / MinSecond — минимальные размеры панелей в пикселях.
	// Клэмп применяется при перетаскивании и раскладке (но НЕ при коллапсе).
	MinFirst  int
	MinSecond int

	// Background / HoverColor — цвет полосы (обычный / при hover или drag).
	Background color.RGBA
	HoverColor color.RGBA

	// OnPositionChanged вызывается при изменении Position (drag, коллапс,
	// SetPosition). Аргумент — новая доля 0..1.
	OnPositionChanged func(pos float64)

	capMgr      CaptureManager
	dragging    bool
	dragMoved   bool  // текущий press перешёл в реальное перетаскивание
	hovered     bool
	collapsed   bool
	savedPos    float64 // позиция до коллапса (для восстановления)
	lastClickMs int64   // время последнего «клика» по полосе (для double-click)
}

// NewSplitPanel создаёт SplitPanel с разделителем посередине (Position=0.5).
func NewSplitPanel(orient Orientation) *SplitPanel {
	return &SplitPanel{
		Orientation:  orient,
		SplitterSize: 6,
		Position:     0.5,
		Background:   win10.SplitterBG,
		HoverColor:   win10.SplitterHoverBG,
		savedPos:     0.5,
	}
}

// horizontal возвращает true, если разделение идёт по оси X (Horizontal —
// панели слева/справа, полоса вертикальная).
func (sp *SplitPanel) horizontal() bool {
	return sp.Orientation == OrientationHorizontal
}

// First возвращает первую панель (или nil, если детей нет).
func (sp *SplitPanel) First() Widget {
	if len(sp.children) > 0 {
		return sp.children[0]
	}
	return nil
}

// Second возвращает вторую панель (или nil).
func (sp *SplitPanel) Second() Widget {
	if len(sp.children) > 1 {
		return sp.children[1]
	}
	return nil
}

// IsCollapsed сообщает, свёрнута ли First-панель.
func (sp *SplitPanel) IsCollapsed() bool { return sp.collapsed }

// splitterSize возвращает неотрицательную толщину полосы.
func (sp *SplitPanel) splitterSize() int {
	if sp.SplitterSize < 0 {
		return 0
	}
	return sp.SplitterSize
}

// clampFirst ограничивает размер First-панели диапазоном [0, avail] и, если
// места хватает, минимумами MinFirst/MinSecond.
func (sp *SplitPanel) clampFirst(fw, avail int) int {
	if fw < 0 {
		fw = 0
	}
	if fw > avail {
		fw = avail
	}
	minF, minS := sp.MinFirst, sp.MinSecond
	if minF < 0 {
		minF = 0
	}
	if minS < 0 {
		minS = 0
	}
	// Минимумы применяем только когда они физически помещаются.
	if minF+minS <= avail {
		if fw < minF {
			fw = minF
		}
		if fw > avail-minS {
			fw = avail - minS
		}
	}
	return fw
}

// firstExtent вычисляет текущий размер First-панели (fw), доступное место
// (avail) и толщину полосы (ss) вдоль оси разделения.
func (sp *SplitPanel) firstExtent() (fw, avail, ss int) {
	b := sp.bounds
	ss = sp.splitterSize()
	if sp.horizontal() {
		avail = b.Dx() - ss
	} else {
		avail = b.Dy() - ss
	}
	if avail < 0 {
		avail = 0
	}
	if sp.collapsed {
		return 0, avail, ss
	}
	fw = sp.clampFirst(int(math.Round(sp.Position*float64(avail))), avail)
	return fw, avail, ss
}

// barRect возвращает прямоугольник полосы-разделителя в абсолютных координатах.
func (sp *SplitPanel) barRect() image.Rectangle {
	b := sp.bounds
	fw, _, ss := sp.firstExtent()
	if ss <= 0 || b.Empty() {
		return image.Rectangle{}
	}
	if sp.horizontal() {
		return image.Rect(b.Min.X+fw, b.Min.Y, b.Min.X+fw+ss, b.Max.Y)
	}
	return image.Rect(b.Min.X, b.Min.Y+fw, b.Max.X, b.Min.Y+fw+ss)
}

// grabRect — расширенная hit-зона полосы (легче попасть мышью).
func (sp *SplitPanel) grabRect() image.Rectangle {
	bar := sp.barRect()
	if bar.Empty() {
		return bar
	}
	if sp.horizontal() {
		return image.Rect(bar.Min.X-splitGrabPad, bar.Min.Y, bar.Max.X+splitGrabPad, bar.Max.Y)
	}
	return image.Rect(bar.Min.X, bar.Min.Y-splitGrabPad, bar.Max.X, bar.Max.Y+splitGrabPad)
}

// layout раскладывает First/Second по обе стороны полосы.
func (sp *SplitPanel) layout() {
	b := sp.bounds
	if b.Empty() {
		return
	}
	fw, _, ss := sp.firstExtent()
	if sp.horizontal() {
		if len(sp.children) > 0 {
			sp.children[0].SetBounds(image.Rect(b.Min.X, b.Min.Y, b.Min.X+fw, b.Max.Y))
		}
		if len(sp.children) > 1 {
			sp.children[1].SetBounds(image.Rect(b.Min.X+fw+ss, b.Min.Y, b.Max.X, b.Max.Y))
		}
	} else {
		if len(sp.children) > 0 {
			sp.children[0].SetBounds(image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+fw))
		}
		if len(sp.children) > 1 {
			sp.children[1].SetBounds(image.Rect(b.Min.X, b.Min.Y+fw+ss, b.Max.X, b.Max.Y))
		}
	}
}

// SetBounds задаёт границы и перекладывает детей.
func (sp *SplitPanel) SetBounds(r image.Rectangle) {
	sp.Base.SetBounds(r)
	sp.layout()
}

// AddChild добавляет ребёнка (первые два — First/Second) и перекладывает.
func (sp *SplitPanel) AddChild(w Widget) {
	sp.Base.AddChild(w)
	sp.layout()
}

// SetPosition задаёт долю разделителя 0..1 (снимает коллапс).
func (sp *SplitPanel) SetPosition(pos float64) {
	sp.collapsed = false
	sp.applyPosition(pos)
}

// applyPosition клэмпит долю, перекладывает детей и уведомляет.
func (sp *SplitPanel) applyPosition(pos float64) {
	if pos < 0 {
		pos = 0
	}
	if pos > 1 {
		pos = 1
	}
	sp.Position = pos
	sp.layout()
	sp.Invalidate()
	if sp.OnPositionChanged != nil {
		sp.OnPositionChanged(pos)
	}
}

// Collapse сворачивает First-панель (Position→0, прежняя позиция запоминается).
func (sp *SplitPanel) Collapse() {
	if sp.collapsed {
		return
	}
	sp.savedPos = sp.Position
	sp.collapsed = true
	sp.layout()
	sp.Invalidate()
	if sp.OnPositionChanged != nil {
		sp.OnPositionChanged(0)
	}
}

// Expand разворачивает ранее свёрнутую First-панель.
func (sp *SplitPanel) Expand() {
	if !sp.collapsed {
		return
	}
	sp.collapsed = false
	sp.applyPosition(sp.savedPos)
}

// ToggleCollapse переключает коллапс First-панели.
func (sp *SplitPanel) ToggleCollapse() {
	if sp.collapsed {
		sp.Expand()
	} else {
		sp.Collapse()
	}
}

// ── Мышь / capture ───────────────────────────────────────────────────────────

// SetCaptureManager сохраняет менеджер захвата мыши (CaptureAware).
func (sp *SplitPanel) SetCaptureManager(cm CaptureManager) { sp.capMgr = cm }

// WantsCapture захватывает мышь при нажатии ЛКМ на полосе-разделителе.
func (sp *SplitPanel) WantsCapture(e MouseEvent) bool {
	if !sp.IsEnabled() || e.Button != MouseLeft || !e.Pressed {
		return false
	}
	return image.Pt(e.X, e.Y).In(sp.grabRect())
}

// OnMouseButton начинает/заканчивает перетаскивание; двойной клик — коллапс.
func (sp *SplitPanel) OnMouseButton(e MouseEvent) bool {
	if e.Button != MouseLeft {
		return false
	}
	if e.Pressed {
		now := time.Now().UnixMilli()
		if sp.lastClickMs != 0 && now-sp.lastClickMs < splitDoubleClickMs {
			// Двойной клик по полосе — коллапс/восстановление.
			sp.lastClickMs = 0
			sp.dragging = false
			sp.ToggleCollapse()
			return true
		}
		sp.lastClickMs = now
		sp.dragMoved = false
		if !sp.dragging {
			sp.dragging = true
			sp.Invalidate()
		}
		return true
	}
	// release
	if sp.dragging {
		sp.dragging = false
		sp.Invalidate()
	}
	// Перетаскивание НЕ должно засчитываться как «клик» для двойного клика:
	// иначе два быстрых drag'а подряд ложно сворачивали бы панель.
	if sp.dragMoved {
		sp.lastClickMs = 0
	}
	sp.dragMoved = false
	if sp.capMgr != nil {
		sp.capMgr.ReleaseCapture()
	}
	return true
}

// OnMouseMove двигает границу при drag'е или обновляет hover полосы.
func (sp *SplitPanel) OnMouseMove(x, y int) {
	if sp.dragging {
		b := sp.bounds
		_, avail, _ := sp.firstExtent()
		if avail <= 0 {
			return
		}
		half := sp.splitterSize() / 2
		var fw int
		if sp.horizontal() {
			fw = x - b.Min.X - half
		} else {
			fw = y - b.Min.Y - half
		}
		fw = sp.clampFirst(fw, avail)
		newPos := float64(fw) / float64(avail)
		sp.collapsed = false
		if newPos != sp.Position {
			sp.dragMoved = true
			sp.applyPosition(newPos)
		}
		return
	}
	hovered := image.Pt(x, y).In(sp.grabRect())
	if hovered != sp.hovered {
		sp.hovered = hovered
		sp.Invalidate()
	}
}

// Cursor возвращает курсор изменения размера над полосой (CursorProvider).
func (sp *SplitPanel) Cursor(x, y int) Cursor {
	if image.Pt(x, y).In(sp.grabRect()) {
		if sp.horizontal() {
			return CursorSizeWE
		}
		return CursorSizeNS
	}
	return CursorArrow
}

// ── Отрисовка / тема ─────────────────────────────────────────────────────────

// Draw рисует детей и полосу-разделитель поверх зазора.
func (sp *SplitPanel) Draw(ctx DrawContext) {
	if sp.bounds.Empty() {
		return
	}
	sp.drawChildren(ctx)
	bar := sp.barRect()
	if bar.Empty() {
		return
	}
	col := sp.Background
	if sp.hovered || sp.dragging {
		col = sp.HoverColor
	}
	// Полоса без скруглений (в т.ч. Classic3D) — простой заливкой, как GridSplitter.
	ctx.FillRect(bar.Min.X, bar.Min.Y, bar.Dx(), bar.Dy(), col)
}

// ApplyTheme перекрашивает полосу из темы.
func (sp *SplitPanel) ApplyTheme(t *Theme) {
	sp.Background = t.SplitterBG
	sp.HoverColor = t.SplitterHoverBG
}
