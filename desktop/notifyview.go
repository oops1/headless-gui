// notifyview.go — вид центра уведомлений Windows 10: состояние, метрики и
// раскладка.
//
// Один вид на две панели. Центр уведомлений и тост над треем рисуют ОДНУ
// карточку (значок, заголовок, текст с раскрытием, время, крестик, действия),
// поэтому раскладка, рисование и разбор ввода живут здесь, а панели —
// NotificationCenter и NotificationToast — лишь поставляют данные и решают,
// где стоят.
//
// Раскладка считается заново на каждое событие и каждый кадр, ничего не
// запоминая: тема, язык, масштаб и акцент могут смениться на открытой панели,
// и всё, что было посчитано раньше, устарело бы вместе с ними. Дёшево это
// потому, что ширины строк кешируются измерителем движка (widget.MeasureUIText*).
// Раскладка одна и для рисования, и для мыши: нажимается то, что нарисовано.
package desktop

import (
	"image"
	"strings"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Имена компонента и ключи метрик центра уведомлений Windows 10. Размеры в
// логических пикселях объявляет профиль (theme/profiles_win10_notify.go).
const (
	// ComponentNotificationCenter — имя компонента в стилях темы. У плоских
	// тем оно продолжает "notifications" (Profile.SetStyleBase), у Windows 10
	// добавляет части panel, header, link, group, card, action, field, glyph,
	// dim, quick.tile, quick.tile.on, scrollbar, toast.
	ComponentNotificationCenter = "notificationcenter"

	// KeyNotificationCenterWidth — ширина панели (360–400 в Windows 10).
	KeyNotificationCenterWidth theme.Key = "notificationcenter.width"
	// KeyNotificationCenterMargin — зазор между панелью и панелью задач. Ноль:
	// центр Windows 10 лежит на панели вплотную. У прочих всплывающих панелей
	// зазор свой (Flyout.Margin) и этой метрикой не меняется.
	KeyNotificationCenterMargin theme.Key = "notificationcenter.margin"
	// KeyNotificationToastTimeout — сколько миллисекунд тост висит на экране.
	KeyNotificationToastTimeout theme.Key = "notificationcenter.toast.timeout"

	ncKeyPad         theme.Key = "notificationcenter.pad"
	ncKeyHeader      theme.Key = "notificationcenter.header.height"
	ncKeyGroup       theme.Key = "notificationcenter.group.height"
	ncKeyGroupIcon   theme.Key = "notificationcenter.group.icon"
	ncKeyCardGap     theme.Key = "notificationcenter.card.gap"
	ncKeyCardPad     theme.Key = "notificationcenter.card.pad"
	ncKeyCardIcon    theme.Key = "notificationcenter.card.icon"
	ncKeyCardIconGap theme.Key = "notificationcenter.card.icon.gap"
	ncKeyCardSlot    theme.Key = "notificationcenter.card.slot"
	ncKeyCardLines   theme.Key = "notificationcenter.card.lines"
	ncKeyCardLinesX  theme.Key = "notificationcenter.card.lines.max"
	ncKeyActionH     theme.Key = "notificationcenter.action.height"
	ncKeyActionGap   theme.Key = "notificationcenter.action.gap"
	ncKeyActionRow   theme.Key = "notificationcenter.action.row.gap"
	ncKeyFooter      theme.Key = "notificationcenter.footer.height"
	ncKeyListMin     theme.Key = "notificationcenter.list.min"
	ncKeyQuickCols   theme.Key = "notificationcenter.quick.columns"
	ncKeyQuickH      theme.Key = "notificationcenter.quick.height"
	ncKeyQuickGap    theme.Key = "notificationcenter.quick.gap"
	ncKeyQuickPad    theme.Key = "notificationcenter.quick.pad"
	ncKeyQuickIcon   theme.Key = "notificationcenter.quick.icon"
	ncKeyScrollbar   theme.Key = "notificationcenter.scrollbar.width"
	ncKeySeverity    theme.Key = "notificationcenter.severity.width"
	ncKeyToastWidth  theme.Key = "notificationcenter.toast.width"
	ncKeyToastMargin theme.Key = "notificationcenter.toast.margin"

	// Центр Windows 11: зазор до края экрана и панели задач, зазор между центром
	// и календарём под ним, сторона кнопок заголовка (колокольчик, «Очистить все»).
	ncKeyEdge     theme.Key = "notificationcenter.edge"
	ncKeyStackGap theme.Key = "notificationcenter.stack.gap"
	ncKeyHeadBtn  theme.Key = "notificationcenter.header.button"
)

// Именованные шрифты темы, которыми пишется центр. Нет в теме — берётся шрифт
// стиля (см. namedFont).
const (
	ncFontTitle   theme.Key = "title"
	ncFontCaption theme.Key = "caption"
	// ncFontHeading — заголовок панели Windows 11 («Уведомления»).
	ncFontHeading theme.Key = "heading"
)

// Части стиля центра (к компоненту "notificationcenter").
const (
	ncPartPanel     = "panel"
	ncPartToast     = "toast"
	ncPartHeader    = "header"
	ncPartLink      = "link"
	ncPartGroup     = "group"
	ncPartCard      = "card"
	ncPartAction    = "action"
	ncPartField     = "field"
	ncPartGlyph     = "glyph"
	ncPartDim       = "dim"
	ncPartTile      = "quick.tile"
	ncPartTileOn    = "quick.tile.on"
	ncPartScrollbar = "scrollbar"
	ncPartSevWarn   = "severity.warning"
	ncPartSevError  = "severity.error"
	// Части центра Windows 11: кнопки заголовка и счётчик группы.
	ncPartHeadBtn = "headbtn"
	ncPartPill    = "pill"
)

// Доли и пределы, которые не размеры, а пропорции или защита от вырожденных
// тем (как делители в tray.go).
const (
	// ncLineRatio — межстрочный интервал: доля пикселя кегля. Windows 10 ставит
	// строки текста уведомления с шагом около 20 точек при 9 pt.
	ncLineRatio = 1.6
	// ncGlyphEm — высота ячейки глифа (подъём + спуск) в кеглях.
	ncGlyphEm = 1.36
	// ncPxPerPt — пикселей в пункте при 96 dpi.
	ncPxPerPt = 4.0 / 3.0
	// ncMaxButtonsInRow — сколько кнопок действий встают в один ряд; остальные
	// переходят в следующий.
	ncMaxButtonsInRow = 3
	// ncFallbackHeight — высота панели, пока ей не назван экран: без него
	// центр не знает, до какого края тянуться.
	ncFallbackHeight = 600
	// ncWheelStep — на сколько точек сдвигает список один тик колеса.
	ncWheelStep = 48
	// ncThumbMin — наименьшая высота бегунка полосы прокрутки.
	ncThumbMin = 24
	// ncSevMin — наименьшая ширина полосы важности, даже если метрика задана
	// нулём: полоса в точку шириной не видна.
	ncSevMin = 1
	// ncThumbHold — сколько бегунок остаётся на виду после прокрутки.
	ncThumbHold = 1200 * time.Millisecond
	// ncBodyMaxScan — защита от текста-простыни: дальше этого числа строк
	// раскладка не разбирает.
	ncBodyMaxScan = 400
)

// ─── Метрики ─────────────────────────────────────────────────────────────────

// ncMetrics — размеры центра, прочитанные из темы за один раз.
type ncMetrics struct {
	width, margin                          int
	pad, headerH, groupH, groupIcon        int
	cardGap, cardPad, cardIcon, cardIconGp int
	slot, lines, linesMax                  int
	actH, actGap, actRow                   int
	footerH, listMin                       int
	qCols, qH, qGap, qPad, qIcon           int
	sbW, sevW                              int
	toastW, toastMargin                    int
	edge, stackGap, headBtn                int
}

// metrics читает размеры из темы. Нулевое значение вместо положительного
// заменяется минимумом: профиль без этих ключей (чужой, написанный до центра
// Windows 10) должен получить работающую панель, а не пустую.
func ncReadMetrics(tm *theme.Manager) ncMetrics {
	get := func(k theme.Key, def int) int {
		if tm != nil {
			if v := int(tm.GetMetric(k)); v > 0 {
				return v
			}
		}
		return def
	}
	zero := func(k theme.Key) int {
		if tm != nil {
			if v := int(tm.GetMetric(k)); v > 0 {
				return v
			}
		}
		return 0
	}
	return ncMetrics{
		width:       get(KeyNotificationCenterWidth, 396),
		margin:      zero(KeyNotificationCenterMargin),
		pad:         get(ncKeyPad, 16),
		headerH:     get(ncKeyHeader, 48),
		groupH:      get(ncKeyGroup, 40),
		groupIcon:   get(ncKeyGroupIcon, 16),
		cardGap:     get(ncKeyCardGap, 4),
		cardPad:     get(ncKeyCardPad, 16),
		cardIcon:    get(ncKeyCardIcon, 48),
		cardIconGp:  get(ncKeyCardIconGap, 16),
		slot:        get(ncKeyCardSlot, 32),
		lines:       get(ncKeyCardLines, 2),
		linesMax:    get(ncKeyCardLinesX, 8),
		actH:        get(ncKeyActionH, 32),
		actGap:      get(ncKeyActionGap, 4),
		actRow:      get(ncKeyActionRow, 12),
		footerH:     get(ncKeyFooter, 32),
		listMin:     get(ncKeyListMin, 96),
		qCols:       get(ncKeyQuickCols, 4),
		qH:          get(ncKeyQuickH, 64),
		qGap:        get(ncKeyQuickGap, 4),
		qPad:        get(ncKeyQuickPad, 10),
		qIcon:       get(ncKeyQuickIcon, 16),
		sbW:         get(ncKeyScrollbar, 4),
		sevW:        get(ncKeySeverity, 3),
		toastW:      get(ncKeyToastWidth, 364),
		toastMargin: zero(ncKeyToastMargin),
		edge:        zero(ncKeyEdge),
		stackGap:    get(ncKeyStackGap, 8),
		headBtn:     get(ncKeyHeadBtn, 32),
	}
}

// ─── Текст ───────────────────────────────────────────────────────────────────

// ncText — шрифт строки: кегль и имя начертания для движка.
type ncText struct {
	size float64
	face string
}

// width возвращает ширину строки.
func (t ncText) width(s string) int {
	if s == "" {
		return 0
	}
	return widget.MeasureUITextFont(s, t.size, t.face)
}

// lineH — высота строки с межстрочным интервалом.
func (t ncText) lineH() int { return int(t.size*ncPxPerPt*ncLineRatio + 0.5) }

// glyphH — высота ячейки глифа: на сколько строка выше самого текста.
func (t ncText) glyphH() int { return int(t.size*ncPxPerPt*ncGlyphEm + 0.5) }

// elide укорачивает строку до ширины maxW, добавляя многоточие.
func (t ncText) elide(s string, maxW int) string {
	if s == "" || maxW <= 0 {
		return ""
	}
	if t.width(s) <= maxW {
		return s
	}
	const ellipsis = "…"
	rs := []rune(s)
	for n := len(rs) - 1; n > 0; n-- {
		cand := strings.TrimRight(string(rs[:n]), " ") + ellipsis
		if t.width(cand) <= maxW {
			return cand
		}
	}
	return ""
}

// wrap разбивает текст по словам на строки не шире maxW, не больше maxLines.
// Если текст не поместился, последняя строка заканчивается многоточием, а
// truncated истинно.
func (t ncText) wrap(s string, maxW, maxLines int) (lines []string, truncated bool) {
	if s == "" || maxW <= 0 || maxLines <= 0 {
		return nil, false
	}
	var all []string
paragraphs:
	for _, para := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			all = append(all, "")
			continue
		}
		cur := ""
		for _, w := range words {
			cand := w
			if cur != "" {
				cand = cur + " " + w
			}
			if t.width(cand) <= maxW {
				cur = cand
				continue
			}
			if cur != "" {
				all = append(all, cur)
				cur = ""
			}
			// Слово шире строки: ломаем его по символам.
			for t.width(w) > maxW {
				n := 1
				rs := []rune(w)
				for n < len(rs) && t.width(string(rs[:n+1])) <= maxW {
					n++
				}
				all = append(all, string(rs[:n]))
				w = string(rs[n:])
				if len(all) > ncBodyMaxScan {
					break paragraphs
				}
			}
			cur = w
		}
		if cur != "" {
			all = append(all, cur)
		}
		if len(all) > ncBodyMaxScan {
			break
		}
	}
	if len(all) <= maxLines {
		return all, false
	}
	lines = all[:maxLines]
	// Последняя строка берёт хвост текста: многоточие стоит там, где обрыв.
	last := lines[maxLines-1]
	if rest := all[maxLines]; rest != "" {
		last += " " + rest
	}
	lines[maxLines-1] = t.elide(last+"…", maxW)
	return lines, true
}

