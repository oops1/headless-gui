// tray.go — значки состояния в трее панели задач: сеть, звук, питание.
//
// Настоящих иконок пока нет (будут позже, из набора темы) — значки рисуются
// фигурами через DrawContext: полосками (сеть, шкала звука), прямоугольником
// с заполнением (батарея). Цвета и отступы — из стиля темы; ничего, что
// зависит от профиля, здесь не зашито числом.
package desktop

import (
	"image"
	"image/color"
	"math"
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
	for i := 0; i < count; i++ {
		h := r.Dy() * (i + 1) / count
		x := r.Min.X + i*colW
		y := r.Max.Y - h
		col := off
		if i < filled {
			col = on
		}
		ctx.FillRect(x, y, colW, h, col)
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

	unsub func()
}

// NewNetworkStatus создаёт значок сети, оформляемый темой tm и следящий за
// показателями st.
func NewNetworkStatus(tm *theme.Manager, st SystemStatus) *NetworkItem {
	it := &NetworkItem{tm: tm, st: st}
	if st != nil {
		it.unsub = st.Subscribe(it.Invalidate)
	}
	return it
}

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
	ratio := net.Quality
	if net.Kind == NetNone {
		ratio = 0
	}
	drawLevelBars(ctx, inner, ratio, networkBars, ink(s), mutedInk(s))
}

func (n *NetworkItem) networkState() NetState {
	if n.st == nil {
		return NetState{}
	}
	return n.st.Network()
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

	unsub func()
}

// NewVolumeStatus создаёт значок звука, оформляемый темой tm и следящий за
// показателями st.
func NewVolumeStatus(tm *theme.Manager, st SystemStatus) *VolumeItem {
	it := &VolumeItem{tm: tm, st: st}
	if st != nil {
		it.unsub = st.Subscribe(it.Invalidate)
	}
	return it
}

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
		drawDiagonal(ctx, inner, ink(s))
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

	unsub func()
}

// NewPowerStatus создаёт значок питания, оформляемый темой tm и следящий за
// показателями st.
func NewPowerStatus(tm *theme.Manager, st SystemStatus) *PowerItem {
	it := &PowerItem{tm: tm, st: st}
	if st != nil {
		it.unsub = st.Subscribe(it.Invalidate)
	}
	return it
}

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
