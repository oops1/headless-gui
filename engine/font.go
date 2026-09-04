// Package engine — кэш TrueType шрифтов.
//
// Использует golang.org/x/image/font/opentype для рендеринга TTF/OTF.
// Встроенный шрифт: Go Regular (поддерживает Cyrillic, Latin, Greek и другие наборы).
// Если файл assets/fonts/Go-Regular.ttf присутствует, используется он;
// иначе — встроенный бинарный TTF из пакета gofont/goregular.
package engine

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	tsfont "github.com/go-text/typesetting/font"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// DefaultDPI — DPI по умолчанию для рендеринга шрифтов.
const DefaultDPI = 96.0

// DefaultFontSize — размер шрифта по умолчанию (в пунктах) для DrawText.
// При DefaultDPI (96) соответствует ~13 px высоты — как basicfont.Face7x13.
const DefaultFontSize = 10.0

// FontCache кэширует font.Face для разных размеров одного TTF-файла.
// Потокобезопасен.
type FontCache struct {
	mu    sync.RWMutex
	ttf   *opentype.Font
	cache map[float64]font.Face
	dpi   float64

	// glyphPresent кэширует наличие глифа для руны (cmap-проверка),
	// чтобы не дёргать sfnt на каждый символ при отрисовке/измерении.
	glyphPresent map[rune]bool
	buf          sfnt.Buffer // переиспользуемый буфер для GlyphIndex

	// glyphs — кэш растеризованных глифов по (размер, руна): opentype.Face
	// растеризует векторный контур при каждом обращении, поэтому маска
	// сохраняется один раз и дальше блиттируется из кэша.
	glyphs map[glyphKey]cachedGlyph
	// metrics — кэш вертикальных метрик по размеру (Face.Metrics не бесплатен).
	metrics map[float64]vMetric
	// kern — кэш кернинга пар (размер + пара рун): Face.Kern дёргает sfnt на
	// каждую пару, а UI многократно рисует одни и те же строки.
	kern map[kernKey]fixed.Int26_6

	// ── Шейпинг (go-text/typesetting) ────────────────────────────────────────
	// ttfData — исходные байты шрифта: typesetting парсит их отдельно от
	// x/image/opentype. shapeFace создаётся лениво — только когда встречается
	// текст, требующий шейпинга (RTL, арабский, индийские скрипты...).
	ttfData      []byte
	shapeFace    *tsfont.Face
	shapeFaceErr bool // парсинг не удался — не пытаться снова
}

// vMetric — вертикальные метрики шрифта одного размера (в пикселях).
type vMetric struct {
	ascent  int // подъём базовой линии
	descent int // спуск под базовую линию
}

// glyphKey — ключ кэша глифов: размер шрифта в пунктах + руна.
type glyphKey struct {
	size float64
	r    rune
}

// kernKey — ключ кэша кернинга: размер в пунктах + упорядоченная пара рун.
type kernKey struct {
	size float64
	a, b rune
}

// maxKernCacheEntries — предел кэша кернинг-пар (см. maxGlyphCacheEntries):
// при переполнении сбрасывается целиком.
const maxKernCacheEntries = 8192

// cachedGlyph — растеризованный глиф: маска покрытия и метрики размещения.
// Маска immutable после создания — читается без блокировки.
type cachedGlyph struct {
	mask    *image.Alpha  // плотная альфа-маска (nil для глифов без пикселей, напр. пробел)
	offX    int           // смещение маски от целочисленной позиции пера
	offY    int           // смещение маски от базовой линии
	advance fixed.Int26_6 // продвижение пера
	ok      bool          // false — глифа нет в шрифте
}

// maxGlyphCacheEntries — предел кэша глифов на один FontCache; при переполнении
// кэш сбрасывается целиком (типичный UI использует сотни уникальных пар
// размер×руна, маска ~200–800 байт — предел удерживает память в единицах МБ).
const maxGlyphCacheEntries = 8192

