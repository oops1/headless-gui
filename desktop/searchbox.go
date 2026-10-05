// searchbox.go — строка поиска в панели задач (Windows 10) и интерфейс
// поставщика результатов.
//
// Строка — обычный элемент области SlotStart, следующий за кнопкой «Пуск»:
//
//	box := desktop.NewSearchBox(tm, provider)
//	box.Bind(menu, startButton.Bounds)       // результаты — в меню «Пуск»
//	bar.AddItem(desktop.SlotStart, box)
//
// Она показывается тремя способами (SearchMode): скрыта, одним значком или
// полем ввода. Ширина поля и значка, поля и размер лупы — метрики темы
// (search.width, search.icon.width, search.pad, search.icon.size,
// search.icon.gap), цвета и рамка — стиль компонента «searchbox» (части ""
// поле, "hint" подсказка, "icon" кнопка-значок). Ни имени темы, ни литералов
// размеров в отрисовке нет.
//
// Сама строка ничего не ищет: запрос уходит поставщику потребителя
// (SearchProvider), результаты приходят от него же и показываются меню
// «Пуск» вместо списка приложений (StartMenu.SetSearchProvider). Подсказка
// берётся из строки интерфейса desktop.search.placeholder и следует за
// языком без пересоздания.
package desktop

import (
	"image"
	"strings"
	"sync"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ComponentSearchBox — имя компонента для стилей темы.
const ComponentSearchBox = "searchbox"

// Ключи метрик строки поиска.
const (
	// KeySearchWidth — ширина поля ввода.
	KeySearchWidth theme.Key = "search.width"
	// KeySearchIconWidth — ширина кнопки в режиме «только значок».
	KeySearchIconWidth theme.Key = "search.icon.width"
	// KeySearchIconSize — сторона лупы.
	KeySearchIconSize theme.Key = "search.icon.size"
	// KeySearchPad — поле слева от лупы.
	KeySearchPad theme.Key = "search.pad"
	// KeySearchIconGap — зазор между лупой и текстом.
	KeySearchIconGap theme.Key = "search.icon.gap"
	// KeySearchHeight — высота поля и кнопки «значок и подпись», по центру
	// высоты панели (Windows 11: 32 в панели 48). 0 — решает панель, как раньше.
	KeySearchHeight theme.Key = "search.height"
	// KeySearchLabelWidth — ширина кнопки «значок и подпись»; 0 — как у поля
	// (search.width).
	KeySearchLabelWidth theme.Key = "search.label.width"
)

// SearchMode — как строка поиска показывается на панели.
type SearchMode int

const (
	// SearchModeHidden — строки нет, место не занято.
	SearchModeHidden SearchMode = iota
	// SearchModeIconOnly — только значок лупы, кнопка шириной search.icon.width.
	SearchModeIconOnly
	// SearchModeBox — поле ввода шириной search.width.
	SearchModeBox
	// SearchModeIconAndLabel — кнопка «значок и подпись» (Windows 11): лупа и
	// слово «Поиск» на плашке поля шириной search.label.width. Набор с
	// клавиатуры, щелчок и Enter открывают поиск так же, как у остальных видов.
	SearchModeIconAndLabel
)

// SearchResult — один результат поиска.
type SearchResult struct {
	// ID — идентификатор результата для SearchProvider.Activate.
	ID       string
	Title    string
	Subtitle string
	// Category — необязательная категория («Приложения», «Документы»).
	Category string
	Icon     image.Image
	// IconAt — значок по размеру в физических пикселях (см. AppInfo.IconAt).
	IconAt func(size int) image.Image
}

// SearchProvider — поиск потребителя. Строка поиска и меню «Пуск» знают о нём
// только это: запрос уходит в SetQuery, результаты читаются из Results.
//
// Поставщик может отвечать сразу или позже: после изменения результатов он
// зовёт подписчика (Subscribe) из своей горутины, меню перерисует список.
// Results и Activate зовутся из горутины кадра и должны быть быстрыми.
type SearchProvider interface {
	// SetQuery сообщает новый запрос; пустая строка — запрос сброшен.
	SetQuery(query string)
	// Results возвращает результаты последнего запроса в порядке показа.
	Results() []SearchResult
	// Activate открывает результат (запускает приложение, файл…).
	Activate(r SearchResult) error
	Subscribe(func()) func()
}

// SearchBox — элемент панели задач: строка поиска. Нулевое значение не годится,
// создавай через NewSearchBox.
type SearchBox struct {
	widget.Base
	FocusState

	tm       *theme.Manager
	provider SearchProvider

	// OnQueryChange вызывается при каждом изменении текста. Вызывается после
	// отпускания замка элемента.
	OnQueryChange func(query string)
	// OnSubmit вызывается по Enter, когда меню «Пуск» не привязано или в его
	// результатах нечего открыть.
	OnSubmit func(query string)
	// OnActivate вызывается нажатием на строку или значок. По умолчанию (если
	// привязано меню) открывает меню «Пуск» в режиме поиска.
	OnActivate func()

	// relayout просит панель задач переложить элементы (см. SetRelayout).
	relayout func()

	mu      sync.Mutex
	mode    SearchMode
	text    []rune
	caret   int
	hovered bool
	armed   bool

	menu   *StartMenu
	anchor func() image.Rectangle

	fade motion
}

// NewSearchBox создаёт строку поиска в режиме SearchModeBox. provider может быть
// nil: тогда запрос сообщается только через OnQueryChange.
func NewSearchBox(tm *theme.Manager, provider SearchProvider) *SearchBox {
	return &SearchBox{tm: tm, provider: provider, mode: SearchModeBox}
}

var (
	_ Item             = (*SearchBox)(nil)
	_ widget.Focusable = (*SearchBox)(nil)
)

// Mode возвращает способ показа.
func (b *SearchBox) Mode() SearchMode {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.mode
}

// SetMode выбирает способ показа. Ширина элемента меняется, и панель, в которую
// он добавлен (Taskbar.AddItem), перекладывает элементы сама.
func (b *SearchBox) SetMode(m SearchMode) {
	b.mu.Lock()
	changed := b.mode != m
	b.mode = m
	relayout := b.relayout
	b.mu.Unlock()
	if !changed {
		return
	}
	if relayout != nil {
		relayout()
	}
	b.Invalidate()
}

// SetRelayout принимает от панели задач функцию перекладки (Taskbar.AddItem
// зовёт её для каждого элемента, который её просит).
func (b *SearchBox) SetRelayout(fn func()) {
	b.mu.Lock()
	b.relayout = fn
	b.mu.Unlock()
}

// Provider возвращает поставщика результатов.
func (b *SearchBox) Provider() SearchProvider { return b.provider }

// Text возвращает введённый запрос.
func (b *SearchBox) Text() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.text)
}

