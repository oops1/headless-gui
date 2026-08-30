// tray.go — значки состояния в трее панели задач: сеть, звук, питание.
//
// Настоящих иконок пока нет (будут позже, из набора темы) — значки рисуются
// фигурами через DrawContext: полосками (сеть, шкала звука), прямоугольником
// с заполнением (батарея). Цвета и отступы — из стиля темы; ничего, что
// зависит от профиля, здесь не зашито числом.
package desktop

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Имена компонентов для стилей темы.
const (
	ComponentNetwork = "tray.network"
	ComponentVolume  = "tray.volume"
	ComponentPower   = "tray.power"
)

// KeyTrayIconSize — сторона квадратного значка трея (общая для сети, звука
// и питания: это одно и то же семейство значков одного размера). Больше
// взять размер неоткуда — в отличие от текста часов, значку нечего мерить.
const KeyTrayIconSize theme.Key = "tray.icon.size"

// Количество «сегментов» в шкалах значков. Это не размер в пикселях (запрет
// на магические размеры — про геометрию в пикселях и про цвета), а счётчик
// делений, такой же по духу, как glowSparks в widget/progressbar_glow.go.
const (
	networkBars     = 4 // число полосок индикатора сети
	volumeBars      = 3 // число дуг шкалы громкости
	volumeBodyDiv   = 3 // доля ширины значка под «корпус» динамика
	batteryNubDiv   = 6 // доля ширины значка под контакт батареи
	batteryInsetDiv = 4 // доля высоты, на которую контакт уже корпуса
	acStripeDiv     = 5 // доля высоты под полоску индикатора «от сети»

	// barGapDiv — доля ширины полоски (colW в drawLevelBars) под зазор до
	// соседней. На значке 16px, под который фигура была прикинана на глаз,
	// полоски и так узкие — зазор в пиксель-два не нужен. На поднятом
	// заказчиком KeyTrayIconSize 24 и тем более 32 полоски без зазора
	// сливаются в сплошную лесенку: вместо шкалы уровня виден один клин, и
	// «неинформативно» из жалобы никуда не делось. Зазор — доля от ширины
	// самой полоски, а не пиксельная константа, поэтому не съедает узкую
	// полоску на маленьком значке и не выглядит непропорционально широким
	// на крупном.
	barGapDiv = 4

	// diagonalThicknessDiv — доля высоты фигуры (Dy её прямоугольника) под
	// толщину перечёркивания drawDiagonalStrike. drawDiagonal сам всегда
	// рисует линию в один пиксель — на 16px этого достаточно, но на 24-32px
	// (тот же подъём KeyTrayIconSize) линия тонет среди остальных фигур, и
	// Muted/«нет сети» перестают читаться с одного взгляда. Доля, а не
	// пиксельная константа — по той же причине, что и у barGapDiv.
	diagonalThicknessDiv = 6
)

// boolState переводит bool в 0|1 для атомарных полей hover/pressed —
// см. widget.b2i (недоступен отсюда, пакет другой).
func boolState(v bool) int32 {
	if v {
		return 1
	}
	return 0
}

// trayHandleMove — общая логика обновления hover для значков трея: наведён,
// только если курсор в границах; перерисовывает при фактическом изменении.
func trayHandleMove(hovered *int32, bounds image.Rectangle, x, y int, invalidate func()) {
	want := boolState(image.Pt(x, y).In(bounds))
	if atomic.SwapInt32(hovered, want) != want {
		invalidate()
	}
}

// trayHandleClick — общая семантика клика по значку трея: нажатие взводит
// кнопку, срабатывает колбэк по ОТПУСКАНИЮ над границами (как
// widget/dialog.go: dialogCloseBtn — крестик закрытия срабатывает на
// release, не на press). Возвращает true, если событие поглощено.
func trayHandleClick(pressed *int32, bounds image.Rectangle, e widget.MouseEvent, onClick func(), invalidate func()) bool {
	if e.Button != widget.MouseLeft {
		return false
	}
	over := image.Pt(e.X, e.Y).In(bounds)
	if e.Pressed {
		if !over {
			return false
		}
		if atomic.SwapInt32(pressed, 1) != 1 {
			invalidate()
		}
		return true
	}
	was := atomic.SwapInt32(pressed, 0) == 1
	if !was {
		return false
	}
	invalidate()
	if over && onClick != nil {
		onClick()
	}
	return true
}

