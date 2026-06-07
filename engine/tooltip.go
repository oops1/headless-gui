// tooltip.go — всплывающие подсказки (ToolTip) для виджетов.
//
// Движок отслеживает позицию курсора и время его остановки. Если курсор
// неподвижен дольше задержки (по умолчанию 600мс) над виджетом с непустым
// ToolTip, поверх всего UI рисуется плашка-подсказка. Поведение глобально
// отключаемо через SetTooltipsEnabled.
package engine

import (
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

// drawTooltip рисует всплывающую подсказку, если курсор достаточно долго
// неподвижен над виджетом с непустым ToolTip. Вызывается из renderFrame
// (под e.mu.RLock) — поэтому НЕ берёт e.mu повторно.
func (e *Engine) drawTooltip() {
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
	var root widget.Widget
	if m := e.topModal(); m != nil {
		root = m
	} else {
		root = e.root
	}
	if root == nil {
		return
	}

	tip := tooltipAt(root, mx, my)
	if tip == "" {
		return
	}
	e.drawTooltipBox(tip, mx, my)
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
func (e *Engine) drawTooltipBox(text string, mx, my int) {
	c := e.canvas
	const padX, padY = 8, 5
	tw := c.MeasureText(text, DefaultFontSize)
	bw := tw + padX*2
	bh := 13 + padY*2

	x := mx + 12
	y := my + 20
	if x+bw > c.W {
		x = c.W - bw
	}
	if x < 0 {
		x = 0
	}
	if y+bh > c.H {
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
}
