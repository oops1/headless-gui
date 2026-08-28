package desktop

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// TestSystemTray_OverflowClearsBoundsOnClose — закрытая раскрывающаяся
// область не должна оставлять скрытым значкам настоящие границы.
//
// drawOverflow (Content раскрывающейся области) расставляет значкам реальные
// прямоугольники, чтобы они ловили клик внутри раскрытой области. Значок при
// этом невидим, но если после закрытия границы не обнулить, они продолжают
// указывать туда, где была область, — и следующий клик по пустому месту
// панели попадёт в невидимый значок (дефект аудита №2).
func TestSystemTray_OverflowClearsBoundsOnClose(t *testing.T) {
	tray, _ := trayTrio(t)
	defer tray.Close()

	// Раскрывающейся области нужен виджет в дереве — движок ищет оверлеи,
	// обходя Children() от корня, а не заглядывая в приватные поля SystemTray.
	root := widget.NewPanel(color.RGBA{})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 200, 200))
	root.AddChild(tray)
	root.AddChild(tray.Overflow())
	tray.Overflow().Screen = image.Rect(0, 0, 200, 200)

	// Ширины хватает от силы на один значок с кнопкой раскрытия — остальные
	// прячутся, и появляется шеврон (см. trayTrio/OverflowHidesExtras).
	tray.SetBounds(image.Rect(0, 0, 30, 24))
	hidden := tray.Hidden()
	if len(hidden) == 0 {
		t.Fatal("в тесную панель влезли все значки — переполнение не сработало")
	}

	press := widget.MouseEvent{X: 28, Y: 12, Button: widget.MouseLeft, Pressed: true}
	if !tray.OnMouseButton(press) {
		t.Fatal("щелчок по шеврону не поглощён")
	}
	if !tray.Overflow().IsOpen() {
		t.Fatal("щелчок по шеврону не раскрыл область")
	}

	eng := engine.New(200, 200, 30)
	eng.SetRoot(root)
	if img := eng.RenderOnce(); img == nil {
		t.Fatal("кадр не отрисован")
	}

	for i, it := range hidden {
		if it.Bounds().Empty() {
			t.Errorf("скрытый значок %d не получил границ в раскрытой области", i)
		}
	}

	// Повторный щелчок по шеврону — то же самое, что клик мимо или Esc:
	// закрывает область через Flyout.Close().
	tray.OnMouseButton(press)
	if tray.Overflow().IsOpen() {
		t.Fatal("повторный щелчок не свернул область")
	}

	for i, it := range hidden {
		if !it.Bounds().Empty() {
			t.Errorf("скрытый значок %d сохранил границы после закрытия — он ловит клики вслепую", i)
		}
	}
}
