package engine

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
)

// Смена холста у остановленного движка не виснет: Stop закрывает канал
// кадров, а слив канала при смене холста (markCanvasReplaced) прежде принимал
// из закрытого канала бесконечно. Регрессия ревью 7.3 (ошибка старше выноса
// цикла).
func TestStop_ThenResolutionChangeReturns(t *testing.T) {
	testreset.Globals(t)
	e := New(100, 100, 30)
	e.Start()
	e.Stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.SetResolution(120, 90)
		e.SetScale(2)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("смена холста после Stop зависла")
	}
}
