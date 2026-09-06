package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Панель инструментов — запрос GG-17.
//
// Теги <ToolBarTray> и <ToolBar> разметка понимала и раньше, но собирала их
// горизонтальным StackPanel: раскладка была, поведения не было. Разделитель
// приходилось изображать узкой панелью, режим «только иконки» — переставлять
// IconPos у каждой кнопки вручную, а панель, не поместившаяся в окно, просто
// обрезалась: часть кнопок становилась недостижимой.

func toolBarWith(width int, texts ...string) (*widget.ToolBar, []*widget.Button) {
	tb := widget.NewToolBar()
	var btns []*widget.Button
	for _, s := range texts {
		b := widget.NewButton(s)
		tb.AddChild(b)
		btns = append(btns, b)
	}
	tb.SetBounds(image.Rect(0, 0, width, 32))
	return tb, btns
}

// Кнопки стоят слева направо и не налезают друг на друга.
func TestToolBar_LaysOutLeftToRight(t *testing.T) {
	tb, btns := toolBarWith(600, "Обновить", "Отправить", "Получить")
	_ = tb

	for i := 1; i < len(btns); i++ {
		prev, cur := btns[i-1].Bounds(), btns[i].Bounds()
		if cur.Min.X < prev.Max.X {
			t.Errorf("кнопка %d начинается на %d, предыдущая кончается на %d",
				i, cur.Min.X, prev.Max.X)
		}
		if cur.Dy() != prev.Dy() {
			t.Errorf("высоты кнопок разошлись: %d и %d", prev.Dy(), cur.Dy())
		}
	}
}

// Ширина кнопки считается по её подписи, а не круглым числом: панель из
// коротких кнопок не должна разъезжаться.
func TestToolBar_ButtonWidthFollowsText(t *testing.T) {
	_, short := toolBarWith(600, "OK")
	_, long := toolBarWith(600, "Очень длинная подпись кнопки")

	if short[0].Bounds().Dx() >= long[0].Bounds().Dx() {
		t.Errorf("короткая кнопка шириной %d, длинная — %d",
			short[0].Bounds().Dx(), long[0].Bounds().Dx())
	}
}

// Разделитель занимает своё место и рисуется чертой.
func TestToolBar_SeparatorTakesPlace(t *testing.T) {
	tb := widget.NewToolBar()
	a := widget.NewButton("Слева")
	b := widget.NewButton("Справа")
	tb.AddChild(a)
	tb.AddSeparator()
	tb.AddChild(b)
	tb.SetBounds(image.Rect(0, 0, 600, 32))

	gap := b.Bounds().Min.X - a.Bounds().Max.X
	if gap < 8 {
		t.Errorf("между кнопками %d точек — разделителю негде встать", gap)
	}
}

// Не поместившиеся кнопки прячутся, а не обрезаются.
func TestToolBar_OverflowHidesTail(t *testing.T) {
	tb, btns := toolBarWith(160, "Первая", "Вторая", "Третья", "Четвёртая")

	if tb.OverflowCount() == 0 {
		t.Fatal("на узкой панели ничего не переполнилось")
	}
	if !btns[0].IsVisible() {
		t.Error("первая кнопка спряталась — панель превратилась в один шеврон")
	}
	last := btns[len(btns)-1]
	if last.IsVisible() {
		t.Error("последняя кнопка осталась видимой на узкой панели")
	}
	// Видимые кнопки целиком внутри панели, а не обрезаны краем.
	for i, b := range btns {
		if !b.IsVisible() {
			continue
		}
		if b.Bounds().Max.X > tb.Bounds().Max.X {
			t.Errorf("кнопка %d вылезла за панель: %v при панели %v", i, b.Bounds(), tb.Bounds())
		}
	}
}

// Широкая панель ничего не прячет.
func TestToolBar_NoOverflowWhenItFits(t *testing.T) {
	tb, btns := toolBarWith(900, "Раз", "Два", "Три")

	if tb.OverflowCount() != 0 {
		t.Errorf("на широкой панели переполнилось %d элементов", tb.OverflowCount())
	}
	for i, b := range btns {
		if !b.IsVisible() {
			t.Errorf("кнопка %d спряталась без нужды", i)
		}
	}
}

