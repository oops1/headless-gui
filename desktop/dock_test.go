package desktop

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// managerFor — менеджер со встроенными профилями и выбранной темой.
func managerFor(t *testing.T, name string) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(name); err != nil {
		t.Fatal(err)
	}
	return m
}

// threeWindows — модель с тремя окнами, второе активно.
func threeWindows() *FakeWindowModel {
	return NewFakeWindowModel(
		WindowInfo{ID: 1, Title: "Первое"},
		WindowInfo{ID: 2, Title: "Второе", Active: true},
		WindowInfo{ID: 3, Title: "Третье", Minimized: true},
	)
}

// TestDock_SameComponentDifferentShape — один и тот же компонент под
// Windows 11 рисуется полосой кнопок, под macOS — доком. Это требование
// приёмки: тема меняет форму, а не только палитру.
func TestDock_SameComponentDifferentShape(t *testing.T) {
	wm := threeWindows()

	win := NewRunningApplications(managerFor(t, theme.ProfileWindows11), wm)
	defer win.Close()
	win.SetBounds(image.Rect(0, 0, 400, 48))

	mac := NewRunningApplications(managerFor(t, theme.ProfileMacOS), wm)
	defer mac.Close()
	mac.SetBounds(image.Rect(0, 0, 400, 64))

	if PresenterFor(win.tm, PresenterKeyRunningApps) != nil {
		t.Error("под Windows 11 назначен презентер — там своя отрисовка не нужна")
	}
	p := PresenterFor(mac.tm, PresenterKeyRunningApps)
	if p == nil {
		t.Fatal("под macOS презентер дока не назначен")
	}

	// Отрисовка идёт разными путями, но поведение у компонента одно.
	winCtx, macCtx := &countingCtx{}, &countingCtx{}
	win.Draw(winCtx)
	mac.Draw(macCtx)
	if winCtx.images+winCtx.fills == 0 || macCtx.images+macCtx.fills == 0 {
		t.Fatal("одна из тем не нарисовала ничего")
	}
	// Док центрирует ряд: крайние ячейки не прижаты к краю области.
	if macCtx.minX <= mac.Bounds().Min.X {
		t.Errorf("док не центрирован: первая ячейка в x=%d при границе %d",
			macCtx.minX, mac.Bounds().Min.X)
	}
}

// TestDock_BehaviourIsIdenticalUnderBothThemes — активация и сворачивание
// работают одинаково под обеими темами: презентер меняет вид, а не логику.
func TestDock_BehaviourIsIdenticalUnderBothThemes(t *testing.T) {
	for _, name := range []string{theme.ProfileWindows11, theme.ProfileMacOS} {
		name := name
		t.Run(name, func(t *testing.T) {
			wm := threeWindows()
			r := NewRunningApplications(managerFor(t, name), wm)
			defer r.Close()
			r.SetBounds(image.Rect(0, 0, 600, 64))
			r.Draw(&countingCtx{}) // раскладка считается при отрисовке

			if len(r.btns) == 0 {
				t.Fatal("кнопки не разложены")
			}
			// Клик по первому (неактивному) окну — активация.
			first := r.btns[0].rect
			pt := image.Pt(first.Min.X+first.Dx()/2, first.Min.Y+first.Dy()/2)
			r.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true})
			r.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: false})

			if len(wm.Activated) != 1 || wm.Activated[0] != 1 {
				t.Errorf("активация не дошла до модели: %v", wm.Activated)
			}
		})
	}
}

// TestDock_MagnifiesUnderCursor — значок под курсором крупнее соседей.
func TestDock_MagnifiesUnderCursor(t *testing.T) {
	tm := managerFor(t, theme.ProfileMacOS)
	icon, mag, _ := dockMetrics(tm)
	if icon <= 0 || mag <= 1 {
		t.Fatalf("тема macOS не задала размеры дока: icon=%d mag=%v", icon, mag)
	}

	// Без курсора все значки одинаковы.
	if got := dockIconSize(icon, mag, 1, -1); got != icon {
		t.Errorf("без наведения значок = %d, ждали %d", got, icon)
	}
	// Под курсором — полный масштаб, у соседа — промежуточный, вдали — обычный.
	under := dockIconSize(icon, mag, 2, 2)
	near := dockIconSize(icon, mag, 3, 2)
	far := dockIconSize(icon, mag, 9, 2)
	if under <= near || near <= far {
		t.Errorf("увеличение не спадает от курсора: под=%d рядом=%d вдали=%d", under, near, far)
	}
	if far != icon {
		t.Errorf("вдали от курсора значок = %d, ждали обычный %d", far, icon)
	}
}

// TestDock_MeasureLeavesRoomForMagnification — док просит ширину с запасом,
// иначе увеличенный значок выпихивал бы соседей за край.
func TestDock_MeasureLeavesRoomForMagnification(t *testing.T) {
	wm := threeWindows()
	r := NewRunningApplications(managerFor(t, theme.ProfileMacOS), wm)
	defer r.Close()
	r.SetBounds(image.Rect(0, 0, 600, 64))

	icon, mag, gap := dockMetrics(r.tm)
	got := r.PreferredSize(image.Pt(600, 64))
	plain := 3*icon + 2*gap
	if got.X <= plain {
		t.Errorf("ширина дока %d не больше ряда без увеличения %d (запас %v)", got.X, plain, mag)
	}
}

// countingCtx — записывающий контекст: считает заливки и картинки и
// запоминает левую границу нарисованного.
type countingCtx struct {
	recCtx
	fills  int
	images int
	minX   int
}

func (c *countingCtx) FillRect(x, y, w, h int, col color.RGBA) {
	c.note(x)
	c.fills++
}

func (c *countingCtx) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	c.note(x)
	c.fills++
}

func (c *countingCtx) DrawImageScaled(src image.Image, x, y, w, h int) {
	c.note(x)
	c.images++
}

func (c *countingCtx) note(x int) {
	if c.fills == 0 && c.images == 0 {
		c.minX = x
		return
	}
	if x < c.minX {
		c.minX = x
	}
}
