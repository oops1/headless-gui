package widget

import (
	"image"
	"image/color"
	"sync"
	"sync/atomic"
)

// MenuItem описывает один пункт контекстного / popup-меню.
type MenuItem struct {
	Text      string // текст пункта
	Icon      string // зарезервировано (иконка)
	Separator bool   // true — горизонтальный разделитель вместо текста
	Disabled  bool   // серый, некликабельный пункт
	OnClick   func() // обработчик
}

// PopupMenu — контекстное / всплывающее меню в стиле Windows 10.
//
// Меню рисуется как overlay поверх всего дерева виджетов и автоматически
// закрывается при клике за пределами или при нажатии Escape.
//
// Поддержка XAML:
//
//	<PopupMenu Name="ctxMenu">
//	    <MenuItem Text="Копировать"/>
//	    <MenuItem Separator="True"/>
//	    <MenuItem Text="Удалить"/>
//	</PopupMenu>
//
// Также может быть открыто программно:
//
//	menu.Show(x, y)      // показать в указанных координатах
//	menu.ShowBelow(btn)   // показать под виджетом
type PopupMenu struct {
	Base

	mu    sync.RWMutex
	items []MenuItem

	// Положение popup (абсолютные координаты).
	popupX, popupY int
	popupW, popupH int

	open     int32 // 1 — показано, 0 — скрыто (атомарно)
	hoverIdx int32 // индекс пункта под курсором (-1 = нет)

	// Стиль.
	Background    color.RGBA
	BorderColor   color.RGBA
	TextColor     color.RGBA
	DisabledColor color.RGBA
	HoverBG       color.RGBA
	SeparatorColor color.RGBA
	ShadowColor   color.RGBA

	ItemHeight    int // высота обычного пункта (по умолчанию 30)
	SeparatorH    int // высота разделителя (по умолчанию 9)
	PaddingX      int // горизонтальный отступ текста
	MinWidth      int // минимальная ширина меню

	// OnSelect вызывается при выборе пункта (index, text).
	OnSelect func(index int, text string)
}

// NewPopupMenu создаёт пустое popup-меню.
func NewPopupMenu() *PopupMenu {
	return &PopupMenu{
		hoverIdx:      -1,
		Background:    color.RGBA{R: 44, G: 44, B: 49, A: 250},
		BorderColor:   color.RGBA{R: 70, G: 70, B: 78, A: 255},
		TextColor:     color.RGBA{R: 230, G: 230, B: 230, A: 255},
		DisabledColor: color.RGBA{R: 110, G: 110, B: 115, A: 255},
		HoverBG:       color.RGBA{R: 62, G: 62, B: 70, A: 255},
		SeparatorColor: color.RGBA{R: 70, G: 70, B: 78, A: 255},
		ShadowColor:   color.RGBA{R: 0, G: 0, B: 0, A: 60},
		ItemHeight:    30,
		SeparatorH:    9,
		PaddingX:      16,
		MinWidth:      160,
	}
}

// AddItem добавляет пункт меню.
func (m *PopupMenu) AddItem(text string, onClick func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append(m.items, MenuItem{Text: text, OnClick: onClick})
}

// AddSeparator добавляет горизонтальный разделитель.
func (m *PopupMenu) AddSeparator() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append(m.items, MenuItem{Separator: true})
}

// SetItems заменяет все пункты меню.
func (m *PopupMenu) SetItems(items []MenuItem) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = items
}

// Items возвращает копию пунктов.
func (m *PopupMenu) Items() []MenuItem {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]MenuItem, len(m.items))
	copy(result, m.items)
	return result
}

// IsOpen возвращает true если меню показано.
func (m *PopupMenu) IsOpen() bool {
	return atomic.LoadInt32(&m.open) == 1
}

// Show открывает popup-меню в указанных абсолютных координатах.
func (m *PopupMenu) Show(x, y int) {
	m.mu.RLock()
	w, h := m.calcSize()
	m.mu.RUnlock()

	m.popupX = x
	m.popupY = y
	m.popupW = w
	m.popupH = h
	atomic.StoreInt32(&m.hoverIdx, -1)
	atomic.StoreInt32(&m.open, 1)
}

// ShowBelow открывает popup-меню прямо под указанным виджетом.
func (m *PopupMenu) ShowBelow(w Widget) {
	b := w.Bounds()
	m.Show(b.Min.X, b.Max.Y)
}