// SetText ставит запрос программно. Сообщает поставщику и меню так же, как
// набор с клавиатуры.
func (b *SearchBox) SetText(s string) { b.setText(s, true) }

// Bind связывает строку с меню «Пуск»: запрос показывается в меню, а меню
// открывается набором текста и нажатием на строку. anchor возвращает
// прямоугольник кнопки «Пуск», от которой раскрывается меню (обычно
// startButton.Bounds). Меню получает поставщика строки, если у него своего
// нет.
func (b *SearchBox) Bind(menu *StartMenu, anchor func() image.Rectangle) {
	b.mu.Lock()
	b.menu, b.anchor = menu, anchor
	b.mu.Unlock()
	if menu != nil {
		menu.bindSearchBox(b)
		if b.provider != nil && menu.SearchProvider() == nil {
			menu.SetSearchProvider(b.provider)
		}
	}
}

// PreferredSize реализует Item. Скрытая строка просит нулевую ширину, и панель
// не оставляет ей места.
func (b *SearchBox) PreferredSize(avail image.Point) image.Point {
	b.mu.Lock()
	mode := b.mode
	b.mu.Unlock()
	w := 0
	switch mode {
	case SearchModeIconOnly:
		w = b.metric(KeySearchIconWidth)
	case SearchModeBox:
		w = b.metric(KeySearchWidth)
	case SearchModeIconAndLabel:
		w = b.metric(KeySearchLabelWidth)
		if w <= 0 {
			w = b.metric(KeySearchWidth)
		}
	}
	if avail.X > 0 && w > avail.X {
		w = avail.X
	}
	if w < 0 {
		w = 0
	}
	// Поле и кнопка с подписью ниже кнопок панели (Windows 11: 32 против 40);
	// значок и всё прежнее высоты не просят.
	h := 0
	if mode == SearchModeBox || mode == SearchModeIconAndLabel {
		h = b.metric(KeySearchHeight)
	}
	return image.Pt(w, h)
}

