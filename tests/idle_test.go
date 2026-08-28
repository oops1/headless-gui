package tests

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Неподвижный рабочий стол не готовит кадров.
//
// Часы на панели задач заведены секундной зациклённой анимацией, и сам факт
// её существования заставлял движок готовить кадр каждый тик. Хуже того — не
// частичный, а ПОЛНЫЙ: при пустом damage кадр идёт по полному пути, то есть
// блит фона во весь холст, обход всего дерева без клипа и сравнение всех
// тайлов. Кадр, которому нечего делать, стоил дороже кадра с изменением;
// заказчик намерил этим 110% ядра на неподвижном столе 1080p.
func TestIdle_DesktopWithClockRendersNothing(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileWindows11Dark); err != nil {
		t.Fatal(err)
	}

	root := widget.NewPanel(color.RGBA{R: 20, G: 24, B: 30, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 800, 600))

	clock := desktop.NewClock(m, desktop.SystemClock{})
	clock.SetBounds(image.Rect(700, 560, 800, 600))
	root.AddChild(clock)
	t.Cleanup(clock.Close)

	eng := engine.New(800, 600, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.Start()
	t.Cleanup(eng.Stop)

	// Даём стартовым кадрам пройти: SetRoot инвалидирует всё, а первый Draw
	// часов и заводит их анимацию.
	time.Sleep(150 * time.Millisecond)
	before := eng.RenderCount()

	// Полсекунды покоя. Часы за это время секунду не перешагнут (или
	// перешагнут ровно раз — тогда один кадр законен).
	time.Sleep(500 * time.Millisecond)
	got := eng.RenderCount() - before

	// При потолке 60 fps полсекунды покоя дали бы около тридцати кадров.
	if got > 2 {
		t.Errorf("неподвижный стол подготовил %d кадров за полсекунды — "+
			"зарегистрированная анимация часов будит движок на каждый тик", got)
	}
	t.Logf("кадров за полсекунды покоя: %d", got)
}

// Анимация, заявляющая свои изменения, кадры получает.
//
// Движок больше не рисует по факту существования анимации — он рисует по
// damage. Тик, двигающий виджет через сеттеры (как это делают все анимации
// движка), damage заявляет сам, и кадры идут как шли, только частичные.
func TestIdle_AnimationThatInvalidatesStillRenders(t *testing.T) {
	root := widget.NewPanel(color.RGBA{R: 20, G: 24, B: 30, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 300))

	box := widget.NewPanel(color.RGBA{R: 200, G: 90, B: 40, A: 255})
	box.ShowHeader = false
	box.SetBounds(image.Rect(10, 10, 60, 60))
	root.AddChild(box)

	eng := engine.New(400, 300, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.Start()
	t.Cleanup(eng.Stop)

	time.Sleep(150 * time.Millisecond)
	before := eng.RenderCount()

	a := widget.Animate(2*time.Second, nil, func(t float64) {
		x := 10 + int(t*200)
		box.SetBounds(image.Rect(x, 10, x+50, 60))
	})
	t.Cleanup(a.Stop)

	time.Sleep(300 * time.Millisecond)
	if got := eng.RenderCount() - before; got < 5 {
		t.Errorf("движущаяся анимация получила %d кадров за 300 мс — она бы шла рывками", got)
	}
}

// Поле ввода в фокусе не заставляет движок рисовать на полной частоте.
//
// Каретка мигает дважды в секунду, а движок готовил кадр шестьдесят раз —
// причём полный: damage при этом пуст, а пустой damage уводит кадр по полному
// пути. Отрисовка по запросу теряла смысл, стоило поставить курсор в текст.
func TestIdle_FocusedEditorDoesNotRenderEveryTick(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func() widget.Widget
	}{
		{"однострочное поле", func() widget.Widget {
			in := widget.NewTextInput("")
			in.SetBounds(image.Rect(20, 20, 300, 52))
			in.SetFocused(true)
			return in
		}},
		{"многострочный редактор", func() widget.Widget {
			tb := widget.NewTextBox("строка\nвторая")
			tb.SetBounds(image.Rect(20, 80, 300, 200))
			tb.SetFocused(true)
			return tb
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := widget.NewPanel(color.RGBA{R: 20, G: 24, B: 30, A: 255})
			root.ShowHeader = false
			root.SetBounds(image.Rect(0, 0, 400, 300))
			editor := tc.build()
			root.AddChild(editor)

			eng := engine.New(400, 300, 60)
			eng.SetRenderOnDemand(true)
			eng.SetRoot(root)
			// Фокус ЧЕРЕЗ ДВИЖОК: NeedsAnimation он спрашивает у своего
			// фокуса, а не у того, кто сам себя считает сфокусированным.
			eng.SetFocus(editor)
			eng.Start()
			t.Cleanup(eng.Stop)

			time.Sleep(150 * time.Millisecond)
			before := eng.RenderCount()
			const window = 1200 * time.Millisecond
			time.Sleep(window)
			got := eng.RenderCount() - before

			// Полупериод мигания — 530 мс, значит за 1,2 с каретка сменит
			// фазу два-три раза: столько кадров и ждём. Семьдесят означали бы
			// кадр на каждый тик.
			if got > 6 {
				t.Errorf("%s подготовил %d кадров за 1,2 с покоя", tc.name, got)
			}
			if got == 0 {
				t.Errorf("%s не нарисовал ни кадра — каретка не мигает", tc.name)
			}
			t.Logf("%s: кадров за 1,2 с: %d", tc.name, got)
		})
	}
}
