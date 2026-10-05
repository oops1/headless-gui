// startmenu_tiles.go — меню «Пуск» с боковой панелью, списком приложений и
// плитками: состояние, модель и раскладка.
//
// Это второй вид меню «Пуск». Плоский (startmenu.go) остаётся для тем, которые
// его просят; этот выбирается презентером: профиль называет
// Profile.Presenters["startmenu"] = theme.PresenterStartTiles, а компонент
// спрашивает у темы, есть ли для него такой (StartMenu.tiled) — имени темы он
// не знает. Один и тот же *StartMenu, одни и те же Open, Close, Toggle и
// подписки; меняется то, кто считает размер, рисует и разбирает ввод.
//
// Три области слева направо: боковая панель (свёрнутая 48, развёрнутая 256
// поверх списка, ширина едет по Tween), список приложений («Недавно
// добавленные», алфавитные группы с буквами, папки раскрываются на месте,
// собственная прокрутка и виртуализация) и плитки (группы, четыре размера,
// сетка с шагом, перетаскивание). Метрики — startmenu.* и tile.* профиля;
// литералов размеров в отрисовке нет.
//
// Раскладка и попадание считаются одной арифметикой (startGeometry и
// rowRect/tileRect), чтобы подсветка, клик и клавиатура не расходились. Модели
// (строки списка, раскладка плиток) строятся лениво и кэшируются по ревизии,
// а массивы, раз попавшие в кэш, не меняются на месте: Draw читает их без
// замка.
package desktop

