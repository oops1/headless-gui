package widget

import (
	"image"
	"image/color"
	"sync"
	"sync/atomic"
)

// MenuItem описывает один пункт контекстного / popup-меню.
type MenuItem struct {
	Text      string     // текст пункта
	Icon      string     // зарезервировано (иконка)
	Separator bool       // true — горизонтальный разделитель вместо текста
	Disabled  bool       // серый, некликабельный пункт
	OnClick   func()     // обработчик
	SubItems  []MenuItem // вложенные подменю (3+ уровень)
}

// PopupMenu — контекстное / всплывающее меню в стиле Windows 10.
//
// Меню рисуется как overlay поверх всего дерева виджетов и автоматически
// закрывается при клике за пределами или при нажатии Escape.
// Поддерживает каскадные вложенные подменю (SubItems).
type PopupMenu struct {
	Base

	mu    sync.RWMutex
	items []MenuItem

	// Положение popup (абсолютные координаты).
	popupX, popupY int
	popupW, popupH int

	open     int32 // 1 — показано, 0 — скрыто (атомарно)
	hoverIdx int32 // индекс пункта под курсором (-1 = нет)

	// Каскадное дочернее подменю.
	child        *PopupMenu // текущее открытое вложенное подменю
	childForIdx  int        // индекс пункта, для которого открыт child (-1 = нет)
	parent       *PopupMenu // родительское меню (nil для корневого)

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
	ArrowPadding  int // отступ для стрелки ► справа

	// OnSelect вызывается при выборе пункта (index, text).
	OnSelect func(index int, text string)
}