// trayState собирает состояние для темы из hover/pressed значка.
func trayState(hovered, pressed *int32) theme.State {
	return StateOf(atomic.LoadInt32(hovered) == 1, atomic.LoadInt32(pressed) == 1, false, false, false)
}

// trayIconSize читает общий размер значков трея из темы.
func trayIconSize(tm *theme.Manager) int {
	if tm == nil {
		return 0
	}
	return int(tm.GetMetric(KeyTrayIconSize))
}

// trayStyle читает стиль значка из темы (пустой стиль, если темы нет).
func trayStyle(tm *theme.Manager, component string, st theme.State) *theme.Style {
	if tm == nil {
		return &theme.Style{}
	}
	return tm.GetStyle(component, "", st)
}

// ink — цвет, которым рисуется сам значок.
//
// Значок трея — монохромный глиф, и рисуется он цветом ТЕКСТА: заливка в
// стиле значка описывает его подложку (у большинства тем её нет вовсе), и
// рисовать глиф заливкой значит рисовать невидимым по невидимому. Если тема
// цвет текста не задала, берём то, чем она вообще что-то рисует.
func ink(s *theme.Style) color.RGBA {
	if s.Text.A > 0 {
		return s.Text
	}
	if s.Border.A > 0 {
		return s.Border
	}
	return s.Fill
}

// mutedInk — те же чернила вполсилы: незаполненные деления шкалы, фон
// индикатора. Тема вправе задать для этого рамку; если не задала, глушим
// основной цвет сами.
func mutedInk(s *theme.Style) color.RGBA {
	if s.Border.A > 0 && s.Text.A > 0 {
		return s.Border
	}
	return fade(ink(s), 0.35)
}

// fade умножает цвет на k. Компоненты хранятся с предумноженной альфой, так
// что множатся все четыре — иначе цвет «поплывёт» вместо того, чтобы стать
// прозрачнее.
func fade(c color.RGBA, k float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(c.R) * k),
		G: uint8(float64(c.G) * k),
		B: uint8(float64(c.B) * k),
		A: uint8(float64(c.A) * k),
	}
}

// shrinkByPad обжимает прямоугольник на отступы стиля.
func shrinkByPad(b image.Rectangle, s *theme.Style) image.Rectangle {
	return image.Rect(
		b.Min.X+int(s.PadX), b.Min.Y+int(s.PadY),
		b.Max.X-int(s.PadX), b.Max.Y-int(s.PadY),
	)
}

