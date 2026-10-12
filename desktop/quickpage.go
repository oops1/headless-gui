// quickpage.go — строитель вложенной страницы быстрых настроек Windows 11:
// готовые элементы (список, ползунок, переключатель, кнопка, подпись), которые
// сами берут цвета, скругления и размеры из профиля темы.
//
// Раньше потребитель отдавал в QuickDetails.Content свои виджеты и на каждую
// смену темы перекрашивал их вручную (widget.ApplyThemeTree). Страница-строитель
// ничего не красит заранее: каждый кадр она спрашивает у темы части стиля
// «quicksettings» (w11.page.*, w11.slider.*, w11.label, w11.detail, w11.dim), так
// что смена профиля, тёмной темы и акцента видна сразу, без единого вызова
// потребителя:
//
//	q.Details = func(id desktop.QuickActionID) *desktop.QuickDetails {
//	    page := q.NewDetailPage()
//	    page.AddList(networks, func(i int, it desktop.DetailListItem) { connect(it) })
//	    page.AddToggle("Использовать лимитное подключение", metered, setMetered)
//	    page.AddButton("Другие сети", openNetworks)
//	    return page.Details("Wi-Fi")
//	}
//
// Страница — один виджет: сама раскладывает элементы столбцом, прокручивает их
// колесом, если они не помещаются, принимает мышь и клавиши (стрелки между
// элементами, Enter и пробел нажимают, ←→ двигают ползунок). Методы Set* можно
// звать из любой горутины.
package desktop

