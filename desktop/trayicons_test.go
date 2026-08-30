package desktop

// Тесты значков состояния трея (NetworkItem, VolumeItem, PowerItem) —
// жалоба заказчика (оболочка WinLine): «что за символы, неинформативно» про
// сеть и звук в правой части панели. Три группы тестов:
//
//   - различимость состояний: «нет сети» не должна выглядеть как «слабый
//     сигнал», а Muted обязан читаться на любом размере значка;
//   - подсказки при наведении обновляются вместе с состоянием, а не только
//     после первой отрисовки, и без гонки между горутиной потребителя
//     (Subscribe) и горутиной кадра (GetToolTip);
//   - фигуры не деградируют и не вылезают за границы значка на крупном
//     KeyTrayIconSize (24, 32 — заказчик поднял вдвое от исходных 16).

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
)

// ─── Вспомогательные записывающие контексты и тема ──────────────────────────

// recPixel — recCtx (см. clock_test.go) плюс запись SetPixel. Самому recCtx
// это не нужно (часы пикселями не рисуют, и менять чужой файл ради этого
// нельзя), а перечёркивание значков (drawDiagonal/drawDiagonalStrike в
// tray.go) рисуется именно пикселями: без своего счётчика тесты этого файла
// не отличили бы «перечеркнули» от «не перечеркнули» и «толще» от «так же
// тонко, как раньше».
type recPixel struct {
	recCtx
	pixels []image.Point
}

func (c *recPixel) SetPixel(x, y int, col color.RGBA) {
	c.pixels = append(c.pixels, image.Point{X: x, Y: y})
}

// maxPixelsPerColumn — наибольшее число точек SetPixel с одним и тем же X:
// мера толщины перечёркивания. Обычный drawDiagonal кладёт на каждый X ровно
// одну точку; drawDiagonalStrike на крупном значке — несколько (параллельные
// проходы drawDiagonalOffset).
func maxPixelsPerColumn(pixels []image.Point) int {
	counts := map[int]int{}
	max := 0
	for _, p := range pixels {
		counts[p.X]++
		if counts[p.X] > max {
			max = counts[p.X]
		}
	}
	return max
}

// testThemeManagerSized — то же самое, что testThemeManager (clock_test.go),
// но с настраиваемым KeyTrayIconSize: часть тестов этого файла нарочно
// проверяет крупные значки (заказчик поднял метрику с 16 до 24, тесты берут
// и 32 — с запасом), а testThemeManager намертво зашивает 16.
func testThemeManagerSized(t *testing.T, size float64) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	p := theme.NewProfile("TestSized")

	p.SetStyle(ComponentNetwork, "", theme.StateNormal, theme.StyleDelta{
		Fill:   theme.C(theme.RGB(240, 240, 240)),
		Border: theme.C(theme.RGB(90, 90, 90)),
		PadX:   theme.N(2),
		PadY:   theme.N(2),
	})
	p.SetStyle(ComponentVolume, "", theme.StateNormal, theme.StyleDelta{
		Fill:   theme.C(theme.RGB(240, 240, 240)),
		Border: theme.C(theme.RGB(90, 90, 90)),
		PadX:   theme.N(2),
		PadY:   theme.N(2),
	})
	p.SetStyle(ComponentPower, "", theme.StateNormal, theme.StyleDelta{
		Fill:   theme.C(theme.RGB(60, 200, 90)),
		Border: theme.C(theme.RGB(220, 220, 220)),
		Text:   theme.C(theme.RGB(255, 210, 60)),
		PadX:   theme.N(2),
		PadY:   theme.N(2),
		Corner: theme.N(2),
	})
	p.SetMetric(KeyTrayIconSize, size)

	if err := m.RegisterTheme(p); err != nil {
		t.Fatalf("RegisterTheme: %v", err)
	}
	if err := m.SetTheme("TestSized"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}
	return m
}

// ─── 1. Различимость состояний ──────────────────────────────────────────────

