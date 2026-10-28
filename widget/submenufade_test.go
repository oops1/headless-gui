package widget

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
)

// Проявление подменю по theme.AnimSubmenuOpen (задача 3.5).

func subMenu(fade time.Duration) *PopupMenu {
	m := NewPopupMenu()
	m.SubMenuFade = fade
	m.SetItems([]MenuItem{{Text: "a", SubItems: []MenuItem{{Text: "b"}, {Text: "c"}}}})
	m.Show(10, 10)
	return m
}

func TestSubmenuFade_Steps(t *testing.T) {
	StopAllAnimations()
	defer StopAllAnimations()
	defer SetReduceMotion(false)
	m := subMenu(100 * time.Millisecond)
	defer m.Close()
	m.openChild(0)
	c, _ := m.openChildOf()
	if c == nil {
		t.Fatal("подменю не открылось")
	}
	if a, ok := c.fadeAlpha(); !ok || a != 0 {
		t.Fatalf("подменю не проявляется: alpha %v ok %v", a, ok)
	}
	t0 := time.Now()
	StepAnimations(t0)
	StepAnimations(t0.Add(50 * time.Millisecond))
	if a, ok := c.fadeAlpha(); !ok || a < 0.4 || a > 0.6 {
		t.Fatalf("середина проявления: alpha %v ok %v", a, ok)
	}
	StepAnimations(t0.Add(100 * time.Millisecond))
	if _, ok := c.fadeAlpha(); ok {
		t.Fatal("проявление не закончилось")
	}

	// «Меньше движения» — сразу.
	SetReduceMotion(true)
	r := subMenu(100 * time.Millisecond)
	defer r.Close()
	r.openChild(0)
	if rc, _ := r.openChildOf(); rc == nil {
		t.Fatal("подменю не открылось")
	} else if _, ok := rc.fadeAlpha(); ok {
		t.Fatal("при «меньше движения» подменю проявляется")
	}
}

// Профиль темы передаёт длительность меню; подменю наследует её от родителя.
func TestSubmenuFade_FromTheme(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	ms := MenuStyleFromTheme(m.Active())
	if ms.SubMenuOpenMS != 150 || ms.SubMenuOpenCurve != "out-cubic" {
		t.Fatalf("стиль меню Windows 11: %d мс %q, ждали 150 out-cubic", ms.SubMenuOpenMS, ms.SubMenuOpenCurve)
	}
}

// Меню, созданное под темой Windows 11, получает проявление подменю.
func TestSubmenuFade_ReachesThemedMenu(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	_ = m.SetTheme(theme.ProfileWindows11)
	prev := CurrentTheme()
	ApplyGlobalTheme(Materialize(m.Active()))
	defer ApplyGlobalTheme(prev)
	if d := NewPopupMenu().SubMenuFade; d != 150*time.Millisecond {
		t.Fatalf("меню под Windows 11: SubMenuFade %v, ждали 150 мс", d)
	}
}
