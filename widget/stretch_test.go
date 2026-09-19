package widget

import (
	"image"
	"testing"
)

// GG-84: распорка забирает свободное место, и правая группа кнопок уходит к
// правому краю. Раньше в StackPanel и ToolBar такого элемента не было вовсе:
// кнопки жались влево, а приложение считало остаток само.

func fixedButton(text string, w int) *Button {
	b := NewButton(text)
	b.SetBounds(image.Rect(0, 0, w, 24))
	b.SetXAMLSize(w, 24)
	return b
}

// В строке: левая группа слева, правая — прижата к правому краю.
func TestStretch_StackPanelHorizontal(t *testing.T) {
	sp := NewStackPanel(OrientationHorizontal)
	left := fixedButton("Apply", 80)
	right := fixedButton("Log", 60)
	sp.AddChild(left)
	sp.AddStretch()
	sp.AddChild(right)
	sp.SetBounds(image.Rect(0, 0, 400, 30))

	if got := left.Bounds().Min.X; got != 0 {
		t.Fatalf("левая кнопка начинается с %d, ждал 0", got)
	}
	if got := right.Bounds().Max.X; got != 400 {
		t.Fatalf("правая кнопка кончается на %d, ждал 400 (прижата к краю)", got)
	}
	if got := right.Bounds().Dx(); got != 60 {
		t.Fatalf("ширина правой кнопки %d, ждал 60 — распорка не должна её растягивать", got)
	}
}

// Две распорки делят остаток по весам; остаток от деления не теряется.
func TestStretch_Weights(t *testing.T) {
	sp := NewStackPanel(OrientationHorizontal)
	a := fixedButton("A", 50)
	b := fixedButton("B", 50)
	c := fixedButton("C", 50)
	sp.AddChild(a)
	sp.AddChild(NewStretchWeighted(1))
	sp.AddChild(b)
	sp.AddChild(NewStretchWeighted(3))
	sp.AddChild(c)
	sp.SetBounds(image.Rect(0, 0, 351, 30)) // остаток 201 = 50 + 151

	gap1 := b.Bounds().Min.X - a.Bounds().Max.X
	gap2 := c.Bounds().Min.X - b.Bounds().Max.X
	if gap1+gap2 != 201 {
		t.Fatalf("распорки заняли %d+%d, ждал ровно остаток 201", gap1, gap2)
	}
	if gap2 <= gap1*2 {
		t.Fatalf("доли %d и %d не похожи на 1:3", gap1, gap2)
	}
	if got := c.Bounds().Max.X; got != 351 {
		t.Fatalf("последняя кнопка кончается на %d, ждал 351", got)
	}
}

// В столбике распорка работает по высоте.
func TestStretch_StackPanelVertical(t *testing.T) {
	sp := NewStackPanel(OrientationVertical)
	top := fixedButton("Top", 60)
	bottom := fixedButton("Bottom", 60)
	top.SetBounds(image.Rect(0, 0, 60, 24))
	bottom.SetBounds(image.Rect(0, 0, 60, 24))
	sp.AddChild(top)
	sp.AddStretch()
	sp.AddChild(bottom)
	sp.SetBounds(image.Rect(0, 0, 100, 200))

	if got := bottom.Bounds().Max.Y; got != 200 {
		t.Fatalf("нижняя кнопка кончается на %d, ждал 200", got)
	}
	if got := bottom.Bounds().Dy(); got != 24 {
		t.Fatalf("высота нижней кнопки %d, ждал 24", got)
	}
}

// Места не осталось — распорка просто ничего не занимает.
func TestStretch_NoRoomLeft(t *testing.T) {
	sp := NewStackPanel(OrientationHorizontal)
	a := fixedButton("A", 200)
	b := fixedButton("B", 200)
	sp.AddChild(a)
	sp.AddStretch()
	sp.AddChild(b)
	sp.SetBounds(image.Rect(0, 0, 300, 30))

	if got := b.Bounds().Min.X; got != 200 {
		t.Fatalf("вторая кнопка начинается с %d, ждал 200 (распорка пуста)", got)
	}
}

// В панели инструментов: правая группа у правого края, распорка не попадает в
// меню переполнения.
func TestStretch_ToolBar(t *testing.T) {
	tb := NewToolBar()
	left := NewButton("Pull")
	right := NewButton("Log")
	tb.AddChild(left)
	tb.AddStretch()
	tb.AddChild(right)
	tb.SetBounds(image.Rect(0, 0, 600, 40))

	if got, want := right.Bounds().Max.X, 600-tb.Padding; got != want {
		t.Fatalf("правая кнопка кончается на %d, ждал %d", got, want)
	}
	if left.Bounds().Min.X != tb.Padding {
		t.Fatalf("левая кнопка начинается с %d, ждал %d", left.Bounds().Min.X, tb.Padding)
	}
	if tb.OverflowCount() != 0 {
		t.Fatalf("переполнение на пустом месте: %d", tb.OverflowCount())
	}

	// Тесная панель: распорка съёживается, кнопки остаются на месте.
	tb.SetBounds(image.Rect(0, 0, 160, 40))
	if tb.OverflowCount() != 0 {
		t.Fatalf("кнопки не поместились: переполнение %d", tb.OverflowCount())
	}
	for _, it := range tb.overflowItems() {
		if it.Text == "" {
			t.Fatalf("распорка попала в меню переполнения: %+v", tb.overflowItems())
		}
	}
}

// Разметка: <Stretch/> и <ToolBarStretch/>.
func TestStretch_XAML(t *testing.T) {
	_, reg, err := LoadUIFromXAML([]byte(`<Window Title="T" Width="600" Height="120">
  <ToolBar Name="tb" Left="0" Top="0" Width="600" Height="40">
    <Button Name="first" Content="Pull"/>
    <ToolBarStretch/>
    <Button Name="last" Content="Log"/>
  </ToolBar>
  <StackPanel Name="sp" Orientation="Horizontal" Left="0" Top="50" Width="600" Height="40">
    <Button Content="A" Width="60" Height="24"/>
    <Stretch Weight="2"/>
    <Button Name="spLast" Content="B" Width="60" Height="24"/>
  </StackPanel>
</Window>`))
	if err != nil {
		t.Fatal(err)
	}
	// Окно смещает детей на рамку и полосу заголовка — считаем от границ
	// самих панелей.
	tb := reg["tb"].(*ToolBar)
	last := reg["last"].(*Button)
	if got, want := last.Bounds().Max.X, tb.Bounds().Max.X-tb.Padding; got != want {
		t.Fatalf("кнопка панели кончается на %d, ждал %d", got, want)
	}
	sp := reg["sp"].(*StackPanel)
	spLast := reg["spLast"].(*Button)
	if got, want := spLast.Bounds().Max.X, sp.Bounds().Max.X-sp.Padding; got != want {
		t.Fatalf("кнопка столбика кончается на %d, ждал %d", got, want)
	}
}
