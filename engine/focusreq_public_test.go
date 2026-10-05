package engine

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

func focusPair() (*Engine, *mnemonicHost, *widget.MenuBar) {
	e := New(400, 300, 1)
	bar := widget.NewMenuBar()
	bar.SetBounds(image.Rect(0, 0, 400, 24))
	host := &mnemonicHost{bar: bar}
	host.SetBounds(image.Rect(0, 40, 400, 300))
	root := widget.NewCanvas()
	root.SetBounds(image.Rect(0, 0, 400, 300))
	root.AddChild(bar)
	root.AddChild(host)
	e.SetRoot(root)
	e.SetFocus(host)
	return e, host, bar
}

// Панель, открытая не событием движка, берёт фокус через Engine.RequestFocus, а
// ReturnFocus отдаёт его прежнему владельцу. focusreq.Request вне доставки
// события ничего не делает — ради этого метод и заведён.
func TestEngineRequestFocus_OutsideEventDelivery(t *testing.T) {
	e, host, bar := focusPair()
	defer e.Stop()

	e.RequestFocus(bar)
	if got := e.focus.get(); got != bar {
		t.Fatalf("после RequestFocus фокус у %T", got)
	}
	e.ReturnFocus(bar)
	if got := e.focus.get(); got != host {
		t.Fatalf("после ReturnFocus фокус у %T, ждали прежнего", got)
	}
	// Фокус передали дальше — ReturnFocus его не трогает.
	e.RequestFocus(bar)
	e.SetFocus(nil)
	e.ReturnFocus(bar)
	if got := e.focus.get(); got != nil {
		t.Errorf("ReturnFocus отобрал чужой фокус: %T", got)
	}
	e.RequestFocus(nil) // не падает
}

// Из чужой горутины при запущенном движке вызов уходит в очередь Post и
// выполняется на горутине кадра, в порядке вызовов.
func TestEngineRequestFocus_FromOtherGoroutine(t *testing.T) {
	e, host, bar := focusPair()
	e.Start()
	defer e.Stop()

	done := make(chan struct{})
	go func() {
		e.RequestFocus(bar)
		close(done)
	}()
	<-done
	e.Flush()
	if got := e.focus.get(); got != bar {
		t.Fatalf("фокус у %T, ждали строку меню", got)
	}
	go func() {
		e.ReturnFocus(bar)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for e.focus.get() != host && time.Now().Before(deadline) {
		e.Flush()
		time.Sleep(5 * time.Millisecond)
	}
	if got := e.focus.get(); got != host {
		t.Fatalf("после ReturnFocus фокус у %T", got)
	}
}