// newFontCache создаёт кэш основного шрифта, загружая его из assetsDir.
// Это ЕДИНСТВЕННОЕ место, где невалидный шрифт заменяется встроенным
// Go Regular: без основного шрифта движок работать не может, и лучше
// показать текст «не тем» шрифтом, чем не показать вовсе. Подмена логируется.
func newFontCache(assetsDir string) *FontCache {
	data := loadFontData(assetsDir)
	if fc := newFontCacheFromData(data, DefaultDPI); fc != nil {
		return fc
	}
	log.Printf("engine: основной шрифт из %q не распознан, используется встроенный Go Regular", assetsDir)
	return newFontCacheFromData(goregular.TTF, DefaultDPI)
}

// parseFontData разбирает TTF/OTF/TTC-байты; для TTC-коллекций берётся
// первый шрифт.
func parseFontData(data []byte) (*opentype.Font, error) {
	parsed, err := opentype.Parse(data)
	if err == nil {
		return parsed, nil
	}
	// TTC-коллекция (Nirmala.ttc и т.п.) — первый шрифт.
	if coll, cerr := opentype.ParseCollection(data); cerr == nil {
		if f, ferr := coll.Font(0); ferr == nil {
			return f, nil
		}
	}
	return nil, err
}

// newFontCacheFromData создаёт FontCache из TTF/OTF/TTC-байт и заданного DPI.
// Возвращает nil, если данные невалидны — БЕЗ молчаливой подмены встроенным
// шрифтом (SEC-17): иначе битый файл из RegisterFont/AddFallbackFont
// регистрировался как «ещё один Go Regular», ошибка «невалидный шрифт» в
// RegisterFallbackFontFile была недостижима, а fallback-цепочка росла
// бесполезными дубликатами.
func newFontCacheFromData(data []byte, dpi float64) *FontCache {
	parsed, err := parseFontData(data)
	if err != nil {
		return nil
	}
	return &FontCache{
		ttf:     parsed,
		cache:   make(map[float64]font.Face),
		dpi:     dpi,
		ttfData: data,
	}
}

