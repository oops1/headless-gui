// taskpill.go — индикаторы кнопок окон Windows 11: пилюля под значком,
// наложение прогресса, счётчик в кружке и «внимание».
//
// Всё это включает ТЕМА: пока она не объявила метрики индикаторов
// (taskbutton.pill.height, .progress.height, .badge.size, флаг
// taskbutton.attention), кнопка рисуется как раньше — старой меткой
// DrawUnderline и без наложений. Поэтому Windows 10, Windows 2000 и macOS
// получают от этого файла ровно прежние кадры, даже если модель окон заполняет
// новые поля WindowInfo.
//
// Данные приходят от потребителя в WindowInfo (ProgressState, Progress, Badge,
// Attention); ни имени темы, ни литералов размеров и цветов здесь нет: размеры —
// метрики темы, цвета — части стиля «taskbutton».
//
// Движение — по токенам анимации темы и под общим флагом «меньше движения»:
//
//   - пилюля плавно меняет ширину между «запущено» (6) и «активно» (16), цвет
//     идёт за шириной от серого к акценту (анимация taskbutton.pill); каждый
//     шаг перерисовывает только область своей кнопки;
//   - «внимание» мигает подложкой несколько раз (taskbutton.attention —
//     длительность одного мигания, taskbutton.attention.blinks — их число) и
//     остаётся постоянной подсветкой, пока окно не станет активным. При
//     «меньше движения» мигания нет — сразу постоянная.
package desktop

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Ключи токенов индикаторов кнопки окна.
const (
	// KeyTaskButtonHeight — высота кнопки (подсветка и область щелчка),
	// по центру высоты панели; 0 — во всю высоту панели, как раньше.
	KeyTaskButtonHeight theme.Key = "taskbutton.height"

	// KeyTaskPillHeight — толщина пилюли; 0 — пилюли нет (прежняя метка).
	KeyTaskPillHeight theme.Key = "taskbutton.pill.height"
	// KeyTaskPillIdle и KeyTaskPillActive — длина пилюли у запущенного и у
	// активного окна.
	KeyTaskPillIdle   theme.Key = "taskbutton.pill.idle"
	KeyTaskPillActive theme.Key = "taskbutton.pill.active"
	// KeyTaskPillOffset — расстояние от нижнего края кнопки до нижнего края
	// пилюли; отрицательное — пилюля стоит ниже кнопки, у самого края панели.
	KeyTaskPillOffset theme.Key = "taskbutton.pill.offset"
	// KeyTaskPillIdleOpacity — непрозрачность цвета пилюли запущенного окна
	// (доля 0..1 от цвета части "pill"): серая, а не сплошная.
	KeyTaskPillIdleOpacity theme.Key = "taskbutton.pill.idle.opacity"

	// KeyTaskProgressHeight — толщина полосы прогресса; 0 — наложения нет.
	KeyTaskProgressHeight theme.Key = "taskbutton.progress.height"
	// KeyTaskBadgeSize — диаметр (высота) счётчика; 0 — счётчика нет.
	KeyTaskBadgeSize theme.Key = "taskbutton.badge.size"

	// KeyTaskAttention — флаг: тема рисует «внимание» (подложка мигает).
	KeyTaskAttention theme.Key = "taskbutton.attention"
	// KeyTaskAttentionBlinks — сколько раз подложка мигает до постоянной.
	KeyTaskAttentionBlinks theme.Key = "taskbutton.attention.blinks"

	// AnimTaskPill — смена ширины и цвета пилюли.
	AnimTaskPill theme.Key = "taskbutton.pill"
	// AnimTaskAttention — одно мигание «внимания».
	AnimTaskAttention theme.Key = "taskbutton.attention"
)

// Части стиля «taskbutton» для индикаторов.
const (
	// PartTaskPill — пилюля: Fill в покое — цвет запущенного, в Active — активного.
	PartTaskPill = "pill"
	// PartTaskProgress — дорожка полосы прогресса; ".fill", ".paused", ".error" —
	// сама полоса по состоянию.
	PartTaskProgress       = "progress"
	PartTaskProgressFill   = "progress.fill"
	PartTaskProgressPaused = "progress.paused"
	PartTaskProgressError  = "progress.error"
	// PartTaskBadge — кружок счётчика: Fill — кружок, Text — цифры, Font.Size —
	// кегль цифр, PadX — поля по бокам у числа из нескольких цифр.
	PartTaskBadge = "badge"
	// PartTaskAttention — цвет подложки «внимания».
	PartTaskAttention = "attention"
)

