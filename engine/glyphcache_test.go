package engine

// glyphcache_test.go — приёмка ADR-0003: общий на процесс кэш растров
// глифов. Два движка делят маски; Stop одного не трогает кэш другого;
// бюджет держит память; копии байт шрифта не удерживаются.

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"runtime"
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/gomono"

	hgtext "github.com/oops1/headless-gui/v3/core/text"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

const glyphTestText = "Hello, мир! 0123 ✓"

// Два движка рисуют одну строку: маски указательно одни, записей в общем
// кэше после второго движка не прибавляется; другой DPI — другие записи;
// Stop первого движка кэш второго не трогает.
func TestGlyphCache_TwoEnginesShareRasters(t *testing.T) {
	testreset.Globals(t)
	e1, e2 := New(200, 60, 20), New(200, 60, 20)
	defer e2.Stop()
	col := color.RGBA{255, 255, 255, 255}

	e1.canvas.DrawTextSize(glyphTestText, 4, 4, 12, col)
	after1 := hgtext.GlyphCacheStatsNow()
	e2.canvas.DrawTextSize(glyphTestText, 4, 4, 12, col)
	after2 := hgtext.GlyphCacheStatsNow()
	if after2.Entries != after1.Entries || after2.Bytes != after1.Bytes {
		t.Errorf("второй движок добавил записей: %+v → %+v", after1, after2)
	}
	g1, g2 := e1.canvas.DefaultFont().Glyph(12, 'H'), e2.canvas.DefaultFont().Glyph(12, 'H')
	if g1.Mask == nil || g1.Mask != g2.Mask {
		t.Fatalf("маски двух движков не общие: %p и %p", g1.Mask, g2.Mask)
	}

	// Другой DPI — другие маски: ключ различает параметры вида.
	e2.SetDPI(120)
	e2.canvas.DrawTextSize(glyphTestText, 4, 4, 12, col)
	after3 := hgtext.GlyphCacheStatsNow()
	if after3.Entries <= after2.Entries {
		t.Errorf("смена DPI не дала новых записей: %+v → %+v", after2, after3)
	}
	if g := e2.canvas.DefaultFont().Glyph(12, 'H'); g.Mask == g1.Mask {
		t.Error("после SetDPI(120) маска та же, что при 96")
	}
	e2.SetDPI(DefaultDPI)

	// Stop первого движка: у второго те же указатели и та же статистика.
	e1.Stop()
	before := hgtext.GlyphCacheStatsNow()
	e2.canvas.DrawTextSize(glyphTestText, 4, 4, 12, col)
	if g := e2.canvas.DefaultFont().Glyph(12, 'H'); g.Mask != g1.Mask {
		t.Error("после Stop первого движка второй получил другую маску")
	}
	if st := hgtext.GlyphCacheStatsNow(); st != before {
		t.Errorf("после Stop первого движка кэш изменился: %+v → %+v", before, st)
	}
}

// Два движка рендерят одновременно (у первого — клон-холст попапа с
// RTL-строкой), третья горутина меряет текст обоих, четвёртая регистрирует
// шрифт из копии байт. Под -race.
func TestGlyphCache_RaceTwoEnginesAndMeasurer(t *testing.T) {
	testreset.Globals(t)
	scene := func(e *Engine, label string) {
		p := widget.NewPanel(color.RGBA{30, 30, 34, 255})
		p.SetBounds(image.Rect(0, 0, 320, 120))
		for i := 0; i < 4; i++ {
			l := widget.NewLabel(fmt.Sprintf("%s %d ✓ שלום", label, i), color.RGBA{220, 220, 220, 255})
			l.SetBounds(image.Rect(4, 4+i*26, 316, 28+i*26))
			p.AddChild(l)
		}
		e.SetRoot(p)
	}
	e1, e2 := New(320, 120, 20), New(320, 120, 20)
	defer e1.Stop()
	defer e2.Stop()
	scene(e1, "один")
	scene(e2, "два")
	popup := e1.canvas.CloneForSize(200, 40, 1, nil)
	col := color.RGBA{255, 255, 255, 255}

	const rounds = 40
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			e1.Invalidate()
			e1.renderFrame()
			popup.DrawTextSize("שלום עולם", 4, 4, 14, col)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			e2.Invalidate()
			e2.renderFrame()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < rounds*4; i++ {
			e1.canvas.MeasureText("✓ abc שלום", 12)
			e2.canvas.MeasureText("✓ abc שלום", 12)
			e1.canvas.MeasureTextFont("Job mono", 12, "Job0")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 8; i++ {
			e1.RegisterFont(fmt.Sprintf("Job%d", i), append([]byte(nil), gomono.TTF...))
		}
	}()
	wg.Wait()
}

