package tests

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/theme"
)

// Кнопки заголовка widget.Window (не классического): цвет значков и подсветка
// под указателем выводятся из заголовка, а не зашиты серым 180 (замечание
// WinLine после v3.37.1).

// glyphScene — окно под встроенной темой Windows 10 с явными цветами заголовка.
func glyphScene(t *testing.T, bg, text color.RGBA) *capScene {
	t.Helper()
	s := capWindow(t, capProfile(t, theme.ProfileWindows10, nil))
	s.win.TitleBG, s.win.TitleColor = bg, text
	return s
}

// capBg — цвет фона заголовка в точке у левого края полосы (там нет ни текста,
// ни кнопок).
func capBg(img *image.RGBA, s *capScene) color.RGBA {
	b := s.win.Bounds()
	return capPixel(img, b.Min.X+30, b.Min.Y+2)
}

// glyphInk — самый далёкий от фона заголовка пиксель в прямоугольнике кнопки:
// цвет значка. Смотрим середину кнопки: ни линии под заголовком, ни
// скруглённого угла окна в неё не попадает.
func glyphInk(img *image.RGBA, r image.Rectangle, bg color.RGBA) color.RGBA {
	best, bestD := bg, 0
	cx, cy := capCenter(r)
	for y := cy - 8; y <= cy+8; y++ {
		for x := cx - 9; x <= cx+9; x++ {
			c := capPixel(img, x, y)
			d := 0
			for _, v := range []int{int(c.R) - int(bg.R), int(c.G) - int(bg.G), int(c.B) - int(bg.B)} {
				if v < 0 {
					v = -v
				}
				d += v
			}
			if d > bestD {
				best, bestD = c, d
			}
		}
	}
	return best
}

func capLuma(c color.RGBA) int { return (299*int(c.R) + 587*int(c.G) + 114*int(c.B)) / 1000 }

