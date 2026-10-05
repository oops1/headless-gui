package desktop_test

import (
	"fmt"
	"testing"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/widget"
)

// Снимки меню «Пуск» Windows 11 (GOLDEN_OUT=каталог): светлая и тёмная темы,
// главный вид, «Все приложения», поиск, наведение, фокус клавиатуры, масштабы
// 100/150/200 %. Сами по себе тесты проверяют лишь, что кадр получен; смотреть
// результат надо глазами.
func TestStartMenu11_Snapshots(t *testing.T) {
	const w, h = 1280, 800
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) {
			s := newWin11Scene(t, w, h, dark, 1)
			s.open()
			img := s.frame()
			if img == nil {
				t.Fatal("кадр не отрисован")
			}
			savePNG(t, img, "start11_"+name)

			// Наведение на ячейку и кнопку.
			r := s.rect()
			s.menu.OnMouseMove(r.Min.X+33+96*2+48, r.Min.Y+130)
			s.frame() // первый кадр запускает переход цвета
			finishAnims()
			s.eng.Invalidate()
			savePNG(t, s.frame(), "start11_"+name+"_hover")

			// Фокус клавиатуры: Tab из поиска на закреплённые, вправо.
			s.menu.OnMouseMove(r.Min.X-50, r.Min.Y-50)
			s.key(widget.KeyTab)
			s.key(widget.KeyRight)
			s.frame()
			finishAnims()
			s.eng.Invalidate()
			savePNG(t, s.frame(), "start11_"+name+"_focus")

			// «Все приложения».
			s.menu.SetView(desktop.StartViewAllApps)
			s.frame()
			finishAnims()
			s.eng.Invalidate()
			savePNG(t, s.frame(), "start11_"+name+"_all")

			// Поиск.
			s.menu.SetView(desktop.StartViewMain)
			s.typ("ed")
			s.frame()
			finishAnims()
			s.eng.Invalidate()
			savePNG(t, s.frame(), "start11_"+name+"_search")
		})
	}
	for _, scale := range []float64{1, 1.5, 2} {
		scale := scale
		t.Run(fmt.Sprintf("scale%.0f", scale*100), func(t *testing.T) {
			s := newWin11Scene(t, w*3/4, h*3/4, false, scale)
			s.open()
			savePNG(t, s.frame(), fmt.Sprintf("start11_light_%d", int(scale*100)))
		})
	}
}