// GetToolTip перекрывает промоутнутый из widget.Base: у кнопки-значка подсказка
// («Поиск») берётся при каждом показе, поэтому следует за языком. У поля
// подсказки нет — его роль играет заполнитель.
func (b *SearchBox) GetToolTip() string {
	if b.Mode() == SearchModeIconOnly {
		return tr(StrSearchLabel)
	}
	return ""
}

// SetBounds скрытой строке оставляет пустые границы: невидимая остановка Tab и
// мёртвая зона под курсором ни к чему.
func (b *SearchBox) SetBounds(r image.Rectangle) {
	if b.Mode() == SearchModeHidden {
		r = image.Rectangle{}
	}
	b.Base.SetBounds(r)
}

// ─── Ввод текста ─────────────────────────────────────────────────────────────

// setText заменяет запрос. notify — сообщать ли меню и поставщику (меню само
// сбрасывает строку при закрытии и не должно получать этого обратно).
func (b *SearchBox) setText(s string, notify bool) {
	b.mu.Lock()
	if string(b.text) == s {
		b.mu.Unlock()
		return
	}
	b.text = []rune(s)
	b.caret = len(b.text)
	menu, anchor, cb := b.menu, b.anchor, b.OnQueryChange
	b.mu.Unlock()
	b.Invalidate()
	if !notify {
		return
	}
	// Меню, у которого тот же поставщик, сообщит ему запрос само: дважды один
	// запрос поиск выполнять не должен.
	if b.provider != nil && (menu == nil || menu.SearchProvider() != b.provider) {
		b.provider.SetQuery(s)
	}
	if cb != nil {
		cb(s)
	}
	if menu != nil {
		menu.setQueryFromBox(s)
		if s != "" && !menu.IsOpen() && anchor != nil {
			menu.openForSearch(anchor())
		}
	}
}

// clearSilently очищает запрос, не сообщая о нём меню (его закрытие).
func (b *SearchBox) clearSilently() {
	b.mu.Lock()
	had := len(b.text) > 0
	b.text, b.caret = nil, 0
	b.mu.Unlock()
	if had {
		if b.provider != nil {
			b.provider.SetQuery("")
		}
		if b.OnQueryChange != nil {
			b.OnQueryChange("")
		}
		b.Invalidate()
	}
}

// insertRune вставляет символ в позицию каретки.
func (b *SearchBox) insertRune(r rune) {
	b.mu.Lock()
	text := append(append(append([]rune(nil), b.text[:b.caret]...), r), b.text[b.caret:]...)
	caret := b.caret + 1
	b.mu.Unlock()
	b.setText(string(text), true)
	b.mu.Lock()
	b.caret = caret
	b.mu.Unlock()
}

// SetFocused реализует widget.Focusable.
func (b *SearchBox) SetFocused(v bool) {
	if b.FocusState.Set(v) {
		b.Invalidate()
	}
}

// TabIndex исключает скрытую строку из обхода.
func (b *SearchBox) TabIndex() int { return focusTabIndex(b) }

// OnKeyEvent: набор текста, правка, Enter, Esc; Вверх/Вниз/PageUp/PageDown
// уходят в список результатов меню, если оно привязано и открыто.
func (b *SearchBox) OnKeyEvent(e widget.KeyEvent) {
	if !e.Pressed {
		return
	}
	if b.FocusState.noteKey() {
		b.Invalidate()
	}
	if e.Mod&(widget.ModCtrl|widget.ModAlt|widget.ModMeta) != 0 {
		if e.Mod&widget.ModCtrl != 0 && e.Code == widget.KeyV {
			if clip := strings.TrimSpace(strings.ReplaceAll(widget.ClipboardGetText(), "\n", " ")); clip != "" {
				b.SetText(b.Text() + clip)
			}
		}
		return
	}

	b.mu.Lock()
	menu, mode := b.menu, b.mode
	n, caret := len(b.text), b.caret
	b.mu.Unlock()

	// Значок и «значок и подпись» без введённого текста — кнопка: Enter и Space
	// открывают поиск, как щелчок, а стрелки, Home и End переносят фокус к соседу
	// по области (в поле они двигают каретку). Печатный символ по-прежнему идёт в
	// запрос и открывает поиск.
	if (mode == SearchModeIconOnly || mode == SearchModeIconAndLabel) && n == 0 &&
		(menu == nil || !menu.IsOpen()) {
		if b.HandleKey(b, e, b.activate, b.Invalidate) {
			return
		}
	}

	switch e.Code {
	case widget.KeyUp, widget.KeyDown, widget.KeyPageUp, widget.KeyPageDown:
		if menu != nil && menu.IsOpen() && menu.SearchKey(e) {
			return
		}
	case widget.KeyEnter:
		if menu != nil && menu.IsOpen() && menu.SearchKey(e) {
			return
		}
		if b.OnSubmit != nil {
			b.OnSubmit(b.Text())
		}
		return
	case widget.KeyEscape:
		if menu != nil && menu.IsOpen() {
			menu.Close()
			return
		}
		b.setText("", true)
		return
	case widget.KeyBackspace:
		if caret > 0 {
			b.edit(caret-1, caret)
		}
		return
	case widget.KeyDelete:
		if caret < n {
			b.edit(caret, caret+1)
		}
		return
	case widget.KeyLeft, widget.KeyRight, widget.KeyHome, widget.KeyEnd:
		b.moveCaret(e.Code, n, caret)
		return
	}
	if e.Rune >= 32 && e.Rune != 127 {
		b.insertRune(e.Rune)
	}
}