// NewPopupMenu создаёт пустое popup-меню.
func NewPopupMenu() *PopupMenu {
	return &PopupMenu{
		hoverIdx:       -1,
		childForIdx:    -1,
		Background:     color.RGBA{R: 44, G: 44, B: 49, A: 250},
		BorderColor:    color.RGBA{R: 70, G: 70, B: 78, A: 255},
		TextColor:      color.RGBA{R: 230, G: 230, B: 230, A: 255},
		DisabledColor:  color.RGBA{R: 110, G: 110, B: 115, A: 255},
		HoverBG:        color.RGBA{R: 62, G: 62, B: 70, A: 255},
		SeparatorColor: color.RGBA{R: 70, G: 70, B: 78, A: 255},
		ShadowColor:    color.RGBA{R: 0, G: 0, B: 0, A: 60},
		ItemHeight:     30,
		SeparatorH:     9,
		PaddingX:       16,
		MinWidth:       160,
		ArrowPadding:   20,
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

// Close закрывает меню и все дочерние подменю.
func (m *PopupMenu) Close() {
	m.closeChild()
	atomic.StoreInt32(&m.open, 0)
	atomic.StoreInt32(&m.hoverIdx, -1)
}

// closeChild закрывает дочернее подменю, если открыто.
func (m *PopupMenu) closeChild() {
	if m.child != nil && m.child.IsOpen() {
		m.child.Close()
	}
	m.child = nil
	m.childForIdx = -1
}

// openChild открывает каскадное подменю для пункта idx.
func (m *PopupMenu) openChild(idx int) {
	m.mu.RLock()
	if idx < 0 || idx >= len(m.items) || len(m.items[idx].SubItems) == 0 {
		m.mu.RUnlock()
		return
	}
	subItems := m.items[idx].SubItems
	m.mu.RUnlock()

	// Если уже открыто для этого пункта — ничего не делаем.
	if m.childForIdx == idx && m.child != nil && m.child.IsOpen() {
		return
	}

	// Закрываем предыдущее дочернее.
	m.closeChild()

	child := NewPopupMenu()
	child.parent = m
	child.Background = m.Background
	child.BorderColor = m.BorderColor
	child.TextColor = m.TextColor
	child.DisabledColor = m.DisabledColor
	child.HoverBG = m.HoverBG
	child.SeparatorColor = m.SeparatorColor
	child.ShadowColor = m.ShadowColor
	child.ItemHeight = m.ItemHeight
	child.SeparatorH = m.SeparatorH
	child.PaddingX = m.PaddingX
	child.MinWidth = m.MinWidth
	child.ArrowPadding = m.ArrowPadding
	child.SetItems(subItems)
	child.OnSelect = m.OnSelect

	// Позиция: справа от текущего popup, на уровне пункта.
	itemY := m.itemYForIndex(idx)
	child.Show(m.popupX+m.popupW-2, itemY)

	m.child = child
	m.childForIdx = idx
}

// itemYForIndex возвращает абсолютную Y-координату верхнего края пункта.
func (m *PopupMenu) itemYForIndex(idx int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	y := m.popupY + 2
	for i, item := range m.items {
		if i == idx {
			return y
		}
		if item.Separator {
			y += m.SeparatorH
		} else {
			y += m.ItemHeight
		}
	}
	return y
}

// fullBounds возвращает объединённые bounds этого popup и всех дочерних.
func (m *PopupMenu) fullBounds() image.Rectangle {
	r := m.popupRect()
	if m.child != nil && m.child.IsOpen() {
		r = r.Union(m.child.fullBounds())
	}
	return r
}

// Dismiss реализует Dismissable — закрывает меню при DismissAll.
func (m *PopupMenu) Dismiss() {
	m.Close()
}

// ─── Размеры ────────────────────────────────────────────────────────────────

// calcSize вычисляет ширину и высоту popup (вызывать под RLock).
func (m *PopupMenu) calcSize() (w, h int) {
	w = m.MinWidth
	hasSubItems := false
	for _, item := range m.items {
		if item.Separator {
			h += m.SeparatorH
		} else {
			h += m.ItemHeight
			if len(item.SubItems) > 0 {
				hasSubItems = true
			}
			// Примерная ширина текста: 8px на символ + padding.
			textW := len(item.Text)*8 + m.PaddingX*2 + 24
			if textW > w {
				w = textW
			}
		}
	}
	// Добавляем место для стрелки ► если есть подменю.
	if hasSubItems {
		w += m.ArrowPadding
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

// Bounds возвращает расширенные bounds включая popup и дочерние для hit-test.
func (m *PopupMenu) Bounds() image.Rectangle {
	base := m.Base.Bounds()
	if atomic.LoadInt32(&m.open) == 0 {
		return base
	}
	return base.Union(m.fullBounds())
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

// DrawOverlay рисует popup-меню поверх всего UI (включая каскадные подменю).
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

		// Hover-подсветка (а также подсветка пункта с открытым дочерним подменю).
		isChildOpen := m.childForIdx == i && m.child != nil && m.child.IsOpen()
		if (i == hover || isChildOpen) && !item.Disabled {
			ctx.FillRect(px+2, curY, pw-4, m.ItemHeight, m.HoverBG)
		}

		// Текст.
		textY := curY + (m.ItemHeight-13)/2
		textCol := m.TextColor
		if item.Disabled {
			textCol = m.DisabledColor
		}
		ctx.DrawText(item.Text, px+m.PaddingX, textY, textCol)

		// Стрелка ► для пунктов с подменю.
		if len(item.SubItems) > 0 {
			arrowX := px + pw - m.PaddingX
			ctx.DrawText("\u25b8", arrowX, textY, textCol)
		}

		curY += m.ItemHeight
	}

	// Рекурсивно рисуем дочернее подменю.
	if m.child != nil && m.child.IsOpen() {
		m.child.DrawOverlay(ctx)
	}
}

// Draw — основной виджет невидим (всё рисуется через DrawOverlay).
func (m *PopupMenu) Draw(ctx DrawContext) {
	// PopupMenu не имеет основного рендеринга — только overlay.
}

// ─── События ────────────────────────────────────────────────────────────────

// OnMouseMove обрабатывает hover по пунктам и каскадные подменю.
func (m *PopupMenu) OnMouseMove(x, y int) {
	if atomic.LoadInt32(&m.open) == 0 {
		return
	}

	// Сначала проверяем дочернее подменю.
	if m.child != nil && m.child.IsOpen() {
		childRect := m.child.fullBounds()
		if image.Pt(x, y).In(childRect) {
			m.child.OnMouseMove(x, y)
			return
		}
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

	// Если навели на пункт с SubItems — открываем дочернее подменю.
	if idx >= 0 {
		m.mu.RLock()
		hasSubItems := idx < len(m.items) && len(m.items[idx].SubItems) > 0
		m.mu.RUnlock()
		if hasSubItems {
			m.openChild(idx)
		} else {
			// Навели на пункт без подменю — закрываем дочернее.
			m.closeChild()
		}
	}
}

// OnMouseButton обрабатывает клик: выбор пункта или закрытие.
func (m *PopupMenu) OnMouseButton(e MouseEvent) bool {
	if atomic.LoadInt32(&m.open) == 0 {
		return false
	}

	// Сначала проверяем дочернее подменю.
	if m.child != nil && m.child.IsOpen() {
		childRect := m.child.fullBounds()
		if image.Pt(e.X, e.Y).In(childRect) {
			return m.child.OnMouseButton(e)
		}
	}

	if e.Button != MouseLeft || e.Pressed {
		// Закрытие по правому клику.
		if e.Button == MouseRight && !e.Pressed {
			m.Close()
			return true
		}
		// Поглощаем mouseDown внутри popup, чтобы dismissOutside
		// не закрыл меню до mouseUp.
		pr := m.popupRect()
		if image.Pt(e.X, e.Y).In(pr) {
			return true
		}
		return false
	}

	// Отпускание ЛКМ.
	pr := m.popupRect()
	if !image.Pt(e.X, e.Y).In(pr) {
		// Клик за пределами — закрыть всё.
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

		// Если у пункта есть подменю — не закрываем, а открываем каскад.
		if len(item.SubItems) > 0 {
			m.openChild(idx)
			return true
		}

		// Закрываем всю цепочку меню (вверх до корня).
		m.closeAll()

		if item.OnClick != nil {
			go item.OnClick()
		}
		if m.OnSelect != nil {
			go m.OnSelect(idx, item.Text)
		}
	}

	return true
}

// closeAll закрывает текущее меню и всех родителей (всю цепочку).
func (m *PopupMenu) closeAll() {
	// Находим корневое меню.
	root := m
	for root.parent != nil {
		root = root.parent
	}
	root.Close()
}

// OnKeyEvent обрабатывает навигацию: стрелки, Enter, Escape, Right (подменю), Left (назад).
func (m *PopupMenu) OnKeyEvent(e KeyEvent) {
	if !e.Pressed || atomic.LoadInt32(&m.open) == 0 {
		return
	}

	// Если есть открытое дочернее подменю — делегируем ему.
	if m.child != nil && m.child.IsOpen() {
		m.child.OnKeyEvent(e)
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
		if m.parent != nil {
			// Закрываем только текущий уровень (возврат к родителю).
			m.Close()
		} else {
			m.Close()
		}

	case KeyUp:
		hover = m.prevActiveItem(hover)
		atomic.StoreInt32(&m.hoverIdx, int32(hover))

	case KeyDown:
		hover = m.nextActiveItem(hover)
		atomic.StoreInt32(&m.hoverIdx, int32(hover))

	case KeyRight:
		// Войти в подменю, если у текущего пункта есть SubItems.
		if hover >= 0 {
			m.mu.RLock()
			hasSubItems := hover < len(m.items) && len(m.items[hover].SubItems) > 0
			m.mu.RUnlock()
			if hasSubItems {
				m.openChild(hover)
				// Устанавливаем hover на первый пункт дочернего меню.
				if m.child != nil {
					first := m.child.nextActiveItem(-1)
					atomic.StoreInt32(&m.child.hoverIdx, int32(first))
				}
			}
		}

	case KeyLeft:
		// Если есть родитель — закрываем текущий уровень.
		if m.parent != nil {
			m.Close()
		}

	case KeyEnter:
		if hover >= 0 {
			m.mu.RLock()
			item := m.items[hover]
			m.mu.RUnlock()
			if !item.Disabled && !item.Separator {
				// Если есть подменю — открываем каскад.
				if len(item.SubItems) > 0 {
					m.openChild(hover)
					if m.child != nil {
						first := m.child.nextActiveItem(-1)
						atomic.StoreInt32(&m.child.hoverIdx, int32(first))
					}
					return
				}
				m.closeAll()
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