// maxTaskBadge — предел счётчика: больше показывается «99+».
const maxTaskBadge = 99

// taskOverlay — наложения одной кнопки: всё, что модель окон говорит о
// прогрессе, счётчике и внимании. Сравнимо — по нему узнаётся, что изменились
// только наложения и достаточно перерисовать одну кнопку.
type taskOverlay struct {
	pstate    ProgressState
	progress  float64
	badge     int
	attention bool
}

// overlayOf собирает наложения кнопки по её окнам. У стопки показывается самое
// важное состояние прогресса (ошибка, пауза, обычный) с наибольшей долей среди
// окон этого состояния, счётчики суммируются, «внимание» — если его просит хоть
// одно не активное окно.
func overlayOf(wins []WindowInfo) taskOverlay {
	var o taskOverlay
	for _, w := range wins {
		if w.ProgressState > o.pstate || (w.ProgressState == o.pstate && w.ProgressState != ProgressNone && w.Progress > o.progress) {
			o.pstate, o.progress = w.ProgressState, w.Progress
		}
		if w.Badge > 0 {
			o.badge += w.Badge
		}
		if w.Attention && !w.Active {
			o.attention = true
		}
	}
	switch {
	case o.progress < 0:
		o.progress = 0
	case o.progress > 1:
		o.progress = 1
	}
	return o
}

// ─── Плавное число (ширина пилюли) ──────────────────────────────────────────

// numMotion — плавные числа для набора ячеек: у каждой ячейки одно число, идущее
// к цели по анимации темы. Устроено как motion (цвет), но число своё; шаги
// перерисовывают область ячейки, а не элемент.
type numMotion struct {
	mu    sync.Mutex
	cells map[any]*numCell
}

type numCell struct {
	cur, tgt float64
	rect     image.Rectangle
	tag      string
}

// value возвращает текущее значение числа ячейки id и ведёт его к target.
// Первое появление ячейки сразу стоит на цели: пилюля кнопки, которая только
// что возникла активной, и рисуется активной. r — область, которую
// перерисовывают шаги.
func (m *numMotion) value(tm *theme.Manager, key theme.Key, id any, r image.Rectangle, target float64) float64 {
	m.mu.Lock()
	if m.cells == nil {
		m.cells = make(map[any]*numCell)
	}
	c := m.cells[id]
	if c == nil {
		if len(m.cells) >= maxMotionCells {
			for k, v := range m.cells {
				if v.cur == v.tgt {
					delete(m.cells, k)
				}
			}
		}
		m.cells[id] = &numCell{cur: target, tgt: target, rect: r, tag: "num:" + fmt.Sprint(id)}
		m.mu.Unlock()
		return target
	}
	c.rect = r
	if c.tgt == target {
		cur := c.cur
		m.mu.Unlock()
		return cur
	}
	dur, curve := animation(tm, key)
	if dur <= 0 {
		c.cur, c.tgt = target, target
		m.mu.Unlock()
		return target
	}
	from := c.cur
	c.tgt = target
	cell, tag := c, c.tag
	m.mu.Unlock()

	// Новая анимация той же ячейки заменяет прежнюю и стартует от текущего
	// значения: оборванный переход не прыгает.
	widget.AnimateOwned(m, tag, dur, curve, func(t float64) {
		m.mu.Lock()
		cell.cur = widget.LerpF(from, target, t)
		if t >= 1 {
			cell.cur = target
		}
		rect := cell.rect
		m.mu.Unlock()
		widget.InvalidateRect(rect)
	})
	m.mu.Lock()
	cur := c.cur
	m.mu.Unlock()
	return cur
}

// ─── «Внимание» ─────────────────────────────────────────────────────────────

// attnMotion — мигание подложки кнопок, просящих внимания.
type attnMotion struct {
	mu    sync.Mutex
	cells map[any]*attnCell
}

type attnCell struct {
	factor float64 // 0..1 — насколько подложка видна сейчас
	rect   image.Rectangle
	tag    string
}

