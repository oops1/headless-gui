// Тест кодов функциональных клавиш (ENGINE_ISSUES winline, v3.13.2):
// KeyF1–KeyF12 определены и совпадают с VK-кодами 0x70–0x7B.
package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

func TestKeyCodes_FunctionKeys(t *testing.T) {
	if widget.KeyF1 != 112 || widget.KeyF12 != 123 {
		t.Fatalf("KeyF1=%d KeyF12=%d, ждали 112/123 (VK-совместимость)", widget.KeyF1, widget.KeyF12)
	}
	for i := 0; i < 12; i++ {
		want := widget.KeyCode(112 + i)
		got := widget.KeyF1 + widget.KeyCode(i)
		if got != want {
			t.Errorf("KeyF%d = %d, ждали %d", i+1, got, want)
		}
	}
}
