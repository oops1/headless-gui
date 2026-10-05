package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Заголовок классического окна Windows 2000 (замечания WinLine): градиент,
// метрики заголовка и кнопок, значок окна с системным меню, явная передача
// нажатия потомку поверх заголовка, PopupMenu.DismissedByPress.

// capProfile — плоская тема из встроенного профиля.
func capProfile(t *testing.T, name string, accent *color.RGBA) *widget.Theme {
	t.Helper()
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(name); err != nil {
		t.Fatal(err)
	}
	if accent != nil {
		m.SetAccent(*accent)
	}
	return widget.Materialize(m.Active())
}

type capScene struct {
	eng *engine.Engine
	win *widget.Window
}

// capWindow — окно 360×170 в углу холста 400×260 под темой th.
func capWindow(t *testing.T, th *widget.Theme) *capScene {
	t.Helper()
	win := widget.NewWindow("Терминал — bash", 360, 170)
	win.MainWindow = false
	win.SetBounds(image.Rect(20, 20, 380, 190))
	eng := engine.New(400, 260, 30)
	eng.SetRoot(win)
	eng.SetTheme(th)
	win.SetBounds(image.Rect(20, 20, 380, 190))
	return &capScene{eng: eng, win: win}
}

func (s *capScene) shot() *image.RGBA {
	s.eng.RenderOnce()
	return snapshotRGBA(s.eng.RenderOnce())
}

func (s *capScene) click(x, y int) {
	s.eng.SendMouseMove(x, y)
	s.eng.SendMouseButton(x, y, widget.MouseLeft, true)
	s.eng.SendMouseButton(x, y, widget.MouseLeft, false)
}

func capCenter(r image.Rectangle) (int, int) { return r.Min.X + r.Dx()/2, r.Min.Y + r.Dy()/2 }

func capPixel(img *image.RGBA, x, y int) color.RGBA {
	i := img.PixOffset(x, y)
	return color.RGBA{R: img.Pix[i], G: img.Pix[i+1], B: img.Pix[i+2], A: 255}
}

// capNear — цвета отличаются не больше чем на tol по каждому каналу.
func capNear(a, b color.RGBA, tol int) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol
}

// ─── Градиент заголовка ─────────────────────────────────────────────────────

// Заголовок окна Windows 2000 рисуется градиентом: тёмно-синий слева, голубой
// справа. Раньше профиль второй точки не объявлял, и заголовок был сплошным.
func TestWindow2000_TitleGradientActive(t *testing.T) {
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	img := s.shot()
	tb := s.win.CloseBtnRect() // правый край полосы — за кнопками, берём точку слева от них
	y := tb.Min.Y + tb.Dy()/2
	left := capPixel(img, 30, y)
	right := capPixel(img, tb.Min.X-100, y) // левее кнопок и плашки локали
	if !capNear(left, color.RGBA{R: 10, G: 36, B: 106, A: 255}, 12) {
		t.Errorf("левый край заголовка %v, ждали тёмно-синий", left)
	}
	if right.B <= left.B+30 || right.R <= left.R+30 {
		t.Errorf("градиента нет: слева %v, справа %v", left, right)
	}
}

// Неактивное окно — серый градиент.
func TestWindow2000_TitleGradientInactive(t *testing.T) {
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	s.win.SetActive(false)
	img := s.shot()
	tb := s.win.CloseBtnRect()
	y := tb.Min.Y + tb.Dy()/2
	left, right := capPixel(img, 30, y), capPixel(img, tb.Min.X-100, y)
	if left.R != left.G || left.G != left.B || right.R != right.G || right.G != right.B {
		t.Errorf("неактивный заголовок не серый: %v → %v", left, right)
	}
	if right.R <= left.R+20 {
		t.Errorf("градиента неактивного окна нет: %v → %v", left, right)
	}
}

// После SetAccent градиент идёт от акцента к его светлому оттенку, а не в
// прежний голубой.
func TestWindow2000_TitleGradientFollowsAccent(t *testing.T) {
	green := theme.RGB(16, 124, 16)
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, &green))
	img := s.shot()
	tb := s.win.CloseBtnRect()
	y := tb.Min.Y + tb.Dy()/2
	left, right := capPixel(img, 30, y), capPixel(img, tb.Min.X-100, y)
	if !capNear(left, green, 14) {
		t.Errorf("градиент начинается с %v, ждали акцент %v", left, green)
	}
	if right.G <= right.B || right.G <= right.R {
		t.Errorf("градиент ушёл из зелёного: справа %v", right)
	}
}

