package engine

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// pipeline_bench_test.go — бенчмарки конвейера отрисовки на сцене НАСТОЯЩЕГО
// рабочего стола (пакет desktop/), а не синтетической сетки лейблов.
//
// Собирается практически то же самое, что cmd/desktopdemo/main.go: обои,
// панель задач с кнопкой «Пуск», областью приложений, треем и часами, тема
// Windows 11. Разница с демонстрацией — данные системы фиксированы (включая
// время часов), а переключатель тем и всплывающие панели не нужны: их здесь
// никто не открывает, и они только усложнили бы сцену.
//
// Числа отсюда отвечают на вопрос: сколько стоит кадр в наиболее частых
// сценариях простоя рабочего стола (тик часов, наведение на кнопку,
// перетаскивание окна) и что даёт пропуск поддеревьев вне изменившейся
// области (Engine.SetSubtreeCulling).

const (
	pipeW = 1280
	pipeH = 800
)

// pipeDesktop — собранная для бенчмарков сцена рабочего стола и то, что
// нужно её закрыть (панель и часы держат подписки/тикер).
type pipeDesktop struct {
	eng      *Engine
	bar      *desktop.Taskbar
	startBtn *desktop.StartButton
	clock    *desktop.ClockItem
	closeAll func()
}

// buildPipelineDesktop строит рабочий стол 1280×800 под тему Windows 11 —
// тем же способом, что и cmd/desktopdemo/main.go: фейковые источники данных
// движка, часы с ФИКСИРОВАННЫМ временем (иначе бенчмарк невоспроизводим —
// от запуска к запуску менялась бы отрисованная строка часов и, значит,
// стоимость кадра).
func buildPipelineDesktop() pipeDesktop {
	eng := New(pipeW, pipeH, 60)

	tm := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(tm); err != nil {
		panic(err)
	}
	tm.SetIconResolver(widget.BuiltinIcons())
	if err := tm.SetTheme(theme.ProfileWindows11); err != nil {
		panic(err)
	}

	root := widget.NewPanel(color.RGBA{R: 24, G: 48, B: 92, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, pipeW, pipeH))
	addPipelineWallpaper(root)

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

	// Фиксированное время часов: 2026-08-28 14:35:00 — постоянная точка, а
	// не time.Now(). Меняющееся время меняло бы длину/содержимое строки часов
	// и делало бы измерения кадра невоспроизводимыми между прогонами.
	fixedTime := time.Date(2026, 8, 28, 14, 35, 0, 0, time.UTC)
	fakeClock := desktop.NewFakeClock(fixedTime)

	bar := desktop.NewTaskbar(tm)
	startBtn := desktop.NewStartButton(tm)
	apps := desktop.NewApplicationArea(tm, cat, wm)

	tray := desktop.NewSystemTray(tm)
	net := desktop.NewNetworkStatus(tm, status)
	vol := desktop.NewVolumeStatus(tm, status)
	pwr := desktop.NewPowerStatus(tm, status)
	clock := desktop.NewClock(tm, fakeClock)
	tray.AddItem(net)
	tray.AddItem(vol)
	tray.AddItem(pwr)

	bar.SetItems(desktop.SlotStart, startBtn)
	bar.SetItems(desktop.SlotApps, apps)
	bar.SetItems(desktop.SlotTray, tray, clock)

	barH := bar.Height()
	if barH <= 0 {
		barH = 48
	}
	if bar.Edge() == desktop.EdgeTop {
		bar.SetBounds(image.Rect(0, 0, pipeW, barH))
	} else {
		bar.SetBounds(image.Rect(0, pipeH-barH, pipeW, pipeH))
	}
	root.AddChild(bar)

	eng.SetRoot(root)

	return pipeDesktop{
		eng:      eng,
		bar:      bar,
		startBtn: startBtn,
		clock:    clock,
		closeAll: func() {
			bar.Close()
			apps.Close()
			tray.Close()
			clock.Close()
		},
	}
}

