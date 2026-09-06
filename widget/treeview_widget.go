// Package widget — виджет-обёртка TreeView для интеграции в дерево виджетов.
//
// TreeViewWidget оборачивает treeview.TreeView, реализуя интерфейсы:
//   - Widget (Draw, Bounds, SetBounds, Children, AddChild)
//   - MouseClickHandler, MouseMoveHandler, KeyHandler
//   - Focusable
//   - Themeable (ApplyTheme)
//   - ScrollHandler (ScrollBy)
package widget

import (
	"image"
	"image/color"

	"github.com/oops1/headless-gui/v3/widget/treeview"
)

// ─── DrawContextAdapter ────────────────────────────────────────────────────

// treeViewDrawAdapter адаптирует widget.DrawContext → treeview.DrawContextBridge.
type treeViewDrawAdapter struct {
	ctx DrawContext
}

func (a *treeViewDrawAdapter) FillRect(x, y, w, h int, col color.RGBA) {
	a.ctx.FillRect(x, y, w, h, col)
}
func (a *treeViewDrawAdapter) FillRectAlpha(x, y, w, h int, col color.RGBA) {
	a.ctx.FillRectAlpha(x, y, w, h, col)
}
func (a *treeViewDrawAdapter) DrawBorder(x, y, w, h int, col color.RGBA) {
	a.ctx.DrawBorder(x, y, w, h, col)
}
func (a *treeViewDrawAdapter) DrawText(text string, x, y int, col color.RGBA) {
	a.ctx.DrawText(text, x, y, col)
}
func (a *treeViewDrawAdapter) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	a.ctx.DrawTextSize(text, x, y, sizePt, col)
}
func (a *treeViewDrawAdapter) MeasureText(text string, sizePt float64) int {
	return a.ctx.MeasureText(text, sizePt)
}
func (a *treeViewDrawAdapter) SetClip(r image.Rectangle) {
	a.ctx.SetClip(r)
}
func (a *treeViewDrawAdapter) ClearClip() {
	a.ctx.ClearClip()
}
func (a *treeViewDrawAdapter) DrawHLine(x, y, length int, col color.RGBA) {
	a.ctx.DrawHLine(x, y, length, col)
}
func (a *treeViewDrawAdapter) DrawVLine(x, y, length int, col color.RGBA) {
	a.ctx.DrawVLine(x, y, length, col)
}
func (a *treeViewDrawAdapter) DrawImageScaled(src image.Image, x, y, w, h int) {
	a.ctx.DrawImageScaled(src, x, y, w, h)
}
func (a *treeViewDrawAdapter) SetPixel(x, y int, col color.RGBA) {
	a.ctx.SetPixel(x, y, col)
}
func (a *treeViewDrawAdapter) DrawTextBold(text string, x, y int, sizePt float64, col color.RGBA) {
	a.ctx.DrawTextFont(text, x, y, sizePt, BuiltinFontBold, col)
}
func (a *treeViewDrawAdapter) MeasureTextBold(text string, sizePt float64) int {
	return a.ctx.MeasureTextFont(text, sizePt, BuiltinFontBold)
}

// ─── TreeViewWidget ────────────────────────────────────────────────────────

// TreeViewWidget — виджет-обёртка для интеграции treeview.TreeView в дерево виджетов.
type TreeViewWidget struct {
	Base
	Tree *treeview.TreeView

	// NodeContextMenu собирает контекстное меню для узла под курсором.
	//
	// nil-узел означает щелчок мимо узлов (пустое место под последней
	// строкой) — приложение может вернуть общее меню дерева или nil. Пустой
	// список означает «меню здесь нет»: движок пойдёт искать его выше по
	// дереву, как и с обычным ContextMenu.
	//
	// Выделение при правом щелчке НЕ меняется: узел приходит в колбэк явным
	// аргументом, а менять выбор пользователя за его спиной движок не вправе.
	NodeContextMenu func(item *treeview.TreeViewItem) []MenuItem

	// nodeMenu — меню, отданное движку из ContextMenuAt: рисует его виджет,
	// которому оно принадлежит (см. contextmenuat.go).
	nodeMenu rowMenuHost
}

// ContextMenuAt собирает меню для узла под точкой (ContextMenuProvider).
func (w *TreeViewWidget) ContextMenuAt(x, y int) *PopupMenu {
	if !w.IsEnabled() || w.NodeContextMenu == nil {
		return nil
	}
	if !image.Pt(x, y).In(w.Bounds()) {
		return nil
	}
	return w.nodeMenu.build(w.NodeContextMenu(w.Tree.ItemAtY(y)))
}

// NewTreeViewWidget создаёт виджет TreeView.
func NewTreeViewWidget() *TreeViewWidget {
	tw := &TreeViewWidget{
		Tree: treeview.New(),
	}
	return tw
}

// SetBounds обновляет bounds виджета и вложенного TreeView.
func (w *TreeViewWidget) SetBounds(r image.Rectangle) {
	w.Base.SetBounds(r)
	w.Tree.SetBounds(r)
}

// Draw отрисовывает TreeView.
func (w *TreeViewWidget) Draw(ctx DrawContext) {
	adapter := &treeViewDrawAdapter{ctx: ctx}
	w.Tree.Draw(adapter)
	w.drawDisabledOverlay(ctx)
}

// ─── Overlay (контекстное меню узла) ───────────────────────────────────────

// HasOverlay реализует OverlayDrawer.
func (w *TreeViewWidget) HasOverlay() bool { return w.nodeMenu.open() }

// DrawOverlay рисует контекстное меню узла поверх всего UI.
func (w *TreeViewWidget) DrawOverlay(ctx DrawContext) { w.nodeMenu.drawOverlay(ctx) }

