//go:build windows

package window

import "testing"

// Окно меньше дефолтного минимума Win32 (320×240) должно создаваться своего
// размера: иначе Windows растит его при создании, кадр остаётся прежним, и
// разница висит непрокрашенной полосой (артефакт «рамка больше диалога»).
func TestFitMinTrack(t *testing.T) {
	cases := []struct {
		name             string
		minW, minH, w, h int
		wantW, wantH     int
	}{
		{"диалог ниже минимума по высоте", 0, 0, 420, 168, 0, 168},
		{"диалог 520x226", 0, 0, 520, 226, 0, 226}, // ширина уже выше дефолта — не трогаем
		{"окно больше минимума", 0, 0, 1280, 900, 0, 0},
		{"узкий попап", 0, 0, 150, 80, 150, 80},
		{"явный минимум важнее", 300, 200, 420, 168, 300, 200},
		{"нулевой размер не трогаем", 0, 0, 0, 0, 0, 0},
	}
	for _, c := range cases {
		gotW, gotH := fitMinTrack(c.minW, c.minH, c.w, c.h)
		if gotW != c.wantW || gotH != c.wantH {
			t.Errorf("%s: fitMinTrack(%d,%d,%d,%d) = %d,%d; ждали %d,%d",
				c.name, c.minW, c.minH, c.w, c.h, gotW, gotH, c.wantW, c.wantH)
		}
	}
}
