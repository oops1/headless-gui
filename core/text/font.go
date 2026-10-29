// font.go — кэш TrueType шрифтов.
//
// Использует golang.org/x/image/font/opentype для рендеринга TTF/OTF.
// Встроенный шрифт: Go Regular (поддерживает Cyrillic, Latin, Greek и другие наборы).
// Если файл assets/fonts/Go-Regular.ttf присутствует, используется он;
// иначе — встроенный бинарный TTF из пакета gofont/goregular.
//
// FontCache — «вид» на общий шрифт процесса (glyphstore.go): настройки
// одного холста (DPI, подпиксели, качество) и кэш первого уровня с прежним
// ключом; растры при промахе берутся из общего кэша и делятся всеми
// движками (ADR-0003).

package text

import (
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
//
// Это вид на общий шрифт процесса: разобранный шрифт и растры глифов лежат
// в общем кэше (glyphstore.go) и делятся всеми видами с тем же содержимым;
// у вида — настройки холста и кэш первого уровня.
type FontCache struct {
	mu    sync.RWMutex
	TTF   *opentype.Font // общий sfnt.Font: безопасен с отдельным sfnt.Buffer
	font  *fontData
	cache map[float64]font.Face // личные face для Face(); не для одновременного использования
	dpi   float64
	// gen растёт при каждой смене настроек: промах, посчитанный со старыми
	// параметрами, не попадает в L1 после сброса.
	gen uint64

	// glyphPresent кэширует наличие глифа для руны (cmap-проверка),
	// чтобы не дёргать sfnt на каждый символ при отрисовке/измерении.
	glyphPresent map[rune]bool
	buf          sfnt.Buffer // переиспользуемый буфер для GlyphIndex

	// Кэш первого уровня (L1) с ключом по (размер, руна, фаза): DPI,
	// подпиксели и качество — состояние вида, их смена сбрасывает L1. Общий
	// кэш (L2) ключует те же растры ещё и параметрами вида.
	glyphs map[glyphKey]CachedGlyph
	// metrics — кэш вертикальных метрик по размеру (Face.Metrics не бесплатен).
	metrics map[float64]VMetric
	// kern — кэш кернинга пар (размер + пара рун): Face.Kern дёргает sfnt на
	// каждую пару, а UI многократно рисует одни и те же строки.
	kern map[kernKey]fixed.Int26_6
	// adv — кэш шагов глифов по (размер, руна) для измерения без face.
	adv map[advKey]advEntry
	// maxAdv — наибольший шаг глифа по кеглю (оценка охвата строки без
	// измерения, MaxAdvance).
	maxAdv map[float64]fixed.Int26_6

	// ── Шейпинг (go-text/typesetting) ────────────────────────────────────────
	// shapeFace — личный typesetting-face вида над общим tsfont.Font; у него
	// свои кэши, поэтому он читается только под замком шейпера холста.
	// Создаётся лениво — только когда встречается текст, требующий шейпинга.
	shapeFace    *tsfont.Face
	shapeFaceErr bool // общий шрифт не разобрался typesetting'ом — не пытаться снова

	// subpixel — позиционирование глифов по долям пикселя (SetSubpixel).
	// По умолчанию выключено: шаг каждого глифа округляется до целого, как
	// было всегда, и вид существующих тем не меняется.
	subpixel bool
	// quality — слой качества текста (textquality.go): хинтинг, контраст, LCD.
	// По умолчанию нулевой — вид глифов прежний; рассылается всем шрифтам
	// канваса, как и subpixel. Ширин не меняет.
	quality TextQuality
	// famKey — ключ семейства, под которым шрифт записан в таблице семейств
	// (fontfamily.go); "" — шрифт в таблицу не входит.
	famKey string
}

// VMetric — вертикальные метрики шрифта одного размера (в пикселях).
type VMetric struct {
	Ascent  int // подъём базовой линии
	Descent int // спуск под базовую линию
	LineGap int // рекомендуемый зазор между строками (не отрицательный)
}

// glyphKey — ключ кэша глифов: размер шрифта в пунктах + руна + доля пикселя,
// на которую сдвинуто перо (см. FontCache.GlyphAt; 0 при целочисленном пере).
type glyphKey struct {
	size  float64
	r     rune
	phase uint8
}

// kernKey — ключ кэша кернинга: размер в пунктах + упорядоченная пара рун.
type kernKey struct {
	size float64
	a, b rune
}

// advKey — ключ кэша шагов: размер в пунктах + руна.
type advKey struct {
	size float64
	r    rune
}

// maxKernCacheEntries — предел кэша кернинг-пар (см. maxGlyphCacheEntries):
// при переполнении сбрасывается целиком.
const maxKernCacheEntries = 8192

// CachedGlyph — растеризованный глиф: маска покрытия и метрики размещения.
// Маска immutable после создания — читается без блокировки.
type CachedGlyph struct {
	Mask    *image.Alpha  // плотная альфа-маска (nil для глифов без пикселей, напр. пробел)
	LCD     *LCDMask      // покрытие по цветным каналам (TextQuality.LCD); тогда Mask == nil
	OffX    int           // смещение маски от целочисленной позиции пера
	OffY    int           // смещение маски от базовой линии
	Advance fixed.Int26_6 // продвижение пера
	OK      bool          // false — глифа нет в шрифте
}

// maxGlyphCacheEntries — предел кэша глифов на один FontCache; при переполнении
// кэш сбрасывается целиком (типичный UI использует сотни уникальных пар
// размер×руна, маска ~200–800 байт — предел удерживает память в единицах МБ).
const maxGlyphCacheEntries = 8192

// NewFontCache создаёт кэш основного шрифта, загружая его из assetsDir.
// Это ЕДИНСТВЕННОЕ место, где невалидный шрифт заменяется встроенным
// Go Regular: без основного шрифта движок работать не может, и лучше
// показать текст «не тем» шрифтом, чем не показать вовсе. Подмена логируется.
func NewFontCache(assetsDir string) *FontCache {
	data := loadFontData(assetsDir)
	if fc := NewFontCacheFromData(data, DefaultDPI); fc != nil {
		return fc
	}
	log.Printf("engine: основной шрифт из %q не распознан, используется встроенный Go Regular", assetsDir)
	return NewFontCacheFromData(goregular.TTF, DefaultDPI)
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

// NewFontCacheFromData создаёт FontCache из TTF/OTF/TTC-байт и заданного DPI.
// Возвращает nil, если данные невалидны — БЕЗ молчаливой подмены встроенным
// шрифтом (SEC-17): иначе битый файл из RegisterFont/AddFallbackFont
// регистрировался как «ещё один Go Regular», ошибка «невалидный шрифт» в
// RegisterFallbackFontFile была недостижима, а fallback-цепочка росла
// бесполезными дубликатами.
//
// Шрифт с тем же содержимым разбирается один раз на процесс: вид получает
// общий разобранный шрифт и общий кэш растров (glyphstore.go). Копия байт
// после регистрации не удерживается.
func NewFontCacheFromData(data []byte, dpi float64) *FontCache {
	f := store.intern(data)
	if f == nil {
		return nil
	}
	return &FontCache{
		TTF:   f.ttf,
		font:  f,
		cache: make(map[float64]font.Face),
		dpi:   dpi,
	}
}

// NewFontCacheLike разбирает шрифт data с настройками текста шрифта ref (DPI,
// субпиксели, качество): новый шрифт холста рисует так же, как имеющиеся.
// nil — данные не шрифт.
func NewFontCacheLike(data []byte, ref *FontCache) *FontCache {
	fc := NewFontCacheFromData(data, ref.DPI())
	if fc != nil {
		fc.subpixel = ref.Subpixel()
		fc.quality = ref.Quality()
	}
	return fc
}

// NewView — новый вид того же шрифта: личный L1 над тем же общим кэшем
// (ADR-0003) с теми же DPI, подпикселем и качеством на момент вызова.
// Воркеру тайлового растра (ADR-0005) нужен свой вид: замок одного вида
// между воркерами съедает параллелизм. Настройки вида дальше живут порознь —
// их смену у холста движок встречает пересозданием воркеров.
func (fc *FontCache) NewView() *FontCache {
	fc.mu.RLock()
	v := &FontCache{
		TTF:      fc.TTF,
		font:     fc.font,
		cache:    make(map[float64]font.Face),
		dpi:      fc.dpi,
		subpixel: fc.subpixel,
		quality:  fc.quality,
		famKey:   fc.famKey,
	}
	fc.mu.RUnlock()
	return v
}

// SameParams сообщает, совпадают ли настройки вида (DPI, подпиксель,
// качество) и шрифт с ref: воркер сверяет личный вид с видом холста движка.
func (fc *FontCache) SameParams(ref *FontCache) bool {
	if fc == nil || ref == nil {
		return fc == ref
	}
	fc.mu.RLock()
	dpi, sub, q, f := fc.dpi, fc.subpixel, fc.quality, fc.font
	fc.mu.RUnlock()
	ref.mu.RLock()
	defer ref.mu.RUnlock()
	return f == ref.font && dpi == ref.dpi && sub == ref.subpixel && q == ref.quality
}

// PixelSize переводит кегль sizePt (pt) в пиксели 26.6 при DPI шрифта.
func (fc *FontCache) PixelSize(sizePt float64) fixed.Int26_6 {
	return fixed.Int26_6(sizePt * fc.dpi / 72.0 * 64.0)
}

// paramsLocked — снимок настроек вида и его поколение (нужны только на
// промахе L1). Под fc.mu (любой режим).
func (fc *FontCache) paramsLocked() (faceKey, TextQuality, uint64) {
	return faceKey{dpi: fc.dpi, subpixel: fc.subpixel}, fc.quality, fc.gen
}

// shaperFace возвращает typesetting-face для шейпинга (лениво, кэшируется).
// nil — шрифт не удалось распарсить; вызывающий код откатывается на простой
// (per-rune) путь отрисовки. Face личный у вида: его кэши не потокобезопасны,
// поэтому все обращения к нему идут под замком шейпера холста.
func (fc *FontCache) shaperFace() *tsfont.Face {
	fc.mu.RLock()
	f, failed := fc.shapeFace, fc.shapeFaceErr
	fc.mu.RUnlock()
	if f != nil || failed {
		return f
	}

	tsf := fc.font.typesetting()
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.shapeFace != nil || fc.shapeFaceErr {
		return fc.shapeFace
	}
	if tsf == nil {
		fc.shapeFaceErr = true
		return nil
	}
	fc.shapeFace = tsfont.NewFace(tsf)
	return fc.shapeFace
}

// Face возвращает font.Face для заданного размера (в пунктах).
// Кэшируется. Потокобезопасно само по себе, но полученный face — нет (как у
// x/image): он личный у этого вида, внутренние пути движка им не пользуются.
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
	// HintingFull в x/image не правит контуры (хинтинга там нет) — он только
	// округляет шаг каждого глифа и кернинг до целого пикселя. На мелком
	// кегле это и делает строку неровной: у Open Sans 8,5 pt (11,33 px)
	// «ширина» буквы 6,3 px становится то 6, то 7. Для подпиксельного режима
	// шаг оставляем дробным.
	hint := font.HintingFull
	if fc.subpixel {
		hint = font.HintingNone
	}
	face, err := opentype.NewFace(fc.TTF, &opentype.FaceOptions{
		Size:    sizePt,
		DPI:     fc.dpi,
		Hinting: hint,
	})
	if err != nil {
		return basicfont.Face7x13
	}
	fc.cache[sizePt] = face
	return face
}

// Measure возвращает ширину строки text в пикселях для шрифта размером sizePt.
func (fc *FontCache) Measure(text string, sizePt float64) int {
	var w fixed.Int26_6
	prev := rune(-1)
	for _, r := range text {
		if prev >= 0 {
			w += fc.Kern(sizePt, prev, r)
		}
		a, ok := fc.Advance(sizePt, r)
		if !ok {
			a, _ = fc.Advance(sizePt, '?')
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
	runes := []rune(text)
	pos := make([]int, len(runes)+1)
	var w fixed.Int26_6
	for i, r := range runes {
		if i > 0 {
			w += fc.Kern(sizePt, runes[i-1], r)
		}
		a, ok := fc.Advance(sizePt, r)
		if !ok {
			a, _ = fc.Advance(sizePt, '?')
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
	fc.gen++
	fc.cache = make(map[float64]font.Face) // очищаем кэш
	fc.glyphs = nil                        // маски зависят от DPI
	fc.metrics = nil
	fc.kern = nil // кернинг зависит от DPI/размера
	fc.adv = nil
	fc.maxAdv = nil
}

// SetSubpixel включает и выключает дробное позиционирование глифов. Меняет
// шаг и кернинг, поэтому сбрасывает все кэши, зависящие от них. Вызывать до
// отрисовки (через Engine.SetTextSubpixel, под блокировкой кадра).
func (fc *FontCache) SetSubpixel(on bool) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.subpixel == on {
		return
	}
	fc.subpixel = on
	fc.gen++
	fc.cache = make(map[float64]font.Face)
	fc.glyphs = nil
	fc.metrics = nil
	fc.kern = nil
	fc.adv = nil
	fc.maxAdv = nil
}

// SetQuality задаёт слой качества текста (TextQuality). Меняет только маски
// глифов — шаг, кернинг и метрики те же, — поэтому сбрасывается лишь кэш масок.
// Вызывать до отрисовки (под блокировкой кадра).
func (fc *FontCache) SetQuality(q TextQuality) {
	q = q.normalized()
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.quality == q {
		return
	}
	fc.quality = q
	fc.gen++
	fc.glyphs = nil
}

// Quality возвращает действующий слой качества текста.
func (fc *FontCache) Quality() TextQuality {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.quality
}

// Subpixel сообщает, включено ли дробное позиционирование глифов.
func (fc *FontCache) Subpixel() bool {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.subpixel
}

// Ascent возвращает подъём базовой линии (в пикселях) для размера sizePt.
// Кэшируется — Face.Metrics пересчитывает метрики при каждом вызове.
func (fc *FontCache) Ascent(sizePt float64) int {
	a, _ := fc.VMetrics(sizePt)
	return a
}

// VMetrics возвращает (ascent, descent) в пикселях для размера sizePt (кэшируется).
func (fc *FontCache) VMetrics(sizePt float64) (ascent, descent int) {
	m := fc.VMetricsFull(sizePt)
	return m.Ascent, m.Descent
}

// VMetricsFull возвращает все вертикальные метрики размера sizePt (кэшируется).
//
// Подъём и спуск округляются ровно так же, как это делает отрисовка
// (drawTextWithFont берёт их из VMetrics): отдельный пересчёт «для приложений»
// рисковал разойтись с ней на пиксель, и базовая линия, о которой сообщили
// виджету, оказывалась не там, где реально встал текст.
func (fc *FontCache) VMetricsFull(sizePt float64) VMetric {
	fc.mu.RLock()
	if m, ok := fc.metrics[sizePt]; ok {
		fc.mu.RUnlock()
		return m
	}
	k, _, gen := fc.paramsLocked()
	fc.mu.RUnlock()

	m := fc.font.shard(sizePt).vmetrics(k)

	fc.mu.Lock()
	if fc.gen == gen {
		if fc.metrics == nil {
			fc.metrics = make(map[float64]VMetric)
		}
		fc.metrics[sizePt] = m
	}
	fc.mu.Unlock()
	return m
}

// Glyph возвращает растеризованный глиф руны r для размера sizePt.
// Первое обращение растеризует контур через opentype и сохраняет плотную
// копию альфа-маски; последующие — только чтение кэша.
func (fc *FontCache) Glyph(sizePt float64, r rune) CachedGlyph {
	return fc.GlyphAt(sizePt, r, 0)
}

// SubpixelPhases — на сколько долей делится пиксель по горизонтали при
// дробном позиционировании. Четверти достаточно: глаз на 11-пиксельной строке
// различает сдвиг в 1/4 px едва-едва, а кэш глифов растёт ровно вчетверо.
const SubpixelPhases = 4

// GlyphAt — Glyph для пера, сдвинутого на phase/SubpixelPhases пикселя вправо
// от целочисленной позиции. Маска растеризуется с этим сдвигом, поэтому её
// offX отсчитывается от того же целого пикселя, что и у phase == 0.
//
// Попадание в L1 — под RLock вида, как и раньше; промах идёт в общий кэш
// процесса, где маску, возможно, уже растеризовал другой движок.
func (fc *FontCache) GlyphAt(sizePt float64, r rune, phase int) CachedGlyph {
	if phase < 0 || phase >= SubpixelPhases {
		phase = 0
	}
	k := glyphKey{size: sizePt, r: r, phase: uint8(phase)}
	fc.mu.RLock()
	if g, ok := fc.glyphs[k]; ok {
		fc.mu.RUnlock()
		return g
	}
	fk, q, gen := fc.paramsLocked()
	fc.mu.RUnlock()

	g := fc.font.shard(sizePt).glyphAt(fk, q, r, phase)

	fc.mu.Lock()
	if fc.gen == gen {
		if fc.glyphs == nil {
			fc.glyphs = make(map[glyphKey]CachedGlyph)
		} else if len(fc.glyphs) >= maxGlyphCacheEntries {
			fc.glyphs = make(map[glyphKey]CachedGlyph, maxGlyphCacheEntries/4)
		}
		fc.glyphs[k] = g
	}
	fc.mu.Unlock()
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
	fk, _, gen := fc.paramsLocked()
	fc.mu.RUnlock()

	v := fc.font.shard(sizePt).kerning(fk, a, b)

	fc.mu.Lock()
	if fc.gen == gen {
		if fc.kern == nil {
			fc.kern = make(map[kernKey]fixed.Int26_6)
		} else if len(fc.kern) >= maxKernCacheEntries {
			fc.kern = make(map[kernKey]fixed.Int26_6, maxKernCacheEntries/4)
		}
		fc.kern[k] = v
	}
	fc.mu.Unlock()
	return v
}

// Advance возвращает шаг глифа руны r для размера sizePt и признак, что глиф
// есть (как у font.Face.GlyphAdvance). Кэшируется; измерение идёт через него,
// а не через face, поэтому меряющая горутина не делит face с кадром.
func (fc *FontCache) Advance(sizePt float64, r rune) (fixed.Int26_6, bool) {
	k := advKey{size: sizePt, r: r}
	fc.mu.RLock()
	if e, ok := fc.adv[k]; ok {
		fc.mu.RUnlock()
		return e.adv, e.ok
	}
	fk, _, gen := fc.paramsLocked()
	fc.mu.RUnlock()

	var e advEntry
	e.adv, e.ok = fc.font.shard(sizePt).advance(fk, r)

	fc.mu.Lock()
	if fc.gen == gen {
		if fc.adv == nil {
			fc.adv = make(map[advKey]advEntry)
		} else if len(fc.adv) >= maxKernCacheEntries {
			fc.adv = make(map[advKey]advEntry, maxKernCacheEntries/4)
		}
		fc.adv[k] = e
	}
	fc.mu.Unlock()
	return e.adv, e.ok
}

// MaxAdvance возвращает наибольший шаг глифа шрифта для размера sizePt (в
// физических пикселях 26.6): верхняя оценка ширины строки — число рун × шаг
// — без её измерения. Кэшируется; первый вызов на (шрифт, кегль, DPI)
// обходит все глифы шрифта.
func (fc *FontCache) MaxAdvance(sizePt float64) fixed.Int26_6 {
	fc.mu.RLock()
	if v, ok := fc.maxAdv[sizePt]; ok {
		fc.mu.RUnlock()
		return v
	}
	fk, _, gen := fc.paramsLocked()
	fc.mu.RUnlock()

	v := fc.font.shard(sizePt).maxAdvance(fk)

	fc.mu.Lock()
	if fc.gen == gen {
		if fc.maxAdv == nil {
			fc.maxAdv = make(map[float64]fixed.Int26_6)
		}
		fc.maxAdv[sizePt] = v
	}
	fc.mu.Unlock()
	return v
}

// HasGlyph сообщает, есть ли в шрифте глиф для руны r (cmap-проверка).
// Результат кэшируется. Потокобезопасно. Для пробела всегда true.
func (fc *FontCache) HasGlyph(r rune) bool {
	if fc == nil || fc.TTF == nil {
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
	gi, err := fc.TTF.GlyphIndex(&fc.buf, r)
	present := err == nil && gi != 0
	fc.glyphPresent[r] = present
	return present
}

// AssetFallbackFontPaths возвращает пути к нашим собственным шрифтам широкого
// покрытия. Они идут ПОСЛЕ системных: на машине со шрифтами системная копия
// ближе к тому, что видят остальные программы, и должна выигрывать. Смысл этих
// путей — машина, на которой системных шрифтов нет вовсе: без них там нечем
// нарисовать ни псевдографику, ни ✓ ✗ ⚠, и текст молча теряет символы
// (fcForRune не рисует .notdef). Каталог ищется относительно рабочего каталога
// процесса — как и вся регистрация из assets/fonts.
func AssetFallbackFontPaths() []string {
	return []string{
		filepath.Join("assets", "fonts", "DejaVuSans.ttf"),
		filepath.Join("assets", "fonts", "DejaVuSansMono.ttf"),
	}
}

// SystemFallbackFontPaths возвращает список путей к системным шрифтам с широким
// покрытием символов (✓ ✗ ⚠, box-drawing, стрелки и т.п.), которые движок
// пытается подгрузить как fallback к встроенному Go Regular. Best-effort:
// отсутствующие файлы молча пропускаются (BUG-2).
func SystemFallbackFontPaths() []string {
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
// каждую оторванную док-панель (см. window/core/modal_host.go, dock_host.go). В
// профиле это десятки мегабайт os.ReadFile и лишние миллисекунды на КАЖДОЕ
// открытие диалога. Шрифты в рантайме не меняются, поэтому содержимое файлов
// кэшируется на процесс. Данные только читаются (sfnt/typesetting держат их
// как read-only), так что общий срез безопасен. Один и тот же срез у всех
// движков — ещё и быстрый путь интернирования шрифта (glyphstore.go).

var (
	fontFileMu    sync.Mutex
	fontFileCache = map[string][]byte{} // путь → содержимое (nil — файла нет)
)

// ReadFontFile — os.ReadFile с процессным кэшем. Ошибка тоже кэшируется:
// отсутствующий файл не перепроверяется на каждом создании движка.
func ReadFontFile(path string) ([]byte, error) {
	fontFileMu.Lock()
	data, seen := fontFileCache[path]
	fontFileMu.Unlock()
	if seen {
		if data == nil {
			return nil, os.ErrNotExist
		}
		return data, nil
	}
	data, err := readFileBounded(path, MaxFontFileSize)
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

// MaxFontFileSize — верхняя граница размера файла шрифта, который движок
// согласен прочитать в память (SEC-17): пути fallback-шрифтов зависят от
// окружения (SystemRoot и т.п.), и «шрифт» на десятки гигабайт не должен
// превращаться в OOM при старте. Самые крупные реальные шрифты (Noto CJK,
// Arial Unicode, TTC-коллекции) — 20–40 МБ.
const MaxFontFileSize = 128 << 20

// LoadFontFile читает файл шрифта без процессного кэша (явная регистрация
// приложением), не больше MaxFontFileSize.
func LoadFontFile(path string) ([]byte, error) { return readFileBounded(path, MaxFontFileSize) }

// readFileBounded читает файл целиком, но не более limit байт; файл больше
// лимита — ошибка, а не усечённые данные (усечённый шрифт бесполезен).
func readFileBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil {
		if st.Size() > limit {
			return nil, fmt.Errorf("%s: файл шрифта слишком большой (%d байт > %d)", path, st.Size(), limit)
		}
		// Размер известен: один буфер точного размера. io.ReadAll растёт
		// вдвое и на 22 МБ шрифтов давал 53 МБ выделений.
		if n := st.Size(); n > 0 {
			data := make([]byte, n)
			if _, err := io.ReadFull(f, data); err != nil {
				return nil, err
			}
			return data, nil
		}
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
		if data, err := ReadFontFile(p); err == nil {
			return data
		}
	}
	return goregular.TTF
}
