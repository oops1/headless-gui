package text

// glyphstore.go — общий на процесс кэш растров глифов (ADR-0003, задача 4.2).
//
// Движков в процессе много (Tandem — по движку на монитор, WinLine —
// hosted-диалоги и задания печати), и каждый растеризовал бы одни и те же
// глифы заново. Здесь — второй уровень кэша (L2), общий для всех видов
// (FontCache) одного шрифта: шрифт интернируется по содержимому, растры
// лежат в шардах по (шрифт, кегль) с ключами, в которые входят DPI,
// подпиксельная фаза и слой качества, — поэтому два движка с разными
// настройками делят шард, но не маски. Первый уровень (L1) остаётся у вида с
// прежним ключом и прежним замком: горячий путь попадания не меняется, а
// сюда обращаются только на промахе L1.
//
// Небезопасные объекты (opentype.Face, sfnt.Buffer, vector.Rasterizer) живут
// в шарде и используются только под его write-замком; scratch tsfont.Face
// шрифта — под fontData.shapeMu. Замки на промахе берутся с defer: паника
// растеризатора на битом шрифте уходит в границу кадра движка (FailureIsolate),
// а шард, общий для всех движков, не остаётся захваченным. Опубликованные
// маски неизменяемы.
//
// Память ограничена бюджетом байт на процесс: при превышении самые давно не
// нужные шарды очищаются целиком (LRU по часам промахов). Учёт шарда — его
// единственный источник правды: хранилище получает дельту учёта шарда на
// каждом промахе, так что сумма учётов шардов индекса равна учёту хранилища
// на любом пути (вставка, зоны хинтинга, переполнение, очистка). Кэш ничего
// не знает о движках: Engine.Stop его не трогает, вид умирает как обычный
// объект.

