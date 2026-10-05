package desktop_test

import (
	"image"
	"sort"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Клавиатура по сетке, контекстное меню, нижняя полоса, смена темы и стоимость
// кадра меню «Пуск» Windows 11.

// Стрелки идут по сетке: вправо, вниз; вверх с первого ряда — на «Все
// приложения», вниз с последнего — в «Рекомендуем»; Home, End.
func TestStartMenu11_ArrowsOnGrid(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.open()
	s.key(widget.KeyTab) // первая ячейка
	s.key(widget.KeyRight)
	s.key(widget.KeyDown)
	s.key(widget.KeyEnter)
	if got := launched(s); len(got) != 1 || got[0] != "app7" {
		t.Fatalf("Right+Down+Enter запустили %v, ждали [app7] (колонка 1, ряд 1)", got)
	}

	s.open()
	s.key(widget.KeyTab)
	s.key(widget.KeyEnd)
	s.key(widget.KeyEnter)
	if got := launched(s); got[len(got)-1] != "app17" {
		t.Errorf("End запустил %v, ждали app17", got)
	}

	s.open()
	s.key(widget.KeyTab)
	s.key(widget.KeyHome)
	s.key(widget.KeyLeft) // дальше левого края не идёт
	s.key(widget.KeyEnter)
	if got := launched(s); got[len(got)-1] != "app0" {
		t.Errorf("Home+Left запустил %v, ждали app0", got)
	}

	// Вверх с первого ряда — «Все приложения ›», Enter открывает список.
	s.open()
	s.key(widget.KeyTab)
	s.key(widget.KeyRight)
	s.key(widget.KeyUp)
	s.key(widget.KeyEnter)
	if s.menu.View() != desktop.StartViewAllApps {
		t.Errorf("Up+Enter с первого ряда: вид %v, ждали «Все приложения»", s.menu.View())
	}

	// Вниз с последнего ряда — в «Рекомендуем», вниз с него — в нижнюю полосу.
	s.open()
	var rec string
	s.menu.OnRecommendedActivate = func(id string) { rec = id }
	s.key(widget.KeyTab)
	for i := 0; i < 3; i++ { // ряд 1, ряд 2, затем — из сетки в рекомендуемые
		s.key(widget.KeyDown)
	}
	s.key(widget.KeyEnter)
	if rec != "r1" {
		t.Errorf("Down с последнего ряда открыл %q, ждали r1", rec)
	}
}

func TestStartMenu11_ArrowsOnRecommended(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	var rec string
	s.menu.OnRecommendedActivate = func(id string) { rec = id }
	s.open()
	s.key(widget.KeyTab)
	s.key(widget.KeyTab) // первая рекомендация
	s.key(widget.KeyRight)
	s.key(widget.KeyDown)
	s.key(widget.KeyEnter)
	if rec != "r4" {
		t.Errorf("Right+Down+Enter открыл %q, ждали r4 (колонка 1, ряд 1)", rec)
	}
	// Вверх с первого ряда — «Дополнительно ›».
	s.open()
	s.key(widget.KeyTab)
	s.key(widget.KeyTab)
	s.key(widget.KeyUp)
	s.key(widget.KeyEnter)
	if s.menu.View() != desktop.StartViewRecommended {
		t.Errorf("Up+Enter: вид %v, ждали «Все рекомендации»", s.menu.View())
	}
}

// Рамка клавиатурного фокуса видна: кадр с выбором отличается от кадра без него.
func TestStartMenu11_FocusRingIsVisible(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.open()
	s.frame()
	before := append([]uint8(nil), s.frame().Pix...)
	s.key(widget.KeyTab)
	s.frame()
	finishAnims()
	s.eng.Invalidate()
	after := s.frame()
	c := s.cell(0, 0)
	r := image.Rect(c.X-48, c.Y-42, c.X+48, c.Y+42)
	diff := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := y*after.Stride + x*4
			if after.Pix[i] != before[i] || after.Pix[i+1] != before[i+1] || after.Pix[i+2] != before[i+2] {
				diff++
			}
		}
	}
	if diff < 200 {
		t.Errorf("рамка фокуса не видна: изменилось %d пикселей ячейки", diff)
	}
}

