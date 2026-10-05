package desktop_test

import (
	"fmt"
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Материалы Solid / Mica / MicaAlt и мягкие тени на панелях Windows 11.
// Сцена и вспомогательные функции — snapshots_test.go; кадры сохраняются при
// MAT_OUT (каталог).

func init() {
	// Окно сцены берёт материал из стиля "window" (флаг Mica профиля).
	matWindowHook = func(w *widget.Window, m *theme.Manager) {
		if st := m.GetStyle("window", "", theme.StateNormal); st.Backdrop.Material.IsMica() {
			w.SetBackdrop(st.Backdrop)
		}
	}
}

func withFlags(flags ...theme.Key) func(m *theme.Manager) {
	return func(m *theme.Manager) {
		for _, f := range flags {
			m.SetFlag(f, true)
		}
	}
}

// Снимки «Пуска» и быстрых настроек на Solid, Mica, MicaAlt и с мягкими
// тенями — светлая и тёмная.
func TestMaterials_Win11Panels(t *testing.T) {
	for _, profile := range []string{theme.ProfileWindows11, theme.ProfileWindows11Dark} {
		tag := map[string]string{theme.ProfileWindows11: "light", theme.ProfileWindows11Dark: "dark"}[profile]
		for _, panel := range []matPanel{matStart, matQuick, matNotify, matCalendar, matMenu, matWindow} {
			solid, area := matScene(t, profile, panel, nil)
			mica, _ := matScene(t, profile, panel, withFlags(theme.FlagBackdropMica))
			alt, _ := matScene(t, profile, panel, withFlags(theme.FlagBackdropMica, theme.FlagBackdropMicaAlt))
			soft, _ := matScene(t, profile, panel, withFlags(theme.FlagBackdropMica, theme.FlagShadowSoft))
			if area.Empty() {
				t.Fatalf("%s/%s: панель не открылась", tag, panel)
			}
			name := func(s string) string { return fmt.Sprintf("%s_%s_%s", panel, tag, s) }
			matSave(t, "MAT_OUT", solid, name("solid"))
			matSave(t, "MAT_OUT", mica, name("mica"))
			matSave(t, "MAT_OUT", alt, name("micaalt"))
			matSave(t, "MAT_OUT", soft, name("mica_softshadow"))

			if panel == matMenu {
				// Меню остаётся акрилом, Mica его не касается; его тень по токенам
				// подключает сам PopupMenu (widget.MenuShadow).
				if !equalImages(solid, mica) {
					t.Errorf("%s/menu: флаг Mica изменил контекстное меню", tag)
				}
				continue
			}
			if equalImages(solid, mica) {
				t.Errorf("%s/%s: Mica ничем не отличается от Solid", tag, panel)
			}
			if panel == matWindow {
				continue // окно: клиентскую область закрывает Background содержимого
			}
			inner := area.Inset(24)
			lm, la := meanLum(mica, inner), meanLum(alt, inner)
			if la >= lm {
				t.Errorf("%s/%s: MicaAlt (%.1f) не темнее Mica (%.1f)", tag, panel, la, lm)
			}
			// Мягкая тень: полоса над панелью темнее, чем без тени.
			above := image.Rect(area.Min.X+20, area.Min.Y-14, area.Max.X-20, area.Min.Y-2)
			if meanLum(soft, above) >= meanLum(mica, above) {
				t.Errorf("%s/%s: мягкая тень не затемнила область над панелью", tag, panel)
			}
		}
	}
}

// Два кадра Mica на одних обоях побайтно равны: размытие не копится.
func TestMaterials_MicaIsStable(t *testing.T) {
	a, _ := matScene(t, theme.ProfileWindows11, matStart, withFlags(theme.FlagBackdropMica))
	b, _ := matScene(t, theme.ProfileWindows11, matStart, withFlags(theme.FlagBackdropMica))
	if !equalImages(a, b) {
		t.Fatal("два кадра Mica на одних обоях разные")
	}
}

// Без обоев Mica — сплошной цвет темы (Fallback = surface).
func TestMaterials_MicaWithoutWallpaperIsSolid(t *testing.T) {
	m := theme.NewManager()
	_ = theme.RegisterBuiltinProfiles(m)
	_ = m.SetTheme(theme.ProfileWindows11)
	m.SetFlag(theme.FlagBackdropMica, true)
	eng := engine.New(matW, matH, 30)
	if err := eng.ApplyThemeProfile(m); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(widget.StopAllAnimations)
	root := widget.NewCanvas()
	root.SetBounds(image.Rect(0, 0, matW, matH))
	cat := desktop.NewStaticAppCatalog(desktop.AppInfo{ID: "x", Title: "X"})
	p := desktop.NewStartMenu(m, cat)
	p.Screen = image.Rect(0, 0, matW, matH)
	root.AddChild(p)
	p.Open(image.Rect(matW/2-24, matH-48, matW/2+24, matH))
	p.Settle()
	t.Cleanup(p.Close)
	eng.SetRoot(root)
	img := eng.RenderOnce()
	r := p.OverlayBounds()
	// Угол панели: туда не легли ни строки, ни рамка.
	got := img.RGBAAt(r.Max.X-6, r.Min.Y+6)
	want := m.GetStyle(desktop.ComponentStartMenu, "", theme.StateNormal).Backdrop.Fallback
	if got != want {
		t.Errorf("Mica без обоев = %v, ждали сплошной %v", got, want)
	}
}

// Включение Mica на лету перерисовывает панели без пересоздания: тот же
// компонент, другой кадр.
func TestMaterials_FlagSwitchesLive(t *testing.T) {
	m := theme.NewManager()
	_ = theme.RegisterBuiltinProfiles(m)
	_ = m.SetTheme(theme.ProfileWindows11)
	eng := engine.New(matW, matH, 30)
	if err := eng.SetBackground(matWallpaper(matW, matH)); err != nil {
		t.Fatal(err)
	}
	_ = eng.ApplyThemeProfile(m)
	t.Cleanup(widget.StopAllAnimations)
	root := widget.NewCanvas()
	root.SetBounds(image.Rect(0, 0, matW, matH))
	p := desktop.NewQuickSettings(m, desktop.NewFakeSystemStatus())
	p.Screen = image.Rect(0, 0, matW, matH)
	root.AddChild(p)
	p.Open(image.Rect(matW-110, matH-48, matW-60, matH))
	p.Settle()
	t.Cleanup(p.Close)
	eng.SetRoot(root)

	solid := cloneImg(eng.RenderOnce())
	m.SetFlag(theme.FlagBackdropMica, true)
	if err := eng.ApplyThemeProfile(m); err != nil {
		t.Fatal(err)
	}
	mica := cloneImg(eng.RenderOnce())
	if equalImages(solid, mica) {
		t.Fatal("флаг Mica на лету не изменил кадр")
	}
	m.SetFlag(theme.FlagBackdropMica, false)
	_ = eng.ApplyThemeProfile(m)
	back := eng.RenderOnce()
	if !equalImages(solid, back) {
		t.Fatal("после снятия флага кадр не вернулся к сплошному")
	}
}

func cloneImg(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}

// SetMotionReduce переключает режим у менеджера и у виджетов.
func TestMaterials_EngineSetMotionReduce(t *testing.T) {
	m := theme.NewManager()
	_ = theme.RegisterBuiltinProfiles(m)
	_ = m.SetTheme(theme.ProfileWindows11)
	eng := engine.New(100, 100, 30)
	defer widget.SetReduceMotion(false)
	if err := eng.SetMotionReduce(m, true); err != nil {
		t.Fatal(err)
	}
	if !m.MotionReduced() || !widget.ReduceMotion() {
		t.Errorf("режим не включён: менеджер %v, виджеты %v", m.MotionReduced(), widget.ReduceMotion())
	}
	if err := eng.SetMotionReduce(m, false); err != nil {
		t.Fatal(err)
	}
	if m.MotionReduced() || widget.ReduceMotion() {
		t.Errorf("режим не выключен: менеджер %v, виджеты %v", m.MotionReduced(), widget.ReduceMotion())
	}
	if err := eng.SetMotionReduce(nil, true); err == nil {
		t.Error("без менеджера должна быть ошибка")
	}
}
