package widget

import (
	"image"
	"image/color"
	"sync"
	"sync/atomic"
	"time"
)

// MenuItem описывает один пункт контекстного / popup-меню.
type MenuItem struct {
	Text string // текст пункта

	// Icon — значок пункта картинкой.
	//
	// Раньше поле было строкой-путём и не рисовалось вовсе. Путь здесь не
	// годится: значки приложения обычно вкомпилированы (SVG, растеризованный
	// в цвет темы), файла на диске у них нет — а картинку отдать можно любую.
	//
	// Зона под значок отводится ВСЕМУ меню, если значок есть хоть у одного
	// пункта: иначе подписи разъезжались бы по левому краю (то же правило, что
	// у Checkable).
	Icon image.Image
	// IconHover — значок пункта, когда он под плашкой наведения. Нужен, если
	// значок не одноцветный: тёмный на синей плашке классики не виден. Не
	// задан — рисуется Icon.
	IconHover image.Image
	// IconTint — значок одноцветный: рисуется цветом текста пункта в его
	// текущем состоянии (обычный, под плашкой, недоступный), по альфа-каналу
	// картинки. Один рисунок тогда годится для всех состояний.
	IconTint bool
	// IconSize — сторона значка в точках; 0 — по высоте пункта.
	IconSize int
	// IconPath — путь, из которого значок загрузила разметка (XAML Icon="…").
	// Движок его не читает: поле для приложения, которому нужно знать источник.
	IconPath string

	// Shortcut — сочетание клавиш пункта, написанное справа приглушённым
	// цветом: «Ctrl+S», «Ctrl+Shift+N», «Del».
	//
	// Меню его только ПОКАЗЫВАЕТ — обрабатывает сочетание приложение, через
	// InputBindings или свой обработчик клавиш. Иначе одно и то же
	// сочетание пришлось бы объявлять дважды, и однажды они разошлись бы:
	// в меню написано одно, работает другое.
	Shortcut string

	Separator bool       // true — горизонтальный разделитель вместо текста
	Disabled  bool       // серый, некликабельный пункт
	OnClick   func()     // обработчик
	SubItems  []MenuItem // вложенные подменю (3+ уровень)

	// Checkable — пункт-переключатель: слева от подписи отводится место под
	// отметку, даже когда она снята. Без этого поля соседние пункты одного
	// меню разъезжались бы по левому краю, стоило отметить один из них.
	//
	// Отводится место сразу всему меню, если хоть один пункт помечен
	// Checkable: так же ведёт себя WPF, и так подписи стоят в столбик.
	Checkable bool
	// Checked — отметка стоит. Осмысленно только при Checkable.
	Checked bool
	// RadioGroup — имя группы взаимного исключения. Пункты с одинаковым
	// непустым именем В ОДНОМ подменю ведут себя как переключатель: отметка
	// одного снимает её с остальных (SetItemChecked, PopupMenu.SetItemChecked).
	// Пустое имя — обычный флажок, независимый от соседей.
	RadioGroup string
}

// PopupMenu — контекстное / всплывающее меню в стиле Windows 10.
//
// Меню рисуется как overlay поверх всего дерева виджетов и автоматически
// закрывается при клике за пределами или при нажатии Escape.
// Поддерживает каскадные вложенные подменю (SubItems).
type PopupMenu struct {
	Base

	mu    sync.RWMutex
	items []MenuItem

	// Положение popup (абсолютные координаты) и каскадное дочернее подменю.
	// Пишутся из обработчиков событий (Show/openChild/closeChild), читаются
	// рендер-горутиной (DrawOverlay/OverlayBounds) — под отдельным geoMu, чтобы
	// не смешивать с m.mu (items) и не ловить гонку по memory model (SEC-18).
	// Порядок захвата: m.mu → geoMu (никогда наоборот).
	geoMu          sync.Mutex
	popupX, popupY int
	popupW, popupH int
	// Прокрутка длинного меню (под geoMu): пикселей содержимого над видимой
	// частью, высота содержимого, включена ли прокрутка.
	scroll, contentH int
	scrollable       bool
	scrollDir        int        // -1/+1 — курсор на стрелке края, 0 — нет
	scrollAnim       *Animation // повтор прокрутки, пока курсор на стрелке
	// Отложенное переключение подменю (под geoMu): пункт, над которым ждём.
	switchIdx  int
	switchAnim *Animation
	// applied — что профиль объявил при последнем ApplyMenuStyle.
	applied MenuStyle

	open     int32 // 1 — показано, 0 — скрыто (атомарно)
	// dismissSeq — номер нажатия (CurrentPressSeq), которым меню было
	// погашено через Dismiss. Атомарно.
	dismissSeq uint64
	hoverIdx int32 // индекс пункта под курсором (-1 = нет)

	// Каскадное дочернее подменю (под geoMu).
	child       *PopupMenu // текущее открытое вложенное подменю
	childForIdx int        // индекс пункта, для которого открыт child (-1 = нет)
	parent      *PopupMenu // родительское меню (nil для корневого)

	// Стиль.
	Background     color.RGBA
	BorderColor    color.RGBA
	TextColor      color.RGBA
	DisabledColor  color.RGBA
	HoverBG        color.RGBA
	HoverTextColor color.RGBA // текст пункта под курсором (Win2000 — белый на navy)
	SeparatorColor color.RGBA
	ShadowColor    color.RGBA

	ItemHeight   int // высота обычного пункта (по умолчанию 30)
	SeparatorH   int // высота разделителя (по умолчанию 9)
	PaddingX     int // горизонтальный отступ текста
	MinWidth     int // минимальная ширина меню
	ArrowPadding int // отступ для стрелки ► справа

	// Поля ниже заполняет профиль темы (ApplyMenuStyle, theme.KeyMenu*). Ноль
	// везде означает прежнее поведение.

	// PadLeft — поле от края меню до отметки/значка; PadRight — от сочетания
	// клавиш до края. 0 — PaddingX.
	PadLeft, PadRight int
	// PaddingY — отступ первого пункта от верхней рамки и последнего от
	// нижней. 0 — 2.
	PaddingY int
	// ItemInset — отступ плашки наведения от краёв меню. 0 — 2.
	ItemInset int
	// SeparatorInset — отступ линии разделителя от краёв. 0 — 8.
	SeparatorInset int
	// CornerRadius — скругление меню (и тени); ItemCorner — плашки наведения.
	CornerRadius, ItemCorner int
	// Elevation — высота над подложкой: > 0 заменяет прямоугольную тень со
	// смещением мягкой (по ShadowColor), если контекст умеет ShadowDrawer.
	Elevation float64
	// ShortcutColor — цвет сочетаний клавиш. Прозрачный — приглушённый
	// оттенок цвета текста. Под плашкой с другим цветом текста сочетание
	// следует за ним.
	ShortcutColor color.RGBA
	// IconSize — сторона значка по умолчанию (0 — по высоте пункта); IconGap —
	// зазор между значком и подписью (0 — 8).
	IconSize, IconGap int
	// ChevronRight > 0 включает тонкий шеврон подменю «›» вместо глифа «▸»;
	// число — расстояние от правого края меню до его середины.
	// ChevronSize — высота шеврона (0 — 8).
	ChevronRight, ChevronSize int
	// TintIcons — все значки меню одноцветные (см. MenuItem.IconTint).
	TintIcons bool
	// SubMenuMinWidth — наименьшая ширина подменю. 0 — как у родителя
	// (MinWidth, прежнее поведение); отрицательное — без минимума.
	SubMenuMinWidth int
	// SubMenuDelay — задержка раскрытия и закрытия подменю при наведении, мс.
	// 0 — сразу. Нажатие и клавиши задержку не ждут.
	SubMenuDelay int
	// WorkArea — область (в координатах холста), за которую меню не выходит:
	// экран без панели задач. Пусто — SetPopupWorkArea, а если и она пуста —
	// весь холст. Подменю наследуют.
	WorkArea image.Rectangle
	// MaxHeight — наибольшая высота меню; длиннее — прокрутка (колесо,
	// стрелки на концах, клавиши). 0 — высота доступной области.
	MaxHeight int

	// UseMnemonics — читать в подписях пунктов мнемоники: «_Файл» рисуется
	// как «Файл» с чертой под Ф, и нажатие этой буквы при открытом меню
	// выбирает пункт. Двойное подчёркивание означает сам знак подчёркивания.
	//
	// По умолчанию выключено: в подписях бывают настоящие подчёркивания —
	// имена файлов в контекстном меню, — и молча съедать их нельзя.
	UseMnemonics bool

	// OnSelect вызывается при выборе пункта (index, text).
	OnSelect func(index int, text string)
}

