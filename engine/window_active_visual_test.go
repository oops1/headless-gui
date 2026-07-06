package engine

import (
	"image"
	"os"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// TestWindowActive_VisualSample — окно в активном и неактивном состоянии
// для тем Win2000 и Win11 Light (HEADLESS_GUI_ACTIVE_PNG=file.png).
func TestWindowActive_VisualSample(t *testing.T) {
	out := os.Getenv("HEADLESS_GUI_ACTIVE_PNG")
	if out == "" {
		t.Skip("HEADLESS_GUI_ACTIVE_PNG не задан")
	}
	eng := New(560, 460, 20)
	c := eng.canvas
	c.blitBackground()

	place := func(theme string, x, y int, active bool, title string) {
		widget.ApplyGlobalTheme(widget.ThemeByName(theme))
		w := widget.NewWindow(title, 250, 190)
		w.SetBounds(image.Rect(x, y, x+250, y+190))
		widget.ApplyThemeTree(w, widget.ThemeByName(theme))
		w.SetActive(active)
		w.Draw(c)
	}

	place("Win2000", 20, 20, true, "Активное — Win2000")
	place("Win2000", 290, 20, false, "Неактивное — Win2000")
	place("Win11 Light", 20, 240, true, "Активное — Win11")
	place("Win11 Light", 290, 240, false, "Неактивное — Win11")

	savePNG(c.back, out)
	t.Logf("сохранено: %s", out)
}
