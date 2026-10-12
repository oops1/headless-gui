package window

import "testing"

// Win32: TrackMouseEvent(TME_LEAVE) одноразовый. Окно взводит его на первом
// движении после входа и больше не трогает, пока система не пришлёт
// WM_MOUSELEAVE.

func TestLeaveTracker_ArmOncePerEntry(t *testing.T) {
	var tr leaveTracker
	if !tr.needArm() {
		t.Fatal("первое движение должно взводить запрос")
	}
	tr.armedOK(true)
	for i := 0; i < 5; i++ {
		if tr.needArm() {
			t.Fatal("взведённый запрос на каждое движение не повторяется")
		}
	}
	tr.left() // пришёл WM_MOUSELEAVE
	if !tr.needArm() {
		t.Fatal("после ухода следующее движение должно взвести запрос заново")
	}
}

// Система отказала (окна уже нет) — запрос не взведён, пробуем на следующем
// движении.
func TestLeaveTracker_RefusalRetries(t *testing.T) {
	var tr leaveTracker
	tr.armedOK(false)
	if !tr.needArm() {
		t.Fatal("после отказа системы запрос надо повторить")
	}
}

// WM_MOUSELEAVE при переходе на нарисованную приложением кнопку заголовка —
// не уход: подсветку кнопки ведёт WM_NCMOUSEMOVE.
func TestMouseLeaveReal(t *testing.T) {
	cases := []struct {
		name   string
		under  HitArea
		inside bool
		want   bool
	}{
		{"ушёл за окно", HitClient, false, true},
		{"за окном на месте кнопки", HitCloseButton, false, true},
		{"на кнопку «свернуть»", HitMinButton, true, false},
		{"на кнопку «развернуть»", HitMaxButton, true, false},
		{"на кнопку «закрыть»", HitCloseButton, true, false},
		{"на заголовок", HitCaption, true, true},
		{"край окна (рамка ресайза)", HitClient, true, true},
	}
	for _, c := range cases {
		if got := mouseLeaveReal(c.under, c.inside); got != c.want {
			t.Errorf("%s: mouseLeaveReal(%v,%v) = %v, ждал %v", c.name, c.under, c.inside, got, c.want)
		}
	}
}
