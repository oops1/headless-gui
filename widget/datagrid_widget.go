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

// ─── DataGridWidget ────────────────────────────────────────────────────────

// DataGridWidget — виджет-обёртка для интеграции datagrid.DataGrid в дерево виджетов.
type DataGridWidget struct {
	Base
	Grid *datagrid.DataGrid

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

// ─── Mouse handling ────────────────────────────────────────────────────────

// OnMouseButton обрабатывает клики.
func (w *DataGridWidget) OnMouseButton(e MouseEvent) bool {
	if !w.IsEnabled() {
		return false
	}
	if e.Button != MouseLeft {
		return false
	}

	consumed := w.Grid.OnMouseButton(e.X, e.Y, int(e.Button), e.Pressed)

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

	return consumed
}

// OnMouseMove обрабатывает перемещение.
func (w *DataGridWidget) OnMouseMove(x, y int) {
	if !w.IsEnabled() {
		return
	}
	w.Grid.OnMouseMove(x, y)
}

// ─── Keyboard handling ─────────────────────────────────────────────────────

// OnKeyEvent обрабатывает клавиатурный ввод.
func (w *DataGridWidget) OnKeyEvent(e KeyEvent) {
	if !w.IsEnabled() {
		return
	}

	shift := e.Mod&ModShift != 0
	ctrl := e.Mod&ModCtrl != 0

	w.Grid.OnKeyEvent(int(e.Code), e.Rune, e.Pressed, shift, ctrl)
}

// ─── Focus ─────────────────────────────────────────────────────────────────

// SetFocused реализует Focusable.
func (w *DataGridWidget) SetFocused(v bool) {
	w.Grid.SetFocused(v)
}

// IsFocused реализует Focusable.
func (w *DataGridWidget) IsFocused() bool {
	return w.Grid.IsFocused()
}

// ─── Scroll ────────────────────────────────────────────────────────────────

// ScrollBy прокручивает DataGrid на delta пикселей (для колеса мыши).
func (w *DataGridWidget) ScrollBy(delta int) {
	w.Grid.ScrollBy(delta)
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
