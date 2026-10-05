package desktop

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Светлое контекстное меню в светлом Windows 10: профиль объявляет стиль menu
// для флага taskbar.light, и меню команд кнопок приложений (и ПКМ-меню «Пуска»)
// следует режиму панели. Раньше меню оставалось тёмным в обоих режимах.

var (
	menuDarkFill  = color.RGBA{R: 43, G: 43, B: 43, A: 255}
	menuLightFill = color.RGBA{R: 242, G: 242, B: 242, A: 255}
)

func TestMenuStyle_FollowsLightFlag(t *testing.T) {
	tm := win10Fast(t)
	tm.SetFlag(theme.KeyTaskbarLight, false)
	if got := tm.GetStyle("menu", "", theme.StateNormal); got.Fill != menuDarkFill {
		t.Errorf("тёмная панель: заливка меню %v, ждали %v", got.Fill, menuDarkFill)
	}
	tm.SetFlag(theme.KeyTaskbarLight, true)
	light := tm.GetStyle("menu", "", theme.StateNormal)
	if light.Fill != menuLightFill {
		t.Errorf("светлая панель: заливка меню %v, ждали %v", light.Fill, menuLightFill)
	}
	if lum(light.Text) > 64 {
		t.Errorf("светлая панель: текст меню %v не тёмный", light.Text)
	}
	if hover := tm.GetStyle("menu", "item", theme.StateHover); lum(hover.Fill) < 180 {
		t.Errorf("светлая панель: наведение %v тёмное", hover.Fill)
	}
	if dis := tm.GetStyle("menu", "item", theme.StateDisabled); lum(dis.Text) < 100 || lum(dis.Text) > 200 {
		t.Errorf("светлая панель: недоступный пункт %v не приглушён", dis.Text)
	}
	// Назад на лету.
	tm.SetFlag(theme.KeyTaskbarLight, false)
	if got := tm.GetStyle("menu", "", theme.StateNormal); got.Fill != menuDarkFill {
		t.Errorf("после выключения флага заливка меню %v", got.Fill)
	}
	if hover := tm.GetStyle("menu", "item", theme.StateHover); lum(hover.Fill) > 100 {
		t.Errorf("тёмная панель: наведение %v светлое", hover.Fill)
	}
	if dis := tm.GetStyle("menu", "item", theme.StateDisabled); lum(dis.Text) > 160 {
		t.Errorf("тёмная панель: недоступный пункт %v слишком светлый", dis.Text)
	}
}

func lum(c color.RGBA) int {
	return (299*int(c.R) + 587*int(c.G) + 114*int(c.B)) / 1000
}

// menuProbe — пиксель внутри открытого меню команд (у левого верхнего угла,
// левее текста первого пункта).
func menuProbe(t *testing.T, g *goldenParts) (*image.RGBA, image.Point) {
	t.Helper()
	b := g.area.menu.overlayBounds()
	if b.Empty() {
		t.Fatal("меню команд не открылось")
	}
	return g.render(1), image.Pt(b.Min.X+3, b.Min.Y+3)
}

func TestAppMenu_LightPanelGivesLightMenu(t *testing.T) {
	for _, tc := range []struct {
		light bool
		want  color.RGBA
	}{{false, menuDarkFill}, {true, menuLightFill}} {
		g := goldenScene(t, tc.light)
		g.area.ShowCommands(0)
		img, pt := menuProbe(t, g)
		if got := img.RGBAAt(pt.X, pt.Y); got != tc.want {
			t.Errorf("light=%v: фон меню команд %v, ждали %v", tc.light, got, tc.want)
		}
	}
}

// Флаг переключают, пока меню открыто: оно перекрашивается без пересоздания.
func TestAppMenu_RecolorsWhileOpen(t *testing.T) {
	g := goldenScene(t, false)
	g.area.ShowCommands(0)
	menu := g.area.menu.get()

	img, pt := menuProbe(t, g)
	if got := img.RGBAAt(pt.X, pt.Y); got != menuDarkFill {
		t.Fatalf("тёмный режим: фон меню %v", got)
	}
	g.tm.SetFlag(theme.KeyTaskbarLight, true)
	img, pt = menuProbe(t, g)
	if got := img.RGBAAt(pt.X, pt.Y); got != menuLightFill {
		t.Errorf("после переключения на светлый фон меню %v, ждали %v", got, menuLightFill)
	}
	g.tm.SetFlag(theme.KeyTaskbarLight, false)
	img, pt = menuProbe(t, g)
	if got := img.RGBAAt(pt.X, pt.Y); got != menuDarkFill {
		t.Errorf("после возврата на тёмный фон меню %v, ждали %v", got, menuDarkFill)
	}
	if g.area.menu.get() != menu {
		t.Error("меню пересоздано при смене режима")
	}
}

// Меню «Пуска» по ПКМ красится так же, как меню кнопок.
func TestStartContextMenu_FollowsPanelMode(t *testing.T) {
	for _, light := range []bool{false, true} {
		tm := win10Fast(t)
		tm.SetFlag(theme.KeyTaskbarLight, light)
		m := NewStartMenu(tm, NewStaticAppCatalog())
		m.ContextMenu = func(StartTarget) []widget.MenuItem {
			return []widget.MenuItem{{Text: "Pin"}}
		}
		m.showContext(StartTarget{Kind: StartTargetApp, App: "x"}, image.Pt(10, 10))
		p := m.v.popup
		if p == nil {
			t.Fatalf("light=%v: меню не создано", light)
		}
		want := menuDarkFill
		if light {
			want = menuLightFill
		}
		if p.Background != want {
			t.Errorf("light=%v: фон меню «Пуска» %v, ждали %v", light, p.Background, want)
		}
	}
}
