package desktop

import (
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
)

// Детерминированные проверки подписчика FakeTileSource (tileSub): устаревшая
// доставка не доходит до fn, а вложенный Set из fn ставится в очередь и уходит
// тем же циклом, без взаимной блокировки.

func TestFakeTileSub_StaleDeliveryDropped(t *testing.T) {
	testreset.Globals(t)
	var got []string
	sub := &tileSub{fn: func(c TileContent) { got = append(got, c.Title) }}
	sub.deliver(2, TileContent{Title: "новое"})
	sub.deliver(1, TileContent{Title: "старое"})
	sub.deliver(2, TileContent{Title: "повтор"})
	if len(got) != 1 || got[0] != "новое" {
		t.Errorf("доставлено %v, ждали только «новое»", got)
	}
}

func TestFakeTileSub_NestedSetFromHandlerIsQueued(t *testing.T) {
	testreset.Globals(t)
	var got []string
	var sub *tileSub
	sub = &tileSub{fn: func(c TileContent) {
		got = append(got, c.Title)
		if c.Title == "первое" {
			// Вложенная доставка из обработчика не должна взаимно блокироваться
			// и должна уйти после возврата из fn, тем же циклом.
			sub.deliver(3, TileContent{Title: "третье"})
			sub.deliver(2, TileContent{Title: "второе"})
		}
	}}
	sub.deliver(1, TileContent{Title: "первое"})
	if want := []string{"первое", "третье"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("доставлено %v, ждали %v", got, want)
	}
}

func TestFakeTileSub_ClosedGetsNothing(t *testing.T) {
	testreset.Globals(t)
	calls := 0
	sub := &tileSub{fn: func(TileContent) { calls++ }}
	sub.close()
	sub.deliver(1, TileContent{Title: "поздно"})
	if calls != 0 {
		t.Errorf("закрытый подписчик получил %d доставок", calls)
	}
}