// ─── Метрики заголовка и кнопок ─────────────────────────────────────────────

// Профиль Windows 2000: заголовок 18, кнопки 16×14, рамка 5.
func TestWindow2000_CaptionMetricsApplied(t *testing.T) {
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	w := s.win
	b := w.Bounds()
	cb := w.ContentBounds()
	if got := cb.Min.Y - (b.Min.Y + 5); got != 18 {
		t.Errorf("высота заголовка %d, ждали 18", got)
	}
	closeR, maxR, minR := w.CloseBtnRect(), w.MaxBtnRect(), w.MinBtnRect()
	for name, r := range map[string]image.Rectangle{"×": closeR, "□": maxR, "─": minR} {
		if r.Empty() {
			t.Fatalf("кнопка %s отсутствует", name)
		}
		if r.Dx() != 16 || r.Dy() != 14 {
			t.Errorf("кнопка %s %dx%d, ждали 16x14", name, r.Dx(), r.Dy())
		}
	}
	// Отступ 2 справа и 2 сверху, как в настоящей Windows 2000.
	if got := b.Max.X - 5 - closeR.Max.X; got != 2 {
		t.Errorf("отступ крестика справа %d, ждали 2", got)
	}
	if got := closeR.Min.Y - (b.Min.Y + 5); got != 2 {
		t.Errorf("отступ кнопок сверху %d, ждали 2", got)
	}
	// Кнопки ─ □ рядом, крест отделён зазором 2.
	if minR.Max.X != maxR.Min.X || maxR.Max.X+2 != closeR.Min.X {
		t.Errorf("раскладка кнопок: ─%v □%v ×%v", minR, maxR, closeR)
	}
}

// Попадание по кнопке 16×14 закрывает окно, а по зазору над ней — нет.
func TestWindow2000_SmallCloseButtonHit(t *testing.T) {
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	closed := 0
	s.win.OnClose = func() { closed++ }
	r := s.win.CloseBtnRect()
	s.click(r.Max.X-1, r.Max.Y-1)
	if closed != 1 {
		t.Fatalf("нажатие по углу кнопки не закрыло окно (%d)", closed)
	}
	s.click(r.Min.X+4, r.Min.Y-1) // строка над кнопкой — заголовок, не кнопка
	if closed != 1 {
		t.Errorf("нажатие над кнопкой закрыло окно (%d)", closed)
	}
}

// Без метрик (пресет Win2000, тема без профиля) всё как прежде: заголовок 24,
// квадратные кнопки 18×18. Другие темы не меняются.
func TestWindow2000_NoMetricsKeepsOldGeometry(t *testing.T) {
	s := capWindow(t, widget.ThemeByName("Win2000"))
	w := s.win
	b := w.Bounds()
	if got := w.ContentBounds().Min.Y - (b.Min.Y + 5); got != 24 {
		t.Errorf("заголовок без метрики %d, ждали 24", got)
	}
	if r := w.CloseBtnRect(); r.Dx() != 18 || r.Dy() != 18 {
		t.Errorf("кнопка без метрики %dx%d, ждали 18x18", r.Dx(), r.Dy())
	}
	if r := w.CloseBtnRect(); r.Min.Y-(b.Min.Y+5) != 3 {
		t.Errorf("отступ кнопки сверху без метрики %d, ждали 3", r.Min.Y-(b.Min.Y+5))
	}
	// Современные темы: 32 px и кнопки 46 px, как всегда.
	for _, name := range []string{theme.ProfileWindows10, theme.ProfileWindows11} {
		s := capWindow(t, capProfile(t, name, nil))
		if got := s.win.ContentBounds().Min.Y - s.win.Bounds().Min.Y; got != 32 {
			t.Errorf("%s: заголовок %d, ждали 32", name, got)
		}
		if got := s.win.CloseBtnRect().Dx(); got != 46 {
			t.Errorf("%s: кнопка %d, ждали 46", name, got)
		}
	}
}