// drawLevelBars рисует count растущих слева направо (по высоте) полосок:
// закрашено столько, сколько соответствует ratio ∈ [0,1]. Используется и
// сетью (Quality), и звуком (Level) — это одна и та же фигура с разными
// данными и разными цветами on/off.
func drawLevelBars(ctx widget.DrawContext, r image.Rectangle, ratio float64, count int, on, off color.RGBA) {
	if r.Empty() || count <= 0 {
		return
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(math.Round(ratio * float64(count)))

	colW := r.Dx() / count
	if colW <= 0 {
		return
	}
	// Зазор — см. barGapDiv: доля самой полоски, а не абсолютное число
	// пикселей. На совсем узкой полоске (мелкий значок, много делений)
	// зазор в барGapDiv долю округлится в 0 — тогда полоски просто
	// остаются слитыми, как раньше, что лучше, чем схлопнуть barW в 0 и
	// нарисовать пустоту вместо фигуры.
	barW := colW - colW/barGapDiv
	if barW < 1 {
		barW = colW
	}
	for i := 0; i < count; i++ {
		h := r.Dy() * (i + 1) / count
		x := r.Min.X + i*colW
		y := r.Max.Y - h
		col := off
		if i < filled {
			col = on
		}
		ctx.FillRect(x, y, barW, h, col)
	}
}

// drawDiagonal рисует диагональ из верхнего левого в правый нижний угол r —
// перечёркивание значка (звук в Muted). Фигура, а не иконка: настоящий
// глиф придёт из набора темы позже.
func drawDiagonal(ctx widget.DrawContext, r image.Rectangle, col color.RGBA) {
	if r.Dx() <= 0 || r.Dy() <= 0 {
		return
	}
	for i := 0; i <= r.Dx(); i++ {
		x := r.Min.X + i
		y := r.Min.Y + i*r.Dy()/r.Dx()
		ctx.SetPixel(x, y, col)
	}
}

// drawDiagonalOffset — тот же проход, что и drawDiagonal, но сдвинутый по Y
// на off пикселей и обрезанный по границам r. Сам по себе не фигура — вспомогательный
// проход drawDiagonalStrike для утолщения линии: сдвигать пришлось саму
// линию, а не сторону r, потому что drawDiagonal каждый раз пересчитывает
// наклон из Dy/Dx нового прямоугольника, и сдвиг r целиком менял бы угол,
// а не просто толщину.
func drawDiagonalOffset(ctx widget.DrawContext, r image.Rectangle, col color.RGBA, off int) {
	if r.Dx() <= 0 || r.Dy() <= 0 {
		return
	}
	for i := 0; i <= r.Dx(); i++ {
		x := r.Min.X + i
		y := r.Min.Y + i*r.Dy()/r.Dx() + off
		if y < r.Min.Y || y > r.Max.Y {
			// Обрезка по границам значка: без неё утолщение у самых концов
			// диагонали вылезало бы за r, а фигуры трея вылезать за
			// границы значка не должны ни при одном размере.
			continue
		}
		ctx.SetPixel(x, y, col)
	}
}

// drawDiagonalStrike — перечёркивание r диагональю толщиной, растущей с
// размером фигуры (diagonalThicknessDiv). Базовая линия — drawDiagonal как
// есть, тем же приёмом, каким notificationcenter.go и quicksettings.go
// рисуют перечёркивание в один пиксель (их не трогаем — вне этой задачи);
// сверху добавляются симметричные проходы drawDiagonalOffset, пока не
// наберётся нужная толщина. На 16px, под который линия была рассчитана
// изначально, толщина остаётся минимальной; на 24-32px (заказчик поднял
// KeyTrayIconSize) утолщается, иначе перечёркивание тонет среди остальных
// фигур значка и Muted/«нет сети» перестают читаться с одного взгляда.
func drawDiagonalStrike(ctx widget.DrawContext, r image.Rectangle, col color.RGBA) {
	if r.Dx() <= 0 || r.Dy() <= 0 {
		return
	}
	drawDiagonal(ctx, r, col)
	thickness := r.Dy() / diagonalThicknessDiv
	half := thickness / 2
	for d := 1; d <= half; d++ {
		drawDiagonalOffset(ctx, r, col, d)
		drawDiagonalOffset(ctx, r, col, -d)
	}
}

// trayTooltip — синхронизированный текст подсказки значка трея.
//
// widget.Base.ToolTip — обычное поле без замка: у большинства виджетов его
// меняет только горутина ввода/раскладки движка, там гонки нет. Значки трея
// — исключение: подсказку обновляет подписка на SystemStatus (см.
// networkTooltip и соседей), а замыкание Subscribe зовётся из горутины
// ПОТРЕБИТЕЛЯ (раздел «Из какой горутины что зовётся» в contract.go) — то
// есть возможно одновременно с тем, как движок читает подсказку через
// GetToolTip() из горутины кадра (engine/tooltip.go). widget/base.go —
// общий пакет, трогать его нельзя, поэтому значки трея вовсе не пишут в
// промоутнутое поле Base.ToolTip: заводят свой текст под своим замком и
// перекрывают GetToolTip/SetToolTip тем же приёмом, что и
// widget.TabControl.GetToolTip у вкладок.
type trayTooltip struct {
	mu   sync.Mutex
	text string
}

// get читает текст под замком — вызывается из горутины кадра (движком,
// через GetToolTip()).
func (t *trayTooltip) get() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.text
}

// set меняет текст под замком — вызывается из горутины потребителя (из
// замыкания Subscribe) или явным SetToolTip.
func (t *trayTooltip) set(s string) {
	t.mu.Lock()
	t.text = s
	t.mu.Unlock()
}

// ─── Сеть ────────────────────────────────────────────────────────────────────

// NetworkItem — значок состояния сети в трее.
type NetworkItem struct {
	widget.Base

	tm *theme.Manager
	st SystemStatus

	// OnClick — колбэк клика (оболочка вешает на него открытие панели сети).
	OnClick func()

	hovered int32
	pressed int32

	// tt — подсказка «Сеть: <имя>, подключено» / «Сеть: нет подключения»,
	// обновляемая подпиской на st (см. trayTooltip — своя синхронизация,
	// потому что промоутнутое Base.ToolTip её не даёт).
	tt trayTooltip

	unsub func()
}

