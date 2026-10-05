// quicksettings.go — панель быстрых настроек: плитки Wi-Fi/звука/питания и
// ползунок громкости, всплывающие над панелью задач общей основой Flyout
// (см. flyout.go), как в Windows 11.
//
// Панель ничего не решает сама: она читает состояние через SystemStatus и
// сообщает о действиях пользователя колбэками (OnToggleWiFi, OnToggleMute,
// OnVolumeChange) — включать Wi-Fi, приглушать звук и менять громкость
// системы должен тот, кто эту панель создал.
//
// Это прежний вид панели. Вариант Windows 11 24H2 — список плиток из модели
// потребителя, ползунки громкости и яркости, вложенные страницы, режим
// правки — выбирается презентером профиля темы и включается, когда задана
// модель плиток (SetQuickActions); он описан в quickpanel.go.
package desktop

import (
	"image"
	"sync"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ComponentQuickSettings — имя компонента для стилей темы. Части:
// "tile.network"/"tile.volume"/"tile.power" — плитки (состояние Active —
// «включено», Normal — «выключено»); "slider"/"slider.fill" — дорожка и
// заполненная часть ползунка громкости.
const ComponentQuickSettings = "quicksettings"

// Ключи метрик темы, которыми управляется размер панели.
const (
	// KeyQuickSettingsWidth — ширина панели.
	KeyQuickSettingsWidth theme.Key = "quicksettings.width"
	// KeyQuickSettingsTile — сторона квадратной плитки. Ползунок громкости
	// снизу получает такую же высоту — визуально он весит как ряд плиток
	// над ним.
	KeyQuickSettingsTile theme.Key = "quicksettings.tile"
	// KeyQuickSettingsGap — зазор между плитками и между рядом плиток и
	// ползунком.
	KeyQuickSettingsGap theme.Key = "quicksettings.gap"
)

// Пропорции элементов ползунка громкости — не пиксельный размер, а
// соотношение, как networkBars и соседние константы в tray.go: во сколько
// раз тоньше дорожка и во сколько шире бегунок этой самой дорожки.
const (
	quickSettingsTrackDiv = 6 // во сколько раз полоса ползунка выше самой дорожки
	quickSettingsThumbDiv = 3 // во сколько раз бегунок шире дорожки по высоте
)

// Индексы плиток в ряду (слева направо): сеть, звук, питание.
const (
	quickSettingsTileNetwork = iota
	quickSettingsTileVolume
	quickSettingsTilePower
)

// QuickSettings — панель быстрых настроек, оформляемая темой tm и
// показывающая показатели st. Встраивает Flyout — открытие, закрытие,
// подложку, закрытие по Esc и клику мимо целиком делает он.
type QuickSettings struct {
	*Flyout

	st SystemStatus

	// OnToggleWiFi/OnToggleMute — колбэки клика по соответствующей плитке.
	OnToggleWiFi func()
	OnToggleMute func()
	// OnVolumeChange — колбэк перетаскивания ползунка громкости, получает
	// новый уровень в [0,1].
	OnVolumeChange func(float64)

	// Колбэки и источники варианта Windows 11 (quickpanel.go). Прежний вид
	// панели их не читает.
	//
	// Details отдаёт вложенную страницу «›» плитки (QuickVolumeID — громкости);
	// nil — страниц нет. VolumeDetails включает «›» у громкости.
	Details       func(QuickActionID) *QuickDetails
	VolumeDetails bool
	// OnBrightnessChange — пользователь двигает ползунок яркости, новый
	// уровень в [0,1]. Ползунок есть, только если потребитель задал яркость
	// (SetBrightness).
	OnBrightnessChange func(float64)
	// OnReorder — новый порядок плиток после режима правки.
	OnReorder func([]QuickActionID)
	// OnEdit — режим правки включён или выключен (карандаш внизу).
	OnEdit func(editing bool)
	// OnSettings — нажата шестерёнка («Параметры»). Панель к этому моменту уже
	// закрыта: параметры открываются поверх рабочего стола, а не под ней.
	OnSettings func()

	mu       sync.Mutex
	unsub    func()
	dragging bool

	// fade — плавный переход цвета плиток при смене Active (тема: hover).
	fade motion

	pn *qsPanel // вариант Windows 11
	fs FocusState
}

// NewQuickSettings создаёт панель быстрых настроек, оформляемую темой tm и
// показывающую показатели st.
func NewQuickSettings(tm *theme.Manager, st SystemStatus) *QuickSettings {
	q := &QuickSettings{
		Flyout: NewFlyout(tm, ComponentQuickSettings),
		st:     st,
	}
	q.Content = q.drawContent
	q.Size = q.size
	q.Place = q.place
	q.pn = newQSPanel(q)
	// Подписки и фокус открывают панель со стороны (менеджер, группа) так же
	// часто, как её собственный Open, — поэтому крючки Flyout, а не Open.
	q.Flyout.afterOpen = q.onOpened
	q.Flyout.afterClose = q.onClosed
	q.Flyout.beforeOpen = q.pn.reset
	return q
}

// place — Flyout.Place: вариант Windows 11 парит над значком с полем до края
// экрана и до панели задач; прежний вид размещается обычным образом.
func (q *QuickSettings) place(anchor, screen image.Rectangle, edge Edge, size image.Point) (image.Rectangle, bool) {
	if !q.rich() {
		return image.Rectangle{}, false
	}
	return q.pn.place(anchor, screen, edge, size)
}

// attach подписывает панель на SystemStatus. Подписка живёт, пока панель
// открыта — вне экрана перерисовывать её по чужим уведомлениям незачем.
func (q *QuickSettings) attach() {
	q.mu.Lock()
	need := q.unsub == nil && q.st != nil
	q.mu.Unlock()
	if !need {
		return
	}
	u := q.st.Subscribe(q.onStatus)
	q.mu.Lock()
	if q.unsub == nil {
		q.unsub, u = u, nil
	}
	q.mu.Unlock()
	if u != nil {
		u()
	}
}

// detach снимает подписку на SystemStatus.
func (q *QuickSettings) detach() {
	q.mu.Lock()
	unsub := q.unsub
	q.unsub = nil
	q.mu.Unlock()
	if unsub != nil {
		unsub()
	}
}

// onStatus — изменились показания системы (горутина потребителя).
func (q *QuickSettings) onStatus() {
	if q.rich() {
		q.pn.onStatus()
		return
	}
	q.Invalidate()
}

// Open открывает панель; подписка на SystemStatus ставится крючком
// Flyout.afterOpen и живёт, пока панель открыта.
func (q *QuickSettings) Open(anchor image.Rectangle) {
	q.Flyout.Open(anchor)
}

// Close закрывает панель (как обычный Flyout.Close); крючок afterClose
// отписывается от SystemStatus — иначе закрытая, но не отпущенная панель
// продолжала бы держать подписчика и просыпаться на каждое изменение сети,
// звука или питания вечно.
func (q *QuickSettings) Close() {
	q.Flyout.Close()
}

// Bounds расширяет обычные границы виджета до прямоугольника открытой
// панели — так движок находит её при поиске оверлея под курсором (тот же
// приём, что widget.Dropdown.Bounds и widget.MenuBar.Bounds). Пока идёт
// перетаскивание ползунка, границы дополнительно растягиваются на весь
// экран: движок доставляет отпускание кнопки мыши виджету-поглотителю
// press'а, только если курсор к этому моменту всё ещё внутри его Bounds
// (engine/events.go, восстановление pressConsumer) — а отпустить ползунок
// пользователь может где угодно, уже покинув панель.
func (q *QuickSettings) Bounds() image.Rectangle {
	b := q.Flyout.Bounds()
	if q.IsOpen() && q.isDragging() && !q.Screen.Empty() {
		b = b.Union(q.Screen)
	}
	return b
}

// Draw — у панели нет собственной раскладки в потоке виджетов: содержимое
// целиком рисуется оверлеем (Flyout.DrawOverlay, унаследованный от
// встроенной панели). Метод обязателен по интерфейсу widget.Widget — тем же
// путём идёт widget.PopupMenu.Draw.
func (q *QuickSettings) Draw(widget.DrawContext) {}

// ─── Мышь ────────────────────────────────────────────────────────────────────

// OnMouseButton разбирает клики по плиткам и ползунку. Всё остальное (клик
// мимо панели, клик по пустому месту внутри неё) отдаётся встроенному
// Flyout — он либо ничего не делает, либо закрывает панель.
func (q *QuickSettings) OnMouseButton(e widget.MouseEvent) bool {
	if q.IsOpen() && q.rich() {
		return q.richMouseButton(e)
	}
	if !q.IsOpen() || e.Button != widget.MouseLeft {
		return q.Flyout.OnMouseButton(e)
	}
	pt := image.Pt(e.X, e.Y)
	if e.Pressed {
		switch {
		case pt.In(q.tileRectAt(quickSettingsTileNetwork)):
			if q.OnToggleWiFi != nil {
				q.OnToggleWiFi()
			}
			return true
		case pt.In(q.tileRectAt(quickSettingsTileVolume)):
			if q.OnToggleMute != nil {
				q.OnToggleMute()
			}
			return true
		case pt.In(q.tileRectAt(quickSettingsTilePower)):
			// У плитки питания нет переключателя — это индикатор. Клик по
			// ней всё равно поглощаем, чтобы не закрыть панель кликом
			// «мимо» по её же собственной плитке.
			return true
		case pt.In(q.sliderRect()):
			q.beginDrag(e.X)
			return true
		}
		return q.Flyout.OnMouseButton(e)
	}
	if q.endDrag() {
		return true
	}
	return q.Flyout.OnMouseButton(e)
}

// OnMouseMove продолжает перетаскивание ползунка, если оно идёт.
func (q *QuickSettings) OnMouseMove(x, y int) {
	if q.IsOpen() && q.rich() {
		q.richMouseMove(x, y)
		return
	}
	if q.isDragging() {
		q.applyDragX(x)
	}
}

// beginDrag начинает перетаскивание и сразу переносит ползунок под курсор —
// как и обычные слайдеры (см. widget/slider.go), клик по дорожке в стороне
// от бегунка сразу выставляет громкость, а не только начинает drag.
func (q *QuickSettings) beginDrag(x int) {
	q.mu.Lock()
	q.dragging = true
	q.mu.Unlock()
	q.applyDragX(x)
}

// applyDragX пересчитывает уровень громкости по абсолютной координате x и
// сообщает его колбэком. Сам уровень нигде не хранится — источник истины
// снаружи (SystemStatus), панель только просит его изменить.
func (q *QuickSettings) applyDragX(x int) {
	tr := q.trackRect()
	if tr.Dx() <= 0 {
		return
	}
	ratio := float64(x-tr.Min.X) / float64(tr.Dx())
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	q.Invalidate()
	if q.OnVolumeChange != nil {
		q.OnVolumeChange(ratio)
	}
}

// endDrag завершает перетаскивание, если оно шло. Возвращает true, если
// событие тем самым поглощено.
func (q *QuickSettings) endDrag() bool {
	q.mu.Lock()
	was := q.dragging
	q.dragging = false
	q.mu.Unlock()
	return was
}

func (q *QuickSettings) isDragging() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.dragging
}

