package widget

import (
	"image"
	"image/color"
	"sync"
	"sync/atomic"
)

// Dropdown — выпадающий список в стиле Windows 10 Dark.
type Dropdown struct {
	Base

	mu       sync.RWMutex
	items    []string
	selIdx   int32 // атомарно
	hoverIdx int32 // индекс пункта под курсором (-1 = нет)

	Background  color.RGBA
	BorderColor color.RGBA
	TextColor   color.RGBA
	ArrowColor  color.RGBA
	FocusBorder color.RGBA
	HoverItemBG color.RGBA // фон пункта при наведении
	HoverBG     color.RGBA // фон заголовка при наведении

	open     int32 // 1 — список раскрыт, 0 — закрыт (атомарно)
	isHover  int32 // курсор над заголовком (атомарно)
	focused  int32 // 0 | 1 (атомарно)

	PaddingX int

	// OnChange вызывается при смене выбора.
	OnChange func(idx int, text string)
}

// NewDropdown создаёт выпадающий список с заданными пунктами.
func NewDropdown(items ...string) *Dropdown {
	return &Dropdown{
		items:       items,
		hoverIdx:    -1,
		Background:  win10.DropBG,
		BorderColor: win10.DropBorder,
		TextColor:   win10.DropText,
		ArrowColor:  win10.DropArrow,
		FocusBorder: win10.Accent,
		HoverItemBG: color.RGBA{R: 45, G: 45, B: 80, A: 255},
		HoverBG:     color.RGBA{R: 55, G: 55, B: 60, A: 255},
		PaddingX:    6,
	}
}

// SetSelected потокобезопасно устанавливает индекс выбранного пункта.
func (d *Dropdown) SetSelected(idx int) {
	d.mu.RLock()
	n := len(d.items)
	d.mu.RUnlock()
	if idx >= 0 && idx < n {
		atomic.StoreInt32(&d.selIdx, int32(idx))
	}
}

// Selected возвращает индекс текущего выбранного пункта.
func (d *Dropdown) Selected() int {
	return int(atomic.LoadInt32(&d.selIdx))
}

// SelectedText возвращает текст выбранного пункта (или "" если список пуст).
func (d *Dropdown) SelectedText() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	idx := int(atomic.LoadInt32(&d.selIdx))
	if idx < 0 || idx >= len(d.items) {
		return ""
	}
	return d.items[idx]
}

// IsOpen возвращает true если список раскрыт. Потокобезопасно.
func (d *Dropdown) IsOpen() bool {
	return atomic.LoadInt32(&d.open) == 1
}

// SetOpen открывает или закрывает выпадающий список. Потокобезопасно.
func (d *Dropdown) SetOpen(v bool) {
	if v {
		atomic.StoreInt32(&d.open, 1)
	} else {
		atomic.StoreInt32(&d.open, 0)
	}
}

// OnMouseMove обновляет hover-состояние заголовка и пунктов.
func (d *Dropdown) OnMouseMove(x, y int) {
	header := d.Base.Bounds()
	if image.Pt(x, y).In(header) {
		atomic.StoreInt32(&d.isHover, 1)
	} else {
		atomic.StoreInt32(&d.isHover, 0)
	}

	if atomic.LoadInt32(&d.open) == 1 {
		const itemH = 30
		d.mu.RLock()
		n := len(d.items)
		d.mu.RUnlock()
		hov := int32(-1)
		for i := 0; i < n; i++ {
			itemTop := header.Max.Y + i*itemH
			if y >= itemTop && y < itemTop+itemH && x >= header.Min.X && x < header.Max.X {
				hov = int32(i)
				break
			}
		}
		atomic.StoreInt32(&d.hoverIdx, hov)
	} else {
		atomic.StoreInt32(&d.hoverIdx, -1)
	}
}