// NewNetworkStatus создаёт значок сети, оформляемый темой tm и следящий за
// показателями st.
func NewNetworkStatus(tm *theme.Manager, st SystemStatus) *NetworkItem {
	it := &NetworkItem{tm: tm, st: st}
	it.refreshTooltip()
	if st != nil {
		it.unsub = st.Subscribe(func() {
			// Короткое замыкание (контракт горутины потребителя): читает
			// текущее состояние через уже потокобезопасный st.Network() и
			// пишет под замком trayTooltip; Invalidate — уже вне замка,
			// как и в остальных компонентах пакета (RunningApplications).
			it.refreshTooltip()
			it.Invalidate()
		})
	}
	return it
}

// refreshTooltip пересчитывает подсказку по текущему состоянию сети.
func (n *NetworkItem) refreshTooltip() {
	n.tt.set(networkTooltip(n.networkState()))
}

// GetToolTip / SetToolTip перекрывают промоутнутые из widget.Base — см.
// trayTooltip.
func (n *NetworkItem) GetToolTip() string  { return n.tt.get() }
func (n *NetworkItem) SetToolTip(s string) { n.tt.set(s) }

// Close снимает подписку на SystemStatus.
func (n *NetworkItem) Close() {
	if n.unsub != nil {
		n.unsub()
		n.unsub = nil
	}
}

// PreferredSize — квадрат стороной из темы.
func (n *NetworkItem) PreferredSize(image.Point) image.Point {
	size := trayIconSize(n.tm)
	// Ширина — значок плюс отступы стиля с обеих сторон: вплотную друг к
	// другу значки трея не стоят ни в одной из тем.
	return image.Point{X: size + 2*int(trayStyle(n.tm, ComponentNetwork, theme.StateNormal).PadX), Y: size}
}

// OnMouseMove обновляет hover.
func (n *NetworkItem) OnMouseMove(x, y int) {
	trayHandleMove(&n.hovered, n.Bounds(), x, y, n.Invalidate)
}

// OnMouseButton реализует клик (release над границами).
func (n *NetworkItem) OnMouseButton(e widget.MouseEvent) bool {
	return trayHandleClick(&n.pressed, n.Bounds(), e, n.OnClick, n.Invalidate)
}

// Draw рисует растущие полоски сигнала: закрашено столько, сколько
// заслуживает Quality; NetNone — сигнала нет вовсе.
func (n *NetworkItem) Draw(ctx widget.DrawContext) {
	b := n.Bounds()
	if b.Empty() {
		return
	}
	st := trayState(&n.hovered, &n.pressed)
	s := trayStyle(n.tm, ComponentNetwork, st)
	PaintStyle(ctx, b, s)

	inner := shrinkByPad(b, s)
	if inner.Empty() {
		return
	}
	net := n.networkState()
	if net.Kind == NetNone {
		// Раньше «нет сети» рисовалось как ratio=0 — те же полоски, что и у
		// слабого сигнала, просто все тусклые. С одного взгляда «отключено»
		// от «еле ловит» так не отличить: оттенок — не тот признак, который
		// замечают мельком. Нужна другая ФИГУРА — перечёркивание.
		//
		// Но полоски под ним остаются, и это важно: одна голая диагональ
		// сообщает «что-то перечёркнуто», а не «нет сети». Значок обязан
		// сперва читаться как значок сети, и только потом — как выключенный,
		// ровно так же, как перечёркнутый динамик остаётся динамиком.
		drawLevelBars(ctx, inner, 0, networkBars, mutedInk(s), mutedInk(s))
		drawDiagonalStrike(ctx, inner, ink(s))
		return
	}
	drawLevelBars(ctx, inner, net.Quality, networkBars, ink(s), mutedInk(s))
}

func (n *NetworkItem) networkState() NetState {
	if n.st == nil {
		return NetState{}
	}
	return n.st.Network()
}

// networkTooltip формирует текст подсказки сети. Полоски и перечёркивание
// говорят «связь/нет связи», но не называют сеть по имени — ради этого и
// нужна подсказка, а не только ради самого факта её наличия.
func networkTooltip(net NetState) string {
	if net.Kind == NetNone {
		return "Сеть: нет подключения"
	}
	if net.Name == "" {
		return "Сеть: подключено"
	}
	return "Сеть: " + net.Name + ", подключено"
}

// ─── Звук ────────────────────────────────────────────────────────────────────

// VolumeItem — значок громкости в трее.
type VolumeItem struct {
	widget.Base

	tm *theme.Manager
	st SystemStatus

	// OnClick — колбэк клика (оболочка вешает на него открытие панели громкости).
	OnClick func()

	hovered int32
	pressed int32

	// tt — подсказка «Звук: N%» / «Звук: выключен» (см. NetworkItem.tt —
	// та же причина: Base.ToolTip без замка, а подписка зовётся из
	// горутины потребителя).
	tt trayTooltip

	unsub func()
}

