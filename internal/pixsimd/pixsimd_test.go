package pixsimd

import (
	"bytes"
	"math/rand"
	"os"
	"testing"
)

// Векторная версия обязана совпадать со скалярной бит в бит: по этому
// работают эталонные кадры движка. В сборке без GOEXPERIMENT=simd выбранная
// реализация и есть скалярная — тесты тогда проверяют лишь сами себя, и это
// нормально: смысл у них в сборке с экспериментом (go test с GOEXPERIMENT=simd).

func TestLogImpl(t *testing.T) { t.Logf("реализация: %s", Impl()) }

// PIXSIMD_REQUIRE=avx2 требует векторный путь. Задача CI с GOEXPERIMENT=simd
// ставит её, чтобы не пройти молча на скалярной версии — например, на машине
// без AVX2 или со сломанными тегами сборки, когда векторный файл не попал в
// сборку и все проверки «вектор = скаляр» сравнивали скаляр сам с собой.
func TestRequiredImpl(t *testing.T) {
	want := os.Getenv("PIXSIMD_REQUIRE")
	if want == "" {
		t.Skip("PIXSIMD_REQUIRE не задан")
	}
	if Impl() != want {
		t.Fatalf("выбрана реализация %q, требуется %q", Impl(), want)
	}
}

// Замена деления на 65535 точна на всех значениях, которые возникают в ядрах.
// Перебор тот же, что был прогнан перед написанием векторных версий: если его
// когда-нибудь сломать (сменив формулы в движке), тест покажет, где.
func TestDiv65535Exact(t *testing.T) {
	if testing.Short() {
		t.Skip("перебор 16 млн значений — не для -short")
	}
	fast := func(x uint32) uint32 { return (x + (x >> 16) + 1) >> 16 }
	check := func(x uint64) {
		if x > 0xFFFFFFFF {
			t.Fatalf("произведение %d не влезает в 32 бита", x)
		}
		if uint32(x/m16) != fast(uint32(x)) {
			t.Fatalf("x=%d: %d вместо %d", x, fast(uint32(x)), x/m16)
		}
	}
	for p := uint64(0); p < 256; p++ {
		for inv := uint64(0); inv <= m16; inv++ {
			check(p * 0x101 * inv) // фон в BlendMaskRow
		}
		for a := uint64(0); a < 256; a++ {
			check(p * 0x101 * a * 0x101)         // цвет × покрытие
			check(p * ((m16 - a*0x101) * 0x101)) // фон в OverSolidRow
		}
	}
}

func randBytes(r *rand.Rand, n int) []byte {
	b := make([]byte, n)
	r.Read(b)
	return b
}

// Маска со всеми краевыми значениями: 0 (пропуск у скаляра), 255, середина.
func randMask(r *rand.Rand, n int) []byte {
	m := randBytes(r, n)
	for i := range m {
		switch r.Intn(4) {
		case 0:
			m[i] = 0
		case 1:
			m[i] = 255
		}
	}
	return m
}

func TestBlendMaskRowMatchesGeneric(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for iter := 0; iter < 3000; iter++ {
		n := r.Intn(70) // длины и меньше восьми, и с хвостом
		mask := randMask(r, n)
		dst := randBytes(r, n*4)
		want := bytes.Clone(dst)

		// Цвет и premultiplied, и нет: у второго сумма может перевалить за
		// 255, и векторная версия обязана отбросить старшие биты так же.
		c := [4]uint32{uint32(r.Intn(256)), uint32(r.Intn(256)), uint32(r.Intn(256)), uint32(r.Intn(256))}
		s := func(v uint32) uint32 { return v * 0x101 }

		blendMaskRowGeneric(want, mask, s(c[0]), s(c[1]), s(c[2]), s(c[3]))
		BlendMaskRow(dst, mask, s(c[0]), s(c[1]), s(c[2]), s(c[3]))
		if !bytes.Equal(dst, want) {
			t.Fatalf("итерация %d, длина %d, цвет %v: расхождение\n  вектор %v\n  скаляр %v",
				iter, n, c, dst, want)
		}
	}
}

// Перебор по всем парам «фон × покрытие» для нескольких цветов: случайные
// тесты могут не попасть в редкий край, перебор — попадёт.
func TestBlendMaskRowExhaustive(t *testing.T) {
	colors := [][4]uint32{{0, 0, 0, 255}, {255, 255, 255, 255}, {0, 120, 215, 255}, {40, 40, 40, 128}, {255, 0, 0, 10}}
	for _, c := range colors {
		s := [4]uint32{c[0] * 0x101, c[1] * 0x101, c[2] * 0x101, c[3] * 0x101}
		for m := 0; m < 256; m++ {
			mask := bytes.Repeat([]byte{byte(m)}, 256)
			dst := make([]byte, 256*4)
			for p := 0; p < 256; p++ {
				dst[p*4], dst[p*4+1], dst[p*4+2], dst[p*4+3] = byte(p), byte(255-p), byte(p/2), byte(p)
			}
			want := bytes.Clone(dst)
			blendMaskRowGeneric(want, mask, s[0], s[1], s[2], s[3])
			BlendMaskRow(dst, mask, s[0], s[1], s[2], s[3])
			if !bytes.Equal(dst, want) {
				t.Fatalf("цвет %v, покрытие %d: расхождение", c, m)
			}
		}
	}
}