// ─── Размер экрана (для удержания меню в пределах канваса) ───────────────────

var (
	screenMu     sync.Mutex
	screenWidth  int
	screenHeight int
)

// activeRootPopup — единственное открытое корневое контекстное меню.
// Гарантирует, что одновременно показано не более одного контекстного меню:
// при открытии нового предыдущее закрывается (подменю не считаются — у них parent != nil).
var (
	activePopupMu   sync.Mutex
	activeRootPopup *PopupMenu
)

var (
	workAreaMu    sync.Mutex
	popupWorkArea image.Rectangle
)

// SetPopupWorkArea задаёт область холста, за которую не выходят меню и их
// подменю: экран без панели задач. Нужна, когда панель нарисована на том же
// холсте, — иначе подменю, прижатое к нижнему краю, ложится на неё.
// Пустой прямоугольник снимает ограничение (весь холст). Меню может
// переопределить область полем PopupMenu.WorkArea.
func SetPopupWorkArea(r image.Rectangle) {
	workAreaMu.Lock()
	popupWorkArea = r
	workAreaMu.Unlock()
}

// PopupWorkArea возвращает область, заданную SetPopupWorkArea.
func PopupWorkArea() image.Rectangle {
	workAreaMu.Lock()
	defer workAreaMu.Unlock()
	return popupWorkArea
}

// SetScreenBounds сообщает виджетам размер канваса (вызывается движком при
// создании/смене разрешения). Используется popup-меню и другими overlay'ами,
// чтобы не выходить за границы экрана.
func SetScreenBounds(w, h int) {
	screenMu.Lock()
	screenWidth, screenHeight = w, h
	screenMu.Unlock()
}

// popupsHosted — глобальный флаг: оверлеи выносятся в собственные нативные
// окна-попапы ОС (движок зарегистрировал PopupSink). Когда он взведён, клэмп
// позиции popup по границам канваса (getScreenBounds) пропускается — экранным
// позиционированием занимается хост, а popup вправе выходить за пределы холста.
// В headless (без хоста) флаг сброшен и поведение прежнее до пикселя.
var popupsHosted atomic.Bool

// SetPopupsHosted включает/выключает режим вынесенных popup-оверлеев.
// Вызывается движком при регистрации/снятии PopupSink.
func SetPopupsHosted(v bool) { popupsHosted.Store(v) }

// PopupsHosted сообщает, активен ли режим вынесенных popup-оверлеев.
func PopupsHosted() bool { return popupsHosted.Load() }

// getScreenBounds возвращает размер канваса (0,0 если ещё не задан).
func getScreenBounds() (int, int) {
	screenMu.Lock()
	defer screenMu.Unlock()
	return screenWidth, screenHeight
}

// NewPopupMenu создаёт пустое popup-меню.
func NewPopupMenu() *PopupMenu {
	// Цвета — из активной темы: контекстные меню создаются на лету
	// (TextInput и др.) и должны следовать текущей теме, а не Win10 Dark.
	m := &PopupMenu{
		hoverIdx:       -1,
		childForIdx:    -1,
		switchIdx:      -1,
		Background:     win10.MenuBG,
		BorderColor:    win10.DropBorder,
		TextColor:      win10.DropText,
		DisabledColor:  win10.Disabled,
		HoverBG:        win10.MenuHoverBG,
		HoverTextColor: win10.MenuHoverText,
		SeparatorColor: win10.DropBorder,
		ShadowColor:    win10.ShadowColor,
		ItemHeight:     30,
		SeparatorH:     9,
		PaddingX:       16,
		MinWidth:       160,
		ArrowPadding:   20,
	}
	if win10.Style.Classic3D {
		m.ItemHeight = 22 // классика: компактные пункты меню
		m.SeparatorH = 7
	}
	// Вид, объявленный профилем текущей темы (Materialize кладёт его в
	// ThemeStyle.Menu): меню, созданные на лету, следуют ему без помощи
	// потребителя. Профиль без объявлений оставляет меню прежним.
	if ms := currentStyle().Menu; !ms.isZero() {
		m.ApplyMenuStyle(ms)
	}
	return m
}

// AddItem добавляет пункт меню.
func (m *PopupMenu) AddItem(text string, onClick func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append(m.items, MenuItem{Text: text, OnClick: onClick})
}

// AddSeparator добавляет горизонтальный разделитель.
func (m *PopupMenu) AddSeparator() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append(m.items, MenuItem{Separator: true})
}

// SetItems заменяет все пункты меню.
func (m *PopupMenu) SetItems(items []MenuItem) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = items
}

// SetItemText меняет надпись пункта по индексу. Нужен, когда меню уже собрано,
// а текст должен обновиться на лету — например при смене языка интерфейса.
func (m *PopupMenu) SetItemText(idx int, text string) {
	m.mu.Lock()
	changed := false
	if idx >= 0 && idx < len(m.items) && m.items[idx].Text != text {
		m.items[idx].Text = text
		changed = true
	}
	m.mu.Unlock()
	if changed {
		notifyUIChanged()
	}
}

// Items возвращает копию пунктов.
func (m *PopupMenu) Items() []MenuItem {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]MenuItem, len(m.items))
	copy(result, m.items)
	return result
}

// IsOpen возвращает true если меню показано.
func (m *PopupMenu) IsOpen() bool {
	return atomic.LoadInt32(&m.open) == 1
}

// Show открывает popup-меню в указанных абсолютных координатах.
// Позиция корректируется так, чтобы меню целиком помещалось в канвасе
// (сдвиг влево/вверх у правого/нижнего края — меню не обрезается границей окна).
func (m *PopupMenu) Show(x, y int) {
	// Закрываем предыдущее корневое контекстное меню (одно меню за раз).
	if m.parent == nil {
		activePopupMu.Lock()
		prev := activeRootPopup
		activeRootPopup = m
		activePopupMu.Unlock()
		if prev != nil && prev != m {
			prev.Close()
		}
	}

	m.mu.RLock()
	w, h := m.calcSize()
	m.mu.RUnlock()

	// Содержимое выше доступной области — меню получает высоту области и
	// прокрутку. Сама высота содержимого — без полей сверху и снизу.
	contentH := h - 2*m.padY()
	area, hasArea := m.availableArea()
	limit := 0
	if hasArea {
		limit = area.Dy()
	}
	if m.MaxHeight > 0 && (limit == 0 || m.MaxHeight < limit) {
		limit = m.MaxHeight
	}
	scrollable := false
	if limit > 0 && h > limit {
		h = limit
		scrollable = true
	}

	// Клэмп в границы канваса — только БЕЗ хоста попапов. При активном хостинге
	// экранное позиционирование делает popupHost, а меню вправе выходить за холст.
	if hasArea {
		// Холст — в экранных координатах, а (x, y) пришли в кадре виджета: у
		// меню поля, лежащего в прокрутке, это координаты содержимого. Холст в
		// этом кадре — прямоугольник [fo, fo+(sw,sh)) (availableArea уже
		// сдвинут). Без поправки меню у виджета, прокрученного далеко вниз,
		// прижималось бы к нижнему краю СОДЕРЖИМОГО и улетало за экран, а не
		// открывалось у курсора.
		if x+w > area.Max.X {
			x = area.Max.X - w
		}
		if x < area.Min.X {
			x = area.Min.X
		}
		if y+h > area.Max.Y {
			y = area.Max.Y - h
		}
		if y < area.Min.Y {
			y = area.Min.Y
		}
	}

	m.geoMu.Lock()
	m.popupX = x
	m.popupY = y
	m.popupW = w
	m.popupH = h
	m.scroll, m.contentH, m.scrollable, m.scrollDir = 0, contentH, scrollable, 0
	m.geoMu.Unlock()
	atomic.StoreInt32(&m.hoverIdx, -1)
	atomic.StoreInt32(&m.open, 1)
	notifyUIChanged() // появление overlay-меню (вне bounds виджета)
}