import (
	"image"
	"image/color"
	"strconv"
	"sync"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Пропорции страницы, не размеры темы: зазор между элементами, высота кнопки и
// переключателя, размеры самого переключателя, шаг колеса и клавиш.
const (
	dpGap        = 4
	dpToggleH    = 40
	dpButtonH    = 32
	dpTextH      = 28
	dpSwitchW    = 40
	dpSwitchH    = 20
	dpThumbRest  = 12
	dpThumbHover = 14
	dpPad        = 12
	dpWheel      = 40
	dpKeyStep    = 0.05
	dpScrollW    = 4
	dpStateGap   = 8
)

// DetailListItem — строка списка страницы: сеть, устройство, пункт выбора.
type DetailListItem struct {
	// Icon — значок слева; IconAt, если задан, главнее и получает сторону в
	// ФИЗИЧЕСКИХ пикселях (как QuickAction.IconAt). Нет ни того ни другого —
	// строка без значка. Значок перекрашивается цветом текста строки, если
	// профиль поднял theme.KeyQuickIconTint (как плитки).
	Icon   image.Image
	IconAt func(size int) image.Image
	// Title — название; Subtitle — вторая строка серым (защищённая сеть,
	// уровень сигнала); State — короткая надпись справа («Подключено»).
	Title, Subtitle, State string
	// Selected — строка выбрана (полоса акцента слева, залитая подложка).
	Selected bool
	// Disabled — строка недоступна: приглушена и не нажимается.
	Disabled bool
}

// dpElement — элемент страницы. Все методы зовутся под замком страницы.
type dpElement interface {
	height(p *DetailPage, m qsMetrics) int
	// rows — сколько у элемента строк, на которые можно навести и встать
	// клавиатурой (у списка по числу пунктов, у прочих одна или нуль).
	rows() int
	// rowEnabled — строка sub доступна клавиатуре и мыши (у списка недоступные
	// пункты выпадают из обхода).
	rowEnabled(sub int) bool
	// rowRect — прямоугольник строки sub элемента, стоящего в r.
	rowRect(p *DetailPage, m qsMetrics, r image.Rectangle, sub int) image.Rectangle
	draw(ctx widget.DrawContext, p *DetailPage, m qsMetrics, r image.Rectangle, idx int)
	// click выполняет нажатие на строку; возвращает действие для вызова без
	// замка.
	click(p *DetailPage, sub int) func()
	// key разбирает клавишу на строке; false — клавиша не его.
	key(p *DetailPage, sub int, e widget.KeyEvent) (func(), bool)
	enabled() bool
}

// dpHit — адрес строки страницы: номер элемента и строка внутри него.
type dpHit struct{ item, sub int }

var dpNone = dpHit{-1, 0}

// DetailPage — страница быстрых настроек из готовых элементов. Реализует
// widget.Widget и кладётся в QuickDetails.Content (см. Details).
type DetailPage struct {
	widget.Base
	tm *theme.Manager

	mu      sync.Mutex
	items   []dpElement
	scroll  int
	hover   dpHit
	press   dpHit
	kb      dpHit // строка с клавиатурным фокусом
	focused bool  // страницей владеет клавиатура
	ring    bool  // рамка фокуса показана: фокус пришёл с клавиатуры
	drag    bool  // тянут ползунок press.item

	memos map[any]*glyphMemo // перекрашенные значки по строке (только поток кадра)
}

// NewDetailPage создаёт пустую страницу, оформляемую темой tm.
func NewDetailPage(tm *theme.Manager) *DetailPage {
	return &DetailPage{tm: tm, hover: dpNone, press: dpNone, kb: dpNone, memos: map[any]*glyphMemo{}}
}

// NewDetailPage создаёт пустую страницу, оформляемую темой панели.
func (q *QuickSettings) NewDetailPage() *DetailPage { return NewDetailPage(q.Theme()) }

// Details оборачивает страницу во вложенную страницу панели с заголовком title
// (пусто — название плитки).
func (p *DetailPage) Details(title string) *QuickDetails {
	return &QuickDetails{Title: title, Content: p}
}

// Clear убирает все элементы.
func (p *DetailPage) Clear() {
	p.mu.Lock()
	p.items = nil
	p.scroll = 0
	p.hover, p.press, p.kb = dpNone, dpNone, dpNone
	p.mu.Unlock()
	p.repaint()
}

func (p *DetailPage) add(e dpElement) {
	p.mu.Lock()
	p.items = append(p.items, e)
	p.mu.Unlock()
	p.repaint()
}

// repaint заявляет перерисовку области страницы. Зовётся без замка.
func (p *DetailPage) repaint() { widget.InvalidateRect(p.Bounds()) }

// part читает стиль части панели из темы (пустой стиль, если темы нет).
func (p *DetailPage) part(name string, st theme.State) *theme.Style {
	if p.tm == nil {
		return &theme.Style{}
	}
	return p.tm.GetStyle(ComponentQuickSettings, name, st)
}

// ─── Раскладка ───────────────────────────────────────────────────────────────

// dpPlace — где элемент стоит в кадре (с учётом прокрутки).
type dpPlace struct {
	e dpElement
	r image.Rectangle
}

// places раскладывает элементы столбцом; total — полная высота содержимого.
// Под замком.
func (p *DetailPage) places(m qsMetrics) (out []dpPlace, total int) {
	b := p.Bounds()
	y := b.Min.Y - p.scroll
	for i, e := range p.items {
		h := e.height(p, m)
		out = append(out, dpPlace{e, image.Rect(b.Min.X, y, b.Max.X, y+h)})
		y += h
		if i < len(p.items)-1 {
			y += dpGap
		}
	}
	return out, y + p.scroll - b.Min.Y
}

// maxScroll — наибольшая прокрутка. Под замком.
func (p *DetailPage) maxScroll(m qsMetrics) int {
	_, total := p.places(m)
	if v := total - p.Bounds().Dy(); v > 0 {
		return v
	}
	return 0
}

// hitAt находит строку под точкой. Под замком.
func (p *DetailPage) hitAt(m qsMetrics, pt image.Point) (dpHit, dpElement, image.Rectangle) {
	if !pt.In(p.Bounds()) {
		return dpNone, nil, image.Rectangle{}
	}
	pl, _ := p.places(m)
	for i, it := range pl {
		if !pt.In(it.r) {
			continue
		}
		for s := 0; s < it.e.rows(); s++ {
			if rr := it.e.rowRect(p, m, it.r, s); pt.In(rr) {
				return dpHit{i, s}, it.e, it.r
			}
		}
		return dpNone, nil, image.Rectangle{}
	}
	return dpNone, nil, image.Rectangle{}
}

// ─── Рисование ───────────────────────────────────────────────────────────────

// Draw рисует элементы в границах страницы; то, что не помещается, обрезается.
func (p *DetailPage) Draw(ctx widget.DrawContext) {
	b := p.Bounds()
	if b.Empty() || p.tm == nil {
		return
	}
	m := readQSMetrics(p.tm)
	p.mu.Lock()
	defer p.mu.Unlock()
	if v := p.maxScroll(m); p.scroll > v {
		p.scroll = v
	}
	prev := ctx.Clip()
	ctx.SetClip(b.Intersect(prev))
	defer ctx.SetClip(prev)
	pl, total := p.places(m)
	for i, it := range pl {
		if it.r.Intersect(b).Empty() {
			continue
		}
		it.e.draw(ctx, p, m, it.r, i)
	}
	if over := total - b.Dy(); over > 0 {
		h := b.Dy() * b.Dy() / total
		if h < 24 {
			h = 24
		}
		y := b.Min.Y + (b.Dy()-h)*p.scroll/over
		sb := p.part("w11.scrollbar", theme.StateNormal)
		qsPaint(ctx, image.Rect(b.Max.X-dpScrollW, y, b.Max.X, y+h), sb, b)
	}
	if p.ring && p.focused && p.kb != dpNone {
		for i, it := range pl {
			if i == p.kb.item && p.kb.sub < it.e.rows() {
				PaintFocusRing(ctx, it.e.rowRect(p, m, it.r, p.kb.sub).Intersect(b), p.tm, p.part("w11.button", theme.StateNormal))
			}
		}
	}
}

// state собирает состояние строки по наведению и нажатию.
func (p *DetailPage) state(h dpHit, off bool) theme.State {
	hover := !off && p.hover == h
	return StateOf(hover, hover && p.press == h, false, off, false)
}

// drawIcon рисует значок строки в квадрате box: перекрашенный цветом текста,
// если профиль включил перекраску плиток.
func (p *DetailPage) drawIcon(ctx widget.DrawContext, key any, icon image.Image, at func(int) image.Image, box image.Rectangle, col color.RGBA) {
	if box.Empty() || (icon == nil && at == nil) {
		return
	}
	if p.tm == nil || !p.tm.GetFlag(theme.KeyQuickIconTint, false) {
		drawAppIcon(ctx, icon, at, box)
		return
	}
	ph := widget.PhysicalRect(ctx, box)
	side := ph.X
	if ph.Y < side {
		side = ph.Y
	}
	img := icon
	if at != nil {
		if v := at(side); v != nil {
			img = v
		}
	}
	if img == nil {
		return
	}
	memo := p.memos[key]
	if memo == nil {
		memo = &glyphMemo{}
		p.memos[key] = memo
	}
	ctx.DrawImageScaled(memo.tinted(img, col), box.Min.X, box.Min.Y, box.Dx(), box.Dy())
}

// ─── Мышь ────────────────────────────────────────────────────────────────────

// OnMouseMove ведёт наведение и перетаскивание ползунка.
func (p *DetailPage) OnMouseMove(x, y int) {
	m := readQSMetrics(p.tm)
	p.mu.Lock()
	old := p.hover
	if widget.CursorIsNowhere(x, y) {
		p.hover = dpNone
	} else if p.drag {
		pl, _ := p.places(m)
		if p.press.item >= 0 && p.press.item < len(pl) {
			if s, ok := pl[p.press.item].e.(*DetailSlider); ok {
				fn := s.setFromX(p, m, pl[p.press.item].r, x)
				p.mu.Unlock()
				if fn != nil {
					fn()
				}
				p.repaint()
				return
			}
		}
	} else {
		h, _, _ := p.hitAt(m, image.Pt(x, y))
		p.hover = h
		p.ring = false
	}
	changed := old != p.hover
	p.mu.Unlock()
	if changed {
		p.repaint()
	}
}

// OnMouseButton разбирает нажатие и отпускание левой кнопки.
func (p *DetailPage) OnMouseButton(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft {
		return false
	}
	m := readQSMetrics(p.tm)
	pt := image.Pt(e.X, e.Y)
	p.mu.Lock()
	if e.Pressed {
		h, el, r := p.hitAt(m, pt)
		if el == nil || !el.enabled() {
			p.mu.Unlock()
			return false
		}
		p.press, p.kb, p.ring = h, h, false
		var fn func()
		if s, ok := el.(*DetailSlider); ok {
			p.drag = true
			fn = s.setFromX(p, m, r, e.X)
		}
		p.mu.Unlock()
		if fn != nil {
			fn()
		}
		p.repaint()
		return true
	}
	press, drag := p.press, p.drag
	p.press, p.drag = dpNone, false
	var fn func()
	if press != dpNone && !drag {
		// Щелчок засчитывается, если отпустили на той же строке.
		h, el, _ := p.hitAt(m, pt)
		if h == press && el != nil && el.enabled() {
			fn = el.click(p, h.sub)
		}
	}
	p.mu.Unlock()
	if fn != nil {
		fn()
	}
	p.repaint()
	return press != dpNone
}

// OnMouseWheelPixels прокручивает страницу, если она не помещается.
func (p *DetailPage) OnMouseWheelPixels(x, y int, dx, dy float64) bool {
	m := readQSMetrics(p.tm)
	p.mu.Lock()
	max := p.maxScroll(m)
	if max <= 0 || dy == 0 {
		p.mu.Unlock()
		return false
	}
	step := int(dy)
	if step == 0 {
		step = 1
		if dy < 0 {
			step = -1
		}
	}
	v := p.scroll + step
	if v < 0 {
		v = 0
	}
	if v > max {
		v = max
	}
	changed := v != p.scroll
	p.scroll = v
	p.mu.Unlock()
	if changed {
		p.repaint()
	}
	return true
}

// ─── Клавиатура ──────────────────────────────────────────────────────────────

// SetFocused реализует widget.Focusable: панель отдаёт странице клавиатуру,
// когда Tab встаёт на её содержимое.
func (p *DetailPage) SetFocused(f bool) {
	p.mu.Lock()
	p.focused = f
	if !f {
		p.ring = false
	}
	p.mu.Unlock()
	p.repaint()
}

// IsFocused реализует widget.Focusable.
func (p *DetailPage) IsFocused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.focused
}

