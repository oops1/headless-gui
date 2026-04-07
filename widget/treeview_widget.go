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

// ─── TreeViewWidget ────────────────────────────────────────────────────────

// TreeViewWidget — виджет-обёртка для интеграции treeview.TreeView в дерево виджетов.
type TreeViewWidget struct {
	Base
	Tree *treeview.TreeView
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

// ─── Mouse handling ────────────────────────────────────────────────────────

// OnMouseButton обрабатывает клики.
func (w *TreeViewWidget) OnMouseButton(e MouseEvent) bool {
	if !w.IsEnabled() {
		return false
	}
	if e.Button != MouseLeft {
		return false
	}

	pressed := 0
	if e.Pressed {
		pressed = 1
	}
	return w.Tree.OnMouseButton(e.X, e.Y, int(e.Button), pressed)
}

// OnMouseMove обрабатывает перемещение.
func (w *TreeViewWidget) OnMouseMove(x, y int) {
	if !w.IsEnabled() {
		return
	}
	w.Tree.OnMouseMove(x, y)
}

// ─── Keyboard handling ─────────────────────────────────────────────────────

// OnKeyEvent обрабатывает клавиатурный ввод.
func (w *TreeViewWidget) OnKeyEvent(e KeyEvent) {
	if !w.IsEnabled() {
		return
	}

	shift := e.Mod&ModShift != 0
	ctrl := e.Mod&ModCtrl != 0

	w.Tree.OnKeyEvent(int(e.Code), e.Rune, e.Pressed, shift, ctrl)
}

// ─── Focus ─────────────────────────────────────────────────────────────────

// SetFocused реализует Focusable.
func (w *TreeViewWidget) SetFocused(v bool) {
	w.Tree.SetFocused(v)
}

// IsFocused реализует Focusable.
func (w *TreeViewWidget) IsFocused() bool {
	return w.Tree.IsFocused()
}

// ─── Scroll ────────────────────────────────────────────────────────────────

// ScrollBy прокручивает TreeView на delta пикселей (для колеса мыши).
func (w *TreeViewWidget) ScrollBy(delta int) {
	w.Tree.ScrollBy(delta)
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
