package core

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/platform/native"
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

// TestIconColorBuffer проверяет конверсию RGBA → top-down BGRA.
// SetRGBA пишет байты как есть (без премультипликации), что удобно для проверки.
func TestIconColorBuffer(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	img.SetRGBA(1, 0, color.RGBA{R: 40, G: 50, B: 60, A: 128})

	buf := native.IconColorBuffer(img)
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