func TestOverSolidRowMatchesGeneric(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for iter := 0; iter < 3000; iter++ {
		n := r.Intn(70) * 4
		row := randBytes(r, n)
		want := bytes.Clone(row)
		A := uint32(r.Intn(256))
		sa := A * 0x101
		a := (m16 - sa) * 0x101
		sr, sg, sb := uint32(r.Intn(256))*0x101, uint32(r.Intn(256))*0x101, uint32(r.Intn(256))*0x101
		overSolidRowGeneric(want, a, sr, sg, sb, sa)
		OverSolidRow(row, a, sr, sg, sb, sa)
		if !bytes.Equal(row, want) {
			t.Fatalf("итерация %d, длина %d, альфа %d: расхождение", iter, n/4, A)
		}
	}
}

func TestSwapRBMatchesGeneric(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for iter := 0; iter < 500; iter++ {
		n := r.Intn(90) * 4
		src := randBytes(r, n)
		for _, tc := range []struct {
			name      string
			fast, ref func(dst, src []byte)
		}{
			{"SwapRB", SwapRB, swapRBGeneric},
			{"SwapRBOpaque", SwapRBOpaque, swapRBOpaqueGeneric},
		} {
			got, want := make([]byte, n), make([]byte, n)
			tc.fast(got, src)
			tc.ref(want, src)
			if !bytes.Equal(got, want) {
				t.Fatalf("%s, длина %d: расхождение", tc.name, n/4)
			}
		}
	}
}

// Перестановка на месте (dst и src — один буфер) тоже обязана работать:
// скалярная версия это умеет, и вызывающий вправе на это рассчитывать.
func TestSwapRBInPlace(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	buf := randBytes(r, 67*4)
	want := make([]byte, len(buf))
	swapRBGeneric(want, buf)
	SwapRB(buf, buf)
	if !bytes.Equal(buf, want) {
		t.Fatal("перестановка на месте разошлась со скалярной")
	}
}

// ─── Бенчмарки: выбранная реализация против скалярной ───────────────────────

const benchPx = 1920 // строка кадра 1080p

func BenchmarkBlendMaskRow(b *testing.B) {
	r := rand.New(rand.NewSource(5))
	mask, dst := randMask(r, benchPx), randBytes(r, benchPx*4)
	b.SetBytes(benchPx * 4)
	for i := 0; i < b.N; i++ {
		BlendMaskRow(dst, mask, 0x7878, 0xD7D7, 0x1010, 0xFFFF)
	}
}

func BenchmarkBlendMaskRowGeneric(b *testing.B) {
	r := rand.New(rand.NewSource(5))
	mask, dst := randMask(r, benchPx), randBytes(r, benchPx*4)
	b.SetBytes(benchPx * 4)
	for i := 0; i < b.N; i++ {
		blendMaskRowGeneric(dst, mask, 0x7878, 0xD7D7, 0x1010, 0xFFFF)
	}
}

func BenchmarkOverSolidRow(b *testing.B) {
	row := randBytes(rand.New(rand.NewSource(6)), benchPx*4)
	b.SetBytes(benchPx * 4)
	for i := 0; i < b.N; i++ {
		OverSolidRow(row, (m16-0x5A5A)*0x101, 0, 0, 0, 0x5A5A)
	}
}

func BenchmarkOverSolidRowGeneric(b *testing.B) {
	row := randBytes(rand.New(rand.NewSource(6)), benchPx*4)
	b.SetBytes(benchPx * 4)
	for i := 0; i < b.N; i++ {
		overSolidRowGeneric(row, (m16-0x5A5A)*0x101, 0, 0, 0, 0x5A5A)
	}
}

func BenchmarkSwapRB(b *testing.B) {
	src, dst := randBytes(rand.New(rand.NewSource(7)), benchPx*4), make([]byte, benchPx*4)
	b.SetBytes(benchPx * 4)
	for i := 0; i < b.N; i++ {
		SwapRB(dst, src)
	}
}

func BenchmarkSwapRBGeneric(b *testing.B) {
	src, dst := randBytes(rand.New(rand.NewSource(7)), benchPx*4), make([]byte, benchPx*4)
	b.SetBytes(benchPx * 4)
	for i := 0; i < b.N; i++ {
		swapRBGeneric(dst, src)
	}
}
