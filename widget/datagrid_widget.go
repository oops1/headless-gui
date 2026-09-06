// Package widget — виджет-обёртка DataGrid для интеграции в дерево виджетов.
//
// DataGridWidget оборачивает datagrid.DataGrid, реализуя интерфейсы:
//   - Widget (Draw, Bounds, SetBounds, Children, AddChild)
//   - MouseClickHandler, MouseMoveHandler, KeyHandler
//   - Focusable
//   - Themeable (ApplyTheme)
package widget

import (
	"image"
	"image/color"
	"time"

	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

// ─── DrawContextAdapter ────────────────────────────────────────────────────

// drawContextAdapter адаптирует widget.DrawContext → datagrid.DrawContextBridge.
type drawContextAdapter struct {
	ctx DrawContext
}

func (a *drawContextAdapter) FillRect(x, y, w, h int, col color.RGBA) {
	a.ctx.FillRect(x, y, w, h, col)
}
func (a *drawContextAdapter) FillRectAlpha(x, y, w, h int, col color.RGBA) {
	a.ctx.FillRectAlpha(x, y, w, h, col)
}
func (a *drawContextAdapter) DrawBorder(x, y, w, h int, col color.RGBA) {
	a.ctx.DrawBorder(x, y, w, h, col)
}
func (a *drawContextAdapter) DrawText(text string, x, y int, col color.RGBA) {
	a.ctx.DrawText(text, x, y, col)
}
func (a *drawContextAdapter) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	a.ctx.DrawTextSize(text, x, y, sizePt, col)
}
func (a *drawContextAdapter) MeasureText(text string, sizePt float64) int {
	return a.ctx.MeasureText(text, sizePt)
}
func (a *drawContextAdapter) SetClip(r image.Rectangle) {
	a.ctx.SetClip(r)
}
func (a *drawContextAdapter) ClearClip() {
	a.ctx.ClearClip()
}
func (a *drawContextAdapter) DrawHLine(x, y, length int, col color.RGBA) {
	a.ctx.DrawHLine(x, y, length, col)
}
func (a *drawContextAdapter) DrawVLine(x, y, length int, col color.RGBA) {
	a.ctx.DrawVLine(x, y, length, col)
}
func (a *drawContextAdapter) DrawImage(src image.Image, x, y int) {
	a.ctx.DrawImage(src, x, y)
}
func (a *drawContextAdapter) DrawImageScaled(src image.Image, x, y, w, h int) {
	a.ctx.DrawImageScaled(src, x, y, w, h)
}

// ─── DataGridWidget ────────────────────────────────────────────────────────

// DataGridWidget — виджет-обёртка для интеграции datagrid.DataGrid в дерево виджетов.
type DataGridWidget struct {
	Base
	Grid *datagrid.DataGrid

	// RowContextMenu собирает контекстное меню для строки под курсором.
	//
	// row == -1 означает щелчок мимо строк (заголовок, пустое место под
	// последней строкой) — приложение может вернуть общее меню таблицы или
	// nil. Пустой список означает «меню здесь нет»: движок пойдёт искать его
	// выше по дереву, как и с обычным ContextMenu.
	//
	// Выделение при правом щелчке НЕ меняется: строка приходит в колбэк
	// явным аргументом, а менять выбор пользователя за его спиной движок не
	// вправе. Приложению, которому нужно поведение проводника, достаточно
	// позвать SetSelectedIndex(row) в самом колбэке.
	RowContextMenu func(item interface{}, row int) []MenuItem

	// rowMenu — меню, отданное движку из ContextMenuAt: рисует его виджет,
	// которому оно принадлежит (см. contextmenuat.go).
	rowMenu rowMenuHost

	// Для обработки двойного клика
	lastClickTime int64 // ms
	lastClickX    int
	lastClickY    int
}

// NewDataGridWidget создаёт виджет DataGrid.
func NewDataGridWidget() *DataGridWidget {
	dg := &DataGridWidget{
		Grid: datagrid.New(),
	}
	return dg
}

// SetBounds обновляет bounds виджета и вложенного DataGrid.
func (w *DataGridWidget) SetBounds(r image.Rectangle) {
	w.Base.SetBounds(r)
	w.Grid.SetBounds(r)
}

