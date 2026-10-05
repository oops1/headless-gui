package desktop_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
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

// Снимки перехода по буквам, перетаскивания бегунка и предпросмотра новой группы
// (GOLDEN_OUT=каталог, тёмная и светлая тема).
func TestStartMenu10_JumpScrollGroupSnapshots(t *testing.T) {
	const w, h = 1280, 720
	for _, light := range []bool{false, true} {
		name := "dark"
		if light {
			name = "light"
		}
		t.Run(name, func(t *testing.T) {
			s := newWin10Scene(t, w, h, light, 1)
			s.open()
			b := s.menu.OverlayBounds()
			inner := b.Inset(1)
			listRight := inner.Min.X + 48 + 260

			// Сетка букв: открыта по Enter на заголовке.
			s.menu.OnKeyEvent(widget.KeyEvent{Code: widget.KeyDown, Pressed: true}) // «Яндекс.Музыка»
			s.menu.OnKeyEvent(widget.KeyEvent{Code: widget.KeyDown, Pressed: true}) // заголовок «#»
			s.menu.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
			if !s.menu.LetterGridOpen() {
				t.Fatal("сетка букв не открылась")
			}
			finishAnims()
			s.eng.Invalidate()
			savePNG(t, s.frame(), "start10_"+name+"_letters")
			// Выбор буквы: список прокручен к её группе.
			if !s.menu.JumpToLetter("M") {
				t.Fatal("нет группы M")
			}
			s.eng.Invalidate()
			savePNG(t, s.frame(), "start10_"+name+"_letters_jumped")

			// Бегунок списка схвачен и сдвинут: полоса широкая и не гаснет.
			x, y := listRight-5, inner.Min.Y+60
			s.menu.OnMouseMove(x, y)
			s.menu.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
			s.menu.OnMouseMove(x, y+120)
			s.eng.Invalidate()
			savePNG(t, s.frame(), "start10_"+name+"_thumb_drag")
			s.menu.OnMouseButton(widget.MouseEvent{X: x, Y: y + 120, Button: widget.MouseLeft})

			// Новая группа: плитка в пустом месте под последней.
			tx := inner.Min.X + 48 + 260 + 21
			ty := inner.Min.Y + 18
			cx, cy := tx+208+50, ty+141+50 // «Калькулятор»
			s.menu.OnMouseMove(cx, cy)
			s.menu.OnMouseButton(widget.MouseEvent{X: cx, Y: cy, Button: widget.MouseLeft, Pressed: true})
			s.menu.OnMouseMove(cx+10, cy+10)
			bottom := inner.Max.Y - 14
			for i := 0; i < 80; i++ {
				s.menu.OnMouseMove(tx+100, bottom)
			}
			s.eng.Invalidate()
			savePNG(t, s.frame(), "start10_"+name+"_newgroup_preview")
			var got []desktop.TileGroup
			s.menu.OnTilesChanged = func(g []desktop.TileGroup) { got = g }
			s.menu.OnMouseButton(widget.MouseEvent{X: tx + 100, Y: bottom, Button: widget.MouseLeft})
			if len(got) != 4 {
				t.Fatalf("после отпускания групп %d, ждали 4", len(got))
			}
			s.eng.Invalidate()
			savePNG(t, s.frame(), "start10_"+name+"_newgroup_done")
		})
	}
}
