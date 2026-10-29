package core

// initial.go — начальное положение окна и «развёрнуто» до его показа
// (задание PixGo E18).
//
// Приложение хранит положение и признак «развёрнуто» между запусками, но
// восстановить их было нечем: окно создавалось по центру экрана и тут же
// показывалось, а Native() появлялся только после этого. Перенос и Maximize()
// после показа дают видимый прыжок и анимацию разворачивания при каждом
// запуске. Теперь значения задаются ДО Run и попадают в окно в момент
// создания, когда оно ещё не видно:
//
//	win := window.New(eng, "Заголовок")
//	win.SetInitialBounds(x, y, w, h) // где и какого размера
//	win.SetInitialMaximized(true)    // и сразу развёрнуто
//	win.Run()
//
//	Win32   CreateWindowEx получает x, y и размер сразу; развёрнутое окно
//	        создаётся скрытым и первым же ShowWindow(SW_SHOWMAXIMIZED)
//	        показывается развёрнутым — система запоминает прямоугольник
//	        создания как «нормальный», и «восстановить» вернёт его;
//	X11     CreateWindow с x, y; WM_NORMAL_HINTS с USPosition|USSize (и
//	        PPosition|PSize), чтобы менеджер не переставил окно по-своему;
//	        развёрнутому окну _NET_WM_STATE_MAXIMIZED_VERT/HORZ ставится
//	        свойством ДО MapWindow — так EWMH задаёт начальное состояние;
//	Wayland положение задаёт компоновщик: x, y игнорируются, w и h — начальный
//	        размер; развёрнутому окну xdg_toplevel.set_maximized уходит до
//	        первого commit;
//	Cocoa   не поддерживается (no-op): начальный размер окно и так берёт, а
//	        положение (SetPosition) на этом бэкенде не реализовано вовсе.