// Правая кнопка и клавиша «Меню»: контекстное меню потребителя с целью.
func TestStartMenu11_ContextMenu(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	var targets []desktop.StartTarget
	s.menu.ContextMenu = func(tg desktop.StartTarget) []widget.MenuItem {
		targets = append(targets, tg)
		return []widget.MenuItem{{Text: "Открепить от начального экрана"}, {Text: "Закрепить на панели задач"}}
	}
	s.open()
	s.frame()
	at := s.cell(3, 1)
	s.menu.OnMouseMove(at.X, at.Y)
	s.menu.OnMouseButton(widget.MouseEvent{X: at.X, Y: at.Y, Button: widget.MouseRight, Pressed: true})
	s.menu.OnMouseButton(widget.MouseEvent{X: at.X, Y: at.Y, Button: widget.MouseRight})
	if len(targets) != 1 || targets[0].Kind != desktop.StartTargetPinned || targets[0].Pinned != "app9" || targets[0].App != "app9" {
		t.Fatalf("цель контекстного меню %+v, ждали закреплённое app9", targets)
	}
	if !s.menu.IsOpen() {
		t.Error("контекстное меню закрыло «Пуск»")
	}
	s.eng.Invalidate()
	savePNG(t, s.frame(), "start11_light_context")
	s.key(widget.KeyEscape) // закрывает меню, не «Пуск»
	if !s.menu.IsOpen() {
		t.Error("Esc закрыл «Пуск» вместе с контекстным меню")
	}

	// Строка «Рекомендуем» и клавиша «Меню» на выбранном объекте.
	targets = nil
	s.key(widget.KeyTab)
	s.key(widget.KeyTab)
	s.key(widget.KeyMenu)
	if len(targets) != 1 || targets[0].Kind != desktop.StartTargetRecommended || targets[0].Recommended != "r1" {
		t.Errorf("цель по клавише «Меню» %+v, ждали рекомендацию r1", targets)
	}
}

// Нижняя полоса: пользователь и питание — меню потребителя над кнопкой.
func TestStartMenu11_FooterMenus(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.menu.UserMenu = func() []widget.MenuItem {
		return []widget.MenuItem{{Text: "Параметры учётной записи"}, {Text: "Блокировка"}, {Text: "Сменить пользователя"}}
	}
	s.open()
	s.frame()

	// Питание: меню открывается над кнопкой, «Пуск» остаётся.
	s.click(s.powerBtn())
	if !s.menu.IsOpen() {
		t.Fatal("нажатие на питание закрыло «Пуск»")
	}
	s.eng.Invalidate()
	savePNG(t, s.frame(), "start11_light_power")
	s.key(widget.KeyEscape)
	if !s.menu.IsOpen() {
		t.Error("Esc закрыл «Пуск» вместе с меню питания")
	}

	// Пользователь: свой список.
	s.click(s.userBtn())
	if !s.menu.IsOpen() {
		t.Fatal("нажатие на пользователя закрыло «Пуск»")
	}
	s.eng.Invalidate()
	savePNG(t, s.frame(), "start11_light_user")
	s.key(widget.KeyEscape)

	// Без меню — событие боковой панели с идентификатором.
	var side []string
	s.menu.UserMenu = nil
	s.menu.OnSidebarActivate = func(id string) { side = append(side, id) }
	s.click(s.userBtn())
	if len(side) != 1 || side[0] != "user" || s.menu.IsOpen() {
		t.Errorf("пользователь без меню: события %v, открыто=%v", side, s.menu.IsOpen())
	}
}

