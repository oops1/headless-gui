package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Самостоятельный разделитель — запрос GG-44.
//
// Разделитель существовал только внутри меню, панели инструментов и вкладок.
// В произвольной раскладке приходилось класть панель высотой в пиксель и
// красить её вручную — а при смене темы перекрашивать самому, потому что
// панель про цвет разделителя ничего не знает.

// sepScene рисует разделитель на панели и возвращает кадр.
func sepScene(t *testing.T, s *widget.Separator, r image.Rectangle) *image.RGBA {
	t.Helper()
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 24, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 200, 100))
	s.SetBounds(r)
	root.AddChild(s)

	eng := engine.New(200, 100, 30)
	eng.SetRoot(root)
	eng.RenderOnce()
	return snapshotRGBA(eng.RenderOnce())
}

// rowPainted — сколько точек строки y отличаются от фона.
func rowPainted(img *image.RGBA, y int, bg color.RGBA) int {
	n := 0
	for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
		i := img.PixOffset(x, y)
		if img.Pix[i] != bg.R || img.Pix[i+1] != bg.G || img.Pix[i+2] != bg.B {
			n++
		}
	}
	return n
}

// colPainted — сколько точек столбца x отличаются от фона.
func colPainted(img *image.RGBA, x int, bg color.RGBA) int {
	n := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		i := img.PixOffset(x, y)
		if img.Pix[i] != bg.R || img.Pix[i+1] != bg.G || img.Pix[i+2] != bg.B {
			n++
		}
	}
	return n
}

var sepBG = color.RGBA{R: 20, G: 20, B: 24, A: 255}

// Горизонтальный разделитель рисует линию во всю отведённую ширину.
func TestSeparator_HorizontalLine(t *testing.T) {
	s := widget.NewSeparator()
	img := sepScene(t, s, image.Rect(10, 40, 190, 46))

	// Линия по центру отведённой полосы: y = 40 + (6-1)/2 = 42.
	if got := rowPainted(img, 42, sepBG); got != 180 {
		t.Errorf("в строке 42 закрашено %d точек, ждали 180", got)
	}
	// Соседние строки чистые — линия одна, а не заливка всей полосы.
	if got := rowPainted(img, 40, sepBG); got != 0 {
		t.Errorf("верхний край полосы закрашен на %d точек", got)
	}
	if got := rowPainted(img, 45, sepBG); got != 0 {
		t.Errorf("нижний край полосы закрашен на %d точек", got)
	}
}

// Вертикальный — столбец.
func TestSeparator_VerticalLine(t *testing.T) {
	s := widget.NewVerticalSeparator()
	img := sepScene(t, s, image.Rect(100, 10, 106, 90))

	if got := colPainted(img, 102, sepBG); got != 80 {
		t.Errorf("в столбце 102 закрашено %d точек, ждали 80", got)
	}
	if got := colPainted(img, 100, sepBG); got != 0 {
		t.Errorf("левый край полосы закрашен на %d точек", got)
	}
}

// Толщина и отступ вдоль линии.
func TestSeparator_ThicknessAndMargin(t *testing.T) {
	s := widget.NewSeparator()
	s.Thickness = 2
	s.Margin = 20
	img := sepScene(t, s, image.Rect(0, 40, 200, 46))

	// Линия в 2 точки: строки 42 и 43.
	if got := rowPainted(img, 42, sepBG); got != 160 {
		t.Errorf("строка 42: закрашено %d, ждали 160 (200 минус два отступа по 20)", got)
	}
	if got := rowPainted(img, 43, sepBG); got != 160 {
		t.Errorf("строка 43: закрашено %d — толщина 2 не соблюдена", got)
	}
	if got := rowPainted(img, 44, sepBG); got != 0 {
		t.Errorf("строка 44 закрашена на %d точек — линия толще заявленного", got)
	}
}

// Цвет берётся из темы и обновляется вместе с ней — ради этого виджет и нужен.
func TestSeparator_FollowsTheme(t *testing.T) {
	s := widget.NewSeparator()
	dark := widget.Win11DarkTheme()
	light := widget.Win11LightTheme()

	s.ApplyTheme(dark)
	got := s.Color
	if got != dark.Border {
		t.Errorf("после тёмной темы цвет %v, ждали %v", got, dark.Border)
	}
	s.ApplyTheme(light)
	if s.Color != light.Border {
		t.Errorf("после светлой темы цвет %v, ждали %v", s.Color, light.Border)
	}
	if s.Color == got {
		t.Error("цвет не изменился со сменой темы")
	}
}

// Разделитель сообщает раскладке свою толщину: контейнеру не нужно её угадывать.
func TestSeparator_DesiredSize(t *testing.T) {
	h := widget.NewSeparator()
	if _, dy := h.DesiredSize(); dy != 1 {
		t.Errorf("горизонтальный просит высоту %d, ждали 1", dy)
	}
	v := widget.NewVerticalSeparator()
	v.Thickness = 3
	if dx, _ := v.DesiredSize(); dx != 3 {
		t.Errorf("вертикальный просит ширину %d, ждали 3", dx)
	}
}

// Разметка: тег даёт разделитель, а не панель с зашитым цветом.
func TestSeparator_FromXAML(t *testing.T) {
	xaml := `<Window Width="300" Height="200"><Canvas>
	  <Separator x:Name="h" Left="10" Top="50" Width="280"/>
	  <Separator x:Name="v" Left="150" Top="80" Height="60" Orientation="Vertical"/>
	</Canvas></Window>`

	_, reg, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	h, ok := reg["h"].(*widget.Separator)
	if !ok {
		t.Fatalf("Separator собрался как %T", reg["h"])
	}
	if h.Orientation != widget.SeparatorHorizontal {
		t.Error("линия с заданной шириной оказалась вертикальной")
	}
	v, ok := reg["v"].(*widget.Separator)
	if !ok {
		t.Fatalf("вертикальный Separator собрался как %T", reg["v"])
	}
	if v.Orientation != widget.SeparatorVertical {
		t.Error(`Orientation="Vertical" не прочитан`)
	}
}

// Цвет вторичного текста — запрос GG-45.
//
// Disabled означает «этим нельзя пользоваться», и красить им пояснение —
// подменять смысл: человек вправе решить, что параметр выключен.
func TestTheme_SecondaryTextIsItsOwnColor(t *testing.T) {
	for _, th := range []*widget.Theme{widget.Win11DarkTheme(), widget.Win11LightTheme()} {
		if th.SecondaryText.A == 0 {
			t.Errorf("%s: цвет вторичного текста не задан", th.Style.Name)
			continue
		}
		if th.SecondaryText == th.Disabled {
			t.Errorf("%s: вторичный текст совпал с цветом выключенного", th.Style.Name)
		}
		if th.SecondaryText == th.LabelText {
			t.Errorf("%s: вторичный текст совпал с обычным", th.Style.Name)
		}
	}
}