// ─── Состояние вида ──────────────────────────────────────────────────────────

// ncInputKey — поле ввода внутри карточки: уведомление и действие.
type ncInputKey struct {
	note   NotificationID
	action string
}

// ncReply — поле ответа: текст и положение каретки (в рунах).
type ncReply struct {
	text  []rune
	caret int
}

// zoneKind — вид зоны, на которую можно навести и нажать.
type zoneKind uint8

const (
	zoneNone zoneKind = iota
	zoneManage
	zoneClear
	zoneExpand
	zoneGroup
	zoneGroupClose
	zoneGroupToggle
	zoneCard
	zoneCardClose
	zoneCardToggle
	zoneAction
	zoneReply
	zoneReplySend
	zoneSelect
	zoneOption
	zoneTile
	zoneDND // колокольчик «Не беспокоить» в заголовке центра Windows 11
)

// zoneKey однозначно называет зону, не привязывая её к положению: раскладка
// меняется (прокрутка, новое уведомление), а наведение, нажатие и фокус
// остаются на том же элементе.
type zoneKey struct {
	kind   zoneKind
	note   NotificationID
	app    AppID
	action string // ID действия или плитки
	index  int    // номер пункта выпадающего списка
}

// zone — прямоугольник с именем.
type zone struct {
	key zoneKey
	// rect — где зона нарисована; hit — где её можно нажать (rect без части,
	// ушедшей за край списка).
	rect, hit image.Rectangle
	focusable bool
}

