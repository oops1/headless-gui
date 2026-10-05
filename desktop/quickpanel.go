// quickpanel.go — быстрые настройки Windows 11 24H2 как вариант
// desktop.QuickSettings: состояние, подписки, жизненный цикл и публичные
// методы потребителя.
//
// Компонент один. Вариант выбирает презентер, который назначает профиль темы
// (theme.Profile.Presenters["quicksettings"]); имени темы здесь нет. Пока у
// панели нет модели плиток (SetQuickActions) или тема презентера не называет,
// рисуется прежняя панель с тремя плитками и ползунком громкости (см.
// quicksettings.go). Общее для обоих видов — открытие, закрытие, выезд,
// клик мимо и Esc — делает Flyout.
//
// Что в панели решает потребитель, а что — движок:
//
//   - состав и состояние плиток — модель QuickActionModel (ID, Title, Icon,
//     On, Disabled, Unavailable, Detail, HasDetails); нажатие плитки —
//     model.Toggle;
//   - «›» у плитки и у громкости открывает вложенную страницу: содержимое
//     отдаёт QuickSettings.Details, а панель рисует заголовок со стрелкой
//     «назад» и сдвигает страницы (длительность — токен quicksettings.page,
//     «меньше движения» делает переход мгновенным);
//   - громкость — SystemStatus и колбэки OnVolumeChange/OnToggleMute, яркость
//     — SetBrightness и OnBrightnessChange (нет данных — ползунка нет);
//   - режим правки — карандаш внизу: плитки переставляются перетаскиванием,
//     новый порядок уходит в OnReorder и в модель (QuickActionReorderer);
//   - нижняя строка: батарея из SystemStatus, «Изменить» (OnEdit) и
//     «Параметры» (OnSettings).
package desktop