// ─── Раскладка ──────────────────────────────────────────────────────────────

// contentRect — прямоугольник, который получит Content при отрисовке:
// панель минус отступ PadX (Flyout.DrawOverlay инсетит содержимое ровно на
// него — image.Rectangle.Inset одним числом ужимает сразу оба измерения).
// Раскладка плиток и ползунка использует его же, чтобы попадание мыши
// совпадало с тем, что фактически нарисовано.
func (q *QuickSettings) contentRect() image.Rectangle {
	if !q.IsOpen() {
		return image.Rectangle{}
	}
	r := q.rect()
	if r.Empty() {
		return r
	}
	return r.Inset(int(q.style(theme.StateNormal).PadX))
}

// tilesRow — прямоугольник верхнего ряда плиток.
func (q *QuickSettings) tilesRow() image.Rectangle {
	r := q.contentRect()
	tile := q.metric(KeyQuickSettingsTile)
	if r.Empty() || tile <= 0 {
		return image.Rectangle{}
	}
	return image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+tile)
}

// tileRectAt — прямоугольник плитки с индексом index (0 — сеть, 1 — звук,
// 2 — питание; см. quickSettingsTile* выше).
func (q *QuickSettings) tileRectAt(index int) image.Rectangle {
	row := q.tilesRow()
	if row.Empty() {
		return row
	}
	tile := q.metric(KeyQuickSettingsTile)
	gap := q.metric(KeyQuickSettingsGap)
	x := row.Min.X + index*(tile+gap)
	return image.Rect(x, row.Min.Y, x+tile, row.Max.Y)
}