import (
	"bytes"
	"hash/maphash"
	"image"
	"image/color"
	"sort"
	"sync"
	"sync/atomic"

	tsfont "github.com/go-text/typesetting/font"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// DefaultGlyphCacheBudget — бюджет общего кэша растров по умолчанию (байт).
// Типичный UI держит единицы мегабайт; предел нужен на случай многих движков
// с разными DPI и CJK-шрифтов.
const DefaultGlyphCacheBudget = 64 << 20

const (
	// maxInternedFonts — сколько различных по содержимому шрифтов держит
	// индекс; при переполнении выбывает тот, который дольше всех не искали.
	maxInternedFonts = 256
	// maxShardGlyphs — предел масок одного шарда: держит и шрифты, выбывшие
	// из индекса (их бюджет не вытесняет).
	maxShardGlyphs = 8192
	// maxShardSmall — предел записей шагов и кернинга шарда (та же роль:
	// шрифт вне индекса, путь измерения без отрисовки).
	maxShardSmall = 16384

	entryOverhead = 64  // накладные байты записи с маской
	smallEntry    = 32  // шаг, кернинг, метрики
	hintCost      = 256 // зоны автохинтера
)

// glyphStore — хранилище процесса: индекс шрифтов, бюджет и часы.
type glyphStore struct {
	mu      sync.Mutex
	seed    maphash.Seed
	byPtr   map[ptrKey]*fontData
	byHash  map[uint64][]*fontData
	fonts   []*fontData // индекс: шрифты, участвующие в вытеснении
	lookups atomic.Uint64

	budget atomic.Int64
	bytes  atomic.Int64
	clock  atomic.Uint64 // идёт на каждом промахе L2
}

// ptrKey — адрес и длина канонического среза шрифта (быстрый путь индекса).
type ptrKey struct {
	p *byte
	n int
}

var store = newGlyphStore()

func newGlyphStore() *glyphStore {
	s := &glyphStore{
		seed:   maphash.MakeSeed(),
		byPtr:  make(map[ptrKey]*fontData),
		byHash: make(map[uint64][]*fontData),
	}
	s.budget.Store(DefaultGlyphCacheBudget)
	return s
}

// SetGlyphCacheBudget задаёт бюджет общего кэша растров глифов процесса в
// байтах; n ≤ 0 возвращает DefaultGlyphCacheBudget. Превышение вытесняет
// самые давно не нужные записи.
func SetGlyphCacheBudget(n int64) {
	if n <= 0 {
		n = DefaultGlyphCacheBudget
	}
	store.budget.Store(n)
	if store.bytes.Load() > n {
		store.evict()
	}
}

// GlyphCacheStats — состояние общего кэша: шрифтов в индексе, шардов,
// записей-растров (маски, маски шейпера, цветные глифы) и учтённых байт.
type GlyphCacheStats struct {
	Fonts, Shards, Entries int
	Bytes                  int64
}

// GlyphCacheStatsNow возвращает статистику общего кэша (тесты, диагностика).
func GlyphCacheStatsNow() GlyphCacheStats {
	s := store
	s.mu.Lock()
	defer s.mu.Unlock()
	st := GlyphCacheStats{Fonts: len(s.fonts), Bytes: s.bytes.Load()}
	for _, f := range s.fonts {
		f.mu.RLock()
		for _, sh := range f.shards {
			sh.mu.RLock()
			st.Entries += len(sh.glyphs)
			sh.mu.RUnlock()
			st.Shards++
		}
		for _, ss := range f.shaped {
			ss.mu.Lock()
			st.Entries += len(ss.masks) + len(ss.colors) + len(ss.dyn)
			ss.mu.Unlock()
			st.Shards++
		}
		f.mu.RUnlock()
	}
	return st
}

// ResetGlyphCache очищает записи общего кэша (индекс шрифтов остаётся):
// тесты и бенчмарки, которым нужен промах.
func ResetGlyphCache() {
	s := store
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.fonts {
		s.bytes.Add(-f.clearShards())
	}
}

// intern возвращает общий шрифт для байт data, разбирая их один раз на
// процесс; nil — пусто или не шрифт. Поиск и вставка — под s.mu, хэш и разбор
// — вне его: конкурент, вставивший те же байты раньше, выигрывает.
func (s *glyphStore) intern(data []byte) *fontData {
	if len(data) == 0 {
		return nil
	}
	pk := ptrKey{&data[0], len(data)}
	s.mu.Lock()
	if f := s.byPtr[pk]; f != nil {
		s.touch(f)
		s.mu.Unlock()
		return f
	}
	s.mu.Unlock()

	h := maphash.Bytes(s.seed, data)
	s.mu.Lock()
	if f := s.findHash(h, data); f != nil {
		s.touch(f)
		s.mu.Unlock()
		return f
	}
	s.mu.Unlock()

	ttf, err := parseFontData(data)
	if err != nil {
		return nil
	}
	f := &fontData{store: s, data: data, ttf: ttf, hash: h}
	s.mu.Lock()
	defer s.mu.Unlock()
	if g := s.findHash(h, data); g != nil {
		s.touch(g)
		return g
	}
	s.byPtr[pk] = f
	s.byHash[h] = append(s.byHash[h], f)
	s.fonts = append(s.fonts, f)
	s.touch(f)
	if len(s.fonts) > maxInternedFonts {
		s.dropOldestFont()
	}
	return f
}

// findHash ищет шрифт с тем же содержимым среди кандидатов хэша. Под s.mu.
func (s *glyphStore) findHash(h uint64, data []byte) *fontData {
	for _, f := range s.byHash[h] {
		if bytes.Equal(f.data, data) {
			return f
		}
	}
	return nil
}

func (s *glyphStore) touch(f *fontData) { f.lastLookup.Store(s.lookups.Add(1)) }

// dropOldestFont выводит из индекса шрифт, который дольше всех не искали:
// его шарды очищаются и снимаются с учёта, дальше он живёт у своих видов сам
// по себе — без бюджета, под лимитами записей шарда. Повторная регистрация
// разберёт его заново. Под s.mu.
func (s *glyphStore) dropOldestFont() {
	oldest := 0
	for i, f := range s.fonts {
		if f.lastLookup.Load() < s.fonts[oldest].lastLookup.Load() {
			oldest = i
		}
	}
	f := s.fonts[oldest]
	s.fonts = append(s.fonts[:oldest], s.fonts[oldest+1:]...)
	delete(s.byPtr, ptrKey{&f.data[0], len(f.data)})
	cands := s.byHash[f.hash]
	for i, c := range cands {
		if c == f {
			s.byHash[f.hash] = append(cands[:i:i], cands[i+1:]...)
			break
		}
	}
	if len(s.byHash[f.hash]) == 0 {
		delete(s.byHash, f.hash)
	}
	f.detached.Store(true)
	s.bytes.Add(-f.clearShards())
}

// charge учитывает n байт (дельта учёта шарда) и вытесняет при превышении
// бюджета. Вызывать, не держа замков шардов.
func (s *glyphStore) charge(n int64) {
	if n == 0 {
		return
	}
	if s.bytes.Add(n) > s.budget.Load() {
		s.evict()
	}
}

// evict очищает самые давно не нужные шарды, пока учёт не опустится до
// половины бюджета. Под s.mu; замки шардов берутся по одному.
func (s *glyphStore) evict() {
	s.mu.Lock()
	defer s.mu.Unlock()
	budget := s.budget.Load()
	if s.bytes.Load() <= budget {
		return // вытеснил конкурент
	}
	type cand struct {
		last  uint64
		size  int64
		clear func() int64
	}
	var cands []cand
	for _, f := range s.fonts {
		f.mu.RLock()
		for _, sh := range f.shards {
			cands = append(cands, cand{sh.lastUse.Load(), sh.size(), sh.clear})
		}
		for _, ss := range f.shaped {
			cands = append(cands, cand{ss.lastUse.Load(), ss.size(), ss.clear})
		}
		f.mu.RUnlock()
	}
	// Самые давние — первыми; в одну эпоху — сначала крупные: освободить
	// нужное меньшим числом потерянных шардов.
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].last != cands[j].last {
			return cands[i].last < cands[j].last
		}
		return cands[i].size > cands[j].size
	})
	for _, c := range cands {
		if s.bytes.Load() <= budget/2 {
			break
		}
		s.bytes.Add(-c.clear())
	}
}