import (
	"image"
	"sync"

	"github.com/oops1/headless-gui/v3/internal/focusreq"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

func init() {
	RegisterPresenter(theme.QuickSettingsPresenter, qsPresenter{})
}

// QuickVolumeID — зарезервированный идентификатор громкости: по нему панель
// просит у QuickSettings.Details вложенную страницу «›» ползунка громкости
// (выбор устройства вывода). Плитки с таким ID потребитель заводить не должен.
const QuickVolumeID QuickActionID = "desktop.quick.volume"

// QuickDetails — вложенная страница быстрых настроек: то, что открывает «›».
type QuickDetails struct {
	// Title — заголовок страницы. Пусто — название плитки (для громкости —
	// «Громкость» на текущем языке).
	Title string
	// Content — содержимое. Панель задаёт ему границы (область под
	// заголовком), рисует его и передаёт ему мышь и клавиши: события доходят
	// до самого вложенного виджета под курсором, клавиши — корневому
	// виджету, если он реализует widget.KeyHandler. nil — страница с одним
	// заголовком.
	Content widget.Widget
	// OnClose — страница закрыта (назад, закрытие панели). Зовётся после
	// того, как страница ушла с экрана.
	OnClose func()
}

// qsDragKind — что тянут мышью.
type qsDragKind uint8

const (
	qdNone   qsDragKind = iota
	qdTile              // плитка в режиме правки
	qdSlider            // ползунок громкости или яркости
	qdThumb             // бегунок прокрутки сетки
)

// qsDrag — состояние перетаскивания.
type qsDrag struct {
	kind  qsDragKind
	idx   int    // плитка списка (qdTile)
	zone  qsZone // ползунок (qdSlider)
	start image.Point
	cur   image.Point
	moved bool // сдвинулась дальше порога: это перетаскивание, не щелчок
	slot  int  // место, куда ляжет плитка
	grab  image.Point
}

// qsDragThreshold — на сколько точек надо сдвинуть мышь, чтобы нажатие на
// плитку стало перетаскиванием, а не щелчком.
const qsDragThreshold = 5

// qsPanel — состояние варианта Windows 11. Принадлежит QuickSettings; все
// поля под mu, а колбэки потребителя и Invalidate зовутся без замка.
type qsPanel struct {
	q  *QuickSettings
	mu sync.Mutex

	model      QuickActionModel
	unsubModel func()
	list       []QuickAction   // в порядке показа; срез не правится на месте
	order      []QuickActionID // порядок, выбранный в режиме правки (nil — как у модели)

	scroll int

	bright    float64
	hasBright bool

	vol        VolState // последнее показание звука
	pow        PowerState
	volOver    float64 // уровень, который показываем, пока потребитель не ответил
	hasVolOver bool
	seen       bool

	editing bool

	page         *Tween // 0 — главная страница, 1 — вложенная
	details      *QuickDetails
	detailsID    QuickActionID
	detailsEcho  bool // страница уходит: после перехода её надо снять
	hover, press qsZone
	focus        qsZone
	drag         qsDrag
	capture      widget.CaptureManager
	// contentHover — виджет вложенной страницы, над которым сейчас курсор.
	contentHover widget.Widget

	mo    motion
	memos map[QuickActionID]*glyphMemo
	// bound — настоящая рамка панели на время отрисовки (только поток кадра):
	// за неё не должен выходить ни один скруглённый слой, пока страницы едут.
	bound image.Rectangle
}

func newQSPanel(q *QuickSettings) *qsPanel {
	pn := &qsPanel{q: q, memos: map[QuickActionID]*glyphMemo{}}
	pn.page = NewTween(q.Theme(), theme.KeyQuickPage, 0, pn.onPage)
	return pn
}

// ─── Включённость варианта ───────────────────────────────────────────────────

// rich сообщает, рисует ли панель вариант Windows 11: тема назвала презентер и
// потребитель дал модель плиток.
func (q *QuickSettings) rich() bool {
	if q.pn == nil || PresenterFor(q.Theme(), ComponentQuickSettings) == nil {
		return false
	}
	q.pn.mu.Lock()
	defer q.pn.mu.Unlock()
	return q.pn.model != nil
}

func (q *QuickSettings) richSize() image.Point { return q.pn.size() }

func (q *QuickSettings) richDraw(ctx widget.DrawContext) { q.pn.draw(ctx, q.rect()) }

// size — желаемый размер панели: ширина из метрики, высота по плиткам.
func (pn *qsPanel) size() image.Point {
	m := readQSMetrics(pn.q.Theme())
	pn.mu.Lock()
	n, hasBright := len(pn.list), pn.hasBright
	pn.mu.Unlock()
	return image.Pt(m.width, qsHeight(m, n, pn.q.st != nil, hasBright))
}

// place — Flyout.Place: панель стоит над значком, выровненная по его центру и
// не ближе поля margin к краю экрана и к панели задач — как у Windows 11,
// где быстрые настройки парят над панелью, а не прилипают к ней.
func (pn *qsPanel) place(anchor, screen image.Rectangle, edge Edge, size image.Point) (image.Rectangle, bool) {
	if PresenterFor(pn.q.Theme(), ComponentQuickSettings) == nil || edge.Vertical() || screen.Empty() {
		return image.Rectangle{}, false
	}
	m := readQSMetrics(pn.q.Theme())
	x := anchor.Min.X + (anchor.Dx()-size.X)/2
	if anchor.Empty() {
		x = screen.Max.X - size.X
	}
	if x+size.X > screen.Max.X-m.margin {
		x = screen.Max.X - m.margin - size.X
	}
	if x < screen.Min.X+m.margin {
		x = screen.Min.X + m.margin
	}
	y := anchor.Min.Y - m.margin - size.Y
	if edge == EdgeTop {
		y = anchor.Max.Y + m.margin
	}
	return image.Rect(x, y, x+size.X, y+size.Y), true
}

// ─── Публичные методы ────────────────────────────────────────────────────────

// SetQuickActions задаёт модель плиток (nil — без неё: панель рисует прежний
// вид с тремя плитками). Плитки показывает только вариант Windows 11; у других
// тем они остаются в центре уведомлений.
func (q *QuickSettings) SetQuickActions(m QuickActionModel) {
	pn := q.pn
	pn.relayout(func() {
		pn.mu.Lock()
		old := pn.unsubModel
		pn.unsubModel = nil
		pn.model = m
		pn.order = nil
		pn.list = nil
		if m != nil {
			pn.list = pn.arrange(m.List())
		}
		pn.drag = qsDrag{}
		pn.mu.Unlock()
		if old != nil {
			old()
		}
	})
	if q.IsOpen() {
		pn.attachModel()
	}
}

// QuickActions возвращает модель плиток.
func (q *QuickSettings) QuickActions() QuickActionModel {
	q.pn.mu.Lock()
	defer q.pn.mu.Unlock()
	return q.pn.model
}

// SetBrightness задаёт яркость (0..1) и показывает ползунок яркости. Пока
// потребитель её не дал, ползунка нет: на сервере по удалённому рабочему
// столу яркости нет, и честнее скрыть блок, чем показывать мёртвый.
func (q *QuickSettings) SetBrightness(level float64) {
	if level < 0 {
		level = 0
	}
	if level > 1 {
		level = 1
	}
	pn := q.pn
	pn.mu.Lock()
	had, same := pn.hasBright, pn.bright == level
	pn.mu.Unlock()
	if had && same {
		return
	}
	if !had {
		pn.relayout(func() {
			pn.mu.Lock()
			pn.bright, pn.hasBright = level, true
			pn.mu.Unlock()
		})
		return
	}
	pn.mu.Lock()
	pn.bright = level
	dragging := pn.drag.kind == qdSlider && pn.drag.zone.kind == qzBright
	pn.mu.Unlock()
	if !dragging {
		pn.invalidateZone(qsZone{kind: qzBright})
	}
}

// ClearBrightness прячет ползунок яркости.
func (q *QuickSettings) ClearBrightness() {
	pn := q.pn
	pn.mu.Lock()
	had := pn.hasBright
	pn.mu.Unlock()
	if !had {
		return
	}
	pn.relayout(func() {
		pn.mu.Lock()
		pn.hasBright, pn.bright = false, 0
		if pn.drag.kind == qdSlider {
			pn.drag = qsDrag{}
		}
		pn.mu.Unlock()
	})
}

// Brightness возвращает яркость и признак «ползунок показан».
func (q *QuickSettings) Brightness() (float64, bool) {
	q.pn.mu.Lock()
	defer q.pn.mu.Unlock()
	return q.pn.bright, q.pn.hasBright
}

// SetEditing входит в режим правки или выходит из него. Колбэк OnEdit зовётся
// при каждой смене.
func (q *QuickSettings) SetEditing(on bool) {
	pn := q.pn
	pn.mu.Lock()
	if pn.editing == on {
		pn.mu.Unlock()
		return
	}
	pn.editing = on
	if !on && pn.drag.kind == qdTile {
		pn.drag = qsDrag{}
	}
	pn.focus = qsZone{}
	pn.mu.Unlock()
	q.Invalidate()
	if q.OnEdit != nil {
		q.OnEdit(on)
	}
}

// Editing сообщает, идёт ли режим правки.
func (q *QuickSettings) Editing() bool {
	q.pn.mu.Lock()
	defer q.pn.mu.Unlock()
	return q.pn.editing
}

// OpenDetails открывает вложенную страницу плитки id (QuickVolumeID —
// громкости): содержимое отдаёт Details. false — страницы нет (нет Details,
// он вернул nil или панель рисует прежний вид).
func (q *QuickSettings) OpenDetails(id QuickActionID) bool {
	if !q.rich() || q.Details == nil {
		return false
	}
	d := q.Details(id)
	if d == nil {
		return false
	}
	pn := q.pn
	pn.mu.Lock()
	prev := pn.details
	pn.details, pn.detailsID, pn.detailsEcho = d, id, false
	pn.focus = qsZone{kind: qzBack}
	pn.hover, pn.press = qsZone{}, qsZone{}
	pn.drag = qsDrag{}
	pn.mu.Unlock()
	if prev != nil && prev != d && prev.OnClose != nil {
		prev.OnClose()
	}
	pn.page.To(1)
	q.Invalidate()
	return true
}

// CloseDetails возвращает панель с вложенной страницы на главную.
func (q *QuickSettings) CloseDetails() {
	pn := q.pn
	pn.mu.Lock()
	if pn.details == nil || pn.detailsEcho {
		pn.mu.Unlock()
		return
	}
	pn.detailsEcho = true
	pn.focus = qsZone{}
	pn.mu.Unlock()
	pn.page.To(0)
}

// DetailsOpen возвращает идентификатор открытой вложенной страницы.
func (q *QuickSettings) DetailsOpen() (QuickActionID, bool) {
	q.pn.mu.Lock()
	defer q.pn.mu.Unlock()
	if q.pn.details == nil || q.pn.detailsEcho {
		return "", false
	}
	return q.pn.detailsID, true
}

// onPage — шаг перехода между страницами: перерисовать панель, а когда
// страница ушла совсем — снять её.
func (pn *qsPanel) onPage() {
	pn.q.Invalidate()
	if pn.page.Target() != 0 || pn.page.Animating() {
		return
	}
	pn.releaseDetails()
}

// releaseDetails снимает вложенную страницу и сообщает ей об этом.
func (pn *qsPanel) releaseDetails() {
	pn.mu.Lock()
	d := pn.details
	pn.details, pn.detailsEcho = nil, false
	if pn.focus.kind == qzBack || pn.focus.kind == qzContent {
		pn.focus = qsZone{}
	}
	pn.mu.Unlock()
	if d != nil && d.OnClose != nil {
		d.OnClose()
	}
}

// ─── Список плиток ───────────────────────────────────────────────────────────

// arrange ставит плитки в порядок, выбранный в режиме правки (под mu).
func (pn *qsPanel) arrange(list []QuickAction) []QuickAction {
	out := append([]QuickAction(nil), list...)
	if len(pn.order) == 0 {
		return out
	}
	rank := make(map[QuickActionID]int, len(pn.order))
	for i, id := range pn.order {
		rank[id] = i
	}
	// Плитки, которых в порядке нет (появились позже), идут в конце, как у
	// модели.
	pos := func(a QuickAction) int {
		if r, ok := rank[a.ID]; ok {
			return r
		}
		return len(rank)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && pos(out[j]) < pos(out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// sameShape — набор и порядок плиток прежние: можно перерисовать только
// изменившиеся.
func qsSameShape(a, b []QuickAction) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			return false
		}
	}
	return true
}

// qsTileChanged — у плитки изменилось то, что видно.
func qsTileChanged(a, b QuickAction) bool {
	return a.Title != b.Title || a.Detail != b.Detail || a.On != b.On ||
		a.Disabled != b.Disabled || a.Unavailable != b.Unavailable ||
		a.HasDetails != b.HasDetails || !qsSameIcon(a.Icon, b.Icon)
}

// qsSameIcon сравнивает значки, не падая на несравнимых типах.
func qsSameIcon(a, b image.Image) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()
	return a == b
}