// sliderRect — прямоугольник полосы ползунка громкости под рядом плиток.
func (q *QuickSettings) sliderRect() image.Rectangle {
	row := q.tilesRow()
	if row.Empty() {
		return row
	}
	tile := q.metric(KeyQuickSettingsTile)
	gap := q.metric(KeyQuickSettingsGap)
	top := row.Max.Y + gap
	return image.Rect(row.Min.X, top, row.Max.X, top+tile)
}

// trackRect — тонкая дорожка внутри полосы ползунка (см.
// quickSettingsTrackDiv).
func (q *QuickSettings) trackRect() image.Rectangle {
	r := q.sliderRect()
	if r.Empty() {
		return r
	}
	h := r.Dy() / quickSettingsTrackDiv
	if h < 1 {
		h = 1
	}
	top := r.Min.Y + (r.Dy()-h)/2
	return image.Rect(r.Min.X, top, r.Max.X, top+h)
}

// size — Flyout.Size: ширина из темы, высота — ряд плиток плюс зазор плюс
// полоса ползунка (той же высоты, что и плитка) плюс внутренние отступы.
func (q *QuickSettings) size() image.Point {
	if q.rich() {
		return q.pn.size()
	}
	width := q.metric(KeyQuickSettingsWidth)
	tile := q.metric(KeyQuickSettingsTile)
	gap := q.metric(KeyQuickSettingsGap)
	if width <= 0 || tile <= 0 {
		return image.Point{} // без метрик темы рисовать нечего — Flyout не откроется
	}
	pad := int(q.style(theme.StateNormal).PadX)
	height := tile + gap + tile + 2*pad
	return image.Pt(width, height)
}