// ncMode — что показывает вид: центр целиком или одну карточку тоста.
type ncMode int

const (
	ncModeCenter ncMode = iota
	ncModeToast
)

// ncSource — откуда вид берёт данные и куда отдаёт действия пользователя.
type ncSource struct {
	notes   func() []Notification
	quick   func() []QuickAction
	dismiss func(NotificationID)
	toggle  func(QuickActionID)
	manage  func()
	action  func(ev NotificationActionEvent, keep bool)
	invalid func(r image.Rectangle) // перерисовать область (абсолютные координаты)
	// invalidAll перерисовывает всю панель.
	invalidAll func()
	culture    func() DateCulture
	now        func() time.Time
	emptyMsg   func() string
	// Центр Windows 11: «Не беспокоить» (включено ли и переключить) и передача
	// клавиатурного фокуса календарю под центром (delta — направление Tab;
	// true — календарь принял фокус).
	dnd       func() bool
	toggleDND func()
	handoff   func(delta int) bool
}

// richView — вид центра: состояние пользовательского интерфейса и всё, что из
// него следует. Данные (уведомления, быстрые действия) принадлежат модели
// потребителя и читаются при каждой раскладке.
type richView struct {
	tm   *theme.Manager
	mode ncMode
	src  ncSource

	mo motion // плавные переходы цвета при наведении

	mu         sync.Mutex
	collapsed  map[AppID]bool          // свёрнутые группы
	bodyOpen   map[NotificationID]bool // раскрытие карточки, заданное пользователем
	quickOpen  bool                    // сетка быстрых действий развёрнута
	scroll     int
	hover      zoneKey
	pressed    zoneKey
	focus      zoneKey
	sel        map[ncInputKey]int
	reply      map[ncInputKey]*ncReply
	drop       ncInputKey // открытый выпадающий список (нулевой — нет)
	dropHot    int        // подсвеченный пункт списка
	thumbUntil time.Time
	thumbTimer *time.Timer
	drag       bool                    // бегунок тянут мышью
	dragGrab   int                     // на сколько ниже верха бегунка схвачена точка
	thumbHot   bool                    // курсор над бегунком
	slides     map[ncSlideKey]*ncSlide // плавное раскрытие карточек, групп и быстрых действий
	starts     []func()                // анимации, которые раскладка просит запустить после снятия замка
	lastLayout richLayout              // последняя раскладка: для прокрутки колесом и подсветки
	qsnap      []QuickAction           // быстрые действия, как их видел вид в последний раз
}

// setQuickSnapshot запоминает набор быстрых действий, с которым сравнивается
// следующее изменение.
func (v *richView) setQuickSnapshot(list []QuickAction) {
	v.mu.Lock()
	v.qsnap = append(v.qsnap[:0], list...)
	v.mu.Unlock()
}

// quickDiff сравнивает новый набор быстрых действий с прежним. Пока набор и
// порядок плиток те же, возвращает прямоугольники плиток, у которых изменились
// название, включённость или недоступность, и true; иначе false — перерисовать
// надо всю панель. Одно лишь изменение значка не замечается: его отдаёт
// потребитель вместе со сменой On либо пересоставляя набор.
func (v *richView) quickDiff(list []QuickAction) ([]image.Rectangle, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	prev := v.qsnap
	v.qsnap = append([]QuickAction(nil), list...)
	if len(prev) != len(list) || len(v.lastLayout.tiles) == 0 {
		return nil, false
	}
	var rects []image.Rectangle
	for i := range list {
		if prev[i].ID != list[i].ID {
			return nil, false
		}
		if prev[i].On == list[i].On && prev[i].Disabled == list[i].Disabled && prev[i].Title == list[i].Title {
			continue
		}
		for _, t := range v.lastLayout.tiles {
			if t.a.ID == list[i].ID {
				rects = append(rects, t.rect)
			}
		}
	}
	return rects, true
}

// newRichView создаёт вид.
func newRichView(tm *theme.Manager, mode ncMode, src ncSource) *richView {
	return &richView{tm: tm, mode: mode, src: src}
}

// reset сбрасывает недолговечное состояние при закрытии панели: наведение,
// нажатие, открытый список и введённые ответы. Свёрнутые группы и раскрытые
// карточки остаются — это выбор пользователя, а не жест.
func (v *richView) reset() {
	v.mu.Lock()
	v.hover, v.pressed = zoneKey{}, zoneKey{}
	v.drop, v.dropHot = ncInputKey{}, 0
	v.reply = nil
	v.focus = zoneKey{}
	v.scroll = 0
	v.drag, v.thumbHot = false, false
	v.forgetSlides(nil, nil)
	if v.thumbTimer != nil {
		v.thumbTimer.Stop()
		v.thumbTimer = nil
	}
	v.thumbUntil = time.Time{}
	v.mu.Unlock()
}