func (d *Dropdown) Draw(ctx DrawContext) {
	b := d.bounds
	if b.Empty() {
		return
	}

	isOpen := atomic.LoadInt32(&d.open) == 1

	bg := d.Background
	if atomic.LoadInt32(&d.isHover) == 1 && !isOpen && d.HoverBG.A > 0 {
		bg = d.HoverBG
	}
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), bg)

	border := d.BorderColor
	if isOpen {
		border = d.FocusBorder
	}
	ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), border)

	// Текст выбранного пункта
	selText := d.SelectedText()
	if selText == "" && len(d.items) > 0 {
		selText = d.items[0]
	}
	textY := b.Min.Y + (b.Dy()-13)/2
	ctx.DrawText(selText, b.Min.X+d.PaddingX, textY, d.TextColor)

	// Стрелка ▼
	arrowX := b.Max.X - 16
	arrowY := b.Min.Y + b.Dy()/2 - 1
	drawArrowDown(ctx, arrowX, arrowY, d.ArrowColor)

	// Разделитель перед стрелкой
	ctx.DrawVLine(b.Max.X-20, b.Min.Y+2, b.Dy()-4, d.BorderColor)

	// Раскрытый список рисуется через overlay (поверх всех виджетов).
	// См. DrawOverlay / HasOverlay ниже.

	d.drawChildren(ctx)
	d.drawDisabledOverlay(ctx)
}

// HasOverlay возвращает true, когда список раскрыт.
func (d *Dropdown) HasOverlay() bool {
	return atomic.LoadInt32(&d.open) == 1
}

// DrawOverlay рисует раскрытый список поверх всех виджетов.
// Вызывается движком после отрисовки всего дерева.
func (d *Dropdown) DrawOverlay(ctx DrawContext) {
	d.drawOpenList(ctx)
}

// drawOpenList рисует раскрытый список под полем (стиль Windows 10 ComboBox).
func (d *Dropdown) drawOpenList(ctx DrawContext) {
	d.mu.RLock()
	items := d.items
	d.mu.RUnlock()

	b := d.bounds
	const itemH = 30 // высота элемента как в Windows 10
	listY := b.Max.Y
	listW := b.Dx()
	listH := len(items) * itemH

	sel := int(atomic.LoadInt32(&d.selIdx))
	hov := int(atomic.LoadInt32(&d.hoverIdx))

	// Общий фон списка
	ctx.FillRect(b.Min.X, listY, listW, listH, d.Background)

	for i, item := range items {
		iy := listY + i*itemH
		switch {
		case i == hov && d.HoverItemBG.A > 0:
			ctx.FillRect(b.Min.X+2, iy, listW-4, itemH, d.HoverItemBG)
		case i == sel:
			ctx.FillRect(b.Min.X+2, iy, listW-4, itemH, win10.DropItemBG)
		}
		// Текст — вертикально по центру элемента
		ctx.DrawText(item, b.Min.X+d.PaddingX, iy+(itemH-13)/2, d.TextColor)
	}

	// Одна общая рамка вокруг всего списка
	ctx.DrawBorder(b.Min.X, listY, listW, listH, d.FocusBorder)
}

// drawArrowDown рисует маленькую стрелку ▼.
func drawArrowDown(ctx DrawContext, x, y int, col color.RGBA) {
	for i := 0; i < 4; i++ {
		ctx.DrawHLine(x+i, y+i, 7-2*i, col)
	}
	ctx.SetPixel(x+3, y+4, col)
}

// ApplyTheme обновляет цвета выпадающего списка.
func (d *Dropdown) ApplyTheme(t *Theme) {
	d.Background = t.DropBG
	d.BorderColor = t.DropBorder
	d.TextColor = t.DropText
	d.ArrowColor = t.DropArrow
	d.FocusBorder = t.Accent
}

// ─── Focusable ───────────────────────────────────────────────────────────────

func (d *Dropdown) SetFocused(v bool) {
	if v {
		atomic.StoreInt32(&d.focused, 1)
	} else {
		atomic.StoreInt32(&d.focused, 0)
	}
}

