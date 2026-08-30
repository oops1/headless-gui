package desktop

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Предпросмотр окна на панели задач.
//
// Наведение на кнопку окна через задержку показывает миниатюру, прижатую к
// этой кнопке; проведение вдоль панели переносит панель к соседней кнопке без
// мигания; увод курсора закрывает. Свёрнутое окно предпросмотр имеет — это
// главный случай, ради которого он и нужен.

// fakePreviews — модель окон, умеющая отдавать миниатюры. Считает вызовы:
// частота обращения к источнику — отдельное требование, а не деталь.
type fakePreviews struct {
	*FakeWindowModel

	calls int
	last  image.Point
	// missing — окна, для которых миниатюры нет (окно ещё не рисовалось).
	missing map[WindowID]bool
}

func newFakePreviews(windows ...WindowInfo) *fakePreviews {
	return &fakePreviews{
		FakeWindowModel: NewFakeWindowModel(windows...),
		missing:         map[WindowID]bool{},
	}
}

func (f *fakePreviews) Preview(id WindowID, max image.Point) image.Image {
	f.calls++
	f.last = max
	if f.missing[id] {
		return nil
	}
	// Картинка нарочно другой формы, чем отведённое место: панель обязана
	// вписать её по пропорциям, а не растянуть.
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	img.Set(0, 0, color.RGBA{R: uint8(id), A: 255})
	return img
}

// previewTheme — тема с метриками предпросмотра и НУЛЕВЫМИ задержками:
// тесты проверяют поведение, а не терпение.
func previewTheme(t *testing.T, delaysMs float64) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	p := theme.NewProfile("PreviewTest")

	p.SetStyle(ComponentTaskButton, "", theme.StateNormal, theme.StyleDelta{
		Text: theme.C(theme.RGB(220, 220, 220)),
	})
	p.SetStyle(ComponentPreview, "", theme.StateNormal, theme.StyleDelta{
		Fill: theme.C(theme.RGB(40, 40, 40)),
		PadX: theme.N(4),
	})
	p.SetStyle(ComponentPreview, previewPartHeader, theme.StateNormal, theme.StyleDelta{
		Text: theme.C(theme.RGB(230, 230, 230)),
	})
	p.SetStyle(ComponentPreview, previewPartThumb, theme.StateNormal, theme.StyleDelta{
		Fill: theme.C(theme.RGB(20, 20, 20)),
	})

	p.SetMetric(KeyTaskButtonWidth, 40)
	p.SetMetric(KeyTaskButtonMinWidth, 20)
	p.SetMetric(KeyTaskButtonIconSize, 16)
	p.SetMetric(KeyPreviewWidth, 100)
	p.SetMetric(KeyPreviewHeight, 60)
	p.SetMetric(KeyPreviewPad, 4)
	p.SetMetric(KeyPreviewHeader, 16)
	p.SetMetric(KeyPreviewDelayOpen, delaysMs)
	p.SetMetric(KeyPreviewDelayClose, delaysMs)
	p.SetMetric(KeyPreviewRefresh, delaysMs)

	if err := m.RegisterTheme(p); err != nil {
		t.Fatalf("RegisterTheme: %v", err)
	}
	if err := m.SetTheme("PreviewTest"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}
	return m
}

// previewScene — область кнопок окон и подключённый к ней предпросмотр.
func previewScene(t *testing.T, delaysMs float64) (*RunningApplications, *WindowPreview, *fakePreviews) {
	t.Helper()
	m := previewTheme(t, delaysMs)
	wm := newFakePreviews(
		WindowInfo{ID: 10, AppID: "term", Title: "Терминал", Active: true},
		WindowInfo{ID: 11, AppID: "mail", Title: "Почта"},
		WindowInfo{ID: 12, AppID: "notes", Title: "Заметки", Minimized: true},
	)

	area := NewRunningApplications(m, wm)
	area.SetBounds(image.Rect(0, 560, 400, 600))
	t.Cleanup(area.Close)

	p := NewWindowPreview(m, wm)
	p.Screen = image.Rect(0, 0, 800, 600)
	p.Track(area)
	t.Cleanup(p.Close)
	return area, p, wm
}

