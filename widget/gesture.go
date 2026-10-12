package widget

// gesture.go — распознаватель жестов поверх TouchEvent.
//
// Чистая логика без платформы и без дерева виджетов: на вход — события касаний,
// на выход — жесты. Каждое приложение с касаниями иначе писало бы свой щипок и
// свой свайп со своими порогами, и в одной программе жесты вели бы себя по-разному.
//
// Что распознаётся:
//
//   - Tap — короткое касание без заметного сдвига; Count считает серию тапов (2 —
//     двойной). Двойной тап не откладывает одиночный: первый Tap приходит сразу, второй
//     — с Count == 2, и приложение само решает, ждать ли ему.
//   - Pan — один палец ушёл дальше PanSlop и тянет. Приходит потоком Began, Changed…,
//     Ended (или Cancelled). Delta — сдвиг с прошлого события, Total — от начала.
//   - Swipe — быстрый «мазок»: палец оторван на скорости не ниже SwipeMinVelocity,
//     пройдя не меньше SwipeMinDistance вдоль одной из осей. Приходит ПОСЛЕ Ended
//     того Pan, из которого вырос: приложение, которому нужно только листание,
//     слушает один Swipe, а которому нужна прокрутка с инерцией — Pan и Swipe вместе.
//   - Pinch — два пальца: расстояние между ними (Scale — относительно начала, Step
//     — относительно прошлого события) и середина между ними (Center, CenterDelta).
//     Начинается, когда расстояние изменилось на PinchSlop или середина уехала на
//     PanSlop, поэтому двухпальцевая прокрутка без масштаба тоже приходит как Pinch
//     со Scale около единицы.
//
// Когда пальцев больше одного, Pan прекращается (Ended), а оставшийся после щипка палец
// жестов уже не порождает: он «мёртв» до отрыва последнего, иначе картинка прыгнула
// бы в момент, когда человек разжимает щипок. Третий палец жесты не меняет.

import (
	"image"
	"math"
	"time"
)

// GestureConfig — пороги распознавателя. Расстояния и скорости — в ЛОГИЧЕСКИХ
// пикселях (движок отдаёт виджетам координаты уже логические, так что на HiDPI
// палец по-прежнему «десять точек», а не «десять пикселей матрицы»). Нулевое
// поле означает значение по умолчанию (DefaultGestureConfig), поэтому
// GestureRecognizer{} работает без настройки, а задать можно один порог.
type GestureConfig struct {
	// TapSlop — на сколько палец может сдвинуться и всё ещё считаться тапом.
	// По умолчанию 10.
	TapSlop int
	// TapMaxDuration — дольше этого касание не тап (долгое нажатие).
	// По умолчанию 300 мс.
	TapMaxDuration time.Duration
	// DoubleTapInterval — наибольший промежуток между отрывом и следующим
	// тапом, чтобы тот считался продолжением серии. По умолчанию 300 мс.
	DoubleTapInterval time.Duration
	// DoubleTapSlop — наибольшее расстояние между тапами серии. По умолчанию 32.
	DoubleTapSlop int
	// PanSlop — на сколько палец должен уйти от точки касания, чтобы это стало
	// перетаскиванием (и перестало быть тапом). По умолчанию 10.
	PanSlop int
	// PinchSlop — на сколько должно измениться расстояние между двумя пальцами,
	// чтобы начался щипок. По умолчанию 10.
	PinchSlop int
	// SwipeMinDistance — наименьший путь свайпа вдоль его оси от точки касания.
	// По умолчанию 48.
	SwipeMinDistance int
	// SwipeMinVelocity — наименьшая скорость отрыва, пикселей в секунду.
	// По умолчанию 350.
	SwipeMinVelocity float64
	// SwipeVelocityWindow — по какому отрезку перед отрывом считается скорость.
	// Палец, остановившийся перед отрывом, свайпа не даёт. По умолчанию 100 мс.
	SwipeVelocityWindow time.Duration
	// SwipeAxisRatio — во сколько раз скорость вдоль главной оси должна
	// превосходить скорость поперёк, иначе жест слишком «косой» и свайпом не
	// считается. По умолчанию 1.5 (угол от оси не больше примерно 34°).
	SwipeAxisRatio float64
}

// DefaultGestureConfig — пороги по умолчанию.
func DefaultGestureConfig() GestureConfig {
	return GestureConfig{
		TapSlop:             10,
		TapMaxDuration:      300 * time.Millisecond,
		DoubleTapInterval:   300 * time.Millisecond,
		DoubleTapSlop:       32,
		PanSlop:             10,
		PinchSlop:           10,
		SwipeMinDistance:    48,
		SwipeMinVelocity:    350,
		SwipeVelocityWindow: 100 * time.Millisecond,
		SwipeAxisRatio:      1.5,
	}
}