// colorGlyphCount — число цветных глифов в кэше (для Shaper.CacheSizes).
func (s *glyphStore) colorGlyphCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, f := range s.fonts {
		f.mu.RLock()
		for _, ss := range f.shaped {
			ss.mu.Lock()
			n += len(ss.colors) + len(ss.dyn)
			ss.mu.Unlock()
		}
		f.mu.RUnlock()
	}
	return n
}

// ─── Шрифт ──────────────────────────────────────────────────────────────────

// Состояние «есть ли у шрифта цветные глифы» (fontData.colorState).
const (
	colorUnknown uint32 = iota // typesetting ещё не разбирал
	colorNo
	colorYes
)

// fontData — разобранный шрифт, общий для всех видов с тем же содержимым.
type fontData struct {
	store      *glyphStore // хранилище, в котором шрифт учитывается
	data       []byte
	ttf        *opentype.Font // sfnt.Font: безопасен с отдельным Buffer
	hash       uint64
	lastLookup atomic.Uint64
	// detached — шрифт выбыл из индекса: его шарды живут без бюджета.
	detached atomic.Bool
	// colorState — есть ли цветные глифы (COLR/CBDT); читается без замка на
	// горячем пути шейпера, выставляется при разборе typesetting.
	colorState atomic.Uint32

	mu     sync.RWMutex
	shards map[float64]*shard
	shaped map[fixed.Int26_6]*shapedShard

	// Шейпинг: typesetting-шрифт разбирается один раз, лениво; scratch-face
	// служит растеризации масок и цветных глифов. Всё под shapeMu.
	shapeMu sync.Mutex
	tsFont  *tsfont.Font
	tsDone  bool
	scratch *tsfont.Face
}