// part читает стиль части компонента.
func (v *richView) part(part string, st theme.State) *theme.Style {
	if v.tm == nil {
		return &theme.Style{}
	}
	return v.tm.GetStyle(ComponentNotificationCenter, part, st)
}

// namedFont возвращает шрифт из набора темы под именем name; если темa такого
// не объявила — шрифт base (с утолщением, если bold).
func (v *richView) namedFont(name theme.Key, base theme.FontSpec, bold bool, scale float64) theme.FontSpec {
	return themeFont(v.tm, name, base, bold, scale)
}

// themeFont возвращает шрифт из набора темы под именем name; если тема такого не
// объявила — шрифт base, утолщённый (bold) и умноженный на scale. Им же пользуется
// календарь Windows 11.
func themeFont(tm *theme.Manager, name theme.Key, base theme.FontSpec, bold bool, scale float64) theme.FontSpec {
	if tm != nil {
		if f, ok := tm.GetFont(name); ok {
			if f.Size <= 0 {
				f.Size = base.Size
			}
			if f.Family == "" {
				f.Family = base.Family
			}
			return f
		}
	}
	f := base
	if f.Size <= 0 {
		f.Size = widget.DefaultFontSizePt
	}
	f.Size *= scale
	if bold && f.Weight == 0 {
		f.Bold = true
	}
	return f
}

// ncFonts — шрифты строк вида.
type ncFonts struct {
	body, title, caption, link ncText
	// heading — заголовок панели Windows 11.
	heading ncText
}

func ncTextOf(f theme.FontSpec) ncText {
	size := f.Size
	if size <= 0 {
		size = widget.DefaultFontSizePt
	}
	return ncText{size: size, face: FontFaceName(f)}
}

func (v *richView) fonts() ncFonts {
	base := v.part(ncPartCard, theme.StateNormal).Font
	return ncFonts{
		body:    ncTextOf(base),
		title:   ncTextOf(v.namedFont(ncFontTitle, base, true, 1.1)),
		caption: ncTextOf(v.namedFont(ncFontCaption, base, false, 0.9)),
		link:    ncTextOf(base),
		heading: ncTextOf(v.namedFont(ncFontHeading, base, true, 1.5)),
	}
}

// ─── Раскладка ───────────────────────────────────────────────────────────────

// richCard — одна карточка в раскладке.
type richCard struct {
	n          Notification
	rect       image.Rectangle
	icon       image.Rectangle
	hasIcon    bool
	title      image.Rectangle
	titleText  string
	bodyX      int
	bodyY      int
	bodyLines  []string
	timeRect   image.Rectangle
	timeText   string
	open       bool
	canToggle  bool
	closeRect  image.Rectangle
	toggleRect image.Rectangle
	acts       []richAct
	// anim — высота карточки сейчас меняется: содержимое раскрытого вида
	// обрезается её рамкой.
	anim bool
	// Карточка Windows 11: строка приложения (значок в icon, название в
	// appRect) есть только у одиночного уведомления; в группе приложение
	// названо её заголовком, а время стоит в строке заголовка карточки.
	showApp bool
	appRect image.Rectangle
	appText string
}

// richAct — действие карточки в раскладке.
type richAct struct {
	a     NotificationAction
	rect  image.Rectangle // кнопка, ссылка, список целиком или поле ответа
	label image.Rectangle // подпись над выпадающим списком
	send  image.Rectangle // кнопка отправки ответа
	text  string          // надпись кнопки или ссылки, уже усечённая по ширине
}

// richGroup — группа уведомлений одного приложения.
type richGroup struct {
	app         AppID
	name        string
	note        Notification // первое уведомление группы: источник значка
	rect        image.Rectangle
	closeRect   image.Rectangle
	chevronRect image.Rectangle
	collapsed   bool
	count       int
	cards       []richCard
	// anim — группа сворачивается или раскрывается; clip — видимая часть её
	// карточек в этот момент (может быть пуста).
	anim bool
	clip image.Rectangle
}

// richTile — плитка быстрого действия.
type richTile struct {
	a    QuickAction
	rect image.Rectangle
}

// richLayout — полная раскладка панели.
type richLayout struct {
	m        ncMetrics
	fonts    ncFonts
	panel    image.Rectangle
	header   image.Rectangle
	manage   image.Rectangle // ссылка в заголовке
	viewport image.Rectangle
	content  image.Rectangle // горизонтальные границы содержимого списка
	footer   image.Rectangle
	expand   image.Rectangle
	clear    image.Rectangle
	quick    image.Rectangle
	tiles    []richTile
	canExp   bool
	groups   []richGroup
	contentH int
	scroll   int
	maxScrl  int
	empty    bool
	zones    []zone

	// Выпадающий список.
	dropRect  image.Rectangle
	dropNote  NotificationID
	dropAct   string
	dropItems []string

	// Центр Windows 11: заголовок, колокольчик «Не беспокоить», «Очистить все»
	// (неактивно, пока уведомлений нет) и признак раскладки.
	w11      bool
	heading  image.Rectangle
	dnd      image.Rectangle
	clearOff bool
}

// groupNotes раскладывает уведомления по группам: внутри группы новые сверху,
// группы идут по самому свежему своему уведомлению.
func groupNotes(list []Notification) [][]Notification {
	idx := map[AppID]int{}
	var groups [][]Notification
	for _, n := range list {
		i, ok := idx[n.AppID]
		if !ok {
			i = len(groups)
			idx[n.AppID] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], n)
	}
	newer := func(a, b Notification) bool {
		ta, tb := a.At(), b.At()
		if !ta.Equal(tb) {
			return ta.After(tb)
		}
		return a.ID > b.ID
	}
	for _, g := range groups {
		for i := 1; i < len(g); i++ { // вставками: списки короткие и почти упорядочены
			for j := i; j > 0 && newer(g[j], g[j-1]); j-- {
				g[j], g[j-1] = g[j-1], g[j]
			}
		}
	}
	for i := 1; i < len(groups); i++ {
		for j := i; j > 0 && newer(groups[j][0], groups[j-1][0]); j-- {
			groups[j], groups[j-1] = groups[j-1], groups[j]
		}
	}
	return groups
}

