package base

import (
	"image"
	"math"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
)

// Профиль движения цикла (motionprofile.go): ключ цикла сам отвечает своим
// профилем, как движок.

type profLoop struct{ p theme.MotionProfile }

func (l *profLoop) MotionProfile() theme.MotionProfile { return l.p }

// onLoop заводит анимации из fn на цикле key.
func onLoop(key any, fn func()) {
	unbind := BindAnimationLoop(key)
	defer unbind()
	fn()
}

// MotionRemote: переход 300 мс отдаёт тику только 0, 1/3, 2/3 и 1, хотя шаг
// идёт каждые 10 мс; MotionFull на тех же шагах — каждый.
func TestMotionProfile_RemoteQuantizes(t *testing.T) {
	StopAllAnimations()
	defer StopAllAnimations()
	run := func(p theme.MotionProfile) []float64 {
		l := &profLoop{p}
		var got []float64
		onLoop(l, func() {
			Animate(300*time.Millisecond, nil, func(t float64) { got = append(got, t) })
		})
		start := time.Now()
		for ms := 0; ms <= 330; ms += 10 {
			StepAnimationsFor(l, start.Add(time.Duration(ms)*time.Millisecond))
		}
		return got
	}
	got := run(theme.MotionRemote)
	want := []float64{0, 1.0 / 3, 2.0 / 3, 1}
	if len(got) != len(want) {
		t.Fatalf("MotionRemote: тиков %d %v, ждал %v", len(got), got, want)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("MotionRemote: тик %d = %v, ждал %v", i, got[i], want[i])
		}
	}
	if full := run(theme.MotionFull); len(full) < 30 {
		t.Fatalf("MotionFull: тиков %d, ждал на каждом шаге", len(full))
	}
}

// MotionRemote квантует и каждую фазу AutoReverse отдельно.
func TestMotionProfile_RemoteQuantizesReversePhase(t *testing.T) {
	StopAllAnimations()
	defer StopAllAnimations()
	l := &profLoop{theme.MotionRemote}
	var got []float64
	onLoop(l, func() {
		a := Animate(300*time.Millisecond, nil, func(t float64) { got = append(got, t) })
		a.AutoReverse = true
	})
	start := time.Now()
	for ms := 0; ms <= 700; ms += 10 {
		StepAnimationsFor(l, start.Add(time.Duration(ms)*time.Millisecond))
	}
	// Прямая: 0, 1/3, 2/3, 1 (граница); обратная: 1, 2/3, 1/3, 0 (конец).
	want := []float64{0, 1.0 / 3, 2.0 / 3, 1, 1, 2.0 / 3, 1.0 / 3, 0}
	if len(got) != len(want) {
		t.Fatalf("тиков %d %v, ждал %v", len(got), got, want)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("тик %d = %v, ждал %v (%v)", i, got[i], want[i], got)
		}
	}
}

// MotionOff: незацикленная анимация с тиком кончается на первом шаге (с
// OnDone), AutoReverse — в начальном состоянии; зациклённая и таймер идут.
func TestMotionProfile_OffFinishesAtOnce(t *testing.T) {
	StopAllAnimations()
	defer StopAllAnimations()
	l := &profLoop{theme.MotionOff}
	var plain, rev []float64
	done, timerDone := false, false
	loops := 0
	var lp, tm *Animation
	onLoop(l, func() {
		a := Animate(time.Hour, EaseOutCubic, func(t float64) { plain = append(plain, t) })
		a.OnDone = func() { done = true }
		r := Animate(time.Hour, nil, func(t float64) { rev = append(rev, t) })
		r.AutoReverse = true
		lp = Animate(time.Hour, nil, func(float64) { loops++ })
		lp.Loop = true
		tm = Animate(time.Hour, nil, nil)
		tm.OnDone = func() { timerDone = true }
	})
	StepAnimationsFor(l, time.Now())
	if len(plain) != 1 || plain[0] != 1 || !done {
		t.Fatalf("обычная: тики %v, OnDone %v — ждал [1] и вызов", plain, done)
	}
	if len(rev) != 1 || rev[0] != 0 {
		t.Fatalf("AutoReverse: тики %v, ждал [0]", rev)
	}
	if loops != 1 || !lp.Running() {
		t.Fatalf("зациклённая: тиков %d, идёт %v — ждал 1 и идёт", loops, lp.Running())
	}
	if timerDone || !tm.Running() {
		t.Fatal("таймер сработал раньше срока")
	}
}