// charge передаёт дельту учёта шарда в хранилище; шрифт вне индекса бюджета
// не касается.
func (f *fontData) charge(n int64) {
	if f.detached.Load() {
		return
	}
	f.store.charge(n)
}

// clearShards очищает все шарды шрифта и возвращает снятые с учёта байты.
func (f *fontData) clearShards() int64 {
	var n int64
	f.mu.RLock()
	for _, sh := range f.shards {
		n += sh.clear()
	}
	for _, ss := range f.shaped {
		n += ss.clear()
	}
	f.mu.RUnlock()
	return n
}

// typesetting возвращает общий *tsfont.Font (nil — не разобрался); виды
// заворачивают его в личный tsfont.NewFace.
func (f *fontData) typesetting() *tsfont.Font {
	f.shapeMu.Lock()
	defer f.shapeMu.Unlock()
	return f.typesettingLocked()
}

func (f *fontData) typesettingLocked() *tsfont.Font {
	if f.tsDone {
		return f.tsFont
	}
	f.tsDone = true
	face, err := tsfont.ParseTTF(bytes.NewReader(f.data))
	if err != nil {
		// TTC-коллекция — первый шрифт (симметрично parseFontData).
		if faces, cerr := tsfont.ParseTTC(bytes.NewReader(f.data)); cerr == nil && len(faces) > 0 {
			face, err = faces[0], nil
		}
	}
	if err != nil {
		f.colorState.Store(colorNo)
		return nil
	}
	f.tsFont = face.Font
	f.scratch = face
	if faceHasColor(face) {
		f.colorState.Store(colorYes)
	} else {
		f.colorState.Store(colorNo)
	}
	return f.tsFont
}

// withScratch выполняет fn со scratch-face шрифта под shapeMu, выставив ppem
// под размер sizePx (растровые страйки и экстенты зависят от него, а face
// один на все шарды шрифта); nil-face — шрифт не разобрался typesetting'ом.
func (f *fontData) withScratch(sizePx fixed.Int26_6, fn func(face *tsfont.Face)) {
	f.shapeMu.Lock()
	defer f.shapeMu.Unlock()
	f.typesettingLocked()
	face := f.scratch
	if face != nil {
		setPpemFor(face, sizePx)
	}
	fn(face)
}

// setPpemFor выставляет ppem face под кегль sizePx, если у шрифта есть
// растровые страйки (их выбор и экстенты зависят от ppem).
func setPpemFor(face *tsfont.Face, sizePx fixed.Int26_6) {
	if len(face.BitmapSizes()) == 0 {
		return
	}
	em := uint16((int(sizePx) + 32) / 64)
	if em == 0 {
		em = 1
	}
	if x, y := face.Ppem(); x != em || y != em {
		face.SetPpem(em, em)
	}
}

