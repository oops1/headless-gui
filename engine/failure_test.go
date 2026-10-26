package engine

import (
	"image"
	"image/color"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// panicker — виджет, который падает в Draw, пока flag установлен.
type panicker struct {
	widget.Base
	explode atomic.Bool
	draws   atomic.Int32
}

func (p *panicker) Draw(ctx widget.DrawContext) {
	p.draws.Add(1)
	if p.explode.Load() {
		panic("panicker: Draw")
	}
	r := p.Bounds()
	ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{G: 200, A: 255})
}

// failureScene — движок с корнем-панелью, паникёром в ней и накопителем
// PanicInfo.
type failureScene struct {
	eng  *Engine
	root *widget.Panel
	p    *panicker
	mu   sync.Mutex
	got  []PanicInfo
}

func newFailureScene(t *testing.T) *failureScene {
	t.Helper()
	eng := New(200, 120, 30)
	t.Cleanup(eng.Stop)
	eng.SetFailurePolicy(FailureIsolate)
	root := widget.NewPanel(color.RGBA{R: 40, G: 80, B: 120, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 200, 120))
	p := &panicker{}
	p.SetBounds(image.Rect(20, 20, 120, 80))
	root.AddChild(p)
	eng.SetRoot(root)
	sc := &failureScene{eng: eng, root: root, p: p}
	eng.SetOnPanic(func(info PanicInfo) {
		sc.mu.Lock()
		sc.got = append(sc.got, info)
		sc.mu.Unlock()
	})
	t.Cleanup(func() { widget.ClearBroken(p) })
	return sc
}

func (sc *failureScene) infos() []PanicInfo {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return append([]PanicInfo(nil), sc.got...)
}

// Под go test политика по умолчанию — прежняя: паники идут дальше.
func TestFailure_DefaultUnderTestIsPropagate(t *testing.T) {
	testreset.Globals(t)
	eng := New(50, 50, 30)
	defer eng.Stop()
	if got := eng.FailurePolicy(); got != FailurePropagate {
		t.Fatalf("политика под go test: %v, ждали propagate", got)
	}
	if defaultFailurePolicy() != FailurePropagate {
		t.Fatal("defaultFailurePolicy под go test не propagate")
	}
}

// При FailurePropagate паника из Draw доходит до вызывающего.
func TestFailure_DrawPanicPropagates(t *testing.T) {
	testreset.Globals(t)
	sc := newFailureScene(t)
	sc.eng.SetFailurePolicy(FailurePropagate)
	sc.p.explode.Store(true)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("паника не дошла до вызывающего")
		}
	}()
	sc.eng.RenderOnce()
}

// Паника в Draw: процесс жив, первая паника включает учёт, вторая называет
// виновника и помечает его, третий кадр рисует заглушку без паники.
func TestFailure_DrawPanicIsolated(t *testing.T) {
	testreset.Globals(t)
	sc := newFailureScene(t)
	eng := sc.eng
	sc.p.explode.Store(true)

	eng.RenderOnce() // первая: виновник неизвестен, учёт включается
	infos := sc.infos()
	if len(infos) != 1 || infos[0].Where != "Draw" || infos[0].Widget != nil {
		t.Fatalf("после первой паники: %+v", infos)
	}
	if widget.DrawTraceUsers() == 0 {
		t.Fatal("после первой паники учёт рисующихся не включён")
	}

	eng.RenderOnce() // вторая: виновник назван
	infos = sc.infos()
	if len(infos) != 2 || infos[1].Widget != sc.p || !strings.Contains(infos[1].Path, "panicker") {
		t.Fatalf("после второй паники: %+v", infos)
	}
	if !widget.IsBroken(sc.p) {
		t.Fatal("виновник не помечен сломанным")
	}
	if widget.DrawTraceUsers() != 0 {
		t.Fatal("учёт, включённый ради поиска виновника, не снят")
	}

	draws := sc.p.draws.Load()
	img := eng.RenderOnce() // третья: заглушка, Draw не зовётся
	if sc.p.draws.Load() != draws {
		t.Fatal("Draw сломанного виджета вызван")
	}
	if len(sc.infos()) != 2 {
		t.Fatalf("третий кадр дал панику: %+v", sc.infos())
	}
	if c := img.RGBAAt(25, 50); c != brokenFillColor() {
		t.Fatalf("на месте виджета не заглушка: %v", c)
	}
	if s := eng.Stats(); s.Panics != 2 {
		t.Fatalf("Stats.Panics = %d, ждали 2", s.Panics)
	}

	// Починили: виджет снова рисуется сам.
	sc.p.explode.Store(false)
	widget.ClearBroken(sc.p)
	eng.Invalidate()
	img = eng.RenderOnce()
	if c := img.RGBAAt(25, 50); c.G != 200 {
		t.Fatalf("после ClearBroken виджет не нарисован: %v", c)
	}
}

func brokenFillColor() color.RGBA { return color.RGBA{R: 96, G: 96, B: 96, A: 255} }