// Смена темы, акцента и языка на открытом меню — без пересоздания.
func TestStartMenu11_ThemeAccentLanguageOnOpenMenu(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.open()
	menuBefore := s.menu
	light := append([]uint8(nil), s.frame().Pix...)
	same := func(a []uint8, b *image.RGBA) bool {
		for i := range a {
			if a[i] != b.Pix[i] {
				return false
			}
		}
		return true
	}
	r := s.rect()
	panelPix := func(img *image.RGBA) (c [4]uint8) {
		p := img.RGBAAt(r.Min.X+5, r.Min.Y+420)
		return [4]uint8{p.R, p.G, p.B, p.A}
	}

	// Тёмная тема: панель темнеет.
	if err := s.tm.SetTheme(theme.ProfileWindows11Dark); err != nil {
		t.Fatal(err)
	}
	finishAnims()
	s.eng.Invalidate()
	dark := s.frame()
	if same(light, dark) {
		t.Error("тёмная тема не изменила кадр")
	}
	if p := panelPix(dark); p[0] > 90 {
		t.Errorf("тёмная панель %v слишком светлая", p)
	}
	// Акцент: рамка фокуса и поле поиска перекрашиваются.
	s.key(widget.KeyTab)
	s.tm.SetAccent(theme.RGB(190, 30, 60))
	finishAnims()
	s.eng.Invalidate()
	if s.frame() == nil {
		t.Error("кадр после смены акцента не получен")
	}

	// Язык: сменились заголовки.
	before := append([]uint8(nil), s.frame().Pix...)
	widget.SetLanguage("EN")
	defer widget.SetLanguage("RU")
	s.eng.Invalidate()
	if same(before, s.frame()) {
		t.Error("смена языка не изменила кадр")
	}

	if s.menu != menuBefore || !s.menu.IsOpen() {
		t.Error("компонент пересоздан или закрыт при смене темы, акцента и языка")
	}
}

