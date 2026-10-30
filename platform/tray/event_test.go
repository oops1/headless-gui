package tray

import "testing"

// TestTrayEventToClick — табличный тест маппинга кодов сообщений трея.
func TestTrayEventToClick(t *testing.T) {
	cases := []struct {
		name    string
		ev      uint32
		button  int
		dbl     bool
		balloon bool
		ok      bool
	}{
		{"lbuttonup", trayEvtLButtonUp, 0, false, false, true},
		{"ldblclk", trayEvtLButtonDblClk, 0, true, false, true},
		{"rbuttonup", trayEvtRButtonUp, 1, false, false, true},
		{"mbuttonup", trayEvtMButtonUp, 2, false, false, true},
		{"balloon", trayEvtBalloonUserClick, 0, false, true, true},
		{"mousemove_ignored", 0x0200, 0, false, false, false},
		{"lbuttondown_ignored", 0x0201, 0, false, false, false},
		{"high_bits_masked", 0xDEAD0000 | trayEvtRButtonUp, 1, false, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, dbl, balloon, ok := trayEventToClick(c.ev)
			if b != c.button || dbl != c.dbl || balloon != c.balloon || ok != c.ok {
				t.Errorf("trayEventToClick(%#x) = (%d,%v,%v,%v), want (%d,%v,%v,%v)",
					c.ev, b, dbl, balloon, ok, c.button, c.dbl, c.balloon, c.ok)
			}
		})
	}
}