// ─── Отрисовка ──────────────────────────────────────────────────────────────

// drawContent — Flyout.Content: три плитки и ползунок громкости.
func (q *QuickSettings) drawContent(ctx widget.DrawContext, _ image.Rectangle) {
	if q.rich() {
		q.richDraw(ctx)
		return
	}
	q.drawNetworkTile(ctx)
	q.drawVolumeTile(ctx)
	q.drawPowerTile(ctx)
	q.drawSlider(ctx)
}

// drawNetworkTile рисует плитку Wi-Fi: подложка по состоянию (Active —
// сеть подключена), поверх — растущие полоски сигнала (тот же приём, что и
// у NetworkItem в tray.go, — переиспользуем drawLevelBars/ink/mutedInk).
func (q *QuickSettings) drawNetworkTile(ctx widget.DrawContext) {
	r := q.tileRectAt(quickSettingsTileNetwork)
	if r.Empty() {
		return
	}
	s := q.tileStyle("tile.network", r, q.netActive())
	PaintStyle(ctx, r, s)
	inner := shrinkByPad(r, s)
	if inner.Empty() {
		return
	}
	net := q.networkState()
	ratio := net.Quality
	if net.Kind == NetNone {
		ratio = 0
	}
	drawLevelBars(ctx, inner, ratio, networkBars, ink(s), mutedInk(s))
}

// drawVolumeTile рисует плитку звука: подложка по состоянию (Active — звук
// не приглушён), поверх — шкала уровня или перечёркивание при Muted (тот же
// приём, что и у VolumeItem в tray.go).
func (q *QuickSettings) drawVolumeTile(ctx widget.DrawContext) {
	r := q.tileRectAt(quickSettingsTileVolume)
	if r.Empty() {
		return
	}
	s := q.tileStyle("tile.volume", r, q.volumeActive())
	PaintStyle(ctx, r, s)
	inner := shrinkByPad(r, s)
	if inner.Empty() {
		return
	}
	vol := q.volumeState()
	if vol.Muted {
		drawDiagonal(ctx, inner, ink(s))
		return
	}
	drawLevelBars(ctx, inner, vol.Level, volumeBars, ink(s), mutedInk(s))
}

