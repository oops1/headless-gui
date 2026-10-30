package raster

import (
	"bytes"
	hgtext "github.com/oops1/headless-gui/v3/core/text"
	"github.com/oops1/headless-gui/v3/internal/testhook"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"golang.org/x/image/font/gofont/gomono"
	"image/color"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// fbChain — три файла запасной цепочки с известным покрытием:
//
//	A (Go Mono)       — без ✓ U+2713 и без арабского;
//	B (DejaVu Sans)   — ✓ и арабский есть;
//	C (DejaVu Mono)   — ✓ есть.
//
// Основной шрифт канваса (Go Regular) ✓ не покрывает.
type fbChain struct{ a, b, c string }

func makeFBChain(t *testing.T) fbChain {
	t.Helper()
	dir := t.TempDir()
	a := filepath.Join(dir, "A-gomono.ttf")
	if err := os.WriteFile(a, gomono.TTF, 0o644); err != nil {
		t.Fatal(err)
	}
	b := filepath.Join("..", "..", "assets", "fonts", "DejaVuSans.ttf")
	c := filepath.Join("..", "..", "assets", "fonts", "DejaVuSansMono.ttf")
	for _, p := range []string{b, c} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("нет шрифта %s: %v", p, err)
		}
	}
	return fbChain{a, b, c}
}

func lazyCanvas(t *testing.T, paths ...string) *Canvas {
	t.Helper()
	c := NewCanvas(300, 60, hgtext.NewFontCache("assets"))
	for _, p := range paths {
		if !c.addFallbackFontFile(p) {
			t.Fatalf("addFallbackFontFile(%s) отказал", p)
		}
	}
	return c
}

func eagerCanvas(t *testing.T, paths ...string) *Canvas {
	t.Helper()
	c := NewCanvas(300, 60, hgtext.NewFontCache("assets"))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !c.AddFallbackFont(data) {
			t.Fatalf("AddFallbackFont(%s) отказал", p)
		}
	}
	return c
}

// Регистрация пути файла не читает; обычный текст цепочку не трогает.
func TestLazyFallback_NothingReadUntilNeeded(t *testing.T) {
	testreset.Globals(t)
	ch := makeFBChain(t)
	reads := hookFallbackReads(t)
	c := lazyCanvas(t, ch.a, ch.b, ch.c)
	if len(c.fallbacks) != 3 {
		t.Fatalf("в цепочке %d записей, ожидалось 3", len(c.fallbacks))
	}
	c.DrawText("Hello, world 123", 4, 4, color255)
	c.MeasureText("Привет, мир", 12)
	if n := reads.count(); n != 0 {
		t.Fatalf("прочитано %d файлов до первого экзотического глифа: %v", n, reads.list())
	}
	// Несуществующий файл и каталог в цепочку не попадают (как раньше при New).
	if c.addFallbackFontFile(filepath.Join(t.TempDir(), "нет.ttf")) {
		t.Error("несуществующий файл принят")
	}
	if c.addFallbackFontFile(t.TempDir()) {
		t.Error("каталог принят")
	}
	if len(c.fallbacks) != 3 {
		t.Errorf("цепочка изменилась: %d", len(c.fallbacks))
	}
}

// Первый экзотический глиф загружает шрифты по порядку цепочки ровно до
// первого, где он есть; шрифты после него не читаются; повтор не читает вовсе.
func TestLazyFallback_ExoticGlyphLoadsUpToFirstMatch(t *testing.T) {
	testreset.Globals(t)
	ch := makeFBChain(t)
	reads := hookFallbackReads(t)
	c := lazyCanvas(t, ch.a, ch.b, ch.c)

	fc, ok := c.fcForRune(c.fontCache, '✓')
	if !ok || fc == nil {
		t.Fatal("✓ не найден в цепочке")
	}
	got := reads.list()
	if len(got) != 2 || got[0] != ch.a || got[1] != ch.b {
		t.Fatalf("прочитано %v, ожидалось [A B]", got)
	}
	if fc != c.fallbacks[1].Loaded() {
		t.Error("глиф взят не из B — порядок цепочки нарушен")
	}
	// Тот же и другой глиф из уже загруженных — без чтений.
	c.fcForRune(c.fontCache, '✓')
	c.fcForRune(c.fontCache, '→')
	if n := reads.count(); n != 2 {
		t.Fatalf("повторные поиски прочитали ещё файлы: %v", reads.list())
	}
	// Руны нет нигде — перебирается вся цепочка, но каждый файл читается раз.
	if _, ok := c.fcForRune(c.fontCache, 0x10FFFD); ok {
		t.Error("руна вне всех шрифтов найдена")
	}
	c.fcForRune(c.fontCache, 0x10FFFD)
	if reads.count() != 3 {
		t.Fatalf("после перебора всей цепочки прочитано %v", reads.list())
	}
}