// factor возвращает силу подложки «внимания» кнопки id: 0 — нет, 1 — постоянная
// подсветка, между ними — мигание. Мигание начинается, когда on переходит из
// false в true, и идёт blinks раз по одному разу в AnimTaskAttention; с
// нулевой длительностью («меньше движения» или токен не объявлен) подсветка
// сразу постоянная.
func (m *attnMotion) factor(tm *theme.Manager, id any, r image.Rectangle, on bool) float64 {
	m.mu.Lock()
	if m.cells == nil {
		m.cells = make(map[any]*attnCell)
	}
	c := m.cells[id]
	if !on {
		if c != nil {
			delete(m.cells, id)
		}
		m.mu.Unlock()
		return 0
	}
	if c != nil {
		c.rect = r
		f := c.factor
		m.mu.Unlock()
		return f
	}
	if len(m.cells) >= maxMotionCells {
		m.cells = make(map[any]*attnCell) // забытые мигания дорисуются постоянной подсветкой
	}
	c = &attnCell{factor: 1, rect: r, tag: "attn:" + fmt.Sprint(id)}
	m.cells[id] = c
	blinks := int(tmMetric(tm, KeyTaskAttentionBlinks))
	one, curve := animation(tm, AnimTaskAttention)
	if one <= 0 || blinks <= 0 {
		m.mu.Unlock()
		return 1
	}
	cell, tag := c, c.tag
	m.mu.Unlock()

	// Подложка идёт от полной силы вниз и обратно blinks раз; последний шаг
	// приходит ровно в полную силу, поэтому переход к постоянной подсветке без
	// скачка.
	widget.AnimateOwned(m, tag, one*time.Duration(blinks), curve, func(t float64) {
		m.mu.Lock()
		if m.cells[id] != cell {
			m.mu.Unlock()
			return
		}
		cell.factor = 0.5 + 0.5*math.Cos(2*math.Pi*float64(blinks)*t)
		if t >= 1 {
			cell.factor = 1
		}
		rect := cell.rect
		m.mu.Unlock()
		widget.InvalidateRect(rect)
	})
	return 1
}

// ─── Рисование ──────────────────────────────────────────────────────────────

// pillEnabled — рисует ли тема пилюли (иначе прежняя метка).
func pillEnabled(tm *theme.Manager) bool {
	return tmMetric(tm, KeyTaskPillHeight) > 0
}

// pillRect — прямоугольник пилюли шириной w под кнопкой r.
func pillRect(tm *theme.Manager, r image.Rectangle, w int) image.Rectangle {
	h := int(tmMetric(tm, KeyTaskPillHeight))
	bottom := r.Max.Y - int(tmMetric(tm, KeyTaskPillOffset))
	x := r.Min.X + (r.Dx()-w)/2
	return image.Rect(x, bottom-h, x+w, bottom)
}

// drawPill рисует пилюлю кнопки: ширина идёт к цели по анимации темы, цвет — от
// серого запущенного к акценту активного вместе с шириной.
func (a *ApplicationArea) drawPill(ctx widget.DrawContext, r image.Rectangle, e appEntry) {
	idle := tmMetric(a.tm, KeyTaskPillIdle)
	active := tmMetric(a.tm, KeyTaskPillActive)
	if active < idle {
		active = idle
	}
	target := 0.0
	if e.live {
		target = idle
		if e.active {
			target = active
		}
	}
	// Область шагов — кнопка вместе с пилюлей: она может стоять ниже кнопки.
	area := r.Union(pillRect(a.tm, r, int(active)))
	cur := a.pills.value(a.tm, AnimTaskPill, e.key(), area, target)
	w := int(math.Round(cur))
	if w <= 0 {
		return
	}

	k := 1.0
	if active > idle {
		k = (cur - idle) / (active - idle)
	}
	if k < 0 {
		k = 0
	}
	if k > 1 {
		k = 1
	}
	idleStyle := styleOf(a.tm, ComponentTaskButton, PartTaskPill, theme.StateNormal)
	activeStyle := styleOf(a.tm, ComponentTaskButton, PartTaskPill, theme.StateActive)
	opacity := tmMetric(a.tm, KeyTaskPillIdleOpacity)
	if opacity <= 0 || opacity > 1 {
		opacity = 1
	}
	col := widget.LerpColor(fadeColor(idleStyle.Fill, opacity), activeStyle.Fill, k)
	if col.A == 0 {
		return
	}
	pr := pillRect(a.tm, r, w)
	if pr.Empty() {
		return
	}
	corner := pr.Dy() / 2
	if corner*2 > pr.Dx() {
		corner = pr.Dx() / 2
	}
	fillSolid(ctx, pr, corner, col)
}

// drawAttention рисует подложку «внимания» силой f (0..1) поверх подсветки
// кнопки, но под значком.
func drawAttention(ctx widget.DrawContext, tm *theme.Manager, r image.Rectangle, f float64) {
	if f <= 0 {
		return
	}
	s := styleOf(tm, ComponentTaskButton, PartTaskAttention, theme.StateNormal)
	if s.Fill.A == 0 {
		return
	}
	fillSolid(ctx, r, int(s.Corner), fadeColor(s.Fill, f))
}