// ShowBelow открывает popup-меню прямо под указанным виджетом.
func (m *PopupMenu) ShowBelow(w Widget) {
	b := w.Bounds()
	m.Show(b.Min.X, b.Max.Y)
}

// ShowRight открывает popup-меню справа от указанного виджета.
func (m *PopupMenu) ShowRight(w Widget) {
	b := w.Bounds()
	m.Show(b.Max.X, b.Min.Y)
}

// Close закрывает меню и все дочерние подменю.
func (m *PopupMenu) Close() {
	m.cancelTimers()
	m.closeChild()
	wasOpen := atomic.SwapInt32(&m.open, 0) == 1
	atomic.StoreInt32(&m.hoverIdx, -1)
	if m.parent == nil {
		activePopupMu.Lock()
		if activeRootPopup == m {
			activeRootPopup = nil
		}
		activePopupMu.Unlock()
	}
	if wasOpen {
		notifyUIChanged() // исчезновение overlay-меню
	}
}

// setHoverIdx обновляет hover-пункт меню. Пункты рисуются в overlay вне
// bounds виджета — при фактическом изменении инвалидируется весь кадр.
func (m *PopupMenu) setHoverIdx(idx int) {
	if atomic.SwapInt32(&m.hoverIdx, int32(idx)) != int32(idx) {
		notifyUIChanged()
	}
}

// closeChild закрывает дочернее подменю, если открыто.
func (m *PopupMenu) closeChild() {
	m.geoMu.Lock()
	c := m.child
	m.child = nil
	m.childForIdx = -1
	m.geoMu.Unlock()
	if c != nil && c.IsOpen() {
		c.Close()
	}
}

// geo возвращает положение popup под geoMu.
func (m *PopupMenu) geo() (x, y, w, h int) {
	m.geoMu.Lock()
	x, y, w, h = m.popupX, m.popupY, m.popupW, m.popupH
	m.geoMu.Unlock()
	return
}

// ─── Поля и вид по профилю ──────────────────────────────────────────────────

func (m *PopupMenu) padL() int {
	if m.PadLeft > 0 {
		return m.PadLeft
	}
	return m.PaddingX
}

func (m *PopupMenu) padR() int {
	if m.PadRight > 0 {
		return m.PadRight
	}
	return m.PaddingX
}

// padY — отступ пунктов от верхней и нижней рамки.
func (m *PopupMenu) padY() int {
	if m.PaddingY > 0 {
		return m.PaddingY
	}
	return 2
}

// itemInset — отступ плашки наведения от краёв меню.
func (m *PopupMenu) itemInset() int {
	if m.ItemInset > 0 {
		return m.ItemInset
	}
	return 2
}

func (m *PopupMenu) sepInset() int {
	if m.SeparatorInset > 0 {
		return m.SeparatorInset
	}
	return 8
}

func (m *PopupMenu) iconGap() int {
	if m.IconGap > 0 {
		return m.IconGap
	}
	return menuIconGap
}

func (m *PopupMenu) chevronSize() int {
	if m.ChevronSize > 0 {
		return m.ChevronSize
	}
	return 8
}

// scrollBand — высота полосы со стрелкой на каждом конце прокручиваемого меню.
const scrollBand = 14

// availableArea — область, в которой меню обязано поместиться, в координатах
// холста кадра обрабатываемого события. Второе значение false: ограничивать
// нечем (холст неизвестен) или позиционирование ведёт хост попапов.
func (m *PopupMenu) availableArea() (image.Rectangle, bool) {
	if popupsHosted.Load() {
		return image.Rectangle{}, false
	}
	r := m.WorkArea
	if r.Empty() {
		r = PopupWorkArea()
	}
	if r.Empty() {
		sw, sh := getScreenBounds()
		if sw <= 0 || sh <= 0 {
			return image.Rectangle{}, false
		}
		r = image.Rect(0, 0, sw, sh)
	}
	return r.Add(currentEventFrame()), true
}

// menuLayout — геометрия меню, снятая за один раз под geoMu.
type menuLayout struct {
	x, y, w, h int
	// itemsTop — Y первого пункта с учётом прокрутки; viewTop/viewBottom —
	// видимая часть для пунктов (для обычного меню — внутри полей).
	itemsTop, viewTop, viewBottom int
	scroll, maxScroll             int
	scrollable                    bool
}

func (m *PopupMenu) layout() menuLayout {
	m.geoMu.Lock()
	l := menuLayout{x: m.popupX, y: m.popupY, w: m.popupW, h: m.popupH,
		scroll: m.scroll, scrollable: m.scrollable}
	content := m.contentH
	m.geoMu.Unlock()
	pad := m.padY()
	l.viewTop, l.viewBottom = l.y+pad, l.y+l.h-pad
	if l.scrollable {
		l.viewTop += scrollBand
		l.viewBottom -= scrollBand
		if l.maxScroll = content - (l.viewBottom - l.viewTop); l.maxScroll < 0 {
			l.maxScroll = 0
		}
		if l.scroll > l.maxScroll {
			l.scroll = l.maxScroll
		}
	}
	l.itemsTop = l.viewTop - l.scroll
	return l
}

// scrollBy сдвигает прокручиваемое меню на dy точек (вниз — положительное).
func (m *PopupMenu) scrollBy(dy int) bool {
	l := m.layout()
	if !l.scrollable {
		return false
	}
	n := l.scroll + dy
	if n < 0 {
		n = 0
	}
	if n > l.maxScroll {
		n = l.maxScroll
	}
	if n == l.scroll {
		return false
	}
	m.geoMu.Lock()
	m.scroll = n
	m.geoMu.Unlock()
	notifyUIChanged()
	return true
}

// ensureVisible прокручивает меню так, чтобы пункт idx был виден целиком.
func (m *PopupMenu) ensureVisible(idx int) {
	l := m.layout()
	if !l.scrollable || idx < 0 {
		return
	}
	m.mu.RLock()
	top := 0
	h := 0
	for i, it := range m.items {
		ih := m.ItemHeight
		if it.Separator {
			ih = m.SeparatorH
		}
		if i == idx {
			h = ih
			break
		}
		top += ih
	}
	m.mu.RUnlock()
	view := l.viewBottom - l.viewTop
	switch {
	case top < l.scroll:
		m.scrollBy(top - l.scroll)
	case top+h > l.scroll+view:
		m.scrollBy(top + h - l.scroll - view)
	}
}

// bandAt сообщает, на какой стрелке прокрутки стоит курсор: -1 верхней,
// +1 нижней, 0 — ни на какой (или меню не прокручивается).
func (m *PopupMenu) bandAt(y int) int {
	l := m.layout()
	if !l.scrollable {
		return 0
	}
	if y >= l.y && y < l.viewTop {
		return -1
	}
	if y >= l.viewBottom && y < l.y+l.h {
		return 1
	}
	return 0
}

// setScrollDir включает повторную прокрутку, пока курсор стоит на стрелке.
func (m *PopupMenu) setScrollDir(dir int) {
	m.geoMu.Lock()
	same := m.scrollDir == dir
	m.scrollDir = dir
	running := m.scrollAnim != nil && m.scrollAnim.Running()
	m.geoMu.Unlock()
	if dir == 0 || (same && running) {
		return
	}
	m.stepScroll()
}

// stepScroll прокручивает на шаг и заводит следующий, пока курсор на стрелке.
func (m *PopupMenu) stepScroll() {
	m.geoMu.Lock()
	dir := m.scrollDir
	m.geoMu.Unlock()
	if dir == 0 || !m.IsOpen() {
		return
	}
	m.scrollBy(dir * (m.ItemHeight/2 + 1))
	a := Animate(50*time.Millisecond, nil, nil)
	a.OnDone = func() { m.stepScroll() }
	m.geoMu.Lock()
	m.scrollAnim = a
	m.geoMu.Unlock()
}

