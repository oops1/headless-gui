package core

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Боковые кнопки мыши «назад/вперёд» (X1/X2) доходят до движка на всех
// бэкендах под одними и теми же кодами; прежние кнопки не сдвигаются.

func TestNativeButton_X1X2(t *testing.T) {
	cases := []struct {
		code int
		want widget.MouseButton
		ok   bool
	}{
		{0, widget.MouseLeft, true},
		{1, widget.MouseRight, true},
		{2, widget.MouseMiddle, true},
		{3, widget.MouseWheelUp, true},
		{4, widget.MouseWheelDown, true},
		{5, widget.MouseButtonX1, true},
		{6, widget.MouseButtonX2, true},
		{7, 0, false},
		{-1, 0, false},
	}
	for _, c := range cases {
		got, ok := nativeButton(c.code)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("nativeButton(%d) = %v,%v; ждал %v,%v", c.code, got, ok, c.want, c.ok)
		}
	}
}

// Событие X1/X2 с насоса ОС добирается до движка и не превращается в левую
// кнопку.
func TestSurface_X1X2ReachEngine(t *testing.T) {
	_, fn, eng := inputSurface(t)
	fn.onButton(10, 20, 5, true)
	fn.onButton(10, 20, 5, false)
	fn.onButton(10, 20, 6, true)
	fn.onButton(10, 20, 6, false)
	eng.run()
	wantLog(t, eng,
		"button 10,20 5 true",
		"button 10,20 5 false",
		"button 10,20 6 true",
		"button 10,20 6 false")
	if widget.MouseButtonX1 != 5 || widget.MouseButtonX2 != 6 {
		t.Error("числовые значения X1/X2 сдвинулись")
	}
}