// TestNetworkItem_NoneDiffersFromWeakSignal — отключённая сеть (NetNone) не
// должна рисоваться так же, как сеть со слабым сигналом: раньше и то и другое
// сводилось к ratio≈0, то есть к одинаковому набору тусклых полосок, и
// отличить «отключено» от «еле ловит» было нельзя.
//
// Признак — другая ФИГУРА, а не другой оттенок: перечёркивание. Полоски под
// ним остаются, и это тоже проверяется: одна голая диагональ сообщает «что-то
// перечёркнуто», а не «нет сети», — значок обязан сперва читаться как значок
// сети и только потом как выключенный.
func TestNetworkItem_NoneDiffersFromWeakSignal(t *testing.T) {
	tm := testThemeManager(t)
	st := NewFakeSystemStatus()
	it := NewNetworkStatus(tm, st)
	defer it.Close()
	it.SetBounds(image.Rect(0, 0, 16, 16))

	st.SetNetwork(NetState{Kind: NetWiFi, Quality: 0.05, Name: "Слабая"})
	weak := &recPixel{}
	it.Draw(weak)

	st.SetNetwork(NetState{Kind: NetNone})
	none := &recPixel{}
	it.Draw(none)

	// Перечёркивание есть только у отключённой — это и есть признак,
	// который замечают мельком.
	if len(none.pixels) == 0 {
		t.Error("NetNone не перечёркнута — «нет сети» ничем не отмечена")
	}
	if len(weak.pixels) != 0 {
		t.Errorf("слабый, но живой сигнал тоже перечёркнут: %d точек — так его не отличить от NetNone",
			len(weak.pixels))
	}

	// Полоски рисуются в обоих случаях: значок остаётся значком сети.
	// +1 к networkBars — подложка, которую PaintStyle красит до полосок.
	if want := networkBars + 1; len(weak.fills) != want {
		t.Errorf("слабый сигнал нарисовал %d заливок, ждали %d (подложка и все деления шкалы)",
			len(weak.fills), want)
	}
	if len(none.fills) != len(weak.fills) {
		t.Errorf("у отключённой сети %d заливок против %d у слабой — значок перестал быть значком сети",
			len(none.fills), len(weak.fills))
	}

	// А сами полоски у отключённой все тусклые: горящих делений нет.
	s := trayStyle(tm, ComponentNetwork, theme.StateNormal)
	lit, muted := ink(s), mutedInk(s)
	if lit == muted {
		t.Skip("тема не различает горящее и тусклое — проверять нечего")
	}
	litCount := 0
	for _, f := range none.fills {
		if f.col == lit {
			litCount++
		}
	}
	if litCount != 0 {
		t.Errorf("у отключённой сети горит %d делений шкалы", litCount)
	}
}

// TestVolumeItem_MutedDiffersFromUnmutedAtSizes — Muted обязан отличаться от
// обычного состояния при любом размере значка, не только при исходном 16px.
func TestVolumeItem_MutedDiffersFromUnmutedAtSizes(t *testing.T) {
	for _, size := range []int{12, 16, 24, 32} {
		size := size
		t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
			tm := testThemeManagerSized(t, float64(size))
			st := NewFakeSystemStatus()
			it := NewVolumeStatus(tm, st)
			defer it.Close()
			it.SetBounds(image.Rect(0, 0, size, size))

			st.SetVolume(VolState{Level: 0.6})
			unmuted := &recPixel{}
			it.Draw(unmuted)

			st.SetVolume(VolState{Level: 0.6, Muted: true})
			muted := &recPixel{}
			it.Draw(muted)

			if len(unmuted.fills) == 0 {
				t.Fatal("обычное состояние не нарисовало ни одной заливки")
			}
			if len(unmuted.fills) == len(muted.fills) {
				t.Errorf("muted и обычное состояние нарисовали одинаковое число заливок: %d", len(unmuted.fills))
			}
			if len(muted.pixels) == 0 {
				t.Error("Muted не нарисовал перечёркивание вовсе — на этом размере значок неотличим от простого пропадания шкалы")
			}
		})
	}
}

// TestVolumeItem_MutedStrikeThickensWithSize — толщина перечёркивания растёт
// вместе со значком (или как минимум не падает): однопиксельная диагональ,
// достаточная на исходных 16px, тонет среди остальных фигур на поднятом
// заказчиком KeyTrayIconSize (24, 32).
func TestVolumeItem_MutedStrikeThickensWithSize(t *testing.T) {
	sizes := []int{12, 16, 24, 32}
	thickness := make(map[int]int, len(sizes))
	prev := 0
	for _, size := range sizes {
		tm := testThemeManagerSized(t, float64(size))
		st := NewFakeSystemStatus()
		st.SetVolume(VolState{Muted: true})
		it := NewVolumeStatus(tm, st)
		it.SetBounds(image.Rect(0, 0, size, size))

		ctx := &recPixel{}
		it.Draw(ctx)
		it.Close()

		if len(ctx.pixels) == 0 {
			t.Fatalf("size=%d: перечёркивание Muted не нарисовано", size)
		}
		th := maxPixelsPerColumn(ctx.pixels)
		if th < 1 {
			t.Fatalf("size=%d: толщина перечёркивания = %d", size, th)
		}
		if th < prev {
			t.Errorf("size=%d: толщина перечёркивания (%d) меньше, чем на предыдущем, меньшем размере (%d)",
				size, th, prev)
		}
		thickness[size] = th
		prev = th
	}
	// Не просто «не уменьшилась» (это тривиально верно и для фиксированной
	// толщины в 1px — старое поведение drawDiagonal), а по-настоящему выросла
	// между самым мелким и самым крупным проверяемым размером: значок вчетверо
	// больше по площади обязан утолщить перечёркивание, а не просто длину линии.
	if thickness[32] <= thickness[12] {
		t.Errorf("толщина перечёркивания на size=32 (%d) не больше, чем на size=12 (%d) — не растёт с размером значка",
			thickness[32], thickness[12])
	}
}