// drawProgress рисует полосу прогресса у нижнего края значка: дорожка во всю
// ширину значка и полоса долей прогресса цветом состояния.
func drawProgress(ctx widget.DrawContext, tm *theme.Manager, icon image.Rectangle, o taskOverlay) {
	h := int(tmMetric(tm, KeyTaskProgressHeight))
	if h <= 0 || o.pstate == ProgressNone || icon.Empty() {
		return
	}
	track := image.Rect(icon.Min.X, icon.Max.Y-h, icon.Max.X, icon.Max.Y)
	ts := styleOf(tm, ComponentTaskButton, PartTaskProgress, theme.StateNormal)
	corner := int(ts.Corner)
	if ts.Fill.A > 0 {
		fillSolid(ctx, track, corner, ts.Fill)
	}
	part := PartTaskProgressFill
	switch o.pstate {
	case ProgressPaused:
		part = PartTaskProgressPaused
	case ProgressError:
		part = PartTaskProgressError
	}
	fs := styleOf(tm, ComponentTaskButton, part, theme.StateNormal)
	w := int(math.Round(float64(track.Dx()) * o.progress))
	if w <= 0 || fs.Fill.A == 0 {
		return
	}
	bar := image.Rect(track.Min.X, track.Min.Y, track.Min.X+w, track.Max.Y)
	if c := int(fs.Corner); c > 0 {
		corner = c
	}
	if corner*2 > bar.Dx() {
		corner = bar.Dx() / 2
	}
	fillSolid(ctx, bar, corner, fs.Fill)
}

// drawTaskBadge рисует счётчик: кружок (у числа из нескольких цифр — овал) у
// верхнего правого угла значка, не выходя за кнопку r.
func drawTaskBadge(ctx widget.DrawContext, tm *theme.Manager, r, icon image.Rectangle, count int) {
	size := int(tmMetric(tm, KeyTaskBadgeSize))
	if size <= 0 || count <= 0 || icon.Empty() {
		return
	}
	text := strconv.Itoa(count)
	if count > maxTaskBadge {
		text = strconv.Itoa(maxTaskBadge) + "+"
	}
	s := styleOf(tm, ComponentTaskButton, PartTaskBadge, theme.StateNormal)
	w := size
	if tw := MeasureText(ctx, text, s) + 2*int(s.PadX); tw > w {
		w = tw
	}
	// Счётчик перекрывает угол значка: на треть выступает вправо и вверх, но
	// остаётся внутри кнопки — подсветка и область щелчка кончаются на ней.
	right := icon.Max.X + size/3
	if right > r.Max.X-1 {
		right = r.Max.X - 1
	}
	top := icon.Min.Y - size/3
	if top < r.Min.Y {
		top = r.Min.Y
	}
	box := image.Rect(right-w, top, right, top+size)
	if s.Fill.A > 0 {
		fillSolid(ctx, box, size/2, s.Fill)
	}
	DrawTextCentered(ctx, box, text, s)
}

// ─── Частичная перерисовка ──────────────────────────────────────────────────

// sameLook — ячейки выглядят одинаково во всём, кроме наложений: тогда смена
// модели не требует перерисовки области, достаточно кнопки.
func sameLook(a, b appEntry) bool {
	if a.app != b.app || a.title != b.title || a.window != b.window || a.live != b.live ||
		a.active != b.active || a.min != b.min || a.group != b.group || len(a.wins) != len(b.wins) {
		return false
	}
	if !sameIcon(a.icon, b.icon) || (a.iconAt == nil) != (b.iconAt == nil) {
		return false
	}
	for i := range a.wins {
		x, y := a.wins[i], b.wins[i]
		if x.ID != y.ID || x.Title != y.Title || x.Active != y.Active || x.Minimized != y.Minimized ||
			!sameIcon(x.Icon, y.Icon) {
			return false
		}
	}
	return true
}

// sameIcon сравнивает картинки по значению интерфейса; картинка несравнимого
// динамического типа считается другой (перерисуем лишнее, но не упадём).
func sameIcon(a, b image.Image) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()
	return a == b
}

// overlayChanges возвращает номера ячеек, у которых изменились только наложения.
// ok == false — изменилось что-то ещё (состав, заголовки, состояния), и надо
// перерисовывать область целиком.
func overlayChanges(old, cur []appEntry) (idx []int, ok bool) {
	if len(old) != len(cur) {
		return nil, false
	}
	for i := range cur {
		if !sameLook(old[i], cur[i]) {
			return nil, false
		}
		if old[i].ov != cur[i].ov {
			idx = append(idx, i)
		}
	}
	return idx, true
}
