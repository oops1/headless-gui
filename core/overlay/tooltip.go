// tooltip.go — всплывающие подсказки (ToolTip) для виджетов.
//
// Движок отслеживает позицию курсора и время его остановки. Если курсор
// неподвижен дольше задержки (по умолчанию 600мс) над виджетом с непустым
// ToolTip, поверх всего UI рисуется плашка-подсказка. Поведение глобально
// отключаемо через SetTooltipsEnabled.

package overlay

import (
	"image"
	"image/color"
	"time"

	"github.com/oops1/headless-gui/v4/core/input"
	"github.com/oops1/headless-gui/v4/core/raster"
	"github.com/oops1/headless-gui/v4/widget"
)

// SetTooltipsEnabled включает/выключает показ всплывающих подсказок.
func (m *Manager) SetTooltipsEnabled(v bool) {
	m.ttMu.Lock()
	m.ttEnabled = v
	m.ttMu.Unlock()
}

// TooltipsEnabled сообщает, включён ли показ подсказок.
func (m *Manager) TooltipsEnabled() bool {
	m.ttMu.Lock()
	defer m.ttMu.Unlock()
	return m.ttEnabled
}

// SetTooltipDelay задаёт задержку появления подсказки после остановки курсора.
func (m *Manager) SetTooltipDelay(d time.Duration) {
	if d < 0 {
		d = 0
	}
	m.ttMu.Lock()
	m.ttDelay = d
	m.ttMu.Unlock()
}

// RecordMouse сохраняет последнюю позицию курсора и сбрасывает таймер подсказки.
func (m *Manager) RecordMouse(x, y int) {
	m.ttMu.Lock()
	m.ttMouseX = x
	m.ttMouseY = y
	m.ttLastMove = time.Now()
	m.ttHasMouse = true
	m.ttMu.Unlock()
}

// InvalidateShownTooltip инвалидирует область показанной подсказки (стирание
// при движении мыши) и забывает её. No-op, если подсказка не показана.
// Инвалидация гарантирует кадр, на котором область перерисуется без плашки
// (таймер уже сброшен RecordMouse — DrawTooltip её не нарисует).
func (m *Manager) InvalidateShownTooltip() {
	m.ttMu.Lock()
	r := m.ttShownAt
	m.ttShownAt = image.Rectangle{}
	m.ttMu.Unlock()
	if !r.Empty() {
		m.host.DamageRect(r) // дерево под подсказкой не менялось
	}
}

// DrawTooltip рисует всплывающую подсказку, если курсор достаточно долго
// неподвижен над виджетом с непустым ToolTip. Вызывается из renderFrame
// (под frameMu движка) на прямом пути; canvas и root — снапшоты кадра. На пути
// команд подсказка записывается рекордером (toplayers.go).
func (m *Manager) DrawTooltip(c *raster.Canvas, root widget.Widget) {
	if p := m.PlanTooltip(c, root); p.OK {
		m.DrawTooltipBox(c, p.Text, p.Rect)
	}
}

// PlanTooltip — подсказка кадра: текст и плашка (логическая), если она
// показывается.
type TooltipPlan struct {
	Text string
	Rect image.Rectangle
	OK   bool
}

// PlanTooltip решает, показывается ли подсказка, и где.
func (m *Manager) PlanTooltip(c *raster.Canvas, root widget.Widget) TooltipPlan {
	m.ttMu.Lock()
	enabled := m.ttEnabled
	hasMouse := m.ttHasMouse
	mx, my := m.ttMouseX, m.ttMouseY
	elapsed := time.Since(m.ttLastMove)
	delay := m.ttDelay
	m.ttMu.Unlock()

	if !enabled || !hasMouse || elapsed < delay {
		return TooltipPlan{}
	}

	// Корень hit-теста: верхний модальный диалог или корневой виджет.
	if md := m.host.TopModal(); md != nil {
		root = md
	}
	if root == nil {
		return TooltipPlan{}
	}

	tip := TooltipAt(root, mx, my)
	if tip == "" {
		return TooltipPlan{}
	}
	return TooltipPlan{Text: tip, Rect: m.tooltipRect(c, tip, mx, my), OK: true}
}

// TooltipMayAppear — true в МОМЕНТ появления подсказки: курсор остановился и
// задержка только что истекла (окно в пару кадров, чтобы точно успеть её
// нарисовать). Используется on-demand циклом, чтобы не пропустить появление
// tooltip без явной инвалидации.
//
// Важно, что true держится только это короткое окно, а не всё время
// «дозревания»: раньше условие было elapsed <= delay, и КАЖДОЕ движение мыши
// заставляло цикл рендерить полные кадры (фон + всё дерево + полный дифф)
// в течение всей задержки подсказки — постоянная фоновая нагрузка при любом
// шевелении курсора, хотя менялся от силы hover одной кнопки.
func (m *Manager) TooltipMayAppear(frameInterval time.Duration) bool {
	m.ttMu.Lock()
	defer m.ttMu.Unlock()
	if !m.ttEnabled || !m.ttHasMouse {
		return false
	}
	elapsed := time.Since(m.ttLastMove)
	return elapsed >= m.ttDelay && elapsed <= m.ttDelay+2*frameInterval
}

// TooltipAt возвращает ToolTip самого глубокого видимого виджета под (x, y).
//
// Виджет, у разных частей которого разные подсказки (кнопки в заголовке
// панели), отвечает через ToolTipAt(x, y); пустой ответ — спросить его общую
// подсказку.
func TooltipAt(root widget.Widget, x, y int) string {
	path := input.HitPath(root, x, y)
	for i := len(path) - 1; i >= 0; i-- {
		if tp, ok := path[i].W.(interface{ ToolTipAt(x, y int) string }); ok {
			// Точка — в кадре виджета (ребёнок прокрутки судит в координатах
			// содержимого).
			if s := tp.ToolTipAt(x+path[i].Off.X, y+path[i].Off.Y); s != "" {
				return s
			}
		}
		if tp, ok := path[i].W.(interface{ GetToolTip() string }); ok {
			if s := tp.GetToolTip(); s != "" {
				return s
			}
		}
	}
	return ""
}

// DrawTooltipBox рисует плашку подсказки рядом с курсором, не выходя за холст.
// tooltipRect — плашка подсказки для текста у курсора (логические
// координаты): справа-снизу от курсора, с прижатием к краям холста.
func (m *Manager) tooltipRect(c *raster.Canvas, text string, mx, my int) image.Rectangle {
	const padX, padY = 8, 5
	tw := c.MeasureText(text, widget.DefaultFontSize())
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
	return image.Rect(x, y, x+bw, y+bh)
}

// DrawTooltipBox рисует плашку в контекст c: холст (прямой путь, окно back)
// или рекордер команд.
func (m *Manager) DrawTooltipBox(c widget.DrawContext, text string, r image.Rectangle) {
	const padX, padY = 8, 5
	x, y, bw, bh := r.Min.X, r.Min.Y, r.Dx(), r.Dy()

	bg := color.RGBA{R: 45, G: 45, B: 48, A: 240}
	border := color.RGBA{R: 120, G: 120, B: 130, A: 255}
	fg := color.RGBA{R: 240, G: 240, B: 240, A: 255}

	c.FillRoundRect(x, y, bw, bh, 3, bg)
	c.DrawRoundBorder(x, y, bw, bh, 3, border)
	c.DrawText(text, x+padX, y+padY, fg)

	// Запоминаем область плашки — SendMouseMove инвалидирует её для стирания.
	m.ttMu.Lock()
	m.ttShownAt = r
	m.ttMu.Unlock()
}