// edit удаляет символы [from, to) и ставит каретку на from.
func (b *SearchBox) edit(from, to int) {
	b.mu.Lock()
	text := append(append([]rune(nil), b.text[:from]...), b.text[to:]...)
	b.mu.Unlock()
	b.setText(string(text), true)
	b.mu.Lock()
	b.caret = from
	b.mu.Unlock()
	b.Invalidate()
}

func (b *SearchBox) moveCaret(code widget.KeyCode, n, caret int) {
	switch code {
	case widget.KeyLeft:
		caret--
	case widget.KeyRight:
		caret++
	case widget.KeyHome:
		caret = 0
	case widget.KeyEnd:
		caret = n
	}
	if caret < 0 {
		caret = 0
	}
	if caret > n {
		caret = n
	}
	b.mu.Lock()
	changed := b.caret != caret
	b.caret = caret
	b.mu.Unlock()
	if changed {
		b.Invalidate()
	}
}

// ─── Мышь ────────────────────────────────────────────────────────────────────

// OnMouseMove обновляет наведение.
func (b *SearchBox) OnMouseMove(x, y int) {
	over := image.Pt(x, y).In(b.Bounds())
	b.mu.Lock()
	changed := b.hovered != over
	b.hovered = over
	b.mu.Unlock()
	if changed {
		b.Invalidate()
	}
}

// OnMouseButton: нажатие ставит каретку и взводит строку, отпускание над ней —
// открывает меню (или зовёт OnActivate).
func (b *SearchBox) OnMouseButton(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft {
		return false
	}
	over := image.Pt(e.X, e.Y).In(b.Bounds())
	if e.Pressed {
		if !over {
			return false
		}
		b.NotePointer(e)
		b.mu.Lock()
		b.armed = true
		if b.mode == SearchModeBox {
			b.caret = b.caretAt(e.X)
		}
		b.mu.Unlock()
		b.Invalidate()
		return true
	}
	b.mu.Lock()
	was := b.armed
	b.armed = false
	menu, anchor, cb := b.menu, b.anchor, b.OnActivate
	b.mu.Unlock()
	b.Invalidate()
	if was && over {
		if cb != nil {
			cb()
		} else if menu != nil && anchor != nil && !menu.DismissedByAnchor() {
			menu.openForSearch(anchor())
		}
	}
	return was
}

// activate делает то же, что щелчок по строке: зовёт OnActivate, а без него
// открывает меню «Пуск» в режиме поиска.
func (b *SearchBox) activate() {
	b.mu.Lock()
	menu, anchor, cb := b.menu, b.anchor, b.OnActivate
	b.mu.Unlock()
	if cb != nil {
		cb()
	} else if menu != nil && anchor != nil {
		menu.openForSearch(anchor())
	}
}

// caretAt переводит координату щелчка в позицию каретки. Под b.mu.
func (b *SearchBox) caretAt(x int) int {
	field, _ := b.fieldRect()
	rel := x - field.Min.X
	size := b.fontSize(b.style(theme.StateNormal))
	best := 0
	for i := 1; i <= len(b.text); i++ {
		if widget.MeasureUIText(string(b.text[:i]), size) > rel {
			break
		}
		best = i
	}
	// Щелчок правее середины символа ставит каретку за ним.
	if best < len(b.text) {
		lo := widget.MeasureUIText(string(b.text[:best]), size)
		hi := widget.MeasureUIText(string(b.text[:best+1]), size)
		if rel > (lo+hi)/2 {
			best++
		}
	}
	return best
}

