// Тест Dropdown (ENGINE_ISSUES tts-studio #10): HasSelection.
package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// TestDropdown_HasSelection — issue #10.
func TestDropdown_HasSelection(t *testing.T) {
	dd := widget.NewDropdown("a", "b", "c")
	if dd.HasSelection() {
		t.Error("новый Dropdown: HasSelection = true, ждали false")
	}
	if got := dd.Selected(); got != 0 {
		t.Errorf("Selected() = %d, ждали 0 (документированный дефолт)", got)
	}

	dd.SetSelected(1)
	if !dd.HasSelection() {
		t.Error("после SetSelected: HasSelection = false")
	}

	dd.SetItems([]string{"x", "y"})
	if dd.HasSelection() {
		t.Error("после SetItems: HasSelection = true, ждали сброс")
	}
}
