package widget

import (
	"image"
	"image/color"
	"sync"
	"sync/atomic"
)

// MenuBarItem описывает один пункт верхнего горизонтального меню.
type MenuBarItem struct {
	Text    string       // отображаемый текст ("Файл", "Правка", ...)
	Items   []MenuItem   // подменю (пункты PopupMenu)
	OnClick func()       // обработчик, если нет подменю
}

// MenuBar — горизонтальная полоса меню (Menu / MainMenu) в стиле Windows.
//
// Каждый пункт верхнего уровня отображается горизонтально. При клике
// раскрывается PopupMenu с подпунктами. При наведении на соседний пункт
// подменю автоматически переключается.
//
// Поддержка XAML:
//
//	<Menu Name="mainMenu" Left="0" Top="0" Width="800" Height="28">
//	    <MenuItem Header="Файл">
//	        <MenuItem Text="Новый"/>
//	        <MenuItem Text="Открыть"/>
//	        <MenuItem Separator="True"/>
//	        <MenuItem Text="Выход"/>
//	    </MenuItem>
//	    <MenuItem Header="Правка">
//	        <MenuItem Text="Копировать"/>
//	        <MenuItem Text="Вставить"/>
//	    </MenuItem>
//	</Menu>
type MenuBar struct {
	Base

	mu    sync.RWMutex
	items []MenuBarItem

	// Вычисленные горизонтальные позиции каждого пункта.
	itemRects []image.Rectangle

	activeIdx int32 // индекс открытого пункта (-1 = ничего)
	hoverIdx  int32 // hover пункт верхнего уровня (-1 = нет)

	// Внутренний PopupMenu для подменю.
	popup *PopupMenu

	// Стиль.
	Background      color.RGBA
	TextColor       color.RGBA
	HoverBG         color.RGBA
	HoverTextColor  color.RGBA // текст пункта под курсором (Win2000 — белый на navy)
	ActiveBG        color.RGBA
	BorderColor     color.RGBA
	ItemPaddingX    int // горизонтальный padding текста пункта
	Height          int // высота полосы (по умолчанию 28)

	// OnSelect вызывается при выборе подпункта: (topIndex, subIndex, text).
	OnSelect func(topIndex int, subIndex int, text string)
}

// NewMenuBar создаёт пустую горизонтальную панель меню.
func NewMenuBar() *MenuBar {
	mb := &MenuBar{
		activeIdx:    -1,
		hoverIdx:     -1,
		Background:   color.RGBA{R: 30, G: 30, B: 38, A: 255},
		TextColor:    color.RGBA{R: 210, G: 214, B: 230, A: 255},
		HoverBG:      color.RGBA{R: 55, G: 55, B: 68, A: 255},
		ActiveBG:     color.RGBA{R: 44, G: 44, B: 49, A: 255},
		BorderColor:  color.RGBA{R: 50, G: 50, B: 62, A: 255},
		ItemPaddingX: 14,
		Height:       28,
		popup:        NewPopupMenu(),
	}
	return mb
}

// AddMenu добавляет пункт верхнего уровня с подменю.
func (mb *MenuBar) AddMenu(text string, items ...MenuItem) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.items = append(mb.items, MenuBarItem{Text: text, Items: items})
	mb.recalcRects()
}

// MenuCount возвращает число пунктов верхнего уровня.
func (mb *MenuBar) MenuCount() int {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return len(mb.items)
}

// Items возвращает копию пунктов меню (подпункты — как есть, без копирования
// вложенных срезов: их состав после сборки меню не меняется).
func (mb *MenuBar) Items() []MenuBarItem {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	out := make([]MenuBarItem, len(mb.items))
	copy(out, mb.items)
	return out
}

