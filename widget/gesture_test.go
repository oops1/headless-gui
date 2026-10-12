package widget

import (
	"image"
	"math"
	"testing"
	"time"
)

// ms — момент в миллисекундах от начала синтетической последовательности.
func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func tev(id, x, y int, ph TouchPhase, at int) TouchEvent {
	return TouchEvent{ID: id, X: x, Y: y, Phase: ph, Time: ms(at)}
}

// feedAll прогоняет последовательность и собирает все жесты по порядку.
func feedAll(r *GestureRecognizer, evs ...TouchEvent) []Gesture {
	var out []Gesture
	for _, e := range evs {
		out = append(out, r.Feed(e)...)
	}
	return out
}

func TestGesture_Tap(t *testing.T) {
	var r GestureRecognizer
	got := feedAll(&r,
		tev(1, 100, 100, TouchBegan, 0),
		tev(1, 103, 102, TouchMoved, 40), // дрожь пальца в пределах порога
		tev(1, 103, 102, TouchEnded, 90),
	)
	if len(got) != 1 {
		t.Fatalf("жестов %d (%v), ждал один Tap", len(got), got)
	}
	tap, ok := got[0].(Tap)
	if !ok || tap.X != 103 || tap.Y != 102 || tap.Count != 1 {
		t.Errorf("получил %+v", got[0])
	}
}

func TestGesture_LongPressIsNotTap(t *testing.T) {
	var r GestureRecognizer
	got := feedAll(&r,
		tev(1, 50, 50, TouchBegan, 0),
		tev(1, 50, 50, TouchEnded, 700),
	)
	if len(got) != 0 {
		t.Errorf("долгое нажатие дало жесты: %v", got)
	}
}

func TestGesture_DoubleTap(t *testing.T) {
	var r GestureRecognizer
	got := feedAll(&r,
		tev(1, 100, 100, TouchBegan, 0),
		tev(1, 100, 100, TouchEnded, 60),
		tev(2, 108, 104, TouchBegan, 180),
		tev(2, 108, 104, TouchEnded, 230),
		// Третий тап — спустя долгую паузу: серия началась заново.
		tev(3, 108, 104, TouchBegan, 1500),
		tev(3, 108, 104, TouchEnded, 1550),
	)
	if len(got) != 3 {
		t.Fatalf("жестов %d (%v)", len(got), got)
	}
	want := []int{1, 2, 1}
	for i, g := range got {
		if tap, ok := g.(Tap); !ok || tap.Count != want[i] {
			t.Errorf("тап %d: %+v, ждал Count=%d", i, g, want[i])
		}
	}
}

func TestGesture_DoubleTapFarApartIsSingle(t *testing.T) {
	var r GestureRecognizer
	got := feedAll(&r,
		tev(1, 100, 100, TouchBegan, 0),
		tev(1, 100, 100, TouchEnded, 50),
		tev(2, 400, 100, TouchBegan, 150),
		tev(2, 400, 100, TouchEnded, 200),
	)
	if len(got) != 2 || got[1].(Tap).Count != 1 {
		t.Errorf("тапы в разных местах слились в серию: %v", got)
	}
}

func TestGesture_Pan(t *testing.T) {
	var r GestureRecognizer
	got := feedAll(&r,
		tev(1, 100, 100, TouchBegan, 0),
		tev(1, 104, 100, TouchMoved, 16), // ещё в пределах порога
		tev(1, 115, 100, TouchMoved, 32), // порог пройден
		tev(1, 130, 105, TouchMoved, 48),
		tev(1, 130, 105, TouchEnded, 400), // долго стоял — свайпа нет
	)
	if len(got) != 3 {
		t.Fatalf("жестов %d (%v)", len(got), got)
	}
	b, ok := got[0].(Pan)
	if !ok || b.Phase != GestureBegan || b.Delta != image.Pt(15, 0) || b.Total != image.Pt(15, 0) {
		t.Errorf("Began: %+v", got[0])
	}
	c := got[1].(Pan)
	if c.Phase != GestureChanged || c.Delta != image.Pt(15, 5) || c.Total != image.Pt(30, 5) {
		t.Errorf("Changed: %+v", c)
	}
	e := got[2].(Pan)
	if e.Phase != GestureEnded || e.Total != image.Pt(30, 5) {
		t.Errorf("Ended: %+v", e)
	}
	// Сумма дельт всех событий равна полному пути.
	sum := b.Delta.Add(c.Delta)
	if sum != image.Pt(30, 5) {
		t.Errorf("сумма Delta %v, ждал (30,5)", sum)
	}
}