// withDefaults заменяет нулевые поля значениями по умолчанию.
func (c GestureConfig) withDefaults() GestureConfig {
	d := DefaultGestureConfig()
	if c.TapSlop > 0 {
		d.TapSlop = c.TapSlop
	}
	if c.TapMaxDuration > 0 {
		d.TapMaxDuration = c.TapMaxDuration
	}
	if c.DoubleTapInterval > 0 {
		d.DoubleTapInterval = c.DoubleTapInterval
	}
	if c.DoubleTapSlop > 0 {
		d.DoubleTapSlop = c.DoubleTapSlop
	}
	if c.PanSlop > 0 {
		d.PanSlop = c.PanSlop
	}
	if c.PinchSlop > 0 {
		d.PinchSlop = c.PinchSlop
	}
	if c.SwipeMinDistance > 0 {
		d.SwipeMinDistance = c.SwipeMinDistance
	}
	if c.SwipeMinVelocity > 0 {
		d.SwipeMinVelocity = c.SwipeMinVelocity
	}
	if c.SwipeVelocityWindow > 0 {
		d.SwipeVelocityWindow = c.SwipeVelocityWindow
	}
	if c.SwipeAxisRatio > 0 {
		d.SwipeAxisRatio = c.SwipeAxisRatio
	}
	return d
}

// GesturePhase — этап протяжённого жеста (Pan, Pinch).
type GesturePhase int

const (
	// GestureBegan — жест начался (порог пройден).
	GestureBegan GesturePhase = iota
	// GestureChanged — жест продолжается.
	GestureChanged
	// GestureEnded — жест завершён пальцами.
	GestureEnded
	// GestureCancelled — жест отменён (системой или потерей касания): итог надо
	// откатить, а не принять.
	GestureCancelled
)

// Gesture — результат распознавания: Tap, Pan, Pinch или Swipe.
type Gesture interface{ isGesture() }

// Tap — короткое касание.
type Tap struct {
	X, Y int
	// Count — номер тапа в серии: 1 — одиночный, 2 — двойной и так далее.
	Count int
}

// Pan — перетаскивание одним пальцем.
type Pan struct {
	Phase GesturePhase
	// X, Y — где палец сейчас.
	X, Y int
	// Delta — сдвиг с прошлого события жеста (на GestureBegan — от точки
	// касания, то есть путь, пройденный до порога, не теряется).
	Delta image.Point
	// Total — сдвиг от точки касания.
	Total image.Point
}

// Pinch — два пальца: масштаб и середина.
type Pinch struct {
	Phase GesturePhase
	// Scale — расстояние между пальцами относительно начала касания двумя
	// пальцами: больше 1 — разводят, меньше 1 — сводят.
	Scale float64
	// Step — то же относительно прошлого события жеста: масштаб изображения
	// удобно домножать на Step, а не пересчитывать от Scale.
	Step float64
	// Center — середина между пальцами; CenterDelta — её сдвиг с прошлого
	// события (на GestureBegan — от середины в момент второго касания).
	Center, CenterDelta image.Point
}

// SwipeDir — направление свайпа.
type SwipeDir int

const (
	SwipeLeft SwipeDir = iota
	SwipeRight
	SwipeUp
	SwipeDown
)

// String — имя направления.
func (d SwipeDir) String() string {
	switch d {
	case SwipeLeft:
		return "Left"
	case SwipeRight:
		return "Right"
	case SwipeUp:
		return "Up"
	case SwipeDown:
		return "Down"
	}
	return "SwipeDir?"
}

// Swipe — быстрый мазок одним пальцем.
type Swipe struct {
	Dir SwipeDir
	// Velocity — скорость отрыва вдоль Dir, пикселей в секунду (всегда > 0);
	// VX, VY — вектор скорости со знаком (вправо и вниз положительны).
	Velocity float64
	VX, VY   float64
	// Start, End — где палец коснулся и где оторван.
	Start, End image.Point
}

func (Tap) isGesture()   {}
func (Pan) isGesture()   {}
func (Pinch) isGesture() {}
func (Swipe) isGesture() {}

// gMode — состояние распознавателя.
type gMode int

const (
	gIdle   gMode = iota // пальцев нет
	gSingle              // один палец, пока не тап и не перетаскивание
	gPan                 // один палец тянет
	gMulti               // два пальца (щипок)
	gDead                // жест кончился, ждём отрыва остальных пальцев
)

