package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Меню по встроенным профилям — то, что видно на экране (замечание WinLine
// после v3.32.0, п.3): тонкий шеврон и мягкая тень Windows 10, перекраска
// значков Windows 2000 без IconTint на пунктах.

// pmBuiltin — меню, одетое по встроенному профилю name.
func pmBuiltin(t *testing.T, name string, items ...widget.MenuItem) *widget.PopupMenu {
	t.Helper()
	tm := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(tm); err != nil {
		t.Fatal(err)
	}
	if err := tm.SetTheme(name); err != nil {
		t.Fatal(err)
	}
	m := widget.NewPopupMenu()
	m.SetItems(items)
	m.ApplyMenuStyle(widget.MenuStyleFromTheme(tm.Active()))
	return m
}

func TestBuiltinMenu_Windows10ThinChevronAndSoftShadow(t *testing.T) {
	m := pmBuiltin(t, theme.ProfileWindows10, widget.MenuItem{Text: "Создать", SubItems: []widget.MenuItem{{Text: "Папку"}}})
	img := pmRender(t, m, -1)
	r := m.Bounds()

	// Шеврон: тёмных... в тёмном меню Windows 10 шеврон светлый на тёмной заливке.
	cx := r.Max.X - 18
	y0 := 20 + m.PaddingY // меню показано в точке (20,20)
	light := 0
	for y := y0; y < y0+m.ItemHeight; y++ {
		for x := cx - 5; x < cx+5; x++ {
			if lum(img.RGBAAt(x, y)) > 150 {
				light++
			}
		}
	}
	if light < 6 {
		t.Errorf("Windows 10: тонкого шеврона у правого края нет (светлых точек %d)", light)
	}
	// Глифа «▸» у поля (PaddingX от края) больше нет.
	for y := y0; y < y0+m.ItemHeight; y++ {
		for x := r.Max.X - 11; x < r.Max.X-2; x++ {
			if lum(img.RGBAAt(x, y)) > 150 {
				t.Fatalf("Windows 10: правее шеврона светлая точка (%d,%d) — остался глиф стрелки", x, y)
			}
		}
	}

	// Мягкая тень: за краем меню тёмнее холста и затухает от меню.
	ym := 20 + 15
	near, far := lum(img.RGBAAt(r.Max.X+1, ym)), lum(img.RGBAAt(r.Max.X+5, ym))
	if near >= lum(pmCanvas) {
		t.Errorf("Windows 10: тени за краем меню нет: яркость %d при холсте %d", near, lum(pmCanvas))
	}
	if near >= far {
		t.Errorf("Windows 10: тень не затухает от меню: у края %d, дальше %d", near, far)
	}
}

func TestBuiltinMenu_Windows2000TintsIconsWithoutItemFlag(t *testing.T) {
	icon := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := 0; i < len(icon.Pix); i += 4 {
		icon.Pix[i+3] = 255 // сплошной чёрный квадрат; IconTint на пункте НЕ ставится
	}
	item := widget.MenuItem{Text: "Копировать", Icon: icon}

	// Под плашкой наведения (синяя, текст белый) значок белеет.
	m := pmBuiltin(t, theme.ProfileWindows2000, item)
	img := pmRender(t, m, 0)
	probe := func(img *image.RGBA) int {
		return lum(img.RGBAAt(20+1+m.PadLeft+8, 20+m.PaddingY+1+m.ItemHeight/2))
	}
	if l := probe(img); l < 200 {
		t.Errorf("Windows 2000: значок на синей плашке остался тёмным: %d", l)
	}
	// Без наведения — цвета текста (чёрный).
	m2 := pmBuiltin(t, theme.ProfileWindows2000, item)
	if l := probe(pmRender(t, m2, -1)); l > 60 {
		t.Errorf("Windows 2000: значок без наведения не цвета текста: %d", l)
	}

	// Windows 11 значки не перекрашивает: чёрный значок под плашкой остаётся чёрным.
	m3 := pmBuiltin(t, theme.ProfileWindows11, item)
	if m3.TintIcons {
		t.Error("Windows 11: значки меню перекрашиваются")
	}
}