// OverlayBounds отдаёт прямоугольник открытого меню (для выноса в окно ОС).
func (w *TreeViewWidget) OverlayBounds() image.Rectangle { return w.nodeMenu.overlayBounds() }

// Dismiss закрывает контекстное меню узла. Реализует Dismissable.
func (w *TreeViewWidget) Dismiss() { w.nodeMenu.dismiss() }

// applyDirty забирает у ядра накопленный damage и транслирует его в точечную
// инвалидацию: full → весь виджет, иначе — только изменившиеся строки.
func (w *TreeViewWidget) applyDirty() {
	rects, full := w.Tree.TakeDirty()
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
func (w *TreeViewWidget) OnMouseButton(e MouseEvent) bool {
	if !w.IsEnabled() {
		return false
	}

	// Открытое меню узла старше самого дерева: щелчок по пункту не должен
	// доходить до узлов под ним.
	if w.nodeMenu.routeMouse(e) {
		return true
	}

	// Колесо мыши — вертикальная прокрутка на 3 строки за тик.
	// Поглощаем событие ТОЛЬКО когда прокрутка реально сдвинулась; если
	// содержимое помещается или мы уже у границы — возвращаем false,
	// чтобы колесо всплыло к родительскому ScrollView.
	if e.Button == MouseWheelUp || e.Button == MouseWheelDown {
		if !e.Pressed || !image.Pt(e.X, e.Y).In(w.Bounds()) {
			return false
		}
		moved := w.Tree.WheelScroll(e.Button == MouseWheelUp)
		if moved {
			w.Invalidate()
		}
		return moved
	}

	if e.Button != MouseLeft {
		return false
	}

	pressed := 0
	if e.Pressed {
		pressed = 1
	}
	consumed := w.Tree.OnMouseButton(e.X, e.Y, int(e.Button), pressed)
	// Точечная инвалидация: выбор/hover строки — только строки, разворот
	// узла/скролл — весь виджет (см. ядро treeview).
	w.applyDirty()
	return consumed
}

// OnMouseMove обрабатывает перемещение.
func (w *TreeViewWidget) OnMouseMove(x, y int) {
	if !w.IsEnabled() {
		return
	}
	if w.nodeMenu.routeMove(x, y) {
		return // курсор над меню — подсветку узлов не трогаем
	}
	w.Tree.OnMouseMove(x, y)
	// Ядро сообщает точную область смены hover-строки/ползунка.
	w.applyDirty()
}

// ─── Keyboard handling ─────────────────────────────────────────────────────

// OnKeyEvent обрабатывает клавиатурный ввод.
func (w *TreeViewWidget) OnKeyEvent(e KeyEvent) {
	if !w.IsEnabled() {
		return
	}
	if w.nodeMenu.routeKey(e) {
		return
	}

	shift := e.Mod&ModShift != 0
	ctrl := e.Mod&ModCtrl != 0

	w.Tree.OnKeyEvent(int(e.Code), e.Rune, e.Pressed, shift, ctrl)
	// Навигация — только строки, разворачивание/скролл — весь виджет.
	w.applyDirty()
}

// ─── Focus ─────────────────────────────────────────────────────────────────

// SetFocused реализует Focusable.
func (w *TreeViewWidget) SetFocused(v bool) {
	changed := w.Tree.IsFocused() != v
	w.Tree.SetFocused(v)
	if changed {
		w.Invalidate() // рамка фокуса — в bounds
	}
}

// IsFocused реализует Focusable.
func (w *TreeViewWidget) IsFocused() bool {
	return w.Tree.IsFocused()
}

// ─── Scroll ────────────────────────────────────────────────────────────────

// ScrollBy прокручивает TreeView на delta пикселей (для колеса мыши).
func (w *TreeViewWidget) ScrollBy(delta int) {
	w.Tree.ScrollBy(delta)
	w.Invalidate() // Tree не сообщает о клампе — инвалидируем всегда
}

// ─── Theme ─────────────────────────────────────────────────────────────────

// ApplyTheme обновляет цвета TreeView из глобальной темы.
func (w *TreeViewWidget) ApplyTheme(t *Theme) {
	w.Tree.ApplyTheme(&treeview.TreeViewTheme{
		Background:       t.WindowBG,
		Foreground:       t.TreeText,
		ArrowColor:       t.TreeArrow,
		SelectColor:      t.ListItemSelect,
		HoverColor:       t.ListItemHover,
		FocusBorderColor: t.Accent,
		ScrollTrackBG:    t.ScrollTrackBG,
		ScrollThumbBG:    t.ScrollThumbBG,
		ScrollThumbHover: t.Accent,
		IndentGuideColor: t.Border,
	})
}

// ─── Backward-compat convenience methods ──────────────────────────────────

// AddRoot добавляет корневой узел (обратная совместимость).
func (w *TreeViewWidget) AddRoot(n *treeview.TreeViewItem) {
	w.Tree.AddRoot(n)
}

// ClearRoots удаляет все корневые узлы (обратная совместимость).
func (w *TreeViewWidget) ClearRoots() {
	w.Tree.ClearRoots()
}

// SelectedNode возвращает текущий выделенный узел (обратная совместимость).
func (w *TreeViewWidget) SelectedNode() *treeview.TreeViewItem {
	return w.Tree.SelectedItem()
}

// BeginUpdate приостанавливает отрисовку дерева (двойная буферизация).
func (w *TreeViewWidget) BeginUpdate() {
	w.Tree.BeginUpdate()
}

// EndUpdate возобновляет отрисовку дерева.
func (w *TreeViewWidget) EndUpdate() {
	w.Tree.EndUpdate()
}