// onModelChanged — изменилась модель плиток (горутина потребителя). Пока набор
// и порядок прежние, перерисовываются только плитки, у которых что-то
// изменилось: включение Bluetooth не должно будить размытие всей панели.
func (pn *qsPanel) onModelChanged() {
	q := pn.q
	if !q.IsOpen() {
		return
	}
	m := pn.currentModel()
	if m == nil {
		return
	}
	raw := m.List()
	was := q.dirtyRect()

	pn.mu.Lock()
	next := pn.arrange(raw)
	old := pn.list
	same := qsSameShape(old, next)
	pn.list = next
	if !same && pn.drag.kind == qdTile {
		pn.drag = qsDrag{}
	}
	pn.mu.Unlock()

	if !same {
		q.invalidateOverlay(was)
		return
	}
	if pn.page.Value() > 0 {
		q.Invalidate()
		return
	}
	l, _ := pn.layout(q.rect())
	for i := range next {
		if !qsTileChanged(old[i], next[i]) {
			continue
		}
		body, label := l.tileRects(i)
		widget.InvalidateRect(body.Union(label).Intersect(l.grid))
	}
}

func (pn *qsPanel) currentModel() QuickActionModel {
	pn.mu.Lock()
	defer pn.mu.Unlock()
	return pn.model
}