// cancelTimers снимает отложенное переключение подменю и повтор прокрутки.
func (m *PopupMenu) cancelTimers() {
	m.geoMu.Lock()
	sa, ra := m.switchAnim, m.scrollAnim
	m.switchAnim, m.scrollAnim = nil, nil
	m.switchIdx, m.scrollDir = -1, 0
	m.geoMu.Unlock()
	if sa != nil {
		sa.Stop()
	}
	if ra != nil {
		ra.Stop()
	}
}

// openChildOf возвращает текущее открытое дочернее подменю (nil, если его
// нет или оно закрыто) и индекс пункта, для которого оно открыто.
func (m *PopupMenu) openChildOf() (*PopupMenu, int) {
	m.geoMu.Lock()
	c, idx := m.child, m.childForIdx
	m.geoMu.Unlock()
	if c == nil || !c.IsOpen() {
		return nil, idx
	}
	return c, idx
}

// openChild открывает каскадное подменю для пункта idx.
func (m *PopupMenu) openChild(idx int) {
	m.mu.RLock()
	if idx < 0 || idx >= len(m.items) || len(m.items[idx].SubItems) == 0 {
		m.mu.RUnlock()
		return
	}
	subItems := m.items[idx].SubItems
	m.mu.RUnlock()

	// Если уже открыто для этого пункта — ничего не делаем.
	if c, cIdx := m.openChildOf(); c != nil && cIdx == idx {
		return
	}

	// Закрываем предыдущее дочернее.
	m.closeChild()

	child := NewPopupMenu()
	child.parent = m
	child.Background = m.Background
	child.BorderColor = m.BorderColor
	child.TextColor = m.TextColor
	child.HoverTextColor = m.HoverTextColor
	child.DisabledColor = m.DisabledColor
	child.HoverBG = m.HoverBG
	child.SeparatorColor = m.SeparatorColor
	child.ShadowColor = m.ShadowColor
	child.ItemHeight = m.ItemHeight
	child.SeparatorH = m.SeparatorH
	child.PaddingX = m.PaddingX
	child.MinWidth = m.MinWidth
	// Минимальная ширина нужна корневому меню; подменю профиль может
	// освободить от неё (0 — прежнее наследование).
	switch {
	case m.SubMenuMinWidth > 0:
		child.MinWidth = m.SubMenuMinWidth
	case m.SubMenuMinWidth < 0:
		child.MinWidth = 0
	}
	child.SubMenuMinWidth = m.SubMenuMinWidth
	child.ArrowPadding = m.ArrowPadding
	child.UseMnemonics = m.UseMnemonics
	child.PadLeft, child.PadRight, child.PaddingY = m.PadLeft, m.PadRight, m.PaddingY
	child.ItemInset, child.SeparatorInset = m.ItemInset, m.SeparatorInset
	child.CornerRadius, child.ItemCorner, child.Elevation = m.CornerRadius, m.ItemCorner, m.Elevation
	child.ShortcutColor = m.ShortcutColor
	child.IconSize, child.IconGap = m.IconSize, m.IconGap
	child.ChevronRight, child.ChevronSize = m.ChevronRight, m.ChevronSize
	child.TintIcons = m.TintIcons
	child.SubMenuDelay = m.SubMenuDelay
	child.WorkArea, child.MaxHeight = m.WorkArea, m.MaxHeight
	child.SetItems(subItems)
	child.OnSelect = m.OnSelect

	// Позиция: справа от текущего popup, на уровне пункта.
	// Если справа не помещается — открываем слева от родителя (как в WPF/ОС).
	child.mu.RLock()
	cw, _ := child.calcSize()
	child.mu.RUnlock()
	itemY := m.itemYForIndex(idx)
	px, _, pw, _ := m.geo()
	x := px + pw - 2
	// Разворот влево у правого края — только без хоста (хост позиционирует сам).
	// Правый край области — в кадре обрабатываемого события (см. Show).
	if area, ok := m.availableArea(); ok && x+cw > area.Max.X {
		x = px - cw + 2
	}
	child.Show(x, itemY)

	m.geoMu.Lock()
	m.child = child
	m.childForIdx = idx
	m.geoMu.Unlock()
}

// itemYForIndex возвращает абсолютную Y-координату верхнего края пункта.
func (m *PopupMenu) itemYForIndex(idx int) int {
	y := m.layout().itemsTop
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i, item := range m.items {
		if i == idx {
			return y
		}
		if item.Separator {
			y += m.SeparatorH
		} else {
			y += m.ItemHeight
		}
	}
	return y
}

// fullBounds возвращает объединённые bounds этого popup и всех дочерних.
func (m *PopupMenu) fullBounds() image.Rectangle {
	r := m.popupRect()
	if c, _ := m.openChildOf(); c != nil {
		r = r.Union(c.fullBounds())
	}
	return r
}

// Dismiss реализует Dismissable — закрывает меню при DismissAll.
func (m *PopupMenu) Dismiss() {
	// Кто именно погасил меню, важно кнопке-владельцу: движок закрывает
	// overlay'и вне пути клика ДО доставки события, поэтому кнопка, лежащая
	// в другом виджете (шеврон в заголовке окна), иначе не может отличить
	// «меню было закрыто» от «меню закрыл мой же клик». См. CurrentPressSeq.
	//
	// Метку ставим ТОЛЬКО если меню было открыто: движок гасит и уже
	// закрытые меню, а такая отметка заставила бы кнопку проглотить
	// нажатие, которое должно меню открыть.
	if atomic.LoadInt32(&m.open) == 1 {
		atomic.StoreUint64(&m.dismissSeq, CurrentPressSeq())
	}
	m.Close()
}

// dismissedByPress сообщает, было ли меню погашено нажатием seq (0 — никогда).
func (m *PopupMenu) dismissedByPress(seq uint64) bool {
	return seq != 0 && atomic.LoadUint64(&m.dismissSeq) == seq
}

// ─── Размеры ────────────────────────────────────────────────────────────────

// calcSize вычисляет ширину и высоту popup (вызывать под RLock).
func (m *PopupMenu) calcSize() (w, h int) {
	w = m.MinWidth
	hasSubItems := false
	gutter := m.checkGutter() + m.iconGutter()
	for _, item := range m.items {
		if item.Separator {
			h += m.SeparatorH
		} else {
			h += m.ItemHeight
			if len(item.SubItems) > 0 {
				hasSubItems = true
			}
			// Ширина по НАСТОЯЩЕМУ замеру подписи: len в Go считает байты, и
			// кириллический пункт выходил вдвое шире нужного.
			textW := MeasureUIText(mnemonicLabel(item.Text, m.UseMnemonics),
				DefaultFontSize()) + m.padL() + m.padR() + 24 + gutter
			if item.Shortcut != "" {
				// Сочетание пишется справа, и место под него нужно
				// отвести всему меню: иначе подпись и сочетание налезут
				// друг на друга в самом длинном пункте.
				textW += MeasureUIText(item.Shortcut, DefaultFontSize()) + menuShortcutGap
			}
			if textW > w {
				w = textW
			}
		}
	}
	// Добавляем место для стрелки ► если есть подменю.
	if hasSubItems {
		w += m.arrowSpace()
	}
	h += 2 * m.padY() // верхний + нижний padding
	return
}

// arrowSpace — сколько ширины добавляет шеврон подменю. Глиф стоит у правого
// поля и требует ArrowPadding; тонкий шеврон ложится в зону поля и сочетания
// (padR + 24) и добавляет только то, что в неё не поместилось.
func (m *PopupMenu) arrowSpace() int {
	if m.ChevronRight <= 0 {
		return m.ArrowPadding
	}
	need := m.ChevronRight + m.chevronSize()/4 + 6
	if extra := need - (m.padR() + 24); extra > 0 {
		return extra
	}
	return 0
}