// Профиль подхватывается на лету: идущая анимация доходит до конца на
// следующем шаге после MotionOff.
func TestMotionProfile_OffOnTheFly(t *testing.T) {
	StopAllAnimations()
	defer StopAllAnimations()
	l := &profLoop{theme.MotionFull}
	var got []float64
	onLoop(l, func() {
		Animate(time.Second, nil, func(t float64) { got = append(got, t) })
	})
	now := time.Now()
	StepAnimationsFor(l, now)
	StepAnimationsFor(l, now.Add(100*time.Millisecond))
	l.p = theme.MotionOff
	if StepAnimationsFor(l, now.Add(200*time.Millisecond)) {
		t.Fatal("после MotionOff анимация осталась активной")
	}
	if len(got) != 3 || got[2] != 1 {
		t.Fatalf("тики %v, ждал третий — 1", got)
	}
}

// MotionDur и токены темы видят профиль движка, на горутине которого идёт
// вызов; вне его — прежние длительности.
func TestMotionProfile_DurationsFollowLoop(t *testing.T) {
	tm := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(tm); err != nil {
		t.Fatal(err)
	}
	if err := tm.SetTheme(theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	const deco, fn theme.Key = "menu.open", "notification.expand"
	full := tm.GetAnimation(deco).Duration
	if full <= 0 || tm.GetAnimation(fn).Duration <= 0 {
		t.Fatal("у Windows 11 нет анимаций, проверять нечего")
	}
	// Функциональная при флаге темы — образец для MotionReduced.
	flagged := theme.NewManager()
	_ = theme.RegisterBuiltinProfiles(flagged)
	_ = flagged.SetTheme(theme.ProfileWindows11)
	flagged.SetFlag(theme.FlagMotionReduce, true)
	reducedFn := flagged.GetAnimation(fn).Duration

	l := &profLoop{theme.MotionReduced}
	if MotionDur(time.Second) != time.Second || tm.GetAnimation(deco).Duration != full {
		t.Fatal("вне цикла профиль подействовал")
	}
	unbind := BindAnimationLoop(l)
	defer unbind()

	if d := MotionDur(time.Second); d != 0 {
		t.Errorf("MotionReduced: MotionDur = %v", d)
	}
	if d := tm.GetAnimation(deco).Duration; d != 0 {
		t.Errorf("MotionReduced: декоративная %v", d)
	}
	if d := tm.GetAnimation(fn).Duration; d != reducedFn {
		t.Errorf("MotionReduced: функциональная %v, при флаге темы %v", d, reducedFn)
	}

	l.p = theme.MotionOff
	if d := MotionDur(time.Second); d != 0 {
		t.Errorf("MotionOff: MotionDur = %v", d)
	}
	if d := tm.GetAnimation(fn).Duration; d != 0 {
		t.Errorf("MotionOff: функциональная %v", d)
	}

	l.p = theme.MotionRemote
	if MotionDur(time.Second) != time.Second || tm.GetAnimation(deco).Duration != full {
		t.Error("MotionRemote изменил длительности")
	}
	if d := tm.GetAnimationRaw(deco).Duration; d != full {
		t.Errorf("GetAnimationRaw под профилем: %v", d)
	}
}

// Объявления цикла вкладываются: снятие внутреннего возвращает внешний.
func TestBindAnimationLoop_Nested(t *testing.T) {
	n0 := animLoopCount.Load()
	a, b := &testLoop{"a"}, &testLoop{"b"}
	ua := BindAnimationLoop(a)
	ub := BindAnimationLoop(b)
	if currentAnimLoop() != b {
		t.Fatal("внутреннее объявление не действует")
	}
	ub()
	if currentAnimLoop() != a {
		t.Fatal("снятие внутреннего не вернуло внешний")
	}
	ua()
	if currentAnimLoop() != nil || animLoopCount.Load() != n0 {
		t.Fatalf("после снятия обоих: цикл %v, счётчик %d (был %d)", currentAnimLoop(), animLoopCount.Load(), n0)
	}
}

// Переход на MotionRemote посреди фазы не отбрасывает переход назад: шаг
// ниже пройденного не отдаётся.
func TestMotionProfile_RemoteSwitchMidPhaseMonotonic(t *testing.T) {
	StopAllAnimations()
	defer StopAllAnimations()
	l := &profLoop{theme.MotionFull}
	var got []float64
	onLoop(l, func() {
		Animate(time.Second, nil, func(t float64) { got = append(got, t) })
	})
	start := time.Now()
	StepAnimationsFor(l, start)
	StepAnimationsFor(l, start.Add(600*time.Millisecond))
	l.p = theme.MotionRemote
	for ms := 620; ms <= 1000; ms += 20 {
		StepAnimationsFor(l, start.Add(time.Duration(ms)*time.Millisecond))
	}
	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Fatalf("переход назад после смены профиля: %v", got)
		}
	}
	if got[len(got)-1] != 1 || len(got) != 4 {
		t.Fatalf("тики %v, ждал 0, 0.6, 2/3, 1", got)
	}
}