// Draw отрисовывает DataGrid.
func (w *DataGridWidget) Draw(ctx DrawContext) {
	adapter := &drawContextAdapter{ctx: ctx}
	w.Grid.Draw(adapter)
	w.drawDisabledOverlay(ctx)
}

// ─── Overlay (контекстное меню строки) ─────────────────────────────────────

// HasOverlay реализует OverlayDrawer.
func (w *DataGridWidget) HasOverlay() bool { return w.rowMenu.open() }

// DrawOverlay рисует контекстное меню строки поверх всего UI.
func (w *DataGridWidget) DrawOverlay(ctx DrawContext) { w.rowMenu.drawOverlay(ctx) }

// OverlayBounds отдаёт прямоугольник открытого меню (для выноса в окно ОС).
func (w *DataGridWidget) OverlayBounds() image.Rectangle { return w.rowMenu.overlayBounds() }

// Dismiss закрывает контекстное меню строки. Реализует Dismissable.
func (w *DataGridWidget) Dismiss() { w.rowMenu.dismiss() }

// applyDirty забирает у ядра накопленный damage и транслирует его в точечную
// инвалидацию: full → весь виджет, иначе — только изменившиеся строки
// (notifyRectChanged на каждый прямоугольник). Так выбор/hover строки не
// перерисовывает всю таблицу.
func (w *DataGridWidget) applyDirty() {
	rects, full := w.Grid.TakeDirty()
	if full {
		w.Invalidate()
		return
	}
	for _, r := range rects {
		notifyRectChanged(r)
	}
}

// ─── Mouse handling ────────────────────────────────────────────────────────

// OnMouseButton обрабатывает клики.
func (w *DataGridWidget) OnMouseButton(e MouseEvent) bool {
	if !w.IsEnabled() {
		return false
	}

	// Открытое меню строки старше самой таблицы: щелчок по пункту не должен
	// доходить до строк под ним.
	if w.rowMenu.routeMouse(e) {
		return true
	}

	// Колесо мыши — вертикальная прокрутка строк на 3 строки за тик.
	// Поглощаем событие ТОЛЬКО когда прокрутка реально сдвинулась; если
	// строки помещаются или мы уже у границы — возвращаем false, чтобы
	// колесо всплыло к родительскому ScrollView.
	if e.Button == MouseWheelUp || e.Button == MouseWheelDown {
		if !e.Pressed || !image.Pt(e.X, e.Y).In(w.Bounds()) {
			return false
		}
		moved := w.Grid.WheelScroll(e.Button == MouseWheelUp)
		if moved {
			w.Invalidate()
		}
		return moved
	}

	if e.Button != MouseLeft {
		return false
	}

	consumed := w.Grid.OnMouseButtonMod(e.X, e.Y, int(e.Button), e.Pressed,
		e.Mod&ModShift != 0, e.Mod&ModCtrl != 0)

	// Детекция двойного клика (упрощённая)
	if e.Pressed && consumed {
		now := timeNowMs()
		dx := e.X - w.lastClickX
		dy := e.Y - w.lastClickY
		if now-w.lastClickTime < 400 && dx*dx+dy*dy < 25 {
			w.Grid.OnMouseDoubleClick(e.X, e.Y)
		}
		w.lastClickTime = now
		w.lastClickX = e.X
		w.lastClickY = e.Y
	}

	// Точечная инвалидация: выбор/hover строки перерисовывает только строки,
	// скролл/сортировка/resize — весь виджет (см. ядро datagrid).
	w.applyDirty()
	return consumed
}

// OnMouseMove обрабатывает перемещение.
func (w *DataGridWidget) OnMouseMove(x, y int) {
	if !w.IsEnabled() {
		return
	}
	if w.rowMenu.routeMove(x, y) {
		return // курсор над меню — подсветку строк не трогаем
	}
	w.Grid.OnMouseMove(x, y)
	// Ядро сообщает точную область смены hover-строки/ползунка — транслируем
	// её в точечную инвалидацию вместо перерисовки всего виджета.
	w.applyDirty()
}

