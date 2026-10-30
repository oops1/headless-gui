package x11

import "testing"

// Биты маски сверены с протоколом X11 (X.h): FocusChangeMask = 1<<21,
// PropertyChangeMask = 1<<22. Раньше вместо FocusChange стоял PropertyChange
// с подписью «FocusChangeMask», и FocusIn/FocusOut не приходили.
func TestX11TopLevelEventMask_FocusAndProperty(t *testing.T) {
	const (
		focusChange    = 1 << 21
		propertyChange = 1 << 22
	)
	if x11TopLevelEventMask&focusChange == 0 {
		t.Error("окно не слушает FocusIn/FocusOut: SetOnActivate и сброс повтора клавиш не сработают")
	}
	if x11TopLevelEventMask&propertyChange == 0 {
		t.Error("окно не слушает PropertyNotify: полный экран и разворачивание не отслеживаются")
	}
}