// itemAtY возвращает индекс пункта по Y-координатe (абсолютной).
// Возвращает -1 если нет пункта (разделитель, за пределами).
func (m *PopupMenu) itemAtY(y int) int {
	l := m.layout()
	if y < l.viewTop || y >= l.viewBottom {
		return -1 // поля и стрелки прокрутки — не пункты
	}
	curY := l.itemsTop
	for i, item := range m.items {
		var itemH int
		if item.Separator {
			itemH = m.SeparatorH
		} else {
			itemH = m.ItemHeight
		}
		if y >= curY && y < curY+itemH {
			if item.Separator || item.Disabled {
				return -1
			}
			return i
		}
		curY += itemH
	}
	return -1
}

// popupRect возвращает bounds popup-области.
func (m *PopupMenu) popupRect() image.Rectangle {
	x, y, w, h := m.geo()
	return image.Rect(x, y, x+w, y+h)
}

// ─── Bounds (расширенные при открытии) ───────────────────────────────────────

// Bounds возвращает расширенные bounds включая popup и дочерние для hit-test.
func (m *PopupMenu) Bounds() image.Rectangle {
	base := m.Base.Bounds()
	if atomic.LoadInt32(&m.open) == 0 {
		return base
	}
	return base.Union(m.fullBounds())
}

// BaseBounds возвращает оригинальные bounds (без popup).
func (m *PopupMenu) BaseBounds() image.Rectangle {
	return m.Base.Bounds()
}

// ─── Overlay ────────────────────────────────────────────────────────────────

// HasOverlay сообщает движку что меню рисуется как overlay.
func (m *PopupMenu) HasOverlay() bool {
	return atomic.LoadInt32(&m.open) == 1
}

// OverlayBounds возвращает объединённый прямоугольник popup и всех каскадных
// подменю (абсолютные логические координаты) — для выноса в нативное окно.
// Пустой Rect, если меню закрыто. Реализует widget.OverlayBoundsProvider.
func (m *PopupMenu) OverlayBounds() image.Rectangle {
	if atomic.LoadInt32(&m.open) == 0 {
		return image.Rectangle{}
	}
	return m.fullBounds()
}

// DrawOverlay рисует popup-меню поверх всего UI (включая каскадные подменю).
func (m *PopupMenu) DrawOverlay(ctx DrawContext) {
	if atomic.LoadInt32(&m.open) == 0 {
		return
	}

	m.mu.RLock()
	items := m.items
	m.mu.RUnlock()

	l := m.layout()
	px, py, pw, ph := l.x, l.y, l.w, l.h
	openChild, openChildIdx := m.openChildOf()
	hover := int(atomic.LoadInt32(&m.hoverIdx))
	pending := m.pendingSwitch()
	corner := m.CornerRadius

	// Тень.
	m.drawShadow(ctx, px, py, pw, ph)

	// Фон popup.
	if corner > 0 {
		ctx.FillRoundRect(px, py, pw, ph, corner, m.Background)
	} else {
		ctx.FillRect(px, py, pw, ph, m.Background)
	}

	// Рамка: классика — выпуклый 3D-бордюр (как меню Win2000), иначе плоская.
	if st := currentStyle(); st.Classic3D {
		drawBevelRaised(ctx, px, py, pw, ph, st)
	} else if corner > 0 {
		ctx.DrawRoundBorder(px, py, pw, ph, corner, m.BorderColor)
	} else {
		ctx.DrawBorder(px, py, pw, ph, m.BorderColor)
	}

	// Пункты. У прокручиваемого меню — внутри окна просмотра: всё, что
	// выходит за него, обрезается, а в полосах на концах рисуются стрелки.
	var prevClip image.Rectangle
	if l.scrollable {
		prevClip = ctx.Clip()
		view := image.Rect(px, l.viewTop, px+pw, l.viewBottom)
		if !prevClip.Empty() {
			view = view.Intersect(prevClip)
		}
		ctx.SetClip(view)
	}

	padL, padR := m.padL(), m.padR()
	inset := m.itemInset()
	sepInset := m.sepInset()
	curY := l.itemsTop
	for i, item := range items {
		if item.Separator {
			sepY := curY + m.SeparatorH/2
			if m.SeparatorColor.A < 255 {
				ctx.FillRectAlpha(px+sepInset, sepY, pw-2*sepInset, 1, m.SeparatorColor)
			} else {
				ctx.DrawHLine(px+sepInset, sepY, pw-2*sepInset, m.SeparatorColor)
			}
			curY += m.SeparatorH
			continue
		}
		if curY+m.ItemHeight <= l.viewTop || curY >= l.viewBottom {
			curY += m.ItemHeight // за пределами окна просмотра
			continue
		}

		// Hover-подсветка (а также подсветка пункта с открытым дочерним
		// подменю). Пока подменю ждёт переключения на другой пункт, плашку
		// держит только пункт под курсором: две сразу сбивали бы с толку.
		isChildOpen := openChild != nil && openChildIdx == i && (pending < 0 || pending == i)
		hovered := (i == hover || isChildOpen) && !item.Disabled
		if hovered {
			m.drawHoverPlate(ctx, px+inset, curY, pw-2*inset, m.ItemHeight)
		}

		// Текст.
		textY := curY + (m.ItemHeight-13)/2
		textCol := m.TextColor
		if hovered && m.HoverTextColor.A > 0 {
			textCol = m.HoverTextColor // классика: белый на navy
		}
		if item.Disabled {
			textCol = m.DisabledColor
		}
		textX := px + padL + m.checkGutter() + m.iconGutter()
		if item.Checkable && item.Checked {
			drawCheckMark(ctx, image.Rect(px+padL, curY, px+padL+checkMarkSize,
				curY+m.ItemHeight), textCol)
		}
		if icon := m.iconFor(item, hovered, textCol); icon != nil {
			sz := m.iconSizeOf(item)
			ix := px + padL + m.checkGutter()
			ctx.DrawImageScaled(icon, ix, curY+(m.ItemHeight-sz)/2, sz, sz)
		}
		drawMnemonicText(ctx, item.Text, textX, textY, textCol, m.UseMnemonics)

		// Сочетание клавиш — справа, приглушённым цветом: это подсказка, а
		// не вторая подпись, и спорить с названием пункта она не должна.
		if item.Shortcut != "" && len(item.SubItems) == 0 {
			sw := MeasureUIText(item.Shortcut, DefaultFontSize())
			sx := px + pw - padR - sw
			ctx.DrawText(item.Shortcut, sx, textY, m.shortcutColorFor(textCol, hovered, item.Disabled))
		}

		// Стрелка для пунктов с подменю.
		if len(item.SubItems) > 0 {
			m.drawChevron(ctx, px, pw, curY, textY, textCol)
		}

		curY += m.ItemHeight
	}

	if l.scrollable {
		ctx.SetClip(prevClip)
		m.drawScrollArrows(ctx, l)
	}

	// Рекурсивно рисуем дочернее подменю.
	if openChild != nil {
		openChild.DrawOverlay(ctx)
	}
}

// drawShadow рисует тень меню. Меню с Elevation (профиль объявил высоту)
// получает мягкую тень через ShadowDrawer; без неё или без Elevation — прежнюю
// прямоугольную со смещением на 2 точки.
//
// Единственная точка, где тень меню считается: общие токены теней темы
// (размытие, смещение, непрозрачность) подключаются здесь — вместо
// Elevation/ShadowColor.
func (m *PopupMenu) drawShadow(ctx DrawContext, px, py, pw, ph int) {
	// Мягкая тень по общим токенам темы (ShadowBlur, ShadowOffset,
	// ShadowOpacity стиля "menu"), если профиль их объявил, — та же, что у
	// всплывающих панелей и окон. Появление и исчезновение меню и так
	// перерисовывают кадр целиком, поэтому запас под тень в повреждение не
	// добавляется.
	if sp, ok := MenuShadow(); ok {
		DrawShadowSpec(ctx, image.Rect(px, py, px+pw, py+ph), m.CornerRadius, sp)
		return
	}
	if m.Elevation > 0 && m.ShadowColor.A > 0 {
		if sd, ok := ctx.(ShadowDrawer); ok {
			sd.DrawSoftShadow(image.Rect(px, py, px+pw, py+ph), m.CornerRadius, m.Elevation, m.ShadowColor)
			return
		}
	}
	if m.CornerRadius > 0 {
		ctx.FillRoundRect(px+2, py+2, pw, ph, m.CornerRadius, m.ShadowColor)
		return
	}
	ctx.FillRectAlpha(px+2, py+2, pw, ph, m.ShadowColor)
}

