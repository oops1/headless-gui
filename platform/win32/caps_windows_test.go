//go:build windows

package win32

import (
	"testing"

	"golang.org/x/sys/windows"
)

// Возможности по версии: Windows 10 — без скругления DWM и без DIB из
// нескольких резерваций, Windows 11 — с ними.
func TestDetectCaps(t *testing.T) {
	for _, tc := range []struct {
		major, build uint32
		win11        bool
	}{
		{10, 19045, false}, // Windows 10 22H2
		{10, 22000, true},  // Windows 11 21H2
		{10, 26100, true},
		{6, 9600, false}, // Windows 8.1
	} {
		c := detectCaps(&windows.OsVersionInfoEx{MajorVersion: tc.major, BuildNumber: tc.build})
		if c.RoundCorners != tc.win11 || c.SplitDIBSource != tc.win11 {
			t.Errorf("%d.%d: %+v, ждали Windows 11 = %v", tc.major, tc.build, c, tc.win11)
		}
	}
	if Caps.Build == 0 || Caps.Major < 6 {
		t.Errorf("Caps текущей ОС не определены: %+v", Caps)
	}
}

// Без скругления DWM (Windows 10) окно со скруглением остаётся прямоугольным:
// углы ОС не скругляет, корпус рисуется прямым.
func TestCornersFollowCaps(t *testing.T) {
	w := newFsWindow(t)
	w.SetCornerRadius(8)
	if Caps.RoundCorners && !w.RoundsCorners() {
		t.Error("Windows 11: углы не скруглены средствами DWM")
	}
	saved := Caps
	t.Cleanup(func() { Caps = saved })
	Caps.RoundCorners = false
	w.SetCornerRadius(8)
	if w.RoundsCorners() {
		t.Error("без RoundCorners окно сообщает о скруглённых углах")
	}
}
