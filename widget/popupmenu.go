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

	// Checkable — пункт-переключатель: слева от подписи отводится место под
	// отметку, даже когда она снята. Без этого поля соседние пункты одного
	// меню разъезжались бы по левому краю, стоило отметить один из них.
	//
	// Отводится место сразу всему меню, если хоть один пункт помечен
	// Checkable: так же ведёт себя WPF, и так подписи стоят в столбик.
	Checkable bool
	// Checked — отметка стоит. Осмысленно только при Checkable.
	Checked bool
	// RadioGroup — имя группы взаимного исключения. Пункты с одинаковым
	// непустым именем В ОДНОМ подменю ведут себя как переключатель: отметка
	// одного снимает её с остальных (SetItemChecked, PopupMenu.SetItemChecked).
	// Пустое имя — обычный флажок, независимый от соседей.
	RadioGroup string
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

	// Положение popup (абсолютные координаты) и каскадное дочернее подменю.
	// Пишутся из обработчиков событий (Show/openChild/closeChild), читаются
	// рендер-горутиной (DrawOverlay/OverlayBounds) — под отдельным geoMu, чтобы
	// не смешивать с m.mu (items) и не ловить гонку по memory model (SEC-18).
	// Порядок захвата: m.mu → geoMu (никогда наоборот).
	geoMu          sync.Mutex
	popupX, popupY int
	popupW, popupH int

	open     int32 // 1 — показано, 0 — скрыто (атомарно)
	// dismissSeq — номер нажатия (CurrentPressSeq), которым меню было
	// погашено через Dismiss. Атомарно.
	dismissSeq uint64
	hoverIdx int32 // индекс пункта под курсором (-1 = нет)

	// Каскадное дочернее подменю (под geoMu).
	child       *PopupMenu // текущее открытое вложенное подменю
	childForIdx int        // индекс пункта, для которого открыт child (-1 = нет)
	parent      *PopupMenu // родительское меню (nil для корневого)

	// Стиль.
	Background     color.RGBA
	BorderColor    color.RGBA
	TextColor      color.RGBA
	DisabledColor  color.RGBA
	HoverBG        color.RGBA
	HoverTextColor color.RGBA // текст пункта под курсором (Win2000 — белый на navy)
	SeparatorColor color.RGBA
	ShadowColor    color.RGBA

	ItemHeight   int // высота обычного пункта (по умолчанию 30)
	SeparatorH   int // высота разделителя (по умолчанию 9)
	PaddingX     int // горизонтальный отступ текста
	MinWidth     int // минимальная ширина меню
	ArrowPadding int // отступ для стрелки ► справа

	// OnSelect вызывается при выборе пункта (index, text).
	OnSelect func(index int, text string)
}

// ─── Размер экрана (для удержания меню в пределах канваса) ───────────────────

var (
	screenMu     sync.Mutex
	screenWidth  int
	screenHeight int
)

// activeRootPopup — единственное открытое корневое контекстное меню.
// Гарантирует, что одновременно показано не более одного контекстного меню:
// при открытии нового предыдущее закрывается (подменю не считаются — у них parent != nil).
var (
	activePopupMu   sync.Mutex
	activeRootPopup *PopupMenu
)

// SetScreenBounds сообщает виджетам размер канваса (вызывается движком при
// создании/смене разрешения). Используется popup-меню и другими overlay'ами,
// чтобы не выходить за границы экрана.
func SetScreenBounds(w, h int) {
	screenMu.Lock()
	screenWidth, screenHeight = w, h
	screenMu.Unlock()
}

// popupsHosted — глобальный флаг: оверлеи выносятся в собственные нативные
// окна-попапы ОС (движок зарегистрировал PopupSink). Когда он взведён, клэмп
// позиции popup по границам канваса (getScreenBounds) пропускается — экранным
// позиционированием занимается хост, а popup вправе выходить за пределы холста.
// В headless (без хоста) флаг сброшен и поведение прежнее до пикселя.
var popupsHosted atomic.Bool

// SetPopupsHosted включает/выключает режим вынесенных popup-оверлеев.
// Вызывается движком при регистрации/снятии PopupSink.
func SetPopupsHosted(v bool) { popupsHosted.Store(v) }

// PopupsHosted сообщает, активен ли режим вынесенных popup-оверлеев.
func PopupsHosted() bool { return popupsHosted.Load() }

// getScreenBounds возвращает размер канваса (0,0 если ещё не задан).
func getScreenBounds() (int, int) {
	screenMu.Lock()
	defer screenMu.Unlock()
	return screenWidth, screenHeight
}

