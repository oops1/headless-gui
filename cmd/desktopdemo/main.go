// desktopdemo — рабочий стол с системной панелью задач и живой сменой темы.
//
// Показывает то, ради чего затевался пакет desktop/: одни и те же компоненты
// под четырьмя обликами — Windows 11, Windows 10, Windows 2000 и macOS —
// переключаются на ходу, без перезапуска и без пересоздания виджетов. Меняется
// только активная тема; панель, кнопки, значки и всплывающие панели остаются
// теми же объектами.
//
// Запуск (из директории window):
//
//	go run ../cmd/desktopdemo
//
// Тема переключается кнопками в левом верхнем углу.
package main

import (
	"image"
	"image/color"
	"log"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/window"
)

const (
	screenW = 1000
	screenH = 640

	// dockPad — поля плавающей плашки дока по краям от значков.
	dockPad = 12
)

// themeOrder — порядок обхода тем кнопками.
var themeOrder = []struct {
	profile string
	label   string
}{
	{theme.ProfileWindows11, "Windows 11"},
	{theme.ProfileWindows11Dark, "Windows 11 Dark"},
	{theme.ProfileWindows10, "Windows 10"},
	{theme.ProfileWindows2000, "Windows 2000"},
	{theme.ProfileMacOS, "macOS"},
}

// scene — собранный рабочий стол: корень дерева, смена темы и освобождение.
//
// Смена темы отдана наружу отдельным полем, а не спрятана в кнопках: тест
// переключает темы ровно тем же способом, что и человек кнопкой.
type scene struct {
	root  *widget.Panel
	apply func(int)
	close func()
}

func main() {
	eng := engine.New(screenW, screenH, 60)
	sc := buildDesktop(eng)
	defer sc.close()
	eng.SetRoot(sc.root)

	win := window.New(eng, "GuiEngine — рабочий стол и смена тем")
	win.SetMaxFPS(60)
	if err := win.Run(); err != nil {
		log.Fatal(err)
	}
}

