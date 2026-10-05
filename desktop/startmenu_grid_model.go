// startmenu_grid_model.go — данные и публичные настройки меню «Пуск» Windows 11:
// закреплённые, «Рекомендуем», пользователь, вид.
//
// Меню не знает, откуда берутся приложения и рекомендации: всё приходит от
// потребителя типами и интерфейсом этого файла. Список «Все приложения» и поиск
// берут данные у тех же StartMenuSource и SearchProvider, что и меню Windows 10.
package desktop

import (
	"image"
	"sync"
	"time"
)

// StartView — какой вид показывает меню «Пуск» Windows 11.
type StartView int

const (
	// StartViewMain — главный: поиск, «Закреплено», «Рекомендуем», нижняя полоса.
	StartViewMain StartView = iota
	// StartViewAllApps — «Все приложения»: алфавитный список с буквами и папками.
	StartViewAllApps
	// StartViewRecommended — «Все рекомендации» (по «Дополнительно»).
	StartViewRecommended
)

// StartPinned — закреплённое приложение в сетке меню. Порядок задаёт потребитель
// (SetPinned) или каталог (AppCatalog.Pinned), а после перетаскивания — сам
// пользователь: новый порядок сообщает OnPinnedChanged.
type StartPinned struct {
	// ID — идентификатор закрепления; уникален в пределах меню. У закрепления
	// приложения совпадает с AppID.
	ID string
	// App — приложение, которое запускает ячейка (AppCatalog.Launch). Пустое —
	// ячейка запускается обработчиком StartMenu.OnPinnedLaunch.
	App   AppID
	Title string
	Icon  image.Image
	// IconAt — значок по размеру в физических пикселях (см. AppInfo.IconAt).
	IconAt func(size int) image.Image
}

// StartRecommendedItem — строка раздела «Рекомендуем»: недавний файл, недавно
// установленное приложение, рекомендация источника.
type StartRecommendedItem struct {
	ID string
	// App — приложение, которое запускает строка; пустое — обработчик
	// StartMenu.OnRecommendedActivate.
	App   AppID
	Title string
	// Subtitle — вторая строка: время («17 мин назад»), «Недавно добавлено».
	Subtitle string
	Icon     image.Image
	IconAt   func(size int) image.Image
}

// StartRecommendedSource — источник раздела «Рекомендуем». Subscribe зовёт
// замыкание из горутины потребителя при смене содержимого. Разделу отдаются
// первые строки (две колонки на три ряда), остальные видны по «Дополнительно».
type StartRecommendedSource interface {
	Recommended() []StartRecommendedItem
	Subscribe(func()) func()
}

// StartUser — пользователь в нижней полосе меню.
type StartUser struct {
	Name string
	// Avatar, AvatarAt — картинка аватара; без неё рисуется серый круг со
	// значком пользователя.
	Avatar   image.Image
	AvatarAt func(size int) image.Image
}

// startGrid — состояние вида Windows 11, которого нет у общего startView: вид,
// страница закреплённых, модель, перетаскивание. Общее с видом Windows 10 —
// запрос, результаты, поставщик, всплывающее меню, подписки, наведение и выбор —
// лежит в startView.
type startGrid struct {
	mu sync.Mutex

	view   StartView
	page   int
	caret  int
	kind   startKind // какой вид рисовался до последней смены темы
	pinned []StartPinned
	// pinnedSet — закреплённые заданы потребителем; иначе берутся из каталога.
	pinnedSet bool
	recSrc    StartRecommendedSource
	recOff    bool // раздел «Рекомендуем» выключен политикой потребителя
	user      StartUser
	lastWheel time.Time

	drag pinDrag
}

// startKind — какой из видов меню просит тема.
type startKind int

const (
	startKindFlat startKind = iota
	startKindTiles
	startKindGrid
)

// pinDrag — перетаскивание закреплённого: нажатие на ячейке, порог, перенос.
type pinDrag struct {
	pending bool
	active  bool
	id      string
	start   image.Point
	pos     image.Point
	// to — индекс вставки в порядке без переносимого.
	to int
}

// ─── Публичные настройки ─────────────────────────────────────────────────────

// SetPinned задаёт закреплённые приложения сеткой меню. nil возвращает
// закреплённые каталога (AppCatalog.Pinned). Список копируется.
func (m *StartMenu) SetPinned(items []StartPinned) {
	g := m.g
	g.mu.Lock()
	g.pinned = append([]StartPinned(nil), items...)
	g.pinnedSet = items != nil
	g.mu.Unlock()
	m.relayoutGrid()
}

