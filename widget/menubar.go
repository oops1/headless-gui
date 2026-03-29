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

// AddTopItem добавляет пункт без подменю (просто кнопка).
func (mb *MenuBar) AddTopItem(text string, onClick func()) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.items = append(mb.items, MenuBarItem{Text: text, OnClick: onClick})
	mb.recalcRects()
}

// SetBounds переопределяет для пересчёта позиций пунктов.
func (mb *MenuBar) SetBounds(r image.Rectangle) {
	mb.bounds = r
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.recalcRects()
}

// recalcRects вычисляет прямоугольники каждого пункта. Вызывать под Lock.
func (mb *MenuBar) recalcRects() {
	b := mb.bounds
	rects := make([]image.Rectangle, len(mb.items))
	x := b.Min.X
	for i, item := range mb.items {
		textW := len(item.Text)*8 + mb.ItemPaddingX*2
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

// DrawOverlay рисует подменю поверх всего UI.
func (mb *MenuBar) DrawOverlay(ctx DrawContext) {
	if mb.popup.IsOpen() {
		mb.popup.DrawOverlay(ctx)
	}
}

// ─── Draw ────────────────────────────────────────────────────────────────────

func (mb *MenuBar) Draw(ctx DrawContext) {
	b := mb.bounds

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
		if i == active {
			ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), mb.ActiveBG)
		} else if i == hover {
			ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), mb.HoverBG)
		}

		// Текст по центру вертикали.
		textY := r.Min.Y + (r.Dy()-13)/2
		ctx.DrawText(item.Text, r.Min.X+mb.ItemPaddingX, textY, mb.TextColor)
	}
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
	// Если курсор в полосе — обновляем hover и переключаем подменю.
	if image.Pt(x, y).In(mb.Base.Bounds()) {
		idx := mb.hitTopItem(x, y)
		atomic.StoreInt32(&mb.hoverIdx, int32(idx))

		// Если подменю уже открыто — переключаем при наведении.
		if atomic.LoadInt32(&mb.activeIdx) >= 0 && idx >= 0 && idx != int(atomic.LoadInt32(&mb.activeIdx)) {
			mb.openSubmenu(idx)
		}
		return
	}

	// Иначе — делегируем в popup.
	atomic.StoreInt32(&mb.hoverIdx, -1)
	if mb.popup.IsOpen() {
		mb.popup.OnMouseMove(x, y)
	}
}

func (mb *MenuBar) OnMouseButton(e MouseEvent) bool {
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
			atomic.StoreInt32(&mb.activeIdx, -1)
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
			go item.OnClick()
		}
		return
	}

	atomic.StoreInt32(&mb.activeIdx, int32(idx))

	// Настраиваем popup: копируем пункты подменю, настраиваем OnSelect.
	topIdx := idx
	mb.popup.SetItems(item.Items)
	mb.popup.OnSelect = func(subIdx int, text string) {
		atomic.StoreInt32(&mb.activeIdx, -1)
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
	atomic.StoreInt32(&mb.activeIdx, -1)
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
				atomic.StoreInt32(&mb.activeIdx, -1)
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
