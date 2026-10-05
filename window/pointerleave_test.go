package window

import "testing"

func TestX11PointerLeft(t *testing.T) {
	for _, tc := range []struct {
		detail, mode byte
		want         bool
	}{
		{0, 0, true},  // NotifyAncestor, Normal — ушёл
		{3, 0, true},  // NotifyNonlinear, Normal — ушёл в другое окно
		{0, 1, true},  // Grab — компоновщик забрал указатель (move/resize)
		{2, 0, false}, // NotifyInferior — ушёл в дочернее окно, остался над нашим
		{0, 2, false}, // Ungrab — указатель может быть внутри
	} {
		if got := x11PointerLeft(tc.detail, tc.mode); got != tc.want {
			t.Errorf("detail=%d mode=%d: %v, ждали %v", tc.detail, tc.mode, got, tc.want)
		}
	}
}
