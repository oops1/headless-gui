package desktop_test

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Общая сцена тестов быстрых настроек Windows 11: рабочий стол с панелью задач
// и открытой панелью. Кадры сохраняются при QS_OUT (каталог) — смотреть глазами.

// qsSampleActions — плитки как у Windows 11 24H2: часть с «›», выключенные,
// недоступная и отключённая.
func qsSampleActions() *desktop.QuickActionList {
	ink := color.RGBA{A: 255}
	return desktop.NewQuickActionList(
		desktop.QuickAction{ID: "wifi", Title: "Wi-Fi", Detail: "Avanpost", Icon: ncGlyph(32, ink, 1), On: true, HasDetails: true},
		desktop.QuickAction{ID: "bt", Title: "Bluetooth", Detail: "Не подключено", Icon: ncGlyph(32, ink, 2), On: true, HasDetails: true},
		desktop.QuickAction{ID: "plane", Title: "Режим «в самолёте»", Icon: ncGlyph(32, ink, 0)},
		desktop.QuickAction{ID: "saver", Title: "Экономия заряда", Icon: ncGlyph(32, ink, 1), Unavailable: true, HasDetails: true},
		desktop.QuickAction{ID: "night", Title: "Ночной свет", Icon: ncGlyph(32, ink, 2)},
		desktop.QuickAction{ID: "access", Title: "Специальные возможности", Icon: ncGlyph(32, ink, 0), HasDetails: true},
		desktop.QuickAction{ID: "vpn", Title: "VPN", Detail: "Не подключено", Icon: ncGlyph(32, ink, 1), HasDetails: true},
		desktop.QuickAction{ID: "focus", Title: "Фокусировка", Icon: ncGlyph(32, ink, 2), Disabled: true},
	)
}

// qsSampleDetails — вложенная страница: список устройств вывода.
func qsSampleDetails(id desktop.QuickActionID) *desktop.QuickDetails {
	lv := widget.NewListView("Динамики (Realtek Audio)", "Наушники (USB Audio)", "HDMI (Intel Display Audio)")
	return &desktop.QuickDetails{Content: lv}
}

// qsOpts — что менять в снимке.
type qsOpts struct {
	dark     bool
	theme    string // профиль; пусто — Windows 11 (по dark)
	reduce   bool   // «меньше движения»
	mica     bool   // Mica и мягкие тени поверх обоев
	scale    float64
	w, h     int
	noBright bool
	model    func(m *desktop.QuickActionList)
	status   func(s *desktop.FakeSystemStatus)
	setup    func(q *desktop.QuickSettings)
	after    func(q *desktop.QuickSettings)
}

// qsScene — собранная сцена.
type qsScene struct {
	t      *testing.T
	eng    *engine.Engine
	q      *desktop.QuickSettings
	model  *desktop.QuickActionList
	status *desktop.FakeSystemStatus
	tm     *theme.Manager
	root   *widget.Panel
	bar    *desktop.Taskbar
}

// qsBuild собирает рабочий стол Windows 11 с открытыми быстрыми настройками.
func qsBuild(t *testing.T, o qsOpts) *qsScene {
	t.Helper()
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	name := theme.ProfileWindows11
	if o.dark {
		name = theme.ProfileWindows11Dark
	}
	if o.theme != "" {
		name = o.theme
	}
	if err := m.SetTheme(name); err != nil {
		t.Fatal(err)
	}
	if o.reduce {
		m.SetFlag(theme.FlagMotionReduce, true)
	}
	m.SetIconResolver(widget.BuiltinIcons())
	w, h := o.w, o.h
	if w == 0 {
		w, h = 1280, 760
	}
	root, bar := buildScene(t, m, w, h)
	t.Cleanup(bar.Close)

	status := desktop.NewFakeSystemStatus()
	status.SetPower(desktop.PowerState{Charge: 0.87, OnAC: false})
	if o.status != nil {
		o.status(status)
	}
	model := qsSampleActions()
	if o.model != nil {
		o.model(model)
	}
	q := desktop.NewQuickSettings(m, status)
	q.SetQuickActions(model)
	q.VolumeDetails = true
	q.Details = qsSampleDetails
	if !o.noBright {
		q.SetBrightness(0.7)
	}
	q.Screen = image.Rect(0, 0, w, h)
	if o.setup != nil {
		o.setup(q)
	}
	root.AddChild(q)
	t.Cleanup(q.Close)
	t.Cleanup(widget.StopAllAnimations)

	scale := o.scale
	if scale <= 0 {
		scale = 1
	}
	eng := engine.New(w, h, 30)
	eng.SetScale(scale)
	if o.mica {
		m.SetFlag(theme.FlagBackdropMica, true)
		m.SetFlag(theme.FlagShadowSoft, true)
		if err := eng.SetBackground(matWallpaper(w, h)); err != nil {
			t.Fatal(err)
		}
		if err := eng.ApplyThemeProfile(m); err != nil {
			t.Fatal(err)
		}
	}
	eng.SetRoot(root)
	bb := bar.Bounds()
	q.Open(image.Rect(w-150, bb.Min.Y, w-100, bb.Max.Y))
	q.Settle()
	if o.after != nil {
		o.after(q)
	}
	return &qsScene{t: t, eng: eng, q: q, model: model, status: status, tm: m, root: root, bar: bar}
}

// render снимает кадр.
func (s *qsScene) render() *image.RGBA {
	s.eng.Invalidate()
	return s.eng.RenderOnce()
}

// qsSave пишет кадр в каталог QS_OUT, если он задан.
func qsSave(t *testing.T, img *image.RGBA, name string) {
	dir := os.Getenv("QS_OUT")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// qsClick нажимает и отпускает левую кнопку в точке.
func qsClick(q *desktop.QuickSettings, pt image.Point) {
	q.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true})
	q.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft})
}

// qsKey нажимает клавишу.
func qsKey(q *desktop.QuickSettings, code widget.KeyCode, mod widget.KeyMod) {
	q.OnKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: true})
}

// qsFinish доводит все идущие анимации (переход страниц, выезд) до конца.
func qsFinish() {
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(time.Hour))
}