// rowsFlat перечисляет строки, на которые встаёт клавиатура, в порядке показа.
func (p *DetailPage) rowsFlat() []dpHit {
	var out []dpHit
	for i, e := range p.items {
		if !e.enabled() {
			continue
		}
		for s := 0; s < e.rows(); s++ {
			if e.rowEnabled(s) {
				out = append(out, dpHit{i, s})
			}
		}
	}
	return out
}

// OnKeyEvent: стрелки вверх и вниз ходят по строкам, Enter и пробел нажимают,
// ←→ двигают ползунок, PageUp и PageDown листают страницу.
func (p *DetailPage) OnKeyEvent(e widget.KeyEvent) {
	if !e.Pressed {
		return
	}
	m := readQSMetrics(p.tm)
	p.mu.Lock()
	p.ring = true
	flat := p.rowsFlat()
	at := -1
	for i, h := range flat {
		if h == p.kb {
			at = i
		}
	}
	var fn func()
	move := func(to int) {
		if len(flat) == 0 {
			return
		}
		if to < 0 {
			to = 0
		}
		if to >= len(flat) {
			to = len(flat) - 1
		}
		p.kb = flat[to]
		p.reveal(m, p.kb)
	}
	switch e.Code {
	case widget.KeyDown:
		move(at + 1)
	case widget.KeyUp:
		if at < 0 {
			move(0)
		} else {
			move(at - 1)
		}
	case widget.KeyHome:
		move(0)
	case widget.KeyEnd:
		move(len(flat) - 1)
	case widget.KeyPageDown:
		p.scrollBy(m, p.Bounds().Dy())
	case widget.KeyPageUp:
		p.scrollBy(m, -p.Bounds().Dy())
	default:
		if at < 0 {
			if len(flat) > 0 && (e.Code == widget.KeyEnter || e.Code == widget.KeySpace ||
				e.Code == widget.KeyLeft || e.Code == widget.KeyRight) {
				move(0)
			}
		} else if p.kb.item < len(p.items) {
			if f, ok := p.items[p.kb.item].key(p, p.kb.sub, e); ok {
				fn = f
			}
		}
	}
	p.mu.Unlock()
	if fn != nil {
		fn()
	}
	p.repaint()
}