// buildDesktop собирает рабочий стол целиком и возвращает его корень вместе
// с функцией освобождения (компоненты держат подписки и секундный тик часов).
//
// Вынесено из main, чтобы ту же сцену можно было собрать в тесте: молча
// сломавшаяся демонстрация — обычное дело, если её никто не собирает.
func buildDesktop(eng *engine.Engine) scene {
	tm := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(tm); err != nil {
		log.Fatalf("профили тем: %v", err)
	}
	tm.SetIconResolver(widget.BuiltinIcons())
	if err := tm.SetTheme(theme.ProfileWindows11); err != nil {
		log.Fatalf("тема: %v", err)
	}

	root := widget.NewPanel(color.RGBA{R: 24, G: 48, B: 92, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, screenW, screenH))
	addWallpaper(root)

	// ─── Данные системы: в демонстрации — фейки движка, в оболочке их место
	// займут настоящие реализации тех же интерфейсов ───────────────────────
	icons := widget.BuiltinIcons()
	iconOf := func(name string, size int) image.Image {
		return icons.ResolveIcon(theme.IconRef{Name: name}, size)
	}
	wm := desktop.NewFakeWindowModel(
		desktop.WindowInfo{ID: 1, AppID: "files", Title: "Проводник", Icon: iconOf("start", 24)},
		desktop.WindowInfo{ID: 2, AppID: "term", Title: "Терминал", Active: true,
			Icon: iconOf("network.ethernet", 24)},
		desktop.WindowInfo{ID: 3, AppID: "mail", Title: "Почта", Minimized: true,
			Icon: iconOf("battery", 24)},
	)
	cat := desktop.NewStaticAppCatalog(
		desktop.AppInfo{ID: "files", Title: "Проводник", Icon: iconOf("start", 24)},
		desktop.AppInfo{ID: "term", Title: "Терминал", Icon: iconOf("network.ethernet", 24)},
		desktop.AppInfo{ID: "mail", Title: "Почта", Icon: iconOf("battery", 24)},
		desktop.AppInfo{ID: "calc", Title: "Калькулятор", Icon: iconOf("volume", 24)},
		desktop.AppInfo{ID: "paint", Title: "Графический редактор", Icon: iconOf("network.wifi", 24)},
	)
	cat.Pin("files")
	cat.Pin("term")
	status := desktop.NewFakeSystemStatus()
	notes := desktop.NewFakeNotifications()
	notes.Add(desktop.Notification{Title: "Обновление", Body: "Готово к установке"})
	notes.Add(desktop.Notification{Title: "Батарея", Body: "Заряд ниже 20%",
		Severity: desktop.SeverityWarning})

	// ─── Панель задач ───────────────────────────────────────────────────────
	bar := desktop.NewTaskbar(tm)

	startBtn := desktop.NewStartButton(tm)
	apps := desktop.NewApplicationArea(tm, cat, wm)

	tray := desktop.NewSystemTray(tm)
	net := desktop.NewNetworkStatus(tm, status)
	vol := desktop.NewVolumeStatus(tm, status)
	pwr := desktop.NewPowerStatus(tm, status)
	clock := desktop.NewClock(tm, desktop.SystemClock{})
	tray.AddItem(net)
	tray.AddItem(vol)
	tray.AddItem(pwr)

	// Вторая полоса: у macOS рабочий стол разделён надвое — строка меню
	// сверху и док снизу. Полоса создаётся всегда, но под темами Windows
	// остаётся пустой и нулевой высоты, то есть невидимой.
	dock := desktop.NewTaskbar(tm)
	root.AddChild(bar)
	root.AddChild(dock)

	// ─── Всплывающие панели ─────────────────────────────────────────────────
	screen := image.Rect(0, 0, screenW, screenH)
	menu := desktop.NewStartMenu(tm, cat)
	menu.Screen = screen
	quick := desktop.NewQuickSettings(tm, status)
	quick.Screen = screen
	quick.Align = desktop.AlignEnd
	center := desktop.NewNotificationCenter(tm, notes)
	center.Screen = screen
	center.Align = desktop.AlignEnd
	cal := desktop.NewCalendarFlyout(tm, desktop.SystemClock{})
	cal.Screen = screen
	cal.Align = desktop.AlignEnd
	root.AddChild(menu)
	root.AddChild(quick)
	root.AddChild(center)
	root.AddChild(cal)
	tray.Overflow().Screen = screen

	// Одна панель за раз: открывая любую, закрываем остальные — иначе они
	// перекрывают друг друга, чего на настоящем рабочем столе не бывает.
	closeOthers := func(keep any) {
		for _, p := range []any{menu, quick, center, cal} {
			if p == keep {
				continue
			}
			switch v := p.(type) {
			case *desktop.StartMenu:
				v.Close()
			case *desktop.QuickSettings:
				v.Close()
			case *desktop.NotificationCenter:
				v.Close()
			case *desktop.CalendarFlyout:
				v.Close()
			}
		}
	}

	startBtn.OnClick = func() {
		closeOthers(menu)
		menu.Toggle(startBtn.Bounds())
	}
	clock.OnClick = func() {
		closeOthers(cal)
		cal.Toggle(clock.Bounds())
	}
	net.OnClick = func() {
		closeOthers(quick)
		quick.Toggle(net.Bounds())
	}
	vol.OnClick = net.OnClick
	pwr.OnClick = func() {
		closeOthers(center)
		center.Toggle(pwr.Bounds())
	}

	// ─── Переключатель тем ──────────────────────────────────────────────────
	//
	// Кнопки не пересоздают ни один компонент: они только меняют активную
	// тему. Всё остальное — высота панели, форма кнопок, наличие дока,
	// размеры всплывающих панелей — приезжает из профиля.
	// arrange раскладывает компоненты по полосам так, как просит активная
	// тема, и ставит сами полосы к нужным краям экрана.
	//
	// Компоненты при этом НЕ пересоздаются: кнопка «Пуск», часы и значки
	// остаются теми же объектами, меняется только то, в какой полосе они
	// лежат. Иначе смена темы теряла бы их состояние — наведение, открытые
	// панели, подписки.
	arrange := func() {
		dockH := bar.DockHeight()
		if dockH > 0 {
			// Две полосы: строка меню (пуск и статусы) и док (приложения).
			bar.SetItems(desktop.SlotStart, startBtn)
			bar.SetItems(desktop.SlotApps)
			bar.SetItems(desktop.SlotTray, tray, clock)
			dock.SetItems(desktop.SlotApps, apps)
		} else {
			bar.SetItems(desktop.SlotStart, startBtn)
			bar.SetItems(desktop.SlotApps, apps)
			bar.SetItems(desktop.SlotTray, tray, clock)
			dock.SetItems(desktop.SlotApps)
		}

		barH := bar.Height()
		if barH <= 0 {
			barH = 40
		}
		if bar.Edge() == desktop.EdgeTop {
			bar.SetBounds(image.Rect(0, 0, screenW, barH))
		} else {
			bar.SetBounds(image.Rect(0, screenH-barH, screenW, screenH))
		}
		if dockH > 0 {
			// Док — плавающая плашка по содержимому, а не полоса во всю
			// ширину: этим он и отличается от панели задач. Ширину спрашиваем
			// у самой области приложений — сколько ей надо, столько и даём.
			want := apps.PreferredSize(image.Pt(screenW, dockH)).X + 2*dockPad
			if want > screenW {
				want = screenW
			}
			x := (screenW - want) / 2
			// Док не прилипает к краю экрана — под ним остаётся поле.
			dock.SetBounds(image.Rect(x, screenH-dockH-dockPad, x+want, screenH-dockPad))
		} else {
			dock.SetBounds(image.Rectangle{})
		}

		// Всплывающие панели раскрываются от своего края.
		edge := bar.Edge()
		menu.Edge, quick.Edge, center.Edge, cal.Edge = edge, edge, edge, edge
		tray.Overflow().Edge = edge
	}

	apply := func(i int) {
		current := i % len(themeOrder)
		if err := tm.SetTheme(themeOrder[current].profile); err != nil {
			log.Printf("тема %s: %v", themeOrder[current].profile, err)
			return
		}
		arrange()
		eng.Invalidate()
	}

	switcher := widget.NewPanel(color.RGBA{A: 90})
	switcher.ShowHeader = false
	switcher.SetBounds(image.Rect(16, 16, 16+len(themeOrder)*150, 52))
	for i, th := range themeOrder {
		i := i
		btn := widget.NewButton(th.label)
		btn.SetBounds(image.Rect(20+i*150, 20, 20+i*150+140, 48))
		btn.OnClick = func() { apply(i) }
		switcher.AddChild(btn)
	}
	root.AddChild(switcher)

	hint := widget.NewLabel("Кнопки наверху меняют тему; «Пуск», часы и значки трея открывают панели",
		color.RGBA{R: 235, G: 240, B: 250, A: 255})
	hint.SetBounds(image.Rect(20, 60, 700, 84))
	root.AddChild(hint)

	arrange()
	return scene{
		root:  root,
		apply: apply,
		close: func() {
			bar.Close()
			dock.Close()
			apps.Close()
			tray.Close()
			clock.Close()
			quick.Close()
			center.Close()
		},
	}
}

// addWallpaper рисует «обои» — сетку прямоугольников, на которой видно
// размытие подложки у стеклянных тем.
func addWallpaper(root *widget.Panel) {
	const cell = 64
	for y := 0; y < screenH; y += cell {
		for x := 0; x < screenW; x += cell {
			shade := uint8(40 + (x/cell*13+y/cell*29)%120)
			p := widget.NewPanel(color.RGBA{
				R: shade / 2, G: uint8(60 + int(shade)/3), B: uint8(110 + int(shade)/4), A: 255,
			})
			p.ShowHeader = false
			p.SetBounds(image.Rect(x, y, x+cell, y+cell))
			root.AddChild(p)
		}
	}
}
