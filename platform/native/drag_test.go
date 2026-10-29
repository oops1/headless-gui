package native

import (
	"testing"
)

func TestDragEffect_String(t *testing.T) {
	cases := map[DragEffect]string{
		0:                                   "none",
		DragEffectCopy:                      "copy",
		DragEffectCopy | DragEffectMove:     "copy|move",
		DragEffectMove | DragEffectLink:     "move|link",
		DragEffectCopy | DragEffectMove | 4: "copy|move|link",
		8:                                   "none", // неизвестные биты не считаются
	}
	for e, want := range cases {
		if got := e.String(); got != want {
			t.Errorf("DragEffect(%d).String() = %q, ждал %q", uint32(e), got, want)
		}
	}
}

func TestPickDragEffect(t *testing.T) {
	const c, m, l = DragEffectCopy, DragEffectMove, DragEffectLink
	cases := []struct {
		allowed, prefer, want DragEffect
	}{
		{c | m | l, 0, c},     // без пожеланий — копирование
		{m | l, 0, m},         // копирования нет — перемещение
		{l, 0, l},             // только ссылка
		{c | m, m, m},         // пожелание допустимо — берём его
		{c, m, c},             // пожелание недопустимо — то, что можно
		{c | m | l, l | m, m}, // из нескольких пожеланий — по старшинству
		{0, c, 0},             // нечего выбирать
	}
	for _, tc := range cases {
		if got := PickDragEffect(tc.allowed, tc.prefer); got != tc.want {
			t.Errorf("PickDragEffect(%v, %v) = %v, ждал %v", tc.allowed, tc.prefer, got, tc.want)
		}
	}
}

func TestDragEffectForKeys(t *testing.T) {
	const all = DragEffectCopy | DragEffectMove | DragEffectLink
	cases := []struct {
		name             string
		allowed          DragEffect
		ctrl, shift, alt bool
		want             DragEffect
	}{
		{"без клавиш", all, false, false, false, DragEffectCopy},
		{"Ctrl", all, true, false, false, DragEffectCopy},
		{"Shift", all, false, true, false, DragEffectMove},
		{"Ctrl+Shift", all, true, true, false, DragEffectLink},
		{"Alt", all, false, false, true, DragEffectLink},
		{"Shift, но перемещать нельзя", DragEffectCopy, false, true, false, DragEffectCopy},
		{"только перемещение", DragEffectMove, false, false, false, DragEffectMove},
		{"ничего не разрешено", 0, true, false, false, 0},
	}
	for _, tc := range cases {
		if got := DragEffectForKeys(tc.allowed, tc.ctrl, tc.shift, tc.alt); got != tc.want {
			t.Errorf("%s: %v, ждал %v", tc.name, got, tc.want)
		}
	}
}
