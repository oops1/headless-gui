package window

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// TestSeverityInfoFlag проверяет выбор NIIF_* по severity диалога.
func TestSeverityInfoFlag(t *testing.T) {
	cases := []struct {
		sev  widget.DialogSeverity
		want uint32
	}{
		{widget.SeverityNone, 0},     // NIIF_NONE
		{widget.SeverityInfo, 1},     // NIIF_INFO
		{widget.SeverityQuestion, 1}, // question → info
		{widget.SeverityWarning, 2},  // NIIF_WARNING
		{widget.SeverityError, 3},    // NIIF_ERROR
	}
	for _, c := range cases {
		if got := severityInfoFlag(c.sev); got != c.want {
			t.Errorf("severityInfoFlag(%v) = %d, want %d", c.sev, got, c.want)
		}
	}
}

// TestTrayButton проверяет маппинг кода кнопки в widget.MouseButton.
func TestTrayButton(t *testing.T) {
	cases := []struct {
		in   int
		want widget.MouseButton
	}{
		{0, widget.MouseLeft},
		{1, widget.MouseRight},
		{2, widget.MouseMiddle},
		{99, widget.MouseLeft}, // неизвестное → left
	}
	for _, c := range cases {
		if got := trayButton(c.in); got != c.want {
			t.Errorf("trayButton(%d) = %v, want %v", c.in, got, c.want)
		}
	}
}

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

// TestIconColorBuffer проверяет конверсию RGBA → top-down BGRA.
// SetRGBA пишет байты как есть (без премультипликации), что удобно для проверки.
func TestIconColorBuffer(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	img.SetRGBA(1, 0, color.RGBA{R: 40, G: 50, B: 60, A: 128})

	buf := iconColorBuffer(img)
	if len(buf) != 2*1*4 {
		t.Fatalf("len=%d, want 8", len(buf))
	}
	// Пиксель 0 → B,G,R,A = 30,20,10,255
	if buf[0] != 30 || buf[1] != 20 || buf[2] != 10 || buf[3] != 255 {
		t.Errorf("px0 = %v, want [30 20 10 255]", buf[0:4])
	}
	// Пиксель 1 → B,G,R,A = 60,50,40,128
	if buf[4] != 60 || buf[5] != 50 || buf[6] != 40 || buf[7] != 128 {
		t.Errorf("px1 = %v, want [60 50 40 128]", buf[4:8])
	}
}

// TestIconMaskBuffer проверяет AND-маску из альфы и WORD-выравнивание stride.
func TestIconMaskBuffer(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, color.RGBA{A: 255}) // непрозрачный
		}
	}
	img.SetRGBA(0, 0, color.RGBA{}) // прозрачный (a=0)

	mask, stride := iconMaskBuffer(img, 128)
	// 16 пикселей → stride = ceil(16/16)*2 = 2 байта.
	if stride != 2 {
		t.Fatalf("stride=%d, want 2", stride)
	}
	if len(mask) != stride*2 {
		t.Fatalf("len(mask)=%d, want %d", len(mask), stride*2)
	}
	// (0,0) прозрачный → старший бит первого байта = 1.
	if mask[0]&0x80 == 0 {
		t.Errorf("mask bit (0,0) not set: %#x", mask[0])
	}
	// (1,0) непрозрачный → следующий бит = 0.
	if mask[0]&0x40 != 0 {
		t.Errorf("mask bit (1,0) should be clear: %#x", mask[0])
	}
	// Вся строка 1 непрозрачна → байты второй строки == 0.
	for i := stride; i < 2*stride; i++ {
		if mask[i] != 0 {
			t.Errorf("row1 byte %d = %#x, want 0", i, mask[i])
		}
	}
}