// timeLabel подписывает время уведомления: сегодняшнее — часами и минутами,
// вчерашнее — словом, прежние — датой.
func (v *richView) timeLabel(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	cu := cultureOrDefault(nil)
	if v.src.culture != nil {
		cu = cultureOrDefault(v.src.culture())
	}
	now := time.Now()
	if v.src.now != nil {
		now = v.src.now()
	}
	t = t.In(now.Location())
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	yy, ym, yd := now.AddDate(0, 0, -1).Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return t.Format(cu.TimeFormat())
	case y1 == yy && m1 == ym && d1 == yd:
		return tr(StrNotifYesterday)
	}
	return t.Format(cu.DateFormat())
}

// cardOpen решает, раскрыта ли карточка: выбор пользователя, а без него —
// раскрыта та, у которой есть действия (иначе кнопки остались бы невидимыми).
func (v *richView) cardOpen(n Notification) bool {
	if v.mode == ncModeToast {
		return true
	}
	if o, ok := v.bodyOpen[n.ID]; ok {
		return o
	}
	return len(n.Actions) > 0
}

func (v *richView) selected(n Notification, a NotificationAction) int {
	if i, ok := v.sel[ncInputKey{n.ID, a.ID}]; ok && i >= 0 && i < len(a.Options) {
		return i
	}
	if a.Selected >= 0 && a.Selected < len(a.Options) {
		return a.Selected
	}
	return 0
}

func (v *richView) replyOf(n Notification, a NotificationAction) *ncReply {
	if r := v.reply[ncInputKey{n.ID, a.ID}]; r != nil {
		return r
	}
	return &ncReply{}
}

// layoutCard раскладывает одну карточку в прямоугольнике с левым верхним углом
// (x, y) и шириной w. Возвращает карточку с заполненными прямоугольниками;
// высота — rect.Dy().
func (v *richView) layoutCard(m ncMetrics, f ncFonts, n Notification, x, y, w int, hasIcon bool) richCard {
	open := v.cardOpen(n)
	k, live := v.slideStep(ncSlideKey{kind: slideCard, note: n.ID}, open)
	if !live {
		return v.cardAs(m, f, n, x, y, w, hasIcon, open)
	}
	// Раскрытие идёт: высота — между свёрнутой и раскрытой, а содержимое берётся
	// от раскрытого вида и обрезается рамкой карточки. Текст не перекладывается
	// посреди движения, строки просто открываются снизу вверх.
	full := v.cardAs(m, f, n, x, y, w, hasIcon, true)
	short := v.cardAs(m, f, n, x, y, w, hasIcon, false)
	hf, hs := full.rect.Dy(), short.rect.Dy()
	if hf == hs {
		// Раскрывать нечего (текст в две строки, действий нет): без движения.
		v.slideSettle(ncSlideKey{kind: slideCard, note: n.ID}, open)
		return v.cardAs(m, f, n, x, y, w, hasIcon, open)
	}
	full.rect.Max.Y = y + hs + int(float64(hf-hs)*k+0.5)
	full.open, full.anim = open, true
	return full
}

// cardAs раскладывает карточку видом активной темы: Windows 11 или Windows 10.
// hasIcon у Windows 11 — признак «показать строку приложения» (одиночное
// уведомление), а не крупного значка.
func (v *richView) cardAs(m ncMetrics, f ncFonts, n Notification, x, y, w int, hasIcon, open bool) richCard {
	if v.w11() {
		return v.layoutCardAsW11(m, f, n, x, y, w, hasIcon, open)
	}
	return v.layoutCardAs(m, f, n, x, y, w, hasIcon, open)
}

// layoutCardAs раскладывает карточку, раскрытую (open) или свёрнутую.
func (v *richView) layoutCardAs(m ncMetrics, f ncFonts, n Notification, x, y, w int, hasIcon, open bool) richCard {
	c := richCard{n: n, hasIcon: hasIcon}
	c.open = open
	p := m.cardPad
	iconW := 0
	if hasIcon {
		iconW = m.cardIcon + m.cardIconGp
	}
	tx := x + p + iconW
	textRight := x + w - m.slot - 4
	if textRight < tx+8 {
		textRight = tx + 8
	}
	textW := textRight - tx

	cy := y + p
	if hasIcon {
		c.icon = image.Rect(x+p, y+p, x+p+m.cardIcon, y+p+m.cardIcon)
	}
	// Заголовок.
	c.titleText = f.title.elide(n.Title, textW)
	c.title = image.Rect(tx, cy, textRight, cy+f.title.lineH())
	cy += f.title.lineH()
	// Текст.
	if n.Body != "" {
		cy += 2
		short, shortCut := f.body.wrap(n.Body, textW, m.lines)
		full, _ := f.body.wrap(n.Body, textW, m.linesMax)
		c.bodyLines = short
		if c.open {
			c.bodyLines = full
		}
		c.bodyX, c.bodyY = tx, cy
		cy += len(c.bodyLines) * f.body.lineH()
		// Раскрывать (и сворачивать) есть что, когда текст длиннее свёрнутого вида.
		c.canToggle = shortCut || len(full) > len(short)
	}
	if len(n.Actions) > 0 {
		c.canToggle = true
	}
	if v.mode == ncModeToast {
		c.canToggle = false // тост показывает всё сразу и не сворачивается
	}
	// Время.
	c.timeText = v.timeLabel(n.At())
	if c.timeText != "" {
		cy += 4
		c.timeRect = image.Rect(tx, cy, textRight, cy+f.caption.lineH())
		cy += f.caption.lineH()
	}
	// Действия.
	if c.open && len(n.Actions) > 0 {
		cy += m.actRow
		c.acts, cy = v.layoutActions(m, f, n, x+p, cy, w-2*p)
	}
	cy += p
	if hasIcon && cy < y+2*p+m.cardIcon {
		cy = y + 2*p + m.cardIcon
	}
	c.rect = image.Rect(x, y, x+w, cy)
	c.closeRect = image.Rect(x+w-m.slot, y, x+w, y+m.slot)
	c.toggleRect = image.Rect(x+w-m.slot, y+m.slot-2, x+w, y+2*m.slot-2)
	return c
}

