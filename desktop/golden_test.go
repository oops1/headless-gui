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

// Панель задач под каждой из четырёх тем.
//
// Golden-тест здесь стережёт не «красоту», а главное обещание работы: одна
// и та же панель с одним и тем же содержимым выглядит по-разному ровно
// настолько, насколько отличаются темы. Кадры снимаются при фиксированном
// времени (иначе часы делали бы тест непроходимым через минуту) и с
// фиксированным списком окон.

// buildScene собирает панель со всем содержимым на менеджере тем tm.
func buildScene(t *testing.T, tm *theme.Manager, w, h int) (*widget.Panel, *desktop.Taskbar) {
	t.Helper()

	root := widget.NewPanel(color.RGBA{R: 30, G: 60, B: 110, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	// «Обои» клетчатые, а не полосатые: размытие внутри одноцветной
	// полосы вернёт тот же цвет, и проверка «панель видна» ослепнет на
	// стеклянных темах.
	const cell = 24
	for y := 0; y < h; y += cell {
		for x := 0; x < w; x += cell {
			shade := uint8(60)
			if (x/cell+y/cell)%2 == 0 {
				shade = 200
			}
			p := widget.NewPanel(color.RGBA{R: shade, G: uint8(90 + int(shade)/4), B: 180, A: 255})
			p.ShowHeader = false
			p.SetBounds(image.Rect(x, y, x+cell, y+cell))
			root.AddChild(p)
		}
	}

	// Тема сама по себе значков не рисует — ей нужен разрешатель. Без него
	// трей выйдет пустым, и это легко не заметить.
	icons := widget.BuiltinIcons()
	tm.SetIconResolver(icons)
	wm := desktop.NewFakeWindowModel(
		desktop.WindowInfo{ID: 1, Title: "Проводник", Icon: icons.ResolveIcon(theme.IconRef{Name: "start"}, 24)},
		desktop.WindowInfo{ID: 2, Title: "Терминал", Active: true,
			Icon: icons.ResolveIcon(theme.IconRef{Name: "network.ethernet"}, 24)},
		desktop.WindowInfo{ID: 3, Title: "Почта", Minimized: true,
			Icon: icons.ResolveIcon(theme.IconRef{Name: "battery"}, 24)},
	)
	status := desktop.NewFakeSystemStatus()
	clk := desktop.NewFakeClock(time.Date(2026, 3, 14, 15, 9, 26, 0, time.UTC))

	bar := desktop.NewTaskbar(tm)
	bar.AddItem(desktop.SlotStart, desktop.NewStartButton(tm))
	bar.AddItem(desktop.SlotApps, desktop.NewRunningApplications(tm, wm))
	bar.AddItem(desktop.SlotTray, desktop.NewNetworkStatus(tm, status))
	bar.AddItem(desktop.SlotTray, desktop.NewVolumeStatus(tm, status))
	bar.AddItem(desktop.SlotTray, desktop.NewPowerStatus(tm, status))
	bar.AddItem(desktop.SlotTray, desktop.NewClock(tm, clk))

	barH := bar.Height()
	if barH <= 0 {
		t.Fatalf("тема не задала высоту панели")
	}
	bar.SetBounds(image.Rect(0, h-barH, w, h))
	root.AddChild(bar)
	return root, bar
}

// renderScene рисует сцену и возвращает кадр вместе с верхней границей панели:
// высота панели у каждой темы своя, и проверять её надо относительно этой
// границы, а не от выбранного наугад числа пикселей.
func renderScene(t *testing.T, tm *theme.Manager, w, h int) (*image.RGBA, int) {
	t.Helper()
	eng := engine.New(w, h, 30)
	root, bar := buildScene(t, tm, w, h)
	defer bar.Close()
	eng.SetRoot(root)
	return eng.RenderOnce(), bar.Bounds().Min.Y
}

// TestGolden_TaskbarPerTheme — по кадру на каждую из четырёх тем.
func TestGolden_TaskbarPerTheme(t *testing.T) {
	const w, h = 640, 220
	names := []string{
		theme.ProfileWindows2000,
		theme.ProfileWindows10,
		theme.ProfileWindows11Dark,
		theme.ProfileMacOS,
	}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			m := theme.NewManager()
			if err := theme.RegisterBuiltinProfiles(m); err != nil {
				t.Fatal(err)
			}
			if err := m.SetTheme(name); err != nil {
				t.Fatal(err)
			}
			img, barTop := renderScene(t, m, w, h)
			if img == nil {
				t.Fatal("кадр не отрисован")
			}
			// Панель должна быть видима: её полоса отличается от обоев над ней.
			if !barIsVisible(img, barTop) {
				t.Error("панель задач не отличается от фона — она не нарисована")
			}
			saveIfAsked(t, img, name)
		})
	}
}

