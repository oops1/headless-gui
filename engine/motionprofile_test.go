package engine

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// Профиль движения движка доходит до обработчиков ввода, вызванных с чужой
// горутины, и до Post; горутина ввода после события снова ничья.
func TestMotionProfile_InputAndPost(t *testing.T) {
	testreset.Globals(t)
	eng := New(200, 100, 20)
	defer eng.Stop()
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 200, 100))
	btn := widget.NewButton("x")
	btn.SetBounds(image.Rect(10, 10, 90, 40))
	var dur time.Duration
	var prof MotionProfile
	btn.OnClick = func() {
		dur = widget.MotionDur(time.Second)
		prof = widget.CurrentMotionProfile()
	}
	root.AddChild(btn)
	eng.SetRoot(root)
	click := func() {
		eng.SendMouseButton(20, 20, widget.MouseLeft, true)
		eng.SendMouseButton(20, 20, widget.MouseLeft, false)
	}

	if eng.MotionProfile() != MotionFull {
		t.Fatalf("профиль по умолчанию %v", eng.MotionProfile())
	}
	eng.SetMotionProfile(MotionReduced)
	click()
	if dur != 0 || prof != MotionReduced {
		t.Fatalf("обработчик при MotionReduced: MotionDur %v, профиль %v", dur, prof)
	}
	if p := widget.CurrentMotionProfile(); p != MotionFull {
		t.Fatalf("горутина ввода осталась за движком: %v", p)
	}

	eng.SetMotionProfile(MotionFull)
	click()
	if dur != time.Second || prof != MotionFull {
		t.Fatalf("обработчик при MotionFull: MotionDur %v, профиль %v", dur, prof)
	}

	eng.Start()
	eng.SetMotionProfile(MotionOff)
	ch := make(chan MotionProfile, 1)
	eng.Post(func() { ch <- widget.CurrentMotionProfile() })
	select {
	case p := <-ch:
		if p != MotionOff {
			t.Fatalf("Post при MotionOff видит %v", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Post не выполнился")
	}

	eng.SetMotionProfile(MotionProfile(200))
	if eng.MotionProfile() != MotionFull {
		t.Fatalf("неизвестный профиль стал %v, ждал MotionFull", eng.MotionProfile())
	}
}

// Два движка — два профиля: цикл каждого видит свой.
func TestMotionProfile_PerEngine(t *testing.T) {
	testreset.Globals(t)
	a := New(100, 100, 20)
	defer a.Stop()
	b := New(100, 100, 20)
	defer b.Stop()
	a.Start()
	b.Start()
	a.SetMotionProfile(MotionRemote)
	pa, pb := make(chan MotionProfile, 1), make(chan MotionProfile, 1)
	a.Post(func() { pa <- widget.CurrentMotionProfile() })
	b.Post(func() { pb <- widget.CurrentMotionProfile() })
	for name, ch := range map[string]chan MotionProfile{"a": pa, "b": pb} {
		select {
		case p := <-ch:
			want := MotionFull
			if name == "a" {
				want = MotionRemote
			}
			if p != want {
				t.Errorf("движок %s: профиль %v, ждал %v", name, p, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("Post движка %s не выполнился", name)
		}
	}
}

// Профиль виден Draw синхронного кадра и очереди Post незапущенного движка —
// обе идут на горутине вызывающего.
func TestMotionProfile_SyncRenderAndPost(t *testing.T) {
	testreset.Globals(t)
	eng := New(100, 100, 20)
	defer eng.Stop()
	probe := &profileProbe{}
	probe.SetBounds(image.Rect(0, 0, 100, 100))
	eng.SetRoot(probe)
	eng.SetMotionProfile(MotionReduced)
	var posted MotionProfile = 255
	eng.Post(func() { posted = widget.CurrentMotionProfile() })
	eng.RenderOnce()
	if posted != MotionReduced {
		t.Fatalf("Post незапущенного движка видит %v", posted)
	}
	if probe.seen != MotionReduced {
		t.Fatalf("Draw синхронного кадра видит %v", probe.seen)
	}
	if p := widget.CurrentMotionProfile(); p != MotionFull {
		t.Fatalf("после кадра горутина осталась за движком: %v", p)
	}
}

type profileProbe struct {
	widget.Base
	seen MotionProfile
}

func (p *profileProbe) Draw(widget.DrawContext) { p.seen = widget.CurrentMotionProfile() }

// Упавший обработчик не оставляет горутину ввода за движком — ни при
// изоляции сбоя, ни при FailurePropagate.
func TestMotionProfile_PanicUnbinds(t *testing.T) {
	testreset.Globals(t)
	for _, pol := range []FailurePolicy{FailureIsolate, FailurePropagate} {
		eng := New(100, 100, 20)
		eng.SetFailurePolicy(pol)
		eng.SetOnPanic(func(PanicInfo) {}) // паника ожидаемая — без стека в журнал
		root := widget.NewPanel(color.RGBA{A: 255})
		root.ShowHeader = false
		root.SetBounds(image.Rect(0, 0, 100, 100))
		btn := widget.NewButton("x")
		btn.SetBounds(image.Rect(10, 10, 90, 40))
		btn.OnClick = func() { panic("click") }
		root.AddChild(btn)
		eng.SetRoot(root)
		eng.SetMotionProfile(MotionOff)
		func() {
			defer func() { recover() }()
			eng.SendMouseButton(20, 20, widget.MouseLeft, true)
			eng.SendMouseButton(20, 20, widget.MouseLeft, false)
		}()
		if p := widget.CurrentMotionProfile(); p != MotionFull {
			t.Errorf("политика %v: после паники горутина за движком (%v)", pol, p)
		}
		eng.Stop()
	}
}

// Ввод, посланный движку B из Post движка A, видит профиль B, а не A; после
// него горутина цикла A снова за A.
func TestMotionProfile_CrossEngineInput(t *testing.T) {
	testreset.Globals(t)
	a := New(100, 100, 20)
	defer a.Stop()
	b := New(100, 100, 20)
	defer b.Stop()
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 100, 100))
	btn := widget.NewButton("x")
	btn.SetBounds(image.Rect(10, 10, 90, 40))
	var inB MotionProfile = 255
	btn.OnClick = func() { inB = widget.CurrentMotionProfile() }
	root.AddChild(btn)
	b.SetRoot(root)
	a.SetMotionProfile(MotionOff)
	a.Start()
	after := make(chan MotionProfile, 1)
	a.Post(func() {
		b.SendMouseButton(20, 20, widget.MouseLeft, true)
		b.SendMouseButton(20, 20, widget.MouseLeft, false)
		after <- widget.CurrentMotionProfile()
	})
	select {
	case p := <-after:
		if inB != MotionFull {
			t.Errorf("обработчик движка B видит %v, ждал MotionFull", inB)
		}
		if p != MotionOff {
			t.Errorf("после ввода в B цикл A видит %v, ждал MotionOff", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Post не выполнился")
	}
}

// Stop выводит движок из счёта: привязка чужих горутин снова не нужна.
func TestMotionProfile_StopUncounts(t *testing.T) {
	testreset.Globals(t)
	n0 := motionEngines.Load()
	eng := New(50, 50, 20)
	eng.SetMotionProfile(MotionRemote)
	eng.SetMotionProfile(MotionOff)
	if motionEngines.Load() != n0+1 {
		t.Fatalf("счёт %d, ждал %d", motionEngines.Load(), n0+1)
	}
	eng.Stop()
	if motionEngines.Load() != n0 {
		t.Fatalf("после Stop счёт %d, ждал %d", motionEngines.Load(), n0)
	}
	eng.SetMotionProfile(MotionReduced)
	if motionEngines.Load() != n0 || eng.MotionProfile() != MotionReduced {
		t.Fatalf("после Stop: счёт %d, профиль %v", motionEngines.Load(), eng.MotionProfile())
	}
}