// shard возвращает шард кегля sizePt, создавая при первом обращении.
func (f *fontData) shard(sizePt float64) *shard {
	f.mu.RLock()
	s := f.shards[sizePt]
	f.mu.RUnlock()
	if s != nil {
		return s
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if s = f.shards[sizePt]; s != nil {
		return s
	}
	if f.shards == nil {
		f.shards = make(map[float64]*shard)
	}
	s = &shard{font: f, sizePt: sizePt}
	f.shards[sizePt] = s
	return s
}

// shapedShard возвращает шард шейпера размера sizePx (26.6).
func (f *fontData) shapedShard(sizePx fixed.Int26_6) *shapedShard {
	f.mu.RLock()
	s := f.shaped[sizePx]
	f.mu.RUnlock()
	if s != nil {
		return s
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if s = f.shaped[sizePx]; s != nil {
		return s
	}
	if f.shaped == nil {
		f.shaped = make(map[fixed.Int26_6]*shapedShard)
	}
	s = &shapedShard{font: f, sizePx: sizePx}
	f.shaped[sizePx] = s
	return s
}

// ─── Шард per-rune пути ─────────────────────────────────────────────────────

// faceKey — параметры вида, от которых зависят face x/image и всё, что он
// считает: DPI и подпиксельный режим (хинтинг шага зависит от него).
type faceKey struct {
	dpi      float64
	subpixel bool
}

type l2GlyphKey struct {
	faceKey
	r     rune
	phase uint8
	q     TextQuality
}

type l2AdvKey struct {
	faceKey
	r rune
}

type l2KernKey struct {
	faceKey
	a, b rune
}

type advEntry struct {
	adv fixed.Int26_6
	ok  bool
}

// shard — растры одного шрифта одного кегля для всех DPI и режимов.
type shard struct {
	font    *fontData
	sizePt  float64
	lastUse atomic.Uint64

	mu      sync.RWMutex
	faces   map[faceKey]font.Face
	glyphs  map[l2GlyphKey]CachedGlyph
	adv     map[l2AdvKey]advEntry
	kern    map[l2KernKey]fixed.Int26_6
	metrics map[faceKey]VMetric
	hints   map[fixed.Int26_6]*vHint // по ppem (кегль × DPI)
	buf     sfnt.Buffer
	rast    *vector.Rasterizer
	bytes   int64 // учёт шарда — источник правды для хранилища
}

// hit отмечает попадание: lastUse догоняет часы, только если отстал, —
// в установившемся режиме попадания ничего не пишут.
func (s *shard) hit() {
	if c := s.font.store.clock.Load(); s.lastUse.Load() != c {
		s.lastUse.Store(c)
	}
}

func (s *shard) miss() { s.lastUse.Store(s.font.store.clock.Add(1)) }

// size — учтённые байты шарда.
func (s *shard) size() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bytes
}

// clear освобождает содержимое шарда и возвращает учтённые байты.
func (s *shard) clear() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clearLocked()
}

func (s *shard) clearLocked() int64 {
	n := s.bytes
	s.faces, s.glyphs, s.adv, s.kern, s.metrics, s.hints = nil, nil, nil, nil, nil, nil
	s.bytes = 0
	return n
}

// faceLocked — scratch-face x/image для параметров k. Под s.mu.Lock.
func (s *shard) faceLocked(k faceKey) font.Face {
	if f, ok := s.faces[k]; ok {
		return f
	}
	// HintingFull в x/image не правит контуры (хинтинга там нет) — он только
	// округляет шаг каждого глифа и кернинг до целого пикселя. На мелком
	// кегле это и делает строку неровной: у Open Sans 8,5 pt (11,33 px)
	// «ширина» буквы 6,3 px становится то 6, то 7. Для подпиксельного режима
	// шаг оставляем дробным.
	hint := font.HintingFull
	if k.subpixel {
		hint = font.HintingNone
	}
	var face font.Face
	face, err := opentype.NewFace(s.font.ttf, &opentype.FaceOptions{
		Size:    s.sizePt,
		DPI:     k.dpi,
		Hinting: hint,
	})
	if err != nil {
		face = basicfont.Face7x13
	}
	if s.faces == nil {
		s.faces = make(map[faceKey]font.Face)
	}
	s.faces[k] = face
	return face
}

// glyphAt — маска глифа r для параметров k и качества q (фаза — доля пикселя
// пера, см. FontCache.GlyphAt).
func (s *shard) glyphAt(k faceKey, q TextQuality, r rune, phase int) CachedGlyph {
	key := l2GlyphKey{faceKey: k, r: r, phase: uint8(phase), q: q}
	s.mu.RLock()
	g, ok := s.glyphs[key]
	s.mu.RUnlock()
	if ok {
		s.hit()
		return g
	}
	g, delta := s.glyphMiss(key)
	s.miss()
	s.font.charge(delta)
	return g
}