func TestGesture_Swipe(t *testing.T) {
	cases := []struct {
		name  string
		dx    int
		dy    int
		want  SwipeDir
		speed float64 // пикселей в секунду
	}{
		{"влево", -200, 10, SwipeLeft, 1800},
		{"вправо", 200, -10, SwipeRight, 1800},
		{"вверх", 8, -200, SwipeUp, 1800},
		{"вниз", -8, 200, SwipeDown, 1800},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var r GestureRecognizer
			// Палец проходит 200 px за 100 мс шагами по 16 мс.
			evs := []TouchEvent{tev(1, 300, 300, TouchBegan, 0)}
			steps := 6
			for i := 1; i <= steps; i++ {
				evs = append(evs, tev(1, 300+c.dx*i/steps, 300+c.dy*i/steps, TouchMoved, 16*i))
			}
			evs = append(evs, tev(1, 300+c.dx, 300+c.dy, TouchEnded, 16*steps+4))
			got := feedAll(&r, evs...)
			sw, ok := got[len(got)-1].(Swipe)
			if !ok {
				t.Fatalf("последний жест %T (%v), ждал Swipe", got[len(got)-1], got)
			}
			if sw.Dir != c.want {
				t.Errorf("направление %v, ждал %v", sw.Dir, c.want)
			}
			if sw.Velocity < 1500 || sw.Velocity > 2500 {
				t.Errorf("скорость %.0f px/s вне ожидаемой ~2000", sw.Velocity)
			}
			if sw.Start != image.Pt(300, 300) || sw.End != image.Pt(300+c.dx, 300+c.dy) {
				t.Errorf("Start/End %v/%v", sw.Start, sw.End)
			}
			// Swipe приходит после завершения Pan.
			if pan, ok := got[len(got)-2].(Pan); !ok || pan.Phase != GestureEnded {
				t.Errorf("перед Swipe ждал Pan Ended, получил %v", got[len(got)-2])
			}
		})
	}
}

func TestGesture_SlowDragIsNotSwipe(t *testing.T) {
	var r GestureRecognizer
	// 200 px за 2 с: скорость 100 px/s, ниже порога.
	var evs []TouchEvent
	evs = append(evs, tev(1, 0, 0, TouchBegan, 0))
	for i := 1; i <= 20; i++ {
		evs = append(evs, tev(1, i*10, 0, TouchMoved, i*100))
	}
	evs = append(evs, tev(1, 200, 0, TouchEnded, 2010))
	for _, g := range feedAll(&r, evs...) {
		if _, ok := g.(Swipe); ok {
			t.Fatal("медленное перетаскивание дало Swipe")
		}
	}
}

func TestGesture_StopBeforeLiftIsNotSwipe(t *testing.T) {
	var r GestureRecognizer
	got := feedAll(&r,
		tev(1, 0, 0, TouchBegan, 0),
		tev(1, 100, 0, TouchMoved, 30), // быстро ушёл…
		tev(1, 200, 0, TouchMoved, 60),
		tev(1, 200, 0, TouchEnded, 400), // …и стоял 340 мс
	)
	for _, g := range got {
		if _, ok := g.(Swipe); ok {
			t.Fatal("палец остановился перед отрывом, а свайп пришёл")
		}
	}
}