// hoverButton наводит курсор на кнопку i.
func hoverButton(t *testing.T, area *RunningApplications, i int) image.Rectangle {
	t.Helper()
	r := area.ButtonRect(i)
	if r.Empty() {
		t.Fatalf("у кнопки %d нет прямоугольника", i)
	}
	area.OnMouseMove(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
	return r
}

// waitFor ждёт, пока условие станет истинным, продвигая анимации: таймеры
// панели — это анимации, и без StepAnimations они не идут.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		widget.StepAnimations(time.Now())
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("не дождались: %s", what)
}

func TestPreview_HoverOpensAnchoredToTheButton(t *testing.T) {
	area, p, _ := previewScene(t, 10)

	btn := hoverButton(t, area, 1)
	if p.IsOpen() {
		t.Fatal("панель открылась сразу — задержки нет, и она будет мигать при проведении")
	}
	waitFor(t, "открытие панели", p.IsOpen)

	info, ok := p.Window()
	if !ok || info.ID != 11 {
		t.Errorf("показано окно %+v, ждали «Почту» (id 11)", info)
	}

	// Панель прижата к своей кнопке: её горизонтальная середина совпадает с
	// серединой кнопки, а сама она висит НАД панелью задач.
	r := p.Bounds()
	if r.Empty() {
		t.Fatal("у открытой панели пустой прямоугольник")
	}
	if got, want := r.Min.X+r.Dx()/2, btn.Min.X+btn.Dx()/2; abs(got-want) > 1 {
		t.Errorf("середина панели по X = %d, у кнопки %d — панель не прижата к ней", got, want)
	}
	if r.Max.Y > btn.Min.Y {
		t.Errorf("панель %v налезла на кнопку %v", r, btn)
	}
}

// Проведение вдоль панели переносит окно, а не закрывает и открывает заново.
func TestPreview_SlidingAlongButtonsDoesNotBlink(t *testing.T) {
	area, p, _ := previewScene(t, 10)

	hoverButton(t, area, 0)
	waitFor(t, "открытие панели", p.IsOpen)
	first := p.Bounds()

	// Соседняя кнопка: панель обязана остаться открытой всё время.
	hoverButton(t, area, 1)
	if !p.IsOpen() {
		t.Fatal("переход на соседнюю кнопку закрыл панель — это и есть мигание")
	}
	info, _ := p.Window()
	if info.ID != 11 {
		t.Errorf("после перехода показано окно %d, ждали 11", info.ID)
	}
	if second := p.Bounds(); second == first {
		t.Error("панель не переехала к новой кнопке")
	}
}

// Увод курсора закрывает — но не мгновенно: за задержку курсор успевает
// дойти до самой миниатюры.
func TestPreview_LeavingClosesAfterDelay(t *testing.T) {
	area, p, _ := previewScene(t, 10)

	hoverButton(t, area, 0)
	waitFor(t, "открытие панели", p.IsOpen)

	area.OnMouseMove(700, 100) // курсор ушёл с кнопок
	if !p.IsOpen() {
		t.Fatal("панель закрылась мгновенно — до миниатюры курсор довести нельзя")
	}
	waitFor(t, "закрытие панели", func() bool { return !p.IsOpen() })
}

// Курсор, доведённый до самой миниатюры, держит её открытой.
func TestPreview_CursorOnThePanelKeepsItOpen(t *testing.T) {
	area, p, _ := previewScene(t, 20)

	hoverButton(t, area, 0)
	waitFor(t, "открытие панели", p.IsOpen)

	area.OnMouseMove(700, 100) // ушли с кнопки — пошёл отсчёт закрытия
	r := p.Bounds()
	p.OnMouseMove(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)

	// Крутим анимации заведомо дольше задержки закрытия.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		widget.StepAnimations(time.Now())
		time.Sleep(5 * time.Millisecond)
	}
	if !p.IsOpen() {
		t.Error("панель закрылась, хотя курсор на ней самой")
	}
}

// Свёрнутое окно предпросмотр имеет: это главный случай, ради которого он и
// нужен.
func TestPreview_MinimizedWindowHasAPreview(t *testing.T) {
	area, p, _ := previewScene(t, 10)

	hoverButton(t, area, 2) // «Заметки» свёрнуты
	waitFor(t, "открытие панели", p.IsOpen)

	info, ok := p.Window()
	if !ok || !info.Minimized {
		t.Fatalf("показано не свёрнутое окно: %+v", info)
	}
	if p.Thumbnail() == nil {
		t.Error("у свёрнутого окна нет миниатюры")
	}
}

