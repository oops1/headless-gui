package desktop_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// finishAnims доводит анимации до конца: первый шаг запускает часы, второй —
// далеко в будущем.
func finishAnims() {
	t := time.Now()
	widget.StepAnimations(t)
	widget.StepAnimations(t.Add(time.Hour))
}

// Снимки меню «Пуск» Windows 10 (GOLDEN_OUT=каталог): тёмная и светлая тема,
// развёрнутая боковая панель, поиск, масштабы 100/150/200 %. Сами по себе тесты
// проверяют лишь, что кадр получен; смотреть результат надо глазами.
func TestStartMenu10_Snapshots(t *testing.T) {
	const w, h = 1280, 720
	for _, light := range []bool{false, true} {
		name := "dark"
		if light {
			name = "light"
		}
		t.Run(name, func(t *testing.T) {
			s := newWin10Scene(t, w, h, light, 1)
			s.open()
			img := s.frame()
			if img == nil {
				t.Fatal("кадр не отрисован")
			}
			savePNG(t, img, "start10_"+name)

			s.menu.SetSidebarExpanded(true)
			finishAnims()
			s.eng.Invalidate()
			savePNG(t, s.frame(), "start10_"+name+"_sidebar")
		})
	}
	for _, scale := range []float64{1, 1.5, 2} {
		scale := scale
		t.Run(fmt.Sprintf("scale%.0f", scale*100), func(t *testing.T) {
			s := newWin10Scene(t, w/2+w/4, h/2+h/4, false, scale)
			s.open()
			savePNG(t, s.frame(), fmt.Sprintf("start10_dark_%d", int(scale*100)))
		})
	}
}

// Состояния меню: наведение, клавиатурный выбор в трёх областях, поиск.
func TestStartMenu10_StateSnapshots(t *testing.T) {
	const w, h = 1280, 720
	for _, light := range []bool{false, true} {
		name := "dark"
		if light {
			name = "light"
		}
		t.Run(name, func(t *testing.T) {
			s := newWin10Scene(t, w, h, light, 1)
			s.open()
			g := s.menu.OverlayBounds()
			// Наведение на строку списка.
			s.menu.OnMouseMove(g.Min.X+150, g.Min.Y+125)
			finishAnims()
			s.eng.Invalidate()
			savePNG(t, s.frame(), "start10_"+name+"_hover")

			// Папка раскрыта на месте.
			s.menu.SetFolderExpanded("7-Zip", true)
			s.eng.Invalidate()
			savePNG(t, s.frame(), "start10_"+name+"_folder")

			// Клавиатура: плитки.
			s.menu.OnKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Pressed: true})
			s.menu.OnKeyEvent(widget.KeyEvent{Code: widget.KeyRight, Pressed: true})
			s.frame() // первый кадр запускает переход цвета
			finishAnims()
			s.eng.Invalidate()
			savePNG(t, s.frame(), "start10_"+name+"_kbd_tiles")

			// Поиск: набор текста в строке на панели.
			s.search.SetText("ed")
			s.menu.Settle()
			s.eng.Invalidate()
			savePNG(t, s.frame(), "start10_"+name+"_search")
		})
	}
}