// SetMenuText меняет надпись пункта верхнего уровня. Нужен, когда состав меню
// уже собран, а текст должен поменяться на лету — например при смене языка
// интерфейса (пункты меню хранятся строками, а не привязками).
func (mb *MenuBar) SetMenuText(idx int, text string) {
	mb.mu.Lock()
	changed := false
	if idx >= 0 && idx < len(mb.items) && mb.items[idx].Text != text {
		mb.items[idx].Text = text
		changed = true
		mb.recalcRects() // ширина пункта зависит от надписи
	}
	mb.mu.Unlock()
	if changed {
		notifyUIChanged()
	}
}

// SetSubItemText меняет надпись подпункта меню (top — индекс пункта верхнего
// уровня, sub — индекс внутри его подменю).
func (mb *MenuBar) SetSubItemText(top, sub int, text string) {
	mb.mu.Lock()
	changed := false
	if top >= 0 && top < len(mb.items) {
		items := mb.items[top].Items
		if sub >= 0 && sub < len(items) && items[sub].Text != text {
			items[sub].Text = text
			changed = true
		}
	}
	mb.mu.Unlock()
	if changed {
		notifyUIChanged()
	}
}

// AddTopItem добавляет пункт без подменю (просто кнопка).
func (mb *MenuBar) AddTopItem(text string, onClick func()) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.items = append(mb.items, MenuBarItem{Text: text, OnClick: onClick})
	mb.recalcRects()
}

// SetBounds переопределяет для пересчёта позиций пунктов.
func (mb *MenuBar) SetBounds(r image.Rectangle) {
	mb.Base.SetBounds(r) // Base сам инвалидирует старую+новую область
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.recalcRects()
}

// setHoverIdx обновляет hover-пункт верхнего уровня. Полоса меню лежит
// в bounds — при фактическом изменении достаточно точечной инвалидации.
func (mb *MenuBar) setHoverIdx(idx int) {
	if atomic.SwapInt32(&mb.hoverIdx, int32(idx)) != int32(idx) {
		mb.Invalidate()
	}
}

// setActiveIdx обновляет индекс открытого пункта (подсветка в bounds полосы).
// Само подменю — overlay: его показ/скрытие инвалидирует PopupMenu.
func (mb *MenuBar) setActiveIdx(idx int) {
	if atomic.SwapInt32(&mb.activeIdx, int32(idx)) != int32(idx) {
		mb.Invalidate()
	}
}

// recalcRects вычисляет прямоугольники каждого пункта. Вызывать под Lock.
//
// Ширина — по НАСТОЯЩЕМУ замеру подписи, а не по восьми точкам на байт, как
// было раньше. `len` в Go возвращает длину в байтах: кириллический пункт
// получал вдвое большую ширину, чем нужно («Репозиторий» — 11 символов, 22
// байта, то есть 176 точек вместо примерно 90), подсветка при наведении
// выходила шире текста, а от шрифта и масштаба ширина не зависела вовсе.
//
// MeasureUIText, а не ctx.MeasureText: раскладка считается вне отрисовки —
// её просят и при SetBounds, и при смене состава меню, когда контекста нет.
func (mb *MenuBar) recalcRects() {
	b := mb.bounds
	rects := make([]image.Rectangle, len(mb.items))
	x := b.Min.X
	for i, item := range mb.items {
		textW := MeasureUIText(item.Text, DefaultFontSizePt) + mb.ItemPaddingX*2
		rects[i] = image.Rect(x, b.Min.Y, x+textW, b.Max.Y)
		x += textW
	}
	mb.itemRects = rects
}

// ─── Bounds (расширенные при открытом подменю) ──────────────────────────────

// Bounds возвращает bounds с учётом открытого popup.
func (mb *MenuBar) Bounds() image.Rectangle {
	base := mb.Base.Bounds()
	if atomic.LoadInt32(&mb.activeIdx) < 0 || !mb.popup.IsOpen() {
		return base
	}
	return base.Union(mb.popup.Bounds())
}

// ─── Overlay ──────────────────────────────────────────────────────────────────

