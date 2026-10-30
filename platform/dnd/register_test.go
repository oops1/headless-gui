package dnd

import (
	"runtime"
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// Флаг ставит init пакета там, где перетаскивание есть: Windows и Linux.
func TestRegister_DragDrop(t *testing.T) {
	want := runtime.GOOS == "windows" || runtime.GOOS == "linux"
	if got := native.DragDropEnabled(); got != want {
		t.Errorf("перетаскивание включено: %v, на %s ждал %v", got, runtime.GOOS, want)
	}
}