// Явный TitleBarHeight сильнее метрики темы.
func TestWindow2000_ExplicitTitleBarHeightWins(t *testing.T) {
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	s.win.TitleBarHeight = 22
	s.win.SetBounds(s.win.Bounds())
	if got := s.win.ContentBounds().Min.Y - (s.win.Bounds().Min.Y + 5); got != 22 {
		t.Errorf("явная высота 22, получили %d", got)
	}
}

// Метрики переживают круг профиль → плоская тема → профиль → плоская тема.
func TestThemeBridge_CaptionMetricsRoundTrip(t *testing.T) {
	th := capProfile(t, theme.ProfileWindows2000, nil)
	st := th.Style
	if st.TitleBarHeight != 18 || st.CaptionButtonW != 16 || st.CaptionButtonH != 14 || st.CaptionIconSize != 16 {
		t.Fatalf("Materialize: %+v", st)
	}
	m := theme.NewManager()
	if err := m.RegisterTheme(widget.ProfileFromTheme(th)); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(th.Style.Name); err != nil {
		t.Fatal(err)
	}
	st2 := widget.Materialize(m.Active()).Style
	if st2.TitleBarHeight != 18 || st2.CaptionButtonW != 16 || st2.CaptionButtonH != 14 || st2.CaptionIconSize != 16 {
		t.Errorf("метрики потерялись в круге: %+v", st2)
	}
	for _, name := range []string{theme.ProfileWindows10, theme.ProfileWindows11, theme.ProfileMacOS} {
		s := capProfile(t, name, nil).Style
		if s.TitleBarHeight != 0 || s.CaptionButtonW != 0 || s.CaptionButtonH != 0 || s.CaptionIconSize != 0 {
			t.Errorf("%s объявляет метрики заголовка: %+v", name, s)
		}
	}
}

// Недоступный пункт меню из встроенного профиля читается: текст не
// прозрачный. Без этого пункт «Восстановить» системного меню исчезал.
func TestMaterialize_DisabledMenuItemVisible(t *testing.T) {
	for _, name := range []string{theme.ProfileWindows2000, theme.ProfileWindows10} {
		th := capProfile(t, name, nil)
		if th.Disabled.A == 0 && th.Style.Menu.Disabled.A == 0 {
			t.Errorf("%s: цвет недоступного пункта меню прозрачен", name)
		}
	}
}

// ─── Значок окна ────────────────────────────────────────────────────────────