// layoutActions раскладывает действия карточки рядами сверху вниз и
// возвращает нижнюю границу.
func (v *richView) layoutActions(m ncMetrics, f ncFonts, n Notification, x, y, w int) ([]richAct, int) {
	var out []richAct
	acts := n.Actions
	i := 0
	for i < len(acts) {
		a := acts[i]
		switch a.Kind {
		case NotificationActionSelect:
			ra := richAct{a: a}
			ra.label = image.Rect(x, y, x+w, y+f.body.lineH())
			y += f.body.lineH() + 4
			ra.rect = image.Rect(x, y, x+w, y+m.actH)
			y += m.actH
			out = append(out, ra)
			i++
		case NotificationActionReply:
			ra := richAct{a: a}
			label := a.Title
			if label == "" {
				label = tr(StrNotifReply)
			}
			sendW := f.body.width(label) + 2*m.cardPad
			if min := 4 * m.actH; sendW < min {
				sendW = min
			}
			if sendW > w/2 {
				sendW = w / 2
			}
			ra.text = f.body.elide(label, sendW-m.cardPad)
			ra.send = image.Rect(x+w-sendW, y, x+w, y+m.actH)
			ra.rect = image.Rect(x, y, x+w-sendW-m.actGap, y+m.actH)
			y += m.actH
			out = append(out, ra)
			i++
		case NotificationActionLink:
			cx := x
			rowH := m.actH
			for i < len(acts) && acts[i].Kind == NotificationActionLink {
				l := acts[i]
				tw := f.link.width(l.Title)
				bw := tw + m.cardPad
				if cx+bw > x+w && cx > x {
					break
				}
				if bw > w {
					bw = w
				}
				out = append(out, richAct{a: l, text: f.link.elide(l.Title, bw-m.cardPad),
					rect: image.Rect(cx, y, cx+bw, y+rowH)})
				cx += bw
				i++
			}
			y += rowH
		default: // кнопки: до ncMaxButtonsInRow в ряд, поровну
			j := i
			for j < len(acts) && acts[j].Kind == NotificationActionButton && j-i < ncMaxButtonsInRow {
				j++
			}
			k := j - i
			for q := 0; q < k; q++ {
				x0 := x + (w+m.actGap)*q/k
				x1 := x + (w+m.actGap)*(q+1)/k - m.actGap
				b := acts[i+q]
				out = append(out, richAct{a: b, text: f.body.elide(b.Title, x1-x0-m.cardPad),
					rect: image.Rect(x0, y, x1, y+m.actH)})
			}
			y += m.actH
			i = j
		}
		if i < len(acts) {
			y += m.actGap * 2
		}
	}
	return out, y
}

// quickRows — сколько рядов плиток помещается в раскрытой сетке: все, но не
// больше, чем оставляет списку его наименьшая высота.
func (v *richView) quickRows(m ncMetrics, panel image.Rectangle, rows int) int {
	avail := panel.Dy() - m.headerH - m.footerH - m.listMin - 2*m.qGap
	maxRows := (avail + m.qGap) / (m.qH + m.qGap)
	if maxRows < 1 {
		maxRows = 1
	}
	if rows > maxRows {
		rows = maxRows
	}
	return rows
}

// hasIcon сообщает, есть ли у уведомления значок.
func hasNoteIcon(n Notification) bool { return n.Icon != nil || n.IconAt != nil }

// layout строит раскладку панели panel для текущих данных и состояния.
func (v *richView) layout(panel image.Rectangle) *richLayout {
	m := ncReadMetrics(v.tm)
	f := v.fonts()
	l := &richLayout{m: m, fonts: f, panel: panel}
	if v.mode == ncModeToast {
		v.layoutToast(l)
		return l
	}
	if v.w11() {
		v.layoutW11(l)
		return l
	}

	var notes []Notification
	if v.src.notes != nil {
		notes = v.src.notes()
	}
	var quick []QuickAction
	if v.src.quick != nil {
		quick = v.src.quick()
	}

	// Анимации раскрытия запускаются уже без замка (см. slideStep): defer идут
	// в обратном порядке, и flushSlides отработает после Unlock.
	defer v.flushSlides()
	v.mu.Lock()
	defer v.mu.Unlock()

	pad := m.pad
	x0 := panel.Min.X + pad
	w := panel.Dx() - 2*pad
	if w < 1 {
		w = 1
	}
	l.content = image.Rect(x0, panel.Min.Y, x0+w, panel.Max.Y)

	// Заголовок: ссылка «Управление уведомлениями» у правого края.
	l.header = image.Rect(panel.Min.X, panel.Min.Y, panel.Max.X, panel.Min.Y+m.headerH)
	manageLabel := f.link.elide(tr(StrNotifManage), w)
	mw := f.link.width(manageLabel) + 16
	linkH := m.actH
	ly := l.header.Min.Y + (m.headerH-linkH)/2
	l.manage = image.Rect(x0+w+8-mw, ly, x0+w+8, ly+linkH)

	// Нижняя часть: быстрые действия, над ними строка ссылок.
	bottom := panel.Max.Y - m.qGap
	if n := len(quick); n > 0 {
		rows := (n + m.qCols - 1) / m.qCols
		l.canExp = rows > 1
		show := 1
		if v.quickOpen {
			show = v.quickRows(m, panel, rows)
		}
		qh := show*m.qH + (show-1)*m.qGap
		if l.canExp {
			// Сетка раскрывается плавно: высота едет между одним рядом и всеми,
			// плитки лежат в раскрытой раскладке и обрезаются рамкой сетки.
			if k, live := v.slideStep(ncSlideKey{kind: slideQuick}, v.quickOpen); live {
				if rowsFull := v.quickRows(m, panel, rows); rowsFull > 1 {
					fullH := rowsFull*m.qH + (rowsFull-1)*m.qGap
					qh = m.qH + int(float64(fullH-m.qH)*k+0.5)
					show = rowsFull
				} else {
					v.slideSettle(ncSlideKey{kind: slideQuick}, v.quickOpen)
				}
			}
		}
		l.quick = image.Rect(x0, bottom-qh, x0+w, bottom)
		visible := show * m.qCols
		if visible > n {
			visible = n
		}
		for i := 0; i < visible; i++ {
			col, row := i%m.qCols, i/m.qCols
			x1 := x0 + (w+m.qGap)*col/m.qCols
			x2 := x0 + (w+m.qGap)*(col+1)/m.qCols - m.qGap
			y1 := l.quick.Min.Y + row*(m.qH+m.qGap)
			l.tiles = append(l.tiles, richTile{a: quick[i], rect: image.Rect(x1, y1, x2, y1+m.qH)})
		}
		bottom = l.quick.Min.Y - m.qGap
	}
	if len(notes) > 0 || l.canExp {
		l.footer = image.Rect(panel.Min.X, bottom-m.footerH, panel.Max.X, bottom)
		ty := l.footer.Min.Y + (m.footerH-linkH)/2
		if l.canExp {
			label := tr(StrNotifExpand)
			if v.quickOpen {
				label = tr(StrNotifCollapse)
			}
			ew := f.link.width(label) + 16
			l.expand = image.Rect(x0-8, ty, x0-8+ew, ty+linkH)
		}
		if len(notes) > 0 {
			cw := f.link.width(tr(StrNotifClear)) + 16
			l.clear = image.Rect(x0+w+8-cw, ty, x0+w+8, ty+linkH)
		}
		bottom = l.footer.Min.Y
	}
	// image.Rect переставил бы перевёрнутые границы местами и получил бы окно
	// выше шапки; на крохотной панели окно списка пусто и сидит под шапкой.
	top := l.header.Max.Y
	if top > panel.Max.Y {
		top = panel.Max.Y
	}
	if bottom < top {
		bottom = top
	}
	l.viewport = image.Rectangle{Min: image.Pt(panel.Min.X, top), Max: image.Pt(panel.Max.X, bottom)}

	// Содержимое списка в системе координат «сверху вниз от начала списка».
	groups, y := v.flowGroups(m, f, notes, x0, w)
	l.groups = groups
	l.empty = len(groups) == 0
	l.contentH = y + m.cardGap
	if l.empty {
		l.contentH = 0
	}
	l.maxScrl = l.contentH - l.viewport.Dy()
	if l.maxScrl < 0 {
		l.maxScrl = 0
	}
	if v.scroll > l.maxScrl {
		v.scroll = l.maxScrl
	}
	if v.scroll < 0 {
		v.scroll = 0
	}
	l.scroll = v.scroll

	v.shiftGroups(l)

	v.buildZones(l)
	l.lastKeep(v)
	return l
}