// ─── Отрисовка ───────────────────────────────────────────────────────────────

func (b *SearchBox) metric(k theme.Key) int {
	if b.tm == nil {
		return 0
	}
	return int(b.tm.GetMetric(k))
}

func (b *SearchBox) style(st theme.State) *theme.Style {
	return styleOf(b.tm, ComponentSearchBox, "", st)
}

func (b *SearchBox) part(part string, st theme.State) *theme.Style {
	return styleOf(b.tm, ComponentSearchBox, part, st)
}

func (b *SearchBox) fontSize(s *theme.Style) float64 {
	if s != nil && s.Font.Size > 0 {
		return s.Font.Size
	}
	return widget.DefaultFontSizePt
}

// fieldRect — область текста внутри поля (за вычетом лупы и полей) и
// координата лупы по X.
func (b *SearchBox) fieldRect() (field image.Rectangle, iconX int) {
	bd := b.Bounds()
	pad := b.metric(KeySearchPad)
	icon := b.metric(KeySearchIconSize)
	gap := b.metric(KeySearchIconGap)
	iconX = bd.Min.X + pad
	left := iconX + icon + gap
	return image.Rect(left, bd.Min.Y, bd.Max.X-pad/2, bd.Max.Y), iconX
}

// Draw рисует поле или кнопку-значок. Скрытая строка (пустые границы) не
// рисуется.
func (b *SearchBox) Draw(ctx widget.DrawContext) {
	bd := b.Bounds()
	if bd.Empty() {
		return
	}
	b.mu.Lock()
	mode, text, caret := b.mode, append([]rune(nil), b.text...), b.caret
	hovered, armed := b.hovered, b.armed
	b.mu.Unlock()
	focused := b.IsFocused()
	iconSize := b.metric(KeySearchIconSize)

	if mode == SearchModeIconOnly {
		st := StateOf(hovered, armed, false, false, b.FocusVisible())
		s := b.fade.ItemStyle(b.tm, 0, bd, st, func(st theme.State) *theme.Style { return b.part("icon", st) })
		PaintStyle(ctx, bd, s)
		side := image.Rect(0, 0, iconSize, iconSize).Add(image.Pt(
			bd.Min.X+(bd.Dx()-iconSize)/2, bd.Min.Y+(bd.Dy()-iconSize)/2))
		drawGlyph(ctx, glyphSearch, side, s.Text)
		return
	}

	st := StateOf(hovered, armed, false, false, false)
	if focused {
		st = theme.StateActive
	}
	s := b.fade.ItemStyle(b.tm, 0, bd, st, func(st theme.State) *theme.Style { return b.style(st) })
	PaintStyle(ctx, bd, s)

	field, iconX := b.fieldRect()
	iconRect := image.Rect(iconX, bd.Min.Y+(bd.Dy()-iconSize)/2, iconX+iconSize, bd.Min.Y+(bd.Dy()-iconSize)/2+iconSize)
	drawGlyph(ctx, glyphSearch, iconRect, s.Text)

	size := b.fontSize(s)
	prev := ctx.Clip()
	ctx.SetClip(field.Intersect(prev))
	defer ctx.SetClip(prev)

	// «Значок и подпись»: вместо поля слово «Поиск» подсказкой, без каретки.
	if mode == SearchModeIconAndLabel {
		hs := *b.part("hint", theme.StateNormal)
		hs.Font = s.Font
		drawTextAt(ctx, field, field.Min.X, tr(StrSearchLabel), &hs)
		return
	}

	if len(text) == 0 {
		hint := b.part("hint", theme.StateNormal)
		hs := *hint
		hs.Font = s.Font
		drawTextAt(ctx, field, field.Min.X, b.placeholder(), &hs)
		if focused {
			drawCaret(ctx, field, field.Min.X, size, s)
		}
		return
	}
	// Правее поля не уходим: длинный запрос сдвигается так, чтобы каретка
	// оставалась видна.
	full := string(text)
	caretX := measureAt(ctx, string(text[:caret]), s)
	shift := 0
	if over := caretX - (field.Dx() - 2); over > 0 {
		shift = over
	}
	x := field.Min.X - shift
	drawTextAt(ctx, field, x, full, s)
	if focused {
		drawCaret(ctx, field, x+caretX, size, s)
	}
}