// drawHoverPlate рисует плашку наведения. Полупрозрачный цвет (у Windows 11 —
// чёрная плёнка) кладётся поверх меню, а не записывается как есть: запись
// превращала бы его в почти чёрный и закрывала текст.
func (m *PopupMenu) drawHoverPlate(ctx DrawContext, x, y, w, h int) {
	switch {
	case m.ItemCorner > 0:
		ctx.FillRoundRect(x, y, w, h, m.ItemCorner, m.HoverBG)
	case m.HoverBG.A < 255:
		ctx.FillRectAlpha(x, y, w, h, m.HoverBG)
	default:
		ctx.FillRect(x, y, w, h, m.HoverBG)
	}
}

// iconFor выбирает картинку значка для состояния пункта: свою для плашки
// наведения, если есть, и одноцветную — перекрашенную в цвет текста.
func (m *PopupMenu) iconFor(item MenuItem, hovered bool, ink color.RGBA) image.Image {
	icon := item.Icon
	if hovered && item.IconHover != nil {
		icon = item.IconHover
	}
	if icon == nil {
		return nil
	}
	if item.IconTint || m.TintIcons {
		return menuTintImage(icon, ink)
	}
	return icon
}

// menuTintImage перекрашивает картинку в один цвет по её альфа-каналу.
func menuTintImage(src image.Image, col color.RGBA) image.Image {
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			_, _, _, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a == 0 {
				continue
			}
			// Цвет предумножен: каждый канал, включая альфу, масштабируется
			// на долю непрозрачности точки картинки.
			k := func(c uint8) uint8 { return uint8(uint32(c) * a / 0xffff) }
			out.SetRGBA(x, y, color.RGBA{R: k(col.R), G: k(col.G), B: k(col.B), A: k(col.A)})
		}
	}
	return out
}

// shortcutColorFor — цвет сочетания пункта. Заданный профилем (ShortcutColor)
// применяется к обычному пункту; недоступный и пункт под плашкой с иным цветом
// текста следуют своему тексту, приглушённому.
func (m *PopupMenu) shortcutColorFor(text color.RGBA, hovered, disabled bool) color.RGBA {
	if m.ShortcutColor.A != 0 && !disabled &&
		!(hovered && m.HoverTextColor.A > 0 && m.HoverTextColor != m.TextColor) {
		return m.ShortcutColor
	}
	return m.shortcutColor(text)
}

// drawChevron рисует признак подменю у правого края пункта: тонкий «›» по
// профилю (ChevronRight) или прежний глиф «▸» у поля.
func (m *PopupMenu) drawChevron(ctx DrawContext, px, pw, rowY, textY int, col color.RGBA) {
	if m.ChevronRight <= 0 {
		ctx.DrawText("▸", px+pw-m.PaddingX, textY, col)
		return
	}
	drawThinChevron(ctx, px+pw-m.ChevronRight, rowY+m.ItemHeight/2, m.chevronSize(), 1, col)
}

// drawThinChevron рисует «›» высотой size с серединой в (cx, cy); dir — 1
// вправо, -1 влево. Фигура, а не символ шрифта: тонкая линия обязана выглядеть
// одинаково в любом шрифте темы (тот же довод, что у галочки).
func drawThinChevron(ctx DrawContext, cx, cy, size, dir int, col color.RGBA) {
	half := size / 2
	w := size / 4
	if w < 2 {
		w = 2
	}
	x0, x1 := cx-w/2, cx-w/2+w
	if dir < 0 {
		x0, x1 = x1, x0
	}
	if aa, ok := ctx.(AAShapes); ok {
		aa.StrokePolylineAA([]image.Point{
			{X: x0, Y: cy - half}, {X: x1, Y: cy}, {X: x0, Y: cy + half},
		}, 1.1, false, col)
		return
	}
	step := 1
	if dir < 0 {
		step = -1
	}
	for i := 0; i <= w; i++ {
		ctx.SetPixel(x0+step*i, cy-half+i*half/w, col)
		ctx.SetPixel(x0+step*i, cy+half-i*half/w, col)
	}
}

// drawScrollArrows рисует стрелки в полосах на концах прокручиваемого меню:
// яркая — в ту сторону, куда ещё есть что показать.
func (m *PopupMenu) drawScrollArrows(ctx DrawContext, l menuLayout) {
	cx := l.x + l.w/2
	for _, up := range []bool{true, false} {
		col := m.DisabledColor
		if (up && l.scroll > 0) || (!up && l.scroll < l.maxScroll) {
			col = m.TextColor
		}
		cy := l.y + m.padY() + scrollBand/2
		if !up {
			cy = l.y + l.h - m.padY() - scrollBand/2
		}
		// Треугольник из горизонтальных линий: вершина к краю меню.
		for j := 0; j < 4; j++ {
			y := cy - 2 + j
			if !up {
				y = cy + 1 - j
			}
			ctx.DrawHLine(cx-j, y, 1+2*j, col)
		}
	}
}

// Draw — основной виджет невидим (всё рисуется через DrawOverlay).
func (m *PopupMenu) Draw(ctx DrawContext) {
	b := m.bounds
	if b.Empty() {
		return
	}
	// PopupMenu не имеет основного рендеринга — только overlay.
	m.drawDisabledOverlay(ctx)
}

// ─── События ────────────────────────────────────────────────────────────────

// OnMouseMove обрабатывает hover по пунктам и каскадные подменю.
func (m *PopupMenu) OnMouseMove(x, y int) {
	if !m.IsEnabled() {
		return
	}
	if atomic.LoadInt32(&m.open) == 0 {
		return
	}

	// Сначала проверяем дочернее подменю.
	if c, _ := m.openChildOf(); c != nil {
		childRect := c.fullBounds()
		if image.Pt(x, y).In(childRect) {
			// Курсор дошёл до подменю: ждать переключения больше не нужно.
			m.cancelSwitch()
			c.OnMouseMove(x, y)
			return
		}
	}

	pr := m.popupRect()
	if !image.Pt(x, y).In(pr) {
		m.setHoverIdx(-1)
		m.setScrollDir(0)
		return
	}

	// Стрелка на конце прокручиваемого меню: курсор на ней листает список.
	if band := m.bandAt(y); band != 0 {
		m.setHoverIdx(-1)
		m.setScrollDir(band)
		return
	}
	m.setScrollDir(0)

	m.mu.RLock()
	idx := m.itemAtY(y)
	m.mu.RUnlock()
	m.setHoverIdx(idx)

	// Если навели на пункт с SubItems — открываем дочернее подменю.
	if idx >= 0 {
		m.mu.RLock()
		hasSubItems := idx < len(m.items) && len(m.items[idx].SubItems) > 0
		m.mu.RUnlock()
		if m.SubMenuDelay > 0 {
			m.switchChildLater(idx, hasSubItems)
		} else if hasSubItems {
			m.openChild(idx)
		} else {
			// Навели на пункт без подменю — закрываем дочернее.
			m.closeChild()
		}
	}
}