// Щелчок по шеврону открывает меню, а пункт в нём выполняет действие кнопки.
func TestToolBar_OverflowMenuRunsHiddenButton(t *testing.T) {
	root := widget.NewPanel(color.RGBA{R: 24, G: 28, B: 36, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 120))

	tb := widget.NewToolBar()
	fired := ""
	for _, s := range []string{"Первая", "Вторая", "Третья", "Четвёртая"} {
		s := s
		b := widget.NewButton(s)
		b.OnClick = func() { fired = s }
		tb.AddChild(b)
	}
	tb.SetBounds(image.Rect(0, 0, 170, 32))
	root.AddChild(tb)

	eng := engine.New(400, 120, 30)
	eng.SetRoot(root)
	eng.RenderOnce()

	if tb.OverflowCount() == 0 {
		t.Fatal("панель ничего не спрятала — проверять нечего")
	}

	// Шеврон — в правом краю панели.
	cx, cy := 170-4-9, 16
	eng.SendMouseButton(cx, cy, widget.MouseLeft, true)
	eng.SendMouseButton(cx, cy, widget.MouseLeft, false)
	if !tb.HasOverlay() {
		t.Fatal("щелчок по шеврону не открыл меню переполнения")
	}

	// Первый пункт меню лежит сразу под шевроном.
	my := 32 + 8
	eng.SendMouseMove(cx+10, my)
	eng.SendMouseButton(cx+10, my, widget.MouseLeft, true)
	eng.SendMouseButton(cx+10, my, widget.MouseLeft, false)

	if fired == "" {
		t.Error("пункт меню переполнения не выполнил действие кнопки")
	}
	if fired == "Первая" {
		t.Errorf("в меню попала видимая кнопка %q", fired)
	}
}

// Режим «только иконки» прячет подписи и возвращает их обратно.
func TestToolBar_IconsOnlyIsReversible(t *testing.T) {
	tb := widget.NewToolBar()
	btn := widget.NewButton("Обновить")
	btn.Icon = image.NewRGBA(image.Rect(0, 0, 16, 16))
	btn.IconPos = widget.IconLeft
	tb.AddChild(btn)
	tb.SetBounds(image.Rect(0, 0, 600, 32))
	wide := btn.Bounds().Dx()

	tb.SetIconsOnly(true)
	if btn.IconPos != widget.IconOnly {
		t.Errorf("в режиме иконок подпись осталась на месте: %v", btn.IconPos)
	}
	if got := btn.Bounds().Dx(); got >= wide {
		t.Errorf("кнопка без подписи шириной %d, с подписью была %d", got, wide)
	}

	tb.SetIconsOnly(false)
	if btn.IconPos != widget.IconLeft {
		t.Errorf("после выключения режима подпись не вернулась: %v", btn.IconPos)
	}
	if got := btn.Bounds().Dx(); got != wide {
		t.Errorf("ширина после возврата %d, была %d", got, wide)
	}
}

// Кнопке без иконки прятать подпись не во что — режим её не трогает.
func TestToolBar_IconsOnlyKeepsTextlessButtons(t *testing.T) {
	tb := widget.NewToolBar()
	btn := widget.NewButton("Без иконки")
	tb.AddChild(btn)
	tb.SetBounds(image.Rect(0, 0, 600, 32))

	tb.SetIconsOnly(true)
	if btn.IconPos == widget.IconOnly {
		t.Error("кнопка без иконки осталась без подписи и без иконки — пустой прямоугольник")
	}
}

// Разметка: <ToolBar> собирается панелью, <Separator/> — разделителем.
func TestToolBar_FromXAML(t *testing.T) {
	xaml := `<Window Width="600" Height="100">
	  <Canvas>
	    <ToolBarTray Left="0" Top="0" Width="600" Height="40">
	      <ToolBar x:Name="tb" IconsOnly="False" Left="0" Top="0" Width="600" Height="36">
	        <Button Content="Обновить"/>
	        <Separator/>
	        <Button Content="Отправить"/>
	      </ToolBar>
	    </ToolBarTray>
	  </Canvas>
	</Window>`

	_, reg, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	tb, ok := reg["tb"].(*widget.ToolBar)
	if !ok {
		t.Fatalf("ToolBar собрался как %T", reg["tb"])
	}
	if got := len(tb.Children()); got != 3 {
		t.Errorf("в панели %d элементов, ждали три (кнопка, разделитель, кнопка)", got)
	}
}