// ─── 2. Подсказки при наведении ──────────────────────────────────────────────

// TestNetworkItem_ToolTipTracksState — подсказка сети непустая сразу после
// создания и меняется вслед за состоянием (обновляется в подписке на
// SystemStatus, не в Draw).
func TestNetworkItem_ToolTipTracksState(t *testing.T) {
	tm := testThemeManager(t)
	st := NewFakeSystemStatus() // Kind: NetWiFi, Quality: 0.8, Name: "Сеть"
	it := NewNetworkStatus(tm, st)
	defer it.Close()

	if got := it.GetToolTip(); got == "" {
		t.Fatal("подсказка сети пуста сразу после создания — до первого Draw")
	}
	if want := "Сеть: Сеть, подключено"; it.GetToolTip() != want {
		t.Errorf("GetToolTip() = %q, ждали %q", it.GetToolTip(), want)
	}

	st.SetNetwork(NetState{Kind: NetNone})
	if got, want := it.GetToolTip(), "Сеть: нет подключения"; got != want {
		t.Errorf("после отключения сети GetToolTip() = %q, ждали %q", got, want)
	}

	st.SetNetwork(NetState{Kind: NetEthernet, Quality: 1, Name: "Провод"})
	if got, want := it.GetToolTip(), "Сеть: Провод, подключено"; got != want {
		t.Errorf("после переключения на другую сеть GetToolTip() = %q, ждали %q", got, want)
	}

	// Имя пустое — подсказка не рисует пустоту после двоеточия.
	st.SetNetwork(NetState{Kind: NetWiFi, Quality: 0.5})
	if got, want := it.GetToolTip(), "Сеть: подключено"; got != want {
		t.Errorf("с пустым именем сети GetToolTip() = %q, ждали %q", got, want)
	}
}

// TestVolumeItem_ToolTipTracksState — подсказка звука непустая сразу после
// создания и меняется вслед за уровнем/Muted.
func TestVolumeItem_ToolTipTracksState(t *testing.T) {
	tm := testThemeManager(t)
	st := NewFakeSystemStatus() // Level: 0.65
	it := NewVolumeStatus(tm, st)
	defer it.Close()

	if got := it.GetToolTip(); got == "" {
		t.Fatal("подсказка звука пуста сразу после создания")
	}
	if want := "Звук: 65%"; it.GetToolTip() != want {
		t.Errorf("GetToolTip() = %q, ждали %q", it.GetToolTip(), want)
	}

	st.SetVolume(VolState{Level: 0.4, Muted: true})
	if got, want := it.GetToolTip(), "Звук: выключен"; got != want {
		t.Errorf("после Muted GetToolTip() = %q, ждали %q", got, want)
	}

	st.SetVolume(VolState{Level: 0.4})
	if got, want := it.GetToolTip(), "Звук: 40%"; got != want {
		t.Errorf("после снятия Muted GetToolTip() = %q, ждали %q", got, want)
	}
}

// TestPowerItem_ToolTipTracksState — подсказка питания непустая сразу после
// создания и меняется вслед за OnAC/Charge.
func TestPowerItem_ToolTipTracksState(t *testing.T) {
	tm := testThemeManager(t)
	st := NewFakeSystemStatus() // Charge: 0.5, OnAC: true
	it := NewPowerStatus(tm, st)
	defer it.Close()

	if got := it.GetToolTip(); got == "" {
		t.Fatal("подсказка питания пуста сразу после создания")
	}
	if want := "Питание от сети"; it.GetToolTip() != want {
		t.Errorf("GetToolTip() = %q, ждали %q", it.GetToolTip(), want)
	}

	st.SetPower(PowerState{Charge: 0.8, OnAC: false})
	if got, want := it.GetToolTip(), "Батарея: 80%"; got != want {
		t.Errorf("после отключения от сети GetToolTip() = %q, ждали %q", got, want)
	}
}