// HasOverlay сообщает движку что подменю рисуется как overlay.
func (mb *MenuBar) HasOverlay() bool {
	return mb.popup.IsOpen()
}

// OverlayBounds возвращает объединённый прямоугольник всего каскада открытых
// подменю (абсолютные логические координаты) — для выноса в нативное окно.
// Пустой Rect, если подменю закрыто. Реализует widget.OverlayBoundsProvider.
func (mb *MenuBar) OverlayBounds() image.Rectangle {
	if !mb.popup.IsOpen() {
		return image.Rectangle{}
	}
	return mb.popup.OverlayBounds()
}

// DrawOverlay рисует подменю поверх всего UI.
func (mb *MenuBar) DrawOverlay(ctx DrawContext) {
	if mb.popup.IsOpen() {
		mb.popup.DrawOverlay(ctx)
	}
}

// ─── Draw ────────────────────────────────────────────────────────────────────

func (mb *MenuBar) Draw(ctx DrawContext) {
	b := mb.bounds
	if b.Empty() {
		return
	}

	// Фон полосы.
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), mb.Background)

	// Нижняя граница.
	ctx.DrawHLine(b.Min.X, b.Max.Y-1, b.Dx(), mb.BorderColor)

	mb.mu.RLock()
	items := mb.items
	rects := mb.itemRects
	mb.mu.RUnlock()

	active := int(atomic.LoadInt32(&mb.activeIdx))
	hover := int(atomic.LoadInt32(&mb.hoverIdx))

	for i, item := range items {
		if i >= len(rects) {
			break
		}
		r := rects[i]

		// Подсветка активного/hover.
		textCol := mb.TextColor
		if st := currentStyle(); st.Classic3D {
			// Классика Win2000: hover — выпуклая рамка, открытое меню —
			// утопленная; фон не меняется, текст остаётся чёрным.
			if i == active {
				drawBevelSunken(ctx, r.Min.X, r.Min.Y+1, r.Dx(), r.Dy()-2, st)
			} else if i == hover {
				ctx.DrawHLine(r.Min.X, r.Min.Y+1, r.Dx(), st.BevelLight)
				ctx.DrawVLine(r.Min.X, r.Min.Y+1, r.Dy()-2, st.BevelLight)
				ctx.DrawHLine(r.Min.X, r.Max.Y-2, r.Dx(), st.BevelShadow)
				ctx.DrawVLine(r.Max.X-1, r.Min.Y+1, r.Dy()-2, st.BevelShadow)
			}
		} else if i == active {
			ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), mb.ActiveBG)
		} else if i == hover {
			ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), mb.HoverBG)
			if mb.HoverTextColor.A > 0 {
				textCol = mb.HoverTextColor
			}
		}

		// Текст по центру вертикали.
		textY := r.Min.Y + (r.Dy()-13)/2
		ctx.DrawText(item.Text, r.Min.X+mb.ItemPaddingX, textY, textCol)
	}

	mb.drawDisabledOverlay(ctx)
}

// ─── События мыши ────────────────────────────────────────────────────────────

// hitTopItem возвращает индекс верхнего пункта под координатами, или -1.
func (mb *MenuBar) hitTopItem(x, y int) int {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	pt := image.Pt(x, y)
	for i, r := range mb.itemRects {
		if pt.In(r) {
			return i
		}
	}
	return -1
}

func (mb *MenuBar) OnMouseMove(x, y int) {
	if !mb.IsEnabled() {
		return
	}
	// Если курсор в полосе — обновляем hover и переключаем подменю.
	if image.Pt(x, y).In(mb.Base.Bounds()) {
		idx := mb.hitTopItem(x, y)
		mb.setHoverIdx(idx)

		// Если подменю уже открыто — переключаем при наведении.
		if atomic.LoadInt32(&mb.activeIdx) >= 0 && idx >= 0 && idx != int(atomic.LoadInt32(&mb.activeIdx)) {
			mb.openSubmenu(idx)
		}
		return
	}

	// Иначе — делегируем в popup.
	mb.setHoverIdx(-1)
	if mb.popup.IsOpen() {
		mb.popup.OnMouseMove(x, y)
	}
}