// glyphMiss растеризует и кладёт глиф под замком; возвращает дельту учёта.
func (s *shard) glyphMiss(key l2GlyphKey) (CachedGlyph, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if g, ok := s.glyphs[key]; ok {
		return g, 0
	}
	before := s.bytes
	if s.glyphs == nil {
		s.glyphs = make(map[l2GlyphKey]CachedGlyph)
	} else if len(s.glyphs) >= maxShardGlyphs {
		s.clearLocked()
		s.glyphs = make(map[l2GlyphKey]CachedGlyph, maxShardGlyphs/4)
	}
	face := s.faceLocked(key.faceKey)
	g := s.rasterLocked(face, key.faceKey, key.q, key.r, int(key.phase))
	s.glyphs[key] = g
	s.bytes += glyphCost(g)
	return g, s.bytes - before
}

// rasterLocked растеризует глиф: хинтованный/LCD путь (texthint.go) либо
// opentype.Face.Glyph с плотной копией маски. Под s.mu.Lock.
func (s *shard) rasterLocked(face font.Face, k faceKey, q TextQuality, r rune, phase int) CachedGlyph {
	if q.Hinting || q.LCD {
		if hg, ok := s.hintedGlyph(face, k.dpi, r, phase, q); ok {
			return hg
		}
	}
	// Растеризация в целочисленной позиции пера: при HintingFull advance
	// квантуется до целых пикселей, поэтому дробная часть пера всегда 0 и
	// маска, снятая в (0,0), корректна для любой целочисленной позиции. В
	// подпиксельном режиме шаг дробный, и маска снимается для каждой из
	// четвертей пикселя отдельно (phase).
	dot := fixed.Point26_6{X: fixed.Int26_6(phase * (64 / SubpixelPhases))}
	dr, maskImg, maskPt, adv, ok := face.Glyph(dot, r)
	g := CachedGlyph{Advance: adv, OK: ok}
	if ok && !dr.Empty() {
		g.OffX, g.OffY = dr.Min.X, dr.Min.Y
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
		g.Mask = m
	}
	return g
}

// glyphCost — учтённые байты записи маски.
func glyphCost(g CachedGlyph) int64 {
	n := int64(entryOverhead)
	if g.Mask != nil {
		n += int64(len(g.Mask.Pix))
	}
	if g.LCD != nil {
		n += int64(len(g.LCD.Pix))
	}
	return n
}

// advance — шаг глифа r (face.GlyphAdvance) с его признаком ok.
func (s *shard) advance(k faceKey, r rune) (fixed.Int26_6, bool) {
	key := l2AdvKey{faceKey: k, r: r}
	s.mu.RLock()
	e, ok := s.adv[key]
	s.mu.RUnlock()
	if ok {
		s.hit()
		return e.adv, e.ok
	}
	e, delta := s.advanceMiss(key)
	s.miss()
	s.font.charge(delta)
	return e.adv, e.ok
}

func (s *shard) advanceMiss(key l2AdvKey) (advEntry, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.adv[key]; ok {
		return e, 0
	}
	before := s.bytes
	if s.adv == nil {
		s.adv = make(map[l2AdvKey]advEntry)
	} else if len(s.adv) >= maxShardSmall {
		s.bytes -= int64(len(s.adv)) * smallEntry
		s.adv = make(map[l2AdvKey]advEntry, maxShardSmall/4)
	}
	var e advEntry
	e.adv, e.ok = s.faceLocked(key.faceKey).GlyphAdvance(key.r)
	s.adv[key] = e
	s.bytes += smallEntry
	return e, s.bytes - before
}

// kerning — кернинг пары (face.Kern).
func (s *shard) kerning(k faceKey, a, b rune) fixed.Int26_6 {
	key := l2KernKey{faceKey: k, a: a, b: b}
	s.mu.RLock()
	v, ok := s.kern[key]
	s.mu.RUnlock()
	if ok {
		s.hit()
		return v
	}
	v, delta := s.kernMiss(key)
	s.miss()
	s.font.charge(delta)
	return v
}

