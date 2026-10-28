package engine

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// Переходы хостовых попапов (popuptrans.go): хост получает тот же Img с
// шагами прозрачности и сдвига; исчезающий остаётся в кадрах до конца.

type popupRec struct{ calls [][]PopupFrame }

func (r *popupRec) sink(f []PopupFrame) { r.calls = append(r.calls, append([]PopupFrame(nil), f...)) }
func (r *popupRec) last() []PopupFrame {
	if len(r.calls) == 0 {
		return nil
	}
	return r.calls[len(r.calls)-1]
}

func newPopupTransScene(t *testing.T, fade bool, offset float64) (*ovtScene, *popupRec) {
	t.Helper()
	m := ovtManager(t, 100*time.Millisecond, offset)
	s := newOvtScene(t, true, m, false)
	rec := &popupRec{}
	s.eng.SetPopupSink(rec.sink)
	t.Cleanup(func() { s.eng.SetPopupSink(nil) })
	s.eng.SetPopupFade(fade)
	return s, rec
}

// renderLikeLoop рисует кадр, только если цикл тоже нарисовал бы его.
func renderLikeLoop(e *Engine, gen *uint64) {
	if g := e.invGen.Load(); g != *gen {
		*gen = g
		e.RenderFrameNow()
	}
}

func TestPopupTrans_OpenClose(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	s, rec := newPopupTransScene(t, true, 10)
	s.setOpen(true)
	gen := s.eng.invGen.Load()
	s.eng.RenderFrameNow()
	f := rec.last()
	if len(f) != 1 || f[0].Transition == nil || f[0].Transition.Opacity != 0 || f[0].Transition.Closing {
		t.Fatalf("первый кадр хосту: %+v", f)
	}
	if f[0].Transition.Offset != image.Pt(0, -10) {
		t.Fatalf("сдвиг в начале %v, ждали (0,-10): меню под владельцем выезжает сверху", f[0].Transition.Offset)
	}
	img, draws := f[0].Img, s.o.draws
	t0 := time.Now()
	prev := -1.0
	for ms := 0; ms <= 120; ms += 20 {
		widget.StepAnimationsFor(s.eng, t0.Add(time.Duration(ms)*time.Millisecond))
		renderLikeLoop(s.eng, &gen)
		f := rec.last()
		if len(f) != 1 || f[0].Img != img {
			t.Fatalf("на %d мс кадр хосту %+v: Img сменился или пропал", ms, f)
		}
		if tr := f[0].Transition; tr != nil {
			if tr.Opacity < prev {
				t.Fatalf("непрозрачность пошла назад: %v после %v", tr.Opacity, prev)
			}
			prev = tr.Opacity
		}
	}
	if f := rec.last(); f[0].Transition != nil {
		t.Fatalf("появление не закончилось покоем: %+v", f[0].Transition)
	}
	if s.o.draws != draws {
		t.Fatalf("шаги перерисовывали оверлей: %d → %d", draws, s.o.draws)
	}

	// Исчезновение: ID в кадрах с Closing до конца, потом пропадает.
	s.setOpen(false)
	s.eng.RenderFrameNow()
	gen = s.eng.invGen.Load()
	f = rec.last()
	if len(f) != 1 || f[0].Transition == nil || !f[0].Transition.Closing || f[0].Img != img {
		t.Fatalf("кадр закрытия хосту: %+v", f)
	}
	t1 := time.Now()
	for ms := 0; ms <= 120; ms += 20 {
		widget.StepAnimationsFor(s.eng, t1.Add(time.Duration(ms)*time.Millisecond))
		renderLikeLoop(s.eng, &gen)
	}
	if f := rec.last(); len(f) != 0 {
		t.Fatalf("после исчезновения в кадрах осталось %+v", f)
	}
}

// Без SetPopupFade, без SetOverlayTransitions и при MotionOff — как прежде:
// кадры без Transition, закрытый уходит сразу.
func TestPopupTrans_Disabled(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	for _, tc := range []struct {
		name  string
		fade  bool
		setup func(*Engine)
	}{
		{"без SetPopupFade", false, func(*Engine) {}},
		{"без SetOverlayTransitions", true, func(e *Engine) { e.SetOverlayTransitions(false) }},
		{"MotionOff", true, func(e *Engine) { e.SetMotionProfile(MotionOff) }},
	} {
		s, rec := newPopupTransScene(t, tc.fade, 0)
		tc.setup(s.eng)
		s.setOpen(true)
		s.eng.RenderFrameNow()
		if f := rec.last(); len(f) != 1 || f[0].Transition != nil {
			t.Errorf("%s: кадр открытия %+v", tc.name, f)
		}
		s.setOpen(false)
		s.eng.RenderFrameNow()
		if f := rec.last(); len(f) != 0 {
			t.Errorf("%s: закрытый не ушёл сразу: %+v", tc.name, f)
		}
		s.eng.SetMotionProfile(MotionFull)
	}
}

