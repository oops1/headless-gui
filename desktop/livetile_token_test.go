package desktop

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Токен tile.cycle из профиля Windows 10 заводит переход живой плитки меню
// «Пуск» (ADR-0010, задача 6.4). Состояние источника доходит до плитки через
// Post, поэтому после каждой смены нужен кадр.

// shownAfter присылает состояние источника и рисует кадр, чтобы оно
// применилось.
func shownAfter(s *transScene, title string) {
	s.src.emit(TileContent{Title: title})
	s.eng.RenderOnce()
}

// Длительность перехода Windows 10 — 300 мс. Запасной токен flyout.open —
// 150 мс: переход, который ещё идёт на 200 мс и кончился к 400 мс, заведён
// именно токеном tile.cycle.
func TestLiveTileToken_ProfileDrivesTransition(t *testing.T) {
	testreset.Globals(t)
	tm := managerFor(t, theme.ProfileWindows10)
	s := newTransScene(t, tm, 1)
	shownAfter(s, "a")
	shownAfter(s, "b")
	if got := shownOf(s.m, "t2").Title; got != "b" {
		t.Fatalf("показано %q, ждали состояние источника b", got)
	}
	if s.trans() == nil {
		t.Fatal("профиль Windows 10 не завёл переход плитки")
	}
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(200 * time.Millisecond))
	if s.trans() == nil {
		t.Fatal("переход кончился за 200 мс: длительность не 300 мс, токен tile.cycle не применён")
	}
	widget.StepAnimations(t0.Add(400 * time.Millisecond))
	if s.trans() != nil {
		t.Error("переход Windows 10 не кончился за 400 мс")
	}
}

// «Меньше движения» обнуляет декоративный токен: смена мгновенная.
func TestLiveTileToken_ReducedMotionSkipsTransition(t *testing.T) {
	testreset.Globals(t)
	tm := managerFor(t, theme.ProfileWindows10)
	tm.SetFlag(theme.FlagMotionReduce, true)
	s := newTransScene(t, tm, 1)
	shownAfter(s, "a")
	shownAfter(s, "b")
	if got := shownOf(s.m, "t2").Title; got != "b" {
		t.Fatalf("показано %q, ждали состояние источника b", got)
	}
	if s.trans() != nil {
		t.Error("«меньше движения»: переход плитки завёлся, ждали мгновенную смену")
	}
}