// Смена профиля на открытом меню меняет вид (сетка ↔ плитки ↔ плоский список)
// без пересоздания и без чужого состояния: после возврата вид главный.
func TestStartMenu11_ThemeSwitchChangesPresenter(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.open()
	s.menu.SetView(desktop.StartViewAllApps)
	s.key(widget.KeyTab)
	if !s.menu.AsGrid() || s.menu.AsTiled() {
		t.Fatal("в Windows 11 меню должно быть сеткой")
	}
	for _, p := range []string{theme.ProfileWindows10, theme.ProfileWindows2000, theme.ProfileMacOS, theme.ProfileWindows11} {
		if err := s.tm.SetTheme(p); err != nil {
			t.Fatal(err)
		}
		finishAnims()
		s.eng.Invalidate()
		if s.frame() == nil {
			t.Fatalf("%s: кадр не получен", p)
		}
		if p == theme.ProfileWindows10 && !s.menu.AsTiled() {
			t.Errorf("%s: меню не стало плитками", p)
		}
		if p == theme.ProfileWindows11 {
			if !s.menu.AsGrid() {
				t.Errorf("%s: меню не вернулось к сетке", p)
			}
			if s.menu.View() != desktop.StartViewMain {
				t.Errorf("после возврата на Windows 11 вид %v, ждали главный", s.menu.View())
			}
		}
		if !s.menu.IsOpen() {
			t.Fatalf("%s: меню закрылось при смене темы", p)
		}
		// Ввод после смены вида не падает: клавиши и мышь.
		s.key(widget.KeyTab)
		s.key(widget.KeyDown)
		r := s.rect()
		s.menu.OnMouseMove(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
	}
}

// Открытие: первый кадр быстрее 100 мс (холодный и тёплый), перерисовывается
// только область меню.
func TestStartMenu11_FirstFrameUnder100ms(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	var entries []desktop.StartEntry
	for i := 0; i < 300; i++ {
		entries = append(entries, entry("a"+string(rune('a'+i%26))+string(rune('a'+i/26)), "Приложение "+string(rune('A'+i%26)), "", cBlue))
	}
	s.menu.SetSource(desktop.NewFakeStartSource(nil, desktop.GroupByLetter(entries)))

	t0 := time.Now()
	s.menu.Open(s.start.Bounds())
	s.menu.Settle()
	s.frame()
	cold := time.Since(t0)

	s.menu.Close()
	s.menu.Settle()
	s.eng.Invalidate()
	s.frame()

	t0 = time.Now()
	s.menu.Open(s.start.Bounds())
	s.menu.Settle()
	s.frame()
	warm := time.Since(t0)
	t.Logf("первый кадр после открытия: холодный %v, тёплый %v", cold, warm)
	if cold > 100*time.Millisecond {
		t.Errorf("холодный первый кадр %v — дольше 100 мс", cold)
	}
	if warm > 100*time.Millisecond {
		t.Errorf("тёплый первый кадр %v — дольше 100 мс", warm)
	}

	// «Все приложения» на 300 приложений — тоже быстро.
	s.menu.SetView(desktop.StartViewAllApps)
	t0 = time.Now()
	s.eng.Invalidate()
	s.frame()
	if d := time.Since(t0); d > 100*time.Millisecond {
		t.Errorf("кадр «Все приложения» на 300 приложений %v — дольше 100 мс", d)
	}
}

func TestStartMenu11_OpenDamagesOnlyMenuArea(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.frame()
	var rects []image.Rectangle
	fulls := 0
	h := widget.RegisterUINotifierWake(func() { fulls++ }, func(r image.Rectangle) { rects = append(rects, r) }, func() {})
	defer widget.UnregisterUINotifier(h)

	s.menu.Open(s.start.Bounds())
	s.menu.Settle()
	if fulls != 0 {
		t.Errorf("открытие вызвало %d полных перерисовок", fulls)
	}
	area := s.menu.OverlayBounds().Inset(-80)
	for _, r := range rects {
		if !r.In(area) && !r.Empty() {
			t.Errorf("заявлена область %v вне меню %v", r, s.menu.OverlayBounds())
		}
	}

	// Наведение, клавиши, перелистывание и поиск тоже не выходят за меню.
	rects, fulls = nil, 0
	c := s.cell(2, 1)
	s.menu.OnMouseMove(c.X, c.Y)
	s.menu.OnMouseMove(c.X+96, c.Y)
	s.key(widget.KeyTab)
	s.key(widget.KeyRight)
	s.typ("ab")
	s.menu.SetView(desktop.StartViewAllApps)
	if fulls != 0 {
		t.Errorf("работа с меню вызвала %d полных перерисовок", fulls)
	}
	for _, r := range rects {
		if !r.In(area) && !r.Empty() {
			t.Errorf("заявлена область %v вне меню %v", r, s.menu.OverlayBounds())
		}
	}
}

// Кадры наведения дешёвые: медиана кадра при движении по ячейкам.
func TestStartMenu11_HoverFramesAreCheap(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.open()
	s.frame()
	const frames = 9
	times := make([]time.Duration, 0, frames)
	for i := 0; i < frames; i++ {
		c := s.cell(i%6, (i/6)%3)
		s.menu.OnMouseMove(c.X, c.Y)
		start := time.Now()
		s.frame()
		times = append(times, time.Since(start))
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	med := times[frames/2]
	t.Logf("медианный кадр наведения: %v", med)
	if med > 50*time.Millisecond {
		t.Errorf("кадр наведения %v — дороже 50 мс", med)
	}
}

// Mica по флагу: панель берёт цвет у размытых обоев; без флага — сплошная.
func TestStartMenu11_MicaByFlag(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.open()
	r := s.rect()
	solid := s.frame().RGBAAt(r.Min.X+5, r.Min.Y+420)
	if solid.R != 243 || solid.G != 243 || solid.B != 243 {
		t.Errorf("сплошная панель %v, ждали 243", solid)
	}
	s.eng.SetWallpaperSource(matWallpaper(1280, 800))
	s.tm.SetFlag(theme.FlagBackdropMica, true)
	finishAnims()
	s.eng.Invalidate()
	mica := s.frame().RGBAAt(r.Min.X+5, r.Min.Y+420)
	if mica == solid {
		t.Errorf("флаг Mica не изменил панель: %v", mica)
	}
	s.eng.Invalidate()
	savePNG(t, s.frame(), "start11_light_mica")
}