// Бюджет держит учёт байт, а кадр при тесном бюджете (с вытеснениями прямо
// внутри кадра) побайтно равен кадру при бюджете по умолчанию. Второй кадр
// рисует свежий движок: его L1 пуст, и каждый глиф идёт через общий кэш.
func TestGlyphCache_BudgetBounded(t *testing.T) {
	testreset.Globals(t)
	col := color.RGBA{255, 255, 255, 255}
	alphabets := []string{
		"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ",
		"абвгдеёжзийклмнопрстуфхцчшщъыьэюяАБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯ",
		"αβγδεζηθικλμνξοπρστυφχψω0123456789 ✓✗⚠",
	}
	render := func(e *Engine) []byte {
		e.canvas.FillRect(0, 0, 900, 400, color.RGBA{0, 0, 0, 255})
		y := 4
		for _, size := range []float64{8, 10, 12, 16, 24} {
			for _, s := range alphabets {
				e.canvas.DrawTextSize(s, 4, y, size, col)
				y += int(size*1.5) + 2
			}
		}
		return append([]byte(nil), e.canvas.Back().Pix...)
	}
	e1 := New(900, 400, 20)
	defer e1.Stop()
	want := render(e1)

	const budget = 256 << 10
	hgtext.SetGlyphCacheBudget(budget)
	hgtext.ResetGlyphCache()
	e2 := New(900, 400, 20)
	defer e2.Stop()
	got := render(e2)
	st := hgtext.GlyphCacheStatsNow()
	if st.Entries == 0 || st.Bytes == 0 {
		t.Fatalf("общий кэш не задействован: %+v", st)
	}
	if st.Bytes > budget {
		t.Errorf("учёт %d байт выше бюджета %d", st.Bytes, budget)
	}
	if !bytes.Equal(got, want) {
		t.Error("кадр при тесном бюджете отличается от кадра при бюджете по умолчанию")
	}
}

// Копии байт шрифта интернируются в тот же шрифт и не удерживаются;
// пустые данные не регистрируются и не паникуют.
func TestGlyphCache_InternCopies(t *testing.T) {
	testreset.Globals(t)
	e := New(64, 64, 20)
	defer e.Stop()
	e.RegisterFont("Mono0", gomono.TTF)
	base := hgtext.GlyphCacheStatsNow().Fonts

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 1; i <= 200; i++ {
		e.RegisterFont(fmt.Sprintf("Mono%d", i), append([]byte(nil), gomono.TTF...))
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if st := hgtext.GlyphCacheStatsNow(); st.Fonts != base {
		t.Errorf("копии байт дали новые шрифты в индексе: %d → %d", base, st.Fonts)
	}
	// 200 копий Go Mono — больше 30 МБ; удерживаться они не должны.
	if grew := int64(after.HeapInuse) - int64(before.HeapInuse); grew > 8<<20 {
		t.Errorf("после 200 регистраций копий куча выросла на %d байт", grew)
	}
	if fc := e.canvas.FontFor("Mono200"); fc == e.canvas.DefaultFont() {
		t.Error("шрифт из копии байт не зарегистрировался")
	}

	e.RegisterFont("Empty", nil)
	e.RegisterFont("Empty", []byte{})
	if e.RegisterFallbackFont([]byte{}) {
		t.Error("пустой запасной шрифт принят")
	}
	if e.canvas.HasFont("Empty") {
		t.Error("пустые данные зарегистрированы как шрифт")
	}
	if st := hgtext.GlyphCacheStatsNow(); st.Fonts != base {
		t.Errorf("пустые данные попали в индекс: %d → %d", base, st.Fonts)
	}
}

// BenchmarkDrawTextTwoEngines — два движка рисуют одну строку одним шрифтом
// и кеглем параллельно; ns/op — на строку на поток. Не хуже DrawText: L1
// вида личный, общий кэш трогается только на промахе.
func BenchmarkDrawTextTwoEngines(b *testing.B) {
	e1, e2 := New(640, 480, 20), New(640, 480, 20)
	col := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	const s = "Пример строки Example string 0123456789"
	e1.canvas.DrawTextSize(s, 10, 100, 12, col)
	e2.canvas.DrawTextSize(s, 10, 100, 12, col)
	b.ReportAllocs()
	b.ResetTimer()
	var wg sync.WaitGroup
	wg.Add(2)
	for _, e := range []*Engine{e1, e2} {
		go func(c *Canvas) {
			defer wg.Done()
			for i := 0; i < b.N; i++ {
				c.DrawTextSize(s, 10, 100, 12, col)
			}
		}(e.canvas)
	}
	wg.Wait()
}
