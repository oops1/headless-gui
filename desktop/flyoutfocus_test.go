package desktop

import (
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

type focusLog struct{ log []string }

func (l *focusLog) RequestFocus(w widget.Widget) { l.log = append(l.log, "request") }
func (l *focusLog) ReturnFocus(w widget.Widget)  { l.log = append(l.log, "return") }

// Панель, открытая вне события движка, просит фокус через исполнителя, а
// закрывшись — возвращает его; после снятия подписки не делает ни того ни
// другого.
func TestFocusOnOpen_RequestsOnOpenReturnsOnClose(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows2000)
	f := motionFlyout(tm)
	rec := &focusLog{}
	stop := FocusOnOpen(f, rec, nil)

	f.Open(motionAnchor)
	f.Close()
	if got := rec.log; len(got) != 2 || got[0] != "request" || got[1] != "return" {
		t.Fatalf("ждали request, return; получили %v", got)
	}
	stop()
	f.Open(motionAnchor)
	f.Close()
	if len(rec.log) != 2 {
		t.Errorf("после stop панель всё ещё просит фокус: %v", rec.log)
	}
}

func TestFlyoutManager_FocusOnOpen(t *testing.T) {
	defer widget.StopAllAnimations()
	m, _, _ := managerFixture(t, theme.ProfileWindows2000, "start", "quick")
	rec := &focusLog{}
	stop := m.FocusOnOpen(rec)
	defer stop()

	m.Open("start", motionAnchor)
	m.Open("quick", motionAnchor) // «Пуск» закрывается, быстрые настройки открываются
	m.CloseAll()
	want := []string{"request", "return", "request", "return"}
	if len(rec.log) != len(want) {
		t.Fatalf("ждали %v, получили %v", want, rec.log)
	}
	for i := range want {
		if rec.log[i] != want[i] {
			t.Fatalf("ждали %v, получили %v", want, rec.log)
		}
	}
}