func TestGesture_DiagonalIsNotSwipe(t *testing.T) {
	var r GestureRecognizer
	got := feedAll(&r,
		tev(1, 0, 0, TouchBegan, 0),
		tev(1, 60, 50, TouchMoved, 30),
		tev(1, 120, 100, TouchMoved, 60),
		tev(1, 180, 150, TouchEnded, 90),
	)
	for _, g := range got {
		if _, ok := g.(Swipe); ok {
			t.Fatal("диагональ дала Swipe")
		}
	}
}

func TestGesture_ShortFlickIsNotSwipe(t *testing.T) {
	var r GestureRecognizer
	// Быстро, но всего 30 px: меньше SwipeMinDistance.
	got := feedAll(&r,
		tev(1, 0, 0, TouchBegan, 0),
		tev(1, 15, 0, TouchMoved, 10),
		tev(1, 30, 0, TouchEnded, 20),
	)
	for _, g := range got {
		if _, ok := g.(Swipe); ok {
			t.Fatal("короткий мазок дал Swipe")
		}
	}
}

func TestGesture_Pinch(t *testing.T) {
	var r GestureRecognizer
	got := feedAll(&r,
		tev(1, 100, 200, TouchBegan, 0),
		tev(2, 200, 200, TouchBegan, 5), // расстояние 100, середина (150,200)
		tev(1, 98, 200, TouchMoved, 20), // 102: порога нет
		tev(2, 220, 200, TouchMoved, 25),
		tev(1, 80, 200, TouchMoved, 40), // расстояние 140
		tev(2, 220, 200, TouchMoved, 45),
		tev(1, 50, 200, TouchMoved, 60), // расстояние 170
		tev(2, 220, 200, TouchEnded, 80),
	)
	var pinches []Pinch
	for _, g := range got {
		switch g := g.(type) {
		case Pinch:
			pinches = append(pinches, g)
		default:
			t.Errorf("лишний жест %T: %v", g, g)
		}
	}
	if len(pinches) < 3 {
		t.Fatalf("щипков %d: %v", len(pinches), pinches)
	}
	first := pinches[0]
	if first.Phase != GestureBegan || first.Scale < 1.1 {
		t.Errorf("Began: %+v", first)
	}
	// Накопленный масштаб растёт, а произведение Step даёт Scale.
	prod := 1.0
	for _, p := range pinches {
		if p.Phase == GestureEnded {
			continue
		}
		prod *= p.Step
	}
	last := pinches[len(pinches)-2] // перед Ended
	if math.Abs(prod-last.Scale) > 1e-9 {
		t.Errorf("произведение Step %.4f != Scale %.4f", prod, last.Scale)
	}
	if math.Abs(last.Scale-1.7) > 1e-9 {
		t.Errorf("итоговый масштаб %.4f, ждал 1.7 (170/100)", last.Scale)
	}
	if last.Center != image.Pt(135, 200) {
		t.Errorf("центр %v, ждал (135,200)", last.Center)
	}
	end := pinches[len(pinches)-1]
	if end.Phase != GestureEnded || end.Scale != last.Scale {
		t.Errorf("Ended: %+v", end)
	}
}

func TestGesture_PinchIn(t *testing.T) {
	var r GestureRecognizer
	got := feedAll(&r,
		tev(1, 0, 100, TouchBegan, 0),
		tev(2, 200, 100, TouchBegan, 0),
		tev(1, 50, 100, TouchMoved, 20),
		tev(2, 150, 100, TouchMoved, 20),
	)
	p, ok := got[len(got)-1].(Pinch)
	if !ok || math.Abs(p.Scale-0.5) > 1e-9 {
		t.Fatalf("щипок внутрь: %+v", got)
	}
	if p.Center != image.Pt(100, 100) {
		t.Errorf("центр %v", p.Center)
	}
}