// addPipelineWallpaper — те же «обои» с полосами и мягкими пятнами, что и в
// desktopdemo: сквозь стеклянные панели видна размытая подложка, и на плоской
// заливке размытие от неё неотличимо, так что нагрузка на блюр была бы
// нереалистично лёгкой.
func addPipelineWallpaper(root *widget.Panel) {
	const band = 8
	for y := 0; y < pipeH; y += band {
		k := float64(y) / float64(pipeH)
		p := widget.NewPanel(color.RGBA{
			R: uint8(20 + 70*k),
			G: uint8(60 + 90*k),
			B: uint8(130 + 60*k),
			A: 255,
		})
		p.ShowHeader = false
		p.SetBounds(image.Rect(0, y, pipeW, y+band))
		root.AddChild(p)
	}

	spots := []struct {
		x, y, r int
		c       color.RGBA
	}{
		{160, 180, 90, color.RGBA{R: 90, G: 140, B: 220, A: 255}},
		{420, 120, 60, color.RGBA{R: 140, G: 190, B: 240, A: 255}},
		{760, 260, 120, color.RGBA{R: 60, G: 110, B: 190, A: 255}},
		{300, 430, 70, color.RGBA{R: 110, G: 170, B: 230, A: 255}},
		{470, 760, 120, color.RGBA{R: 235, G: 150, B: 90, A: 255}},
		{700, 770, 80, color.RGBA{R: 120, G: 200, B: 150, A: 255}},
		{250, 12, 70, color.RGBA{R: 240, G: 190, B: 100, A: 255}},
		{1080, 650, 50, color.RGBA{R: 150, G: 200, B: 245, A: 255}},
	}
	for _, s := range spots {
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

// BenchmarkPipeline_FullFrame — полная перерисовка всего рабочего стола
// (Invalidate() перед каждым кадром). Верхняя граница стоимости кадра: то,
// что стоил бы любой кадр без пропуска поддеревьев и без diff — например,
// первый кадр после смены темы или resize окна.
func BenchmarkPipeline_FullFrame(b *testing.B) {
	sc := buildPipelineDesktop()
	b.Cleanup(sc.closeAll)
	eng := sc.eng
	eng.renderFrame() // прогрев: первый кадр синхронизирует front-буфер
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng.Invalidate()
		eng.renderFrame()
	}
}

// BenchmarkPipeline_ClockTick — меняются только часы: инвалидируется
// небольшая область в правом нижнем углу панели (примерно место часов,
// ~90×40), остальной рабочий стол не тронут. Это ГЛАВНЫЙ сценарий простоя
// настоящего рабочего стола — часы тикают каждую секунду, пока пользователь
// ничего не делает, и кадр должен стоить копейки: пропуск поддеревьев вне
// изменившейся области должен исключить обои, кнопку «Пуск» и область
// приложений из работы кадра.
func BenchmarkPipeline_ClockTick(b *testing.B) {
	sc := buildPipelineDesktop()
	b.Cleanup(sc.closeAll)
	eng := sc.eng
	eng.SetRenderOnDemand(true)
	eng.renderFrame()

	// Область часов — правый нижний угол панели задач (панель прижата к
	// нижнему краю экрана под темой Windows 11).
	clockRect := image.Rect(pipeW-98, pipeH-44, pipeW-8, pipeH-4)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng.InvalidateRect(clockRect)
		eng.renderFrame()
	}
}

// BenchmarkPipeline_ClockTickNoCulling — тот же тик часов, но с выключенным
// пропуском поддеревьев (SetSubtreeCulling(false)). Пара к
// BenchmarkPipeline_ClockTick: разница между ними — это ровно то, что даёт
// пропуск поддеревьев вне изменившейся области на сцене настоящего рабочего
// стола (обои из полутора десятков панелей, панель задач с несколькими
// компонентами).
func BenchmarkPipeline_ClockTickNoCulling(b *testing.B) {
	sc := buildPipelineDesktop()
	b.Cleanup(sc.closeAll)
	eng := sc.eng
	eng.SetRenderOnDemand(true)
	eng.SetSubtreeCulling(false)
	b.Cleanup(func() { eng.SetSubtreeCulling(true) })
	eng.renderFrame()

	clockRect := image.Rect(pipeW-98, pipeH-44, pipeW-8, pipeH-4)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng.InvalidateRect(clockRect)
		eng.renderFrame()
	}
}

// BenchmarkPipeline_HoverButton — курсор попеременно наводится на кнопку
// «Пуск» и уводится с неё (SendMouseMove на две точки по очереди), кадр
// рисуется после каждого движения. Типичный интерактивный сценарий панели
// задач: подсветка кнопки на hover — самая частая причина перерисовки,
// когда пользователь просто водит мышью по экрану.
func BenchmarkPipeline_HoverButton(b *testing.B) {
	sc := buildPipelineDesktop()
	b.Cleanup(sc.closeAll)
	eng := sc.eng
	eng.SetRenderOnDemand(true)
	eng.renderFrame()

	btn := sc.startBtn.Bounds()
	onX, onY := (btn.Min.X+btn.Max.X)/2, (btn.Min.Y+btn.Max.Y)/2
	// Точка вне кнопки — по вертикали середина рабочего стола, гарантированно
	// не пересекается ни с панелью задач, ни с кнопкой «Пуск».
	offX, offY := onX, pipeH/2

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			eng.SendMouseMove(onX, onY)
		} else {
			eng.SendMouseMove(offX, offY)
		}
		eng.renderFrame()
	}
}

