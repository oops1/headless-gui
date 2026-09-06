package tests

import (
	"image"
	"image/color"
	"strconv"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Усечение подписи кнопки (GG-49) и свой кегль у кнопки, флажка и списка
// (второй GG-50).
//
// Кнопка рисовала текст целиком: подпись, не влезающая в неё, вылезала за край
// и наезжала на соседний виджет — клиппинга у кнопки нет. А кегль был только у
// Label, TextBox и TextInput; остальные жёстко брали DefaultFontSizePt, и
// единственным рычагом оставалось менять его на всё приложение.

// paintedBox — охватывающий прямоугольник точек, отличных от фона.
func paintedBox(img *image.RGBA, bg color.RGBA) image.Rectangle {
	box := image.Rectangle{}
	n := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			i := img.PixOffset(x, y)
			if img.Pix[i] == bg.R && img.Pix[i+1] == bg.G && img.Pix[i+2] == bg.B {
				continue
			}
			r := image.Rect(x, y, x+1, y+1)
			if n == 0 {
				box = r
			} else {
				box = box.Union(r)
			}
			n++
		}
	}
	return box
}

var wideBG = color.RGBA{R: 20, G: 20, B: 24, A: 255}

func widgetScene(t *testing.T, w widget.Widget, r image.Rectangle) *image.RGBA {
	t.Helper()
	root := widget.NewPanel(wideBG)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 120))
	w.SetBounds(r)
	root.AddChild(w)

	eng := engine.New(400, 120, 30)
	eng.SetRoot(root)
	eng.RenderOnce()
	return snapshotRGBA(eng.RenderOnce())
}

// Длинная подпись не вылезает за кнопку.
func TestButton_EllipsizesLabel(t *testing.T) {
	btn := widget.NewButton("Очень длинная подпись кнопки, которая точно не влезет")
	img := widgetScene(t, btn, image.Rect(40, 40, 160, 72))

	box := paintedBox(img, wideBG)
	if box.Empty() {
		t.Fatal("кнопка не нарисована")
	}
	if box.Min.X < 40 || box.Max.X > 160 {
		t.Errorf("нарисованное вышло за кнопку: %v при кнопке (40,40)-(160,72)", box)
	}
}

// Короткая подпись не трогается.
func TestButton_ShortLabelUntouched(t *testing.T) {
	short := widget.NewButton("OK")
	a := widgetScene(t, short, image.Rect(40, 40, 200, 72))

	same := widget.NewButton("OK")
	b := widgetScene(t, same, image.Rect(40, 40, 200, 72))

	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatal("две одинаковые кнопки нарисованы по-разному")
		}
	}
}

// diffBoxOf — где два кадра расходятся.
func diffBoxOf(a, b *image.RGBA) image.Rectangle {
	box := image.Rectangle{}
	n := 0
	for i := range a.Pix {
		if a.Pix[i] == b.Pix[i] {
			continue
		}
		y := i / a.Stride
		x := (i % a.Stride) / 4
		r := image.Rect(x, y, x+1, y+1)
		if n == 0 {
			box = r
		} else {
			box = box.Union(r)
		}
		n++
	}
	return box
}

// Свой кегль меняет размер подписи у кнопки, флажка и списка.
//
// Меряем ТЕКСТ, а не виджет: фон и рамка занимают всю отведённую область при
// любом кегле, и охватывающий прямоугольник самого виджета не изменился бы.
// Поэтому сравниваем кадр с подписью и кадр без неё — расходятся они ровно на
// буквах.
func TestFontSize_AppliesToWidgets(t *testing.T) {
	rect := image.Rect(20, 30, 380, 70)
	cases := []struct {
		name string
		make func(text string, size float64) widget.Widget
	}{
		{"кнопка", func(text string, s float64) widget.Widget {
			b := widget.NewButton(text)
			b.FontSize = s
			return b
		}},
		{"флажок", func(text string, s float64) widget.Widget {
			c := widget.NewCheckBox(text)
			c.FontSize = s
			return c
		}},
		{"список", func(text string, s float64) widget.Widget {
			d := widget.NewDropdown(text)
			d.FontSize = s
			return d
		}},
	}

	for _, tc := range cases {
		textBox := func(size float64) image.Rectangle {
			with := widgetScene(t, tc.make("Текст", size), rect)
			without := widgetScene(t, tc.make("", size), rect)
			return diffBoxOf(with, without)
		}
		small, large := textBox(8), textBox(20)
		if small.Empty() || large.Empty() {
			t.Errorf("%s: подпись не найдена (кегли 8 и 20: %v, %v)", tc.name, small, large)
			continue
		}
		if large.Dy() <= small.Dy() {
			t.Errorf("%s: кегль 20 дал высоту подписи %d, кегль 8 — %d",
				tc.name, large.Dy(), small.Dy())
		}
	}
}

// Ноль означает общий размер: виджет без своего кегля рисуется как раньше.
func TestFontSize_ZeroMeansDefault(t *testing.T) {
	plain := widget.NewButton("Текст")
	a := widgetScene(t, plain, image.Rect(40, 40, 200, 72))

	explicit := widget.NewButton("Текст")
	explicit.FontSize = 0
	b := widgetScene(t, explicit, image.Rect(40, 40, 200, 72))

	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatal("FontSize = 0 нарисовал иначе, чем отсутствие кегля")
		}
	}
}

// Атрибут разметки читается всеми тремя тегами.
func TestFontSize_FromXAML(t *testing.T) {
	xaml := `<Window Width="400" Height="200"><Canvas>
	  <Button x:Name="b" Left="10" Top="10" Width="120" Height="30" Content="Кнопка" FontSize="18"/>
	  <CheckBox x:Name="c" Left="10" Top="50" Width="120" Height="24" Content="Флажок" FontSize="9"/>
	  <ComboBox x:Name="d" Left="10" Top="90" Width="120" Height="26" FontSize="14"/>
	</Canvas></Window>`

	_, reg, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	if got := reg["b"].(*widget.Button).FontSize; got != 18 {
		t.Errorf("кнопка: кегль %s", strconv.FormatFloat(got, 'g', -1, 64))
	}
	if got := reg["c"].(*widget.CheckBox).FontSize; got != 9 {
		t.Errorf("флажок: кегль %s", strconv.FormatFloat(got, 'g', -1, 64))
	}
	if got := reg["d"].(*widget.Dropdown).FontSize; got != 14 {
		t.Errorf("список: кегль %s", strconv.FormatFloat(got, 'g', -1, 64))
	}
}