func TestGesture_TwoFingerPanIsPinchWithUnitScale(t *testing.T) {
	var r GestureRecognizer
	got := feedAll(&r,
		tev(1, 100, 100, TouchBegan, 0),
		tev(2, 200, 100, TouchBegan, 0),
		tev(1, 100, 140, TouchMoved, 20),
		tev(2, 200, 140, TouchMoved, 20),
	)
	var p Pinch
	for _, g := range got {
		if pp, ok := g.(Pinch); ok {
			p = pp
		}
	}
	if p.Phase == 0 && p.Scale == 0 {
		t.Fatalf("двухпальцевая прокрутка не распознана: %v", got)
	}
	if math.Abs(p.Scale-1) > 0.05 {
		t.Errorf("масштаб %.3f, ждал около 1", p.Scale)
	}
	if p.CenterDelta.Y == 0 {
		t.Errorf("середина не сдвинулась: %+v", p)
	}
}

func TestGesture_SecondFingerEndsPan(t *testing.T) {
	var r GestureRecognizer
	got := feedAll(&r,
		tev(1, 0, 0, TouchBegan, 0),
		tev(1, 30, 0, TouchMoved, 16), // Pan Began
		tev(2, 100, 0, TouchBegan, 20),
	)
	if len(got) != 2 {
		t.Fatalf("жесты: %v", got)
	}
	if p := got[1].(Pan); p.Phase != GestureEnded {
		t.Errorf("при втором пальце Pan должен кончиться: %+v", p)
	}
}

func TestGesture_AfterPinchRemainingFingerIsDead(t *testing.T) {
	var r GestureRecognizer
	feedAll(&r,
		tev(1, 0, 0, TouchBegan, 0),
		tev(2, 100, 0, TouchBegan, 0),
		tev(2, 160, 0, TouchMoved, 16),
		tev(2, 160, 0, TouchEnded, 20),
	)
	// Оставшийся палец ходит где угодно — ни Pan, ни Tap.
	got := feedAll(&r,
		tev(1, 50, 0, TouchMoved, 40),
		tev(1, 400, 0, TouchMoved, 60),
		tev(1, 400, 0, TouchEnded, 80),
	)
	if len(got) != 0 {
		t.Errorf("остаточный палец породил жесты: %v", got)
	}
	// А после отрыва всех распознаватель снова чист.
	if r.Active() != 0 {
		t.Fatalf("остались пальцы: %d", r.Active())
	}
	got = feedAll(&r, tev(5, 10, 10, TouchBegan, 500), tev(5, 10, 10, TouchEnded, 540))
	if len(got) != 1 {
		t.Errorf("после отрыва всех тап не распознан: %v", got)
	}
}

func TestGesture_ThirdFingerIgnored(t *testing.T) {
	var r GestureRecognizer
	got := feedAll(&r,
		tev(1, 100, 100, TouchBegan, 0),
		tev(2, 200, 100, TouchBegan, 0),
		tev(3, 500, 500, TouchBegan, 1),
		tev(3, 900, 900, TouchMoved, 10),
		tev(3, 900, 900, TouchEnded, 20),
		tev(2, 240, 100, TouchMoved, 30),
	)
	if len(got) != 1 {
		t.Fatalf("жесты: %v", got)
	}
	if p, ok := got[0].(Pinch); !ok || math.Abs(p.Scale-1.4) > 1e-9 {
		t.Errorf("третий палец исказил щипок: %+v", got[0])
	}
}

func TestGesture_CancelDuringPan(t *testing.T) {
	var r GestureRecognizer
	got := feedAll(&r,
		tev(1, 0, 0, TouchBegan, 0),
		tev(1, 80, 0, TouchMoved, 16),
		tev(1, 160, 0, TouchCancelled, 30),
	)
	last := got[len(got)-1]
	if p, ok := last.(Pan); !ok || p.Phase != GestureCancelled {
		t.Errorf("ждал Pan Cancelled, получил %v", last)
	}
	for _, g := range got {
		if _, ok := g.(Swipe); ok {
			t.Error("отменённое касание дало Swipe")
		}
	}
}