// shaperFace возвращает typesetting-face для шейпинга (лениво, кэшируется).
// nil — шрифт не удалось распарсить; вызывающий код откатывается на простой
// (per-rune) путь отрисовки.
func (fc *FontCache) shaperFace() *tsfont.Face {
	fc.mu.RLock()
	f, failed := fc.shapeFace, fc.shapeFaceErr
	fc.mu.RUnlock()
	if f != nil || failed {
		return f
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.shapeFace != nil || fc.shapeFaceErr {
		return fc.shapeFace
	}
	face, err := tsfont.ParseTTF(bytes.NewReader(fc.ttfData))
	if err != nil {
		// TTC-коллекция — первый шрифт (симметрично newFontCacheFromData).
		if faces, cerr := tsfont.ParseTTC(bytes.NewReader(fc.ttfData)); cerr == nil && len(faces) > 0 {
			face, err = faces[0], nil
		}
	}
	if err != nil {
		fc.shapeFaceErr = true
		return nil
	}
	fc.shapeFace = face
	return face
}

// Face возвращает font.Face для заданного размера (в пунктах).
// Кэшируется. Потокобезопасно.
func (fc *FontCache) Face(sizePt float64) font.Face {
	fc.mu.RLock()
	if f, ok := fc.cache[sizePt]; ok {
		fc.mu.RUnlock()
		return f
	}
	fc.mu.RUnlock()

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if f, ok := fc.cache[sizePt]; ok {
		return f
	}
	face, err := opentype.NewFace(fc.ttf, &opentype.FaceOptions{
		Size:    sizePt,
		DPI:     fc.dpi,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return basicfont.Face7x13
	}
	fc.cache[sizePt] = face
	return face
}

// Measure возвращает ширину строки text в пикселях для шрифта размером sizePt.
func (fc *FontCache) Measure(text string, sizePt float64) int {
	face := fc.Face(sizePt)
	var w fixed.Int26_6
	prev := rune(-1)
	for _, r := range text {
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
	return w.Round()
}

// MeasureRunes возвращает массив накопленных ширин: positions[0]==0,
// positions[i] — ширина первых i символов строки (в пикселях).
// Длина результата: len([]rune(text))+1.
func (fc *FontCache) MeasureRunes(text string, sizePt float64) []int {
	face := fc.Face(sizePt)
	runes := []rune(text)
	pos := make([]int, len(runes)+1)
	var w fixed.Int26_6
	for i, r := range runes {
		if i > 0 {
			w += face.Kern(runes[i-1], r)
		}
		a, ok := face.GlyphAdvance(r)
		if !ok {
			a, _ = face.GlyphAdvance('?')
		}
		w += a
		pos[i+1] = w.Round()
	}
	return pos
}

// SetDPI обновляет DPI и сбрасывает кэш face (чтобы шрифты перерендерились).
func (fc *FontCache) SetDPI(dpi float64) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.dpi = dpi
	fc.cache = make(map[float64]font.Face) // очищаем кэш
	fc.glyphs = nil                        // маски зависят от DPI
	fc.metrics = nil
	fc.kern = nil // кернинг зависит от DPI/размера
}

// Ascent возвращает подъём базовой линии (в пикселях) для размера sizePt.
// Кэшируется — Face.Metrics пересчитывает метрики при каждом вызове.
func (fc *FontCache) Ascent(sizePt float64) int {
	a, _ := fc.vMetrics(sizePt)
	return a
}

// vMetrics возвращает (ascent, descent) в пикселях для размера sizePt (кэшируется).
func (fc *FontCache) vMetrics(sizePt float64) (ascent, descent int) {
	fc.mu.RLock()
	if m, ok := fc.metrics[sizePt]; ok {
		fc.mu.RUnlock()
		return m.ascent, m.descent
	}
	fc.mu.RUnlock()

	face := fc.Face(sizePt)
	met := face.Metrics()
	ascent = met.Ascent.Round()
	descent = met.Descent.Round()

	fc.mu.Lock()
	if fc.metrics == nil {
		fc.metrics = make(map[float64]vMetric)
	}
	fc.metrics[sizePt] = vMetric{ascent: ascent, descent: descent}
	fc.mu.Unlock()
	return ascent, descent
}

// Glyph возвращает растеризованный глиф руны r для размера sizePt.
// Первое обращение растеризует контур через opentype и сохраняет плотную
// копию альфа-маски; последующие — только чтение кэша.
func (fc *FontCache) Glyph(sizePt float64, r rune) cachedGlyph {
	k := glyphKey{size: sizePt, r: r}
	fc.mu.RLock()
	if g, ok := fc.glyphs[k]; ok {
		fc.mu.RUnlock()
		return g
	}
	fc.mu.RUnlock()

	face := fc.Face(sizePt)

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if g, ok := fc.glyphs[k]; ok {
		return g
	}
	if fc.glyphs == nil {
		fc.glyphs = make(map[glyphKey]cachedGlyph)
	} else if len(fc.glyphs) >= maxGlyphCacheEntries {
		fc.glyphs = make(map[glyphKey]cachedGlyph, maxGlyphCacheEntries/4)
	}

	// Растеризация в целочисленной позиции пера: при HintingFull advance
	// квантуется до целых пикселей, поэтому дробная часть пера всегда 0 и
	// маска, снятая в (0,0), корректна для любой целочисленной позиции.
	dr, maskImg, maskPt, adv, ok := face.Glyph(fixed.Point26_6{}, r)
	g := cachedGlyph{advance: adv, ok: ok}
	if ok && !dr.Empty() {
		g.offX, g.offY = dr.Min.X, dr.Min.Y
		w, h := dr.Dx(), dr.Dy()
		m := image.NewAlpha(image.Rect(0, 0, w, h))
		if src, isAlpha := maskImg.(*image.Alpha); isAlpha {
			for y := 0; y < h; y++ {
				so := src.PixOffset(maskPt.X, maskPt.Y+y)
				copy(m.Pix[y*m.Stride:y*m.Stride+w], src.Pix[so:so+w])
			}
		} else {
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					_, _, _, a := maskImg.At(maskPt.X+x, maskPt.Y+y).RGBA()
					m.Pix[y*m.Stride+x] = uint8(a >> 8)
				}
			}
		}
		g.mask = m
	}
	fc.glyphs[k] = g
	return g
}