// gSample — точка траектории пальца для расчёта скорости.
type gSample struct {
	t    time.Duration
	x, y int
}

// gTouch — палец, известный распознавателю.
type gTouch struct {
	id     int
	sx, sy int           // точка касания
	st     time.Duration // момент касания
	x, y   int           // текущее положение
	hist   []gSample
}

// maxGestureSamples ограничивает траекторию: скорости хватает последних
// десятков отсчётов, а залипший палец не должен расти памятью бесконечно.
const maxGestureSamples = 64

// minVelocityDt — наименьший отрезок времени для скорости: два отсчёта в
// пределах пары миллисекунд дали бы астрономическую скорость из шума.
const minVelocityDt = 20 * time.Millisecond

// GestureRecognizer превращает поток TouchEvent в жесты. Нулевое значение
// готово к работе с порогами по умолчанию. Не потокобезопасен: события подаёт
// одна горутина (обработчик виджета на горутине движка).
type GestureRecognizer struct {
	// Config — пороги; нулевые поля берутся по умолчанию.
	Config GestureConfig

	touches []*gTouch
	mode    gMode

	// Перетаскивание: где палец был при последнем отданном событии.
	panLast image.Point

	// Щипок: пальцы, начальные и прошлые значения.
	pinchA, pinchB int
	pinchD0        float64
	pinchC0        image.Point
	pinchStarted   bool
	pinchPrevD     float64
	pinchPrevC     image.Point
	pinchScale     float64

	// Серия тапов.
	hasTap     bool
	tapT       time.Duration
	tapX, tapY int
	tapCount   int
}

// Reset забывает все касания и серию тапов (например, когда виджет скрыт и
// конец касаний до него не дойдёт).
func (r *GestureRecognizer) Reset() {
	r.touches = r.touches[:0]
	r.mode = gIdle
	r.pinchStarted = false
	r.hasTap = false
}

// Active — сколько пальцев сейчас известно распознавателю.
func (r *GestureRecognizer) Active() int { return len(r.touches) }

func (r *GestureRecognizer) find(id int) *gTouch {
	for _, t := range r.touches {
		if t.id == id {
			return t
		}
	}
	return nil
}

func (r *GestureRecognizer) remove(id int) {
	for i, t := range r.touches {
		if t.id == id {
			r.touches = append(r.touches[:i], r.touches[i+1:]...)
			break
		}
	}
	if len(r.touches) == 0 {
		r.mode = gIdle
		r.pinchStarted = false
	}
}

// Feed принимает событие касания и возвращает жесты, которые из него
// получились (чаще всего ни одного). Срез принадлежит вызывающему.
func (r *GestureRecognizer) Feed(ev TouchEvent) []Gesture {
	cfg := r.Config.withDefaults()
	var out []Gesture
	switch ev.Phase {
	case TouchBegan:
		if r.find(ev.ID) != nil {
			// Номер повторился без отрыва: прежний палец потерян.
			out = r.finish(TouchEvent{ID: ev.ID, X: ev.X, Y: ev.Y, Phase: TouchCancelled, Time: ev.Time}, cfg, out)
		}
		out = r.began(ev, cfg, out)
	case TouchMoved:
		out = r.moved(ev, cfg, out)
	case TouchEnded, TouchCancelled:
		out = r.finish(ev, cfg, out)
	}
	return out
}

func (r *GestureRecognizer) began(ev TouchEvent, cfg GestureConfig, out []Gesture) []Gesture {
	t := &gTouch{id: ev.ID, sx: ev.X, sy: ev.Y, st: ev.Time, x: ev.X, y: ev.Y}
	t.hist = append(t.hist, gSample{ev.Time, ev.X, ev.Y})
	r.touches = append(r.touches, t)
	switch len(r.touches) {
	case 1:
		r.mode = gSingle
	case 2:
		switch r.mode {
		case gPan:
			// Второй палец: перетаскивание кончилось, начинается щипок.
			first := r.touches[0]
			out = append(out, Pan{Phase: GestureEnded, X: first.x, Y: first.y,
				Total: image.Pt(first.x-first.sx, first.y-first.sy)})
			r.startPinch()
		case gSingle:
			r.startPinch()
		}
	}
	return out
}

// startPinch запоминает исходное расстояние и середину двух первых пальцев.
func (r *GestureRecognizer) startPinch() {
	a, b := r.touches[0], r.touches[1]
	r.mode = gMulti
	r.pinchA, r.pinchB = a.id, b.id
	r.pinchD0 = math.Max(dist(a.x, a.y, b.x, b.y), 1)
	r.pinchC0 = mid(a, b)
	r.pinchStarted = false
	r.pinchPrevD = r.pinchD0
	r.pinchPrevC = r.pinchC0
	r.pinchScale = 1
}