import (
	"image"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Ключи метрик меню «Пуск» с плитками.
const (
	// KeyStartSidebarCollapsed и KeyStartSidebarExpanded — ширина боковой панели
	// в свёрнутом и развёрнутом виде.
	KeyStartSidebarCollapsed theme.Key = "startmenu.sidebar.collapsed"
	KeyStartSidebarExpanded  theme.Key = "startmenu.sidebar.expanded"
	// KeyStartSidebarIconSize — сторона значка пункта боковой панели.
	KeyStartSidebarIconSize theme.Key = "startmenu.sidebar.icon.size"
	// KeyStartListWidth — ширина столбца списка приложений.
	KeyStartListWidth theme.Key = "startmenu.list.width"
	// KeyStartLetterHeight — высота заголовка раздела и буквы.
	KeyStartLetterHeight theme.Key = "startmenu.letter.height"
	// KeyStartRowPad и KeyStartRowIconGap — поле слева от значка строки списка
	// и зазор между значком и названием.
	KeyStartRowPad     theme.Key = "startmenu.row.pad"
	KeyStartRowIconGap theme.Key = "startmenu.row.icon.gap"
	// Поля области плиток и число единиц сетки в группе.
	KeyStartTilesPadLeft   theme.Key = "startmenu.tiles.pad.left"
	KeyStartTilesPadRight  theme.Key = "startmenu.tiles.pad.right"
	KeyStartTilesPadTop    theme.Key = "startmenu.tiles.pad.top"
	KeyStartTilesPadBottom theme.Key = "startmenu.tiles.pad.bottom"
	KeyStartTilesColumns   theme.Key = "startmenu.tiles.columns"
	// KeyStartHeight и KeyStartHeightMin — желаемая и наименьшая высота меню;
	// желаемая ужимается под экран за вычетом панели задач (0 — во всю
	// доступную высоту).
	KeyStartHeight    theme.Key = "startmenu.height"
	KeyStartHeightMin theme.Key = "startmenu.height.min"
	// KeyStartMargin — зазор между кнопкой «Пуск» и меню.
	KeyStartMargin theme.Key = "startmenu.margin"
	// Плитки: единица сетки, зазор, заголовок группы и промежутки.
	KeyTileUnit           theme.Key = "tile.unit"
	KeyTileGap            theme.Key = "tile.gap"
	KeyTileGroupHeader    theme.Key = "tile.group.header"
	KeyTileGroupHeaderGap theme.Key = "tile.group.header.gap"
	KeyTileGroupGap       theme.Key = "tile.group.gap"
	// Тонкая полоса прокрутки: ширина в покое и под курсором.
	KeyScrollThinWidth      theme.Key = "scrollbar.thin.width"
	KeyScrollThinHoverWidth theme.Key = "scrollbar.thin.hover.width"
)

// startArea — область меню; порядок — порядок обхода Tab.
type startArea int

const (
	areaNone startArea = iota
	areaSidebar
	areaList
	areaTiles
)

// rowKind — вид строки списка приложений.
type rowKind int

const (
	rowHeader rowKind = iota // «Недавно добавленные», категория результатов
	rowLetter                // буква алфавитной группы
	rowApp                   // приложение
	rowFolder                // папка
	rowChild                 // приложение внутри раскрытой папки
	rowResult                // результат поиска
	rowEmpty                 // «Ничего не найдено»
)

// interactive — строку можно навести, нажать и выбрать клавишами.
func (k rowKind) interactive() bool {
	return k == rowApp || k == rowFolder || k == rowChild || k == rowResult
}

// focusable — на строку можно встать: приложения, папки и заголовки букв, по
// которым открывается сетка перехода (rowLetter). Результаты поиска букв не
// имеют, так что для них focusable совпадает с interactive.
func (k rowKind) focusable() bool { return k.interactive() || k == rowLetter }

// listRow — строка списка вместе с её местом в координатах содержимого.
type listRow struct {
	kind     rowKind
	key      string
	label    string
	labelKey string // ключ перевода: заголовок, который следует за языком
	sub      string
	app      AppID
	folder   string
	icon     image.Image
	iconAt   func(int) image.Image
	expanded bool
	result   SearchResult
	y, h     int
}

// Ключи наведения и выбора: префикс области + идентификатор, поэтому ключи
// разных областей не пересекаются, а выбор переживает пересборку модели.
const (
	keySideMenu = "s:\x00menu"
	prefSide    = "s:"
	prefRow     = "r:"
	prefTile    = "t:"
)

// tileGeom — плитка в раскладке: место относительно начала области плиток при
// нулевой прокрутке и индексы в модели.
type tileGeom struct {
	rect  image.Rectangle
	group int
	index int
	id    TileID
}

// groupGeom — группа плиток в раскладке.
type groupGeom struct {
	header image.Rectangle
	title  string
	first  int // индекс первой плитки в tileLayout.tiles
	count  int
	bottom int // низ сетки плиток группы (у пустой группы — её верх под заголовком)
}

// tileLayout — раскладка плиток; строится целиком и не меняется после
// публикации.
type tileLayout struct {
	cols   int
	height int
	groups []groupGeom
	tiles  []tileGeom
}

// tileKey — от чего зависит раскладка, помимо модели.
type tileKey struct {
	cols, unit, gap, header, headerGap, groupGap int
}

type rowsKey struct {
	rowH, letterH int
	searching     bool
}

// startView — состояние меню «Пуск» с плитками.
type startView struct {
	mu sync.Mutex

	// Модель.
	source  StartMenuSource
	sidebar []StartSidebarItem
	groups  []TileGroup
	folders map[string]bool

	// Кэши: ревизия растёт при любой смене входных данных; кэш считается
	// устаревшим, пока собранная ревизия не догнала текущую.
	rev        int
	rowsRev    int
	rowsC      []listRow
	contentH   int
	rowsK      rowsKey
	tilesRev   int
	tilesCRev  int // ревизия, по которой построен tilesC
	tilesC     *tileLayout
	tilesK     tileKey
	tilesBuilt bool

	// Поиск.
	query    string
	results  []SearchResult
	provider SearchProvider
	box      *SearchBox
	// keepFocus — меню открыто строкой поиска и фокус у неё забирать нельзя.
	keepFocus bool

	// Ввод.
	area       startArea
	hover      string
	press      string
	sel        map[startArea]string
	kbd        bool
	listScroll int
	tileScroll int
	sideOpen   bool

	listBar, tileBar fadeBar

	// Перетаскивание плитки.
	drag tileDrag

	// Перетаскивание бегунка полосы прокрутки мышью.
	bar barDrag

	// Сетка перехода по буквам (startmenu_tiles_letters.go): открыта ли она,
	// строка списка, с которой её открыли (туда возвращается выбор при Esc), и
	// появление по теме.
	grid     bool
	gridFrom string
	gridFade *Tween

	// Подписки на время открытости.
	unsubs []func()

	focused atomic.Bool
	popup   *widget.PopupMenu
	capture widget.CaptureManager

	// mouse — последняя позиция курсора над меню (для пересчёта наведения
	// после прокрутки под неподвижным курсором).
	mouse   image.Point
	mouseOK bool

	sideW *Tween
}

// tileDrag — состояние перетаскивания плитки.
type tileDrag struct {
	pending bool // кнопка нажата на плитке, порог ещё не пройден
	active  bool
	id      TileID
	start   image.Point
	pos     image.Point
	// Целевое место: группа и индекс вставки (после удаления перетаскиваемой).
	toGroup, toIndex int
	// toNew — плитку отпустят в пустое место между группами или под последней:
	// на месте toGroup появится новая группа из одной этой плитки.
	toNew bool
}

func newStartView(m *StartMenu) *startView {
	v := &startView{
		folders: map[string]bool{},
		sel:     map[startArea]string{},
		area:    areaList,
	}
	v.sideW = NewTween(m.tm, AnimMenuOpen, 0, m.sidebarChanged)
	v.gridFade = NewTween(m.tm, AnimMenuOpen, 0, m.gridChanged)
	return v
}

// ─── Выбор вида ──────────────────────────────────────────────────────────────

// tiled сообщает, что активная тема просит меню с плитками. Спрашивается у
// темы каждый раз: смена темы на открытом меню меняет вид без пересоздания.
func (m *StartMenu) tiled() bool {
	if m.tm == nil {
		return false
	}
	t := m.tm.Active()
	return t != nil && t.PresenterName(ComponentStartMenu) == theme.PresenterStartTiles
}

// AsTiled сообщает то же для оболочки: нужно ли ей, например, подключать
// строку поиска к этому меню.
func (m *StartMenu) AsTiled() bool { return m.tiled() }

// basePart — часть стиля, которой Flyout рисует подложку: «panel» (акрил) у
// меню с плитками, обычная у плоского.
func (m *StartMenu) basePart() string {
	if m.tiled() {
		return "panel"
	}
	return ""
}

// flyoutMargin — зазор между кнопкой и меню: токен темы у меню с плитками.
func (m *StartMenu) flyoutMargin() int {
	if m.tiled() {
		return m.metricInt(KeyStartMargin)
	}
	return m.Margin
}

func (m *StartMenu) metricInt(k theme.Key) int {
	if m.tm == nil {
		return 0
	}
	return int(m.tm.GetMetric(k))
}

// tpart — стиль части меню в состоянии st.
func (m *StartMenu) tpart(part string, st theme.State) *theme.Style {
	return styleOf(m.tm, ComponentStartMenu, part, st)
}

// withFont возвращает копию стиля с именованным шрифтом темы (caption, title).
// Темы без такого шрифта оставляют стилю его собственный.
func (m *StartMenu) withFont(s *theme.Style, name theme.Key) *theme.Style {
	if m.tm == nil || s == nil {
		return s
	}
	f, ok := m.tm.GetFont(name)
	if !ok || f.IsZero() {
		return s
	}
	c := *s
	c.Font = f
	return &c
}

// ─── Размер ──────────────────────────────────────────────────────────────────

// availHeight — сколько высоты есть у меню между кнопкой и краем экрана.
// 0 — не ограничено (экран не задан).
func (m *StartMenu) availHeight() int {
	scr := m.Screen
	if scr.Empty() {
		return 0
	}
	margin := m.flyoutMargin()
	if m.Anchor.Empty() {
		return scr.Dy()
	}
	var h int
	if m.Edge == EdgeTop {
		h = scr.Max.Y - m.Anchor.Max.Y - margin
	} else {
		h = m.Anchor.Min.Y - scr.Min.Y - margin
	}
	if h < 0 {
		h = 0
	}
	return h
}

// sizeTiled — Flyout.Size меню с плитками: сумма трёх областей, высота не
// больше доступной (экран минус панель задач) и не меньше наименьшей.
func (m *StartMenu) sizeTiled() image.Point {
	collapsed := m.metricInt(KeyStartSidebarCollapsed)
	listW := m.metricInt(KeyStartListWidth)
	if collapsed <= 0 || listW <= 0 {
		return image.Point{}
	}
	pad := int(m.style(theme.StateNormal).PadX)
	w := collapsed + listW + 2*pad
	if cols := m.metricInt(KeyStartTilesColumns); cols > 0 {
		unit, gap := m.metricInt(KeyTileUnit), m.metricInt(KeyTileGap)
		w += m.metricInt(KeyStartTilesPadLeft) + cols*unit + (cols-1)*gap + m.metricInt(KeyStartTilesPadRight)
	}
	if !m.Screen.Empty() && w > m.Screen.Dx() {
		w = m.Screen.Dx()
	}

	h := m.metricInt(KeyStartHeight)
	avail := m.availHeight()
	if h <= 0 || (avail > 0 && h > avail) {
		h = avail
	}
	if min := m.metricInt(KeyStartHeightMin); h < min {
		h = min
		if avail > 0 && h > avail {
			h = avail
		}
	}
	return image.Pt(w, h)
}

// ─── Геометрия ───────────────────────────────────────────────────────────────

// startGeo — области меню в абсолютных координатах.
type startGeo struct {
	inner     image.Rectangle // внутренность панели (без рамки)
	collapsed int
	sideW     int
	sidebar   image.Rectangle // боковая панель с текущей (анимируемой) шириной
	list      image.Rectangle
	tiles     image.Rectangle // весь столбец плиток
	tinner    image.Rectangle // где лежат группы (за вычетом полей)
	cols      int             // единиц сетки в группе; 0 — плиток нет
}

// startGeometry раскладывает внутренность панели inner на области.
func (m *StartMenu) startGeometry(inner image.Rectangle) startGeo {
	g := startGeo{inner: inner}
	g.collapsed = m.metricInt(KeyStartSidebarCollapsed)
	expanded := m.metricInt(KeyStartSidebarExpanded)
	if expanded < g.collapsed {
		expanded = g.collapsed
	}
	g.sideW = m.sidebarWidth(g.collapsed, expanded)
	g.sidebar = image.Rect(inner.Min.X, inner.Min.Y, inner.Min.X+g.sideW, inner.Max.Y)

	listW := m.metricInt(KeyStartListWidth)
	g.list = image.Rect(inner.Min.X+g.collapsed, inner.Min.Y, inner.Min.X+g.collapsed+listW, inner.Max.Y).Intersect(inner)
	g.tiles = image.Rect(g.list.Max.X, inner.Min.Y, inner.Max.X, inner.Max.Y).Intersect(inner)
	g.tinner = image.Rect(
		g.tiles.Min.X+m.metricInt(KeyStartTilesPadLeft), g.tiles.Min.Y+m.metricInt(KeyStartTilesPadTop),
		g.tiles.Max.X-m.metricInt(KeyStartTilesPadRight), g.tiles.Max.Y-m.metricInt(KeyStartTilesPadBottom))
	if g.tinner.Empty() {
		return g
	}
	unit, gap := m.metricInt(KeyTileUnit), m.metricInt(KeyTileGap)
	if unit > 0 {
		cols := (g.tinner.Dx() + gap) / (unit + gap)
		if max := m.metricInt(KeyStartTilesColumns); max > 0 && cols > max {
			cols = max
		}
		// Уже двух единиц плитка среднего размера не помещается: область скрыта.
		if cols >= 2 {
			g.cols = cols
		}
	}
	return g
}

// sidebarWidth — текущая ширина боковой панели: значение перехода, пока он
// идёт, иначе цель. Пересчёт по метрикам, а не по запомненному числу, чтобы
// смена темы на открытом меню не оставляла прежнюю ширину.
func (m *StartMenu) sidebarWidth(collapsed, expanded int) int {
	v := m.v
	if v.sideW.Animating() {
		w := int(v.sideW.Value() + 0.5)
		if w < collapsed {
			w = collapsed
		}
		if w > expanded {
			w = expanded
		}
		return w
	}
	v.mu.Lock()
	open := v.sideOpen
	v.mu.Unlock()
	if open {
		return expanded
	}
	return collapsed
}

// sidebarTarget — ширина, к которой идёт боковая панель.
func (m *StartMenu) sidebarTarget(open bool) float64 {
	if open {
		e, c := m.metricInt(KeyStartSidebarExpanded), m.metricInt(KeyStartSidebarCollapsed)
		if e < c {
			e = c
		}
		return float64(e)
	}
	return float64(m.metricInt(KeyStartSidebarCollapsed))
}

// sidebarChanged — шаг перехода ширины: перерисовывается только развёрнутая
// область боковой панели, а не меню целиком.
func (m *StartMenu) sidebarChanged() {
	r := m.contentRect()
	if r.Empty() {
		return
	}
	e := m.metricInt(KeyStartSidebarExpanded)
	widget.InvalidateRect(image.Rect(r.Min.X, r.Min.Y, r.Min.X+e, r.Max.Y).Intersect(r))
}

// SidebarExpanded сообщает, развёрнута ли боковая панель (цель, а не текущая
// ширина посреди анимации).
func (m *StartMenu) SidebarExpanded() bool {
	m.v.mu.Lock()
	defer m.v.mu.Unlock()
	return m.v.sideOpen
}

// SidebarToggleLabel — подпись гамбургера для подсказки и доступности:
// «Развернуть» у свёрнутой панели, «Свернуть» у развёрнутой. Берётся из строк
// интерфейса и следует за языком.
func (m *StartMenu) SidebarToggleLabel() string {
	if m.SidebarExpanded() {
		return tr(StrStartCollapse)
	}
	return tr(StrStartExpand)
}

// SetSidebarExpanded разворачивает или сворачивает боковую панель; ширина едет
// по анимации темы (menu.open), нулевая длительность — мгновенно.
func (m *StartMenu) SetSidebarExpanded(open bool) {
	v := m.v
	v.mu.Lock()
	changed := v.sideOpen != open
	v.sideOpen = open
	v.mu.Unlock()
	if !changed {
		return
	}
	v.sideW.To(m.sidebarTarget(open))
	m.sidebarChanged()
}

// ─── Модель: публичные настройки ─────────────────────────────────────────────

// SetSource задаёт источник списка приложений. nil возвращает источник по
// умолчанию — каталог, сгруппированный по буквам.
func (m *StartMenu) SetSource(s StartMenuSource) {
	v := m.v
	v.mu.Lock()
	v.source = s
	v.rev++
	open := m.IsOpen()
	v.mu.Unlock()
	if open {
		m.resubscribe()
		m.Invalidate()
	}
}

// SetSidebarItems задаёт пункты боковой панели (без гамбургера, он есть всегда)
// сверху вниз; панель выкладывает их у нижнего края.
func (m *StartMenu) SetSidebarItems(items []StartSidebarItem) {
	v := m.v
	v.mu.Lock()
	v.sidebar = append([]StartSidebarItem(nil), items...)
	v.mu.Unlock()
	m.Invalidate()
}

// SidebarItems возвращает копию пунктов боковой панели.
func (m *StartMenu) SidebarItems() []StartSidebarItem {
	m.v.mu.Lock()
	defer m.v.mu.Unlock()
	return append([]StartSidebarItem(nil), m.v.sidebar...)
}

// SetTileGroups заменяет плитки. Список копируется; порядок и размеры — как
// передал потребитель.
func (m *StartMenu) SetTileGroups(groups []TileGroup) {
	v := m.v
	v.mu.Lock()
	v.groups = cloneGroups(groups)
	v.tilesRev++
	v.mu.Unlock()
	m.Invalidate()
}

// TileGroups возвращает копию плиток в текущем порядке.
func (m *StartMenu) TileGroups() []TileGroup {
	m.v.mu.Lock()
	defer m.v.mu.Unlock()
	return cloneGroups(m.v.groups)
}

func cloneGroups(src []TileGroup) []TileGroup {
	out := make([]TileGroup, len(src))
	for i, g := range src {
		out[i] = g
		out[i].Tiles = append([]Tile(nil), g.Tiles...)
	}
	return out
}

// SetTileContent меняет содержимое одной плитки и перерисовывает только её.
// Возвращает false, если плитки с таким id нет. Раскладка не пересчитывается:
// размер плитки остаётся прежним.
func (m *StartMenu) SetTileContent(id TileID, c TileContent) bool {
	v := m.v
	v.mu.Lock()
	gi, ti := -1, -1
	for i, g := range v.groups {
		for j, t := range g.Tiles {
			if t.ID == id {
				gi, ti = i, j
			}
		}
	}
	if gi < 0 {
		v.mu.Unlock()
		return false
	}
	// Копирование при записи: Draw читает прежние массивы без замка.
	groups := append([]TileGroup(nil), v.groups...)
	tiles := append([]Tile(nil), groups[gi].Tiles...)
	tiles[ti].Content = c
	groups[gi].Tiles = tiles
	v.groups = groups
	v.mu.Unlock()

	if r := m.tileRectAbs(id); !r.Empty() && m.IsOpen() {
		widget.InvalidateRect(r)
	}
	return true
}

// ─── Список приложений: сборка строк ─────────────────────────────────────────

func (m *StartMenu) startSource() StartMenuSource {
	m.v.mu.Lock()
	s := m.v.source
	m.v.mu.Unlock()
	if s != nil {
		return s
	}
	return NewCatalogSource(m.cat)
}

// listRows возвращает строки списка и полную высоту содержимого; пересобирает
// их, если вход изменился. Модель потребителя читается без замка меню.
func (m *StartMenu) listRows() ([]listRow, int) {
	v := m.v
	key := rowsKey{rowH: m.metricInt(KeyStartMenuRowHeight), letterH: m.metricInt(KeyStartLetterHeight)}
	v.mu.Lock()
	key.searching = v.query != ""
	if v.rowsC != nil && v.rowsRev == v.rev && v.rowsK == key {
		rows, h := v.rowsC, v.contentH
		v.mu.Unlock()
		return rows, h
	}
	rev := v.rev
	query, results := v.query, v.results
	folders := make(map[string]bool, len(v.folders))
	for k, x := range v.folders {
		folders[k] = x
	}
	v.mu.Unlock()

	var rows []listRow
	if query != "" {
		rows = buildResultRows(results, key)
	} else {
		rows = buildAppRows(m.startSource(), folders, key)
	}
	h := 0
	if n := len(rows); n > 0 {
		h = rows[n-1].y + rows[n-1].h
	}

	v.mu.Lock()
	v.rowsC, v.contentH, v.rowsRev, v.rowsK = rows, h, rev, key
	if v.rowsC == nil {
		v.rowsC = []listRow{}
	}
	v.mu.Unlock()
	return rows, h
}

func entryRow(kind rowKind, key string, e StartEntry) listRow {
	return listRow{kind: kind, key: key, label: e.Title, sub: e.Subtitle, app: e.ID,
		icon: e.Icon, iconAt: e.IconAt}
}

func buildAppRows(src StartMenuSource, folders map[string]bool, k rowsKey) []listRow {
	var rows []listRow
	add := func(r listRow) {
		switch r.kind {
		case rowHeader, rowLetter:
			r.h = k.letterH
		default:
			r.h = k.rowH
		}
		if n := len(rows); n > 0 {
			r.y = rows[n-1].y + rows[n-1].h
		}
		rows = append(rows, r)
	}
	addEntry := func(prefix string, e StartEntry) {
		if e.IsFolder() {
			fk := e.folderKey()
			row := entryRow(rowFolder, prefix+"f:"+fk, e)
			row.folder = fk
			row.expanded = folders[fk]
			add(row)
			if row.expanded {
				for _, c := range e.Children {
					child := entryRow(rowChild, prefix+"c:"+fk+"/"+string(c.ID), c)
					child.folder = fk
					add(child)
				}
			}
			return
		}
		add(entryRow(rowApp, prefix+"a:"+string(e.ID), e))
	}
	if src == nil {
		return rows
	}
	if recent := src.Recent(); len(recent) > 0 {
		add(listRow{kind: rowHeader, key: "h:recent", labelKey: StrStartRecent})
		for _, e := range recent {
			addEntry("rc:", e)
		}
	}
	for _, g := range src.Groups() {
		add(listRow{kind: rowLetter, key: "l:" + g.Letter, label: g.Letter})
		for _, e := range g.Entries {
			addEntry("", e)
		}
	}
	return rows
}

func buildResultRows(results []SearchResult, k rowsKey) []listRow {
	var rows []listRow
	add := func(r listRow) {
		if r.kind == rowHeader {
			r.h = k.letterH
		} else {
			r.h = k.rowH
		}
		if n := len(rows); n > 0 {
			r.y = rows[n-1].y + rows[n-1].h
		}
		rows = append(rows, r)
	}
	if len(results) == 0 {
		add(listRow{kind: rowEmpty, key: "e:none", labelKey: StrStartNoResults})
		return rows
	}
	cat := "\x00"
	for i, res := range results {
		if res.Category != cat {
			cat = res.Category
			if cat != "" {
				add(listRow{kind: rowHeader, key: "h:" + cat, label: cat})
			} else if i == 0 {
				add(listRow{kind: rowHeader, key: "h:results", labelKey: StrStartResults})
			}
		}
		add(listRow{kind: rowResult, key: "q:" + res.ID, label: res.Title, sub: res.Subtitle,
			icon: res.Icon, iconAt: res.IconAt, result: res})
	}
	return rows
}

// rowIndex ищет строку по ключу (-1 — нет).
func rowIndex(rows []listRow, key string) int {
	for i := range rows {
		if rows[i].key == key {
			return i
		}
	}
	return -1
}

// firstRowAt возвращает индекс первой строки, нижняя кромка которой ниже y
// (содержимое отсортировано по y): начало видимого диапазона.
func firstRowAt(rows []listRow, y int) int {
	return sort.Search(len(rows), func(i int) bool { return rows[i].y+rows[i].h > y })
}

// listViewport — окно прокрутки списка: область, где лежат строки.
func (m *StartMenu) listViewport(g startGeo) image.Rectangle {
	r := g.list
	if m.inlineQueryBar() {
		r.Min.Y += m.metricInt(KeyStartLetterHeight)
	}
	return r
}

// inlineQueryBar — нужна ли в списке своя строка с запросом: запрос есть, а
// строки поиска на панели, куда его вводят, нет.
func (m *StartMenu) inlineQueryBar() bool {
	m.v.mu.Lock()
	defer m.v.mu.Unlock()
	return m.v.query != "" && m.v.box == nil
}

// clampScroll приводит прокрутку к допустимому диапазону.
func clampScroll(scroll, content, view int) int {
	max := content - view
	if max < 0 {
		max = 0
	}
	if scroll > max {
		scroll = max
	}
	if scroll < 0 {
		scroll = 0
	}
	return scroll
}

// rowRectAbs возвращает прямоугольник строки i в абсолютных координатах.
func (m *StartMenu) rowRectAbs(g startGeo, rows []listRow, i, scroll int) image.Rectangle {
	vp := m.listViewport(g)
	r := rows[i]
	return image.Rect(vp.Min.X, vp.Min.Y+r.y-scroll, vp.Max.X, vp.Min.Y+r.y-scroll+r.h)
}

// ─── Плитки: раскладка ───────────────────────────────────────────────────────

// layoutTiles возвращает раскладку плиток для geometry; кэшируется.
func (m *StartMenu) layoutTiles(g startGeo) *tileLayout {
	k := m.tileKeyFor(g)
	v := m.v
	v.mu.Lock()
	// Кэш годен, пока не сменились ни метрики, ни модель: ревизия растёт при
	// SetTileGroups, переносе плитки и смене темы.
	if v.tilesBuilt && v.tilesC != nil && v.tilesK == k && v.tilesCRev == v.tilesRev {
		l := v.tilesC
		v.mu.Unlock()
		return l
	}
	groups := v.groups
	rev := v.tilesRev
	v.mu.Unlock()

	l := computeTileLayout(groups, k)

	v.mu.Lock()
	if v.tilesRev == rev {
		v.tilesC, v.tilesK, v.tilesBuilt, v.tilesCRev = l, k, true, rev
	}
	v.mu.Unlock()
	return l
}

// computeTileLayout считает раскладку групп: заголовок, сетка плиток, промежуток.
func computeTileLayout(groups []TileGroup, k tileKey) *tileLayout {
	l := &tileLayout{cols: k.cols}
	if k.cols < 1 || k.unit < 1 {
		return l
	}
	step := k.unit + k.gap
	y := 0
	for gi, g := range groups {
		sizes := make([]TileSize, len(g.Tiles))
		for i, t := range g.Tiles {
			sizes[i] = t.Size
		}
		places, rows := placeTiles(sizes, k.cols)
		gg := groupGeom{
			title:  g.Title,
			header: image.Rect(0, y, k.cols*step-k.gap, y+k.header),
			first:  len(l.tiles),
			count:  len(g.Tiles),
		}
		top := y + k.header + k.headerGap
		for i, t := range g.Tiles {
			p := places[i]
			l.tiles = append(l.tiles, tileGeom{
				rect: image.Rect(p.Col*step, top+p.Row*step,
					p.Col*step+p.Cols*step-k.gap, top+p.Row*step+p.Rows*step-k.gap),
				group: gi, index: i, id: t.ID,
			})
		}
		bottom := top
		if rows > 0 {
			bottom = top + rows*step - k.gap
		}
		gg.bottom = bottom
		l.groups = append(l.groups, gg)
		y = bottom + k.groupGap
	}
	l.height = y - k.groupGap
	if l.height < 0 {
		l.height = 0
	}
	return l
}

// tileRectAbs возвращает прямоугольник плитки id на экране (пустой, если её
// нет в раскладке).
func (m *StartMenu) tileRectAbs(id TileID) image.Rectangle {
	if !m.IsOpen() || !m.tiled() {
		return image.Rectangle{}
	}
	g := m.startGeometry(m.contentRect())
	if g.cols == 0 {
		return image.Rectangle{}
	}
	l := m.layoutTiles(g)
	m.v.mu.Lock()
	scroll := m.v.tileScroll
	m.v.mu.Unlock()
	for _, t := range l.tiles {
		if t.id == id {
			return t.rect.Add(image.Pt(g.tinner.Min.X, g.tinner.Min.Y-scroll)).Intersect(g.tiles)
		}
	}
	return image.Rectangle{}
}

// tilesViewHeight — высота окна прокрутки плиток.
func tilesViewHeight(g startGeo) int { return g.tinner.Dy() }

// ─── Тонкая полоса прокрутки с автоскрытием ──────────────────────────────────

// Паузы тонкой полосы; переменные, чтобы тест мог их сократить.
var (
	thinBarHold = 1200 * time.Millisecond
	thinBarFade = 200 * time.Millisecond
)

// fadeBar — прозрачность тонкой полосы. Появляется по Poke (движение мыши над
// областью, прокрутка), через паузу плавно гаснет. Один таймер на серию
// событий: повторный Poke лишь сдвигает отсчёт, а не заводит новый.
type fadeBar struct {
	mu    sync.Mutex
	alpha float64
	last  time.Time
	timer *time.Timer
	live  bool
	anim  *widget.Animation
	// pinned — полоса удерживается на виду (бегунок тянут или над дорожкой
	// курсор): затухание не стартует, пока флаг не снят.
	pinned bool
}

// Alpha — прозрачность для кадра.
func (f *fadeBar) Alpha() float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.alpha
}