// TestTrayIcons_ToolTipRace — Subscribe (горутина потребителя) и GetToolTip
// (горутина кадра, как его вызывал бы engine/tooltip.go) должны быть
// безопасны при гонке: без замка вокруг trayTooltip -race валит этот тест.
func TestTrayIcons_ToolTipRace(t *testing.T) {
	tm := testThemeManager(t)
	st := NewFakeSystemStatus()
	net := NewNetworkStatus(tm, st)
	vol := NewVolumeStatus(tm, st)
	pw := NewPowerStatus(tm, st)
	defer net.Close()
	defer vol.Close()
	defer pw.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			st.SetNetwork(NetState{Kind: NetWiFi, Quality: float64(i%10) / 10, Name: "Сеть"})
			st.SetVolume(VolState{Level: float64(i%10) / 10, Muted: i%2 == 0})
			st.SetPower(PowerState{Charge: float64(i%10) / 10, OnAC: i%3 == 0})
		}
	}()
	for i := 0; i < 200; i++ {
		_ = net.GetToolTip()
		_ = vol.GetToolTip()
		_ = pw.GetToolTip()
	}
	<-done
}

// ─── 3. Фигуры на крупном значке ────────────────────────────────────────────

// TestTrayIcons_ShapesStayWithinBounds — ни одна заливка и ни один пиксель
// перечёркивания не выходят за границы значка при размерах 12..32
// (KeyTrayIconSize уменьшать нельзя, но потребитель вправе задать любой
// размер от мелкого до вдвое большего).
func TestTrayIcons_ShapesStayWithinBounds(t *testing.T) {
	for _, size := range []int{12, 16, 24, 32} {
		size := size
		t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
			tm := testThemeManagerSized(t, float64(size))
			b := image.Rect(0, 0, size, size)

			check := func(name string, ctx *recPixel) {
				t.Helper()
				for _, f := range ctx.fills {
					r := image.Rect(f.x, f.y, f.x+f.w, f.y+f.h)
					if !r.In(b) {
						t.Errorf("%s: заливка %v вышла за границы значка %v", name, r, b)
					}
				}
				for _, p := range ctx.pixels {
					if !p.In(b) {
						t.Errorf("%s: пиксель перечёркивания %v вышел за границы значка %v", name, p, b)
					}
				}
			}

			st := NewFakeSystemStatus()

			net := NewNetworkStatus(tm, st)
			net.SetBounds(b)
			for _, ns := range []NetState{
				{Kind: NetNone},
				{Kind: NetWiFi, Quality: 0.3, Name: "W"},
				{Kind: NetEthernet, Quality: 1},
			} {
				st.SetNetwork(ns)
				ctx := &recPixel{}
				net.Draw(ctx)
				check("сеть", ctx)
			}
			net.Close()

			vol := NewVolumeStatus(tm, st)
			vol.SetBounds(b)
			for _, vs := range []VolState{
				{Level: 0.2},
				{Level: 1},
				{Level: 0.5, Muted: true},
			} {
				st.SetVolume(vs)
				ctx := &recPixel{}
				vol.Draw(ctx)
				check("звук", ctx)
			}
			vol.Close()

			pw := NewPowerStatus(tm, st)
			pw.SetBounds(b)
			for _, ps := range []PowerState{
				{Charge: 0.5, OnAC: true},
				{Charge: 1, OnAC: false},
				{Charge: 0, OnAC: false},
			} {
				st.SetPower(ps)
				ctx := &recPixel{}
				pw.Draw(ctx)
				check("питание", ctx)
			}
			pw.Close()
		})
	}
}

// TestDrawLevelBars_GapKeepsBarsApart — полоски одного цвета (соседние
// заполненные деления шкалы) не сливаются в один сплошной прямоугольник на
// крупном значке: ширина каждой полоски меньше её собственного шага, значит
// между их правым и левым краями есть промежуток.
func TestDrawLevelBars_GapKeepsBarsApart(t *testing.T) {
	on := color.RGBA{R: 255, A: 255}
	off := color.RGBA{A: 0}
	r := image.Rect(0, 0, 24, 24)

	ctx := &recCtx{}
	drawLevelBars(ctx, r, 1, networkBars, on, off) // ratio=1 — все деления залиты «on»

	if len(ctx.fills) != networkBars {
		t.Fatalf("нарисовано %d полосок, ждали %d", len(ctx.fills), networkBars)
	}
	colW := r.Dx() / networkBars
	for _, f := range ctx.fills {
		if f.w >= colW {
			t.Errorf("полоска шириной %d не уже шага %d — соседние полоски сольются на крупном значке", f.w, colW)
		}
	}
}