// scrollBy сдвигает прокрутку на d с ограничением. Под замком.
func (p *DetailPage) scrollBy(m qsMetrics, d int) {
	v := p.scroll + d
	if max := p.maxScroll(m); v > max {
		v = max
	}
	if v < 0 {
		v = 0
	}
	p.scroll = v
}

// reveal прокручивает страницу так, чтобы строка h была видна целиком. Под
// замком.
func (p *DetailPage) reveal(m qsMetrics, h dpHit) {
	pl, _ := p.places(m)
	if h.item < 0 || h.item >= len(pl) {
		return
	}
	r := pl[h.item].e.rowRect(p, m, pl[h.item].r, h.sub)
	b := p.Bounds()
	switch {
	case r.Min.Y < b.Min.Y:
		p.scrollBy(m, r.Min.Y-b.Min.Y)
	case r.Max.Y > b.Max.Y:
		p.scrollBy(m, r.Max.Y-b.Max.Y)
	}
}

// ─── Список ──────────────────────────────────────────────────────────────────

// DetailList — список страницы: значок, название, подпись, состояние, выбор.
type DetailList struct {
	p        *DetailPage
	items    []DetailListItem
	selected int
	onSelect func(int, DetailListItem)
}

// AddList добавляет список. onSelect зовётся по нажатию на строку; строка при
// этом становится выбранной (недоступные не нажимаются).
func (p *DetailPage) AddList(items []DetailListItem, onSelect func(int, DetailListItem)) *DetailList {
	l := &DetailList{p: p, onSelect: onSelect, selected: -1}
	l.setItems(items)
	p.add(l)
	return l
}

func (l *DetailList) setItems(items []DetailListItem) {
	l.items = append([]DetailListItem(nil), items...)
	l.selected = -1
	for i, it := range l.items {
		if it.Selected {
			l.selected = i
			break
		}
	}
}

// SetItems заменяет строки списка (выбранной станет первая с Selected).
func (l *DetailList) SetItems(items []DetailListItem) {
	l.p.mu.Lock()
	l.setItems(items)
	l.p.memos = map[any]*glyphMemo{}
	l.p.mu.Unlock()
	l.p.repaint()
}

// Items возвращает копию строк списка.
func (l *DetailList) Items() []DetailListItem {
	l.p.mu.Lock()
	defer l.p.mu.Unlock()
	return append([]DetailListItem(nil), l.items...)
}

// Selected возвращает номер выбранной строки (-1 — никакая).
func (l *DetailList) Selected() int {
	l.p.mu.Lock()
	defer l.p.mu.Unlock()
	return l.selected
}