func capIcon(c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

var capRed = color.RGBA{R: 255, G: 0, B: 0, A: 255}

// Значок стоит слева в заголовке: 16 px, отступ 2, по центру полосы 18 px.
func TestWindowIcon_PlacedAndDrawn(t *testing.T) {
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	if !s.win.IconBounds().Empty() || s.win.HasIcon() {
		t.Fatal("значок есть без SetIcon")
	}
	s.win.SetIcon(capIcon(capRed))
	r := s.win.IconBounds()
	cb, b := s.win.ContentBounds(), s.win.Bounds()
	if r.Dx() != 16 || r.Dy() != 16 {
		t.Fatalf("значок %v, ждали 16x16", r)
	}
	if r.Min.X != b.Min.X+5+2 || r.Min.Y != b.Min.Y+5+1 || r.Max.Y > cb.Min.Y {
		t.Errorf("значок %v не по Windows 2000 (рамка %v, клиент %v)", r, b, cb)
	}
	img := s.shot()
	x, y := capCenter(r)
	if got := capPixel(img, x, y); got != capRed {
		t.Errorf("в центре значка %v, ждали красный", got)
	}

	s.win.SetIcon(nil)
	if s.win.HasIcon() || !s.win.IconBounds().Empty() {
		t.Error("SetIcon(nil) не убрал значок")
	}
	if got := capPixel(s.shot(), x, y); got == capRed {
		t.Error("значок остался на экране после SetIcon(nil)")
	}
}

// Подпись сдвигается за значок сама: начинка полосы стоит правее, чем без него.
func TestWindowIcon_ShiftsTitle(t *testing.T) {
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	s.win.Title = "bash" // короткая подпись: начинке остаётся место в полосе
	s.win.SetTitleBarContent(widget.NewTextInput(""))
	before := s.win.TitleBarContentBounds().Min.X
	s.win.SetIcon(capIcon(capRed))
	after := s.win.TitleBarContentBounds().Min.X
	// Подпись начиналась через 12 от края полосы, теперь — через 2+16+3.
	if shift := after - before; shift != (2+16+3)-12 {
		t.Errorf("подпись сдвинулась на %d, ждали %d", shift, (2+16+3)-12)
	}
}

// В заголовке macOS слева стоят кнопки окна — значка там нет.
func TestWindowIcon_NotInMacLayout(t *testing.T) {
	s := capWindow(t, capProfile(t, theme.ProfileMacOS, nil))
	s.win.SetIcon(capIcon(capRed))
	if r := s.win.IconBounds(); !r.Empty() {
		t.Errorf("значок в mac-раскладке: %v", r)
	}
}

// SVG-значок рисуется и переживает ошибку разбора.
func TestWindowIcon_SVG(t *testing.T) {
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect width="16" height="16" fill="#ff0000"/></svg>`
	if err := s.win.SetIconSVG([]byte(svg)); err != nil {
		t.Fatal(err)
	}
	x, y := capCenter(s.win.IconBounds())
	if got := capPixel(s.shot(), x, y); !capNear(got, capRed, 6) {
		t.Errorf("в центре SVG-значка %v, ждали красный", got)
	}
	if err := s.win.SetIconSVG([]byte("не svg")); err == nil {
		t.Error("ошибка разбора не вернулась")
	}
	if !s.win.HasIcon() {
		t.Error("после ошибки разбора прежний значок пропал")
	}
}

// ─── Системное меню ─────────────────────────────────────────────────────────

// capSysStrings заново регистрирует подписи системного меню: другие тесты
// пакета зовут ClearStrings, и тот стирает вместе с чужими и встроенные
// таблицы (то, что они зарегистрированы при старте, проверяет
// widget.TestWindowIcon_BuiltInStrings).
func capSysStrings() {
	widget.RegisterStrings("EN", map[string]string{
		"win.sys.restore": "Restore", "win.sys.move": "Move", "win.sys.size": "Size",
		"win.sys.minimize": "Minimize", "win.sys.maximize": "Maximize", "win.sys.close": "Close",
	})
	widget.RegisterStrings("RU", map[string]string{
		"win.sys.restore": "Восстановить", "win.sys.move": "Переместить", "win.sys.size": "Размер",
		"win.sys.minimize": "Свернуть", "win.sys.maximize": "Развернуть", "win.sys.close": "Закрыть",
	})
}

func capMenuTexts(m *widget.PopupMenu) []string {
	var out []string
	for _, it := range m.Items() {
		if it.Separator {
			out = append(out, "—")
			continue
		}
		out = append(out, it.Text)
	}
	return out
}

// Щелчок по значку открывает системное меню с подписями через Tr; без
// колбэков окна пункты недоступны, но не пропадают.
func TestSystemMenu_OpensWithLocalizedItems(t *testing.T) {
	defer widget.SetLanguage(widget.Language())
	capSysStrings()
	widget.SetLanguage("EN")
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	s.win.SetIcon(capIcon(capRed))
	x, y := capCenter(s.win.IconBounds())
	s.click(x, y)
	m := s.win.SystemMenu()
	if m == nil || !m.IsOpen() {
		t.Fatal("щелчок по значку не открыл системное меню")
	}
	want := []string{"Restore", "Move", "Size", "Minimize", "Maximize", "—", "Close"}
	got := capMenuTexts(m)
	if len(got) != len(want) {
		t.Fatalf("пункты %v, ждали %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("пункт %d: %q, ждали %q", i, got[i], want[i])
		}
	}
	for _, it := range m.Items() {
		if !it.Separator && !it.Disabled {
			t.Errorf("пункт %q доступен без колбэков окна", it.Text)
		}
	}

	widget.SetLanguage("RU")
	m.Close()
	if !s.win.OpenSystemMenu() {
		t.Fatal("OpenSystemMenu не открыл меню")
	}
	if got := capMenuTexts(m); got[0] != "Восстановить" || got[6] != "Закрыть" {
		t.Errorf("русские подписи: %v", got)
	}
}

// Действия пунктов — на колбэки окна; состояние «развёрнуто» меняет доступность.
func TestSystemMenu_ItemsRunWindowCallbacks(t *testing.T) {
	defer widget.SetLanguage(widget.Language())
	capSysStrings()
	widget.SetLanguage("EN")
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	w := s.win
	w.Resize = widget.ResizeModeCanResize
	var minimized, maximized, closed, moved, sized int
	w.OnMinimize = func() { minimized++ }
	w.OnMaximize = func() { maximized++ }
	w.OnClose = func() { closed++ }
	w.OnNativeMove = func() bool { moved++; return true }
	w.OnNativeResize = func(edges int) bool { sized = edges; return true }
	w.SetIcon(capIcon(capRed))

	byText := func(items []widget.MenuItem, text string) widget.MenuItem {
		for _, it := range items {
			if it.Text == text {
				return it
			}
		}
		t.Fatalf("нет пункта %q", text)
		return widget.MenuItem{}
	}
	items := w.SystemMenuItems()
	if byText(items, "Restore").Disabled {
		// не развёрнуто — «Восстановить» недоступно
	} else {
		t.Error("«Восстановить» доступно у не развёрнутого окна")
	}
	for _, name := range []string{"Move", "Size", "Minimize", "Maximize", "Close"} {
		if byText(items, name).Disabled {
			t.Errorf("пункт %q недоступен при заданных колбэках", name)
		}
	}
	byText(items, "Minimize").OnClick()
	byText(items, "Maximize").OnClick()
	byText(items, "Move").OnClick()
	byText(items, "Size").OnClick()
	byText(items, "Close").OnClick()
	if minimized != 1 || maximized != 1 || closed != 1 || moved != 1 || sized != widget.NativeEdgeBottom|widget.NativeEdgeRight {
		t.Errorf("колбэки: свернуть %d, развернуть %d, закрыть %d, переместить %d, размер %d",
			minimized, maximized, closed, moved, sized)
	}

	w.SetMaximized(true)
	items = w.SystemMenuItems()
	if byText(items, "Restore").Disabled || !byText(items, "Maximize").Disabled ||
		!byText(items, "Move").Disabled || !byText(items, "Size").Disabled {
		t.Errorf("развёрнутое окно: %+v", items)
	}
	byText(items, "Restore").OnClick()
	if maximized != 2 {
		t.Errorf("«Восстановить» не позвало OnMaximize (%d)", maximized)
	}
}

// Пункт меню срабатывает по щелчку мышью.
func TestSystemMenu_ClickOnItem(t *testing.T) {
	defer widget.SetLanguage(widget.Language())
	capSysStrings()
	widget.SetLanguage("EN")
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	minimized := 0
	s.win.OnMinimize = func() { minimized++ }
	s.win.Resize = widget.ResizeModeCanResize
	s.win.SetIcon(capIcon(capRed))
	x, y := capCenter(s.win.IconBounds())
	s.click(x, y)
	m := s.win.SystemMenu()
	if !m.IsOpen() {
		t.Fatal("меню не открылось")
	}
	// «Свернуть» — четвёртый пункт (индекс 3); меню открыто под значком.
	top := s.win.IconBounds().Max.Y
	s.click(x+20, top+padY(m)+3*m.ItemHeight+m.ItemHeight/2)
	if minimized != 1 {
		t.Errorf("щелчок по «Minimize» не вызвал OnMinimize (%d)", minimized)
	}
}

// Повторное нажатие на значок закрывает меню и не открывает его заново.
func TestSystemMenu_SecondPressClosesNotReopens(t *testing.T) {
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	s.win.SetIcon(capIcon(capRed))
	r := s.win.IconBounds()
	y := r.Min.Y + r.Dy()/2
	m := func() *widget.PopupMenu { return s.win.SystemMenu() }
	// Нажатия идут подряд, и в одной точке они слились бы в двойной щелчок
	// (он закрывает окно): ходим по значку из края в край — дальше допуска
	// движка (4 точки).
	left, right := r.Min.X+2, r.Max.X-2

	s.click(left, y)
	if !m().IsOpen() {
		t.Fatal("первое нажатие не открыло меню")
	}
	s.click(right, y)
	if m().IsOpen() {
		t.Error("повторное нажатие на значок не закрыло меню")
	}
	s.click(left, y)
	if !m().IsOpen() {
		t.Error("третье нажатие не открыло меню снова")
	}
}

// Двойной щелчок по значку закрывает окно, как в Windows.
func TestSystemMenu_DoubleClickCloses(t *testing.T) {
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	closed := 0
	s.win.OnClose = func() { closed++ }
	s.win.SetIcon(capIcon(capRed))
	x, y := capCenter(s.win.IconBounds())
	s.click(x, y)
	s.click(x, y)
	if closed != 1 {
		t.Errorf("двойной щелчок по значку: OnClose %d раз", closed)
	}
}

// Нажатие на значок не тащит окно; нажатие на подпись — тащит.
func TestWindowIcon_PressDoesNotDragWindow(t *testing.T) {
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	dragged := 0
	s.win.OnDragMove = func(dx, dy int) { dragged++ }
	s.win.SetIcon(capIcon(capRed))
	x, y := capCenter(s.win.IconBounds())

	s.eng.SendMouseMove(x, y)
	s.eng.SendMouseButton(x, y, widget.MouseLeft, true)
	s.eng.SendMouseMove(x+30, y+10)
	s.eng.SendMouseButton(x+30, y+10, widget.MouseLeft, false)
	if dragged != 0 {
		t.Errorf("нажатие на значок потащило окно (%d)", dragged)
	}
	if got := s.win.Bounds(); got != image.Rect(20, 20, 380, 190) {
		t.Errorf("окно сдвинулось: %v", got)
	}

	// Для сравнения — подпись окно тащит. Отпускание — в той же точке, куда
	// ушла мышь, а само перемещение приходит как OnDragMove.
	tx := s.win.IconBounds().Max.X + 60
	s.eng.SendMouseMove(tx, y)
	s.eng.SendMouseButton(tx, y, widget.MouseLeft, true)
	s.eng.SendMouseMove(tx+30, y+10)
	s.eng.SendMouseButton(tx+30, y+10, widget.MouseLeft, false)
	if dragged == 0 {
		t.Error("нажатие на подпись не потащило окно")
	}
}

// Свой список пунктов заменяет умолчание; пустой список убирает меню, и тогда
// значок — просто картинка, за которую окно тащат.
func TestSystemMenu_CustomAndDisabled(t *testing.T) {
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	s.win.SetIcon(capIcon(capRed))
	hits := 0
	s.win.SetSystemMenu(append(s.win.SystemMenuItems(),
		widget.MenuItem{Separator: true},
		widget.MenuItem{Text: "Новая вкладка", OnClick: func() { hits++ }}))
	x, y := capCenter(s.win.IconBounds())
	s.click(x, y)
	got := capMenuTexts(s.win.SystemMenu())
	if len(got) != 9 || got[8] != "Новая вкладка" {
		t.Fatalf("пункты %v", got)
	}
	s.win.SystemMenu().Close()

	s.win.SetSystemMenu([]widget.MenuItem{})
	if s.win.SystemMenu() != nil {
		t.Error("пустой список не убрал меню")
	}
	dragged := 0
	s.win.OnDragMove = func(dx, dy int) { dragged++ }
	s.eng.SendMouseMove(x, y)
	s.eng.SendMouseButton(x, y, widget.MouseLeft, true)
	s.eng.SendMouseMove(x+20, y+10)
	s.eng.SendMouseButton(x+20, y+10, widget.MouseLeft, false)
	if dragged == 0 {
		t.Error("значок без меню должен тащить окно")
	}

	s.win.SetSystemMenu(nil) // вернуть умолчание
	if s.win.SystemMenu() == nil {
		t.Error("SetSystemMenu(nil) не вернул меню по умолчанию")
	}
}

// ─── Нажатие на заголовке: кому оно принадлежит ─────────────────────────────

// capChild — ребёнок окна поверх заголовка; считает полученные нажатия.
type capChild struct {
	widget.Base
	presses int
	owns    bool
}

func (c *capChild) Draw(widget.DrawContext) {}
func (c *capChild) OnMouseButton(e widget.MouseEvent) bool {
	if e.Pressed {
		c.presses++
	}
	return true
}
func (c *capChild) OwnsTitleBarPress(pt image.Point) bool { return c.owns && pt.In(c.Bounds()) }

func capTitleChild(t *testing.T) (*capScene, *capChild, image.Point) {
	t.Helper()
	s := capWindow(t, capProfile(t, theme.ProfileWindows2000, nil))
	c := &capChild{}
	r := image.Rect(150, 27, 190, 37) // внутри полосы заголовка, правее подписи
	s.win.AddChild(c)
	c.SetBounds(r)
	return s, c, image.Pt(170, 32)
}

// Ребёнок поверх заголовка, не просящий захват и не назвавшийся владельцем,
// нажатия не получает: окно забирает его под перетаскивание. Это поведение
// прежних приложений — оно не меняется.
func TestTitleBar_ChildWithoutClaimLosesPress(t *testing.T) {
	s, c, pt := capTitleChild(t)
	if !s.win.WantsCapture(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true}) {
		t.Fatal("окно не просит захват на нажатие по заголовку")
	}
	s.click(pt.X, pt.Y)
	if c.presses != 0 {
		t.Errorf("ребёнок получил нажатие без захвата (%d)", c.presses)
	}
}

// TitleBarPressOwner отдаёт нажатие ребёнку без захвата мыши и без
// перетаскивания окна.
func TestTitleBar_PressOwnerGetsPress(t *testing.T) {
	s, c, pt := capTitleChild(t)
	c.owns = true
	dragged := 0
	s.win.OnDragMove = func(dx, dy int) { dragged++ }
	if s.win.WantsCapture(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true}) {
		t.Fatal("окно просит захват на нажатие по виджету-владельцу")
	}
	s.eng.SendMouseMove(pt.X, pt.Y)
	s.eng.SendMouseButton(pt.X, pt.Y, widget.MouseLeft, true)
	s.eng.SendMouseMove(pt.X+20, pt.Y+5)
	s.eng.SendMouseButton(pt.X+20, pt.Y+5, widget.MouseLeft, false)
	if c.presses != 1 {
		t.Errorf("владелец получил %d нажатий, ждали 1", c.presses)
	}
	if dragged != 0 {
		t.Errorf("окно потащили за владельца (%d)", dragged)
	}
	// Вне виджета заголовок по-прежнему тащит окно.
	if !s.win.WantsCapture(widget.MouseEvent{X: 260, Y: 32, Button: widget.MouseLeft, Pressed: true}) {
		t.Error("остальной заголовок перестал быть зоной перетаскивания")
	}
}

// SetTitleBarHitTest исключает из перетаскивания произвольные точки полосы.
func TestTitleBar_HitTestExcludesPoint(t *testing.T) {
	s, c, pt := capTitleChild(t)
	s.win.SetTitleBarHitTest(func(p image.Point) bool { return p.In(c.Bounds()) })
	if s.win.WantsCapture(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true}) {
		t.Fatal("точка из SetTitleBarHitTest тащит окно")
	}
	s.click(pt.X, pt.Y)
	if c.presses != 1 {
		t.Errorf("ребёнок получил %d нажатий, ждали 1", c.presses)
	}
	s.win.SetTitleBarHitTest(nil)
	if !s.win.WantsCapture(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true}) {
		t.Error("nil не снял исключение")
	}
}

// ─── PopupMenu.DismissedByPress ─────────────────────────────────────────────

// Движок гасит меню нажатием вне его: DismissedByPress верно до начала
// следующего нажатия, и Toggle повторно меню не открывает.
func TestPopupMenu_DismissedByPressAndToggle(t *testing.T) {
	m := widget.NewPopupMenu()
	m.AddItem("Один", nil)

	if m.DismissedByPress() {
		t.Error("новое меню считает себя погашенным нажатием")
	}
	widget.BumpPressSeq() // нажатие 1: открывает меню
	if !m.Toggle(10, 10) || !m.IsOpen() {
		t.Fatal("Toggle не открыл меню")
	}
	widget.BumpPressSeq() // нажатие 2: движок гасит меню и отдаёт нажатие кнопке
	m.Dismiss()
	if !m.DismissedByPress() {
		t.Fatal("меню, погашенное нажатием, не знает об этом")
	}
	if m.Toggle(10, 10) || m.IsOpen() {
		t.Error("Toggle открыл меню заново тем же нажатием")
	}
	widget.BumpPressSeq() // нажатие 3
	if m.DismissedByPress() {
		t.Error("метка пережила следующее нажатие")
	}
	if !m.Toggle(10, 10) {
		t.Error("следующее нажатие не открыло меню")
	}
	if m.Toggle(10, 10) || m.IsOpen() {
		t.Error("Toggle при открытом меню не закрыл его")
	}

	// Меню, закрытое клавишей (Close), метки не оставляет: следующее нажатие
	// открывает его, а не «теряется».
	m.Show(5, 5)
	m.Close()
	widget.BumpPressSeq()
	if m.DismissedByPress() {
		t.Error("Close оставил метку нажатия")
	}
}