func (mb *MenuBar) OnMouseButton(e MouseEvent) bool {
	if !mb.IsEnabled() {
		return false
	}
	// Клик в полосе меню.
	if image.Pt(e.X, e.Y).In(mb.Base.Bounds()) {
		if e.Button != MouseLeft || e.Pressed {
			return true
		}

		idx := mb.hitTopItem(e.X, e.Y)
		if idx < 0 {
			return true
		}

		active := int(atomic.LoadInt32(&mb.activeIdx))
		if active == idx {
			// Повторный клик — закрыть.
			mb.closeSubmenu()
		} else {
			mb.openSubmenu(idx)
		}
		return true
	}

	// Клик в popup.
	if mb.popup.IsOpen() {
		handled := mb.popup.OnMouseButton(e)
		if !mb.popup.IsOpen() {
			// Popup закрылся (выбран пункт или клик за пределами).
			mb.setActiveIdx(-1)
		}
		return handled
	}

	return false
}

// ─── Подменю ─────────────────────────────────────────────────────────────────

func (mb *MenuBar) openSubmenu(idx int) {
	mb.mu.RLock()
	if idx < 0 || idx >= len(mb.items) {
		mb.mu.RUnlock()
		return
	}
	item := mb.items[idx]
	rects := mb.itemRects
	mb.mu.RUnlock()

	if len(item.Items) == 0 {
		// Нет подменю — просто выполняем OnClick.
		mb.closeSubmenu()
		if item.OnClick != nil {
			item.OnClick() // синхронно — вне mb.mu
		}
		return
	}

	mb.setActiveIdx(idx)

	// Настраиваем popup: копируем пункты подменю, настраиваем OnSelect.
	topIdx := idx
	mb.popup.SetItems(item.Items)
	mb.popup.OnSelect = func(subIdx int, text string) {
		mb.setActiveIdx(-1)
		if mb.OnSelect != nil {
			mb.OnSelect(topIdx, subIdx, text)
		}
	}

	// Открываем popup прямо под пунктом.
	r := rects[idx]
	mb.popup.Show(r.Min.X, r.Max.Y)
}

func (mb *MenuBar) closeSubmenu() {
	mb.popup.Close()
	mb.setActiveIdx(-1)
}

// ─── Клавиатура ──────────────────────────────────────────────────────────────

func (mb *MenuBar) OnKeyEvent(e KeyEvent) {
	if !e.Pressed {
		return
	}

	active := int(atomic.LoadInt32(&mb.activeIdx))

	// Если подменю открыто — делегируем Up/Down/Enter/Escape.
	if mb.popup.IsOpen() {
		switch e.Code {
		case KeyLeft:
			mb.navigateTop(-1)
			return
		case KeyRight:
			mb.navigateTop(1)
			return
		case KeyEscape:
			mb.closeSubmenu()
			return
		default:
			mb.popup.OnKeyEvent(e)
			if !mb.popup.IsOpen() {
				mb.setActiveIdx(-1)
			}
			return
		}
	}

	// Подменю не открыто.
	switch e.Code {
	case KeyLeft:
		mb.navigateTop(-1)
	case KeyRight:
		mb.navigateTop(1)
	case KeyEnter, KeyDown:
		if active >= 0 {
			mb.openSubmenu(active)
		}
	case KeyEscape:
		mb.closeSubmenu()
	}
}

// navigateTop переключает активный верхний пункт на delta (+1 или -1).
func (mb *MenuBar) navigateTop(delta int) {
	mb.mu.RLock()
	n := len(mb.items)
	mb.mu.RUnlock()
	if n == 0 {
		return
	}

	active := int(atomic.LoadInt32(&mb.activeIdx))
	if active < 0 {
		active = 0
	} else {
		active = (active + delta + n) % n
	}

	mb.openSubmenu(active)
}