// Клик по миниатюре поднимает окно и закрывает предпросмотр.
func TestPreview_ClickRaisesTheWindow(t *testing.T) {
	area, p, wm := previewScene(t, 10)

	hoverButton(t, area, 1)
	waitFor(t, "открытие панели", p.IsOpen)

	r := p.Bounds()
	p.OnMouseButton(widget.MouseEvent{
		X: r.Min.X + r.Dx()/2, Y: r.Min.Y + r.Dy()/2,
		Button: widget.MouseLeft, Pressed: true,
	})

	if p.IsOpen() {
		t.Error("после клика по миниатюре панель осталась открытой")
	}
	if len(wm.Activated) == 0 || wm.Activated[len(wm.Activated)-1] != 11 {
		t.Errorf("окно не поднято: журнал активаций %v", wm.Activated)
	}
}

// Тема вправе не хотеть предпросмотра вовсе — Windows 2000 его не имела.
func TestPreview_ThemeCanTurnItOff(t *testing.T) {
	area, p, wm := previewScene(t, 10)

	prof := theme.NewProfile("NoPreview")
	prof.SetFlag(KeyPreview, false)
	prof.SetMetric(KeyTaskButtonWidth, 40)
	if err := p.tm.RegisterTheme(prof); err != nil {
		t.Fatal(err)
	}
	if err := p.tm.SetTheme("NoPreview"); err != nil {
		t.Fatal(err)
	}
	if p.Enabled() {
		t.Fatal("тема выключила предпросмотр, а панель считает себя включённой")
	}

	before := wm.calls
	hoverButton(t, area, 0)
	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		widget.StepAnimations(time.Now())
		time.Sleep(5 * time.Millisecond)
	}
	if p.IsOpen() {
		t.Error("панель открылась при выключенном предпросмотре")
	}
	if wm.calls != before {
		t.Errorf("выключенный предпросмотр всё равно спрашивал миниатюры: %d вызовов", wm.calls-before)
	}
}

// Модель без миниатюр предпросмотра не получает — и не падает.
func TestPreview_ModelWithoutPreviewsIsSilent(t *testing.T) {
	m := previewTheme(t, 10)
	wm := NewFakeWindowModel(WindowInfo{ID: 10, Title: "Терминал"})

	area := NewRunningApplications(m, wm)
	area.SetBounds(image.Rect(0, 560, 400, 600))
	defer area.Close()

	p := NewWindowPreview(m, wm)
	p.Screen = image.Rect(0, 0, 800, 600)
	p.Track(area)
	defer p.Close()

	hoverButton(t, area, 0)
	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		widget.StepAnimations(time.Now())
		time.Sleep(5 * time.Millisecond)
	}
	if p.IsOpen() {
		t.Error("панель открылась, хотя модель миниатюр не отдаёт")
	}
}

// Закрытая панель источник не дёргает вовсе, а открытая — не чаще, чем
// просит тема. Это требование по трафику, а не мелочь: живая миниатюра в
// каждом кадре вернула бы непрерывный поток кадров.
func TestPreview_SourceIsAskedOnlyWhileOpen(t *testing.T) {
	area, p, wm := previewScene(t, 30)

	before := wm.calls
	// Пока панель закрыта, крутим анимации — обращений быть не должно.
	for i := 0; i < 20; i++ {
		widget.StepAnimations(time.Now())
		time.Sleep(5 * time.Millisecond)
	}
	if wm.calls != before {
		t.Errorf("закрытая панель спросила миниатюру %d раз", wm.calls-before)
	}

	hoverButton(t, area, 0)
	waitFor(t, "открытие панели", p.IsOpen)
	atOpen := wm.calls

	// Полсекунды при обновлении раз в 30 мс — это порядка полутора десятков
	// обращений, но никак не по одному на кадр (60 в секунду).
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		widget.StepAnimations(time.Now())
		time.Sleep(2 * time.Millisecond)
	}
	got := wm.calls - atOpen
	if got == 0 {
		t.Error("открытая панель миниатюру не обновляла вовсе")
	}
	if got > 30 {
		t.Errorf("миниатюра обновлялась %d раз за полсекунды — чаще, чем просит тема", got)
	}

	p.Close()
	afterClose := wm.calls
	for i := 0; i < 40; i++ {
		widget.StepAnimations(time.Now())
		time.Sleep(5 * time.Millisecond)
	}
	if wm.calls != afterClose {
		t.Errorf("закрытая панель продолжила спрашивать миниатюры: %d раз", wm.calls-afterClose)
	}
}

