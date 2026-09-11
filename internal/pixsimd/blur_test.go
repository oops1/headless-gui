package pixsimd

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
)

// Деление умножением точно на всех суммах окна: перебор для окон до 1024 и
// края (кратные d, соседние с ними, наибольшая сумма) для окон пошире.
func TestDividerExact(t *testing.T) {
	if testing.Short() {
		t.Skip("перебор сотен миллионов значений — не для -short")
	}
	check := func(d int, n uint32) {
		q := newDivider(d)
		if got, want := q.div(n), uint8(n/uint32(d)); got != want {
			t.Fatalf("d=%d, n=%d: %d вместо %d", d, n, got, want)
		}
	}
	for d := 1; d <= 1024; d++ {
		for n := uint32(0); n <= uint32(255*d); n++ {
			check(d, n)
		}
	}
	for d := 1025; d < 1<<17; d += 97 {
		for k := uint32(0); k <= 255; k++ {
			for _, n := range []uint32{k * uint32(d), k*uint32(d) + uint32(d) - 1} {
				check(d, n)
			}
		}
	}
	check(65535, 255*65535)
	check(65537, 255*65537) // здесь уже честное деление
}

// Векторное деление (⌈2²⁴/d⌉, сдвиг 24, младшие 32 бита произведения) точно
// на всех суммах для всех окон векторного пути.
func TestRecip24Exact(t *testing.T) {
	for d := 1; d <= maxVecWin; d++ {
		m := uint64(recip24(d))
		for n := uint64(0); n <= uint64(255*d); n++ {
			p := n * m
			if p > 0xFFFFFFFF {
				t.Fatalf("d=%d, n=%d: произведение не влезает в 32 бита", d, n)
			}
			if p>>24 != n/uint64(d) {
				t.Fatalf("d=%d, n=%d: %d вместо %d", d, n, p>>24, n/uint64(d))
			}
		}
	}
}

// Прежний алгоритм движка — с честным делением, дословно. Скалярная версия
// пакета обязана совпадать с ним: эталонные кадры рисовались им.
func legacyBlurRun(p []byte, step, n, radius int, out []byte) {
	win := uint32(2*radius + 1)
	var sR, sG, sB, sA uint32
	for k := -radius; k <= radius; k++ {
		i := clampInt(k, 0, n-1) * step
		sR, sG, sB, sA = sR+uint32(p[i]), sG+uint32(p[i+1]), sB+uint32(p[i+2]), sA+uint32(p[i+3])
	}
	for x := 0; x < n; x++ {
		out[x*4], out[x*4+1], out[x*4+2], out[x*4+3] = uint8(sR/win), uint8(sG/win), uint8(sB/win), uint8(sA/win)
		po, pi := clampInt(x-radius, 0, n-1)*step, clampInt(x+radius+1, 0, n-1)*step
		sR += uint32(p[pi]) - uint32(p[po])
		sG += uint32(p[pi+1]) - uint32(p[po+1])
		sB += uint32(p[pi+2]) - uint32(p[po+2])
		sA += uint32(p[pi+3]) - uint32(p[po+3])
	}
	for x := 0; x < n; x++ {
		copy(p[x*step:x*step+4], out[x*4:x*4+4])
	}
}

type blurCase struct{ w, h, stride, radius int }

func randBlurCase(r *rand.Rand) blurCase {
	w, h := 1+r.Intn(45), 1+r.Intn(45)
	radii := []int{1, 2, 3, 5, 8, 17, 60, 127, 128, 200}
	return blurCase{w, h, (w + r.Intn(5)) * 4, radii[r.Intn(len(radii))]}
}

// Скаляр пакета = прежний алгоритм движка.
func TestBoxBlurGenericMatchesLegacy(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	var bb BlurBuf
	for iter := 0; iter < 400; iter++ {
		c := randBlurCase(r)
		buf := randBytes(r, c.h*c.stride+4)
		for _, rows := range []bool{true, false} {
			got, want := bytes.Clone(buf), bytes.Clone(buf)
			out := make([]byte, max(c.w, c.h)*4)
			if rows {
				boxBlurRowsGeneric(&bb, got, c.stride, c.w, c.h, c.radius)
				for y := 0; y < c.h; y++ {
					legacyBlurRun(want[y*c.stride:], 4, c.w, c.radius, out)
				}
			} else {
				boxBlurColsGeneric(&bb, got, c.stride, c.w, c.h, c.radius)
				for x := 0; x < c.w; x++ {
					legacyBlurRun(want[x*4:], c.stride, c.h, c.radius, out)
				}
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("итерация %d, %+v, строки=%v: расхождение с прежним алгоритмом", iter, c, rows)
			}
		}
	}
}