// switchChildLater переключает подменю после задержки SubMenuDelay: пока
// курсор идёт по диагонали от пункта к раскрытому подменю, он задевает
// соседние пункты, и мгновенное переключение закрывало бы подменю под ним.
// Плашка наведения при этом следует за курсором сразу — ждёт только подменю.
func (m *PopupMenu) switchChildLater(idx int, hasSub bool) {
	c, cIdx := m.openChildOf()
	if (hasSub && c != nil && cIdx == idx) || (!hasSub && c == nil) {
		m.cancelSwitch() // уже так, как надо
		return
	}
	m.geoMu.Lock()
	if m.switchIdx == idx && m.switchAnim != nil && m.switchAnim.Running() {
		m.geoMu.Unlock()
		return // отсчёт для этого пункта уже идёт
	}
	old := m.switchAnim
	m.switchIdx = idx
	m.geoMu.Unlock()
	if old != nil {
		old.Stop()
	}
	// Анимация без покадрового колбэка — ровно таймер движка.
	a := Animate(time.Duration(m.SubMenuDelay)*time.Millisecond, nil, nil)
	a.OnDone = func() {
		m.geoMu.Lock()
		still := m.switchIdx == idx
		m.switchAnim, m.switchIdx = nil, -1
		m.geoMu.Unlock()
		if !still || !m.IsOpen() || int(atomic.LoadInt32(&m.hoverIdx)) != idx {
			return
		}
		if hasSub {
			m.openChild(idx)
		} else {
			m.closeChild()
		}
	}
	m.geoMu.Lock()
	m.switchAnim = a
	m.geoMu.Unlock()
}

// cancelSwitch отменяет отложенное переключение подменю.
func (m *PopupMenu) cancelSwitch() {
	m.geoMu.Lock()
	a := m.switchAnim
	m.switchAnim, m.switchIdx = nil, -1
	m.geoMu.Unlock()
	if a != nil {
		a.Stop()
	}
}

// pendingSwitch — пункт, над которым подменю ждёт переключения (-1 — нет).
func (m *PopupMenu) pendingSwitch() int {
	m.geoMu.Lock()
	defer m.geoMu.Unlock()
	if m.switchAnim == nil {
		return -1
	}
	return m.switchIdx
}

// OnMouseButton обрабатывает клик: выбор пункта или закрытие.
func (m *PopupMenu) OnMouseButton(e MouseEvent) bool {
	if !m.IsEnabled() {
		return false
	}
	if atomic.LoadInt32(&m.open) == 0 {
		return false
	}

	// Сначала проверяем дочернее подменю.
	if c, _ := m.openChildOf(); c != nil {
		childRect := c.fullBounds()
		if image.Pt(e.X, e.Y).In(childRect) {
			return c.OnMouseButton(e)
		}
	}

	// Колесо листает длинное меню; короткое событие поглощает, как раньше.
	if e.Button == MouseWheelUp || e.Button == MouseWheelDown {
		if e.Pressed && image.Pt(e.X, e.Y).In(m.popupRect()) {
			step := 3 * m.ItemHeight
			if e.Button == MouseWheelUp {
				step = -step
			}
			m.scrollBy(step)
		}
		return image.Pt(e.X, e.Y).In(m.popupRect())
	}

	if e.Button != MouseLeft || e.Pressed {
		// Закрытие по правому клику.
		if e.Button == MouseRight && !e.Pressed {
			m.Close()
			return true
		}
		// Поглощаем mouseDown внутри popup, чтобы dismissOutside
		// не закрыл меню до mouseUp.
		pr := m.popupRect()
		if image.Pt(e.X, e.Y).In(pr) {
			return true
		}
		return false
	}

	// Отпускание ЛКМ.
	pr := m.popupRect()
	if !image.Pt(e.X, e.Y).In(pr) {
		// Клик за пределами — закрыть всё.
		m.Close()
		return true
	}

	if band := m.bandAt(e.Y); band != 0 {
		m.scrollBy(band * m.ItemHeight * 2) // щелчок по стрелке — на два пункта
		return true
	}

	m.mu.RLock()
	idx := m.itemAtY(e.Y)
	m.mu.RUnlock()

	if idx >= 0 {
		m.mu.RLock()
		item := m.items[idx]
		m.mu.RUnlock()

		// Если у пункта есть подменю — не закрываем, а открываем каскад.
		if len(item.SubItems) > 0 {
			m.openChild(idx)
			return true
		}

		// Закрываем всю цепочку меню (вверх до корня).
		m.closeAll()

		if item.OnClick != nil {
			item.OnClick() // синхронно — меню уже закрыто, локи отпущены
		}
		if m.OnSelect != nil {
			m.OnSelect(idx, item.Text)
		}
	}

	return true
}

// closeAll закрывает текущее меню и всех родителей (всю цепочку).
func (m *PopupMenu) closeAll() {
	// Находим корневое меню.
	root := m
	for root.parent != nil {
		root = root.parent
	}
	root.Close()
}

// OnKeyEvent обрабатывает навигацию: стрелки, Enter, Escape, Right (подменю), Left (назад).
func (m *PopupMenu) OnKeyEvent(e KeyEvent) {
	if !e.Pressed || atomic.LoadInt32(&m.open) == 0 {
		return
	}

	// Если есть открытое дочернее подменю — делегируем ему.
	if c, _ := m.openChildOf(); c != nil {
		c.OnKeyEvent(e)
		return
	}

	m.mu.RLock()
	count := len(m.items)
	m.mu.RUnlock()

	if count == 0 {
		return
	}

	hover := int(atomic.LoadInt32(&m.hoverIdx))

	switch e.Code {
	case KeyEscape:
		if m.parent != nil {
			// Закрываем только текущий уровень (возврат к родителю).
			m.Close()
		} else {
			m.Close()
		}

	case KeyUp:
		hover = m.prevActiveItem(hover)
		m.setHoverIdx(hover)
		m.ensureVisible(hover)

	case KeyDown:
		hover = m.nextActiveItem(hover)
		m.setHoverIdx(hover)
		m.ensureVisible(hover)

	case KeyRight:
		// Войти в подменю, если у текущего пункта есть SubItems.
		if hover >= 0 {
			m.mu.RLock()
			hasSubItems := hover < len(m.items) && len(m.items[hover].SubItems) > 0
			m.mu.RUnlock()
			if hasSubItems {
				m.openChild(hover)
				// Устанавливаем hover на первый пункт дочернего меню.
				if c, _ := m.openChildOf(); c != nil {
					first := c.nextActiveItem(-1)
					c.setHoverIdx(first)
				}
			}
		}

	case KeyLeft:
		// Если есть родитель — закрываем текущий уровень.
		if m.parent != nil {
			m.Close()
		}

	case KeyEnter:
		if hover >= 0 {
			m.mu.RLock()
			item := m.items[hover]
			m.mu.RUnlock()
			if !item.Disabled && !item.Separator {
				// Если есть подменю — открываем каскад.
				if len(item.SubItems) > 0 {
					m.openChild(hover)
					if c, _ := m.openChildOf(); c != nil {
						first := c.nextActiveItem(-1)
						c.setHoverIdx(first)
					}
					return
				}
				m.closeAll()
				if item.OnClick != nil {
					item.OnClick() // синхронно — меню уже закрыто, локи отпущены
				}
				if m.OnSelect != nil {
					m.OnSelect(hover, item.Text)
				}
			}
		}

	default:
		m.handleMnemonic(e, hover)
	}
}

// handleMnemonic выбирает пункт по подчёркнутой букве.
//
// Если буква у пунктов одна на двоих, нажатие не выбирает ни одного, а
// переставляет подсветку на следующий такой пункт: так ведёт себя меню
// Windows, и это единственное разумное — выбрать за человека наугад нельзя.
func (m *PopupMenu) handleMnemonic(e KeyEvent, hover int) {
	if !m.UseMnemonics || e.Mod&(ModCtrl) != 0 {
		return
	}
	m.mu.RLock()
	var hits []int
	for i, it := range m.items {
		if it.Separator || it.Disabled {
			continue
		}
		if _, key, _ := splitMnemonic(it.Text); matchesMnemonic(key, e) {
			hits = append(hits, i)
		}
	}
	m.mu.RUnlock()
	if len(hits) == 0 {
		return
	}
	if len(hits) > 1 {
		next := hits[0]
		for _, i := range hits {
			if i > hover {
				next = i
				break
			}
		}
		m.setHoverIdx(next)
		return
	}
	m.activateItem(hits[0])
}