// Kern возвращает кернинг пары рун для размера sizePt (кэшируется).
// Face.Kern обращается к sfnt на каждую пару; типичный UI рисует одни и те же
// строки многократно, поэтому пары кэшируются. Потокобезопасно.
func (fc *FontCache) Kern(sizePt float64, a, b rune) fixed.Int26_6 {
	k := kernKey{size: sizePt, a: a, b: b}
	fc.mu.RLock()
	if v, ok := fc.kern[k]; ok {
		fc.mu.RUnlock()
		return v
	}
	fc.mu.RUnlock()

	v := fc.Face(sizePt).Kern(a, b)

	fc.mu.Lock()
	if fc.kern == nil {
		fc.kern = make(map[kernKey]fixed.Int26_6)
	} else if len(fc.kern) >= maxKernCacheEntries {
		fc.kern = make(map[kernKey]fixed.Int26_6, maxKernCacheEntries/4)
	}
	fc.kern[k] = v
	fc.mu.Unlock()
	return v
}

// HasGlyph сообщает, есть ли в шрифте глиф для руны r (cmap-проверка).
// Результат кэшируется. Потокобезопасно. Для пробела всегда true.
func (fc *FontCache) HasGlyph(r rune) bool {
	if fc == nil || fc.ttf == nil {
		return false
	}
	if r == ' ' {
		return true
	}
	// PERF-14: попадание в кэш — под RLock (HasGlyph зовётся на КАЖДУЮ руну
	// при активной fallback-цепочке). Write-lock берём только на промах.
	fc.mu.RLock()
	v, ok := fc.glyphPresent[r]
	fc.mu.RUnlock()
	if ok {
		return v
	}

	// Промах: считаем cmap ВНЕ мьютекса нельзя — fc.buf разделяемый (sfnt.Buffer
	// не потокобезопасен), поэтому расчёт идёт под write-lock, как и раньше.
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.glyphPresent == nil {
		fc.glyphPresent = make(map[rune]bool)
	} else if v, ok := fc.glyphPresent[r]; ok {
		return v // кто-то посчитал, пока мы ждали lock
	}
	gi, err := fc.ttf.GlyphIndex(&fc.buf, r)
	present := err == nil && gi != 0
	fc.glyphPresent[r] = present
	return present
}

// assetFallbackFontPaths возвращает пути к нашим собственным шрифтам широкого
// покрытия. Они идут ПОСЛЕ системных: на машине со шрифтами системная копия
// ближе к тому, что видят остальные программы, и должна выигрывать. Смысл этих
// путей — машина, на которой системных шрифтов нет вовсе: без них там нечем
// нарисовать ни псевдографику, ни ✓ ✗ ⚠, и текст молча теряет символы
// (fcForRune не рисует .notdef). Каталог ищется относительно рабочего каталога
// процесса — как и вся регистрация из assets/fonts.
func assetFallbackFontPaths() []string {
	return []string{
		filepath.Join("assets", "fonts", "DejaVuSans.ttf"),
		filepath.Join("assets", "fonts", "DejaVuSansMono.ttf"),
	}
}

