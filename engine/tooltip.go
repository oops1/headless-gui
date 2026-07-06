// tooltip.go — всплывающие подсказки (ToolTip) для виджетов.
//
// Движок отслеживает позицию курсора и время его остановки. Если курсор
// неподвижен дольше задержки (по умолчанию 600мс) над виджетом с непустым
// ToolTip, поверх всего UI рисуется плашка-подсказка. Поведение глобально
// отключаемо через SetTooltipsEnabled.
package engine

import (
	"image"
	"image/color"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// SetTooltipsEnabled включает/выключает показ всплывающих подсказок.
func (e *Engine) SetTooltipsEnabled(v bool) {
	e.ttMu.Lock()
	e.ttEnabled = v
	e.ttMu.Unlock()
}

// TooltipsEnabled сообщает, включён ли показ подсказок.
func (e *Engine) TooltipsEnabled() bool {
	e.ttMu.Lock()
	defer e.ttMu.Unlock()
	return e.ttEnabled
}

// SetTooltipDelay задаёт задержку появления подсказки после остановки курсора.
func (e *Engine) SetTooltipDelay(d time.Duration) {
	if d < 0 {
		d = 0
	}
	e.ttMu.Lock()
	e.ttDelay = d
	e.ttMu.Unlock()
}

// recordMouse сохраняет последнюю позицию курсора и сбрасывает таймер подсказки.
func (e *Engine) recordMouse(x, y int) {
	e.ttMu.Lock()
	e.ttMouseX = x
	e.ttMouseY = y
	e.ttLastMove = time.Now()
	e.ttHasMouse = true
	e.ttMu.Unlock()
}

// invalidateShownTooltip инвалидирует область показанной подсказки (стирание
// при движении мыши) и забывает её. No-op, если подсказка не показана.
// Инвалидация гарантирует кадр, на котором область перерисуется без плашки
// (таймер уже сброшен recordMouse — drawTooltip её не нарисует).
func (e *Engine) invalidateShownTooltip() {
	e.ttMu.Lock()
	r := e.ttShownAt
	e.ttShownAt = image.Rectangle{}
	e.ttMu.Unlock()
	if !r.Empty() {
		e.InvalidateRect(r)
	}
}

// drawTooltip рисует всплывающую подсказку, если курсор достаточно долго
// неподвижен над виджетом с непустым ToolTip. Вызывается из renderFrame
// (под e.frameMu); canvas и root — снапшоты кадра.
func (e *Engine) drawTooltip(c *Canvas, root widget.Widget) {
	e.ttMu.Lock()
	enabled := e.ttEnabled
	hasMouse := e.ttHasMouse
	mx, my := e.ttMouseX, e.ttMouseY
	elapsed := time.Since(e.ttLastMove)
	delay := e.ttDelay
	e.ttMu.Unlock()

	if !enabled || !hasMouse || elapsed < delay {
		return
	}

	// Корень hit-теста: верхний модальный диалог или корневой виджет.
	if m := e.topModal(); m != nil {
		root = m
	}
	if root == nil {
		return
	}

	tip := tooltipAt(root, mx, my)
	if tip == "" {
		return
	}
	e.drawTooltipBox(c, tip, mx, my)
}

// tooltipMayAppear — true, пока подсказка «дозревает»: курсор остановился,
// задержка ещё не истекла (плюс один кадр сверху, чтобы успеть нарисовать
// появившуюся подсказку). Используется on-demand циклом, чтобы не пропустить
// момент появления tooltip без явной инвалидации.
func (e *Engine) tooltipMayAppear(frameInterval time.Duration) bool {
	e.ttMu.Lock()
	defer e.ttMu.Unlock()
	if !e.ttEnabled || !e.ttHasMouse {
		return false
	}
	return time.Since(e.ttLastMove) <= e.ttDelay+frameInterval
}

// tooltipAt возвращает ToolTip самого глубокого видимого виджета под (x, y).
func tooltipAt(root widget.Widget, x, y int) string {
	path := hitTestPath(root, x, y)
	for i := len(path) - 1; i >= 0; i-- {
		if tp, ok := path[i].(interface{ GetToolTip() string }); ok {
			if s := tp.GetToolTip(); s != "" {
				return s
			}
		}
	}
	return ""
}

// drawTooltipBox рисует плашку подсказки рядом с курсором, не выходя за холст.
func (e *Engine) drawTooltipBox(c *Canvas, text string, mx, my int) {
	const padX, padY = 8, 5
	tw := c.MeasureText(text, DefaultFontSize)
	bw := tw + padX*2
	bh := 13 + padY*2

	lw, lh := c.LogicalSize()
	x := mx + 12
	y := my + 20
	if x+bw > lw {
		x = lw - bw
	}
	if x < 0 {
		x = 0
	}
	if y+bh > lh {
		y = my - bh - 6 // показываем над курсором, если снизу не помещается
	}
	if y < 0 {
		y = 0
	}

	bg := color.RGBA{R: 45, G: 45, B: 48, A: 240}
	border := color.RGBA{R: 120, G: 120, B: 130, A: 255}
	fg := color.RGBA{R: 240, G: 240, B: 240, A: 255}

	c.FillRoundRect(x, y, bw, bh, 3, bg)
	c.DrawRoundBorder(x, y, bw, bh, 3, border)
	c.DrawText(text, x+padX, y+padY, fg)

	// Запоминаем область плашки — SendMouseMove инвалидирует её для стирания.
	e.ttMu.Lock()
	e.ttShownAt = image.Rect(x, y, x+bw, y+bh)
	e.ttMu.Unlock()
}
