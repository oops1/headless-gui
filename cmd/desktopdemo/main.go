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
	"time"

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

	// Всплывающие панели и кнопки, от которых они открываются: снимки
	// делаются с открытыми панелями, а открывать их надо тем же путём, что
	// и человек — от значка.
	startBtn *desktop.StartButton
	clock    *desktop.ClockItem
	tray     *desktop.SystemTray
	menu     *desktop.StartMenu
	cal      *desktop.CalendarFlyout
	quick    *desktop.QuickSettings
	center   *desktop.NotificationCenter

	// setAutoHide и revealByCursor — то же, что делает человек кнопкой и
	// подведением курсора к краю: снимки должны показывать настоящее
	// поведение, а не выставленное поле.
	setAutoHide    func(bool)
	revealByCursor func()
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
	// Модель с миниатюрами: на неё опирается предпросмотр окна при наведении
	// на кнопку панели задач.
	wm := desktop.NewFakeWindowPreviews(
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

	// Предпросмотр окна: цепляется к области значков и дальше следит за
	// наведением сам.
	preview := desktop.NewWindowPreview(tm, wm)
	preview.Track(apps)

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
	dock.StyleComponent = desktop.ComponentDockbar
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
	preview.Screen = screen
	root.AddChild(menu)
	root.AddChild(quick)
	root.AddChild(center)
	root.AddChild(cal)
	// Предпросмотр — такой же оверлей, как остальные панели: в дереве он
	// обязан быть, иначе движок его не найдёт.
	root.AddChild(preview)

	// Уведомления и календарь Windows показывает один над другим — значит
	// они группа: клик по числу календаря не должен гасить центр уведомлений.
	desktop.NewFlyoutGroup(cal.Flyout, center.Flyout)
	tray.Overflow().Screen = screen
	// Раскрывающаяся область трея — оверлей, и движок находит оверлеи обходом
	// ДЕРЕВА: без этой строки шеврон открывался бы, а показывать было бы
	// нечего. SystemTray не добавляет её себе в дети сам — она всплывает над
	// панелью, а не живёт внутри неё.
	root.AddChild(tray.Overflow())

	// Одна панель за раз: открывая любую, закрываем остальные — иначе они
	// перекрывают друг друга, чего на настоящем рабочем столе не бывает.
	// Список по интерфейсу, а не по типам: Close() есть у каждой панели, и
	// перечисление типов в switch пришлось бы дополнять при появлении новой —
	// причём молча, ничего не сломав: забытый тип просто не закрывался бы.
	type closable interface{ Close() }
	panels := []closable{menu, quick, center, cal}
	closeOthers := func(keep closable) {
		for _, p := range panels {
			if p != keep {
				p.Close()
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
	switcher.SetBounds(image.Rect(16, 16, 16+len(themeOrder)*150+170, 52))
	for i, th := range themeOrder {
		i := i
		btn := widget.NewButton(th.label)
		btn.SetBounds(image.Rect(20+i*150, 20, 20+i*150+140, 48))
		btn.OnClick = func() { apply(i) }
		switcher.AddChild(btn)
	}
	root.AddChild(switcher)

	// Автоскрытие: панель уезжает за край и выезжает, когда курсор подведён
	// к самому краю экрана.
	hideBtn := widget.NewButton("Скрывать панель")
	hideBtn.SetBounds(image.Rect(20+len(themeOrder)*150, 20, 20+len(themeOrder)*150+160, 48))
	hideBtn.OnClick = func() {
		on := !bar.AutoHide()
		bar.SetAutoHide(on)
		dock.SetAutoHide(on)
		if on {
			hideBtn.Text = "Показать панель"
		} else {
			hideBtn.Text = "Скрывать панель"
		}
		eng.Invalidate()
	}
	switcher.AddChild(hideBtn)

	hint := widget.NewLabel("Кнопки наверху меняют тему; «Пуск», часы и значки трея открывают панели",
		color.RGBA{R: 235, G: 240, B: 250, A: 255})
	hint.SetBounds(image.Rect(20, 60, 700, 84))
	root.AddChild(hint)

	arrange()
	return scene{
		root:  root,
		apply: apply,
		setAutoHide: func(on bool) {
			bar.SetAutoHide(on)
			dock.SetAutoHide(on)
		},
		revealByCursor: func() {
			// Курсор у того края, к которому прижата полоса.
			y := screenH - 1
			if bar.Edge() == desktop.EdgeTop {
				y = 0
			}
			bar.OnMouseMove(screenW/2, y)
			dock.OnMouseMove(screenW/2, screenH-1)
			settleAnimations()
		},
		startBtn: startBtn,
		clock:    clock,
		tray:     tray,
		menu:     menu,
		cal:      cal,
		quick:    quick,
		center:   center,
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

// settleAnimations доводит выезд панели до конца: снимок делается сразу, а
// анимация иначе застыла бы на середине.
func settleAnimations() {
	now := time.Now()
	for i := 0; i < 60 && widget.AnimationsActive(); i++ {
		now = now.Add(16 * time.Millisecond)
		widget.StepAnimations(now)
	}
}

// addWallpaper рисует «обои» — не одноцветные клетки, а картинку с мелкими
// деталями и переходами.
//
// Это не украшательство: сквозь стекло видно размытую подложку, а размытие
// одноцветного квадрата от него самого неотличимо. На плоских клетках любая
// стеклянная панель выглядит просто серым прямоугольником — и не разберёшь,
// работает размытие или нет.
func addWallpaper(root *widget.Panel) {
	// Небо: горизонтальные полосы с плавным переходом сверху вниз.
	const band = 8
	for y := 0; y < screenH; y += band {
		k := float64(y) / float64(screenH)
		p := widget.NewPanel(color.RGBA{
			R: uint8(20 + 70*k),
			G: uint8(60 + 90*k),
			B: uint8(130 + 60*k),
			A: 255,
		})
		p.ShowHeader = false
		p.SetBounds(image.Rect(0, y, screenW, y+band))
		root.AddChild(p)
	}

	// Светлые пятна разного размера — по ним и видно, что подложка размыта.
	spots := []struct {
		x, y, r int
		c       color.RGBA
	}{
		{160, 180, 90, color.RGBA{R: 90, G: 140, B: 220, A: 255}},
		{420, 120, 60, color.RGBA{R: 140, G: 190, B: 240, A: 255}},
		{760, 260, 120, color.RGBA{R: 60, G: 110, B: 190, A: 255}},
		{300, 430, 70, color.RGBA{R: 110, G: 170, B: 230, A: 255}},
		// Пятна НА ПУТИ полос: под доком и под строкой меню. Без них стекло
		// нечему показывать — размытие ровного фона от него неотличимо, и
		// панель выглядит просто светлым прямоугольником.
		{470, 610, 120, color.RGBA{R: 235, G: 150, B: 90, A: 255}},
		{700, 620, 80, color.RGBA{R: 120, G: 200, B: 150, A: 255}},
		{250, 12, 70, color.RGBA{R: 240, G: 190, B: 100, A: 255}},
		{880, 520, 50, color.RGBA{R: 150, G: 200, B: 245, A: 255}},
	}
	for _, s := range spots {
		// Пятно набирается кольцами: чем ближе к центру, тем светлее — так
		// у него мягкий край, который размытие заметно растягивает.
		for i := 4; i > 0; i-- {
			rad := s.r * i / 4
			p := widget.NewPanel(s.c)
			p.ShowHeader = false
			p.CornerRadius = rad
			p.SetBounds(image.Rect(s.x-rad, s.y-rad, s.x+rad, s.y+rad))
			root.AddChild(p)
		}
	}
}