// NewPopupMenu создаёт пустое popup-меню.
func NewPopupMenu() *PopupMenu {
	// Цвета — из активной темы: контекстные меню создаются на лету
	// (TextInput и др.) и должны следовать текущей теме, а не Win10 Dark.
	m := &PopupMenu{
		hoverIdx:       -1,
		childForIdx:    -1,
		Background:     win10.MenuBG,
		BorderColor:    win10.DropBorder,
		TextColor:      win10.DropText,
		DisabledColor:  win10.Disabled,
		HoverBG:        win10.MenuHoverBG,
		HoverTextColor: win10.MenuHoverText,
		SeparatorColor: win10.DropBorder,
		ShadowColor:    win10.ShadowColor,
		ItemHeight:     30,
		SeparatorH:     9,
		PaddingX:       16,
		MinWidth:       160,
		ArrowPadding:   20,
	}
	if win10.Style.Classic3D {
		m.ItemHeight = 22 // классика: компактные пункты меню
		m.SeparatorH = 7
	}
	return m
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

// SetItemText меняет надпись пункта по индексу. Нужен, когда меню уже собрано,
// а текст должен обновиться на лету — например при смене языка интерфейса.
func (m *PopupMenu) SetItemText(idx int, text string) {
	m.mu.Lock()
	changed := false
	if idx >= 0 && idx < len(m.items) && m.items[idx].Text != text {
		m.items[idx].Text = text
		changed = true
	}
	m.mu.Unlock()
	if changed {
		notifyUIChanged()
	}
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
// Позиция корректируется так, чтобы меню целиком помещалось в канвасе
// (сдвиг влево/вверх у правого/нижнего края — меню не обрезается границей окна).
func (m *PopupMenu) Show(x, y int) {
	// Закрываем предыдущее корневое контекстное меню (одно меню за раз).
	if m.parent == nil {
		activePopupMu.Lock()
		prev := activeRootPopup
		activeRootPopup = m
		activePopupMu.Unlock()
		if prev != nil && prev != m {
			prev.Close()
		}
	}

	m.mu.RLock()
	w, h := m.calcSize()
	m.mu.RUnlock()

	// Клэмп в границы канваса — только БЕЗ хоста попапов. При активном хостинге
	// экранное позиционирование делает popupHost, а меню вправе выходить за холст.
	if sw, sh := getScreenBounds(); sw > 0 && sh > 0 && !popupsHosted.Load() {
		if x+w > sw {
			x = sw - w
		}
		if x < 0 {
			x = 0
		}
		if y+h > sh {
			y = sh - h
		}
		if y < 0 {
			y = 0
		}
	}

	m.geoMu.Lock()
	m.popupX = x
	m.popupY = y
	m.popupW = w
	m.popupH = h
	m.geoMu.Unlock()
	atomic.StoreInt32(&m.hoverIdx, -1)
	atomic.StoreInt32(&m.open, 1)
	notifyUIChanged() // появление overlay-меню (вне bounds виджета)
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
	wasOpen := atomic.SwapInt32(&m.open, 0) == 1
	atomic.StoreInt32(&m.hoverIdx, -1)
	if m.parent == nil {
		activePopupMu.Lock()
		if activeRootPopup == m {
			activeRootPopup = nil
		}
		activePopupMu.Unlock()
	}
	if wasOpen {
		notifyUIChanged() // исчезновение overlay-меню
	}
}

// setHoverIdx обновляет hover-пункт меню. Пункты рисуются в overlay вне
// bounds виджета — при фактическом изменении инвалидируется весь кадр.
func (m *PopupMenu) setHoverIdx(idx int) {
	if atomic.SwapInt32(&m.hoverIdx, int32(idx)) != int32(idx) {
		notifyUIChanged()
	}
}

// closeChild закрывает дочернее подменю, если открыто.
func (m *PopupMenu) closeChild() {
	m.geoMu.Lock()
	c := m.child
	m.child = nil
	m.childForIdx = -1
	m.geoMu.Unlock()
	if c != nil && c.IsOpen() {
		c.Close()
	}
}

// geo возвращает положение popup под geoMu.
func (m *PopupMenu) geo() (x, y, w, h int) {
	m.geoMu.Lock()
	x, y, w, h = m.popupX, m.popupY, m.popupW, m.popupH
	m.geoMu.Unlock()
	return
}

// openChildOf возвращает текущее открытое дочернее подменю (nil, если его
// нет или оно закрыто) и индекс пункта, для которого оно открыто.
func (m *PopupMenu) openChildOf() (*PopupMenu, int) {
	m.geoMu.Lock()
	c, idx := m.child, m.childForIdx
	m.geoMu.Unlock()
	if c == nil || !c.IsOpen() {
		return nil, idx
	}
	return c, idx
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
	if c, cIdx := m.openChildOf(); c != nil && cIdx == idx {
		return
	}

	// Закрываем предыдущее дочернее.
	m.closeChild()

	child := NewPopupMenu()
	child.parent = m
	child.Background = m.Background
	child.BorderColor = m.BorderColor
	child.TextColor = m.TextColor
	child.HoverTextColor = m.HoverTextColor
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
	// Если справа не помещается — открываем слева от родителя (как в WPF/ОС).
	child.mu.RLock()
	cw, _ := child.calcSize()
	child.mu.RUnlock()
	itemY := m.itemYForIndex(idx)
	px, _, pw, _ := m.geo()
	x := px + pw - 2
	// Разворот влево у правого края — только без хоста (хост позиционирует сам).
	if sw, _ := getScreenBounds(); sw > 0 && x+cw > sw && !popupsHosted.Load() {
		x = px - cw + 2
	}
	child.Show(x, itemY)

	m.geoMu.Lock()
	m.child = child
	m.childForIdx = idx
	m.geoMu.Unlock()
}

// itemYForIndex возвращает абсолютную Y-координату верхнего края пункта.
func (m *PopupMenu) itemYForIndex(idx int) int {
	_, py, _, _ := m.geo()
	m.mu.RLock()
	defer m.mu.RUnlock()
	y := py + 2
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
	if c, _ := m.openChildOf(); c != nil {
		r = r.Union(c.fullBounds())
	}
	return r
}

// Dismiss реализует Dismissable — закрывает меню при DismissAll.
func (m *PopupMenu) Dismiss() {
	// Кто именно погасил меню, важно кнопке-владельцу: движок закрывает
	// overlay'и вне пути клика ДО доставки события, поэтому кнопка, лежащая
	// в другом виджете (шеврон в заголовке окна), иначе не может отличить
	// «меню было закрыто» от «меню закрыл мой же клик». См. CurrentPressSeq.
	//
	// Метку ставим ТОЛЬКО если меню было открыто: движок гасит и уже
	// закрытые меню, а такая отметка заставила бы кнопку проглотить
	// нажатие, которое должно меню открыть.
	if atomic.LoadInt32(&m.open) == 1 {
		atomic.StoreUint64(&m.dismissSeq, CurrentPressSeq())
	}
	m.Close()
}

// dismissedByPress сообщает, было ли меню погашено нажатием seq (0 — никогда).
func (m *PopupMenu) dismissedByPress(seq uint64) bool {
	return seq != 0 && atomic.LoadUint64(&m.dismissSeq) == seq
}

// ─── Размеры ────────────────────────────────────────────────────────────────

// calcSize вычисляет ширину и высоту popup (вызывать под RLock).
func (m *PopupMenu) calcSize() (w, h int) {
	w = m.MinWidth
	hasSubItems := false
	gutter := m.checkGutter()
	for _, item := range m.items {
		if item.Separator {
			h += m.SeparatorH
		} else {
			h += m.ItemHeight
			if len(item.SubItems) > 0 {
				hasSubItems = true
			}
			// Ширина по НАСТОЯЩЕМУ замеру подписи: len в Go считает байты, и
			// кириллический пункт выходил вдвое шире нужного.
			textW := MeasureUIText(item.Text, DefaultFontSizePt) + m.PaddingX*2 + 24 + gutter
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
	_, py, _, _ := m.geo()
	curY := py + 2 // верхний padding
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
	x, y, w, h := m.geo()
	return image.Rect(x, y, x+w, y+h)
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

// OverlayBounds возвращает объединённый прямоугольник popup и всех каскадных
// подменю (абсолютные логические координаты) — для выноса в нативное окно.
// Пустой Rect, если меню закрыто. Реализует widget.OverlayBoundsProvider.
func (m *PopupMenu) OverlayBounds() image.Rectangle {
	if atomic.LoadInt32(&m.open) == 0 {
		return image.Rectangle{}
	}
	return m.fullBounds()
}

// DrawOverlay рисует popup-меню поверх всего UI (включая каскадные подменю).
func (m *PopupMenu) DrawOverlay(ctx DrawContext) {
	if atomic.LoadInt32(&m.open) == 0 {
		return
	}

	m.mu.RLock()
	items := m.items
	m.mu.RUnlock()

	px, py, pw, ph := m.geo()
	openChild, openChildIdx := m.openChildOf()
	hover := int(atomic.LoadInt32(&m.hoverIdx))

	// Тень (2px смещение вправо-вниз).
	ctx.FillRectAlpha(px+2, py+2, pw, ph, m.ShadowColor)

	// Фон popup.
	ctx.FillRect(px, py, pw, ph, m.Background)

	// Рамка: классика — выпуклый 3D-бордюр (как меню Win2000), иначе плоская.
	if st := currentStyle(); st.Classic3D {
		drawBevelRaised(ctx, px, py, pw, ph, st)
	} else {
		ctx.DrawBorder(px, py, pw, ph, m.BorderColor)
	}

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
		isChildOpen := openChild != nil && openChildIdx == i
		hovered := (i == hover || isChildOpen) && !item.Disabled
		if hovered {
			ctx.FillRect(px+2, curY, pw-4, m.ItemHeight, m.HoverBG)
		}

		// Текст.
		textY := curY + (m.ItemHeight-13)/2
		textCol := m.TextColor
		if hovered && m.HoverTextColor.A > 0 {
			textCol = m.HoverTextColor // классика: белый на navy
		}
		if item.Disabled {
			textCol = m.DisabledColor
		}
		textX := px + m.PaddingX + m.checkGutter()
		if item.Checkable && item.Checked {
			drawCheckMark(ctx, image.Rect(px+m.PaddingX, curY, px+m.PaddingX+checkMarkSize,
				curY+m.ItemHeight), textCol)
		}
		ctx.DrawText(item.Text, textX, textY, textCol)

		// Стрелка ► для пунктов с подменю.
		if len(item.SubItems) > 0 {
			arrowX := px + pw - m.PaddingX
			ctx.DrawText("\u25b8", arrowX, textY, textCol)
		}

		curY += m.ItemHeight
	}

	// Рекурсивно рисуем дочернее подменю.
	if openChild != nil {
		openChild.DrawOverlay(ctx)
	}
}

// Draw — основной виджет невидим (всё рисуется через DrawOverlay).
func (m *PopupMenu) Draw(ctx DrawContext) {
	b := m.bounds
	if b.Empty() {
		return
	}
	// PopupMenu не имеет основного рендеринга — только overlay.
	m.drawDisabledOverlay(ctx)
}

// ─── События ────────────────────────────────────────────────────────────────

// OnMouseMove обрабатывает hover по пунктам и каскадные подменю.
func (m *PopupMenu) OnMouseMove(x, y int) {
	if !m.IsEnabled() {
		return
	}
	if atomic.LoadInt32(&m.open) == 0 {
		return
	}

	// Сначала проверяем дочернее подменю.
	if c, _ := m.openChildOf(); c != nil {
		childRect := c.fullBounds()
		if image.Pt(x, y).In(childRect) {
			c.OnMouseMove(x, y)
			return
		}
	}

	pr := m.popupRect()
	if !image.Pt(x, y).In(pr) {
		m.setHoverIdx(-1)
		return
	}

	m.mu.RLock()
	idx := m.itemAtY(y)
	m.mu.RUnlock()
	m.setHoverIdx(idx)

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
	if !m.IsEnabled() {
		return false
	}
	if atomic.LoadInt32(&m.open) == 0 {
		return false
	}

	// Сначала проверяем дочернее подменю.
	if c, _ := m.openChildOf(); c != nil {
		childRect := c.fullBounds()
		if image.Pt(e.X, e.Y).In(childRect) {
			return c.OnMouseButton(e)
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
			item.OnClick() // синхронно — меню уже закрыто, локи отпущены
		}
		if m.OnSelect != nil {
			m.OnSelect(idx, item.Text)
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
	if c, _ := m.openChildOf(); c != nil {
		c.OnKeyEvent(e)
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
		m.setHoverIdx(hover)

	case KeyDown:
		hover = m.nextActiveItem(hover)
		m.setHoverIdx(hover)

	case KeyRight:
		// Войти в подменю, если у текущего пункта есть SubItems.
		if hover >= 0 {
			m.mu.RLock()
			hasSubItems := hover < len(m.items) && len(m.items[hover].SubItems) > 0
			m.mu.RUnlock()
			if hasSubItems {
				m.openChild(hover)
				// Устанавливаем hover на первый пункт дочернего меню.
				if c, _ := m.openChildOf(); c != nil {
					first := c.nextActiveItem(-1)
					c.setHoverIdx(first)
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
					if c, _ := m.openChildOf(); c != nil {
						first := c.nextActiveItem(-1)
						c.setHoverIdx(first)
					}
					return
				}
				m.closeAll()
				if item.OnClick != nil {
					item.OnClick() // синхронно — меню уже закрыто, локи отпущены
				}
				if m.OnSelect != nil {
					m.OnSelect(hover, item.Text)
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
	m.Background = t.MenuBG
	m.BorderColor = t.DropBorder
	m.TextColor = t.DropText
	m.DisabledColor = t.Disabled
	m.HoverBG = t.MenuHoverBG
	m.HoverTextColor = t.MenuHoverText
	m.SeparatorColor = t.DropBorder
	m.ShadowColor = t.ShadowColor
	if t.Style.Classic3D {
		m.ItemHeight = 22 // классика: компактные пункты
		m.SeparatorH = 7
	} else {
		m.ItemHeight = 30
		m.SeparatorH = 9
	}
}

// ─── Отметки у пунктов (WPF MenuItem.IsChecked) ─────────────────────────────

// checkMarkSize — сторона поля под отметку. Отметка рисуется фигурой, а не
// символом шрифта: галочка обязана выглядеть одинаково при любом шрифте темы,
// включая тот, в котором нужного знака нет вовсе (тот же довод, что у уголка
// трея в desktop/systemtray.go).
const checkMarkSize = 14

// checkGutter — ширина поля под отметку слева от подписей.
//
// Отводится всему меню разом, если хоть один пункт объявлен Checkable: иначе
// подписи разъезжались бы по левому краю в тот момент, когда пользователь
// ставит отметку, — а меню не должно дёргаться от щелчка по нему.
func (m *PopupMenu) checkGutter() int {
	for _, item := range m.items {
		if item.Checkable {
			return checkMarkSize + 4
		}
	}
	return 0
}

// SetItemChecked ставит или снимает отметку у пункта idx.
//
// Пункт с непустым RadioGroup ведёт себя как переключатель: отметка снимается
// у соседей той же группы В ЭТОМ ЖЕ меню. Соседи глубже или выше по дереву к
// группе не относятся — иначе одно имя группы в разных подменю связывало бы
// несвязанные наборы.
func (m *PopupMenu) SetItemChecked(idx int, checked bool) {
	m.mu.Lock()
	changed := setCheckedIn(m.items, idx, checked)
	m.mu.Unlock()
	if changed {
		m.Invalidate()
		notifyUIChanged()
	}
}

// setCheckedIn — общая механика отметки для PopupMenu и MenuBar.
func setCheckedIn(items []MenuItem, idx int, checked bool) bool {
	if idx < 0 || idx >= len(items) {
		return false
	}
	if items[idx].Checked == checked {
		return false
	}
	items[idx].Checked = checked
	// Пункт, которому ставят отметку, обязан её показывать: иначе вызов
	// молча ничего не изменил бы на экране.
	if checked {
		items[idx].Checkable = true
	}

	if group := items[idx].RadioGroup; checked && group != "" {
		for i := range items {
			if i != idx && items[i].RadioGroup == group {
				items[i].Checked = false
			}
		}
	}
	return true
}

// drawCheckMark рисует галочку в отведённом поле.
//
// Две линии под углом, набранные точками: короткая вниз-вправо и длинная
// вверх-вправо. Толщина в две точки — иначе на светлой подложке галочка
// теряется, ровно как терялся уголок трея.
func drawCheckMark(ctx DrawContext, r image.Rectangle, col color.RGBA) {
	if col.A == 0 || r.Empty() {
		return
	}
	side := r.Dx()
	if r.Dy() < side {
		side = r.Dy()
	}
	// Галочка занимает не всё поле: по краям остаётся воздух, иначе она
	// сливается с подсветкой пункта.
	inset := side / 4
	if inset < 1 {
		inset = 1
	}
	x0 := r.Min.X + inset
	y0 := r.Min.Y + r.Dy()/2
	short := (side - 2*inset) / 3
	long := side - 2*inset - short
	if short < 1 || long < 1 {
		return
	}

	for i := 0; i <= short; i++ {
		ctx.SetPixel(x0+i, y0+i, col)
		ctx.SetPixel(x0+i, y0+i+1, col)
	}
	for i := 0; i <= long; i++ {
		ctx.SetPixel(x0+short+i, y0+short-i, col)
		ctx.SetPixel(x0+short+i, y0+short-i+1, col)
	}
}