// Анимация-часы (Clock) профилем не квантуется и до конца не доводится.
func TestMotionProfile_ClockIsUntouched(t *testing.T) {
	StopAllAnimations()
	defer StopAllAnimations()
	for _, p := range []theme.MotionProfile{theme.MotionRemote, theme.MotionOff} {
		l := &profLoop{p}
		var got []float64
		onLoop(l, func() {
			a := Animate(300*time.Millisecond, nil, func(t float64) { got = append(got, t) })
			a.Clock = true
		})
		start := time.Now()
		for ms := 0; ms <= 330; ms += 10 {
			StepAnimationsFor(l, start.Add(time.Duration(ms)*time.Millisecond))
		}
		if len(got) < 30 || got[1] == 1 {
			t.Errorf("профиль %v: часы получили %d тиков (второй %v), ждал на каждом шаге", p, len(got), got[1])
		}
	}
}

// Функциональная анимация при MotionReduced укорачивается до метрики темы, при
// MotionOff — нуль; nil-тема при профиле не падает.
func TestMotionProfile_FunctionalCap(t *testing.T) {
	m := theme.NewManager()
	p := theme.NewProfile("MP")
	p.Anims["mp.expand"] = theme.AnimSpec{Duration: 300 * time.Millisecond}
	p.Anims["mp.fade"] = theme.AnimSpec{Duration: 300 * time.Millisecond}
	p.SetMetric(theme.KeyMotionReduceFunctionalMS, 60)
	if err := m.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("MP"); err != nil {
		t.Fatal(err)
	}
	theme.RegisterAnimationKind("mp.expand", theme.AnimFunctional)
	l := &profLoop{theme.MotionReduced}
	unbind := BindAnimationLoop(l)
	defer unbind()

	if d := m.GetAnimation("mp.expand").Duration; d != 60*time.Millisecond {
		t.Errorf("MotionReduced: функциональная %v, ждал 60 мс", d)
	}
	if d := m.GetAnimation("mp.fade").Duration; d != 0 {
		t.Errorf("MotionReduced: декоративная %v", d)
	}
	if !m.ReducesMotion() || m.MotionReduced() {
		t.Errorf("ReducesMotion %v, MotionReduced %v — ждал профиль, но не флаг", m.ReducesMotion(), m.MotionReduced())
	}
	var nilTheme *theme.Theme
	if d := nilTheme.AnimReduced("mp.expand", theme.AnimSpec{Duration: time.Second}).Duration; d != 0 {
		t.Errorf("nil-тема: %v", d)
	}
	l.p = theme.MotionOff
	if d := m.GetAnimation("mp.expand").Duration; d != 0 {
		t.Errorf("MotionOff: функциональная %v", d)
	}
	l.p = theme.MotionRemote
	if m.ReducesMotion() {
		t.Error("MotionRemote считается уменьшенным движением")
	}
}

// Прокрутка колесом на движке без движения или удалённом — сразу, без
// маховика; маховик, начатый до смены профиля, не проезжает дальше жеста.
func TestMotionProfile_ScrollInertia(t *testing.T) {
	StopAllAnimations()
	defer StopAllAnimations()
	newSV := func() *ScrollView {
		sv := NewScrollView()
		sv.SetBounds(image.Rect(0, 0, 200, 200))
		sv.ContentHeight = 100000
		return sv
	}
	if currentStyle().Classic3D {
		t.Skip("стиль по умолчанию классический — маховика нет")
	}
	for _, p := range []theme.MotionProfile{theme.MotionOff, theme.MotionRemote} {
		sv := newSV()
		onLoop(&profLoop{p}, func() { sv.wheelPixelsY(40) })
		if sv.ScrollY() != 40 {
			t.Errorf("профиль %v: прокрутка %d сразу после жеста, ждал 40", p, sv.ScrollY())
		}
	}

	sv := newSV()
	l := &profLoop{theme.MotionFull}
	onLoop(l, func() { sv.wheelPixelsY(40) })
	if sv.ScrollY() != 0 {
		t.Fatalf("MotionFull: прокрутка %d до первого шага — маховика нет?", sv.ScrollY())
	}
	start := time.Now()
	StepAnimationsFor(l, start)
	StepAnimationsFor(l, start.Add(16*time.Millisecond))
	l.p = theme.MotionOff
	for ms := 32; ms <= 4100; ms += 16 {
		StepAnimationsFor(l, start.Add(time.Duration(ms)*time.Millisecond))
	}
	if y := sv.ScrollY(); y < 30 || y > 50 {
		t.Errorf("маховик после смены профиля проехал %d, ждал около 40", y)
	}
}