func (d *Dropdown) IsFocused() bool {
	return atomic.LoadInt32(&d.focused) == 1
}

// ─── KeyHandler ──────────────────────────────────────────────────────────────

func (d *Dropdown) OnKeyEvent(e KeyEvent) {
	if !d.IsEnabled() || !e.Pressed {
		return
	}

	isOpen := atomic.LoadInt32(&d.open) == 1

	switch e.Code {
	case KeySpace, KeyEnter:
		// Открыть/закрыть список
		if isOpen {
			atomic.StoreInt32(&d.open, 0)
		} else {
			atomic.StoreInt32(&d.open, 1)
		}

	case KeyUp:
		idx := int(atomic.LoadInt32(&d.selIdx))
		if idx > 0 {
			atomic.StoreInt32(&d.selIdx, int32(idx-1))
			d.fireOnChange(idx - 1)
		}

	case KeyDown:
		d.mu.RLock()
		n := len(d.items)
		d.mu.RUnlock()
		idx := int(atomic.LoadInt32(&d.selIdx))
		if idx < n-1 {
			atomic.StoreInt32(&d.selIdx, int32(idx+1))
			d.fireOnChange(idx + 1)
		}

	case KeyEscape:
		if isOpen {
			atomic.StoreInt32(&d.open, 0)
		}
	}
}

// fireOnChange вызывает OnChange в горутине (если задан).
func (d *Dropdown) fireOnChange(idx int) {
	if d.OnChange == nil {
		return
	}
	d.mu.RLock()
	text := ""
	if idx >= 0 && idx < len(d.items) {
		text = d.items[idx]
	}
	d.mu.RUnlock()
	go d.OnChange(idx, text)
}

// BaseBounds возвращает базовый прямоугольник заголовка (без выпавшего списка).
// Используется ShiftWidget, чтобы при перетаскивании панели не раздувать bounds.
func (d *Dropdown) BaseBounds() image.Rectangle {
	return d.Base.Bounds()
}

// Dismiss закрывает выпадающий список. Реализует widget.Dismissable.
func (d *Dropdown) Dismiss() {
	atomic.StoreInt32(&d.open, 0)
	atomic.StoreInt32(&d.hoverIdx, -1)
}

// Bounds возвращает расширенный прямоугольник когда список открыт, чтобы
// hitTest(engine/events.go) находил виджет при клике на пункты списка.
func (d *Dropdown) Bounds() image.Rectangle {
	b := d.Base.Bounds()
	if atomic.LoadInt32(&d.open) == 0 {
		return b
	}
	d.mu.RLock()
	n := len(d.items)
	d.mu.RUnlock()
	const itemH = 30
	return image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Max.Y+n*itemH)
}

// OnMouseButton реализует widget.MouseClickHandler.
func (d *Dropdown) OnMouseButton(e MouseEvent) bool {
	if !d.IsEnabled() {
		return false
	}
	if e.Button != MouseLeft || !e.Pressed {
		return false
	}

	header := d.Base.Bounds()

	if atomic.LoadInt32(&d.open) == 0 {
		atomic.StoreInt32(&d.open, 1)
		return true
	}

	// Список открыт: проверяем попадание по пункту.
	const itemH = 30
	d.mu.RLock()
	items := d.items
	d.mu.RUnlock()

	for i := range items {
		itemTop := header.Max.Y + i*itemH
		itemBot := itemTop + itemH
		if e.X >= header.Min.X && e.X < header.Max.X && e.Y >= itemTop && e.Y < itemBot {
			prev := int(atomic.SwapInt32(&d.selIdx, int32(i)))
			atomic.StoreInt32(&d.open, 0)
			if prev != i && d.OnChange != nil {
				d.mu.RLock()
				text := ""
				if i < len(d.items) {
					text = d.items[i]
				}
				d.mu.RUnlock()
				go d.OnChange(i, text)
			}
			return true
		}
	}

	atomic.StoreInt32(&d.open, 0)
	return true
}