// SetSelected выбирает строку i (-1 снимает выбор), не зовя onSelect.
func (l *DetailList) SetSelected(i int) {
	l.p.mu.Lock()
	if i < 0 || i >= len(l.items) {
		i = -1
	}
	l.selected = i
	l.p.mu.Unlock()
	l.p.repaint()
}

func (l *DetailList) enabled() bool { return len(l.items) > 0 }
func (l *DetailList) rows() int     { return len(l.items) }
func (l *DetailList) rowEnabled(sub int) bool {
	return sub >= 0 && sub < len(l.items) && !l.items[sub].Disabled
}

func (l *DetailList) rowH(m qsMetrics) int {
	for _, it := range l.items {
		if it.Subtitle != "" {
			return m.pageRow
		}
	}
	return m.pageRow - 8
}

func (l *DetailList) height(_ *DetailPage, m qsMetrics) int {
	return len(l.items) * l.rowH(m)
}

func (l *DetailList) rowRect(_ *DetailPage, m qsMetrics, r image.Rectangle, sub int) image.Rectangle {
	h := l.rowH(m)
	return image.Rect(r.Min.X, r.Min.Y+sub*h, r.Max.X, r.Min.Y+(sub+1)*h)
}

func (l *DetailList) draw(ctx widget.DrawContext, p *DetailPage, m qsMetrics, r image.Rectangle, idx int) {
	b := p.Bounds()
	for i, it := range l.items {
		rr := l.rowRect(p, m, r, i)
		if rr.Intersect(b).Empty() {
			continue
		}
		part := "w11.page.item"
		if i == l.selected {
			part = "w11.page.item.on"
		}
		s := p.part(part, p.state(dpHit{idx, i}, it.Disabled))
		qsPaint(ctx, rr, s, b)
		if i == l.selected && !it.Disabled {
			bar := image.Rect(rr.Min.X, rr.Min.Y+(rr.Dy()-16)/2, rr.Min.X+3, rr.Min.Y+(rr.Dy()-16)/2+16)
			qsPaint(ctx, bar, p.part("w11.page.bar", theme.StateNormal), b)
		}
		x := rr.Min.X + dpPad
		if it.Icon != nil || it.IconAt != nil {
			box := image.Rect(x, rr.Min.Y+(rr.Dy()-m.pageIcon)/2, x+m.pageIcon, rr.Min.Y+(rr.Dy()-m.pageIcon)/2+m.pageIcon)
			p.drawIcon(ctx, [2]int{idx, i}, it.Icon, it.IconAt, box, s.Text)
			x += m.pageIcon + dpPad
		}
		right := rr.Max.X - dpPad
		if it.State != "" {
			ds := p.part("w11.dim", theme.StateNormal)
			w := MeasureText(ctx, it.State, ds)
			if max := (right - x) / 2; w > max {
				w = max
			}
			DrawTextLeft(ctx, image.Rect(right-w, rr.Min.Y, right, rr.Max.Y), Elide(ctx, it.State, ds, w), ds)
			right -= w + dpStateGap
		}
		ls := p.part("w11.label", theme.StateNormal)
		if it.Disabled {
			ls = p.part("w11.label", theme.StateDisabled)
		}
		tw := right - x
		if it.Subtitle == "" {
			DrawTextLeft(ctx, image.Rect(x, rr.Min.Y, right, rr.Max.Y), Elide(ctx, it.Title, ls, tw), ls)
			continue
		}
		half := rr.Dy() / 2
		DrawTextLeft(ctx, image.Rect(x, rr.Min.Y+2, right, rr.Min.Y+half), Elide(ctx, it.Title, ls, tw), ls)
		ds := p.part("w11.detail", theme.StateNormal)
		DrawTextLeft(ctx, image.Rect(x, rr.Min.Y+half-2, right, rr.Max.Y-2), Elide(ctx, it.Subtitle, ds, tw), ds)
	}
}

func (l *DetailList) click(_ *DetailPage, sub int) func() {
	if sub < 0 || sub >= len(l.items) || l.items[sub].Disabled {
		return nil
	}
	l.selected = sub
	it, fn := l.items[sub], l.onSelect
	if fn == nil {
		return nil
	}
	return func() { fn(sub, it) }
}

func (l *DetailList) key(p *DetailPage, sub int, e widget.KeyEvent) (func(), bool) {
	if e.Code == widget.KeyEnter || e.Code == widget.KeySpace {
		return l.click(p, sub), true
	}
	return nil, false
}

// ─── Ползунок ────────────────────────────────────────────────────────────────

// DetailSlider — ползунок страницы (яркость, уровень, громкость приложения):
// значок слева, дорожка, значение в процентах справа.
type DetailSlider struct {
	p        *DetailPage
	icon     image.Image
	iconAt   func(int) image.Image
	level    float64
	off      bool
	onChange func(float64)
}

