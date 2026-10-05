// applicationarea.go — область приложений: закреплённые вперемешку с
// запущенными окнами.
//
// Отличие от RunningApplications в том, что показываются не только открытые
// окна, но и закреплённые приложения, которые сейчас не запущены: щелчок по
// такому запускает его, щелчок по запущенному — переключает окно. Именно так
// устроена панель задач начиная с Windows 7 и док macOS.
//
// Тема может попросить сгруппировать окна (taskbutton.group): несколько окон
// одного приложения тогда живут в одной кнопке, щелчок по ней отдаёт список
// окон предпросмотру, правый щелчок открывает меню команд (AppCommands).
//
// Компонент, как и RunningApplications, отдаёт отрисовку презентеру темы,
// если та его назначила: под macOS это тот же док. Поведение при этом
// остаётся здесь и одинаково для всех тем.
package desktop

import (
	"fmt"
	"image"
	"sync"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ApplicationArea — область закреплённых и запущенных приложений.
type ApplicationArea struct {
	widget.Base
	FocusState

	tm  *theme.Manager
	cat AppCatalog
	wm  WindowModel

	mu       sync.RWMutex
	entries  []appEntry
	rects    []image.Rectangle
	hoverIdx int
	armedIdx int
	// grouped — по какому режиму собраны entries. Тема могла смениться на ту,
	// что группирует окна иначе; layout и PreferredSize замечают это и
	// пересобирают ячейки, не требуя от оболочки пересоздавать область.
	grouped bool

	// onHover — кому сообщать о смене ячейки под курсором (см.
	// SetHoverListener). Этим живёт предпросмотр окна.
	onHover func(idx int)
	// onGroup — кому отдавать щелчок по кнопке со многими окнами (см.
	// SetGroupListener).
	onGroup func(idx int)
	// cmds — команды контекстного меню от потребителя; nil — DefaultAppCommands.
	cmds AppCommands

	// menu — контекстное меню кнопки; рисует и разбирает его сама область.
	menu appMenuHost

	unsubCat func()
	unsubWM  func()

	// fade — плавный переход цвета ячеек (тема: taskbar.item).
	fade motion

	// vertical — область лежит в столбце боковой панели: ячейки идут сверху
	// вниз, без подписей и без презентера дока (applicationarea_vertical.go).
	vertical bool
}

// appEntry — одна ячейка области: либо окно, либо закреплённое приложение,
// либо все окна одного приложения.
//
// Закреплённое приложение, у которого есть открытое окно, показывается ОДИН
// раз — окном: две ячейки на одно и то же приложение сбивают с толку, а
// щелчок по ним делает разное.
type appEntry struct {
	app   AppID
	title string
	icon  image.Image
	// iconAt — значок по размеру (AppInfo.IconAt); при отрисовке побеждает icon.
	iconAt func(size int) image.Image
	// window — окно ячейки; у стопки — главное (активное, а без него первое).
	window WindowID
	// wins — все окна ячейки: одно у обычной, несколько у стопки.
	wins   []WindowInfo
	live   bool // есть открытое окно
	active bool // активно любое из окон
	min    bool // свёрнуты все окна
	// group — ячейка собрана по правилу «окна одного приложения — одна кнопка»:
	// её ключ — приложение, а не окно, и он не меняется, пока меняется главное
	// окно стопки.
	group bool
}

// stack — у ячейки больше одного окна.
func (e appEntry) stack() bool { return len(e.wins) > 1 }

// key — устойчивый ключ ячейки для плавных переходов: номер окна, пока оно
// есть, иначе закреплённое приложение. Индекс не годится: при закрытии окна
// ячейки сдвигаются, и переход перетёк бы на соседа.
func (e appEntry) key() any {
	if e.group {
		return e.app
	}
	if e.live && e.window != 0 {
		return e.window
	}
	return e.app
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
	a.menu.dismiss()
}

// refresh перестраивает содержимое и перерисовывает область.
func (a *ApplicationArea) refresh() {
	a.rebuild()
	a.layout()
	a.Invalidate()
}

// groupMode сообщает, просит ли тема собирать окна одного приложения в одну
// кнопку.
func (a *ApplicationArea) groupMode() bool {
	return a.tm != nil && a.tm.GetFlag(KeyTaskButtonGroup, false)
}

// syncGrouping пересобирает ячейки, если тема с тех пор сменила режим
// группировки.
func (a *ApplicationArea) syncGrouping() {
	a.mu.RLock()
	stale := a.grouped != a.groupMode()
	a.mu.RUnlock()
	if stale {
		a.rebuild()
	}
}

// rebuild собирает ячейки: сначала закреплённые (в порядке закрепления),
// затем окна незакреплённых приложений. В режиме группировки все окна одного
// приложения собраны в одну ячейку.
func (a *ApplicationArea) rebuild() {
	group := a.groupMode()
	var entries []appEntry

	byApp := map[AppID][]WindowInfo{}
	var order []WindowInfo
	if a.wm != nil {
		order = a.wm.Windows()
		for _, w := range order {
			if w.AppID != "" {
				byApp[w.AppID] = append(byApp[w.AppID], w)
			}
		}
	}

	apps := map[AppID]AppInfo{}
	pinned := map[AppID]bool{}
	if a.cat != nil {
		for _, app := range a.cat.Apps() {
			apps[app.ID] = app
		}
		for _, id := range a.cat.Pinned() {
			pinned[id] = true
			e := appEntry{app: id, title: string(id)}
			if info, ok := apps[id]; ok {
				e.title, e.icon, e.iconAt = info.Title, info.Icon, info.IconAt
			}
			if wins := byApp[id]; len(wins) > 0 {
				if !group {
					wins = wins[:1] // без группировки показывается первое окно
				}
				e = withWindows(e, wins, group)
			}
			entries = append(entries, e)
		}
	}

	seen := map[AppID]bool{}
	for _, w := range order {
		if w.AppID != "" && pinned[w.AppID] {
			continue // уже показано закреплённой ячейкой
		}
		if group && w.AppID != "" {
			if seen[w.AppID] {
				continue // окно уже вошло в ячейку своего приложения
			}
			seen[w.AppID] = true
			e := appEntry{app: w.AppID, title: w.Title}
			if info, ok := apps[w.AppID]; ok {
				e.title, e.icon, e.iconAt = info.Title, info.Icon, info.IconAt
			}
			entries = append(entries, withWindows(e, byApp[w.AppID], true))
			continue
		}
		entries = append(entries, appEntry{
			app: w.AppID, title: w.Title, icon: w.Icon,
			window: w.ID, wins: []WindowInfo{w}, live: true, active: w.Active, min: w.Minimized,
		})
	}

	a.mu.Lock()
	a.entries = entries
	a.grouped = group
	a.mu.Unlock()
}

// withWindows дописывает ячейке её окна. Главным становится активное окно, а
// без него первое не свёрнутое, а без него первое: щелчок по кнопке с одним
// окном обязан вести в то, что видно.
func withWindows(e appEntry, wins []WindowInfo, group bool) appEntry {
	rep := wins[0]
	for _, w := range wins {
		if w.Active {
			rep = w
			break
		}
		if rep.Minimized && !w.Minimized {
			rep = w
		}
	}
	e.wins = append([]WindowInfo(nil), wins...)
	e.window, e.live, e.group = rep.ID, true, group
	e.min = true
	for _, w := range wins {
		e.active = e.active || w.Active
		e.min = e.min && w.Minimized
	}
	if rep.Icon != nil {
		e.icon, e.iconAt = rep.Icon, nil
	}
	return e
}

// SetBounds задаёт границы области и раскладывает ячейки.
func (a *ApplicationArea) SetBounds(r image.Rectangle) {
	a.Base.SetBounds(r)
	a.layout()
}

// PreferredSize — как у полосы кнопок окон; презентеру темы, если он есть,
// размер решать самому.
func (a *ApplicationArea) PreferredSize(avail image.Point) image.Point {
	a.syncGrouping()
	if p := a.presenter(); p != nil {
		return p.Measure(a, avail)
	}
	n := len(a.Cells())
	if n == 0 {
		return image.Point{}
	}
	if a.vertical {
		return a.preferredVertical(n, avail)
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
	a.syncGrouping()
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

	if a.vertical {
		a.layoutVertical(b, n)
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
	a.syncGrouping()
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
	// Док macOS — ряд; в столбце боковой панели область рисуется сама.
	if a.vertical {
		return nil
	}
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
	focus := -1
	if a.FocusVisible() {
		focus = a.FocusState.Cell(len(rects))
	}

	iconSize := int(a.metric(KeyTaskButtonIconSize))
	labelGap := int(a.metric(KeyTaskButtonLabelGap))
	labels := (a.tm == nil || a.tm.GetFlag(KeyTaskButtonLabel, true)) && !a.vertical
	muted := taskButtonMuted(a.tm)
	prev := ctx.Clip()

	for i, r := range rects {
		if i >= len(entries) {
			break
		}
		e := entries[i]
		st := StateOf(i == hover, i == armed, e.active, (e.min || !e.live) && muted, i == focus)
		s := a.fade.ItemStyle(a.tm, e.key(), r, st, func(st theme.State) *theme.Style {
			return styleOf(a.tm, ComponentTaskButton, "", st)
		})
		PaintStyle(ctx, r, s)

		// Метка открытого окна: закреплённое, но незапущенное её не получает —
		// в этом вся разница между «закреплено» и «открыто».
		if e.live {
			drawTaskMark(ctx, a.tm, r, e.active, s)
		}

		padX := int(s.PadX)
		iconX := r.Min.X + padX
		var iconRect image.Rectangle
		if (e.icon != nil || e.iconAt != nil) && iconSize > 0 {
			iconRect = image.Rect(iconX, r.Min.Y+(r.Dy()-iconSize)/2, iconX+iconSize, r.Min.Y+(r.Dy()-iconSize)/2+iconSize)
			drawAppIcon(ctx, e.icon, e.iconAt, iconRect)
		}
		// Торцы стопки: за значком видно, что окон несколько.
		if e.stack() {
			stackIcon := iconRect
			if stackIcon.Empty() {
				stackIcon = image.Rect(iconX, r.Min.Y+(r.Dy()-iconSize)/2, iconX+iconSize, r.Min.Y+(r.Dy()-iconSize)/2+iconSize)
			}
			drawStack(ctx, a.tm, stackIcon, s)
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
	if a.menu.routeMove(x, y) {
		return // курсор над открытым меню: ячейки под ним не подсвечиваем
	}
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

	// Слушателя зовём ПОСЛЕДНИМ и вне замка: он в ответ трогает свои
	// структуры и может спросить у нас же прямоугольник ячейки.
	a.mu.RLock()
	onHover := a.onHover
	a.mu.RUnlock()
	if onHover != nil {
		onHover(idx)
	}
}

// SetHoverListener реализует HoverArea: кому докладывать о смене ячейки под
// курсором (-1 — курсора на ячейках нет). Этим живёт предпросмотр окна.
func (a *ApplicationArea) SetHoverListener(fn func(idx int)) {
	a.mu.Lock()
	a.onHover = fn
	a.mu.Unlock()
}

// SetGroupListener реализует GroupClickArea: кому отдавать щелчок по кнопке со
// многими окнами. Без слушателя область переключает окна приложения по кругу.
func (a *ApplicationArea) SetGroupListener(fn func(idx int)) {
	a.mu.Lock()
	a.onGroup = fn
	a.mu.Unlock()
}

// SetCommands задаёт источник команд контекстного меню кнопки. nil возвращает
// набор по умолчанию (DefaultAppCommands).
func (a *ApplicationArea) SetCommands(c AppCommands) {
	a.mu.Lock()
	a.cmds = c
	a.mu.Unlock()
}

// ButtonRect реализует HoverArea: прямоугольник ячейки i в абсолютных
// логических координатах (пустой — такой ячейки нет).
//
// Наружу это нужно тому, кто прижимает к ячейке своё окно, — предпросмотру.
// Без этого метода оболочке пришлось бы повторить у себя раскладку области и
// ломать копию при каждом её изменении.
func (a *ApplicationArea) ButtonRect(i int) image.Rectangle {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if i < 0 || i >= len(a.rects) {
		return image.Rectangle{}
	}
	return a.rects[i]
}

// WindowAt реализует HoverArea: окно ячейки i.
//
// У закреплённого незапущенного приложения окна нет — показывать в
// предпросмотре нечего, и второе значение ложно.
func (a *ApplicationArea) WindowAt(i int) (WindowInfo, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if i < 0 || i >= len(a.entries) {
		return WindowInfo{}, false
	}
	e := a.entries[i]
	if !e.live {
		return WindowInfo{}, false
	}
	return WindowInfo{
		ID:        e.window,
		Title:     e.title,
		AppID:     e.app,
		Icon:      e.icon,
		Active:    e.active,
		Minimized: e.min,
	}, true
}

// WindowsAt реализует GroupHoverArea: все окна ячейки i (у незапущенной —
// ни одного).
func (a *ApplicationArea) WindowsAt(i int) []WindowInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if i < 0 || i >= len(a.entries) || !a.entries[i].live {
		return nil
	}
	return append([]WindowInfo(nil), a.entries[i].wins...)
}

// OnMouseButton: щелчок по закреплённому незапущенному запускает его, по
// запущенному — активирует или сворачивает, по кнопке со многими окнами —
// открывает их список. Срабатывает на отпускании над той же ячейкой — как у
// всех кнопок панели задач. Правая кнопка открывает меню команд.
func (a *ApplicationArea) OnMouseButton(e widget.MouseEvent) bool {
	if a.menu.routeMouse(e) {
		return true
	}
	if e.Button == widget.MouseRight {
		if !e.Pressed {
			if idx := a.hit(e.X, e.Y); idx >= 0 {
				a.ShowCommands(idx)
				return true
			}
		}
		return false
	}
	if e.Button != widget.MouseLeft {
		return false
	}
	idx := a.hit(e.X, e.Y)
	if e.Pressed {
		if idx < 0 {
			return false
		}
		a.NotePointer(e)
		a.mu.Lock()
		a.armedIdx = idx
		a.mu.Unlock()
		a.Invalidate()
		return true
	}

	a.mu.Lock()
	was := a.armedIdx
	a.armedIdx = -1
	a.mu.Unlock()
	a.Invalidate()

	if was < 0 || was != idx {
		return was >= 0 // нажатие мы поглотили, отпускание в сторону — отмена
	}
	a.activate(was, true)
	return true
}

// activate делает с ячейкой i то, что делает щелчок по ней: запускает
// закреплённое незапущенное, сворачивает активное окно, активирует остальные,
// а у стопки открывает список окон (showList) — или, когда списка не
// показать, переключает окна по кругу. С клавиатуры список недоступен: в него
// некуда перенести фокус, поэтому Enter и Space у стопки листают окна.
func (a *ApplicationArea) activate(i int, showList bool) {
	a.mu.RLock()
	var entry appEntry
	ok := i >= 0 && i < len(a.entries)
	if ok {
		entry = a.entries[i]
	}
	onGroup := a.onGroup
	a.mu.RUnlock()
	if !ok {
		return
	}
	switch {
	case !entry.live && a.cat != nil:
		_ = a.cat.Launch(entry.app)
	case !entry.live:
	case entry.stack() && showList && onGroup != nil:
		onGroup(i)
	case entry.stack():
		a.cycle(entry)
	case entry.active:
		a.wm.Minimize(entry.window)
	default:
		a.wm.Activate(entry.window)
	}
}

// cycle переключает окна стопки по кругу: от активного к следующему, а без
// активного — к главному. Так кнопка со многими окнами работает, пока никто
// не показывает их список.
func (a *ApplicationArea) cycle(e appEntry) {
	next := e.window
	for i, w := range e.wins {
		if w.Active {
			next = e.wins[(i+1)%len(e.wins)].ID
			break
		}
	}
	a.wm.Activate(next)
}

// ShowCommands открывает меню команд кнопки i. Команды берутся у потребителя
// (SetCommands), а без него — DefaultAppCommands. Меню встаёт у края кнопки,
// обращённого к рабочему столу.
func (a *ApplicationArea) ShowCommands(i int) {
	a.mu.RLock()
	var e appEntry
	ok := i >= 0 && i < len(a.entries) && i < len(a.rects)
	var anchor image.Rectangle
	if ok {
		e, anchor = a.entries[i], a.rects[i]
	}
	src := a.cmds
	a.mu.RUnlock()
	if !ok {
		return
	}

	btn := AppButton{App: e.app, Title: e.title, Windows: e.wins}
	if a.cat != nil {
		for _, id := range a.cat.Pinned() {
			if id == e.app && id != "" {
				btn.Pinned = true
			}
		}
	}
	var cmds []AppCommand
	if src != nil {
		cmds = src.Commands(btn)
	} else {
		cmds = DefaultAppCommands(a.cat, a.wm, btn)
	}
	// Панель вверху экрана (строка меню) раскрывает меню вниз.
	below := a.tm != nil && a.tm.GetFlag(KeyTaskbarTop, false)
	a.menu.show(a.tm, menuItems(cmds), anchor, below)
}

// ToolTipAt возвращает подсказку кнопки под точкой: название приложения (у
// стопки — с числом окон). Там, где есть предпросмотр, подсказка не нужна —
// его показывает наведение; остаются закреплённые незапущенные кнопки и
// области без предпросмотра. Реализует интерфейс подсказок движка.
func (a *ApplicationArea) ToolTipAt(x, y int) string {
	i := a.hit(x, y)
	a.mu.RLock()
	defer a.mu.RUnlock()
	if i < 0 || i >= len(a.entries) {
		return ""
	}
	e := a.entries[i]
	if e.title == "" || (e.live && a.previewShown()) {
		return ""
	}
	if e.stack() {
		return fmt.Sprintf(tr(StrAppWindows), e.title, len(e.wins))
	}
	return e.title
}

// previewShown — покажет ли наведение на живую кнопку предпросмотр: за
// областью следят, тема его допускает, а модель умеет отдавать миниатюры.
// Вызывается под a.mu.
func (a *ApplicationArea) previewShown() bool {
	if a.onHover == nil || a.wm == nil {
		return false
	}
	if _, ok := a.wm.(WindowPreviews); !ok {
		return false
	}
	return a.tm == nil || a.tm.GetFlag(KeyPreview, true)
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

// ─── Контекстное меню как оверлей ───────────────────────────────────────────

// HasOverlay реализует widget.OverlayDrawer: меню команд рисуется поверх всего.
func (a *ApplicationArea) HasOverlay() bool { return a.menu.open() }

// DrawOverlay реализует widget.OverlayDrawer.
func (a *ApplicationArea) DrawOverlay(ctx widget.DrawContext) { a.menu.drawOverlay(ctx) }

// OverlayBounds реализует widget.OverlayBoundsProvider (для выноса меню в
// окно ОС).
func (a *ApplicationArea) OverlayBounds() image.Rectangle { return a.menu.overlayBounds() }

// Dismiss реализует widget.Dismissable: клик мимо закрывает меню.
func (a *ApplicationArea) Dismiss() { a.menu.dismiss() }

// DismissOnEscape реализует widget.EscapeDismisser.
func (a *ApplicationArea) DismissOnEscape() bool {
	if !a.menu.open() {
		return false
	}
	a.menu.dismiss()
	return true
}