func TestGesture_CancelDuringPinch(t *testing.T) {
	var r GestureRecognizer
	got := feedAll(&r,
		tev(1, 0, 0, TouchBegan, 0),
		tev(2, 100, 0, TouchBegan, 0),
		tev(2, 200, 0, TouchMoved, 16),
		tev(2, 200, 0, TouchCancelled, 30),
	)
	if p, ok := got[len(got)-1].(Pinch); !ok || p.Phase != GestureCancelled {
		t.Errorf("ждал Pinch Cancelled: %v", got)
	}
}

func TestGesture_CancelledTouchIsNotTap(t *testing.T) {
	var r GestureRecognizer
	got := feedAll(&r, tev(1, 5, 5, TouchBegan, 0), tev(1, 5, 5, TouchCancelled, 30))
	if len(got) != 0 {
		t.Errorf("отмена дала тап: %v", got)
	}
}

func TestGesture_RepeatedBeganResetsFinger(t *testing.T) {
	var r GestureRecognizer
	feedAll(&r, tev(1, 0, 0, TouchBegan, 0), tev(1, 90, 0, TouchMoved, 16))
	// Тот же ID без отрыва — прежний палец потерян, а новый — самостоятельный.
	got := feedAll(&r, tev(1, 300, 300, TouchBegan, 100), tev(1, 300, 300, TouchEnded, 140))
	var gotTap bool
	for _, g := range got {
		if _, ok := g.(Tap); ok {
			gotTap = true
		}
	}
	if !gotTap {
		t.Errorf("повторный Began не начал чистое касание: %v", got)
	}
}

func TestGesture_Thresholds(t *testing.T) {
	// Порог перетаскивания настраивается; 10 px при PanSlop=3 уже Pan.
	r := GestureRecognizer{Config: GestureConfig{PanSlop: 3}}
	got := feedAll(&r, tev(1, 0, 0, TouchBegan, 0), tev(1, 5, 0, TouchMoved, 16))
	if len(got) != 1 {
		t.Fatalf("PanSlop=3: %v", got)
	}
	// Нулевая настройка — значения по умолчанию.
	var d GestureRecognizer
	got = feedAll(&d, tev(1, 0, 0, TouchBegan, 0), tev(1, 5, 0, TouchMoved, 16))
	if len(got) != 0 {
		t.Errorf("по умолчанию 5 px не перетаскивание: %v", got)
	}
	if def := DefaultGestureConfig(); def.PanSlop != 10 || def.TapSlop != 10 || def.SwipeMinDistance != 48 {
		t.Errorf("значения по умолчанию изменились: %+v", def)
	}
}

func TestTouchGestures_Handler(t *testing.T) {
	var seen []Gesture
	h := &TouchGestures{OnGesture: func(g Gesture) { seen = append(seen, g) }}
	var _ TouchHandler = h
	if !h.OnTouch(tev(1, 1, 1, TouchBegan, 0)) || !h.OnTouch(tev(1, 1, 1, TouchEnded, 30)) {
		t.Fatal("обработчик жестов обязан принимать касания")
	}
	if len(seen) != 1 {
		t.Errorf("жестов %d", len(seen))
	}
}

func TestTouchPhase_String(t *testing.T) {
	for p, want := range map[TouchPhase]string{
		TouchBegan: "Began", TouchMoved: "Moved", TouchEnded: "Ended", TouchCancelled: "Cancelled",
	} {
		if p.String() != want {
			t.Errorf("%d: %q", p, p.String())
		}
	}
	if TouchBegan.Terminal() || TouchMoved.Terminal() || !TouchEnded.Terminal() || !TouchCancelled.Terminal() {
		t.Error("Terminal")
	}
}