// Паника в Draw при включённой проверке контракта называет виновника сразу.
func TestFailure_DrawPanicWithTraceOn(t *testing.T) {
	testreset.Globals(t)
	sc := newFailureScene(t)
	sc.eng.SetDrawContractCheck(true)
	sc.p.explode.Store(true)
	sc.eng.RenderOnce()
	infos := sc.infos()
	if len(infos) != 1 || infos[0].Widget != sc.p {
		t.Fatalf("виновник не назван с первого раза: %+v", infos)
	}
	if !sc.eng.DrawContractCheck() {
		t.Fatal("проверка контракта выключена чужой паникой")
	}
}

// Остановка движка снимает учёт, включённый ради поиска виновника.
func TestFailure_StopReleasesTrace(t *testing.T) {
	testreset.Globals(t)
	sc := newFailureScene(t)
	sc.p.explode.Store(true)
	sc.eng.RenderOnce()
	if widget.DrawTraceUsers() == 0 {
		t.Fatal("учёт не включён")
	}
	sc.eng.Stop()
	if n := widget.DrawTraceUsers(); n != 0 {
		t.Fatalf("после Stop учёт держат %d", n)
	}
}

// Паника в функции из Post не мешает остальным функциям очереди.
func TestFailure_PostPanicIsolated(t *testing.T) {
	testreset.Globals(t)
	sc := newFailureScene(t)
	ran := false
	sc.eng.Post(func() { panic("post") })
	sc.eng.Post(func() { ran = true })
	sc.eng.RenderOnce()
	if !ran {
		t.Fatal("функция после упавшей не выполнена")
	}
	infos := sc.infos()
	if len(infos) != 1 || infos[0].Where != "Post" {
		t.Fatalf("%+v", infos)
	}
}

// Паника в таймере: Every продолжает идти.
func TestFailure_TimerPanicIsolated(t *testing.T) {
	testreset.Globals(t)
	sc := newFailureScene(t)
	eng := sc.eng
	var fired atomic.Int32
	eng.Every(2*time.Millisecond, func() {
		if fired.Add(1) == 1 {
			panic("timer")
		}
	})
	eng.Start()
	deadline := time.Now().Add(3 * time.Second)
	for fired.Load() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("Every после паники сработал %d раз", fired.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if infos := sc.infos(); len(infos) == 0 || infos[0].Where != "Timer" {
		t.Fatalf("%+v", infos)
	}
}

// Паника в тике анимации: следующие анимации этого движка продолжают шагать.
func TestFailure_AnimationPanicIsolated(t *testing.T) {
	testreset.Globals(t)
	sc := newFailureScene(t)
	eng := sc.eng
	eng.Start()
	eng.Post(func() {
		widget.Animate(50*time.Millisecond, nil, func(float64) { panic("tick") })
	})
	deadline := time.Now().Add(3 * time.Second)
	for len(sc.infos()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("паника тика не поймана")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if sc.infos()[0].Where != "Animation" {
		t.Fatalf("%+v", sc.infos())
	}
	var ticks atomic.Int32
	eng.Post(func() {
		widget.Animate(200*time.Millisecond, nil, func(float64) { ticks.Add(1) })
	})
	deadline = time.Now().Add(3 * time.Second)
	for ticks.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("анимации после паники замерли: тиков %d", ticks.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Паника в обработчике нажатия: захват мыши снят, ввод работает дальше.
func TestFailure_InputPanicIsolated(t *testing.T) {
	testreset.Globals(t)
	sc := newFailureScene(t)
	eng := sc.eng
	btn := widget.NewButton("boom")
	btn.SetBounds(image.Rect(130, 20, 190, 50))
	sc.root.AddChild(btn)
	clicks := 0
	btn.OnClick = func() {
		clicks++
		if clicks == 1 {
			panic("click")
		}
	}
	eng.SetCapture(btn)
	eng.SendMouseButton(160, 35, widget.MouseLeft, true)
	eng.SendMouseButton(160, 35, widget.MouseLeft, false)
	if eng.getCaptured() != nil {
		t.Fatal("захват мыши остался после паники")
	}
	infos := sc.infos()
	if len(infos) != 1 || infos[0].Where != "SendMouseButton" {
		t.Fatalf("%+v", infos)
	}
	eng.SendMouseButton(160, 35, widget.MouseLeft, true)
	eng.SendMouseButton(160, 35, widget.MouseLeft, false)
	if clicks != 2 {
		t.Fatalf("после паники нажатие не дошло: clicks=%d", clicks)
	}
}

type panicSink struct{}

func (panicSink) Present(output.Frame) { panic("sink") }

// Паника в стоке кадров: цикл жив, кадр в канал отдан.
func TestFailure_SinkPanicIsolated(t *testing.T) {
	testreset.Globals(t)
	sc := newFailureScene(t)
	sc.eng.SetFrameSink(panicSink{})
	sc.eng.deliver(output.Frame{Seq: 7})
	select {
	case fr := <-sc.eng.Frames():
		if fr.Seq != 7 {
			t.Fatalf("в канал пришёл не тот кадр: %d", fr.Seq)
		}
	default:
		t.Fatal("кадр не дошёл до канала после паники стока")
	}
	if infos := sc.infos(); len(infos) != 1 || infos[0].Where != "FrameSink" {
		t.Fatalf("%+v", infos)
	}
}