// NewVolumeStatus создаёт значок звука, оформляемый темой tm и следящий за
// показателями st.
func NewVolumeStatus(tm *theme.Manager, st SystemStatus) *VolumeItem {
	it := &VolumeItem{tm: tm, st: st}
	it.refreshTooltip()
	if st != nil {
		it.unsub = st.Subscribe(func() {
			it.refreshTooltip()
			it.Invalidate()
		})
	}
	return it
}

// refreshTooltip пересчитывает подсказку по текущему состоянию звука.
func (v *VolumeItem) refreshTooltip() {
	v.tt.set(volumeTooltip(v.volumeState()))
}

// GetToolTip / SetToolTip перекрывают промоутнутые из widget.Base — см.
// trayTooltip.
func (v *VolumeItem) GetToolTip() string  { return v.tt.get() }
func (v *VolumeItem) SetToolTip(s string) { v.tt.set(s) }

// Close снимает подписку на SystemStatus.
func (v *VolumeItem) Close() {
	if v.unsub != nil {
		v.unsub()
		v.unsub = nil
	}
}

// PreferredSize — квадрат стороной из темы.
func (v *VolumeItem) PreferredSize(image.Point) image.Point {
	size := trayIconSize(v.tm)
	// Ширина — значок плюс отступы стиля с обеих сторон: вплотную друг к
	// другу значки трея не стоят ни в одной из тем.
	return image.Point{X: size + 2*int(trayStyle(v.tm, ComponentVolume, theme.StateNormal).PadX), Y: size}
}

// OnMouseMove обновляет hover.
func (v *VolumeItem) OnMouseMove(x, y int) {
	trayHandleMove(&v.hovered, v.Bounds(), x, y, v.Invalidate)
}

// OnMouseButton реализует клик (release над границами).
func (v *VolumeItem) OnMouseButton(e widget.MouseEvent) bool {
	return trayHandleClick(&v.pressed, v.Bounds(), e, v.OnClick, v.Invalidate)
}

// Draw рисует «корпус» динамика слева и шкалу громкости справа; при Muted
// шкала не рисуется вовсе, а значок перечёркнут по диагонали.
func (v *VolumeItem) Draw(ctx widget.DrawContext) {
	b := v.Bounds()
	if b.Empty() {
		return
	}
	st := trayState(&v.hovered, &v.pressed)
	s := trayStyle(v.tm, ComponentVolume, st)
	PaintStyle(ctx, b, s)

	inner := shrinkByPad(b, s)
	if inner.Empty() {
		return
	}
	vol := v.volumeState()

	bodyW := inner.Dx() / volumeBodyDiv
	body := image.Rect(inner.Min.X, inner.Min.Y, inner.Min.X+bodyW, inner.Max.Y)
	ctx.FillRect(body.Min.X, body.Min.Y, body.Dx(), body.Dy(), ink(s))

	if vol.Muted {
		// drawDiagonalStrike, не drawDiagonal: на 16px однопиксельная
		// диагональ ещё видна, но на поднятом заказчиком KeyTrayIconSize
		// (24, 32) тонет среди остальных фигур — Muted обязан читаться при
		// любом размере значка, а не только при исходном.
		drawDiagonalStrike(ctx, inner, ink(s))
		return
	}
	bars := image.Rect(body.Max.X, inner.Min.Y, inner.Max.X, inner.Max.Y)
	drawLevelBars(ctx, bars, vol.Level, volumeBars, ink(s), mutedInk(s))
}

func (v *VolumeItem) volumeState() VolState {
	if v.st == nil {
		return VolState{}
	}
	return v.st.Volume()
}

// volumeTooltip формирует текст подсказки звука.
func volumeTooltip(vol VolState) string {
	if vol.Muted {
		return "Звук: выключен"
	}
	return fmt.Sprintf("Звук: %d%%", int(math.Round(vol.Level*100)))
}

// ─── Питание ─────────────────────────────────────────────────────────────────

// PowerItem — значок питания (батарея) в трее. На настольной машине без
// батареи (PowerState.NoBattery) значок вообще не показывается —
// PreferredSize отдаёт нулевую ширину, и панель не отводит ему места.
type PowerItem struct {
	widget.Base

	tm *theme.Manager
	st SystemStatus

	// OnClick — колбэк клика (оболочка вешает на него открытие панели питания).
	OnClick func()

	hovered int32
	pressed int32

	// tt — подсказка «Батарея: N%» / «Питание от сети» (см. NetworkItem.tt).
	tt trayTooltip

	unsub func()
}

