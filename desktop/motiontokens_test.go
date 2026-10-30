package desktop

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/theme"
)

// Токены движения панелей (задача 3.5): «Пуск» — start.open, остальные
// панели — flyout.open, без них — menu.open.

func TestMotionTokens_StartMenuAndFlyouts(t *testing.T) {
	testreset.Globals(t)
	tm := managerFor(t, theme.ProfileWindows11)
	start := NewStartMenu(tm, NewStaticAppCatalog())
	if d, _ := animationFirst(tm, start.animKeys()...); d != 250*time.Millisecond {
		t.Errorf("«Пуск» Windows 11: %v, ждали start.open 250 мс", d)
	}
	cal := NewCalendarFlyout(tm, nil)
	if d, _ := animationFirst(tm, cal.animKeys()...); d != 200*time.Millisecond {
		t.Errorf("календарь Windows 11: %v, ждали flyout.open 200 мс", d)
	}

	// Тема без новых ключей — всё по menu.open, как прежде.
	m := theme.NewManager()
	p := theme.NewProfile("old")
	p.Anims[theme.AnimMenuOpen] = theme.AnimSpec{Duration: 90 * time.Millisecond}
	if err := m.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	_ = m.SetTheme("old")
	if d, _ := animationFirst(m, NewStartMenu(m, NewStaticAppCatalog()).animKeys()...); d != 90*time.Millisecond {
		t.Errorf("«Пуск» без start.open: %v, ждали menu.open 90 мс", d)
	}
}