// poke делает полосу видимой и отсчитывает паузу заново. inval перерисовывает
// только саму полосу.
func (f *fadeBar) poke(inval func()) {
	f.mu.Lock()
	f.last = time.Now()
	if f.pinned {
		// Удерживается: таймер затухания не нужен, а на виду полоса уже.
		f.mu.Unlock()
		return
	}
	shown := f.alpha == 1 && !f.live
	f.alpha = 1
	f.live = false
	prev := f.anim
	f.anim = nil
	if f.timer == nil {
		f.timer = time.AfterFunc(thinBarHold, func() { f.expire(inval) })
	}
	f.mu.Unlock()
	if prev != nil {
		prev.Stop()
	}
	// Полоса уже на виду — перерисовывать нечего: каждое движение мыши над
	// списком не должно будить кадр.
	if !shown {
		inval()
	}
}

// expire зовётся по таймеру: если пауза не истекла (был свежий poke), заводит
// таймер на остаток, иначе запускает затухание.
func (f *fadeBar) expire(inval func()) {
	f.mu.Lock()
	if f.pinned {
		f.timer = nil
		f.mu.Unlock()
		return
	}
	if rem := thinBarHold - time.Since(f.last); rem > 0 {
		f.timer.Reset(rem)
		f.mu.Unlock()
		return
	}
	f.timer = nil
	f.live = true
	f.mu.Unlock()
	a := widget.AnimateOwned(f, "fade", thinBarFade, nil, func(t float64) {
		f.mu.Lock()
		if !f.live {
			f.mu.Unlock()
			return
		}
		f.alpha = 1 - t
		if t >= 1 {
			f.alpha, f.live = 0, false
		}
		f.mu.Unlock()
		inval()
	})
	f.mu.Lock()
	f.anim = a
	f.mu.Unlock()
}

// pin удерживает полосу на виду, пока бегунок тянут или над дорожкой курсор;
// снятие удержания отсчитывает паузу затухания заново. inval перерисовывает
// только саму полосу и зовётся лишь тогда, когда она действительно появилась.
func (f *fadeBar) pin(on bool, inval func()) {
	f.mu.Lock()
	if f.pinned == on {
		f.mu.Unlock()
		return
	}
	f.pinned = on
	if !on {
		f.mu.Unlock()
		f.poke(inval)
		return
	}
	shown := f.alpha == 1 && !f.live
	f.alpha, f.live = 1, false
	if f.timer != nil {
		f.timer.Stop()
		f.timer = nil
	}
	prev := f.anim
	f.anim = nil
	f.mu.Unlock()
	if prev != nil {
		prev.Stop()
	}
	if !shown {
		inval()
	}
}

// hide мгновенно убирает полосу (меню закрыто).
func (f *fadeBar) hide() {
	f.mu.Lock()
	f.pinned = false
	f.alpha, f.live = 0, false
	if f.timer != nil {
		f.timer.Stop()
		f.timer = nil
	}
	prev := f.anim
	f.anim = nil
	f.mu.Unlock()
	if prev != nil {
		prev.Stop()
	}
}