// ShowRight открывает popup-меню справа от указанного виджета.
func (m *PopupMenu) ShowRight(w Widget) {
	b := w.Bounds()
	m.Show(b.Max.X, b.Min.Y)
}

// Close закрывает меню.
func (m *PopupMenu) Close() {
	atomic.StoreInt32(&m.open, 0)
	atomic.StoreInt32(&m.hoverIdx, -1)
}

// Dismiss реализует Dismissable — закрывает меню при DismissAll.
func (m *PopupMenu) Dismiss() {
	m.Close()
}

// ─── Размеры ────────────────────────────────────────────────────────────────

// calcSize вычисляет ширину и высоту popup (вызывать под RLock).
func (m *PopupMenu) calcSize() (w, h int) {
	w = m.MinWidth
	for _, item := range m.items {
		if item.Separator {
			h += m.SeparatorH
		} else {
			h += m.ItemHeight
			// Примерная ширина текста: 8px на символ + padding.
			textW := len(item.Text)*8 + m.PaddingX*2 + 24
			if textW > w {
				w = textW
			}
		}
	}
	h += 4 // верхний + нижний padding
	return
}

// itemAtY возвращает индекс пункта по Y-координатe (абсолютной).
// Возвращает -1 если нет пункта (разделитель, за пределами).
func (m *PopupMenu) itemAtY(y int) int {
	curY := m.popupY + 2 // верхний padding
	for i, item := range m.items {
		var itemH int
		if item.Separator {
			itemH = m.SeparatorH
		} else {
			itemH = m.ItemHeight
		}
		if y >= curY && y < curY+itemH {
			if item.Separator || item.Disabled {
				return -1
			}
			return i
		}
		curY += itemH
	}
	return -1
}

// popupRect возвращает bounds popup-области.
func (m *PopupMenu) popupRect() image.Rectangle {
	return image.Rect(m.popupX, m.popupY, m.popupX+m.popupW, m.popupY+m.popupH)
}

// ─── Bounds (расширенные при открытии) ───────────────────────────────────────

// Bounds возвращает расширенные bounds включая popup для hit-test.
func (m *PopupMenu) Bounds() image.Rectangle {
	base := m.Base.Bounds()
	if atomic.LoadInt32(&m.open) == 0 {
		return base
	}
	pr := m.popupRect()
	return base.Union(pr)
}

// BaseBounds возвращает оригинальные bounds (без popup).
func (m *PopupMenu) BaseBounds() image.Rectangle {
	return m.Base.Bounds()
}

// ─── Overlay ────────────────────────────────────────────────────────────────

// HasOverlay сообщает движку что меню рисуется как overlay.
func (m *PopupMenu) HasOverlay() bool {
	return atomic.LoadInt32(&m.open) == 1
}

// DrawOverlay рисует popup-меню поверх всего UI.
func (m *PopupMenu) DrawOverlay(ctx DrawContext) {
	if atomic.LoadInt32(&m.open) == 0 {
		return
	}

	m.mu.RLock()
	items := m.items
	m.mu.RUnlock()

	px, py := m.popupX, m.popupY
	pw, ph := m.popupW, m.popupH
	hover := int(atomic.LoadInt32(&m.hoverIdx))

	// Тень (2px смещение вправо-вниз).
	ctx.FillRectAlpha(px+2, py+2, pw, ph, m.ShadowColor)

	// Фон popup.
	ctx.FillRect(px, py, pw, ph, m.Background)

	// Рамка.
	ctx.DrawBorder(px, py, pw, ph, m.BorderColor)

	// Пункты.
	curY := py + 2
	for i, item := range items {
		if item.Separator {
			sepY := curY + m.SeparatorH/2
			ctx.DrawHLine(px+8, sepY, pw-16, m.SeparatorColor)
			curY += m.SeparatorH
			continue
		}

		// Hover-подсветка.
		if i == hover && !item.Disabled {
			ctx.FillRect(px+2, curY, pw-4, m.ItemHeight, m.HoverBG)
		}

		// Текст.
		textY := curY + (m.ItemHeight-13)/2
		textCol := m.TextColor
		if item.Disabled {
			textCol = m.DisabledColor
		}
		ctx.DrawText(item.Text, px+m.PaddingX, textY, textCol)

		curY += m.ItemHeight
	}
}