// Источник получает тот размер, который просит тема.
func TestPreview_AsksForTheThemeSize(t *testing.T) {
	area, p, wm := previewScene(t, 10)

	hoverButton(t, area, 0)
	waitFor(t, "открытие панели", p.IsOpen)

	if want := (image.Point{X: 100, Y: 60}); wm.last != want {
		t.Errorf("миниатюру просили размером %v, тема задала %v", wm.last, want)
	}
}

// Окно без миниатюры панель показывает без неё, а не пустым местом с чужой
// картинкой.
func TestPreview_MissingThumbnailIsNotSomeoneElses(t *testing.T) {
	area, p, wm := previewScene(t, 10)

	hoverButton(t, area, 0)
	waitFor(t, "открытие панели", p.IsOpen)
	if p.Thumbnail() == nil {
		t.Fatal("у первого окна нет миниатюры — тест ничего не проверяет")
	}

	wm.missing[11] = true
	hoverButton(t, area, 1)
	if got := p.Thumbnail(); got != nil {
		t.Error("для окна без миниатюры показана картинка соседнего окна")
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Простаивающий стол с ОТКРЫТЫМ предпросмотром не шлёт больше кадров, чем
// частота обновления миниатюры.
//
// Это главное требование к панели, а не мелочь: всплывающее окно — крупный
// непрозрачный прямоугольник, и живая миниатюра в каждом кадре вернула бы
// ровно тот дефект, который движок чинил в 3.16.1 — неподвижный рабочий стол
// начинал слать кадры непрерывно.
func TestPreview_OpenPanelDoesNotFloodFrames(t *testing.T) {
	const refreshMs = 50

	m := previewTheme(t, refreshMs)
	wm := newFakePreviews(
		WindowInfo{ID: 10, Title: "Терминал", Active: true},
		WindowInfo{ID: 11, Title: "Почта"},
	)

	root := widget.NewPanel(color.RGBA{R: 20, G: 24, B: 30, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 800, 600))

	area := NewRunningApplications(m, wm)
	area.SetBounds(image.Rect(0, 560, 400, 600))
	root.AddChild(area)
	defer area.Close()

	p := NewWindowPreview(m, wm)
	p.Screen = image.Rect(0, 0, 800, 600)
	p.Track(area)
	root.AddChild(p)
	defer p.Close()

	eng := engine.New(800, 600, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.Start()
	defer eng.Stop()

	time.Sleep(150 * time.Millisecond) // стартовые кадры

	btn := area.ButtonRect(0)
	if btn.Empty() {
		t.Fatal("у кнопки нет прямоугольника")
	}
	eng.SendMouseMove(btn.Min.X+btn.Dx()/2, btn.Min.Y+btn.Dy()/2)

	waitFor(t, "открытие панели", p.IsOpen)
	time.Sleep(100 * time.Millisecond)

	before := eng.RenderCount()
	const window = 600 * time.Millisecond
	time.Sleep(window)
	got := eng.RenderCount() - before

	if !p.IsOpen() {
		t.Fatal("панель закрылась сама — тест мерил не то")
	}
	// За 600 мс при обновлении раз в 50 мс это дюжина кадров. При потолке
	// 60 fps «кадр на тик» дал бы под сорок.
	if want := int(window/time.Millisecond)/refreshMs + 4; got > uint64(want) {
		t.Errorf("открытый предпросмотр дал %d кадров за %v при обновлении раз в %d мс "+
			"(ждали не больше %d)", got, window, refreshMs, want)
	}
	t.Logf("кадров за %v: %d (обновление раз в %d мс)", window, got, refreshMs)
}
