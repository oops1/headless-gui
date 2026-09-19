package widget

import "image"

// tabcontrol_overflow.go — полоса вкладок, в которую вкладки не помещаются.
//
// Раньше заголовки выкладывались подряд от левого края и молча уезжали за
// правый: вкладка существовала, но добраться до неё было нечем — ни колеса,
// ни меню. Приложение об этом не узнавало (TabCount по-прежнему считал её), а
// пользователь видел обрезанный заголовок и пустоту за ним.
//
// Теперь лишние заголовки прячутся в меню под шевроном в конце полосы — тем
// же знаком и тем же способом, что и переполнение ToolBar. Активная вкладка
// из полосы не исчезает никогда: если она оказалась за краем, полоса
// прокручивается так, чтобы показать её (а уехавшие влево тоже попадают в
// меню) — иначе выбор вкладки из меню приводил бы к тому, что вкладка
// открыта, а в полосе её нет.

// tabChevronW — ширина кнопки «остальные вкладки» в конце полосы.
const tabChevronW = 22

// tabSlot — место вкладки в полосе: её индекс, зазор-разделитель перед ней и
// ширина заголовка.
type tabSlot struct{ idx, lead, width int }

// tabSlots раскладывает заголовки видимых вкладок по полосе.
//
// Возвращает прямоугольник каждой вкладки (пустой у скрытых и у не
// поместившихся), список не поместившихся и прямоугольник шеврона. Это
// ЕДИНСТВЕННЫЙ расчёт раскладки полосы: по нему и рисуют, и ищут попадание
// клика (см. комментарий layoutTabs про рассинхрон Draw и хит-теста).
//
// Вызывать под tc.mu.
func (tc *TabControl) tabSlots() (rects []image.Rectangle, hidden []int, chevron image.Rectangle) {
	rects = make([]image.Rectangle, len(tc.tabs))
	b := tc.bounds
	if b.Empty() {
		return rects, nil, chevron
	}
	widths := tc.layoutTabs()

	var order []tabSlot
	seen := false
	for i, tab := range tc.tabs {
		if tab.Hidden {
			continue // скрытая вкладка места в полосе не занимает
		}
		lead := 0
		if tab.SeparatorBefore && seen {
			lead = tabSepW
		}
		seen = true
		order = append(order, tabSlot{idx: i, lead: lead, width: widths[i]})
	}
	if len(order) == 0 {
		return rects, nil, chevron
	}

	total := 0
	for _, s := range order {
		total += s.lead + s.width
	}

	// place выкладывает вкладки от order[start] и возвращает индекс последней
	// поместившейся. Первая показывается всегда, даже если она шире полосы:
	// полоса из одного шеврона не сказала бы, что за вкладки за ним.
	place := func(start, limit int) int {
		for i := range rects {
			rects[i] = image.Rectangle{}
		}
		x := b.Min.X
		last := start
		for k := start; k < len(order); k++ {
			lead := order[k].lead
			if k == start {
				lead = 0 // первой в строке разделитель группы не с чем разделять
			}
			if k > start && x+lead+order[k].width > limit {
				break
			}
			x += lead
			rects[order[k].idx] = image.Rect(x, b.Min.Y, x+order[k].width, b.Min.Y+tc.TabHeight)
			x += order[k].width
			last = k
		}
		return last
	}

	if !tc.Overflow || total <= b.Dx() {
		place(0, b.Max.X)
		return rects, nil, chevron
	}

	// Активная вкладка обязана быть видимой: сдвигаем начало полосы вправо,
	// пока она не покажется.
	active := -1
	for k, s := range order {
		if s.idx == tc.active {
			active = k
			break
		}
	}
	limit := b.Max.X - tabChevronW
	start := 0
	last := place(start, limit)
	for active > last && start < active {
		start++
		last = place(start, limit)
	}

	for k, s := range order {
		if k < start || k > last {
			hidden = append(hidden, s.idx)
		}
	}
	if len(hidden) == 0 {
		return rects, nil, chevron // всё-таки поместилось — шеврон не нужен
	}
	chevron = image.Rect(b.Max.X-tabChevronW, b.Min.Y, b.Max.X, b.Min.Y+tc.TabHeight)
	return rects, hidden, chevron
}

// drawTabChevron рисует знак переполнения — ту же двойную стрелку, что и ToolBar.
func (tc *TabControl) drawTabChevron(ctx DrawContext, c image.Rectangle) {
	if c.Empty() {
		return
	}
	ctx.FillRect(c.Min.X, c.Min.Y, c.Dx(), c.Dy(), tc.TabBG)
	cx, cy := c.Min.X+c.Dx()/2, c.Min.Y+c.Dy()/2
	for _, dx := range []int{-3, 1} {
		for i := 0; i < 4; i++ {
			ctx.SetPixel(cx+dx-2+i, cy-3+i, tc.TabText)
			ctx.SetPixel(cx+dx-2+i, cy+3-i, tc.TabText)
		}
	}
}

// OverflowCount сообщает, сколько вкладок не поместилось в полосу.
//
// Нужен приложению, которое хочет узнать, что полоса тесна, — и тестам,
// которым иначе пришлось бы искать шеврон по пикселям.
func (tc *TabControl) OverflowCount() int {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	_, hidden, _ := tc.tabSlots()
	return len(hidden)
}

// overflowTabItems превращает не поместившиеся вкладки в пункты меню.
// Вызывать вне tc.mu (пункты замыкаются на tc.activateTab).
func (tc *TabControl) overflowTabItems(hidden []int) []MenuItem {
	tc.mu.Lock()
	items := make([]MenuItem, 0, len(hidden))
	for _, idx := range hidden {
		if idx < 0 || idx >= len(tc.tabs) {
			continue
		}
		tab := tc.tabs[idx]
		i := idx
		items = append(items, MenuItem{
			Text:    tab.Header,
			Icon:    tab.Icon,
			Checked: i == tc.active,
			OnClick: func() { tc.activateTab(i) },
		})
	}
	tc.mu.Unlock()
	return items
}

// activateTab делает вкладку активной так же, как щелчок по её заголовку:
// с пересчётом раскладки содержимого и уведомлением OnTabChange.
func (tc *TabControl) activateTab(idx int) {
	tc.mu.Lock()
	changed := idx >= 0 && idx < len(tc.tabs) && idx != tc.active
	header := ""
	if changed {
		tc.active = idx
		header = tc.tabs[idx].Header
	}
	onTab := tc.OnTabChange
	tc.mu.Unlock()
	if !changed {
		return
	}
	tc.layoutContent()
	tc.Invalidate()
	if onTab != nil {
		onTab(idx, header)
	}
}

// ─── Overlay (меню переполнения) ───────────────────────────────────────────

// HasOverlay реализует OverlayDrawer.
func (tc *TabControl) HasOverlay() bool { return tc.menu.open() }

// DrawOverlay рисует меню переполнения поверх всего UI.
func (tc *TabControl) DrawOverlay(ctx DrawContext) { tc.menu.drawOverlay(ctx) }

// OverlayBounds отдаёт прямоугольник открытого меню (для выноса в окно ОС).
func (tc *TabControl) OverlayBounds() image.Rectangle { return tc.menu.overlayBounds() }

// Dismiss закрывает меню переполнения. Реализует Dismissable.
func (tc *TabControl) Dismiss() { tc.menu.dismiss() }
