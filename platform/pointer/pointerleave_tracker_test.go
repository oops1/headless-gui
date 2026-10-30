package pointer

import (
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// Win32: TrackMouseEvent(TME_LEAVE) одноразовый. Окно взводит его на первом
// движении после входа и больше не трогает, пока система не пришлёт
// WM_MOUSELEAVE.

func TestLeaveTracker_ArmOncePerEntry(t *testing.T) {
	var tr LeaveTracker
	if !tr.NeedArm() {
		t.Fatal("первое движение должно взводить запрос")
	}
	tr.ArmedOK(true)
	for i := 0; i < 5; i++ {
		if tr.NeedArm() {
			t.Fatal("взведённый запрос на каждое движение не повторяется")
		}
	}
	tr.Left() // пришёл WM_MOUSELEAVE
	if !tr.NeedArm() {
		t.Fatal("после ухода следующее движение должно взвести запрос заново")
	}
}

// Система отказала (окна уже нет) — запрос не взведён, пробуем на следующем
// движении.
func TestLeaveTracker_RefusalRetries(t *testing.T) {
	var tr LeaveTracker
	tr.ArmedOK(false)
	if !tr.NeedArm() {
		t.Fatal("после отказа системы запрос надо повторить")
	}
}

// WM_MOUSELEAVE при переходе на нарисованную приложением кнопку заголовка —
// не уход: подсветку кнопки ведёт WM_NCMOUSEMOVE.
func TestMouseLeaveReal(t *testing.T) {
	cases := []struct {
		name   string
		under  native.HitArea
		inside bool
		want   bool
	}{
		{"ушёл за окно", native.HitClient, false, true},
		{"за окном на месте кнопки", native.HitCloseButton, false, true},
		{"на кнопку «свернуть»", native.HitMinButton, true, false},
		{"на кнопку «развернуть»", native.HitMaxButton, true, false},
		{"на кнопку «закрыть»", native.HitCloseButton, true, false},
		{"на заголовок", native.HitCaption, true, true},
		{"край окна (рамка ресайза)", native.HitClient, true, true},
	}
	for _, c := range cases {
		if got := MouseLeaveReal(c.under, c.inside); got != c.want {
			t.Errorf("%s: MouseLeaveReal(%v,%v) = %v, ждал %v", c.name, c.under, c.inside, got, c.want)
		}
	}
}