func (s *shard) kernMiss(key l2KernKey) (fixed.Int26_6, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.kern[key]; ok {
		return v, 0
	}
	before := s.bytes
	if s.kern == nil {
		s.kern = make(map[l2KernKey]fixed.Int26_6)
	} else if len(s.kern) >= maxShardSmall {
		s.bytes -= int64(len(s.kern)) * smallEntry
		s.kern = make(map[l2KernKey]fixed.Int26_6, maxShardSmall/4)
	}
	v := s.faceLocked(key.faceKey).Kern(key.a, key.b)
	s.kern[key] = v
	s.bytes += smallEntry
	return v, s.bytes - before
}

// vmetrics — вертикальные метрики (face.Metrics) с округлением, как у
// отрисовки.
func (s *shard) vmetrics(k faceKey) VMetric {
	s.mu.RLock()
	m, ok := s.metrics[k]
	s.mu.RUnlock()
	if ok {
		s.hit()
		return m
	}
	m, delta := s.metricsMiss(k)
	s.miss()
	s.font.charge(delta)
	return m
}

func (s *shard) metricsMiss(k faceKey) (VMetric, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.metrics[k]; ok {
		return m, 0
	}
	before := s.bytes
	met := s.faceLocked(k).Metrics()
	m := VMetric{
		Ascent:  met.Ascent.Round(),
		Descent: met.Descent.Round(),
	}
	// Height у font.Metrics — шаг между строками, УЖЕ включающий зазор
	// (ascent+descent+lineGap по hhea), поэтому зазор — это остаток. У части
	// шрифтов округлённые части в сумме дают больше округлённого целого —
	// остаток тогда отрицательный, а отрицательный «зазор» сжал бы строки,
	// наезжая друг на друга; его обрезаем нулём.
	if gap := met.Height.Round() - m.Ascent - m.Descent; gap > 0 {
		m.LineGap = gap
	}
	if s.metrics == nil {
		s.metrics = make(map[faceKey]VMetric)
	}
	s.metrics[k] = m
	s.bytes += smallEntry
	return m, s.bytes - before
}

// ─── Шард шейпера ───────────────────────────────────────────────────────────

type shapedKey struct {
	gid  tsfont.GID
	hint bool
}

type dynKey struct {
	gid tsfont.GID
	col colorKey
}

// colorKey — RGBA как сравнимое значение ключа.
type colorKey struct{ r, g, b, a uint8 }

// shapedShard — маски и цветные глифы по GID одного шрифта одного размера
// в пикселях (путь шейпера). Растеризация — scratch-face шрифта под
// fontData.shapeMu, который берётся под s.mu.
type shapedShard struct {
	font    *fontData
	sizePx  fixed.Int26_6
	lastUse atomic.Uint64

	mu     sync.Mutex
	masks  map[shapedKey]*GlyphMask
	colors map[tsfont.GID]*ColorGlyph
	dyn    map[dynKey]*ColorGlyph
	vh     *vHint
	vhDone bool
	bytes  int64
}

func (s *shapedShard) hit() {
	if c := s.font.store.clock.Load(); s.lastUse.Load() != c {
		s.lastUse.Store(c)
	}
}

func (s *shapedShard) miss() { s.lastUse.Store(s.font.store.clock.Add(1)) }

func (s *shapedShard) size() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bytes
}

func (s *shapedShard) clear() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clearLocked()
}

func (s *shapedShard) clearLocked() int64 {
	n := s.bytes
	s.masks, s.colors, s.dyn, s.vh, s.vhDone = nil, nil, nil, nil, false
	s.bytes = 0
	return n
}

// mask — альфа-маска глифа gid (с автохинтингом по hint).
func (s *shapedShard) mask(gid tsfont.GID, hint bool) *GlyphMask {
	key := shapedKey{gid: gid, hint: hint}
	s.mu.Lock()
	m, ok := s.masks[key]
	s.mu.Unlock()
	if ok {
		s.hit()
		return m
	}
	m, delta := s.maskMiss(key)
	s.miss()
	s.font.charge(delta)
	return m
}