// KeySearchShortHint — флаг темы: пустое поле показывает короткое слово «Поиск»
// (Windows 11), а не длинную подсказку «Чтобы начать поиск, введите здесь запрос»
// (Windows 10), которая в поле шириной 200 не помещается.
const KeySearchShortHint theme.Key = "search.hint.short"

// placeholder — подсказка пустого поля: длинная или, по флагу темы, короткая.
func (b *SearchBox) placeholder() string {
	if b.tm != nil && b.tm.GetFlag(KeySearchShortHint, false) {
		return tr(StrSearchLabel)
	}
	return tr(StrSearchPlaceholder)
}

// drawTextAt рисует строку со смещением x по вертикальному центру r.
func drawTextAt(ctx widget.DrawContext, r image.Rectangle, x int, text string, s *theme.Style) {
	if text == "" || s == nil {
		return
	}
	size := s.Font.Size
	if size <= 0 {
		size = widget.DefaultFontSizePt
	}
	y := r.Min.Y + (r.Dy()-int(size*1.4))/2
	drawText(ctx, text, x, y, size, s)
}

// measureAt — ширина строки шрифтом стиля.
func measureAt(ctx widget.DrawContext, text string, s *theme.Style) int {
	return MeasureText(ctx, text, s)
}

// drawCaret рисует каретку-палочку высотой в строку текста.
func drawCaret(ctx widget.DrawContext, field image.Rectangle, x int, size float64, s *theme.Style) {
	h := int(size * 1.5)
	if h > field.Dy() {
		h = field.Dy()
	}
	y := field.Min.Y + (field.Dy()-h)/2
	ctx.FillRect(x, y, 1, h, s.Text)
}

// FocusRing реализует FocusRinger: рамка обводит всю строку.
func (b *SearchBox) FocusRing() (image.Rectangle, *theme.Style) {
	part := ""
	if b.Mode() == SearchModeIconOnly {
		part = "icon"
	}
	return b.Bounds(), styleOf(b.tm, ComponentSearchBox, part, theme.StateNormal)
}

// ─── Поставщик для тестов и демонстрации ─────────────────────────────────────

// FakeSearchProvider — поиск по списку записей подстрокой названия без учёта
// регистра. Запоминает запросы и открытые результаты для тестов.
type FakeSearchProvider struct {
	mu      sync.Mutex
	items   []SearchResult
	query   string
	subs    map[int]func()
	nextSub int

	// Queries — журнал запросов; Activated — открытые результаты.
	Queries   []string
	Activated []string
	// ActivateErr — если задана, Activate возвращает её.
	ActivateErr error
}

// NewFakeSearchProvider создаёт поставщика над списком записей.
func NewFakeSearchProvider(items ...SearchResult) *FakeSearchProvider {
	return &FakeSearchProvider{items: items, subs: map[int]func(){}}
}

// SetQuery запоминает запрос и оповещает подписчиков.
func (p *FakeSearchProvider) SetQuery(q string) {
	p.mu.Lock()
	p.query = q
	p.Queries = append(p.Queries, q)
	subs := make([]func(), 0, len(p.subs))
	for _, f := range p.subs {
		subs = append(subs, f)
	}
	p.mu.Unlock()
	for _, f := range subs {
		f()
	}
}

// Results возвращает записи, в названии которых есть запрос.
func (p *FakeSearchProvider) Results() []SearchResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.query == "" {
		return nil
	}
	q := strings.ToLower(p.query)
	var out []SearchResult
	for _, it := range p.items {
		if strings.Contains(strings.ToLower(it.Title), q) {
			out = append(out, it)
		}
	}
	return out
}

// Activate записывает открытый результат.
func (p *FakeSearchProvider) Activate(r SearchResult) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Activated = append(p.Activated, r.ID)
	return p.ActivateErr
}

// Subscribe подписывает на смену результатов.
func (p *FakeSearchProvider) Subscribe(fn func()) func() {
	p.mu.Lock()
	p.nextSub++
	id := p.nextSub
	p.subs[id] = fn
	p.mu.Unlock()
	return func() {
		p.mu.Lock()
		delete(p.subs, id)
		p.mu.Unlock()
	}
}