// Draw — основной виджет невидим (всё рисуется через DrawOverlay).
func (m *PopupMenu) Draw(ctx DrawContext) {
	// PopupMenu не имеет основного рендеринга — только overlay.
}

// ─── События ────────────────────────────────────────────────────────────────

// OnMouseMove обрабатывает hover по пунктам.
func (m *PopupMenu) OnMouseMove(x, y int) {
	if atomic.LoadInt32(&m.open) == 0 {
		return
	}

	pr := m.popupRect()
	if !image.Pt(x, y).In(pr) {
		atomic.StoreInt32(&m.hoverIdx, -1)
		return
	}

	m.mu.RLock()
	idx := m.itemAtY(y)
	m.mu.RUnlock()
	atomic.StoreInt32(&m.hoverIdx, int32(idx))
}

// OnMouseButton обрабатывает клик: выбор пункта или закрытие.
func (m *PopupMenu) OnMouseButton(e MouseEvent) bool {
	if atomic.LoadInt32(&m.open) == 0 {
		return false
	}

	if e.Button != MouseLeft || e.Pressed {
		// Закрытие по правому клику.
		if e.Button == MouseRight && !e.Pressed {
			m.Close()
			return true
		}
		return false
	}

	// Отпускание ЛКМ.
	pr := m.popupRect()
	if !image.Pt(e.X, e.Y).In(pr) {
		// Клик за пределами — закрыть.
		m.Close()
		return true
	}

	m.mu.RLock()
	idx := m.itemAtY(e.Y)
	m.mu.RUnlock()

	if idx >= 0 {
		m.mu.RLock()
		item := m.items[idx]
		m.mu.RUnlock()

		m.Close()

		if item.OnClick != nil {
			go item.OnClick()
		}
		if m.OnSelect != nil {
			go m.OnSelect(idx, item.Text)
		}
	}

	return true
}

// OnKeyEvent обрабатывает навигацию: стрелки, Enter, Escape.
func (m *PopupMenu) OnKeyEvent(e KeyEvent) {
	if !e.Pressed || atomic.LoadInt32(&m.open) == 0 {
		return
	}

	m.mu.RLock()
	count := len(m.items)
	m.mu.RUnlock()

	if count == 0 {
		return
	}

	hover := int(atomic.LoadInt32(&m.hoverIdx))

	switch e.Code {
	case KeyEscape:
		m.Close()

	case KeyUp:
		hover = m.prevActiveItem(hover)
		atomic.StoreInt32(&m.hoverIdx, int32(hover))

	case KeyDown:
		hover = m.nextActiveItem(hover)
		atomic.StoreInt32(&m.hoverIdx, int32(hover))

	case KeyEnter:
		if hover >= 0 {
			m.mu.RLock()
			item := m.items[hover]
			m.mu.RUnlock()
			if !item.Disabled && !item.Separator {
				m.Close()
				if item.OnClick != nil {
					go item.OnClick()
				}
				if m.OnSelect != nil {
					go m.OnSelect(hover, item.Text)
				}
			}
		}
	}
}

// nextActiveItem ищет следующий активный (не disabled, не separator) пункт.
func (m *PopupMenu) nextActiveItem(from int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := len(m.items)
	for i := 1; i <= n; i++ {
		idx := (from + i) % n
		if !m.items[idx].Separator && !m.items[idx].Disabled {
			return idx
		}
	}
	return -1
}

// prevActiveItem ищет предыдущий активный пункт.
func (m *PopupMenu) prevActiveItem(from int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := len(m.items)
	if from < 0 {
		from = 0
	}
	for i := 1; i <= n; i++ {
		idx := (from - i + n) % n
		if !m.items[idx].Separator && !m.items[idx].Disabled {
			return idx
		}
	}
	return -1
}

// ─── Focusable ──────────────────────────────────────────────────────────────

func (m *PopupMenu) SetFocused(v bool) {}
func (m *PopupMenu) IsFocused() bool   { return m.IsOpen() }

// ─── Themeable ──────────────────────────────────────────────────────────────

// ApplyTheme обновляет цвета из темы.
func (m *PopupMenu) ApplyTheme(t *Theme) {
	m.Background = t.DropBG
	m.BorderColor = t.DropBorder
	m.TextColor = t.DropText
	m.HoverBG = t.ListItemHover
}
