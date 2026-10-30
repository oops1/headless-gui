package desktop

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/theme"
	"github.com/oops1/headless-gui/v4/widget"
)

// Профиль движения движка (engine.SetMotionProfile) в компонентах рабочего
// стола: ключ цикла сам отвечает профилем, как движок.

type profLoop struct{ p theme.MotionProfile }

func (l *profLoop) MotionProfile() theme.MotionProfile { return l.p }

// «Внимание» мигает и на удалённом столе: квантование на три шага попадало
// бы в полную силу на каждом и мигание пропадало.
func TestMotionProfile_AttentionBlinksRemote(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	l := &profLoop{theme.MotionRemote}
	unbind := widget.BindAnimationLoop(l)
	defer unbind()
	s := newW11Scene(t, theme.ProfileWindows11)
	s.area.layout()
	cell := s.area.ButtonRect(5)
	steady := attentionAlpha(areaFills(s.area), cell)
	t0 := time.Now()
	widget.StepAnimationsFor(l, t0)
	widget.StepAnimationsFor(l, t0.Add(225*time.Millisecond))
	if mid := attentionAlpha(areaFills(s.area), cell); mid >= steady {
		t.Errorf("MotionRemote: на середине мигания подложка %d, ждали слабее постоянной %d", mid, steady)
	}
}

// Автоскрытие на движке MotionReduced выезжает сразу, а не за умолчание
// 140 мс: нулевая длительность токена там — «без движения», а не «токена нет».
func TestMotionProfile_AutoHideReduced(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	l := &profLoop{theme.MotionReduced}
	unbind := widget.BindAnimationLoop(l)
	defer unbind()
	bar := hidingBar(t, theme.ProfileWindows10)
	full := bar.Bounds()
	bar.SetAutoHide(true)
	widget.StepAnimationsFor(l, time.Now())
	bar.OnMouseMove(400, screenBottom-1)
	widget.StepAnimationsFor(l, time.Now())
	if got := bar.Bounds(); got != full {
		t.Errorf("MotionReduced: после первого шага панель в %v, ждали сразу %v", got, full)
	}
}
