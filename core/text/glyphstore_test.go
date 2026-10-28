package text

import (
	"sync"
	"testing"

	tsfont "github.com/go-text/typesetting/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/math/fixed"
)

// Один срез — быстрый путь по адресу; копия — по хэшу; оба дают один шрифт.
// Пустые и невалидные данные — nil без записи в индекс.
func TestGlyphStore_InternByContent(t *testing.T) {
	s := newGlyphStore()
	f1 := s.intern(goregular.TTF)
	if f1 == nil {
		t.Fatal("Go Regular не интернировался")
	}
	if f2 := s.intern(goregular.TTF); f2 != f1 {
		t.Error("тот же срез дал другой шрифт")
	}
	if f3 := s.intern(append([]byte(nil), goregular.TTF...)); f3 != f1 {
		t.Error("копия байт дала другой шрифт")
	}
	if len(s.byPtr) != 1 {
		t.Errorf("в адресном индексе %d записей, ожидалась 1 (канонический срез)", len(s.byPtr))
	}
	if s.intern(nil) != nil || s.intern([]byte{}) != nil || s.intern([]byte("не шрифт")) != nil {
		t.Error("пустые или невалидные данные интернировались")
	}
	if len(s.fonts) != 1 {
		t.Errorf("в индексе %d шрифтов, ожидался 1", len(s.fonts))
	}
	if f4 := s.intern(gomono.TTF); f4 == f1 || len(s.fonts) != 2 {
		t.Error("другой шрифт не получил своей записи")
	}
}

// Переполнение индекса выводит шрифт, который дольше всех не искали; живой
// шрифт продолжает работать.
func TestGlyphStore_IndexBounded(t *testing.T) {
	s := newGlyphStore()
	first := s.intern(goregular.TTF)
	for i := 0; i < maxInternedFonts; i++ {
		// Разное содержимое: хвостовые байты парсер не читает.
		data := append(append([]byte(nil), gomono.TTF...), byte(i), byte(i>>8))
		if s.intern(data) == nil {
			t.Fatalf("вариант %d не разобрался", i)
		}
	}
	if len(s.fonts) != maxInternedFonts {
		t.Errorf("в индексе %d шрифтов, ожидалось %d", len(s.fonts), maxInternedFonts)
	}
	if _, ok := s.byPtr[ptrKey{&goregular.TTF[0], len(goregular.TTF)}]; ok {
		t.Error("самый старый шрифт остался в адресном индексе")
	}
	if g := first.shard(10).glyphAt(faceKey{dpi: 96}, TextQuality{}, 'a', 0); !g.OK || g.Mask == nil {
		t.Error("выбывший из индекса шрифт перестал растеризовать")
	}
	if f := s.intern(goregular.TTF); f == first {
		t.Error("выбывший шрифт нашёлся в индексе")
	}
}

// Бюджет: вытесняются самые давно не нужные шарды (попадание делает шард
// свежим наравне с теми, у кого были промахи в ту же эпоху), учёт не уходит
// выше бюджета и возвращается к нулю после сброса.
func TestGlyphStore_EvictsLRU(t *testing.T) {
	ResetGlyphCache()
	SetGlyphCacheBudget(DefaultGlyphCacheBudget)
	t.Cleanup(func() { SetGlyphCacheBudget(DefaultGlyphCacheBudget); ResetGlyphCache() })
	fc := NewFontCacheFromData(goregular.TTF, DefaultDPI)
	fill := func(size float64) {
		for r := 'a'; r <= 'z'; r++ {
			fc.font.shard(size).glyphAt(faceKey{dpi: 96}, TextQuality{}, r, 0)
		}
	}
	fill(9)
	fill(10)
	fill(11)
	fc.font.shard(9).glyphAt(faceKey{dpi: 96}, TextQuality{}, 'a', 0) // попадание: 9 — самый свежий
	perShard := fc.font.shard(9).bytes
	if perShard == 0 {
		t.Fatal("шард без учёта байт")
	}
	SetGlyphCacheBudget(perShard * 2) // три шарда не умещаются
	if n := store.bytes.Load(); n > perShard*2 {
		t.Errorf("после сужения бюджета учёт %d > бюджета %d", n, perShard*2)
	}
	if fc.font.shard(10).bytes != 0 {
		t.Error("шард 10, к которому не обращались дольше всех, остался")
	}
	if alive9, alive11 := fc.font.shard(9).bytes != 0, fc.font.shard(11).bytes != 0; alive9 == alive11 {
		t.Errorf("после вытеснения до половины бюджета шарды 9 и 11: %v, %v — ожидался ровно один", alive9, alive11)
	}
	ResetGlyphCache()
	if st := GlyphCacheStatsNow(); st.Entries != 0 {
		t.Errorf("после сброса %d записей", st.Entries)
	}
}