// Pinned возвращает закреплённые в текущем порядке (копия): заданные
// потребителем, а без них — из каталога.
func (m *StartMenu) Pinned() []StartPinned { return m.pinnedList() }

// SetRecommended задаёт источник раздела «Рекомендуем». nil возвращает источник
// по умолчанию — раздел «Недавно добавленные» StartMenuSource (Recent).
func (m *StartMenu) SetRecommended(src StartRecommendedSource) {
	g := m.g
	g.mu.Lock()
	g.recSrc = src
	g.mu.Unlock()
	if m.IsOpen() {
		m.resubscribe()
	}
	m.relayoutGrid()
}

// SetRecommendedEnabled включает и выключает раздел «Рекомендуем» (политика
// потребителя). Выключенный раздел исчезает, а закреплённые занимают его место.
func (m *StartMenu) SetRecommendedEnabled(on bool) {
	g := m.g
	g.mu.Lock()
	changed := g.recOff == on
	g.recOff = !on
	g.mu.Unlock()
	if changed {
		m.relayoutGrid()
	}
}

// RecommendedEnabled сообщает, показывается ли раздел «Рекомендуем».
func (m *StartMenu) RecommendedEnabled() bool {
	m.g.mu.Lock()
	defer m.g.mu.Unlock()
	return !m.g.recOff
}

// SetUser задаёт пользователя нижней полосы.
func (m *StartMenu) SetUser(u StartUser) {
	g := m.g
	g.mu.Lock()
	g.user = u
	g.mu.Unlock()
	m.Invalidate()
}

// User возвращает пользователя нижней полосы.
func (m *StartMenu) User() StartUser {
	m.g.mu.Lock()
	defer m.g.mu.Unlock()
	return m.g.user
}

// View возвращает текущий вид меню Windows 11.
func (m *StartMenu) View() StartView {
	m.g.mu.Lock()
	defer m.g.mu.Unlock()
	return m.g.view
}

// SetView переключает вид («Все приложения», «Все рекомендации», главный).
// Меню при открытии всегда начинает с главного.
func (m *StartMenu) SetView(v StartView) {
	g := m.g
	g.mu.Lock()
	changed := g.view != v
	g.view = v
	g.mu.Unlock()
	if !changed {
		return
	}
	vw := m.v
	vw.mu.Lock()
	vw.rev++
	vw.listScroll = 0
	vw.grid, vw.gridFrom = false, ""
	vw.hover, vw.press = "", ""
	vw.sel = map[startArea]string{}
	if v == StartViewMain {
		vw.area = areaSearch
	} else {
		vw.area = areaList
	}
	vw.mu.Unlock()
	vw.gridFade.Set(0)
	m.relayoutGrid()
}

// PageCount возвращает число страниц закреплённых главного вида (1, если меню
// закрыто или закреплённые помещаются на одной).
func (m *StartMenu) PageCount() int { return m.pageCount() }

// Page возвращает номер показанной страницы закреплённых (с нуля).
func (m *StartMenu) Page() int { return m.pageNow() }

// SetPage переходит на страницу закреплённых (в пределах PageCount).
func (m *StartMenu) SetPage(p int) { m.setPage(p) }

// AsGrid сообщает, что активная тема просит меню Windows 11: сетка закреплённых
// с поиском внутри меню (строка поиска на панели задач для него не нужна).
func (m *StartMenu) AsGrid() bool { return m.grid() }

// pinnedList возвращает закреплённые для показа: заданные потребителем или
// каталога. Приложение, пропавшее из каталога, не показывается.
func (m *StartMenu) pinnedList() []StartPinned {
	g := m.g
	g.mu.Lock()
	set, list := g.pinnedSet, g.pinned
	g.mu.Unlock()
	if set {
		return append([]StartPinned(nil), list...)
	}
	if m.cat == nil {
		return nil
	}
	byID := map[AppID]AppInfo{}
	for _, a := range m.cat.Apps() {
		byID[a.ID] = a
	}
	var out []StartPinned
	for _, id := range m.cat.Pinned() {
		if a, ok := byID[id]; ok {
			out = append(out, StartPinned{ID: string(a.ID), App: a.ID, Title: a.Title, Icon: a.Icon, IconAt: a.IconAt})
		}
	}
	return out
}

