// applicationarea.go — область приложений: закреплённые вперемешку с
// запущенными окнами.
//
// Отличие от RunningApplications в том, что показываются не только открытые
// окна, но и закреплённые приложения, которые сейчас не запущены: щелчок по
// такому запускает его, щелчок по запущенному — переключает окно. Именно так
// устроена панель задач начиная с Windows 7 и док macOS.
//
// Компонент, как и RunningApplications, отдаёт отрисовку презентеру темы,
// если та его назначила: под macOS это тот же док. Поведение при этом
// остаётся здесь и одинаково для всех тем.
package desktop

import (
	"image"
	"sync"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ApplicationArea — область закреплённых и запущенных приложений.
type ApplicationArea struct {
	widget.Base

	tm  *theme.Manager
	cat AppCatalog
	wm  WindowModel

	mu       sync.RWMutex
	entries  []appEntry
	rects    []image.Rectangle
	hoverIdx int
	armedIdx int

	unsubCat func()
	unsubWM  func()
}

// appEntry — одна ячейка области: либо окно, либо закреплённое приложение.
//
// Закреплённое приложение, у которого есть открытое окно, показывается ОДИН
// раз — окном: две ячейки на одно и то же приложение сбивают с толку, а
// щелчок по ним делает разное.
type appEntry struct {
	app    AppID
	title  string
	icon   image.Image
	window WindowID
	live   bool // есть открытое окно
	active bool
	min    bool
}

// NewApplicationArea создаёт область приложений: закреплённые берутся из cat,
// открытые окна — из wm. Подписывается на оба источника; отписка — в Close.
func NewApplicationArea(tm *theme.Manager, cat AppCatalog, wm WindowModel) *ApplicationArea {
	a := &ApplicationArea{tm: tm, cat: cat, wm: wm, hoverIdx: -1, armedIdx: -1}
	if cat != nil {
		if s, ok := cat.(interface{ Subscribe(func()) func() }); ok {
			a.unsubCat = s.Subscribe(a.refresh)
		}
	}
	if wm != nil {
		a.unsubWM = wm.Subscribe(a.refresh)
	}
	a.rebuild()
	return a
}

// Close отписывается от источников. Без него закрытая область продолжала бы
// просыпаться на каждое открытие и закрытие любого окна в системе.
func (a *ApplicationArea) Close() {
	if a.unsubCat != nil {
		a.unsubCat()
		a.unsubCat = nil
	}
	if a.unsubWM != nil {
		a.unsubWM()
		a.unsubWM = nil
	}
}

// refresh перестраивает содержимое и перерисовывает область.
func (a *ApplicationArea) refresh() {
	a.rebuild()
	a.layout()
	a.Invalidate()
}

// rebuild собирает ячейки: сначала закреплённые (в порядке закрепления),
// затем окна незакреплённых приложений.
func (a *ApplicationArea) rebuild() {
	var entries []appEntry

	windows := map[AppID]WindowInfo{}
	var order []WindowInfo
	if a.wm != nil {
		order = a.wm.Windows()
		for _, w := range order {
			if _, seen := windows[w.AppID]; !seen && w.AppID != "" {
				windows[w.AppID] = w
			}
		}
	}

	pinned := map[AppID]bool{}
	if a.cat != nil {
		apps := map[AppID]AppInfo{}
		for _, app := range a.cat.Apps() {
			apps[app.ID] = app
		}
		for _, id := range a.cat.Pinned() {
			pinned[id] = true
			e := appEntry{app: id, title: string(id)}
			if info, ok := apps[id]; ok {
				e.title, e.icon = info.Title, info.Icon
			}
			if w, ok := windows[id]; ok {
				e.window, e.live, e.active, e.min = w.ID, true, w.Active, w.Minimized
				if w.Icon != nil {
					e.icon = w.Icon
				}
			}
			entries = append(entries, e)
		}
	}

	for _, w := range order {
		if w.AppID != "" && pinned[w.AppID] {
			continue // уже показано закреплённой ячейкой
		}
		entries = append(entries, appEntry{
			app: w.AppID, title: w.Title, icon: w.Icon,
			window: w.ID, live: true, active: w.Active, min: w.Minimized,
		})
	}

	a.mu.Lock()
	a.entries = entries
	a.mu.Unlock()
}

// SetBounds задаёт границы области и раскладывает ячейки.
func (a *ApplicationArea) SetBounds(r image.Rectangle) {
	a.Base.SetBounds(r)
	a.layout()
}

// PreferredSize — как у полосы кнопок окон; презентеру темы, если он есть,
// размер решать самому.
func (a *ApplicationArea) PreferredSize(avail image.Point) image.Point {
	if p := a.presenter(); p != nil {
		return p.Measure(a, avail)
	}
	n := len(a.Cells())
	if n == 0 {
		return image.Point{}
	}
	ideal := int(a.metric(KeyTaskButtonWidth))
	if a.tm != nil && !a.tm.GetFlag(KeyTaskButtonLabel, true) {
		st := styleOf(a.tm, ComponentTaskButton, "", theme.StateNormal)
		ideal = int(a.metric(KeyTaskButtonIconSize)) + 2*int(st.PadX)
	}
	gap := int(a.metric(KeyTaskButtonGap))
	want := n*ideal + gap*(n-1)
	if avail.X > 0 && want > avail.X {
		want = avail.X
	}
	return image.Pt(want, avail.Y)
}

// layout считает прямоугольники ячеек: у презентера — его раскладку, иначе
// ряд равных кнопок, сжимающихся до значков.
func (a *ApplicationArea) layout() {
	b := a.Bounds()

	a.mu.Lock()
	a.rects = nil
	n := len(a.entries)
	a.mu.Unlock()
	if b.Empty() || n == 0 {
		return
	}

	// Презентер вызывается БЕЗ замка: он спрашивает у нас же Cells() и
	// HoverIndex(), а взять тот же замок повторно нельзя — это заклинит
	// раскладку намертво.
	if p := a.presenter(); p != nil {
		rects := p.Layout(a, b)
		a.mu.Lock()
		a.rects = rects
		a.mu.Unlock()
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	gap := int(a.metric(KeyTaskButtonGap))
	ideal := int(a.metric(KeyTaskButtonWidth))
	min := int(a.metric(KeyTaskButtonMinWidth))
	// Тема без подписей хочет кнопку со значок, а не полноразмерную.
	if a.tm != nil && !a.tm.GetFlag(KeyTaskButtonLabel, true) {
		st := styleOf(a.tm, ComponentTaskButton, "", theme.StateNormal)
		ideal = int(a.metric(KeyTaskButtonIconSize)) + 2*int(st.PadX)
		min = ideal
	}
	per := (b.Dx() - gap*(n-1)) / n

	w := per
	switch {
	// Тема не задала идеальную ширину — делим место поровну: пустая метрика
	// не повод не показать ни одной ячейки.
	case ideal <= 0:
		w = per
	case per >= ideal:
		w = ideal
	case per >= min:
		w = per
	default:
		w = per // теснее минимума — значки без подписей, ширина как вышло
	}
	if w < 1 {
		return
	}
	x := b.Min.X
	for i := 0; i < n; i++ {
		r := image.Rect(x, b.Min.Y, x+w, b.Max.Y).Intersect(b)
		if r.Empty() {
			break
		}
		a.rects = append(a.rects, r)
		x += w + gap
	}
}

// ─── Component для презентера темы ──────────────────────────────────────────

// Theme реализует Component.
func (a *ApplicationArea) Theme() *theme.Manager { return a.tm }

// Cells реализует Component: закреплённые и запущенные одним списком.
func (a *ApplicationArea) Cells() []Cell {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cells := make([]Cell, 0, len(a.entries))
	for _, e := range a.entries {
		cells = append(cells, Cell{
			Title:  e.title,
			Icon:   e.icon,
			Active: e.active,
			// Приглушены и свёрнутые окна, и закреплённые незапущенные: и то и
			// другое означает «сейчас на экране этого нет».
			Muted: e.min || !e.live,
		})
	}
	return cells
}

// HoverIndex реализует Component.
func (a *ApplicationArea) HoverIndex() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.hoverIdx
}

func (a *ApplicationArea) presenter() Presenter {
	return PresenterFor(a.tm, PresenterKeyRunningApps)
}

// ─── Отрисовка ──────────────────────────────────────────────────────────────

// Draw рисует ячейки — или отдаёт отрисовку презентеру темы.
func (a *ApplicationArea) Draw(ctx widget.DrawContext) {
	b := a.Bounds()
	if b.Empty() {
		return
	}
	if p := a.presenter(); p != nil {
		p.Draw(ctx, a)
		return
	}

	a.mu.RLock()
	entries := append([]appEntry(nil), a.entries...)
	rects := append([]image.Rectangle(nil), a.rects...)
	hover, armed := a.hoverIdx, a.armedIdx
	a.mu.RUnlock()

	iconSize := int(a.metric(KeyTaskButtonIconSize))
	labelGap := int(a.metric(KeyTaskButtonLabelGap))
	labels := a.tm == nil || a.tm.GetFlag(KeyTaskButtonLabel, true)
	prev := ctx.Clip()

	for i, r := range rects {
		if i >= len(entries) {
			break
		}
		e := entries[i]
		st := StateOf(i == hover, i == armed, e.active, e.min || !e.live, false)
		s := styleOf(a.tm, ComponentTaskButton, "", st)
		PaintStyle(ctx, r, s)

		// Метка открытого окна: закреплённое, но незапущенное её не получает —
		// в этом вся разница между «закреплено» и «открыто».
		if e.live {
			DrawUnderline(ctx, r, int(a.metric(KeyTaskButtonUnderline)),
				a.metric(KeyTaskButtonUnderlineLen), e.active, s)
		}

		padX := int(s.PadX)
		iconX := r.Min.X + padX
		if e.icon != nil && iconSize > 0 {
			ctx.DrawImageScaled(e.icon, iconX, r.Min.Y+(r.Dy()-iconSize)/2, iconSize, iconSize)
		}
		// Подпись помещается не всегда, и тема вправе не хотеть её вовсе.
		textLeft := iconX + iconSize + labelGap
		if labels && r.Max.X-textLeft > iconSize {
			ctx.SetClip(r.Intersect(prev))
			DrawTextLeftElided(ctx, image.Rect(textLeft-padX, r.Min.Y, r.Max.X, r.Max.Y), e.title, s)
			ctx.SetClip(prev)
		}
	}
	a.DrawChildren(ctx)
}

// ─── Мышь ───────────────────────────────────────────────────────────────────

// OnMouseMove подсвечивает ячейку под курсором.
func (a *ApplicationArea) OnMouseMove(x, y int) {
	idx := a.hit(x, y)
	a.mu.Lock()
	changed := idx != a.hoverIdx
	a.hoverIdx = idx
	a.mu.Unlock()
	if !changed {
		return
	}
	// У дока от наведения зависят размеры ячеек, а значит и попадание.
	if a.presenter() != nil {
		a.layout()
	}
	a.Invalidate()
}

// OnMouseButton: щелчок по закреплённому незапущенному запускает его, по
// запущенному — активирует или сворачивает. Срабатывает на отпускании над
// той же ячейкой — как у всех кнопок панели задач.
func (a *ApplicationArea) OnMouseButton(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft {
		return false
	}
	idx := a.hit(e.X, e.Y)
	if e.Pressed {
		if idx < 0 {
			return false
		}
		a.mu.Lock()
		a.armedIdx = idx
		a.mu.Unlock()
		a.Invalidate()
		return true
	}

	a.mu.Lock()
	was := a.armedIdx
	a.armedIdx = -1
	entry := appEntry{}
	if was >= 0 && was < len(a.entries) {
		entry = a.entries[was]
	}
	a.mu.Unlock()
	a.Invalidate()

	if was < 0 || was != idx {
		return was >= 0 // нажатие мы поглотили, отпускание в сторону — отмена
	}
	switch {
	case !entry.live && a.cat != nil:
		_ = a.cat.Launch(entry.app)
	case entry.live && entry.active:
		a.wm.Minimize(entry.window)
	case entry.live:
		a.wm.Activate(entry.window)
	}
	return true
}

// hit возвращает индекс ячейки под точкой или -1.
func (a *ApplicationArea) hit(x, y int) int {
	pt := image.Pt(x, y)
	a.mu.RLock()
	defer a.mu.RUnlock()
	for i, r := range a.rects {
		if pt.In(r) {
			return i
		}
	}
	return -1
}

func (a *ApplicationArea) metric(k theme.Key) float64 {
	if a.tm == nil {
		return 0
	}
	return a.tm.GetMetric(k)
}