// ─── Children ────────────────────────────────────────────────────────────────

func (mb *MenuBar) Children() []Widget { return nil }

// ─── Dismiss ─────────────────────────────────────────────────────────────────

func (mb *MenuBar) Dismiss() {
	mb.closeSubmenu()
}

// ─── Focusable ───────────────────────────────────────────────────────────────

func (mb *MenuBar) SetFocused(v bool) {}
func (mb *MenuBar) IsFocused() bool   { return atomic.LoadInt32(&mb.activeIdx) >= 0 }

// ─── Themeable ──────────────────────────────────────────────────────────────

// ApplyTheme обновляет цвета MenuBar и вложенного PopupMenu из темы.
func (mb *MenuBar) ApplyTheme(t *Theme) {
	mb.Background = t.PanelBG
	// Текст меню — цвет обычного текста (TitleText рассчитан на цветной
	// заголовок окна: в Win2000 он белый на navy и нечитаем на серой полосе).
	mb.TextColor = t.LabelText
	mb.HoverBG = t.MenuHoverBG
	mb.HoverTextColor = t.MenuHoverText
	mb.ActiveBG = t.MenuBG
	mb.BorderColor = t.Border
	if mb.popup != nil {
		mb.popup.ApplyTheme(t)
	}
}

// ─── Правка собранного меню ────────────────────────────────────────────────
//
// Меню собирается один раз — из XAML или AddMenu, — а меняться должно на
// лету: язык интерфейса переключают в самом меню, список языков зависит от
// того, какие файлы перевода лежат у пользователя, а галочки «Вид → Тема»
// отмечают текущий выбор. До этих методов состав правили записью в срез из
// Items(), то есть держались на детали реализации — «Items копирует только
// внешний срез».

// itemPath находит пункт по пути индексов и возвращает срез, в котором он
// лежит, вместе с его местом в этом срезе.
//
// Путь адресует любую глубину: {0} — первый пункт полосы, {0,2} — третий
// пункт его подменю, {0,2,1} — второй пункт вложенного подменю. Пустой путь
// или выход за границы — ok=false.
func (mb *MenuBar) itemPath(path []int) (items []MenuItem, idx int, ok bool) {
	if len(path) == 0 || path[0] < 0 || path[0] >= len(mb.items) {
		return nil, 0, false
	}
	if len(path) == 1 {
		// Верхний уровень живёт в MenuBarItem, а не в MenuItem: у него свой
		// метод (SetMenuText) — сюда он не попадает.
		return nil, path[0], false
	}
	items = mb.items[path[0]].Items
	for _, i := range path[1 : len(path)-1] {
		if i < 0 || i >= len(items) {
			return nil, 0, false
		}
		items = items[i].SubItems
	}
	last := path[len(path)-1]
	if last < 0 || last >= len(items) {
		return nil, 0, false
	}
	return items, last, true
}

// SetItemText меняет надпись пункта по пути индексов — на любой глубине.
//
// SetItemText(text, 0) — пункт полосы, SetItemText(text, 0, 2) — третий пункт
// его подменю, SetItemText(text, 0, 2, 1) — второй пункт вложенного. Прежний
// SetSubItemText остаётся и делает то же, что SetItemText(text, top, sub).
func (mb *MenuBar) SetItemText(text string, path ...int) {
	if len(path) == 1 {
		mb.SetMenuText(path[0], text)
		return
	}
	mb.mu.Lock()
	changed := false
	if items, idx, ok := mb.itemPath(path); ok && items[idx].Text != text {
		items[idx].Text = text
		changed = true
	}
	mb.mu.Unlock()
	if changed {
		notifyUIChanged()
	}
}