// Выбранная реализация = скаляр, весь буфер (пиксели вокруг области не
// тронуты). Размеры — с остатками по восемь и меньше восьми, радиусы — и
// больше области, и за пределом векторного пути.
func TestBoxBlurMatchesGeneric(t *testing.T) {
	r := rand.New(rand.NewSource(12))
	var bbA, bbB BlurBuf
	for iter := 0; iter < 1500; iter++ {
		c := randBlurCase(r)
		buf := randBytes(r, c.h*c.stride+4)
		for _, rows := range []bool{true, false} {
			got, want := bytes.Clone(buf), bytes.Clone(buf)
			if rows {
				BoxBlurRows(&bbA, got, c.stride, c.w, c.h, c.radius)
				boxBlurRowsGeneric(&bbB, want, c.stride, c.w, c.h, c.radius)
			} else {
				BoxBlurCols(&bbA, got, c.stride, c.w, c.h, c.radius)
				boxBlurColsGeneric(&bbB, want, c.stride, c.w, c.h, c.radius)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("итерация %d, %+v, строки=%v: расхождение с эталоном", iter, c, rows)
			}
		}
	}
}

// ─── Автоматический обход векторного пути ──────────────────────────────────

// Рабочий набор проходит самопроверку, скалярный — тем более.
func TestSelfTestPasses(t *testing.T) {
	if err := selfTest(generic); err != nil {
		t.Fatalf("скалярный набор: %v", err)
	}
	if err := selfTest(active); err != nil {
		t.Fatalf("выбранный набор %s: %v", active.name, err)
	}
}

// Набор, считающий не так, не ставится: остаётся прежний, причина
// записывается. Без этой проверки самопроверка могла бы молча пропускать всё.
func TestInstallRejectsWrongKernel(t *testing.T) {
	prevActive, prevFallback := active, fallback
	defer func() { active, fallback = prevActive, prevFallback }()
	active, fallback = generic, ""

	broken := generic
	broken.name = "broken"
	broken.blurCols = func(bb *BlurBuf, pix []byte, stride, w, h, radius int) {
		boxBlurColsGeneric(bb, pix, stride, w, h, radius)
		pix[0] ^= 1 // ошибка на единицу в одном канале одного пикселя
	}
	install(broken)
	if active.name != "generic" {
		t.Fatal("набор с ошибкой поставлен")
	}
	if !strings.Contains(fallback, "blurCols") {
		t.Fatalf("причина отказа не названа: %q", fallback)
	}
}

// Паника ядра — тоже отказ, а не падение программы при запуске.
func TestInstallRejectsPanickingKernel(t *testing.T) {
	prevActive, prevFallback := active, fallback
	defer func() { active, fallback = prevActive, prevFallback }()
	active, fallback = generic, ""

	broken := generic
	broken.name = "panicky"
	broken.swapRB = func(dst, src []byte) { panic("нет такой команды") }
	install(broken)
	if active.name != "generic" || !strings.Contains(fallback, "паника") {
		t.Fatalf("паника ядра: active=%s, fallback=%q", active.name, fallback)
	}
}

// ─── Бенчмарки ─────────────────────────────────────────────────────────────

// Типичная подложка: полоса панели задач 1920×48 после уменьшения вчетверо
// с запасом по краям — 480×18, радиус 6; и прежний замер BlurRGBA — 480×120.
func benchBlur(b *testing.B, w, h, radius int, rows, cols func(*BlurBuf, []byte, int, int, int, int)) {
	pix := randBytes(rand.New(rand.NewSource(13)), w*h*4)
	var bb BlurBuf
	b.SetBytes(int64(w * h * 4))
	for i := 0; i < b.N; i++ {
		rows(&bb, pix, w*4, w, h, radius)
		cols(&bb, pix, w*4, w, h, radius)
	}
}

func BenchmarkBoxBlur480x120(b *testing.B) { benchBlur(b, 480, 120, 8, BoxBlurRows, BoxBlurCols) }
func BenchmarkBoxBlur480x120Generic(b *testing.B) {
	benchBlur(b, 480, 120, 8, boxBlurRowsGeneric, boxBlurColsGeneric)
}
func BenchmarkBoxBlur480x18(b *testing.B) { benchBlur(b, 480, 18, 6, BoxBlurRows, BoxBlurCols) }
func BenchmarkBoxBlur480x18Generic(b *testing.B) {
	benchBlur(b, 480, 18, 6, boxBlurRowsGeneric, boxBlurColsGeneric)
}