// AddSlider добавляет ползунок с начальным уровнем level (0..1). icon — значок
// слева (nil — без значка). onChange зовётся при каждом изменении рукой или
// клавишами, не при SetLevel.
func (p *DetailPage) AddSlider(icon image.Image, level float64, onChange func(float64)) *DetailSlider {
	s := &DetailSlider{p: p, icon: icon, level: clamp01(level), onChange: onChange}
	p.add(s)
	return s
}

// SetIconAt задаёт источник значка по физическому размеру (главнее icon).
func (s *DetailSlider) SetIconAt(at func(size int) image.Image) {
	s.p.mu.Lock()
	s.iconAt = at
	s.p.mu.Unlock()
	s.p.repaint()
}

// SetLevel ставит уровень (0..1), не зовя onChange.
func (s *DetailSlider) SetLevel(v float64) {
	s.p.mu.Lock()
	s.level = clamp01(v)
	s.p.mu.Unlock()
	s.p.repaint()
}

// Level возвращает уровень.
func (s *DetailSlider) Level() float64 {
	s.p.mu.Lock()
	defer s.p.mu.Unlock()
	return s.level
}

// SetEnabled включает и выключает ползунок: неактивный серый и не реагирует.
func (s *DetailSlider) SetEnabled(on bool) {
	s.p.mu.Lock()
	s.off = !on
	s.p.mu.Unlock()
	s.p.repaint()
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func (s *DetailSlider) enabled() bool       { return !s.off }
func (s *DetailSlider) rows() int           { return 1 }
func (s *DetailSlider) rowEnabled(int) bool { return !s.off }

func (s *DetailSlider) height(_ *DetailPage, m qsMetrics) int { return m.sliderH }

func (s *DetailSlider) rowRect(_ *DetailPage, _ qsMetrics, r image.Rectangle, _ int) image.Rectangle {
	return r
}

// row — раскладка строки ползунка (та же, что у громкости на главной странице,
// без «›»).
func (s *DetailSlider) row(m qsMetrics, r image.Rectangle) qsRow {
	zone := m.sliderZone
	cy := r.Min.Y + r.Dy()/2
	out := qsRow{row: r}
	out.icon = image.Rect(r.Min.X, cy-zone/2, r.Min.X+zone, cy-zone/2+zone)
	out.value = image.Rect(r.Max.X-m.sliderValue, r.Min.Y, r.Max.X, r.Max.Y)
	left := r.Min.X
	if s.icon != nil || s.iconAt != nil {
		left = out.icon.Max.X + 8
	}
	out.track = image.Rect(left, r.Min.Y, out.value.Min.X-4, r.Max.Y)
	return out
}

// setFromX переводит координату x в уровень и возвращает вызов onChange.
func (s *DetailSlider) setFromX(p *DetailPage, m qsMetrics, r image.Rectangle, x int) func() {
	v := s.row(m, r).level(x, m.sliderThumb)
	if v == s.level {
		return nil
	}
	s.level = v
	if s.onChange == nil {
		return nil
	}
	fn := s.onChange
	return func() { fn(v) }
}

func (s *DetailSlider) draw(ctx widget.DrawContext, p *DetailPage, m qsMetrics, r image.Rectangle, idx int) {
	row := s.row(m, r)
	h := dpHit{idx, 0}
	st := theme.StateNormal
	if s.off {
		st = theme.StateDisabled
	}
	b := p.Bounds()
	if s.icon != nil || s.iconAt != nil {
		col := p.part("w11.button", st).Text
		p.drawIcon(ctx, [2]int{idx, 0}, s.icon, s.iconAt, glyphSquare(image.Rect(row.icon.Min.X+(row.icon.Dx()-m.sliderIcon)/2,
			row.icon.Min.Y+(row.icon.Dy()-m.sliderIcon)/2, row.icon.Min.X+(row.icon.Dx()-m.sliderIcon)/2+m.sliderIcon,
			row.icon.Min.Y+(row.icon.Dy()-m.sliderIcon)/2+m.sliderIcon)), col)
	}
	cy := r.Min.Y + r.Dy()/2
	th := m.sliderTrack
	track := image.Rect(row.track.Min.X, cy-th/2, row.track.Max.X, cy-th/2+th)
	qsPaint(ctx, track, p.part("w11.slider.track", st), b)
	cx := row.thumbCenter(s.level, m.sliderThumb)
	if fill := image.Rect(track.Min.X, track.Min.Y, cx, track.Max.Y); !fill.Empty() {
		qsPaint(ctx, fill, p.part("w11.slider.fill", st), b)
	}
	thumb := image.Rect(cx-m.sliderThumb/2, cy-m.sliderThumb/2, cx-m.sliderThumb/2+m.sliderThumb, cy-m.sliderThumb/2+m.sliderThumb)
	qsPaint(ctx, thumb, p.part("w11.slider.thumb", st), b)
	dot, dst := qsDotRest, st
	switch {
	case s.off:
	case p.drag && p.press == h:
		dot, dst = qsDotPressed, theme.StatePressed
	case p.hover == h:
		dot, dst = qsDotHover, theme.StateHover
	}
	ds := p.part("w11.slider.dot", dst)
	ctx.FillRoundRect(cx-dot/2, cy-dot/2, dot, dot, dot/2, ds.Fill)
	DrawTextCentered(ctx, row.value, strconv.Itoa(int(s.level*100+0.5)), p.part("w11.dim", theme.StateNormal))
}

func (s *DetailSlider) click(*DetailPage, int) func() { return nil }

func (s *DetailSlider) key(p *DetailPage, _ int, e widget.KeyEvent) (func(), bool) {
	v := s.level
	switch e.Code {
	case widget.KeyLeft:
		v -= dpKeyStep
	case widget.KeyRight:
		v += dpKeyStep
	default:
		return nil, false
	}
	v = clamp01(v)
	if v == s.level {
		return nil, true
	}
	s.level = v
	if s.onChange == nil {
		return nil, true
	}
	fn := s.onChange
	return func() { fn(v) }, true
}

// ─── Переключатель ───────────────────────────────────────────────────────────

// DetailToggle — переключатель страницы: название слева, «Вкл.» или «Откл.» и
// сам переключатель справа.
type DetailToggle struct {
	p        *DetailPage
	title    string
	on, off  bool
	onChange func(bool)
}

// AddToggle добавляет переключатель. onChange зовётся по нажатию (мышью, Enter,
// пробелом) с новым состоянием, не при SetOn.
func (p *DetailPage) AddToggle(title string, on bool, onChange func(bool)) *DetailToggle {
	t := &DetailToggle{p: p, title: title, on: on, onChange: onChange}
	p.add(t)
	return t
}

// SetOn ставит состояние, не зовя onChange.
func (t *DetailToggle) SetOn(on bool) {
	t.p.mu.Lock()
	t.on = on
	t.p.mu.Unlock()
	t.p.repaint()
}

// On возвращает состояние.
func (t *DetailToggle) On() bool {
	t.p.mu.Lock()
	defer t.p.mu.Unlock()
	return t.on
}

// SetTitle меняет название.
func (t *DetailToggle) SetTitle(s string) {
	t.p.mu.Lock()
	t.title = s
	t.p.mu.Unlock()
	t.p.repaint()
}

// SetEnabled включает и выключает переключатель.
func (t *DetailToggle) SetEnabled(on bool) {
	t.p.mu.Lock()
	t.off = !on
	t.p.mu.Unlock()
	t.p.repaint()
}

func (t *DetailToggle) enabled() bool                     { return !t.off }
func (t *DetailToggle) rows() int                         { return 1 }
func (t *DetailToggle) rowEnabled(int) bool               { return !t.off }
func (t *DetailToggle) height(*DetailPage, qsMetrics) int { return dpToggleH }
func (t *DetailToggle) rowRect(_ *DetailPage, _ qsMetrics, r image.Rectangle, _ int) image.Rectangle {
	return r
}

func (t *DetailToggle) draw(ctx widget.DrawContext, p *DetailPage, m qsMetrics, r image.Rectangle, idx int) {
	h := dpHit{idx, 0}
	b := p.Bounds()
	bs := p.part("w11.page.item", p.state(h, t.off))
	qsPaint(ctx, r, bs, b)

	sw := image.Rect(r.Max.X-dpPad-dpSwitchW, r.Min.Y+(r.Dy()-dpSwitchH)/2, r.Max.X-dpPad, r.Min.Y+(r.Dy()-dpSwitchH)/2+dpSwitchH)
	track, thumb := "w11.page.toggle", "w11.page.thumb"
	if t.on {
		track, thumb = track+".on", thumb+".on"
	}
	st := theme.StateNormal
	switch {
	case t.off:
		st = theme.StateDisabled
	case p.hover == h:
		st = theme.StateHover
	}
	qsPaint(ctx, sw, p.part(track, st), b)
	d := dpThumbRest
	if st == theme.StateHover || (p.press == h && !t.off) {
		d = dpThumbHover
	}
	tx := sw.Min.X + 4 + d/2
	if t.on {
		tx = sw.Max.X - 4 - d/2
	}
	cy := sw.Min.Y + sw.Dy()/2
	ts := p.part(thumb, st)
	ctx.FillRoundRect(tx-d/2, cy-d/2, d, d, d/2, ts.Fill)

	state := tr(StrQuickToggleOff)
	if t.on {
		state = tr(StrQuickToggleOn)
	}
	ds := p.part("w11.dim", theme.StateNormal)
	sw2 := MeasureText(ctx, state, ds)
	DrawTextLeft(ctx, image.Rect(sw.Min.X-dpStateGap-sw2, r.Min.Y, sw.Min.X-dpStateGap, r.Max.Y), state, ds)
	ls := p.part("w11.label", p.state(dpNone, t.off))
	DrawTextLeft(ctx, image.Rect(r.Min.X+dpPad, r.Min.Y, sw.Min.X-dpStateGap-sw2-dpStateGap, r.Max.Y),
		Elide(ctx, t.title, ls, sw.Min.X-dpStateGap-sw2-dpStateGap-r.Min.X-dpPad), ls)
}

func (t *DetailToggle) click(*DetailPage, int) func() {
	t.on = !t.on
	if t.onChange == nil {
		return nil
	}
	fn, on := t.onChange, t.on
	return func() { fn(on) }
}

func (t *DetailToggle) key(p *DetailPage, sub int, e widget.KeyEvent) (func(), bool) {
	if e.Code == widget.KeyEnter || e.Code == widget.KeySpace {
		return t.click(p, sub), true
	}
	return nil, false
}

// ─── Кнопка ──────────────────────────────────────────────────────────────────

// DetailButton — кнопка страницы на всю ширину.
type DetailButton struct {
	p      *DetailPage
	title  string
	accent bool
	off    bool
	fn     func()
}

// AddButton добавляет кнопку; onClick зовётся по нажатию (мышь, Enter, пробел).
func (p *DetailPage) AddButton(title string, onClick func()) *DetailButton {
	b := &DetailButton{p: p, title: title, fn: onClick}
	p.add(b)
	return b
}

// SetTitle меняет надпись.
func (b *DetailButton) SetTitle(s string) {
	b.p.mu.Lock()
	b.title = s
	b.p.mu.Unlock()
	b.p.repaint()
}

// SetAccent делает кнопку акцентной (главное действие страницы).
func (b *DetailButton) SetAccent(on bool) {
	b.p.mu.Lock()
	b.accent = on
	b.p.mu.Unlock()
	b.p.repaint()
}

// SetEnabled включает и выключает кнопку.
func (b *DetailButton) SetEnabled(on bool) {
	b.p.mu.Lock()
	b.off = !on
	b.p.mu.Unlock()
	b.p.repaint()
}

func (b *DetailButton) enabled() bool                     { return !b.off }
func (b *DetailButton) rows() int                         { return 1 }
func (b *DetailButton) rowEnabled(int) bool               { return !b.off }
func (b *DetailButton) height(*DetailPage, qsMetrics) int { return dpButtonH }
func (b *DetailButton) rowRect(_ *DetailPage, _ qsMetrics, r image.Rectangle, _ int) image.Rectangle {
	return r
}

func (b *DetailButton) draw(ctx widget.DrawContext, p *DetailPage, _ qsMetrics, r image.Rectangle, idx int) {
	part := "w11.page.button"
	if b.accent {
		part += ".accent"
	}
	s := p.part(part, p.state(dpHit{idx, 0}, b.off))
	qsPaint(ctx, r, s, p.Bounds())
	DrawTextCentered(ctx, r, Elide(ctx, b.title, s, r.Dx()-2*dpPad), s)
}

func (b *DetailButton) click(*DetailPage, int) func() { return b.fn }

func (b *DetailButton) key(p *DetailPage, sub int, e widget.KeyEvent) (func(), bool) {
	if e.Code == widget.KeyEnter || e.Code == widget.KeySpace {
		return b.fn, true
	}
	return nil, false
}

// ─── Подпись ─────────────────────────────────────────────────────────────────

// DetailText — строка пояснения или заголовок группы элементов.
type DetailText struct {
	p    *DetailPage
	text string
}

// AddText добавляет строку приглушённого текста.
func (p *DetailPage) AddText(text string) *DetailText {
	t := &DetailText{p: p, text: text}
	p.add(t)
	return t
}

// SetText меняет текст.
func (t *DetailText) SetText(s string) {
	t.p.mu.Lock()
	t.text = s
	t.p.mu.Unlock()
	t.p.repaint()
}

func (t *DetailText) enabled() bool                     { return false }
func (t *DetailText) rows() int                         { return 0 }
func (t *DetailText) rowEnabled(int) bool               { return false }
func (t *DetailText) height(*DetailPage, qsMetrics) int { return dpTextH }
func (t *DetailText) rowRect(*DetailPage, qsMetrics, image.Rectangle, int) image.Rectangle {
	return image.Rectangle{}
}
func (t *DetailText) draw(ctx widget.DrawContext, p *DetailPage, _ qsMetrics, r image.Rectangle, _ int) {
	s := p.part("w11.dim", theme.StateNormal)
	DrawTextLeft(ctx, image.Rect(r.Min.X+dpPad, r.Min.Y, r.Max.X, r.Max.Y), Elide(ctx, t.text, s, r.Dx()-dpPad), s)
}
func (t *DetailText) click(*DetailPage, int) func() { return nil }
func (t *DetailText) key(*DetailPage, int, widget.KeyEvent) (func(), bool) {
	return nil, false
}