// flowGroups раскладывает группы уведомлений сверху вниз от начала списка и
// возвращает их с высотой, до которой дошла раскладка (без нижнего зазора).
// Координаты относительные: y = 0 — начало списка, x0 и w — полоса карточек.
// Зовётся под замком вида.
func (v *richView) flowGroups(m ncMetrics, f ncFonts, notes []Notification, x0, w int) ([]richGroup, int) {
	w11 := v.w11()
	var out []richGroup
	y := 0
	groups := groupNotes(notes)
	for _, g := range groups {
		first := g[0]
		grp := richGroup{app: first.AppID, name: first.AppName, note: first, count: len(g)}
		for _, n := range g {
			if grp.name == "" {
				grp.name = n.AppName
			}
			if !hasNoteIcon(grp.note) && hasNoteIcon(n) {
				grp.note = n
			}
		}
		if grp.name == "" {
			grp.name = string(first.AppID)
		}
		grp.collapsed = v.collapsed[first.AppID]
		// Уведомления без приложения (прежняя модель) идут без заголовка группы.
		hdr := m.groupH
		if grp.name == "" {
			hdr = 0
			if !w11 {
				y += m.cardGap
			}
		}
		// Windows 11: заголовок группы нужен, только когда в ней несколько
		// уведомлений; одиночное — просто карточка со строкой приложения.
		if w11 && len(g) < 2 {
			hdr = 0
		}
		grp.rect = image.Rect(x0, y, x0+w, y+hdr)
		y += hdr
		// Группа сворачивается плавно: пока идёт движение, карточки лежат как у
		// раскрытой, а их область обрезается и укорачивается до долей высоты.
		gk, glive := 1.0, false
		if hdr > 0 {
			gk, glive = v.slideStep(ncSlideKey{kind: slideGroup, app: first.AppID}, !grp.collapsed)
		}
		if !grp.collapsed || glive {
			yCards := y
			for _, n := range g {
				iconArg := hasNoteIcon(n)
				if w11 {
					iconArg = len(g) < 2 // строка приложения — у одиночного уведомления
				}
				c := v.layoutCard(m, f, n, x0, y, w, iconArg)
				grp.cards = append(grp.cards, c)
				y = c.rect.Max.Y + m.cardGap
			}
			if glive {
				grp.anim = true
				shown := int(float64(y-yCards)*gk + 0.5)
				grp.clip = image.Rect(x0, yCards, x0+w, yCards+shown)
				y = yCards + shown
			}
		}
		out = append(out, grp)
	}
	return out, y
}

// shiftGroups переводит раскладку групп из координат списка в абсолютные:
// сдвигает на верх окна списка за вычетом прокрутки и ставит крестик и шеврон
// заголовка группы.
func (v *richView) shiftGroups(l *richLayout) {
	m := l.m
	// Сдвиг в абсолютные координаты.
	dy := l.viewport.Min.Y - l.scroll
	shift := func(r image.Rectangle) image.Rectangle { return r.Add(image.Pt(0, dy)) }
	for gi := range l.groups {
		g := &l.groups[gi]
		g.rect = shift(g.rect)
		g.clip = shift(g.clip)
		if !g.rect.Empty() {
			g.closeRect = image.Rect(g.rect.Max.X-m.slot, g.rect.Min.Y+(m.groupH-m.slot)/2, g.rect.Max.X, g.rect.Min.Y+(m.groupH-m.slot)/2+m.slot)
			g.chevronRect = g.closeRect.Sub(image.Pt(m.slot, 0))
		}
		for ci := range g.cards {
			c := &g.cards[ci]
			c.rect, c.icon = shift(c.rect), shift(c.icon)
			c.title, c.timeRect = shift(c.title), shift(c.timeRect)
			c.appRect = shift(c.appRect)
			c.bodyY += dy
			c.closeRect, c.toggleRect = shift(c.closeRect), shift(c.toggleRect)
			for ai := range c.acts {
				a := &c.acts[ai]
				a.rect, a.label, a.send = shift(a.rect), shift(a.label), shift(a.send)
			}
		}
	}
}

// lastKeep запоминает раскладку: колесо мыши и подсветка читают её, не считая
// заново.
func (l *richLayout) lastKeep(v *richView) { v.lastLayout = *l }