// BenchmarkPipeline_WindowDrag — на рабочем столе лежит окно (widget.Window,
// 720×440), которое перетаскивают между двумя точками: SetBounds на каждой
// итерации плюс кадр. Перетаскивание окна — сценарий с одной из самых
// больших изменившихся областей на рабочем столе (сравнимо по размеру со
// значительной частью экрана), и он же самый частый способ, которым окно
// вообще двигается по рабочему столу.
func BenchmarkPipeline_WindowDrag(b *testing.B) {
	sc := buildPipelineDesktop()
	b.Cleanup(sc.closeAll)
	eng := sc.eng
	eng.SetRenderOnDemand(true)

	win := widget.NewWindow("Окно", 720, 440)
	startPos := image.Pt(120, 80)
	win.SetBounds(image.Rectangle{Min: startPos, Max: startPos.Add(image.Pt(720, 440))})
	// Окно добавляется как виджет рабочего стола, а не панели задач — оно
	// лежит поверх обоев, как и настоящее окно приложения.
	root := eng.Root().(*widget.Panel)
	root.AddChild(win)
	eng.renderFrame()

	// Две точки перетаскивания — по диагонали через заметную часть экрана.
	posA := image.Pt(120, 80)
	posB := image.Pt(360, 260)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pos := posA
		if i%2 == 1 {
			pos = posB
		}
		win.SetBounds(image.Rectangle{Min: pos, Max: pos.Add(image.Pt(720, 440))})
		eng.renderFrame()
	}
}

// ─── Рабочий стол с окнами программ ─────────────────────────────────────────
//
// Сцена выше — пустой рабочий стол: обои и панель задач. Настоящий рабочий
// стол WinLine почти никогда не пуст: на нём лежат окна, и одно из них
// обычно чужое — буфер программы, который движок показывает через
// widget.ExternalSurface. Сценарии ниже меряют кадр именно такой сцены.
//
// Сцена строится ПОВЕРХ buildPipelineDesktop, а не внутри неё: добавить окна
// туда значило бы изменить числа пяти бенчмарков выше, с которыми сравнивают
// всё последующее.

// pipeApps — рабочий стол с тремя окнами и чужой поверхностью.
type pipeApps struct {
	pipeDesktop
	typing  *widget.Label
	scroll  *widget.ScrollView
	surf    *widget.ExternalSurface
	surfBuf *image.RGBA
}

const (
	pipeSurfW, pipeSurfH = 800, 600
	pipeListRows         = 200
	pipeListRowH         = 20
)

// buildPipelineDesktopApps — рабочий стол 1280×800 с окнами:
//
//   - чужая поверхность 800×600 (окно другой программы), лежит ниже всех;
//   - окно-форма: подписи и кнопки;
//   - окно-список: прокрутка с 200 строками;
//   - окно ввода: строка, в которую «печатают».
//
// Только публичный API — так собирает сцену и потребитель.
func buildPipelineDesktopApps() pipeApps {
	sc := buildPipelineDesktop()
	root := sc.eng.Root().(*widget.Panel)
	// Панель задач — последний ребёнок корня; окна кладём под неё, как их
	// и показывает рабочий стол.
	root.RemoveChild(sc.bar)

	// Окна берут палитру виджетов по умолчанию (тёмную): светлый текст на ней
	// виден, как и в настоящем окне.
	ink := color.RGBA{R: 230, G: 230, B: 230, A: 255}

	// Чужое окно: буфер программы с непрозрачным содержимым.
	buf := image.NewRGBA(image.Rect(0, 0, pipeSurfW, pipeSurfH))
	writeRect(buf, buf.Rect, 1)
	surf := widget.NewExternalSurface()
	surf.SetBounds(image.Rect(400, 80, 400+pipeSurfW, 80+pipeSurfH))
	surf.SetBuffer(buf, true)
	root.AddChild(surf)

	// Форма: шесть подписей и три кнопки.
	form := widget.NewWindow("Параметры", 420, 300)
	form.SetBounds(image.Rect(40, 40, 460, 340))
	root.AddChild(form)
	fc := form.ContentBounds()
	for i := 0; i < 6; i++ {
		l := widget.NewLabel("Параметр номер "+string(rune('1'+i)), ink)
		y := fc.Min.Y + 12 + i*28
		l.SetBounds(image.Rect(fc.Min.X+12, y, fc.Min.X+240, y+22))
		form.AddChild(l)
	}
	for i, t := range []string{"ОК", "Отмена", "Применить"} {
		btn := widget.NewButton(t)
		x := fc.Min.X + 12 + i*130
		btn.SetBounds(image.Rect(x, fc.Max.Y-44, x+120, fc.Max.Y-12))
		form.AddChild(btn)
	}

	// Список: прокрутка на всё окно, 200 строк.
	list := widget.NewWindow("Журнал", 420, 360)
	list.SetBounds(image.Rect(40, 360, 460, 720))
	root.AddChild(list)
	lc := list.ContentBounds()
	sv := widget.NewScrollView()
	sv.SetBounds(lc)
	sv.ContentHeight = pipeListRows * pipeListRowH
	for i := 0; i < pipeListRows; i++ {
		y := lc.Min.Y + i*pipeListRowH
		l := widget.NewLabel("Строка журнала "+itoa(i), ink)
		l.SetBounds(image.Rect(lc.Min.X+8, y, lc.Max.X-20, y+pipeListRowH))
		sv.AddChild(l)
	}
	list.AddChild(sv)

	// Ввод: строка, которую дописывают.
	edit := widget.NewWindow("Терминал", 520, 280)
	edit.SetBounds(image.Rect(480, 420, 1000, 700))
	root.AddChild(edit)
	ec := edit.ContentBounds()
	typing := widget.NewLabel(pipeTypingText[0], ink)
	typing.SetBounds(image.Rect(ec.Min.X+8, ec.Min.Y+8, ec.Max.X-8, ec.Min.Y+30))
	edit.AddChild(typing)

	root.AddChild(sc.bar)

	return pipeApps{
		pipeDesktop: sc,
		typing:      typing,
		scroll:      sv,
		surf:        surf,
		surfBuf:     buf,
	}
}

