package widget

import (
	"image"
	"image/color"
	"testing"
)

// Поля цвета в движке не было вовсе: приложение набирало код в обычный TextBox
// и ставило рядом панель нужного цвета. Тесты держат то, ради чего поле
// заводилось: код читается и пишется, палитра выбирается мышью и клавиатурой,
// каналы двигаются, а о каждой смене узнаёт приложение.

func TestParseHexColor(t *testing.T) {
	cases := []struct {
		in   string
		want color.RGBA
		ok   bool
	}{
		{"#0078D7", color.RGBA{R: 0x00, G: 0x78, B: 0xD7, A: 255}, true},
		{"0078d7", color.RGBA{R: 0x00, G: 0x78, B: 0xD7, A: 255}, true},
		{"  #FFFFFF  ", color.RGBA{R: 255, G: 255, B: 255, A: 255}, true},
		// Короткая запись раскрывается удвоением — как в CSS.
		{"#0AF", color.RGBA{R: 0x00, G: 0xAA, B: 0xFF, A: 255}, true},
		{"", color.RGBA{}, false},
		{"#12345", color.RGBA{}, false},
		{"#GGGGGG", color.RGBA{}, false},
		{"вишнёвый", color.RGBA{}, false},
	}
	for _, c := range cases {
		got, ok := ParseHexColor(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("ParseHexColor(%q) = %v, %v; ждал %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestHexColor(t *testing.T) {
	// Заглавными: код сверяют глазами, и вперемешку регистр мешает.
	if got := HexColor(color.RGBA{R: 0, G: 0x78, B: 0xD7, A: 255}); got != "#0078D7" {
		t.Errorf("HexColor = %q, ждал «#0078D7»", got)
	}
	// Альфа в код не попадает: поле работает с цветом, а не с плёнкой.
	if got := HexColor(color.RGBA{R: 1, G: 2, B: 3, A: 128}); got != "#010203" {
		t.Errorf("HexColor = %q, ждал «#010203»", got)
	}
}

func newPickerAt(r image.Rectangle) *ColorPicker {
	p := NewColorPicker()
	p.SetBounds(r)
	return p
}

func TestColorPicker_SetValueNotifies(t *testing.T) {
	p := newPickerAt(image.Rect(0, 0, 160, 28))
	var got []color.RGBA
	p.OnChanged = func(c color.RGBA) { got = append(got, c) }

	want := color.RGBA{R: 0x10, G: 0x7C, B: 0x10, A: 255}
	if !p.SetValue(want) {
		t.Fatal("SetValue вернул false на новом цвете")
	}
	if p.Value() != want {
		t.Errorf("Value = %v, ждал %v", p.Value(), want)
	}
	if p.Text() != "#107C10" {
		t.Errorf("Text = %q, ждал «#107C10»", p.Text())
	}
	// Тот же цвет второй раз — не событие.
	if p.SetValue(want) {
		t.Error("SetValue вернул true на том же цвете")
	}
	if len(got) != 1 {
		t.Errorf("событий %d, ждал одно", len(got))
	}

	// Прозрачность не проходит: цвет интерфейса — это цвет.
	p.SetValue(color.RGBA{R: 5, G: 6, B: 7})
	if p.Value().A != 255 {
		t.Errorf("альфа %d, ждал 255", p.Value().A)
	}
}

// Код набирается прямо в поле: Enter применяет, Esc откатывает.
func TestColorPicker_TypeHexCommit(t *testing.T) {
	p := newPickerAt(image.Rect(0, 0, 160, 28))
	var last color.RGBA
	p.OnChanged = func(c color.RGBA) { last = c }
	p.SetFocused(true)

	for _, r := range "#E81123" {
		p.OnKeyEvent(KeyEvent{Rune: r, Pressed: true})
	}
	if p.Text() != "#E81123" {
		t.Fatalf("в поле %q, ждал набранное «#E81123»", p.Text())
	}
	if p.Value() != (color.RGBA{A: 255}) {
		t.Error("цвет сменился до Enter")
	}

	p.OnKeyEvent(KeyEvent{Code: KeyEnter, Pressed: true})
	want := color.RGBA{R: 0xE8, G: 0x11, B: 0x23, A: 255}
	if p.Value() != want {
		t.Errorf("после Enter цвет %v, ждал %v", p.Value(), want)
	}
	if last != want {
		t.Errorf("OnChanged получил %v, ждал %v", last, want)
	}
}

func TestColorPicker_TypeHexEscapeReverts(t *testing.T) {
	p := newPickerAt(image.Rect(0, 0, 160, 28))
	p.SetValue(color.RGBA{R: 0x00, G: 0x78, B: 0xD7, A: 255})
	p.SetFocused(true)

	p.OnKeyEvent(KeyEvent{Code: KeyDelete, Pressed: true})
	for _, r := range "FF0000" {
		p.OnKeyEvent(KeyEvent{Rune: r, Pressed: true})
	}
	p.OnKeyEvent(KeyEvent{Code: KeyEscape, Pressed: true})

	if p.Value() != (color.RGBA{R: 0x00, G: 0x78, B: 0xD7, A: 255}) {
		t.Errorf("Esc не откатил набранное: %v", p.Value())
	}
	if p.Text() != "#0078D7" {
		t.Errorf("в поле %q, ждал прежний код", p.Text())
	}
}

// Неразобранный код не подставляет чёрный: поле помечает ошибку и держит цвет.
func TestColorPicker_BadHexKeepsValue(t *testing.T) {
	p := newPickerAt(image.Rect(0, 0, 160, 28))
	p.SetValue(color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 255})
	p.SetFocused(true)

	p.OnKeyEvent(KeyEvent{Code: KeyDelete, Pressed: true})
	for _, r := range "FF00" {
		p.OnKeyEvent(KeyEvent{Rune: r, Pressed: true})
	}
	p.OnKeyEvent(KeyEvent{Code: KeyEnter, Pressed: true})

	if p.Value() != (color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 255}) {
		t.Errorf("цвет сменился на неразобранном коде: %v", p.Value())
	}
	p.mu.Lock()
	invalid := p.invalid
	p.mu.Unlock()
	if !invalid {
		t.Error("поле не помечено ошибкой")
	}
}

// Уход фокуса применяет набранное: код, оставленный в поле, иначе пропал бы.
func TestColorPicker_BlurCommits(t *testing.T) {
	p := newPickerAt(image.Rect(0, 0, 160, 28))
	p.SetFocused(true)
	for _, r := range "#FFB900" {
		p.OnKeyEvent(KeyEvent{Rune: r, Pressed: true})
	}
	p.SetFocused(false)

	want := color.RGBA{R: 0xFF, G: 0xB9, B: 0x00, A: 255}
	if p.Value() != want {
		t.Errorf("после ухода фокуса цвет %v, ждал %v", p.Value(), want)
	}
}

func TestColorPicker_DropDownOpensAndPicks(t *testing.T) {
	field := image.Rect(10, 10, 170, 38)
	p := newPickerAt(field)
	var last color.RGBA
	p.OnChanged = func(c color.RGBA) { last = c }

	// Щелчок по стрелке раскрывает палитру.
	btn := cpButtonRect(field)
	p.OnMouseButton(MouseEvent{Button: MouseLeft, Pressed: true,
		X: btn.Min.X + btn.Dx()/2, Y: btn.Min.Y + btn.Dy()/2})
	if !p.IsDropDownOpen() {
		t.Fatal("палитра не раскрылась")
	}
	if p.OverlayBounds().Empty() {
		t.Error("палитра раскрыта, а прямоугольника оверлея нет")
	}

	// Щелчок по пятому образцу выбирает его и закрывает палитру.
	drop := p.OverlayBounds()
	cell := cpCellRect(drop, 4)
	p.OnMouseButton(MouseEvent{Button: MouseLeft, Pressed: true,
		X: cell.Min.X + 2, Y: cell.Min.Y + 2})

	want := DefaultColorPalette()[4]
	if p.Value() != want {
		t.Errorf("выбран %v, ждал образец %v", p.Value(), want)
	}
	if last != want {
		t.Errorf("OnChanged получил %v, ждал %v", last, want)
	}
	if p.IsDropDownOpen() {
		t.Error("палитра осталась раскрытой после выбора")
	}
}

// Щелчок мимо палитры закрывает её, не меняя цвет.
func TestColorPicker_ClickOutsideCloses(t *testing.T) {
	p := newPickerAt(image.Rect(10, 10, 170, 38))
	p.SetDropDownOpen(true)
	before := p.Value()

	p.OnMouseButton(MouseEvent{Button: MouseLeft, Pressed: true, X: 600, Y: 600})
	if p.IsDropDownOpen() {
		t.Error("палитра не закрылась")
	}
	if p.Value() != before {
		t.Error("цвет сменился от щелчка мимо")
	}
}

func TestColorPicker_BandDragChangesChannel(t *testing.T) {
	field := image.Rect(10, 10, 170, 38)
	p := newPickerAt(field)
	p.SetValue(color.RGBA{A: 255})
	p.SetDropDownOpen(true)

	drop := p.OverlayBounds()
	band := cpBandRect(drop, cpRows(len(p.Palette())), 0) // красный
	// Нажатие в середину дорожки — примерно половина канала.
	p.OnMouseButton(MouseEvent{Button: MouseLeft, Pressed: true,
		X: band.Min.X + band.Dx()/2, Y: band.Min.Y + band.Dy()/2})
	mid := p.Value().R
	if mid < 100 || mid > 155 {
		t.Errorf("после щелчка в середину R=%d, ждал около 128", mid)
	}
	if p.Value().G != 0 || p.Value().B != 0 {
		t.Errorf("сдвинулись соседние каналы: %v", p.Value())
	}
	if !p.IsDropDownOpen() {
		t.Error("палитра закрылась от щелчка по дорожке канала")
	}

	// Перетаскивание ведёт ползунок: до конца дорожки — максимум.
	p.OnMouseMove(band.Max.X+50, band.Min.Y+band.Dy()/2)
	if p.Value().R != 255 {
		t.Errorf("после перетаскивания вправо R=%d, ждал 255", p.Value().R)
	}
	p.OnMouseButton(MouseEvent{Button: MouseLeft, Pressed: false,
		X: band.Max.X + 50, Y: band.Min.Y})
	// После отпускания движение мыши канал не трогает.
	p.OnMouseMove(band.Min.X, band.Min.Y+band.Dy()/2)
	if p.Value().R != 255 {
		t.Errorf("канал поехал после отпускания: R=%d", p.Value().R)
	}
}

func TestColorPicker_KeyboardPicksFromPalette(t *testing.T) {
	p := newPickerAt(image.Rect(10, 10, 170, 38))
	p.SetFocused(true)

	// Alt+↓ раскрывает палитру — как у поля даты и выпадающего списка.
	p.OnKeyEvent(KeyEvent{Code: KeyDown, Mod: ModAlt, Pressed: true})
	if !p.IsDropDownOpen() {
		t.Fatal("Alt+↓ не раскрыл палитру")
	}

	// Курсор стоит на первом образце; вправо-вниз и Enter выбирают.
	p.OnKeyEvent(KeyEvent{Code: KeyRight, Pressed: true})
	p.OnKeyEvent(KeyEvent{Code: KeyDown, Pressed: true})
	p.OnKeyEvent(KeyEvent{Code: KeyEnter, Pressed: true})

	want := DefaultColorPalette()[1+cpPaletteCols]
	if p.Value() != want {
		t.Errorf("выбран %v, ждал %v", p.Value(), want)
	}
	if p.IsDropDownOpen() {
		t.Error("палитра осталась раскрытой после Enter")
	}
}

// Курсор не выходит за сетку: край не должен перескакивать через ряд.
func TestColorPicker_KeyboardStopsAtEdges(t *testing.T) {
	p := newPickerAt(image.Rect(10, 10, 170, 38))
	p.SetDropDownOpen(true)

	for i := 0; i < 50; i++ {
		p.OnKeyEvent(KeyEvent{Code: KeyLeft, Pressed: true})
	}
	p.OnKeyEvent(KeyEvent{Code: KeyEnter, Pressed: true})
	if p.Value() != DefaultColorPalette()[0] {
		t.Errorf("влево до упора дало %v, ждал первый образец", p.Value())
	}

	p.SetDropDownOpen(true)
	for i := 0; i < 50; i++ {
		p.OnKeyEvent(KeyEvent{Code: KeyRight, Pressed: true})
	}
	p.OnKeyEvent(KeyEvent{Code: KeyEnter, Pressed: true})
	last := DefaultColorPalette()[len(DefaultColorPalette())-1]
	if p.Value() != last {
		t.Errorf("вправо до упора дало %v, ждал последний образец %v", p.Value(), last)
	}
}

func TestColorPicker_SetPalette(t *testing.T) {
	p := newPickerAt(image.Rect(0, 0, 160, 28))
	own := []color.RGBA{{R: 1, A: 255}, {G: 2, A: 255}}
	p.SetPalette(own)
	if got := p.Palette(); len(got) != 2 || got[0] != own[0] {
		t.Errorf("палитра %v, ждал заданную", got)
	}
	// Пустой список возвращает набор по умолчанию: палитра без образцов —
	// это список, из которого нечего выбрать.
	p.SetPalette(nil)
	if got := p.Palette(); len(got) != len(DefaultColorPalette()) {
		t.Errorf("палитра из %d цветов, ждал набор по умолчанию", len(got))
	}
}

func TestColorPicker_XAML(t *testing.T) {
	const src = `<Canvas Width="400" Height="200">
  <ColorPicker Name="accent" Left="10" Top="10" Width="180" Height="28"
               Value="#0078D7" Palette="#000000, #FFFFFF, #E81123"/>
  <Swatch Name="mark" Left="10" Top="50" Width="18" Height="18" Color="#C42B1C"/>
</Canvas>`
	root, reg, err := LoadUIFromXAML([]byte(src))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	defer ReleaseXAML(root)

	p, ok := reg["accent"].(*ColorPicker)
	if !ok {
		t.Fatalf("accent — %T, ждал *ColorPicker", reg["accent"])
	}
	if got := p.Value(); got != (color.RGBA{R: 0x00, G: 0x78, B: 0xD7, A: 255}) {
		t.Errorf("Value = %v", got)
	}
	if got := p.Palette(); len(got) != 3 || got[2] != (color.RGBA{R: 0xE8, G: 0x11, B: 0x23, A: 255}) {
		t.Errorf("палитра из разметки: %v", got)
	}

	s, ok := reg["mark"].(*Swatch)
	if !ok {
		t.Fatalf("mark — %T, ждал *Swatch", reg["mark"])
	}
	if s.Color != (color.RGBA{R: 0xC4, G: 0x2B, B: 0x1C, A: 255}) {
		t.Errorf("цвет образца %v", s.Color)
	}
}

// Команда из разметки получает тот же цвет, что и колбэк.
func TestColorPicker_ValueChangedCommand(t *testing.T) {
	p := newPickerAt(image.Rect(0, 0, 160, 28))
	var got color.RGBA
	cmd := &RelayCommand{ExecuteFn: func(param interface{}) {
		if c, ok := param.(color.RGBA); ok {
			got = c
		}
	}}
	if !p.SetCommand("ValueChangedCommand", cmd) {
		t.Fatal("команда не подключилась")
	}
	want := color.RGBA{R: 0x87, G: 0x64, B: 0xB8, A: 255}
	p.SetValue(want)
	if got != want {
		t.Errorf("команда получила %v, ждал %v", got, want)
	}
}