// relayout выполняет fn, меняющую то, от чего зависит размер панели, и
// перерисовывает прежнюю и новую области.
func (pn *qsPanel) relayout(fn func()) {
	q := pn.q
	was := q.dirtyRect()
	fn()
	q.invalidateOverlay(was)
}

// invalidateZone заявляет область одной зоны главной страницы.
func (pn *qsPanel) invalidateZone(z qsZone) {
	q := pn.q
	if !q.IsOpen() {
		return
	}
	if pn.page.Value() > 0 {
		q.Invalidate()
		return
	}
	l, _ := pn.layout(q.rect())
	widget.InvalidateRect(l.zoneRect(z))
}

// ─── Подписки и жизненный цикл ───────────────────────────────────────────────

// attachModel подписывается на модель плиток (под открытой панелью).
func (pn *qsPanel) attachModel() {
	pn.mu.Lock()
	m := pn.model
	need := m != nil && pn.unsubModel == nil
	pn.mu.Unlock()
	if !need {
		return
	}
	u := m.Subscribe(pn.onModelChanged)
	pn.mu.Lock()
	if pn.unsubModel == nil {
		pn.unsubModel, u = u, nil
	}
	pn.mu.Unlock()
	if u != nil {
		u()
	}
}

// detachModel снимает подписку: закрытая панель не будит ни модель, ни рендер.
func (pn *qsPanel) detachModel() {
	pn.mu.Lock()
	u := pn.unsubModel
	pn.unsubModel = nil
	pn.mu.Unlock()
	if u != nil {
		u()
	}
}