import (
	"image"
	"os"
	"runtime"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// initialState — начальные значения окна, заданные приложением до Run.
type initialState struct {
	// boundsSet — SetInitialBounds вызывался. Положение применяется всегда;
	// размер — только если w и h положительны.
	boundsSet  bool
	x, y, w, h int
	maximized  bool
	// sizeSet — размер задан отдельно от положения: Window.SetSize до Run.
	sizeSet bool
}

// hasSize — начальный размер задан (и положителен по обеим осям).
func (s initialState) hasSize() bool { return (s.boundsSet || s.sizeSet) && s.w > 0 && s.h > 0 }

// SetInitialBounds задаёт, где и какого размера появится окно. Вызывается до
// Run; значения применяются один раз, при создании окна ОС, без видимого
// прыжка.
//
// Единицы те же, что у остального API окна:
//
//   - x, y — ЭКРАННЫЕ координаты левого верхнего угла окна, как их отдают
//     NativeWindow.GetPosition и Monitor.Bounds (физические пиксели
//     рабочего стола; на Win32 и X11 монитор может лежать и в отрицательных
//     координатах);
//   - w, h — ЛОГИЧЕСКИЕ пиксели, как размер холста движка и SetMinSize:
//     окно умножает их на масштаб интерфейса. Масштаб берётся у монитора,
//     на который попадает прямоугольник (если его удалось определить), иначе
//     системный: сохранённое «800×600» остаётся тем же на глаз и на втором
//     мониторе с другим DPI.
//
// w или h не больше нуля — размер не задаётся (остаётся размер холста
// движка, либо Width/Height корневого widget.Window), а положение
// применяется всё равно. Размер из SetInitialBounds важнее размера из
// разметки корня.
//
// Положение не проверяется: за видимое место отвечает приложение (MonitorForRect
// и Monitors подскажут, лежит ли прямоугольник на каком-либо мониторе). На
// Wayland x, y игнорируются — положение окна задаёт компоновщик.
//
// Вызов после Run ничего не меняет: для работающего окна положение и размер
// задают прежним путём (Native().SetPosition/SetSize, виджетное окно).
// Совместим с SetFullscreen до Run: окно сперва создаётся с этими границами,
// затем встаёт на весь экран, а выход из него вернёт именно их (и состояние
// «развёрнуто», если оно было задано).
func (win *Window) SetInitialBounds(x, y, w, h int) {
	win.initial.boundsSet = true
	win.initial.x, win.initial.y, win.initial.w, win.initial.h = x, y, w, h
}

// SetInitialMaximized просит показать окно сразу развёрнутым. Вызывается до
// Run; применяется один раз. false — обычный показ (вместе с SetInitialBounds
// прямоугольник и есть положение окна; вместе с true — тот, в который окно
// вернётся по «восстановить»).
//
// После старта IsMaximized отвечает true: на Win32 это IsZoomed, на X11 и
// Wayland — состояние окна, о котором сообщил оконный менеджер/компоновщик (у
// X11 до его ответа — запрошенное значение). Компоновщик или менеджер вправе
// отказать, тогда окно появится обычным, а IsMaximized это покажет.
func (win *Window) SetInitialMaximized(v bool) {
	win.initial.maximized = v
}

// initialPlaceBackend — бэкенд принимает начальное положение до Create.
// Отдельный интерфейс по той же причине, что fullscreenBackend: сигнатуру
// NativeWindow менять нельзя. Бэкенд без него (Cocoa) просто создаёт окно как
// раньше.
type initialPlaceBackend interface {
	SetInitialPlacement(p native.InitialPlacement)
}

// placement собирает native.InitialPlacement из заданного приложением. ok=false —
// приложение ничего не просило.
func (s initialState) placement() (native.InitialPlacement, bool) {
	if !s.boundsSet && !s.maximized {
		return native.InitialPlacement{}, false
	}
	return native.InitialPlacement{
		PosSet:    s.boundsSet,
		X:         s.x,
		Y:         s.y,
		SizeSet:   s.hasSize(),
		Maximized: s.maximized,
	}, true
}

// applyInitialPlacement отдаёт значения бэкенду. Зовётся из bringUp перед Create.
func (win *Window) applyInitialPlacement() {
	p, ok := win.initial.placement()
	if !ok {
		return
	}
	if b, ok := win.native.(initialPlaceBackend); ok {
		b.SetInitialPlacement(p)
	}
}

// applyInitialSize принимает начальный размер как размер окна: холст движка и
// границы корня. Зовётся из bringUp после syncFromWidgetWindow (размер из
// SetInitialBounds важнее разметки) и до расчёта физического размера.
func (win *Window) applyInitialSize() {
	if !win.initial.hasSize() {
		return
	}
	win.w, win.h = win.initial.w, win.initial.h
	if rs, ok := win.eng.(interface{ SetResolution(w, h int) }); ok {
		rs.SetResolution(win.w, win.h)
	}
	if root := win.eng.Root(); root != nil {
		root.SetBounds(image.Rect(0, 0, win.w, win.h))
	}
}

// initialScaleFor выбирает масштаб интерфейса для окна, которое появится в
// прямоугольнике r (экранные координаты): масштаб монитора с наибольшей долей
// площади, иначе монитора с левым верхним углом, иначе base. Окно, созданное
// на мониторе с другим DPI, иначе получило бы масштаб основного: Windows при
// создании WM_DPICHANGED не шлёт, и интерфейс выглядел бы вдвое мельче или
// крупнее до первого переноса. r считается на масштабе base.
func initialScaleFor(list []native.Monitor, r image.Rectangle, base float64) float64 {
	m, ok := MonitorForRect(list, r)
	if !ok {
		m, ok = monitorAt(list, r.Min.X, r.Min.Y)
	}
	if !ok || m.Scale <= 0 {
		return base
	}
	return m.Scale
}

// initialPhysicalRect — прямоугольник окна в экранных пикселях при масштабе k:
// левый верхний угол как задан, размер — логический, умноженный на k.
func initialPhysicalRect(x, y, w, h int, k float64) image.Rectangle {
	if k <= 0 {
		k = 1
	}
	return image.Rect(x, y, x+int(float64(w)*k+0.5), y+int(float64(h)*k+0.5))
}

// initialPerMonitorScale — у каждого монитора платформы свой масштаб, и окно
// надо создавать в масштабе того, на котором оно появится (Win32).
// initialMonitorList — откуда брать мониторы. Переменные, чтобы тест мог
// подложить свои на любой платформе.
var (
	initialPerMonitorScale = runtime.GOOS == "windows"
	initialMonitorList     = Monitors
)

// initialMonitorScale уточняет масштаб интерфейса по месту, где окно
// появится: base — масштаб, найденный detectScale. Только Win32: там
// масштаб у каждого монитора свой, а окно, созданное на другом мониторе, DPI
// монитора уже имеет (WM_DPICHANGED при создании не приходит). X11 знает
// один Xft.dpi на всех, на Wayland положение выбирает компоновщик. Явное
// HEADLESS_GUI_SCALE важнее.
func (win *Window) initialMonitorScale(base float64) float64 {
	// Монитор выбирается по положению: размер без положения (Window.SetSize
	// до Run) его не определяет.
	if !initialPerMonitorScale || !win.initial.boundsSet || !win.initial.hasSize() {
		return base
	}
	if os.Getenv("HEADLESS_GUI_SCALE") != "" {
		return base
	}
	list := initialMonitorList()
	if len(list) == 0 {
		return base
	}
	r := initialPhysicalRect(win.initial.x, win.initial.y, win.initial.w, win.initial.h, base)
	return initialScaleFor(list, r, base)
}