// pipeTypingText — две строки, отличающиеся последними 20 символами: кадр
// печати меняет 20 знаков, как если бы их набрали между кадрами.
var pipeTypingText = [2]string{
	"user@winline:~$ make test && ./run --all --verbose 01234567890123456789",
	"user@winline:~$ make test && ./run --all --verbose abcdefghijklmnopqrst",
}

// itoa — без strconv, чтобы сцена не тянула лишнего в бенчмарк.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// BenchmarkPipeline_AppsFullFrame — полная перерисовка рабочего стола с
// окнами. Пара к BenchmarkPipeline_FullFrame: разница — цена трёх окон и
// чужой поверхности. Отсюда меряется выигрыш будущей тайловой растеризации.
func BenchmarkPipeline_AppsFullFrame(b *testing.B) {
	sc := buildPipelineDesktopApps()
	b.Cleanup(sc.closeAll)
	eng := sc.eng
	eng.renderFrame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng.Invalidate()
		eng.renderFrame()
	}
}

// BenchmarkPipeline_Scroll — прокрутка журнала на 120 px туда и обратно.
//
// Смещение задаётся SetScrollY, а не колесом: колесо запускает инерцию на
// анимации движка, а её шагает цикл кадров, которого в бенчмарке нет, — кадр
// оставался бы неизменным и мерил бы пустоту. Отрисовка при этом та же, что
// после шага инерции.
func BenchmarkPipeline_Scroll(b *testing.B) {
	sc := buildPipelineDesktopApps()
	b.Cleanup(sc.closeAll)
	eng := sc.eng
	eng.SetRenderOnDemand(true)
	eng.renderFrame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		y := 0
		if i%2 == 0 {
			y = 120
		}
		sc.scroll.SetScrollY(y)
		eng.renderFrame()
	}
}

// BenchmarkPipeline_Typing — в строке терминала меняются 20 знаков за кадр.
// Самый частый кадр рабочего стола после часов: человек печатает.
func BenchmarkPipeline_Typing(b *testing.B) {
	sc := buildPipelineDesktopApps()
	b.Cleanup(sc.closeAll)
	eng := sc.eng
	eng.SetRenderOnDemand(true)
	eng.renderFrame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sc.typing.SetText(pipeTypingText[(i+1)%2])
		eng.renderFrame()
	}
}

// BenchmarkPipeline_SurfaceUpdate — чужая программа перерисовала 10 % своего
// окна (253×190 из 800×600). Так обновляется окно браузера или редактора в
// сессии WinLine и кадр сервера в Tandem.
func BenchmarkPipeline_SurfaceUpdate(b *testing.B) {
	sc := buildPipelineDesktopApps()
	b.Cleanup(sc.closeAll)
	eng := sc.eng
	eng.SetRenderOnDemand(true)
	eng.renderFrame()
	r := image.Rect(100, 100, 353, 290)
	damage := []image.Rectangle{r}
	seed := 0
	update := func(buf *image.RGBA) []image.Rectangle {
		writeRect(buf, r, seed)
		return damage
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		seed = i
		sc.surf.Update(update)
		eng.renderFrame()
	}
}