func (s *shapedShard) maskMiss(key shapedKey) (*GlyphMask, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.masks[key]; ok {
		return m, 0
	}
	before := s.bytes
	if s.masks == nil {
		s.masks = make(map[shapedKey]*GlyphMask)
	} else if len(s.masks) >= maxShapedMasks {
		s.clearLocked()
		s.masks = make(map[shapedKey]*GlyphMask, maxShapedMasks/4)
	}
	m := &GlyphMask{}
	s.font.withScratch(s.sizePx, func(face *tsfont.Face) {
		if face == nil {
			return
		}
		var vh *vHint
		if key.hint {
			vh = s.vhintLocked(face)
		}
		m = rasterizeGlyph(face, key.gid, s.sizePx, vh)
	})
	s.masks[key] = m
	s.bytes += entryOverhead
	if m.Mask != nil {
		s.bytes += int64(len(m.Mask.Pix))
	}
	return m, s.bytes - before
}

// colorGlyph — цветной глиф gid (COLR/CBDT) или nil; глифы со слоями
// foreground зависят от цвета текста и кэшируются с ним. Для шрифта без
// цветных глифов вызывающий (Shaper.ColorGlyphFor) сюда не заходит.
func (s *shapedShard) colorGlyph(gid tsfont.GID, textCol colorKey) *ColorGlyph {
	s.mu.Lock()
	cg, ok := s.colors[gid]
	if !ok {
		cg, ok = s.dyn[dynKey{gid: gid, col: textCol}]
	}
	s.mu.Unlock()
	if ok {
		s.hit()
		return cg
	}
	cg, delta := s.colorMiss(gid, textCol)
	s.miss()
	s.font.charge(delta)
	return cg
}

func (s *shapedShard) colorMiss(gid tsfont.GID, textCol colorKey) (*ColorGlyph, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cg, ok := s.colors[gid]; ok {
		return cg, 0
	}
	dk := dynKey{gid: gid, col: textCol}
	if cg, ok := s.dyn[dk]; ok {
		return cg, 0
	}
	before := s.bytes
	var cg *ColorGlyph
	var dynamic bool
	s.font.withScratch(s.sizePx, func(face *tsfont.Face) {
		if face != nil && s.font.colorState.Load() == colorYes {
			cg, dynamic = renderColorGlyph(face, gid, s.sizePx, textCol.rgba())
		}
	})
	if dynamic {
		if s.dyn == nil {
			s.dyn = make(map[dynKey]*ColorGlyph)
		} else if len(s.dyn) >= maxDynColorGlyphs {
			s.dropColorsLocked()
		}
		s.dyn[dk] = cg
	} else {
		if s.colors == nil {
			s.colors = make(map[tsfont.GID]*ColorGlyph)
		} else if len(s.colors) >= maxShapedMasks {
			s.dropColorsLocked()
		}
		s.colors[gid] = cg
	}
	s.bytes += entryOverhead
	if cg != nil && cg.Img != nil {
		s.bytes += int64(len(cg.Img.Pix))
	}
	return cg, s.bytes - before
}

// dropColorsLocked сбрасывает цветные глифы, оставляя маски и зоны; учёт
// пересчитывается по тому, что осталось. Под s.mu.
func (s *shapedShard) dropColorsLocked() {
	var keep int64
	for _, m := range s.masks {
		keep += entryOverhead
		if m.Mask != nil {
			keep += int64(len(m.Mask.Pix))
		}
	}
	if s.vhDone {
		keep += hintCost
	}
	s.colors = make(map[tsfont.GID]*ColorGlyph, maxShapedMasks/4)
	s.dyn = make(map[dynKey]*ColorGlyph, maxDynColorGlyphs/4)
	s.bytes = keep
}

func (c colorKey) rgba() color.RGBA { return color.RGBA{R: c.r, G: c.g, B: c.b, A: c.a} }

func keyOfColor(c color.RGBA) colorKey { return colorKey{c.R, c.G, c.B, c.A} }