// capRatio — контраст двух цветов по WCAG 2.
func capRatio(a, b color.RGBA) float64 {
	lum := func(c color.RGBA) float64 {
		f := func(v uint8) float64 {
			s := float64(v) / 255
			if s <= 0.03928 {
				return s / 12.92
			}
			return math.Pow((s+0.055)/1.055, 2.4)
		}
		return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
	}
	la, lb := lum(a), lum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// Светлый акцентный заголовок (жёлто-оранжевый, как у WinLine): значки тёмные и
// читаются; тёмный — значки светлые, прежнего серого 180.
func TestWindowCaptionGlyphs_FollowTitleBackground(t *testing.T) {
	testreset.Globals(t)
	orange := color.RGBA{R: 255, G: 185, B: 0, A: 255}
	dark := color.RGBA{R: 32, G: 32, B: 32, A: 255}
	black, white := color.RGBA{A: 255}, color.RGBA{R: 255, G: 255, B: 255, A: 255}

	for _, tc := range []struct {
		name      string
		bg, text  color.RGBA
		wantLight bool
	}{
		{"светлый акцент", orange, black, false},
		{"тёмный заголовок", dark, white, true},
	} {
		s := glyphScene(t, tc.bg, tc.text)
		img := s.shot()
		bg := capBg(img, s)
		if !capNear(bg, tc.bg, 2) {
			t.Fatalf("%s: фон заголовка %v, ждали %v", tc.name, bg, tc.bg)
		}
		for _, btn := range []struct {
			name string
			r    image.Rectangle
		}{{"закрыть", s.win.CloseBtnRect()}, {"развернуть", s.win.MaxBtnRect()}, {"свернуть", s.win.MinBtnRect()}} {
			if btn.r.Empty() {
				continue
			}
			ink := glyphInk(img, btn.r, bg)
			if r := capRatio(ink, bg); r < 3 {
				t.Errorf("%s, %s: значок %v на фоне %v — контраст %.2f < 3", tc.name, btn.name, ink, bg, r)
			}
			if light := capLuma(ink) > capLuma(bg); light != tc.wantLight {
				t.Errorf("%s, %s: значок %v, светлый=%v, ждали светлый=%v", tc.name, btn.name, ink, light, tc.wantLight)
			}
		}
	}
	// Тёмный заголовок остаётся на прежнем сером: кадры не меняются.
	s := glyphScene(t, dark, white)
	img := s.shot()
	if ink := glyphInk(img, s.win.MinBtnRect(), capBg(img, s)); ink != (color.RGBA{R: 180, G: 180, B: 180, A: 255}) {
		t.Errorf("значок на тёмном заголовке %v, ждали прежний серый 180", ink)
	}
}

// Светлый заголовок темы (Windows 11): значки тёмные в активном окне и, приглушённые
// вместе с заголовком, тёмно-серые в неактивном — и читаются в обоих.
func TestWindowCaptionGlyphs_LightThemeActiveAndInactive(t *testing.T) {
	testreset.Globals(t)
	var inks [2]color.RGBA
	for i, active := range []bool{true, false} {
		s := capWindow(t, capProfile(t, theme.ProfileWindows11, nil))
		s.win.SetActive(active)
		img := s.shot()
		bg := capBg(img, s)
		if capLuma(bg) < 200 {
			t.Fatalf("активно=%v: заголовок Windows 11 не светлый: %v", active, bg)
		}
		inks[i] = glyphInk(img, s.win.CloseBtnRect(), bg)
		if r := capRatio(inks[i], bg); r < 3 {
			t.Errorf("активно=%v: значок %v на фоне %v — контраст %.2f < 3", active, inks[i], bg, r)
		}
	}
	if capLuma(inks[1]) <= capLuma(inks[0]) {
		t.Errorf("значок неактивного окна %v не приглушён относительно активного %v", inks[1], inks[0])
	}
}

// Подсветка кнопки под указателем: на светлом заголовке — затемнение, на
// тёмном — осветление, на 10–20 %; «закрыть» под указателем — красная, значок белый.
func TestWindowCaptionHover_FollowsTitleBrightness(t *testing.T) {
	testreset.Globals(t)
	orange := color.RGBA{R: 255, G: 185, B: 0, A: 255}
	dark := color.RGBA{R: 32, G: 32, B: 32, A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	for _, tc := range []struct {
		name     string
		bg, text color.RGBA
		darker   bool
	}{
		{"светлый", orange, color.RGBA{A: 255}, true},
		{"тёмный", dark, white, false},
	} {
		s := glyphScene(t, tc.bg, tc.text)
		r := s.win.MinBtnRect()
		x, y := capCenter(r)
		s.eng.SendMouseMove(x, y)
		img := s.shot()
		// Угол кнопки — подложка без значка.
		plate := capPixel(img, r.Min.X+1, r.Min.Y+1)
		// Доля сдвига: по самому изменившемуся каналу, от дистанции до чёрного
		// (затемнение) или белого (осветление).
		var frac float64
		for _, p := range [][2]int{{int(plate.R), int(tc.bg.R)}, {int(plate.G), int(tc.bg.G)}, {int(plate.B), int(tc.bg.B)}} {
			var f float64
			if tc.darker {
				if p[1] == 0 {
					continue
				}
				f = float64(p[1]-p[0]) / float64(p[1])
			} else {
				f = float64(p[0]-p[1]) / float64(255-p[1])
			}
			if f > frac {
				frac = f
			}
		}
		if frac < 0.10 || frac > 0.20 {
			t.Errorf("%s заголовок: подсветка %v на фоне %v — сдвиг %.0f %%, ждали 10–20 %%", tc.name, plate, tc.bg, frac*100)
		}
		if tc.darker && !(capLuma(plate) < capLuma(tc.bg)) || !tc.darker && !(capLuma(plate) > capLuma(tc.bg)) {
			t.Errorf("%s заголовок: подсветка %v не в нужную сторону от %v", tc.name, plate, tc.bg)
		}
		// Без наведения подложки нет.
		s.eng.SendMouseMove(2, 2)
		img = s.shot()
		if p := capPixel(img, r.Min.X+1, r.Min.Y+1); !capNear(p, tc.bg, 2) {
			t.Errorf("%s заголовок: без наведения подложка %v, ждали фон %v", tc.name, p, tc.bg)
		}
	}

	// «Закрыть» под указателем — красная, значок белый, как и была.
	s := glyphScene(t, orange, color.RGBA{A: 255})
	r := s.win.CloseBtnRect()
	x, y := capCenter(r)
	s.eng.SendMouseMove(x, y)
	img := s.shot()
	if p := capPixel(img, r.Min.X+1, r.Min.Y+1); !capNear(p, color.RGBA{R: 232, G: 17, B: 35, A: 255}, 2) {
		t.Errorf("«закрыть» под указателем %v, ждали красный 232,17,35", p)
	}
	if ink := glyphInk(img, r, color.RGBA{R: 232, G: 17, B: 35, A: 255}); !capNear(ink, white, 2) {
		t.Errorf("значок «закрыть» под указателем %v, ждали белый", ink)
	}
}