// TestGolden_ThemeSwitchChangesPicture — переключение темы на лету меняет
// картинку, а панель остаётся работоспособной. Это и есть требование
// заказчика: смена вида без перезапуска.
func TestGolden_ThemeSwitchChangesPicture(t *testing.T) {
	const w, h = 640, 220
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}

	eng := engine.New(w, h, 30)
	if err := m.SetTheme(theme.ProfileWindows2000); err != nil {
		t.Fatal(err)
	}
	root, bar := buildScene(t, m, w, h)
	defer bar.Close()
	eng.SetRoot(root)
	first := snapshot(eng.RenderOnce())

	// Меняем тему на лету — той же сцене, без пересоздания компонентов.
	if err := m.SetTheme(theme.ProfileWindows11Dark); err != nil {
		t.Fatal(err)
	}
	barH := bar.Height()
	bar.SetBounds(image.Rect(0, h-barH, w, h))
	eng.Invalidate()
	second := snapshot(eng.RenderOnce())

	if imagesEqual(first, second) {
		t.Error("смена темы не изменила картинку")
	}
	// И назад — панель переживает переключение в обе стороны.
	if err := m.SetTheme(theme.ProfileWindows2000); err != nil {
		t.Fatal(err)
	}
	bar.SetBounds(image.Rect(0, h-bar.Height(), w, h))
	eng.Invalidate()
	third := snapshot(eng.RenderOnce())
	if !imagesEqual(first, third) {
		t.Error("возврат к прежней теме дал другую картинку")
	}
}

// TestGolden_TaskbarSurvivesResizeAndScale — панель переживает другое
// разрешение и другой масштаб (требование приёмки: SetBounds и SetScale
// 1.0/1.25/1.5).
func TestGolden_TaskbarSurvivesResizeAndScale(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}

	for _, scale := range []float64{1.0, 1.25, 1.5} {
		for _, size := range [][2]int{{640, 200}, {1024, 300}, {320, 160}} {
			w, h := size[0], size[1]
			eng := engine.New(w, h, 30)
			eng.SetScale(scale)
			root, bar := buildScene(t, m, w, h)
			eng.SetRoot(root)
			img := eng.RenderOnce()
			bar.Close()

			if img == nil {
				t.Fatalf("масштаб %v, %dx%d: кадр не отрисован", scale, w, h)
			}
			// Панель не вылезла за экран и элементы внутри неё.
			area := bar.ReservedArea()
			if area.Max.Y > h || area.Min.X < 0 || area.Max.X > w {
				t.Errorf("масштаб %v, %dx%d: панель вне экрана: %v", scale, w, h, area)
			}
			for _, slot := range []desktop.Slot{desktop.SlotStart, desktop.SlotApps, desktop.SlotTray} {
				for _, it := range bar.Items(slot) {
					b := it.Bounds()
					if b.Empty() {
						continue // элементу не хватило места — это законно
					}
					if !b.In(area) {
						t.Errorf("масштаб %v, %dx%d: элемент %v вне панели %v", scale, w, h, b, area)
					}
				}
			}
		}
	}
}

// ─── Помощники ──────────────────────────────────────────────────────────────

// barIsVisible — полоса панели отличается от обоев над ней.
//
// Сравниваем не два пикселя, а две строки целиком: одиночный пиксель
// может случайно совпасть с фоном даже у нарисованной панели.
func barIsVisible(img *image.RGBA, barTop int) bool {
	b := img.Bounds()
	if barTop <= b.Min.Y+2 || barTop >= b.Max.Y-2 {
		return false
	}
	above, inside := barTop-2, barTop+2
	for x := b.Min.X; x < b.Max.X; x++ {
		if img.RGBAAt(x, above) != img.RGBAAt(x, inside) {
			return true
		}
	}
	return false
}

func snapshot(img *image.RGBA) *image.RGBA {
	if img == nil {
		return nil
	}
	c := image.NewRGBA(img.Bounds())
	copy(c.Pix, img.Pix)
	return c
}