func (r *GestureRecognizer) moved(ev TouchEvent, cfg GestureConfig, out []Gesture) []Gesture {
	t := r.find(ev.ID)
	if t == nil {
		return out
	}
	t.x, t.y = ev.X, ev.Y
	t.addSample(ev.Time, ev.X, ev.Y, cfg)

	switch r.mode {
	case gSingle:
		if dist(t.sx, t.sy, t.x, t.y) > float64(cfg.PanSlop) {
			r.mode = gPan
			r.panLast = image.Pt(t.x, t.y)
			total := image.Pt(t.x-t.sx, t.y-t.sy)
			out = append(out, Pan{Phase: GestureBegan, X: t.x, Y: t.y, Delta: total, Total: total})
		}
	case gPan:
		cur := image.Pt(t.x, t.y)
		if cur != r.panLast {
			out = append(out, Pan{Phase: GestureChanged, X: t.x, Y: t.y,
				Delta: cur.Sub(r.panLast), Total: image.Pt(t.x-t.sx, t.y-t.sy)})
			r.panLast = cur
		}
	case gMulti:
		if ev.ID != r.pinchA && ev.ID != r.pinchB {
			return out
		}
		a, b := r.find(r.pinchA), r.find(r.pinchB)
		if a == nil || b == nil {
			return out
		}
		d := dist(a.x, a.y, b.x, b.y)
		c := mid(a, b)
		if !r.pinchStarted {
			if math.Abs(d-r.pinchD0) < float64(cfg.PinchSlop) &&
				dist(c.X, c.Y, r.pinchC0.X, r.pinchC0.Y) < float64(cfg.PanSlop) {
				return out
			}
			r.pinchStarted = true
			r.pinchScale = d / r.pinchD0
			out = append(out, Pinch{Phase: GestureBegan, Scale: r.pinchScale,
				Step: r.pinchScale, Center: c, CenterDelta: c.Sub(r.pinchC0)})
			r.pinchPrevD, r.pinchPrevC = d, c
			return out
		}
		if d == r.pinchPrevD && c == r.pinchPrevC {
			return out
		}
		step := 1.0
		if r.pinchPrevD > 0 {
			step = d / r.pinchPrevD
		}
		r.pinchScale = d / r.pinchD0
		out = append(out, Pinch{Phase: GestureChanged, Scale: r.pinchScale, Step: step,
			Center: c, CenterDelta: c.Sub(r.pinchPrevC)})
		r.pinchPrevD, r.pinchPrevC = d, c
	}
	return out
}

func (r *GestureRecognizer) finish(ev TouchEvent, cfg GestureConfig, out []Gesture) []Gesture {
	t := r.find(ev.ID)
	if t == nil {
		return out
	}
	cancelled := ev.Phase == TouchCancelled
	t.x, t.y = ev.X, ev.Y
	t.addSample(ev.Time, ev.X, ev.Y, cfg)
	phase := GestureEnded
	if cancelled {
		phase = GestureCancelled
	}

	switch r.mode {
	case gSingle:
		if !cancelled {
			if tap, ok := r.tap(t, ev.Time, cfg); ok {
				out = append(out, tap)
			}
		}
	case gPan:
		out = append(out, Pan{Phase: phase, X: t.x, Y: t.y,
			Total: image.Pt(t.x-t.sx, t.y-t.sy)})
		if !cancelled {
			if sw, ok := swipeOf(t, ev.Time, cfg); ok {
				out = append(out, sw)
			}
		}
	case gMulti:
		if (ev.ID == r.pinchA || ev.ID == r.pinchB) && r.pinchStarted {
			out = append(out, Pinch{Phase: phase, Scale: r.pinchScale, Step: 1, Center: r.pinchPrevC})
			r.pinchStarted = false
		}
		if ev.ID == r.pinchA || ev.ID == r.pinchB {
			r.mode = gDead
		}
	}
	r.remove(ev.ID)
	return out
}