// NewPowerStatus создаёт значок питания, оформляемый темой tm и следящий за
// показателями st.
func NewPowerStatus(tm *theme.Manager, st SystemStatus) *PowerItem {
	it := &PowerItem{tm: tm, st: st}
	it.refreshTooltip()
	if st != nil {
		it.unsub = st.Subscribe(func() {
			it.refreshTooltip()
			it.Invalidate()
		})
	}
	return it
}

// refreshTooltip пересчитывает подсказку по текущему состоянию питания.
func (p *PowerItem) refreshTooltip() {
	p.tt.set(powerTooltip(p.powerState()))
}

// GetToolTip / SetToolTip перекрывают промоутнутые из widget.Base — см.
// trayTooltip.
func (p *PowerItem) GetToolTip() string  { return p.tt.get() }
func (p *PowerItem) SetToolTip(s string) { p.tt.set(s) }

// Close снимает подписку на SystemStatus.
func (p *PowerItem) Close() {
	if p.unsub != nil {
		p.unsub()
		p.unsub = nil
	}
}

// PreferredSize — квадрат стороной из темы; нулевая ширина, если батареи нет.
func (p *PowerItem) PreferredSize(image.Point) image.Point {
	if p.powerState().NoBattery {
		return image.Point{}
	}
	size := trayIconSize(p.tm)
	// Ширина — значок плюс отступы стиля с обеих сторон: вплотную друг к
	// другу значки трея не стоят ни в одной из тем.
	return image.Point{X: size + 2*int(trayStyle(p.tm, ComponentPower, theme.StateNormal).PadX), Y: size}
}

// OnMouseMove обновляет hover.
func (p *PowerItem) OnMouseMove(x, y int) {
	trayHandleMove(&p.hovered, p.Bounds(), x, y, p.Invalidate)
}

// OnMouseButton реализует клик (release над границами).
func (p *PowerItem) OnMouseButton(e widget.MouseEvent) bool {
	return trayHandleClick(&p.pressed, p.Bounds(), e, p.OnClick, p.Invalidate)
}

// Draw рисует корпус батареи с контактом и заполнением по Charge; питание от
// сети (OnAC) подсвечивается полоской сверху.
func (p *PowerItem) Draw(ctx widget.DrawContext) {
	b := p.Bounds()
	if b.Empty() {
		return
	}
	pw := p.powerState()
	if pw.NoBattery {
		return
	}
	st := trayState(&p.hovered, &p.pressed)
	s := trayStyle(p.tm, ComponentPower, st)
	PaintStyle(ctx, b, s)

	inner := shrinkByPad(b, s)
	if inner.Empty() {
		return
	}

	nubW := inner.Dx() / batteryNubDiv
	body := image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X-nubW, inner.Max.Y)
	corner := int(s.Corner)
	if corner > 0 {
		ctx.DrawRoundBorder(body.Min.X, body.Min.Y, body.Dx(), body.Dy(), corner, ink(s))
	} else {
		ctx.DrawBorder(body.Min.X, body.Min.Y, body.Dx(), body.Dy(), ink(s))
	}

	nubInset := body.Dy() / batteryInsetDiv
	nub := image.Rect(body.Max.X, body.Min.Y+nubInset, inner.Max.X, body.Max.Y-nubInset)
	if !nub.Empty() {
		ctx.FillRect(nub.Min.X, nub.Min.Y, nub.Dx(), nub.Dy(), ink(s))
	}

	charge := pw.Charge
	if charge < 0 {
		charge = 0
	}
	if charge > 1 {
		charge = 1
	}
	fillW := int(float64(body.Dx()) * charge)
	if fillW > 0 {
		ctx.FillRect(body.Min.X, body.Min.Y, fillW, body.Dy(), ink(s))
	}

	if pw.OnAC {
		stripeH := inner.Dy() / acStripeDiv
		if stripeH > 0 {
			ctx.FillRect(body.Min.X, body.Min.Y, body.Dx(), stripeH, ink(s))
		}
	}
}

func (p *PowerItem) powerState() PowerState {
	if p.st == nil {
		return PowerState{}
	}
	return p.st.Power()
}

// powerTooltip формирует текст подсказки питания.
func powerTooltip(pw PowerState) string {
	if pw.OnAC {
		return "Питание от сети"
	}
	return fmt.Sprintf("Батарея: %d%%", int(math.Round(pw.Charge*100)))
}