// buildZones составляет список зон в порядке обхода сверху вниз; зоны,
// лежащие поверх других (крестики), стоят позже — попадание ищется с конца.
func (v *richView) buildZones(l *richLayout) {
	add := func(k zoneKey, r image.Rectangle, focusable bool, clip image.Rectangle) {
		if r.Empty() {
			return
		}
		hit := r.Intersect(clip)
		if hit.Empty() {
			return
		}
		l.zones = append(l.zones, zone{key: k, rect: r, hit: hit, focusable: focusable})
	}
	whole := l.panel
	add(zoneKey{kind: zoneManage}, l.manage, true, whole)
	if l.w11 {
		// Заголовок Windows 11: колокольчик и «Очистить все»; последняя
		// недоступна, пока очищать нечего.
		add(zoneKey{kind: zoneDND}, l.dnd, true, whole)
		if !l.clearOff {
			add(zoneKey{kind: zoneClear}, l.clear, true, whole)
		}
	}
	vp := l.viewport
	for _, g := range l.groups {
		add(zoneKey{kind: zoneGroup, app: g.app}, g.rect, true, vp)
		add(zoneKey{kind: zoneGroupToggle, app: g.app}, g.chevronRect, false, vp)
		add(zoneKey{kind: zoneGroupClose, app: g.app}, g.closeRect, false, vp)
		// Пока группа сворачивается, нажимается только то, что ещё видно.
		gvp := vp
		if g.anim {
			gvp = vp.Intersect(g.clip)
		}
		for _, c := range g.cards {
			id := c.n.ID
			cvp := gvp
			if c.anim {
				cvp = gvp.Intersect(c.rect)
			}
			add(zoneKey{kind: zoneCard, note: id}, c.rect, true, gvp)
			if c.canToggle {
				add(zoneKey{kind: zoneCardToggle, note: id}, c.toggleRect, false, cvp)
			}
			add(zoneKey{kind: zoneCardClose, note: id}, c.closeRect, false, cvp)
			for _, a := range c.acts {
				switch a.a.Kind {
				case NotificationActionSelect:
					add(zoneKey{kind: zoneSelect, note: id, action: a.a.ID}, a.rect, true, cvp)
				case NotificationActionReply:
					add(zoneKey{kind: zoneReply, note: id, action: a.a.ID}, a.rect, true, cvp)
					add(zoneKey{kind: zoneReplySend, note: id, action: a.a.ID}, a.send, true, cvp)
				default:
					add(zoneKey{kind: zoneAction, note: id, action: a.a.ID}, a.rect, true, cvp)
				}
			}
		}
	}
	add(zoneKey{kind: zoneExpand}, l.expand, true, whole)
	if !l.w11 {
		add(zoneKey{kind: zoneClear}, l.clear, true, whole)
	}
	for _, t := range l.tiles {
		// Плитки обрезаются рамкой сетки: пока она раскрывается, нижний ряд
		// выглядывает не весь.
		add(zoneKey{kind: zoneTile, action: string(t.a.ID)}, t.rect, !t.a.Disabled, l.quick)
	}
	v.buildDropdown(l)
}

// buildDropdown добавляет раскрытый выпадающий список поверх остального.
func (v *richView) buildDropdown(l *richLayout) {
	if v.drop == (ncInputKey{}) {
		return
	}
	for _, g := range l.groups {
		for _, c := range g.cards {
			if c.n.ID != v.drop.note {
				continue
			}
			for _, a := range c.acts {
				if a.a.Kind != NotificationActionSelect || a.a.ID != v.drop.action {
					continue
				}
				h := len(a.a.Options) * l.m.actH
				r := image.Rect(a.rect.Min.X, a.rect.Max.Y, a.rect.Max.X, a.rect.Max.Y+h)
				if r.Max.Y > l.panel.Max.Y-l.m.qGap {
					r = image.Rect(a.rect.Min.X, a.rect.Min.Y-h, a.rect.Max.X, a.rect.Min.Y)
				}
				l.dropRect, l.dropNote, l.dropAct = r, c.n.ID, a.a.ID
				l.dropItems = a.a.Options
				for i := range a.a.Options {
					ir := image.Rect(r.Min.X, r.Min.Y+i*l.m.actH, r.Max.X, r.Min.Y+(i+1)*l.m.actH)
					l.zones = append(l.zones, zone{
						key:  zoneKey{kind: zoneOption, note: c.n.ID, action: a.a.ID, index: i},
						rect: ir, hit: ir.Intersect(l.panel),
					})
				}
				return
			}
		}
	}
}

// zoneAt находит зону под точкой (верхнюю из лежащих друг на друге).
func (l *richLayout) zoneAt(pt image.Point) (zone, bool) {
	// Раскрытый список закрывает всё под собой.
	if !l.dropRect.Empty() && pt.In(l.dropRect) {
		for i := len(l.zones) - 1; i >= 0; i-- {
			if l.zones[i].key.kind == zoneOption && pt.In(l.zones[i].hit) {
				return l.zones[i], true
			}
		}
	}
	for i := len(l.zones) - 1; i >= 0; i-- {
		z := l.zones[i]
		if z.key.kind != zoneOption && pt.In(z.hit) {
			return z, true
		}
	}
	return zone{}, false
}

// find возвращает зону по ключу.
func (l *richLayout) find(k zoneKey) (zone, bool) {
	for _, z := range l.zones {
		if z.key == k {
			return z, true
		}
	}
	return zone{}, false
}

// stops — зоны, на которые встаёт клавиатурный фокус, в порядке обхода.
func (l *richLayout) stops() []zone {
	var out []zone
	for _, z := range l.zones {
		if z.focusable {
			out = append(out, z)
		}
	}
	return out
}

// toastLayoutHeight — высота тоста с его содержимым; используется и панелью,
// чтобы назвать свой размер.
func (v *richView) toastCard(l *richLayout) (richCard, bool) {
	var notes []Notification
	if v.src.notes != nil {
		notes = v.src.notes()
	}
	if len(notes) == 0 {
		return richCard{}, false
	}
	n := notes[0]
	m := l.m
	v.mu.Lock()
	iconArg := hasNoteIcon(n)
	if v.w11() {
		iconArg = true // тост Windows 11 — всегда одиночная карточка со строкой приложения
	}
	c := v.layoutCard(m, l.fonts, n, l.panel.Min.X, l.panel.Min.Y, l.panel.Dx(), iconArg)
	v.mu.Unlock()
	return c, true
}

// layoutToast раскладывает тост: одна карточка на всю панель.
func (v *richView) layoutToast(l *richLayout) {
	c, ok := v.toastCard(l)
	if !ok {
		return
	}
	g := richGroup{app: c.n.AppID, name: c.n.AppName, note: c.n, count: 1, cards: []richCard{c}}
	l.groups = []richGroup{g}
	l.viewport = l.panel
	v.mu.Lock()
	v.buildZones(l)
	v.lastLayout = *l
	v.mu.Unlock()
}