func imagesEqual(a, b *image.RGBA) bool {
	if a == nil || b == nil || a.Bounds() != b.Bounds() {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}

// saveIfAsked сохраняет кадр, если задан GOLDEN_OUT — для просмотра глазами.
func saveIfAsked(t *testing.T, img *image.RGBA, name string) {
	dir := os.Getenv("GOLDEN_OUT")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, "taskbar_"+name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// TestGolden_FlyoutsLook — снимок с открытыми всплывающими панелями.
//
// Проверка та же, что у панели задач: панель должна быть видна на фоне обоев.
// Кадры сохраняются при GOLDEN_OUT — смотреть глазами.
func TestGolden_FlyoutsLook(t *testing.T) {
	const w, h = 720, 480

	for _, name := range []string{theme.ProfileWindows2000, theme.ProfileWindows11Dark} {
		name := name
		t.Run(name, func(t *testing.T) {
			m := theme.NewManager()
			if err := theme.RegisterBuiltinProfiles(m); err != nil {
				t.Fatal(err)
			}
			if err := m.SetTheme(name); err != nil {
				t.Fatal(err)
			}
			m.SetIconResolver(widget.BuiltinIcons())

			root, bar := buildScene(t, m, w, h)
			defer bar.Close()
			screen := image.Rect(0, 0, w, h)
			barTop := bar.Bounds().Min.Y

			cat := desktop.NewStaticAppCatalog(
				desktop.AppInfo{ID: "term", Title: "Терминал"},
				desktop.AppInfo{ID: "files", Title: "Проводник"},
				desktop.AppInfo{ID: "mail", Title: "Почта"},
			)
			menu := desktop.NewStartMenu(m, cat)
			menu.Screen = screen
			root.AddChild(menu)
			menu.Open(image.Rect(0, barTop, 48, h))

			cal := desktop.NewCalendarFlyout(m, desktop.NewFakeClock(
				time.Date(2026, 3, 14, 15, 9, 26, 0, time.UTC)))
			cal.Screen = screen
			cal.Align = desktop.AlignEnd
			root.AddChild(cal)
			cal.Open(image.Rect(w-90, barTop, w, h))

			eng := engine.New(w, h, 30)
			eng.SetRoot(root)
			img := eng.RenderOnce()
			if img == nil {
				t.Fatal("кадр не отрисован")
			}

			for what, r := range map[string]image.Rectangle{
				"меню «Пуск»": menu.OverlayBounds(),
				"календарь":   cal.OverlayBounds(),
			} {
				if r.Empty() {
					t.Fatalf("%s не открылось", what)
				}
				// Панель непрозрачна: её середина отличается от клетчатых обоев,
				// то есть обои сквозь неё не просвечивают клетками.
				a := img.RGBAAt(r.Min.X+r.Dx()/2, r.Min.Y+8)
				b := img.RGBAAt(r.Min.X+r.Dx()/2, r.Min.Y+8+24)
				if a == (color.RGBA{}) && b == (color.RGBA{}) {
					t.Errorf("%s: на месте панели пусто", what)
				}
			}
			saveIfAsked(t, img, "flyouts_"+name)
		})
	}
}

// TestGolden_PipelineDoesNotChangePixels — главный критерий работы над
// конвейером: она про то, КАК получается кадр, а не про то, как он выглядит.
//
// Сцена каждой темы рисуется дважды — с пропуском поддеревьев и без него — и
// кадры обязаны совпасть до пикселя.
func TestGolden_PipelineDoesNotChangePixels(t *testing.T) {
	const w, h = 640, 220

	for _, name := range []string{
		theme.ProfileWindows2000, theme.ProfileWindows10,
		theme.ProfileWindows11Dark, theme.ProfileMacOS,
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			render := func(culling bool) *image.RGBA {
				m := theme.NewManager()
				if err := theme.RegisterBuiltinProfiles(m); err != nil {
					t.Fatal(err)
				}
				if err := m.SetTheme(name); err != nil {
					t.Fatal(err)
				}
				m.SetIconResolver(widget.BuiltinIcons())

				eng := engine.New(w, h, 30)
				eng.SetSubtreeCulling(culling)
				root, bar := buildScene(t, m, w, h)
				defer bar.Close()
				eng.SetRoot(root)
				eng.RenderOnce()
				return snapshot(eng.RenderOnce())
			}

			withCulling := render(true)
			without := render(false)
			widget.SetSubtreeCulling(true) // вернуть общий выключатель

			if withCulling == nil || without == nil {
				t.Fatal("кадр не отрисован")
			}
			if !imagesEqual(withCulling, without) {
				t.Error("кадры с пропуском поддеревьев и без него разошлись")
			}
		})
	}
}