// Ленивая цепочка рисует и измеряет ровно как нагруженная сразу: пиксели
// простого и шейпированного текста совпадают байт в байт, пути выбора шрифта
// тоже.
func TestLazyFallback_SameFramesAsEager(t *testing.T) {
	testreset.Globals(t)
	ch := makeFBChain(t)
	texts := []string{
		"Тест ✓ ⚠ → ≈ ░▒▓ done",
		"لا مرحبا بالعالم",
		"שלום עולם abc",
		"mixed ✓ مرحبا ✗ end",
	}
	for _, order := range [][]string{{ch.a, ch.b, ch.c}, {ch.c, ch.b, ch.a}, {ch.b}} {
		lazy := lazyCanvas(t, order...)
		eager := eagerCanvas(t, order...)
		for i, s := range texts {
			lazy.DrawText(s, 4, 4+i*14, color255)
			eager.DrawText(s, 4, 4+i*14, color255)
			if lw, ew := lazy.MeasureText(s, 14), eager.MeasureText(s, 14); lw != ew {
				t.Errorf("%q: ширина %d (лениво) и %d (сразу)", s, lw, ew)
			}
			lp := lazy.MeasureRunePositions(s, 14)
			ep := eager.MeasureRunePositions(s, 14)
			if len(lp) != len(ep) {
				t.Fatalf("%q: позиций %d и %d", s, len(lp), len(ep))
			}
			for i := range lp {
				if lp[i] != ep[i] {
					t.Errorf("%q: позиция руны %d: %d и %d", s, i, lp[i], ep[i])
					break
				}
			}
		}
		if !bytes.Equal(lazy.back.Pix, eager.back.Pix) {
			t.Errorf("порядок %v: кадр текста с ленивой цепочкой расходится с нагруженной сразу (макс. %d)",
				filepath.Base(order[0]), maxPixDiff(lazy.back, eager.back))
		}
	}
}

// DPI и субпиксели, заданные до загрузки, применяются при загрузке; заданные
// после — рассылаются на загруженные.
func TestLazyFallback_DPIAndSubpixelApplyOnLoad(t *testing.T) {
	testreset.Globals(t)
	ch := makeFBChain(t)
	c := lazyCanvas(t, ch.a, ch.b)
	c.SetDPIAll(144)
	c.SetTextSubpixel(true)
	fc, ok := c.fcForRune(c.fontCache, '✓')
	if !ok {
		t.Fatal("✓ не найден")
	}
	if fc.DPI() != 144 || !fc.Subpixel() {
		t.Fatalf("загруженный шрифт: dpi=%v subpixel=%v, ожидалось 144/true", fc.DPI(), fc.Subpixel())
	}
	c.SetDPIAll(96)
	c.SetTextSubpixel(false)
	if fc.DPI() != 96 || fc.Subpixel() {
		t.Fatalf("после рассылки: dpi=%v subpixel=%v", fc.DPI(), fc.Subpixel())
	}
}

// Поток кадра и поток приложения (измеритель) обращаются к цепочке разом: каждый
// файл читается ровно один раз, гонок нет (прогоняется под -race). Рассылка
// настроек (SetDPIAll) идёт под замком кадра и с чтением не пересекается.
func TestLazyFallback_ConcurrentLoadReadsOnce(t *testing.T) {
	testreset.Globals(t)
	ch := makeFBChain(t)
	reads := hookFallbackReads(t)
	c := lazyCanvas(t, ch.a, ch.b, ch.c)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				c.fcForRune(c.fontCache, rune(0x2713+(i+g)%8))
				c.fcForRune(c.fontCache, 0x10FFFD)
				c.MeasureText("✓ لا →", 12)
			}
		}(g)
	}
	wg.Wait()
	seen := map[string]int{}
	for _, p := range reads.list() {
		seen[p]++
	}
	for p, n := range seen {
		if n != 1 {
			t.Errorf("файл %s прочитан %d раз", p, n)
		}
	}
	if len(seen) != 3 {
		t.Errorf("прочитано %d файлов, ожидалось 3 (руна вне шрифтов перебирает всю цепочку)", len(seen))
	}
}

var color255 = color.RGBA{R: 255, G: 255, B: 255, A: 255}

// fbReads записывает, какие файлы запасных шрифтов были прочитаны.
type fbReads struct {
	mu    sync.Mutex
	paths []string
}

func (r *fbReads) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...)
}

func (r *fbReads) count() int { return len(r.list()) }

// hookFallbackReads подменяет читалку запасных шрифтов счётчиком.
func hookFallbackReads(t *testing.T) *fbReads {
	t.Helper()
	r := &fbReads{}
	orig := testhook.ReadFallbackFont
	testhook.ReadFallbackFont = func(p string) ([]byte, error) {
		r.mu.Lock()
		r.paths = append(r.paths, p)
		r.mu.Unlock()
		return hgtext.ReadFontFile(p)
	}
	t.Cleanup(func() { testhook.ReadFallbackFont = orig })
	return r
}