// SetItemChecked ставит или снимает отметку у пункта по пути индексов.
//
// Пункт с непустым RadioGroup ведёт себя как переключатель: отметка снимается
// у соседей той же группы в том же подменю.
func (mb *MenuBar) SetItemChecked(checked bool, path ...int) {
	mb.mu.Lock()
	changed := false
	if items, idx, ok := mb.itemPath(path); ok {
		changed = setCheckedIn(items, idx, checked)
	}
	mb.mu.Unlock()
	if changed {
		notifyUIChanged()
	}
}

// ItemAt возвращает копию пункта по пути индексов.
//
// Копию, а не указатель: править состав меню полагается методами ниже, а
// возвращённый указатель пережил бы любую перестройку и начал бы менять то,
// чего на экране уже нет.
func (mb *MenuBar) ItemAt(path ...int) (MenuItem, bool) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	items, idx, ok := mb.itemPath(path)
	if !ok {
		return MenuItem{}, false
	}
	return items[idx], true
}

// SetMenus заменяет весь состав полосы меню.
func (mb *MenuBar) SetMenus(items ...MenuBarItem) {
	mb.mu.Lock()
	mb.items = append([]MenuBarItem(nil), items...)
	mb.recalcRects()
	mb.mu.Unlock()
	mb.closeIfOpen()
	notifyUIChanged()
}

// InsertMenu вставляет пункт полосы перед позицией idx. Индекс за пределами
// списка означает «в конец» — так удобнее строить меню в цикле.
func (mb *MenuBar) InsertMenu(idx int, item MenuBarItem) {
	mb.mu.Lock()
	if idx < 0 {
		idx = 0
	}
	if idx > len(mb.items) {
		idx = len(mb.items)
	}
	mb.items = append(mb.items, MenuBarItem{})
	copy(mb.items[idx+1:], mb.items[idx:])
	mb.items[idx] = item
	mb.recalcRects()
	mb.mu.Unlock()
	mb.closeIfOpen()
	notifyUIChanged()
}

// RemoveMenu удаляет пункт полосы. Несуществующий индекс — не событие.
func (mb *MenuBar) RemoveMenu(idx int) {
	mb.mu.Lock()
	if idx < 0 || idx >= len(mb.items) {
		mb.mu.Unlock()
		return
	}
	mb.items = append(mb.items[:idx], mb.items[idx+1:]...)
	mb.recalcRects()
	mb.mu.Unlock()
	mb.closeIfOpen()
	notifyUIChanged()
}

// ClearMenus убирает все пункты.
func (mb *MenuBar) ClearMenus() {
	mb.mu.Lock()
	mb.items = nil
	mb.recalcRects()
	mb.mu.Unlock()
	mb.closeIfOpen()
	notifyUIChanged()
}

// SetMenuItems заменяет подменю пункта верхнего уровня.
//
// Ради этого метода запрос и подан: список языков динамический — пользователь
// кладёт свой файл перевода в каталог, и такой язык надо показать в «Вид →
// Язык», не пересобирая всё меню.
func (mb *MenuBar) SetMenuItems(top int, items ...MenuItem) {
	mb.mu.Lock()
	if top < 0 || top >= len(mb.items) {
		mb.mu.Unlock()
		return
	}
	mb.items[top].Items = append([]MenuItem(nil), items...)
	mb.mu.Unlock()
	mb.closeIfOpen()
	notifyUIChanged()
}

// closeIfOpen закрывает раскрытое подменю после правки состава.
//
// Обязательно: открытое подменю показывает СНИМОК пунктов, взятый при
// открытии, а его размеры и попадание мыши считаются по нему же. Оставить его
// открытым поверх нового состава значит показывать пункты, которых уже нет, и
// звать их обработчики.
func (mb *MenuBar) closeIfOpen() {
	if atomic.LoadInt32(&mb.activeIdx) < 0 {
		return
	}
	mb.setActiveIdx(-1)
	mb.popup.Close()
}