// onStatus — изменились показания системы (горутина потребителя): перерисовать
// только то, что показывает изменившееся, а не панель целиком.
func (pn *qsPanel) onStatus() {
	q := pn.q
	if q.st == nil || !q.IsOpen() {
		return
	}
	vol, pow := q.st.Volume(), q.st.Power()
	pn.mu.Lock()
	volChanged := !pn.seen || vol != pn.vol
	powChanged := !pn.seen || pow != pn.pow
	pn.vol, pn.pow, pn.seen = vol, pow, true
	if volChanged && !(pn.drag.kind == qdSlider && pn.drag.zone.kind == qzVol) {
		pn.hasVolOver = false
	}
	pn.mu.Unlock()
	if volChanged {
		pn.invalidateZone(qsZone{kind: qzVol})
	}
	if powChanged {
		if pn.page.Value() > 0 {
			return
		}
		l, _ := pn.layout(q.rect())
		widget.InvalidateRect(l.battery)
	}
}

// reset приводит панель к исходному виду перед показом: первый кадр открытия
// уже правильный, без остатков прошлого сеанса.
func (pn *qsPanel) reset() {
	q := pn.q
	pn.mu.Lock()
	pn.scroll = 0
	pn.hover, pn.press, pn.focus = qsZone{}, qsZone{}, qsZone{}
	pn.drag = qsDrag{}
	pn.editing = false
	pn.hasVolOver = false
	pn.order = nil
	if pn.model != nil {
		pn.list = pn.arrange(pn.model.List())
	}
	if q.st != nil {
		pn.vol, pn.pow, pn.seen = q.st.Volume(), q.st.Power(), true
	}
	pn.mu.Unlock()
	pn.page.Set(0)
	pn.releaseDetails()
}

// onOpened — панель показана: подписки и фокус.
func (q *QuickSettings) onOpened() {
	q.attach()
	if q.rich() {
		q.pn.attachModel()
		focusreq.Request(q)
	}
}

// onClosed — панель скрыта: отписаться, снять страницу и режим правки.
func (q *QuickSettings) onClosed() {
	q.detach()
	pn := q.pn
	pn.detachModel()
	pn.mu.Lock()
	wasEditing := pn.editing
	pn.editing = false
	pn.drag = qsDrag{}
	pn.mu.Unlock()
	pn.page.Set(0)
	pn.releaseDetails()
	if wasEditing && q.OnEdit != nil {
		q.OnEdit(false)
	}
	focusreq.Return(q)
}

// commitOrder запоминает порядок плиток после перетаскивания и сообщает о нём
// модели и потребителю.
func (pn *qsPanel) commitOrder(ids []QuickActionID) {
	pn.mu.Lock()
	pn.order = append([]QuickActionID(nil), ids...)
	pn.list = pn.arrange(pn.list)
	m := pn.model
	pn.mu.Unlock()
	if r, ok := m.(QuickActionReorderer); ok {
		r.Reorder(ids)
	}
	if pn.q.OnReorder != nil {
		pn.q.OnReorder(append([]QuickActionID(nil), ids...))
	}
}

// layout считает раскладку панели panel по текущему состоянию. Возвращает и
// список плиток, по которому она посчитана.
func (pn *qsPanel) layout(panel image.Rectangle) (*qsLayout, []QuickAction) {
	m := readQSMetrics(pn.q.Theme())
	pn.mu.Lock()
	list := pn.list
	v := qsViewState{
		n: len(list), scroll: pn.scroll, hasVol: pn.q.st != nil, volChev: pn.q.VolumeDetails,
		hasBright: pn.hasBright, editing: pn.editing,
		hasBat: pn.q.st != nil && pn.seen && !pn.pow.NoBattery,
	}
	if pn.editing {
		v.editLabel = tr(StrQuickDone)
	}
	if pn.drag.kind == qdTile && pn.drag.moved && len(list) > 0 {
		v.pos = qsDisplayOrder(len(list), pn.drag.idx, pn.drag.slot)
	}
	pn.mu.Unlock()
	return qsLayoutFor(m, panel, v), list
}