// systemFallbackFontPaths возвращает список путей к системным шрифтам с широким
// покрытием символов (✓ ✗ ⚠, box-drawing, стрелки и т.п.), которые движок
// пытается подгрузить как fallback к встроенному Go Regular. Best-effort:
// отсутствующие файлы молча пропускаются (BUG-2).
func systemFallbackFontPaths() []string {
	switch runtime.GOOS {
	case "windows":
		// SystemRoot — стандартная системная переменная; принимаем только
		// абсолютный путь к существующему каталогу, иначе — C:\Windows (SEC-17).
		root := os.Getenv("SystemRoot")
		if root == "" || !filepath.IsAbs(root) {
			root = `C:\Windows`
		} else if st, err := os.Stat(root); err != nil || !st.IsDir() {
			root = `C:\Windows`
		}
		fonts := filepath.Join(root, "Fonts")
		return []string{
			filepath.Join(fonts, "seguisym.ttf"), // Segoe UI Symbol: ✓✗⚠, стрелки, box-drawing
			filepath.Join(fonts, "l_10646.ttf"),  // Lucida Sans Unicode: ✓✗ и пр.
			filepath.Join(fonts, "arialuni.ttf"), // Arial Unicode MS (если установлен)
			filepath.Join(fonts, "DejaVuSans.ttf"),
			filepath.Join(fonts, "arial.ttf"),    // латиница/кириллица/арабский/иврит
			filepath.Join(fonts, "Nirmala.ttc"),  // Nirmala UI: индийские скрипты (деванагари и др.)
			filepath.Join(fonts, "Nirmala.ttf"),  // вариант поставки одиночным TTF
			filepath.Join(fonts, "LeelawUI.ttf"), // Leelawadee UI: тайский, лаосский, кхмерский
			filepath.Join(fonts, "tahoma.ttf"),   // широкий запасной: тайский/иврит/арабский
			filepath.Join(fonts, "seguiemj.ttf"),
		}
	case "darwin":
		return []string{
			"/System/Library/Fonts/Apple Symbols.ttf",
			"/Library/Fonts/Arial Unicode.ttf",
			"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
		}
	default: // linux и прочие unix
		return []string{
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/truetype/noto/NotoSansSymbols-Regular.ttf",
			"/usr/share/fonts/TTF/DejaVuSans.ttf",
			"/usr/share/fonts/dejavu/DejaVuSans.ttf",
			// Скрипты, требующие шейпинга (см. shaper.go) — best-effort.
			"/usr/share/fonts/truetype/noto/NotoSansArabic-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSansHebrew-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSansDevanagari-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSansThai-Regular.ttf",
		}
	}
}

// ─── Процессный кэш файлов шрифтов ──────────────────────────────────────────
//
// Каждый engine.New перечитывал шрифты с диска — а движков в процессе бывает
// МНОГО: hosted-режим создаёт движок на каждый нативный модальный диалог и на
// каждую оторванную док-панель (см. window/modal_host.go, dock_host.go). В
// профиле это десятки мегабайт os.ReadFile и лишние миллисекунды на КАЖДОЕ
// открытие диалога. Шрифты в рантайме не меняются, поэтому содержимое файлов
// кэшируется на процесс. Данные только читаются (sfnt/typesetting держат их
// как read-only), так что общий срез безопасен.

var (
	fontFileMu    sync.Mutex
	fontFileCache = map[string][]byte{} // путь → содержимое (nil — файла нет)
)

// readFontFile — os.ReadFile с процессным кэшем. Ошибка тоже кэшируется:
// отсутствующий файл не перепроверяется на каждом создании движка.
func readFontFile(path string) ([]byte, error) {
	fontFileMu.Lock()
	data, seen := fontFileCache[path]
	fontFileMu.Unlock()
	if seen {
		if data == nil {
			return nil, os.ErrNotExist
		}
		return data, nil
	}
	data, err := readFileBounded(path, maxFontFileSize)
	if err != nil {
		data = nil
	}
	fontFileMu.Lock()
	fontFileCache[path] = data
	fontFileMu.Unlock()
	if data == nil {
		return nil, err
	}
	return data, nil
}

// maxFontFileSize — верхняя граница размера файла шрифта, который движок
// согласен прочитать в память (SEC-17): пути fallback-шрифтов зависят от
// окружения (SystemRoot и т.п.), и «шрифт» на десятки гигабайт не должен
// превращаться в OOM при старте. Самые крупные реальные шрифты (Noto CJK,
// Arial Unicode, TTC-коллекции) — 20–40 МБ.
const maxFontFileSize = 128 << 20

// readFileBounded читает файл целиком, но не более limit байт; файл больше
// лимита — ошибка, а не усечённые данные (усечённый шрифт бесполезен).
func readFileBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > limit {
		return nil, fmt.Errorf("%s: файл шрифта слишком большой (%d байт > %d)", path, st.Size(), limit)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s: файл шрифта слишком большой (> %d байт)", path, limit)
	}
	return data, nil
}

// loadFontData читает TTF из файла; если не удаётся — возвращает встроенный Go Regular.
func loadFontData(assetsDir string) []byte {
	candidates := []string{
		filepath.Join(assetsDir, "fonts", "Go-Regular.ttf"),
		"assets/fonts/Go-Regular.ttf",
	}
	for _, p := range candidates {
		if data, err := readFontFile(p); err == nil {
			return data
		}
	}
	return goregular.TTF
}