// recommendedList возвращает строки «Рекомендуем»: источник потребителя или
// «Недавно добавленные» StartMenuSource. Пусто и при выключенном разделе.
func (m *StartMenu) recommendedList() []StartRecommendedItem {
	g := m.g
	g.mu.Lock()
	src, off := g.recSrc, g.recOff
	g.mu.Unlock()
	if off {
		return nil
	}
	if src != nil {
		return src.Recommended()
	}
	var out []StartRecommendedItem
	for _, e := range m.startSource().Recent() {
		if e.IsFolder() {
			continue
		}
		out = append(out, StartRecommendedItem{
			ID: string(e.ID), App: e.ID, Title: e.Title, Subtitle: e.Subtitle,
			Icon: e.Icon, IconAt: e.IconAt,
		})
	}
	return out
}

// relayoutGrid перерисовывает всё меню после смены того, от чего зависит
// раскладка (число закреплённых, раздел «Рекомендуем», вид).
func (m *StartMenu) relayoutGrid() {
	m.v.mu.Lock()
	m.v.rev++ // строки вида «Все рекомендации» строятся из модели
	m.v.mu.Unlock()
	n := m.pageCount() // до замка: раскладка читает модель под ним же
	g := m.g
	g.mu.Lock()
	if g.page >= n && n > 0 {
		g.page = n - 1
	}
	g.mu.Unlock()
	if m.IsOpen() {
		m.Invalidate()
	}
}

// ─── Цели действий ───────────────────────────────────────────────────────────

// pinnedByID ищет закреплённое по идентификатору.
func (m *StartMenu) pinnedByID(id string) (StartPinned, bool) {
	for _, p := range m.pinnedList() {
		if p.ID == id {
			return p, true
		}
	}
	return StartPinned{}, false
}

// recommendedByID ищет строку «Рекомендуем» по идентификатору.
func (m *StartMenu) recommendedByID(id string) (StartRecommendedItem, bool) {
	for _, r := range m.recommendedList() {
		if r.ID == id {
			return r, true
		}
	}
	return StartRecommendedItem{}, false
}

// ─── Запуск ──────────────────────────────────────────────────────────────────

// launchPinned запускает закреплённое и закрывает меню.
func (m *StartMenu) launchPinned(p StartPinned) {
	switch {
	case p.App != "":
		m.launch(p.App)
	default:
		if m.OnPinnedLaunch != nil {
			m.OnPinnedLaunch(p.ID)
		}
		m.Close()
	}
}

// launchRecommended открывает строку «Рекомендуем» и закрывает меню.
func (m *StartMenu) launchRecommended(r StartRecommendedItem) {
	switch {
	case r.App != "":
		m.launch(r.App)
	default:
		if m.OnRecommendedActivate != nil {
			m.OnRecommendedActivate(r.ID)
		}
		m.Close()
	}
}

// ─── Тестовые источники ──────────────────────────────────────────────────────

// FakeStartRecommended — источник «Рекомендуем» для тестов и демонстрации.
// Рассылает уведомления подписчикам при замене данных.
type FakeStartRecommended struct {
	mu      sync.Mutex
	items   []StartRecommendedItem
	subs    map[int]func()
	nextSub int
}

var _ StartRecommendedSource = (*FakeStartRecommended)(nil)

// NewFakeStartRecommended создаёт источник с заданными строками.
func NewFakeStartRecommended(items ...StartRecommendedItem) *FakeStartRecommended {
	return &FakeStartRecommended{items: items, subs: map[int]func(){}}
}

// Recommended возвращает строки.
func (f *FakeStartRecommended) Recommended() []StartRecommendedItem {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]StartRecommendedItem(nil), f.items...)
}

// Set заменяет строки и уведомляет подписчиков.
func (f *FakeStartRecommended) Set(items ...StartRecommendedItem) {
	f.mu.Lock()
	f.items = append([]StartRecommendedItem(nil), items...)
	list := make([]func(), 0, len(f.subs))
	for _, fn := range f.subs {
		list = append(list, fn)
	}
	f.mu.Unlock()
	for _, fn := range list {
		fn()
	}
}

// Subscribe подписывает на смену строк.
func (f *FakeStartRecommended) Subscribe(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	f.mu.Lock()
	f.nextSub++
	id := f.nextSub
	f.subs[id] = fn
	f.mu.Unlock()
	return func() {
		f.mu.Lock()
		delete(f.subs, id)
		f.mu.Unlock()
	}
}
