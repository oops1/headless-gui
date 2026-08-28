package desktop

import (
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// paint_bench_test.go — доказательство, что PaintGradient не аллоцирует на
// каждый вызов (см. комментарий у gradientStopsCache в paint.go), и что
// кэш стопов не путает цвета между стилями и не гонится между движками.
//
// recCtx — уже существующий записывающий DrawContext из clock_test.go
// (тот же пакет desktop, этот файл его не трогает, только использует).

// gradientStyle собирает минимальный *theme.Style с линейным градиентом по
// горизонтали (угол 0 — «слева направо», см. Style.GradientAngle):
// horizontalAngle(0) даёt Horizontal=true, и drawLinearGradient идёт
// поколоночным путём — ctx.FillRect на каждый столбец, а не DrawHLine,
// которую recCtx не записывает. Так тесты видят цвет каждой колонки.
func gradientStyle(stops ...theme.GradientStop) *theme.Style {
	return &theme.Style{
		Gradient:      stops,
		GradientAngle: 0,
	}
}

// manualStops — конвертация стопов «в лоб», как раньше делал PaintGradient
// на каждый вызов (make/append). Используется в тестах как независимый
// эталон, с которым сверяется закэшированный путь.
func manualStops(s *theme.Style) []widget.GradientStop {
	out := make([]widget.GradientStop, 0, len(s.Gradient))
	for _, st := range s.Gradient {
		out = append(out, widget.GradientStop{Color: st.Color, Offset: st.Pos})
	}
	return out
}

// BenchmarkPaintGradient — вызов PaintGradient на типичном для панели
// задач прямоугольнике (400×40). Замер -benchmem: до правки — 1 аллокация
// на вызов (make среза стопов внутри PaintGradient), после — 0, потому что
// стопы посчитаны один раз на указатель стиля и переиспользуются.
func BenchmarkPaintGradient(b *testing.B) {
	s := gradientStyle(
		theme.GradientStop{Pos: 0, Color: color.RGBA{R: 10, G: 20, B: 30, A: 255}},
		theme.GradientStop{Pos: 0.5, Color: color.RGBA{R: 100, G: 110, B: 120, A: 255}},
		theme.GradientStop{Pos: 1, Color: color.RGBA{R: 200, G: 210, B: 220, A: 255}},
	)
	r := image.Rect(0, 0, 400, 40)
	ctx := &recCtx{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.fills = ctx.fills[:0]
		PaintGradient(ctx, r, s)
	}
}

// Кэш не меняет картинку: закэшированный путь (два подряд вызова тем же
// стилем — первый заполняет кэш, второй берёт из него) обязан дать те же
// заливки, что и ручная конвертация стопов без всякого кэша.
func TestPaintGradient_CacheMatchesManualConversion(t *testing.T) {
	s := gradientStyle(
		theme.GradientStop{Pos: 0, Color: color.RGBA{R: 5, G: 6, B: 7, A: 255}},
		theme.GradientStop{Pos: 1, Color: color.RGBA{R: 250, G: 240, B: 230, A: 255}},
	)
	r := image.Rect(0, 0, 64, 10)

	ref := &recCtx{}
	widget.DrawLinearGradient(ref, r, &widget.LinearGradient{Horizontal: true, Stops: manualStops(s)})
	if len(ref.fills) == 0 {
		t.Fatal("эталон ничего не нарисовал")
	}

	for i := 0; i < 2; i++ {
		got := &recCtx{}
		PaintGradient(got, r, s)
		if len(got.fills) != len(ref.fills) {
			t.Fatalf("вызов %d: %d заливок, эталон — %d", i, len(got.fills), len(ref.fills))
		}
		for j := range ref.fills {
			if got.fills[j] != ref.fills[j] {
				t.Fatalf("вызов %d, заливка %d: %+v, эталон %+v", i, j, got.fills[j], ref.fills[j])
			}
		}
	}
}

// Смена темы — это смена указателя *theme.Style (resolve() строит новые
// Style с новыми адресами при каждой пересборке, см. theme/resolve.go).
// Кэш стопов ключуется указателем: другой указатель обязан дать СВОИ
// цвета, а не унаследовать цвета из-под другого указателя, — иначе после
// переключения темы панель осталась бы раскрашена старыми цветами.
func TestPaintGradient_StyleChangeIsNotStaleCache(t *testing.T) {
	before := gradientStyle(
		theme.GradientStop{Pos: 0, Color: color.RGBA{R: 255, A: 255}},
		theme.GradientStop{Pos: 1, Color: color.RGBA{B: 255, A: 255}},
	)
	after := gradientStyle(
		theme.GradientStop{Pos: 0, Color: color.RGBA{G: 255, A: 255}},
		theme.GradientStop{Pos: 1, Color: color.RGBA{R: 255, G: 255, A: 255}},
	)
	r := image.Rect(0, 0, 32, 8)

	ctxBefore := &recCtx{}
	PaintGradient(ctxBefore, r, before) // заполняет кэш под указателем before

	ctxAfter := &recCtx{}
	PaintGradient(ctxAfter, r, after) // другой указатель — обязан пересчитать свои цвета

	if len(ctxBefore.fills) == 0 || len(ctxAfter.fills) == 0 {
		t.Fatal("градиент не нарисован")
	}
	if ctxBefore.fills[0].col == ctxAfter.fills[0].col {
		t.Fatalf("после смены стиля первый цвет не изменился: %v", ctxBefore.fills[0].col)
	}

	// Старый указатель, вызванный ПОСЛЕ нового, обязан вернуть СТАРЫЕ
	// цвета — запись нового стиля не должна была затереть чужой ключ.
	ctxBeforeAgain := &recCtx{}
	PaintGradient(ctxBeforeAgain, r, before)
	if len(ctxBeforeAgain.fills) != len(ctxBefore.fills) {
		t.Fatalf("повторный вызов старым стилем: %d заливок, было %d",
			len(ctxBeforeAgain.fills), len(ctxBefore.fills))
	}
	for j := range ctxBefore.fills {
		if ctxBeforeAgain.fills[j] != ctxBefore.fills[j] {
			t.Fatalf("повторный вызов старым стилем: заливка %d изменилась: %+v → %+v",
				j, ctxBefore.fills[j], ctxBeforeAgain.fills[j])
		}
	}
}

// Несколько движков рисуют градиенты параллельно (как в
// tests/twoengines_test.go), каждый своим набором стилей и общими тоже —
// gradientStopsCache должен пережить это под -race без гонок и без
// перепутанных цветов.
func TestPaintGradient_ConcurrentStylesSafe(t *testing.T) {
	styles := make([]*theme.Style, 8)
	for i := range styles {
		styles[i] = gradientStyle(
			theme.GradientStop{Pos: 0, Color: color.RGBA{R: uint8(i * 10), A: 255}},
			theme.GradientStop{Pos: 1, Color: color.RGBA{B: uint8(255 - i*10), A: 255}},
		)
	}
	r := image.Rect(0, 0, 40, 6)

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				s := styles[(g+i)%len(styles)]
				ctx := &recCtx{}
				PaintGradient(ctx, r, s)
				if len(ctx.fills) == 0 {
					t.Errorf("горутина %d: градиент не нарисован", g)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}
