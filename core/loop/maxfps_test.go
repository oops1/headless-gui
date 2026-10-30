package loop

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/internal/testreset"
)

// Потолок частоты (SetMaxFPS) только ограничивает: выше целевой частоты не
// разгоняет, снятый и отрицательный — без потолка.
func TestFrameInterval_MaxFPSIsCapOnly(t *testing.T) {
	testreset.Globals(t)
	e := New(nil, 20)
	if got, want := e.FrameInterval(), 50*time.Millisecond; got != want {
		t.Fatalf("без потолка: %v, ожидалось %v", got, want)
	}
	e.SetMaxFPS(10)
	if got, want := e.FrameInterval(), 100*time.Millisecond; got != want {
		t.Errorf("потолок 10: %v, ожидалось %v", got, want)
	}
	e.SetMaxFPS(60) // выше частоты движка — не разгоняет
	if got, want := e.FrameInterval(), 50*time.Millisecond; got != want {
		t.Errorf("потолок 60 при fps=20: %v, ожидалось %v", got, want)
	}
	e.SetMaxFPS(10)
	e.SetMaxFPS(0) // снят
	if got, want := e.FrameInterval(), 50*time.Millisecond; got != want {
		t.Errorf("потолок снят: %v, ожидалось %v", got, want)
	}
	e.SetMaxFPS(-5)
	if got, want := e.FrameInterval(), 50*time.Millisecond; got != want {
		t.Errorf("отрицательный потолок: %v, ожидалось %v", got, want)
	}
}