// tap проверяет, что завершившееся касание — тап, и ведёт счёт серии.
func (r *GestureRecognizer) tap(t *gTouch, now time.Duration, cfg GestureConfig) (Tap, bool) {
	if dist(t.sx, t.sy, t.x, t.y) > float64(cfg.TapSlop) {
		return Tap{}, false
	}
	if now-t.st > cfg.TapMaxDuration {
		return Tap{}, false
	}
	count := 1
	if r.hasTap && now > 0 && now-r.tapT >= 0 && now-r.tapT <= cfg.DoubleTapInterval &&
		dist(t.x, t.y, r.tapX, r.tapY) <= float64(cfg.DoubleTapSlop) {
		count = r.tapCount + 1
	}
	r.hasTap, r.tapT, r.tapX, r.tapY, r.tapCount = true, now, t.x, t.y, count
	return Tap{X: t.x, Y: t.y, Count: count}, true
}

// addSample дописывает отсчёт траектории и отбрасывает устаревшие.
func (t *gTouch) addSample(at time.Duration, x, y int, cfg GestureConfig) {
	t.hist = append(t.hist, gSample{at, x, y})
	// Старше двух окон скорости отсчёты не нужны.
	cut := at - 2*cfg.SwipeVelocityWindow
	drop := 0
	for drop < len(t.hist)-1 && t.hist[drop].t < cut {
		drop++
	}
	if over := len(t.hist) - drop - maxGestureSamples; over > 0 {
		drop += over
	}
	if drop > 0 {
		t.hist = append(t.hist[:0], t.hist[drop:]...)
	}
}

// swipeOf решает, был ли завершившийся Pan свайпом.
func swipeOf(t *gTouch, now time.Duration, cfg GestureConfig) (Swipe, bool) {
	if len(t.hist) < 2 || now <= 0 {
		return Swipe{}, false
	}
	last := t.hist[len(t.hist)-1]
	from := -1
	for i := 0; i < len(t.hist)-1; i++ {
		if last.t-t.hist[i].t <= cfg.SwipeVelocityWindow {
			from = i
			break
		}
	}
	if from < 0 {
		return Swipe{}, false // палец стоял перед отрывом
	}
	dt := last.t - t.hist[from].t
	if dt < minVelocityDt {
		dt = minVelocityDt
	}
	sec := dt.Seconds()
	vx := float64(last.x-t.hist[from].x) / sec
	vy := float64(last.y-t.hist[from].y) / sec

	ax, ay := math.Abs(vx), math.Abs(vy)
	horizontal := ax >= ay
	major, minor := ax, ay
	if !horizontal {
		major, minor = ay, ax
	}
	if major < cfg.SwipeMinVelocity || major < cfg.SwipeAxisRatio*minor {
		return Swipe{}, false
	}
	dx, dy := t.x-t.sx, t.y-t.sy
	var dir SwipeDir
	var travel int
	if horizontal {
		travel = dx
		dir = SwipeRight
		if vx < 0 {
			dir = SwipeLeft
		}
		if (vx < 0) != (dx < 0) {
			return Swipe{}, false // отрыв в сторону, обратную пройденному пути
		}
	} else {
		travel = dy
		dir = SwipeDown
		if vy < 0 {
			dir = SwipeUp
		}
		if (vy < 0) != (dy < 0) {
			return Swipe{}, false
		}
	}
	if travel < 0 {
		travel = -travel
	}
	if travel < cfg.SwipeMinDistance {
		return Swipe{}, false
	}
	return Swipe{Dir: dir, Velocity: major, VX: vx, VY: vy,
		Start: image.Pt(t.sx, t.sy), End: image.Pt(t.x, t.y)}, true
}

func dist(x0, y0, x1, y1 int) float64 {
	return math.Hypot(float64(x1-x0), float64(y1-y0))
}

// mid — середина между двумя пальцами, с округлением к ближайшему.
func mid(a, b *gTouch) image.Point {
	return image.Pt(int(math.Floor(float64(a.x+b.x)/2+0.5)), int(math.Floor(float64(a.y+b.y)/2+0.5)))
}

// TouchGestures — готовый TouchHandler: подаёт касания распознавателю и
// вызывает OnGesture для каждого жеста. Встраивается в виджет, который хочет
// жесты, но не хочет возиться с касаниями:
//
//	type Viewer struct {
//	    widget.Base
//	    widget.TouchGestures
//	}
//	v.OnGesture = func(g widget.Gesture) { switch g := g.(type) { case widget.Pinch: … } }
//
// Всегда принимает касание (возвращает true на любой фазе).
type TouchGestures struct {
	Recognizer GestureRecognizer
	OnGesture  func(Gesture)
}

// OnTouch реализует TouchHandler.
func (h *TouchGestures) OnTouch(ev TouchEvent) bool {
	for _, g := range h.Recognizer.Feed(ev) {
		if h.OnGesture != nil {
			h.OnGesture(g)
		}
	}
	return true
}