// drawPowerTile рисует плитку питания: подложка по состоянию (Active — от
// сети), поверх — заполнение по заряду.
func (q *QuickSettings) drawPowerTile(ctx widget.DrawContext) {
	r := q.tileRectAt(quickSettingsTilePower)
	if r.Empty() {
		return
	}
	s := q.tileStyle("tile.power", r, q.powerActive())
	PaintStyle(ctx, r, s)
	inner := shrinkByPad(r, s)
	if inner.Empty() {
		return
	}
	charge := q.powerState().Charge
	if charge < 0 {
		charge = 0
	}
	if charge > 1 {
		charge = 1
	}
	fillW := int(float64(inner.Dx()) * charge)
	if fillW > 0 {
		ctx.FillRect(inner.Min.X, inner.Min.Y, fillW, inner.Dy(), ink(s))
	}
}

// drawSlider рисует дорожку, заполненную часть и бегунок ползунка
// громкости частями стиля "slider" и "slider.fill".
func (q *QuickSettings) drawSlider(ctx widget.DrawContext) {
	r := q.sliderRect()
	tr := q.trackRect()
	if r.Empty() || tr.Empty() {
		return
	}

	track := q.partStyle("slider", theme.StateNormal)
	fillStyle := q.partStyle("slider.fill", theme.StateNormal)

	drawBar(ctx, tr, track)

	level := q.volumeState().Level
	if level < 0 {
		level = 0
	}
	if level > 1 {
		level = 1
	}
	fillW := int(float64(tr.Dx()) * level)
	if fillW > 0 {
		drawBar(ctx, image.Rect(tr.Min.X, tr.Min.Y, tr.Min.X+fillW, tr.Max.Y), fillStyle)
	}

	// Бегунок — визуальный ориентир на границе заполненной части, а не
	// хитбокс: тянуть можно с любой точки дорожки (см. beginDrag).
	thumbW := tr.Dy() * quickSettingsThumbDiv
	if thumbW < 1 {
		thumbW = 1
	}
	thumbX := tr.Min.X + fillW - thumbW/2
	if thumbX < r.Min.X {
		thumbX = r.Min.X
	}
	if thumbX+thumbW > r.Max.X {
		thumbX = r.Max.X - thumbW
	}
	if fillStyle.Fill.A > 0 {
		ctx.FillRect(thumbX, r.Min.Y, thumbW, r.Dy(), fillStyle.Fill)
	}
}

// drawBar заливает прямоугольник стилем s: скруглённо, если тема просит
// скругление угла (Corner > 0), иначе — прямым прямоугольником. Общий
// помощник для дорожки и заполненной части ползунка — они отличаются
// только стилем.
func drawBar(ctx widget.DrawContext, r image.Rectangle, s *theme.Style) {
	if r.Empty() || s.Fill.A == 0 {
		return
	}
	if s.Corner > 0 {
		ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), int(s.Corner), s.Fill)
	} else {
		ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), s.Fill)
	}
}

// ─── Тема и состояние ───────────────────────────────────────────────────────

// tileStyle читает стиль плитки part (занимающей r) в состоянии Active
// (active=true) или Normal. Смена состояния идёт плавным переходом цвета.
func (q *QuickSettings) tileStyle(part string, r image.Rectangle, active bool) *theme.Style {
	st := theme.StateNormal
	if active {
		st = theme.StateActive
	}
	return q.fade.Style(q.Theme(), part, r, st, func(st theme.State) *theme.Style {
		return q.partStyle(part, st)
	})
}

// partStyle читает стиль части part панели из активной темы (пустой стиль,
// если темы нет).
func (q *QuickSettings) partStyle(part string, st theme.State) *theme.Style {
	tm := q.Theme()
	if tm == nil {
		return &theme.Style{}
	}
	return tm.GetStyle(ComponentQuickSettings, part, st)
}

func (q *QuickSettings) networkState() NetState {
	if q.st == nil {
		return NetState{}
	}
	return q.st.Network()
}

func (q *QuickSettings) volumeState() VolState {
	if q.st == nil {
		return VolState{}
	}
	return q.st.Volume()
}

func (q *QuickSettings) powerState() PowerState {
	if q.st == nil {
		return PowerState{}
	}
	return q.st.Power()
}

// netActive/volumeActive/powerActive решают, показывать ли плитку
// «включённой» (стиль StateActive) или «выключенной» (StateNormal).
func (q *QuickSettings) netActive() bool    { return q.networkState().Kind != NetNone }
func (q *QuickSettings) volumeActive() bool { return !q.volumeState().Muted }
func (q *QuickSettings) powerActive() bool  { return q.powerState().OnAC }