// Открытие посреди исчезновения продолжает с текущей доли.
func TestPopupTrans_ReopenContinues(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	s, rec := newPopupTransScene(t, true, 0)
	s.setOpen(true)
	s.eng.RenderFrameNow()
	t0 := time.Now()
	widget.StepAnimationsFor(s.eng, t0)
	widget.StepAnimationsFor(s.eng, t0.Add(200*time.Millisecond))
	s.eng.RenderFrameNow()
	s.setOpen(false)
	s.eng.RenderFrameNow()
	t1 := time.Now()
	widget.StepAnimationsFor(s.eng, t1)
	widget.StepAnimationsFor(s.eng, t1.Add(50*time.Millisecond))
	s.eng.RenderFrameNow()
	s.setOpen(true)
	s.eng.RenderFrameNow()
	f := rec.last()
	if len(f) != 1 || f[0].Transition == nil || f[0].Transition.Closing {
		t.Fatalf("кадр переоткрытия: %+v", f)
	}
	if op := f[0].Transition.Opacity; op < 0.4 || op > 0.6 {
		t.Fatalf("переоткрытие начало с %v, ждали около 0.5", op)
	}
}

// Панель, анимирующая себя сама, и в окне-попапе перехода движка не получает.
func TestPopupTrans_SelfAnimatedSkipped(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	m := ovtManager(t, 100*time.Millisecond, 10)
	s := newOvtScene(t, true, m, true)
	rec := &popupRec{}
	s.eng.SetPopupSink(rec.sink)
	defer s.eng.SetPopupSink(nil)
	s.eng.SetPopupFade(true)
	s.setOpen(true)
	s.eng.RenderFrameNow()
	if f := rec.last(); len(f) != 1 || f[0].Transition != nil {
		t.Fatalf("самоанимирующийся оверлей получил переход: %+v", f)
	}
	s.setOpen(false)
	s.eng.RenderFrameNow()
	if f := rec.last(); len(f) != 0 {
		t.Fatalf("самоанимирующийся оверлей исчезает переходом движка: %+v", f)
	}
}

// Владелец ушёл из дерева — попап уходит сразу, без исчезновения поверх
// нового интерфейса.
func TestPopupTrans_OwnerRemovedGoesAtOnce(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	s, rec := newPopupTransScene(t, true, 0)
	s.setOpen(true)
	s.eng.RenderFrameNow()
	t0 := time.Now()
	widget.StepAnimationsFor(s.eng, t0)
	widget.StepAnimationsFor(s.eng, t0.Add(200*time.Millisecond))
	s.eng.RenderFrameNow()
	s.root.RemoveChild(s.ov)
	s.eng.Invalidate()
	s.eng.RenderFrameNow()
	if f := rec.last(); len(f) != 0 {
		t.Fatalf("попап ушедшего владельца остался в кадрах: %+v", f)
	}
}

// Два исчезающих попапа: кадр без изменений хосту не отдаётся — порядок
// исчезающих не зависит от обхода карты.
func TestPopupTrans_ClosingOrderStable(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	s, rec := newPopupTransScene(t, true, 0)
	o2 := &ovtOverlay{}
	o2.SetBounds(image.Rect(220, 20, 300, 44))
	o2.rect = image.Rect(220, 44, 300, 120)
	s.root.AddChild(o2)
	s.o.open, o2.open = true, true
	s.eng.Invalidate()
	s.eng.RenderFrameNow()
	t0 := time.Now()
	widget.StepAnimationsFor(s.eng, t0)
	widget.StepAnimationsFor(s.eng, t0.Add(200*time.Millisecond))
	s.eng.RenderFrameNow()
	s.o.open, o2.open = false, false
	s.eng.Invalidate()
	s.eng.RenderFrameNow()
	calls := len(rec.calls)
	for i := 0; i < 30; i++ {
		s.eng.InvalidateRect(image.Rect(0, 230, 4, 234)) // далеко от попапов
		s.eng.RenderFrameNow()
	}
	if n := len(rec.calls) - calls; n != 0 {
		t.Fatalf("хост получил %d кадров без изменений", n)
	}
}

// Повреждение попапа целиком за краем холста доходит до него.
func TestPopupTrans_OffCanvasDamageReachesPopup(t *testing.T) {
	testreset.Globals(t)
	s, rec := newPopupTransScene(t, false, 0)
	s.o.rect = image.Rect(40, 250, 200, 300) // холст 320×240
	s.setOpen(true)
	s.eng.RenderFrameNow()
	n := len(rec.calls)
	s.o.content = color.RGBA{R: 10, G: 200, B: 10, A: 255}
	s.eng.InvalidateRect(s.o.rect)
	s.eng.RenderFrameNow()
	if len(rec.calls) != n+1 {
		t.Fatalf("смена попапа за краем холста хосту не дошла: вызовов %d → %d", n, len(rec.calls))
	}
}