// activateItem выполняет пункт так же, как Enter: подменю открывает, обычный
// пункт выполняет, закрыв перед этим всё меню.
func (m *PopupMenu) activateItem(idx int) {
	m.mu.RLock()
	if idx < 0 || idx >= len(m.items) {
		m.mu.RUnlock()
		return
	}
	item := m.items[idx]
	m.mu.RUnlock()

	if len(item.SubItems) > 0 {
		m.setHoverIdx(idx)
		m.openChild(idx)
		if c, _ := m.openChildOf(); c != nil {
			c.setHoverIdx(c.nextActiveItem(-1))
		}
		return
	}
	m.closeAll()
	if item.OnClick != nil {
		item.OnClick()
	}
	if m.OnSelect != nil {
		m.OnSelect(idx, item.Text)
	}
}

// nextActiveItem ищет следующий активный (не disabled, не separator) пункт.
func (m *PopupMenu) nextActiveItem(from int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := len(m.items)
	for i := 1; i <= n; i++ {
		idx := (from + i) % n
		if !m.items[idx].Separator && !m.items[idx].Disabled {
			return idx
		}
	}
	return -1
}

// prevActiveItem ищет предыдущий активный пункт.
func (m *PopupMenu) prevActiveItem(from int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := len(m.items)
	if from < 0 {
		from = 0
	}
	for i := 1; i <= n; i++ {
		idx := (from - i + n) % n
		if !m.items[idx].Separator && !m.items[idx].Disabled {
			return idx
		}
	}
	return -1
}

// ─── Focusable ──────────────────────────────────────────────────────────────

func (m *PopupMenu) SetFocused(v bool) {}
func (m *PopupMenu) IsFocused() bool   { return m.IsOpen() }

// ─── Themeable ──────────────────────────────────────────────────────────────

// ApplyTheme обновляет цвета из темы.
func (m *PopupMenu) ApplyTheme(t *Theme) {
	m.Background = t.MenuBG
	m.BorderColor = t.DropBorder
	m.TextColor = t.DropText
	m.DisabledColor = t.Disabled
	m.HoverBG = t.MenuHoverBG
	m.HoverTextColor = t.MenuHoverText
	m.SeparatorColor = t.DropBorder
	m.ShadowColor = t.ShadowColor
	// Размеры и вид, объявленные профилем (ThemeStyle.Menu); без объявлений —
	// прежние 22/7 для классики и 30/9 для остальных.
	m.applyMenuStyle(t.Style.Menu, t.Style.Classic3D)
}

// ─── Отметки у пунктов (WPF MenuItem.IsChecked) ─────────────────────────────

// checkMarkSize — сторона поля под отметку. Отметка рисуется фигурой, а не
// символом шрифта: галочка обязана выглядеть одинаково при любом шрифте темы,
// включая тот, в котором нужного знака нет вовсе (тот же довод, что у уголка
// трея в desktop/systemtray.go).
const checkMarkSize = 14

// Геометрия значка в пункте меню.
const (
	menuIconGap   = 8  // зазор между значком и подписью
	menuIconInset = 14 // на сколько значок мельче высоты пункта
)

// menuShortcutGap — зазор между подписью пункта и его сочетанием клавиш.
const menuShortcutGap = 24

// shortcutColor — цвет подписи сочетания: приглушённый оттенок цвета пункта.
//
// Приглушаем смешением с фоном меню, а не альфой: полупрозрачный текст
// поверх подсвеченного пункта выглядит грязно (та же причина, что у приписки
// узла дерева).
func (m *PopupMenu) shortcutColor(text color.RGBA) color.RGBA {
	bg := m.Background
	if bg.A == 0 {
		return text
	}
	const k = 0.45
	mix := func(a, b uint8) uint8 {
		return uint8(float64(a)*(1-k) + float64(b)*k + 0.5)
	}
	return color.RGBA{R: mix(text.R, bg.R), G: mix(text.G, bg.G), B: mix(text.B, bg.B), A: 255}
}

// checkGutter — ширина поля под отметку слева от подписей.
//
// Отводится всему меню разом, если хоть один пункт объявлен Checkable: иначе
// подписи разъезжались бы по левому краю в тот момент, когда пользователь
// ставит отметку, — а меню не должно дёргаться от щелчка по нему.
// iconGutter — ширина зоны под значки: отводится всему меню, если значок есть
// хоть у одного пункта.
//
// Одна функция и для измерения ширины, и для отрисовки: посчитай зону в двух
// местах по-разному — и подписи разъедутся ровно на разницу.
func (m *PopupMenu) iconGutter() int {
	w := 0
	for _, item := range m.items {
		if item.Icon == nil {
			continue
		}
		if sz := m.iconSizeOf(item); sz > w {
			w = sz
		}
	}
	if w == 0 {
		return 0
	}
	return w + m.iconGap()
}

// iconSizeOf — сторона значка пункта: своя, если задана, иначе по высоте
// пункта, но не крупнее её самой за вычетом полей.
func (m *PopupMenu) iconSizeOf(item MenuItem) int {
	sz := item.IconSize
	if sz <= 0 {
		sz = m.IconSize
	}
	if sz <= 0 {
		sz = m.ItemHeight - menuIconInset
	}
	if max := m.ItemHeight - 2; sz > max {
		sz = max
	}
	if sz < 1 {
		sz = 1
	}
	return sz
}

func (m *PopupMenu) checkGutter() int {
	for _, item := range m.items {
		if item.Checkable {
			return checkMarkSize + 4
		}
	}
	return 0
}

// SetItemChecked ставит или снимает отметку у пункта idx.
//
// Пункт с непустым RadioGroup ведёт себя как переключатель: отметка снимается
// у соседей той же группы В ЭТОМ ЖЕ меню. Соседи глубже или выше по дереву к
// группе не относятся — иначе одно имя группы в разных подменю связывало бы
// несвязанные наборы.
func (m *PopupMenu) SetItemChecked(idx int, checked bool) {
	m.mu.Lock()
	changed := setCheckedIn(m.items, idx, checked)
	m.mu.Unlock()
	if changed {
		m.Invalidate()
		notifyUIChanged()
	}
}

// setCheckedIn — общая механика отметки для PopupMenu и MenuBar.
func setCheckedIn(items []MenuItem, idx int, checked bool) bool {
	if idx < 0 || idx >= len(items) {
		return false
	}
	if items[idx].Checked == checked {
		return false
	}
	items[idx].Checked = checked
	// Пункт, которому ставят отметку, обязан её показывать: иначе вызов
	// молча ничего не изменил бы на экране.
	if checked {
		items[idx].Checkable = true
	}

	if group := items[idx].RadioGroup; checked && group != "" {
		for i := range items {
			if i != idx && items[i].RadioGroup == group {
				items[i].Checked = false
			}
		}
	}
	return true
}

// drawCheckMark рисует галочку в отведённом поле.
//
// Две линии под углом, набранные точками: короткая вниз-вправо и длинная
// вверх-вправо. Толщина в две точки — иначе на светлой подложке галочка
// теряется, ровно как терялся уголок трея.
func drawCheckMark(ctx DrawContext, r image.Rectangle, col color.RGBA) {
	if col.A == 0 || r.Empty() {
		return
	}
	side := r.Dx()
	if r.Dy() < side {
		side = r.Dy()
	}
	// Галочка занимает не всё поле: по краям остаётся воздух, иначе она
	// сливается с подсветкой пункта.
	inset := side / 4
	if inset < 1 {
		inset = 1
	}
	x0 := r.Min.X + inset
	y0 := r.Min.Y + r.Dy()/2
	short := (side - 2*inset) / 3
	long := side - 2*inset - short
	if short < 1 || long < 1 {
		return
	}

	for i := 0; i <= short; i++ {
		ctx.SetPixel(x0+i, y0+i, col)
		ctx.SetPixel(x0+i, y0+i+1, col)
	}
	for i := 0; i <= long; i++ {
		ctx.SetPixel(x0+short+i, y0+short-i, col)
		ctx.SetPixel(x0+short+i, y0+short-i+1, col)
	}
}