// Два вида одного шрифта с разными DPI работают из разных горутин: общий
// шард, разные ключи, без гонок (-race).
func TestGlyphStore_ConcurrentViews(t *testing.T) {
	a := NewFontCacheFromData(goregular.TTF, 96)
	b := NewFontCacheFromData(goregular.TTF, 144)
	var wg sync.WaitGroup
	for _, fc := range []*FontCache{a, b} {
		wg.Add(1)
		go func(fc *FontCache) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				for r := 'A'; r <= 'Z'; r++ {
					fc.GlyphAt(11, r, i%SubpixelPhases)
					fc.Kern(11, r, 'o')
					fc.HasGlyph(r)
				}
				fc.Measure("The quick brown fox", 11)
				fc.VMetricsFull(11)
			}
		}(fc)
	}
	wg.Wait()
	if ga, gb := a.Glyph(11, 'A'), b.Glyph(11, 'A'); ga.Mask == gb.Mask || ga.Mask.Rect == gb.Mask.Rect {
		t.Error("виды с разными DPI получили одну маску")
	}
}

// Измерение через Advance совпадает с прежним измерением по face.
func TestFontCache_MeasureMatchesFace(t *testing.T) {
	fc := NewFontCacheFromData(goregular.TTF, DefaultDPI)
	for _, s := range []string{"Hello, world", "Кириллица", "a؁b", "AV To Wa"} {
		face := fc.Face(12)
		var want int
		{
			prev := rune(-1)
			var w fixed.Int26_6
			for _, r := range s {
				if prev >= 0 {
					w += face.Kern(prev, r)
				}
				a, ok := face.GlyphAdvance(r)
				if !ok {
					a, _ = face.GlyphAdvance('?')
				}
				w += a
				prev = r
			}
			want = w.Round()
		}
		if got := fc.Measure(s, 12); got != want {
			t.Errorf("Measure(%q) = %d, по face %d", s, got, want)
		}
	}
}

// Переполнение кэшей цветных глифов не роняет вставку (карты пересоздаются).
func TestGlyphStore_ColorOverflowNoPanic(t *testing.T) {
	font := &fontData{store: newGlyphStore()}
	font.colorState.Store(colorYes)
	ss := font.shapedShard(16 << 6)
	ss.colors = make(map[tsfont.GID]*ColorGlyph, maxShapedMasks)
	for i := 0; i < maxShapedMasks; i++ {
		ss.colors[tsfont.GID(i)] = nil
	}
	ss.dyn = make(map[dynKey]*ColorGlyph, maxDynColorGlyphs)
	for i := 0; i < maxDynColorGlyphs; i++ {
		ss.dyn[dynKey{gid: 1, col: colorKey{r: uint8(i)}}] = nil
	}
	// Шрифт без данных: typesetting не разберётся, face nil → запись nil.
	if cg := ss.colorGlyph(tsfont.GID(maxShapedMasks+1), colorKey{}); cg != nil {
		t.Errorf("цветной глиф без шрифта: %v", cg)
	}
	if len(ss.colors) > maxShapedMasks/4+1 {
		t.Errorf("после переполнения в colors %d записей", len(ss.colors))
	}
}

// Зоны хинтинга входят в учёт: после сброса учёт хранилища — ноль, а не
// отрицательный.
func TestGlyphStore_HintingAccounting(t *testing.T) {
	ResetGlyphCache()
	SetGlyphCacheBudget(DefaultGlyphCacheBudget)
	t.Cleanup(ResetGlyphCache)
	base := GlyphCacheStatsNow().Bytes
	fc := NewFontCacheFromData(goregular.TTF, DefaultDPI)
	fc.SetQuality(TextQuality{Hinting: true})
	for r := 'a'; r <= 'z'; r++ {
		fc.Glyph(11, r)
		fc.Kern(11, r, 'o')
	}
	fc.VMetricsFull(11)
	if n := GlyphCacheStatsNow().Bytes; n <= base {
		t.Fatalf("учёт не вырос: %d → %d", base, n)
	}
	ResetGlyphCache()
	if n := GlyphCacheStatsNow().Bytes; n != base {
		t.Errorf("после сброса учёт %d, ожидался %d", n, base)
	}
}

// Шрифт, выбывший из индекса, не трогает бюджет хранилища и работает под
// собственными лимитами.
func TestGlyphStore_DetachedFontOffBudget(t *testing.T) {
	s := newGlyphStore()
	first := s.intern(goregular.TTF)
	first.shard(10).glyphAt(faceKey{dpi: 96}, TextQuality{}, 'a', 0)
	if s.bytes.Load() == 0 {
		t.Fatal("учёт индексного шрифта не вырос")
	}
	for i := 0; i < maxInternedFonts; i++ {
		s.intern(append(append([]byte(nil), gomono.TTF...), byte(i), byte(i>>8)))
	}
	if !first.detached.Load() {
		t.Fatal("самый старый шрифт не выбыл из индекса")
	}
	if n := s.bytes.Load(); n != 0 {
		t.Errorf("после выбытия учёт %d, ожидался 0", n)
	}
	for r := 'a'; r <= 'z'; r++ {
		first.shard(10).glyphAt(faceKey{dpi: 96}, TextQuality{}, r, 0)
	}
	if n := s.bytes.Load(); n != 0 {
		t.Errorf("выбывший шрифт заряжает бюджет: %d", n)
	}
	if first.shard(10).size() == 0 {
		t.Error("шард выбывшего шрифта не ведёт свой учёт")
	}
}