// GetToolTip возвращает подсказку строки под курсором, если приложение задало
// RowToolTip, иначе — общий ToolTip виджета.
//
// Без этого строке нельзя было дать свой текст: Base.ToolTip один на весь
// виджет, и обёртке приходилось пересчитывать индекс строки на каждое
// движение мыши и звать SetToolTip.
func (w *DataGridWidget) GetToolTip() string {
	if tip := w.Grid.HoverRowToolTip(); tip != "" {
		return tip
	}
	return w.Base.GetToolTip()
}

// ContextMenuAt собирает меню для строки под точкой (ContextMenuProvider).
func (w *DataGridWidget) ContextMenuAt(x, y int) *PopupMenu {
	if !w.IsEnabled() || w.RowContextMenu == nil {
		return nil
	}
	if !image.Pt(x, y).In(w.Bounds()) {
		return nil
	}
	row := w.Grid.RowIndexAtY(y)
	var item interface{}
	if row >= 0 {
		item = w.Grid.ItemAtRow(row)
	}
	return w.rowMenu.build(x, y, w.RowContextMenu(item, row))
}

// ─── Keyboard handling ─────────────────────────────────────────────────────

// OnKeyEvent обрабатывает клавиатурный ввод.
func (w *DataGridWidget) OnKeyEvent(e KeyEvent) {
	if !w.IsEnabled() {
		return
	}
	if w.rowMenu.routeKey(e) {
		return
	}

	shift := e.Mod&ModShift != 0
	ctrl := e.Mod&ModCtrl != 0

	w.Grid.OnKeyEvent(int(e.Code), e.Rune, e.Pressed, shift, ctrl)
	// Навигация/редактирование сообщают точную область (строка/вьюпорт).
	w.applyDirty()
}

// ─── Focus ─────────────────────────────────────────────────────────────────

// SetFocused реализует Focusable.
func (w *DataGridWidget) SetFocused(v bool) {
	changed := w.Grid.IsFocused() != v
	w.Grid.SetFocused(v)
	if changed {
		w.Invalidate() // рамка фокуса — в bounds
	}
}

// IsFocused реализует Focusable.
func (w *DataGridWidget) IsFocused() bool {
	return w.Grid.IsFocused()
}

// NeedsAnimation — каретка мигает в режиме редактирования ячейки (Animated).
func (w *DataGridWidget) NeedsAnimation() bool {
	return w.Grid.IsEditing()
}

// ─── Scroll ────────────────────────────────────────────────────────────────

// ScrollBy прокручивает DataGrid на delta пикселей (для колеса мыши).
func (w *DataGridWidget) ScrollBy(delta int) {
	w.Grid.ScrollBy(delta)
	w.Invalidate() // Grid не сообщает о клампе — инвалидируем всегда
}

// ─── Theme ─────────────────────────────────────────────────────────────────

// ApplyTheme обновляет цвета DataGrid из глобальной темы.
func (w *DataGridWidget) ApplyTheme(t *Theme) {
	w.Grid.ApplyTheme(&datagrid.DataGridTheme{
		Background:       t.WindowBG,
		HeaderBG:         t.HeaderBG,
		HeaderText:       t.HeaderText,
		TextColor:        t.LabelText,
		BorderColor:      t.Border,
		SelectColor:      t.ListItemSelect,
		HoverColor:       t.ListItemHover,
		AlternateBG:      alternateBG(t.WindowBG),
		GridLineColor:    t.Border,
		ScrollTrackBG:    t.ScrollTrackBG,
		ScrollThumbBG:    t.ScrollThumbBG,
		ScrollThumbHover: t.Accent,
		EditBG:           t.InputBG,
		EditBorder:       t.InputFocus,
	})
}

// alternateBG создаёт слегка изменённый фон для чередования строк.
func alternateBG(bg color.RGBA) color.RGBA {
	delta := int16(7)
	r := clampByte(int16(bg.R) + delta)
	g := clampByte(int16(bg.G) + delta)
	b := clampByte(int16(bg.B) + delta)
	return color.RGBA{R: r, G: g, B: b, A: bg.A}
}

func clampByte(v int16) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// ─── Helpers ───────────────────────────────────────────────────────────────

// timeNowMs возвращает текущее время в миллисекундах.
func timeNowMs() int64 {
	return time.Now().UnixMilli()
}
