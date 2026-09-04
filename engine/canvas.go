// Package engine реализует off-screen рендеринг и детектирование изменений.
//
// Canvas использует двойную буферизацию:
//   - back — текущий рендер (виджеты рисуют сюда)
//   - front — последний отправленный потребителю кадр
//
// После каждого рендера back сравнивается с front побайтово по тайлам 64×64.
// Изменившиеся тайлы копируются во front и возвращаются как []output.DirtyTile.
package engine

import (
	"bytes"
	"image"
	"image/color"
	stdraw "image/draw"
	"math"
	"reflect"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	"golang.org/x/image/draw"
	"golang.org/x/image/math/fixed"

	"github.com/oops1/headless-gui/v3/output"
)

// Canvas — off-screen RGBA-холст с двойной буферизацией.
// Реализует интерфейс widget.DrawContext.
type Canvas struct {
	front      *image.RGBA           // последний отправленный кадр
	back       *image.RGBA           // текущий рендер-таргет (может быть чужой памятью — см. SetSurface)
	backOwn    *image.RGBA           // собственный back-буфер холста; back переключается на него, когда внешняя память не задана
	format     PixelFormat           // порядок каналов back-буфера (см. pixelformat.go)
	formatOwn  PixelFormat           // порядок каналов СОБСТВЕННОГО буфера: чужая память вправе иметь свой, и после возврата к своему буферу надо вернуться к нему же
	bgImage    *image.RGBA           // фоновое изображение (масштабировано под холст, закодировано в format — см. setBackground)
	fontCache  *FontCache            // кэш шрифта по умолчанию
	namedFonts map[string]*FontCache // именованные шрифты (FontFamily из XAML)
	fallbacks  []*FontCache          // fallback-шрифты для отсутствующих глифов (BUG-2)
	clip       image.Rectangle       // активная область отсечения
	hasClip    bool                  // включено ли отсечение
	round      roundClipState        // отсечение по скруглённому контуру (clipround.go)
	baseClip   image.Rectangle       // базовый клип кадра (damage-область частичной перерисовки)
	hasBase    bool                  // активен ли базовый клип
	scaleTmp   *image.RGBA           // переиспользуемый буфер для DrawImageScaled
	shaper     textShaper            // шейпинг сложного текста (RTL, лигатуры; см. shaper.go)
	W, H       int                   // ФИЗИЧЕСКИЙ размер буферов (логический × scale)
	// marks — признак содержимого по тайлам за текущий кадр (regions.go).
	marks []tileMark
	// maskKind — чем считается то, что рисуют маски альфы и цветные глифы
	// прямо сейчас: буквами, фигурой, тенью. Через них идут все три.
	maskKind output.RegionKind

	// drawDamage — области ЭТОГО кадра в логических координатах, по которым
	// обход решает, что можно не рисовать (widget.CullScope). Живут на
	// канвасе, а не в пакете widget: у каждого движка кадр свой.
	drawDamage []image.Rectangle
	// cullingOn — разрешён ли пропуск поддеревьев в этом движке. Атомарный:
	// выключатель дёргает приложение из своей горутины, а читает обход в
	// горутине кадра.
	cullingOn atomic.Bool
	// occlusionOn — разрешено ли вычитание перекрытого (widget/occlusion.go).
	// Отдельный выключатель от cullingOn: ошибиться в них можно по-разному —
	// пропуск по damage промахивается мимо изменившегося, а вычитание верит
	// объявлению виджета о собственной непрозрачности.
	occlusionOn atomic.Bool

	tilesX int
	tilesY int

	// HiDPI (см. scale.go): виджеты живут в логических пикселях,
	// буферы — в физических. При scale == 1 пути тождественны прежним.
	scale              float64
	logicalW, logicalH int

	// fontRev растёт при смене состава шрифтов: по нему кэшированные
	// клоны канваса (popup-оверлеи) понимают, что устарели.
	fontRev uint64

	// Кэш результата масштабирования (см. DrawImageScaled).
	scaledCache  map[scaledKey]*scaledEntry
	scaledClock  uint64
	scaledPixels int
}

// RegisterFont регистрирует именованный шрифт (TTF-данные) в реестре холста.
// fontName соответствует FontFamily в XAML.
func (c *Canvas) RegisterFont(fontName string, ttfData []byte) {
	if c.namedFonts == nil {
		c.namedFonts = make(map[string]*FontCache)
	}
	fc := newFontCacheFromData(ttfData, c.fontCache.dpi)
	if fc != nil {
		c.namedFonts[fontName] = fc
		c.fontRev++
	}
}

// fontFor возвращает FontCache для именованного шрифта; если не найден — default.
func (c *Canvas) fontFor(fontName string) *FontCache {
	if fontName != "" && c.namedFonts != nil {
		if fc, ok := c.namedFonts[fontName]; ok {
			return fc
		}
	}
	return c.fontCache
}

// hasFont сообщает, зарегистрирован ли именованный шрифт.
func (c *Canvas) hasFont(name string) bool {
	_, ok := c.namedFonts[name]
	return ok
}

// SetDefaultFont делает зарегистрированный именованный шрифт шрифтом по умолчанию
// (используется DrawText/DrawTextSize и как primary для fallback). Возвращает
// false, если шрифт с таким именем не зарегистрирован.
func (c *Canvas) SetDefaultFont(name string) bool {
	if name == "" {
		return false
	}
	if fc, ok := c.namedFonts[name]; ok && fc != nil {
		c.fontCache = fc
		c.fontRev++
		return true
	}
	return false
}

// fontNames возвращает отсортированный список зарегистрированных именованных шрифтов.
func (c *Canvas) fontNames() []string {
	out := make([]string, 0, len(c.namedFonts))
	for k := range c.namedFonts {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// AddFallbackFont добавляет fallback-шрифт из TTF/OTF-данных (BUG-2).
// Используется для рун, отсутствующих в основном шрифте.
func (c *Canvas) AddFallbackFont(ttfData []byte) bool {
	fc := newFontCacheFromData(ttfData, c.fontCache.dpi)
	if fc == nil {
		return false
	}
	c.fallbacks = append(c.fallbacks, fc)
	c.fontRev++
	return true
}

// fcForRune возвращает FontCache, содержащий глиф для руны r: сначала primary,
// затем fallback-цепочку. Второе значение — найден ли глиф вообще: если false,
// ни один шрифт не покрывает руну (вызывающий код пропускает отрисовку, чтобы
// не рисовать уродливый .notdef-квадрат «тофу»).
func (c *Canvas) fcForRune(primary *FontCache, r rune) (*FontCache, bool) {
	if primary.HasGlyph(r) {
		return primary, true
	}
	for _, fb := range c.fallbacks {
		if fb.HasGlyph(r) {
			return fb, true
		}
	}
	return primary, false
}

func newCanvas(w, h int, fc *FontCache) *Canvas {
	return newCanvasScaled(w, h, 1.0, fc)
}

// newCanvasScaled создаёт холст логического размера (w, h) с HiDPI-масштабом
// scale: буферы аллоцируются в физических пикселях (w×scale, h×scale).
//
// Размеры валидируются здесь же (SEC-10, вторая линия обороны после
// clampCanvasSize в New/SetResolution): отрицательные и нулевые стороны
// давали пустые буферы и tilesX/tilesY ≤ 0 — всю дальнейшую тайловую
// арифметику это превращало в мусор, а масштаб мог раздуть физический
// размер сверх любых границ.
func newCanvasScaled(w, h int, scale float64, fc *FontCache) *Canvas {
	if scale <= 0 {
		scale = 1
	}
	w, h = clampCanvasSize(w, h)
	pw := int(math.Round(float64(w) * scale))
	ph := int(math.Round(float64(h) * scale))
	pw, ph = clampCanvasSize(pw, ph) // масштаб мог вывести за предел
	ts := output.TileSize
	backBuf := image.NewRGBA(image.Rect(0, 0, pw, ph))
	c := &Canvas{
		front:      image.NewRGBA(image.Rect(0, 0, pw, ph)),
		back:       backBuf,
		backOwn:    backBuf,
		fontCache:  fc,
		namedFonts: make(map[string]*FontCache),
		W:          pw,
		H:          ph,
		tilesX:     (pw + ts - 1) / ts,
		tilesY:     (ph + ts - 1) / ts,
		scale:      scale,
		logicalW:   w,
		logicalH:   h,
	}
	// Пропуск поддеревьев включён по умолчанию: контракт отрисовки объявлен,
	// а приложение с нарушенным вернёт прежний обход одной строкой
	// (Engine.SetSubtreeCulling).
	c.cullingOn.Store(true)
	c.occlusionOn.Store(true)
	c.initTileMarks()
	return c
}

// ─── Background ──────────────────────────────────────────────────────────────

// setBackground масштабирует src до размера холста и сохраняет как фон.
// Фон блиттируется в back-буфер в начале каждого кадра — до отрисовки виджетов.
// Использует билинейную интерполяцию (golang.org/x/image/draw.BiLinear).
func (c *Canvas) setBackground(src image.Image) {
	dst := image.NewRGBA(image.Rect(0, 0, c.W, c.H))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), stdraw.Over, nil)
	if c.format == FormatBGRX {
		// Кодируем ОДИН раз здесь, а не на каждом blitBackground: фон не
		// меняется от кадра к кадру, а blitBackground вызывается каждый.
		swapRB(dst.Pix)
	}
	c.bgImage = dst
}

// DrawDamage реализует widget.CullScope: области кадра в логических
// координатах. Пустой список — рисуем всё.
func (c *Canvas) DrawDamage() []image.Rectangle { return c.drawDamage }

// SubtreeCullingEnabled реализует widget.CullScope.
func (c *Canvas) SubtreeCullingEnabled() bool { return c.cullingOn.Load() }

// setDrawDamage задаёт области кадра (вызывает движок перед обходом).
func (c *Canvas) setDrawDamage(rects []image.Rectangle) {
	c.drawDamage = append(c.drawDamage[:0], rects...)
}

// clearDrawDamage снимает ограничение: обход пройдёт целиком.
func (c *Canvas) clearDrawDamage() { c.drawDamage = c.drawDamage[:0] }

// clearBackground снимает фоновое изображение: blitBackground снова будет
// заливать буфер чёрным.
func (c *Canvas) clearBackground() { c.bgImage = nil }

// blitBackground очищает back-буфер и копирует фон (если задан).
// Вызывается до отрисовки виджетов — перезаписывает весь back.
// Если фонового изображения нет — заливает буфер чёрным цветом,
// чтобы при перемещении виджетов старые пиксели не оставались.
func (c *Canvas) blitBackground() {
	// Построчно через PixOffset обеих сторон, а не одним copy(back.Pix,
	// bgImage.Pix): back.Stride может отличаться от bgImage.Stride (back —
	// чужая память с собственным шагом строки, см. SetSurface), да и просто
	// писать за пределы c.W×c.H в чужой буфер нельзя, даже если у него есть
	// запасные байты дальше по срезу.
	rowBytes := c.W * 4
	all := image.Rect(0, 0, c.W, c.H)
	if c.bgImage != nil {
		for y := 0; y < c.H; y++ {
			dOff := c.back.PixOffset(0, y)
			sOff := c.bgImage.PixOffset(0, y)
			copy(c.back.Pix[dOff:dOff+rowBytes], c.bgImage.Pix[sOff:sOff+rowBytes])
		}
		c.markBackground(all)
		return
	}
	// Очищаем буфер чёрным (RGBA = 0,0,0,255). R=G=B=0 — перестановка
	// каналов (FormatBGRX) здесь ничего не меняет, менять цвет по формату
	// незачем.
	for y := 0; y < c.H; y++ {
		off := c.back.PixOffset(0, y)
		row := c.back.Pix[off : off+rowBytes]
		for i := 0; i < len(row); i += 4 {
			row[i+0] = 0
			row[i+1] = 0
			row[i+2] = 0
			row[i+3] = 255
		}
	}
	c.markBackground(all)
}

// markBackground помечает тайлы, накрытые фоном.
//
// Фон кладётся в начале каждого кадра построчным копированием, и пометки этот
// путь не ставил вовсе. Из-за этого тайл, накрытый ТОЛЬКО фоном — а на
// рабочем столе с обоями это почти весь экран, — оставался нетронутым, и
// кадр сообщал о нём «неизвестно что». Потребитель, выбирающий кодек по
// output.Frame.Regions, на обоях не получал ни одной подсказки и кодировал
// каждый прямоугольник вслепую. Обои при этом — и самое большое на экране, и
// то, на чём кодек без потерь теряет больше всего времени.
//
// Блит изображения помечается точно так же (DrawImage → markImage): фон —
// тот же блит, только раньше остальных.
func (c *Canvas) markBackground(r image.Rectangle) {
	if c.bgImage != nil {
		c.markImage(r)
		return
	}
	// Пустой фон — сплошная чёрная заливка, накрывающая тайлы целиком.
	// Сообщить о ней стоит: это команда заливки вместо тайла с пикселями.
	c.markSolid(r, color.RGBA{A: 255}, true)
}

// blitBackgroundIn — как blitBackground, но только в области r (частичная
// перерисовка): вне r back-буфер сохраняет прошлый кадр.
func (c *Canvas) blitBackgroundIn(r image.Rectangle) {
	r = r.Intersect(c.back.Bounds())
	if r.Empty() {
		return
	}
	if c.bgImage != nil {
		// bgImage — ВСЕГДА собственный image.NewRGBA(W,H) канваса (Stride =
		// 4×W без запаса), а back может быть чужой памятью с другим шагом
		// строки (см. SetSurface) — оффсет считаем для каждого раздельно,
		// один и тот же off тут не годится.
		rowBytes := r.Dx() * 4
		for y := r.Min.Y; y < r.Max.Y; y++ {
			dOff := c.back.PixOffset(r.Min.X, y)
			sOff := c.bgImage.PixOffset(r.Min.X, y)
			copy(c.back.Pix[dOff:dOff+rowBytes], c.bgImage.Pix[sOff:sOff+rowBytes])
		}
		c.markBackground(r)
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		off := c.back.PixOffset(r.Min.X, y)
		row := c.back.Pix[off : off+r.Dx()*4]
		for i := 0; i < len(row); i += 4 {
			row[i+0] = 0
			row[i+1] = 0
			row[i+2] = 0
			row[i+3] = 255
		}
	}
	c.markBackground(r)
}

// ─── Внешняя память back-буфера (Engine.SetSurface, surface.go) ────────────

// setExternalBack переключает back на память потребителя. Размер и шаг
// строки здесь УЖЕ проверены вызывающим (Engine.SetSurface) — Canvas только
// строит обёртку image.RGBA поверх чужого среза, без копирования.
//
// front остаётся собственным буфером канваса всегда (см. поле front) —
// только back меняется.
func (c *Canvas) setExternalBack(pix []byte, stride int, f PixelFormat) {
	c.back = &image.RGBA{Pix: pix, Stride: stride, Rect: image.Rect(0, 0, c.W, c.H)}
	c.format = f
}

// setOwnFormat задаёт формат собственного буфера. Он же становится текущим,
// пока рисуем в свой буфер.
func (c *Canvas) setOwnFormat(f PixelFormat) {
	c.formatOwn = f
	if c.back == c.backOwn {
		c.format = f
	}
}

// useOwnBack возвращает back к собственному буферу канваса
// (Engine.SetSurface(nil, …, …)).
func (c *Canvas) useOwnBack() {
	c.back = c.backOwn
	// Формат возвращается вместе с буфером. Иначе порядок каналов, заданный
	// чужой памятью, оставался бы у собственного буфера навсегда: после
	// SetPixelFormat(BGRX) → SetSurface(внешний RGBA) → SetSurface(nil) все
	// последующие кадры кодировались бы не тем, что просил вызывающий, а
	// Engine.PixelFormat() сообщал бы неправду.
	c.format = c.formatOwn
}

// ─── Clip ───────────────────────────────────────────────────────────────────

// SetClip ограничивает все последующие операции рисования прямоугольником r
// (ЛОГИЧЕСКИЕ координаты; внутренняя область отсечения — физическая).
// При активном базовом клипе (частичная перерисовка) итоговая область —
// пересечение r с базовым клипом: виджеты не могут рисовать вне damage.
func (c *Canvas) SetClip(r image.Rectangle) {
	pr := c.sRect(r)
	if c.hasBase {
		// hasClip остаётся true даже при пустом пересечении — рисовать
		// вне damage нельзя (пустой clip == «не рисовать ничего»).
		c.clip = pr.Intersect(c.baseClip)
		c.hasClip = true
		return
	}
	c.clip = pr.Intersect(c.back.Bounds())
	c.hasClip = !c.clip.Empty()
}

// ClearClip снимает ограничение области рисования (до базового клипа,
// если идёт частичная перерисовка).
func (c *Canvas) ClearClip() {
	if c.hasBase {
		c.clip = c.baseClip
		c.hasClip = true
		return
	}
	c.hasClip = false
}

// setBaseClip включает базовый клип кадра: вся отрисовка (включая
// SetClip/ClearClip виджетов) ограничивается прямоугольником r.
// Используется движком для частичной перерисовки по damage-области.
func (c *Canvas) setBaseClip(r image.Rectangle) {
	c.baseClip = r.Intersect(c.back.Bounds())
	c.hasBase = true
	c.clip = c.baseClip
	c.hasClip = true
}

// clearBaseClip выключает базовый клип (конец частичного кадра).
func (c *Canvas) clearBaseClip() {
	c.hasBase = false
	c.hasClip = false
}

// Clip возвращает текущую область отсечения в ЛОГИЧЕСКИХ координатах
// (или полные логические границы холста).
func (c *Canvas) Clip() image.Rectangle {
	if c.hasClip {
		return c.unRect(c.clip)
	}
	return image.Rect(0, 0, c.logicalW, c.logicalH)
}

// clampRect пересекает r с текущей областью отсечения (или bounds холста).
func (c *Canvas) clampRect(r image.Rectangle) image.Rectangle {
	if c.hasClip {
		return r.Intersect(c.clip)
	}
	return r.Intersect(c.back.Bounds())
}

// ─── DrawContext ────────────────────────────────────────────────────────────

// FillRect заливает прямоугольник сплошным цветом (координаты логические).
func (c *Canvas) FillRect(x, y, w, h int, col color.RGBA) {
	if col.A == 0 {
		return
	}
	c.fillRectPx(c.sRect(image.Rect(x, y, x+w, y+h)), c.enc(col), false)
}

// FillRectAlpha заливает прямоугольник с альфа-смешиванием (Over).
//
// ВАЖНО: col — alpha-premultiplied (модель color.RGBA в Go): R,G,B ≤ A.
// Прямой (straight) цвет вида {R:96, G:156, B:235, A:40} даст пересвет
// и накопление при наложении. Перевести straight → premultiplied:
// R*A/255, G*A/255, B*A/255.
func (c *Canvas) FillRectAlpha(x, y, w, h int, col color.RGBA) {
	c.fillRectPx(c.sRect(image.Rect(x, y, x+w, y+h)), c.enc(col), true)
}

// fillRectPx — заливка в ФИЗИЧЕСКИХ координатах (внутренний примитив).
//
// КОНТРАКТ: col приходит УЖЕ в порядке байт back-буфера (см. Canvas.enc) —
// сама fillRectPx формат не знает и ничего не переставляет. Так и
// backdrop.go может гонять сюда сырые байты, снятые прямо с back (после
// размытия), не рискуя, что их перекодируют повторно.
//
// Ручные циклы вместо stdraw.Draw(&image.Uniform{...}): универсальный путь
// стандартной библиотеки аллоцировал Uniform на каждый вызов (в профиле —
// десятки тысяч объектов за секунды нагрузки). Src — заполнение первой строки
// + copy остальных; Over — побайтово по формуле drawFillOver из image/draw
// (та же 16-битная арифметика, поэтому результат идентичен до бита).
func (c *Canvas) fillRectPx(r image.Rectangle, col color.RGBA, over bool) {
	// Заливка — то, о чём потребителю знать выгоднее всего: сплошную область
	// он передаёт командой протокола и не кодирует вовсе.
	c.markSolid(r, col, !over || col.A == 255)
	r = c.clampRect(r)
	if r.Empty() {
		return
	}
	// Скруглённое отсечение сужает каждую строку по-своему, поэтому заливка
	// идёт построчно. Ветка стоит одну проверку, когда отсечения нет.
	if c.round.active {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			x0, x1, ok := c.round.spanX(y, r.Min.X, r.Max.X)
			if !ok {
				continue
			}
			c.fillRectRaw(image.Rect(x0, y, x1, y+1), col, over)
		}
		return
	}
	c.fillRectRaw(r, col, over)
}

// fillRectRaw — заливка уже зажатого прямоугольника, без проверок отсечения.
func (c *Canvas) fillRectRaw(r image.Rectangle, col color.RGBA, over bool) {
	if over && col.A != 255 {
		if col.A == 0 {
			return
		}
		const m = 1<<16 - 1
		sr, sg, sb, sa := uint32(col.R)*0x101, uint32(col.G)*0x101, uint32(col.B)*0x101, uint32(col.A)*0x101
		a := (m - sa) * 0x101
		for y := r.Min.Y; y < r.Max.Y; y++ {
			i := c.back.PixOffset(r.Min.X, y)
			row := c.back.Pix[i : i+r.Dx()*4]
			for x := 0; x < len(row); x += 4 {
				row[x+0] = uint8((uint32(row[x+0])*a/m + sr) >> 8)
				row[x+1] = uint8((uint32(row[x+1])*a/m + sg) >> 8)
				row[x+2] = uint8((uint32(row[x+2])*a/m + sb) >> 8)
				row[x+3] = uint8((uint32(row[x+3])*a/m + sa) >> 8)
			}
		}
		return
	}
	// Src (или непрозрачный Over — то же самое): первая строка руками,
	// остальные — копированием первой (memmove).
	first := c.back.PixOffset(r.Min.X, r.Min.Y)
	row0 := c.back.Pix[first : first+r.Dx()*4]
	for x := 0; x < len(row0); x += 4 {
		row0[x+0] = col.R
		row0[x+1] = col.G
		row0[x+2] = col.B
		row0[x+3] = col.A
	}
	for y := r.Min.Y + 1; y < r.Max.Y; y++ {
		i := c.back.PixOffset(r.Min.X, y)
		copy(c.back.Pix[i:i+len(row0)], row0)
	}
}

// FillRoundRect заливает прямоугольник со скруглёнными углами радиуса r.
// Углы сглажены (AA-маски, см. aa.go); прямоугольное тело заливается
// быстрым Src-fill. Полупрозрачные цвета идут по старому не-AA пути,
// чтобы не смешивать семантики Src (тело) и Over (углы).
// Все входные координаты логические; при HiDPI скейлятся здесь единожды.
func (c *Canvas) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	if r <= 0 {
		c.FillRect(x, y, w, h, col)
		return
	}
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	// Переход в физические координаты.
	px, py := c.sx(x), c.sx(y)
	pw, ph := c.sl(x, w), c.sl(y, h)
	pr := c.st(r)
	if pr > pw/2 {
		pr = pw / 2
	}
	if pr > ph/2 {
		pr = ph / 2
	}
	if col.A < 255 {
		c.fillRoundRectLegacy(px, py, pw, ph, pr, c.enc(col))
		return
	}
	// Тело идёт через fillRectPx, который col не кодирует (см. его
	// контракт) — кодируем один раз здесь. Углы идут через drawCorners →
	// drawAlphaMask, который кодирует col САМ (см. его комментарий), поэтому
	// туда передаём исходный, ещё не закодированный col — не задваивать же
	// перестановку каналов.
	encCol := c.enc(col)
	// Тело: средняя полоса на всю ширину + верх/низ между углами.
	c.fillRectPx(image.Rect(px, py+pr, px+pw, py+ph-pr), encCol, false)
	c.fillRectPx(image.Rect(px+pr, py, px+pw-pr, py+pr), encCol, false)
	c.fillRectPx(image.Rect(px+pr, py+ph-pr, px+pw-pr, py+ph), encCol, false)
	// Сглаженные углы.
	c.drawCorners(cornersFor(pr, cornerFill), px, py, pw, ph, pr, col)
}

// fillRoundRectLegacy — ступенчатая заливка для полупрозрачных цветов
// (A<255). Координаты ФИЗИЧЕСКИЕ. col приходит уже закодированным
// (см. контракт fillRectPx) — вызывается только из FillRoundRect.
//
// Смешивает, а не пишет цвет как есть. Ветка выбирается ИМЕННО по
// полупрозрачности, и запись без смешивания оставляла в буфере цвет с чужой
// альфой: подсветка кнопки под курсором не ложилась плёнкой поверх панели, а
// пробивала в ней дыру — на экране это выглядело светлым пятном, потому что
// сквозь неё просвечивал фон приёмника.
func (c *Canvas) fillRoundRectLegacy(x, y, w, h, r int, col color.RGBA) {
	c.fillRectPx(image.Rect(x, y+r, x+w, y+h-r), col, true)
	rf := float64(r)
	for i := 0; i < r; i++ {
		dy := float64(r - i - 1)
		inset := r - int(math.Round(math.Sqrt(rf*rf-dy*dy)))
		lineW := w - 2*inset
		if lineW > 0 {
			c.fillRectPx(image.Rect(x+inset, y+i, x+inset+lineW, y+i+1), col, true)     // верх
			c.fillRectPx(image.Rect(x+inset, y+h-1-i, x+inset+lineW, y+h-i), col, true) // низ
		}
	}
}

// DrawRoundBorder рисует 1-пиксельный (логический) контур со скруглёнными
// углами. Дуги углов сглажены (AA-маски четверть-кольца, см. aa.go).
func (c *Canvas) DrawRoundBorder(x, y, w, h, r int, col color.RGBA) {
	// Полупрозрачная рамка — плёнка поверх фона, а не запись цвета вместе с
	// чужой альфой: тонкая светлая обводка стеклянной панели иначе ложится
	// сплошной белой линией.
	blend := col.A < 255
	if r <= 0 {
		c.DrawBorder(x, y, w, h, col)
		return
	}
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	// Переход в физические координаты.
	px, py := c.sx(x), c.sx(y)
	pw, ph := c.sl(x, w), c.sl(y, h)
	pr := c.st(r)
	if pr > pw/2 {
		pr = pw / 2
	}
	if pr > ph/2 {
		pr = ph / 2
	}
	t := c.st(1) // физическая толщина линии
	// Прямые стороны идут через fillRectPx (не кодирует col — см. его
	// контракт), кодируем один раз для них; дуги — либо тот же путь
	// (легаси), либо drawAlphaMask, который кодирует col САМ (см. FillRoundRect
	// выше) — туда исходный, не закодированный col.
	encCol := c.enc(col)
	c.fillRectPx(image.Rect(px+pr, py, px+pw-pr, py+t), encCol, blend)       // верх
	c.fillRectPx(image.Rect(px+pr, py+ph-t, px+pw-pr, py+ph), encCol, blend) // низ
	c.fillRectPx(image.Rect(px, py+pr, px+t, py+ph-pr), encCol, blend)       // лево
	c.fillRectPx(image.Rect(px+pw-t, py+pr, px+pw, py+ph-pr), encCol, blend) // право
	if col.A < 255 {
		c.drawRoundBorderCornersLegacy(px, py, pw, ph, pr, encCol)
		return
	}
	// Сглаженные дуги углов.
	c.drawCorners(cornersFor(pr, cornerRing), px, py, pw, ph, pr, col)
}

// drawRoundBorderCornersLegacy — прежние ступенчатые дуги (для A<255).
// Координаты ФИЗИЧЕСКИЕ. col приходит уже закодированным (вызывается только
// из DrawRoundBorder).
func (c *Canvas) drawRoundBorderCornersLegacy(x, y, w, h, r int, col color.RGBA) {
	// Полупрозрачные дуги СМЕШИВАЮТСЯ с фоном, как и прямые стороны рамки.
	// Ветка выбирается по A<255, и запись цвета вместе с чужой альфой
	// оставляла на углах «пробитые» точки: у стеклянной панели прямые стороны
	// обводки ложились плёнкой, а четыре угла — дырами.
	put := c.setPixelPx
	if col.A < 255 {
		put = c.blendPixelPx
	}
	rf := float64(r)
	for i := 0; i <= r; i++ {
		dy := float64(r - i)
		dx := int(math.Round(math.Sqrt(rf*rf - dy*dy)))
		put(x+r-dx, y+i, col)         // верхний левый
		put(x+w-1-r+dx, y+i, col)     // верхний правый
		put(x+r-dx, y+h-1-i, col)     // нижний левый
		put(x+w-1-r+dx, y+h-1-i, col) // нижний правый
	}
}

// blendPixelPx кладёт полупрозрачную точку поверх фона (Over), уважая клипы.
// Отличается от setPixelPx только этим: та пишет цвет как есть.
func (c *Canvas) blendPixelPx(x, y int, col color.RGBA) {
	if c.hasClip && !image.Pt(x, y).In(c.clip) {
		return
	}
	if !c.round.contains(x, y) {
		return
	}
	if x < 0 || x >= c.W || y < 0 || y >= c.H {
		return
	}
	// Один пиксель — тот же путь, что у заливки с alpha-смешиванием:
	// прямоугольник 1×1 через fillRectPx, чтобы формулу смешивания не
	// пришлось держать в двух местах.
	c.fillRectPx(image.Rect(x, y, x+1, y+1), col, true)
}

// DrawBorder рисует 1-пиксельный (логический) контур прямоугольника.
func (c *Canvas) DrawBorder(x, y, w, h int, col color.RGBA) {
	// Полупрозрачная рамка — плёнка поверх фона, а не запись цвета вместе с
	// чужой альфой: тонкая светлая обводка стеклянной панели иначе ложится
	// сплошной белой линией.
	blend := col.A < 255
	px, py := c.sx(x), c.sx(y)
	pw, ph := c.sl(x, w), c.sl(y, h)
	t := c.st(1)
	encCol := c.enc(col)                                               // fillRectPx col не кодирует — см. его контракт
	c.fillRectPx(image.Rect(px, py, px+pw, py+t), encCol, blend)       // верх
	c.fillRectPx(image.Rect(px, py+ph-t, px+pw, py+ph), encCol, blend) // низ
	c.fillRectPx(image.Rect(px, py, px+t, py+ph), encCol, blend)       // лево
	c.fillRectPx(image.Rect(px+pw-t, py, px+pw, py+ph), encCol, blend) // право
}

// DrawText рисует строку TTF-шрифтом (Go Regular) размером DefaultFontSize.
func (c *Canvas) DrawText(text string, x, y int, col color.RGBA) {
	c.DrawTextSize(text, x, y, DefaultFontSize, col)
}

// DrawTextSize рисует строку шрифтом по умолчанию произвольного размера (в пунктах).
func (c *Canvas) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	c.drawTextWithFont(c.fontCache, text, x, y, sizePt, col)
}

// DrawTextFont рисует строку именованным шрифтом (fontName="") → шрифт по умолчанию.
func (c *Canvas) DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA) {
	c.drawTextWithFont(c.fontFor(fontName), text, x, y, sizePt, col)
}

// clipPenSlackX — запас справа от клипа (физические пиксели), после которого
// отрисовка строки прекращается. Покрывает отрицательный кернинг и левый свес
// маски глифа (g.offX < 0) — то есть случаи, когда перо уже правее клипа, а
// сам глиф ещё мог бы зацепить его край. 64 px кратно перекрывают и то и
// другое для любого разумного кегля.
const clipPenSlackX = 64

func (c *Canvas) drawTextWithFont(fc *FontCache, text string, x, y int, sizePt float64, col color.RGBA) {
	// Обе ветки блиттируют кэшированные альфа-маски глифов (см. FontCache.Glyph)
	// вместо повторной растеризации контуров через font.Drawer.
	//
	// HiDPI: позиция (x, y) логическая → физическая; метрики шрифта уже
	// физические (DPI = 96 × scale), поэтому дальше всё в физических px.
	px, py := c.sx(x), c.sx(y)
	ascent, descent := fc.vMetrics(sizePt)
	baseline := py + ascent

	// Вертикальный отсев: строка целиком вне клипа — не итерируем руны вовсе
	// (важно при частичной перерисовке, когда клип — небольшая damage-область).
	// Запас 4px на глифы, выходящие за ascent/descent (акценты и т.п.).
	if c.hasClip {
		if py-4 >= c.clip.Max.Y || py+ascent+descent+4 <= c.clip.Min.Y {
			return
		}
	}

	// Сложный текст (RTL, арабский, индийские скрипты, комбинируемые знаки) —
	// через шейпер (см. shaper.go). При недоступности шейпинга (шрифт не
	// распарсился typesetting'ом) — деградация на per-rune путь ниже.
	if needsShaping(text) && c.drawShapedText(fc, text, px, baseline, sizePt, col) {
		return
	}

	// Горизонтальный отсев (PERF-7): как только перо ушло правее клипа, все
	// оставшиеся глифы гарантированно за его пределами — продвижение пера в
	// LTR-шрифтах неотрицательно, а сложные скрипты уходят в шейпер выше.
	// Длинная строка в узком поле/скролле раньше целиком прогонялась через
	// кэши глифов и кернинга ради полностью отсечённого результата.
	penLimit := fixed.Int26_6(math.MaxInt32) // клипа нет — предела нет
	if c.hasClip {
		penLimit = fixed.I(c.clip.Max.X + clipPenSlackX)
	}

	// Быстрый путь: нет fallback-шрифтов — один шрифт, с кернингом
	// (поведение прежнего font.Drawer.DrawString; отсутствующий глиф
	// пропускается без продвижения пера — как делал Drawer с opentype).
	if len(c.fallbacks) == 0 {
		pen := fixed.I(px)
		prev := rune(-1)
		for _, r := range text {
			if pen >= penLimit {
				break
			}
			if prev >= 0 {
				pen += fc.Kern(sizePt, prev, r)
			}
			g := fc.Glyph(sizePt, r)
			if !g.ok {
				continue
			}
			c.drawGlyphMask(g, pen.Round()+g.offX, baseline+g.offY, col)
			pen += g.advance
			prev = r
		}
		return
	}

	// Fallback-путь: по рунам, выбирая шрифт с нужным глифом.
	// Baseline от основного шрифта, кернинг не применяется (прежнее поведение).
	pen := fixed.I(px)
	for _, r := range text {
		if pen >= penLimit {
			break
		}
		chosen, found := c.fcForRune(fc, r)
		g := chosen.Glyph(sizePt, r)
		if found && g.ok {
			c.drawGlyphMask(g, pen.Round()+g.offX, baseline+g.offY, col)
		}
		// Отсутствующий глиф не рисуем (без .notdef-квадрата), но сохраняем
		// интервал по ширине пробела, чтобы текст не «слипался».
		adv := g.advance
		if !found || !g.ok {
			if sp := fc.Glyph(sizePt, ' '); sp.ok {
				adv = sp.advance
			}
		}
		pen += adv
	}
}

// drawGlyphMask альфа-блендит маску глифа из per-rune кэша.
func (c *Canvas) drawGlyphMask(g cachedGlyph, gx, gy int, col color.RGBA) {
	if g.mask == nil {
		return
	}
	c.withMaskKind(output.RegionText, func() { c.drawAlphaMask(g.mask, gx, gy, col) })
}

// drawAlphaMask альфа-блендит альфа-маску цветом col в back-буфер (Over,
// premultiplied — как image/draw для font.Drawer). Учитывает clip.
// (gx, gy) — позиция левого верхнего угла маски на холсте.
//
// В отличие от fillRectPx/setPixelPx, здесь col КОДИРУЕТСЯ (c.enc) прямо
// внутри: часть вызывающих кода (aa.go — AA-фигуры, скруглённые углы) не
// входит в файлы этой задачи и не может закодировать col сама на своей
// стороне до вызова. Тем вызывающим в canvas.go, что уже закодировали col
// сами (drawCorners из FillRoundRect/DrawRoundBorder — нет, туда специально
// передаётся ИСХОДНЫЙ col), двойного кодирования тут нет: единственная
// точка входа цвета для альфа-масок — эта функция.
func (c *Canvas) drawAlphaMask(alpha *image.Alpha, gx, gy int, col color.RGBA) {
	col = c.enc(col)
	if b := alpha.Bounds(); !b.Empty() {
		c.markKind(image.Rect(gx, gy, gx+b.Dx(), gy+b.Dy()))
	}
	mw, mh := alpha.Rect.Dx(), alpha.Rect.Dy()
	r := c.clampRect(image.Rect(gx, gy, gx+mw, gy+mh))
	if r.Empty() {
		return
	}
	sr := uint32(col.R) * 0x101 // 16-бит premultiplied компоненты
	sg := uint32(col.G) * 0x101
	sb := uint32(col.B) * 0x101
	sa := uint32(col.A) * 0x101
	const m16 = 1<<16 - 1
	// Строки берём подсрезами (PERF-7): компилятор снимает проверку границ на
	// каждый пиксель, арифметика смешивания — прежняя, результат бит-в-бит тот же.
	mask := alpha.Pix
	mStride := alpha.Stride
	dst := c.back.Pix
	for yy := r.Min.Y; yy < r.Max.Y; yy++ {
		// Скруглённое отсечение сужает строку: текст в панели со скруглёнными
		// углами не должен вылезать за кривую (см. clipround.go).
		lx, rx, ok := c.round.spanX(yy, r.Min.X, r.Max.X)
		if !ok {
			continue
		}
		rw := rx - lx
		mo := (yy-gy)*mStride + (lx - gx)
		mRow := mask[mo : mo+rw]
		dOff := c.back.PixOffset(lx, yy)
		dRow := dst[dOff : dOff+rw*4]
		for i := 0; i < rw; i++ {
			ma := uint32(mRow[i])
			if ma == 0 {
				continue
			}
			ma |= ma << 8 // 0..0xffff
			a := sa * ma / m16
			inv := m16 - a
			p := dRow[i*4 : i*4+4 : i*4+4]
			p[0] = uint8((uint32(p[0])*0x101*inv/m16 + sr*ma/m16) >> 8)
			p[1] = uint8((uint32(p[1])*0x101*inv/m16 + sg*ma/m16) >> 8)
			p[2] = uint8((uint32(p[2])*0x101*inv/m16 + sb*ma/m16) >> 8)
			p[3] = uint8((uint32(p[3])*0x101*inv/m16 + sa*ma/m16) >> 8)
		}
	}
}

// drawColorGlyph блиттит цветной глиф эмодзи (premultiplied RGBA) в back-буфер
// операцией Over. Цвет текста НЕ применяется — источник уже цветной. Учитывает
// clip. (gx, gy) — позиция левого верхнего угла изображения на холсте.
func (c *Canvas) drawColorGlyph(img *image.RGBA, gx, gy int) {
	if b := img.Bounds(); !b.Empty() {
		c.markKind(image.Rect(gx, gy, gx+b.Dx(), gy+b.Dy()))
	}
	iw, ih := img.Rect.Dx(), img.Rect.Dy()
	r := c.clampRect(image.Rect(gx, gy, gx+iw, gy+ih))
	if r.Empty() {
		return
	}
	// img приходит из другого мира (COLR/эмодзи-глиф в emoji.go/colrv1.go,
	// временный буфер тени в shadow.go) — всегда в "логическом" R,G,B,A,
	// без понятия о формате back. Здесь единственная точка, где такие чужие
	// пиксели копируются в back, поэтому здесь и переставляем R/B при
	// FormatBGRX — по одному разу на пиксель, а не через отдельный проход.
	swap := c.format == FormatBGRX
	src := img.Pix
	sStride := img.Stride
	dst := c.back.Pix
	for yy := r.Min.Y; yy < r.Max.Y; yy++ {
		// Скруглённое отсечение: эмодзи в панели с кривыми углами обрезается
		// так же, как текст и заливки.
		lx, rx, ok := c.round.spanX(yy, r.Min.X, r.Max.X)
		if !ok {
			continue
		}
		sRow := (yy-gy)*sStride + (lx-gx)*4
		dOff := c.back.PixOffset(lx, yy)
		for xx := lx; xx < rx; xx++ {
			sa := uint32(src[sRow+3])
			if sa == 0 { // полностью прозрачный пиксель
				sRow += 4
				dOff += 4
				continue
			}
			sr, sg, sb := src[sRow+0], src[sRow+1], src[sRow+2]
			if swap {
				sr, sb = sb, sr
			}
			inv := 255 - sa
			p := dst[dOff : dOff+4 : dOff+4]
			p[0] = uint8(uint32(sr) + uint32(p[0])*inv/255)
			p[1] = uint8(uint32(sg) + uint32(p[1])*inv/255)
			p[2] = uint8(uint32(sb) + uint32(p[2])*inv/255)
			p[3] = uint8(sa + uint32(p[3])*inv/255)
			sRow += 4
			dOff += 4
		}
	}
}

// runeAdvance возвращает ширину руны в выбранном (с учётом fallback) шрифте.
// Для непокрытых рун использует ширину пробела (соответствует drawTextWithFont).
func (c *Canvas) runeAdvance(fc *FontCache, r rune, sizePt float64) fixed.Int26_6 {
	chosen, found := c.fcForRune(fc, r)
	face := chosen.Face(sizePt)
	a, ok := face.GlyphAdvance(r)
	if !ok || !found {
		if sp, ok2 := fc.Face(sizePt).GlyphAdvance(' '); ok2 {
			return sp
		}
	}
	return a
}

// measureWithFallback измеряет ширину строки с учётом fallback-шрифтов.
func (c *Canvas) measureWithFallback(fc *FontCache, text string, sizePt float64) int {
	// Сложный текст: ширина после шейпинга (лигатуры меняют ширину строки).
	if needsShaping(text) {
		if l := c.shaper.layout(fc, c.fallbacks, text, sizePt); l != nil {
			return l.width.Round()
		}
	}
	if len(c.fallbacks) == 0 {
		return fc.Measure(text, sizePt)
	}
	var w fixed.Int26_6
	for _, r := range text {
		w += c.runeAdvance(fc, r, sizePt)
	}
	return w.Round()
}

// drawShapedText рисует строку через шейпер: глифы в видимом порядке,
// маски растеризуются по GID и кэшируются. baseline — Y базовой линии.
// false — шейпинг недоступен, вызывающий код рисует per-rune путём.
func (c *Canvas) drawShapedText(fc *FontCache, text string, x, baseline int, sizePt float64, col color.RGBA) bool {
	l := c.shaper.layout(fc, c.fallbacks, text, sizePt)
	if l == nil {
		return false
	}
	sizePx := fixed.Int26_6(sizePt * fc.dpi / 72.0 * 64.0)
	pen := fixed.I(x)
	for _, g := range l.glyphs {
		// GID 0 = .notdef: руна не покрыта ни одним шрифтом — не рисуем
		// «тофу»-квадрат (философия BUG-2), но перо продвигаем.
		if g.gid == 0 {
			pen += g.adv
			continue
		}
		// Цветной эмодзи (COLR/CBDT): блиттится как RGBA, цветом текста НЕ
		// красится. Для обычных глифов colorGlyphFor сразу вернёт nil.
		if cg := c.shaper.colorGlyphFor(g.face, g.gid, sizePx, col); cg != nil && cg.img != nil {
			gx := (pen + g.xOff).Round() + cg.offX
			gy := baseline - g.yOff.Round() + cg.offY
			c.withMaskKind(output.RegionText, func() { c.drawColorGlyph(cg.img, gx, gy) })
			pen += g.adv
			continue
		}
		m := c.shaper.glyphMaskFor(g.face, g.gid, sizePx)
		if m.mask != nil {
			// Точка отрисовки: перо + XOffset; YOffset положителен вверх.
			gx := (pen + g.xOff).Round() + m.offX
			gy := baseline - g.yOff.Round() + m.offY
			c.withMaskKind(output.RegionText, func() { c.drawAlphaMask(m.mask, gx, gy, col) })
		}
		pen += g.adv
	}
	return true
}

// shapedRunePositions — накопленные ширины по логическим рунам для сложного
// текста: advance каждого кластера распределяется поровну между его рунами.
// Для RTL это ЛОГИЧЕСКИЕ позиции (для каретки TextInput), не визуальные.
func (c *Canvas) shapedRunePositions(fc *FontCache, text string, sizePt float64) []int {
	l := c.shaper.layout(fc, c.fallbacks, text, sizePt)
	if l == nil {
		return nil
	}
	runes := []rune(text)
	n := len(runes)
	// Суммарный advance по кластерам (cluster = индекс первой руны кластера).
	clusterAdv := make(map[int]fixed.Int26_6, n)
	for _, g := range l.glyphs {
		clusterAdv[g.cluster] += g.adv
	}
	// Границы кластеров в логическом порядке.
	starts := make([]int, 0, len(clusterAdv))
	for s := range clusterAdv {
		starts = append(starts, s)
	}
	sort.Ints(starts)

	pos := make([]int, n+1)
	var acc fixed.Int26_6
	for ci, s := range starts {
		end := n
		if ci+1 < len(starts) {
			end = starts[ci+1]
		}
		adv := clusterAdv[s]
		cnt := end - s
		if cnt <= 0 {
			continue
		}
		for k := 1; k <= cnt; k++ {
			pos[s+k] = (acc + adv*fixed.Int26_6(k)/fixed.Int26_6(cnt)).Round()
		}
		acc += adv
	}
	return pos
}

// MeasureText возвращает ширину строки в ЛОГИЧЕСКИХ пикселях (шрифт по
// умолчанию, sizePt). Внутреннее измерение физическое (DPI × scale),
// результат приводится к логическим с округлением вверх.
func (c *Canvas) MeasureText(text string, sizePt float64) int {
	return c.toLogicalLen(c.measureWithFallback(c.fontCache, text, sizePt))
}

// MeasureTextFont возвращает ширину строки именованным шрифтом (логические px).
func (c *Canvas) MeasureTextFont(text string, sizePt float64, fontName string) int {
	return c.toLogicalLen(c.measureWithFallback(c.fontFor(fontName), text, sizePt))
}

// toLogicalLen переводит физическую длину в логическую (округление вверх:
// зарезервированное по измерению место гарантированно вмещает текст).
func (c *Canvas) toLogicalLen(px int) int {
	if c.scale == 1 {
		return px
	}
	return int(math.Ceil(float64(px) / c.scale))
}

// MeasureRunePositions возвращает накопленную ширину после каждого символа
// (в ЛОГИЧЕСКИХ пикселях).
// Результат: len(runes)+1 элементов; positions[0]==0, positions[n] — ширина text[:n].
// Для сложного текста (шейпинг) позиции считаются по кластерам в ЛОГИЧЕСКОМ
// порядке рун — визуальное позиционирование каретки в RTL не поддержано (v1).
func (c *Canvas) MeasureRunePositions(text string, sizePt float64) []int {
	pos := c.measureRunePositionsPx(text, sizePt)
	if c.scale != 1 {
		for i, p := range pos {
			pos[i] = int(math.Round(float64(p) / c.scale))
		}
	}
	return pos
}

// measureRunePositionsPx — позиции рун в физических пикселях.
func (c *Canvas) measureRunePositionsPx(text string, sizePt float64) []int {
	return c.measureRunePositionsPxFont(c.fontCache, text, sizePt)
}

// MeasureRunePositionsFont — как MeasureRunePositions, но ИМЕНОВАННЫМ шрифтом.
//
// Каретка и выделение в моноширинном тексте считаются по нему же: раскладка,
// посчитанная шрифтом по умолчанию, разъезжается с нарисованной, как только
// именованный шрифт действительно зарегистрирован. Пустое имя — шрифт по
// умолчанию.
func (c *Canvas) MeasureRunePositionsFont(text string, sizePt float64, fontName string) []int {
	pos := c.measureRunePositionsPxFont(c.fontFor(fontName), text, sizePt)
	if c.scale != 1 {
		for i, p := range pos {
			pos[i] = int(math.Round(float64(p) / c.scale))
		}
	}
	return pos
}

func (c *Canvas) measureRunePositionsPxFont(fc *FontCache, text string, sizePt float64) []int {
	if needsShaping(text) {
		if pos := c.shapedRunePositions(fc, text, sizePt); pos != nil {
			return pos
		}
	}
	if len(c.fallbacks) == 0 {
		return fc.MeasureRunes(text, sizePt)
	}
	runes := []rune(text)
	pos := make([]int, len(runes)+1)
	var w fixed.Int26_6
	for i, r := range runes {
		w += c.runeAdvance(fc, r, sizePt)
		pos[i+1] = w.Round()
	}
	return pos
}

// SetPixel устанавливает цвет одного ЛОГИЧЕСКОГО пикселя (при HiDPI —
// блок физических пикселей соответствующего размера). Учитывает clip.
func (c *Canvas) SetPixel(x, y int, col color.RGBA) {
	col = c.enc(col) // setPixelPx/fillRectPx col не кодируют — см. их контракт
	if c.scale != 1 {
		c.fillRectPx(image.Rect(c.sx(x), c.sx(y), c.sx(x+1), c.sx(y+1)), col, false)
		return
	}
	c.setPixelPx(x, y, col)
}

// setPixelPx — один ФИЗИЧЕСКИЙ пиксель (внутренний примитив). col приходит
// уже закодированным (см. контракт fillRectPx) — вызывающие кодируют сами.
func (c *Canvas) setPixelPx(x, y int, col color.RGBA) {
	if c.hasClip {
		if !image.Pt(x, y).In(c.clip) {
			return
		}
	}
	if !c.round.contains(x, y) {
		return
	}
	if x >= 0 && x < c.W && y >= 0 && y < c.H {
		c.back.SetRGBA(x, y, col)
	}
}

// DrawHLine рисует горизонтальную линию длиной length логических пикселей.
func (c *Canvas) DrawHLine(x, y, length int, col color.RGBA) {
	c.FillRect(x, y, length, 1, col)
}

// DrawVLine рисует вертикальную линию длиной length логических пикселей.
func (c *Canvas) DrawVLine(x, y, length int, col color.RGBA) {
	c.FillRect(x, y, 1, length, col)
}

// DrawImage рисует изображение в (x, y); логический размер = размеру
// картинки в пикселях (при HiDPI изображение растягивается на scale).
func (c *Canvas) DrawImage(src image.Image, x, y int) {
	if b := src.Bounds(); !b.Empty() {
		c.markImage(c.sRect(image.Rect(x, y, x+b.Dx(), y+b.Dy())))
	}
	if c.scale != 1 {
		c.DrawImageScaled(src, x, y, src.Bounds().Dx(), src.Bounds().Dy())
		return
	}
	r := c.clampRect(image.Rect(x, y, x+src.Bounds().Dx(), y+src.Bounds().Dy()))
	if r.Empty() {
		return
	}
	offset := src.Bounds().Min.Add(image.Pt(r.Min.X-x, r.Min.Y-y))
	c.blitOver(r, src, offset)
}

// blitOver копирует src в back операцией Over в физической области r
// (offset — точка src, соответствующая r.Min). Общая точка для
// DrawImage/DrawImageScaled — единственное место, где в back попадают
// пиксели чужого image.Image напрямую, минуя markSolid/drawAlphaMask.
//
// FormatRGBA: тот же stdraw.Draw, что был здесь и раньше, — байт в байт
// прежнее поведение, никакого нового кода на этом пути не выполняется.
//
// FormatBGRX: stdraw.Draw не знает про наш порядок байт, а переписывать его
// арифметику Over вручную — рискованно разойтись на пару значений округления
// с оригиналом (тест ЗАДАЧИ 1 требует побитового совпадения с RGBA-рендером
// той же сцены). Поэтому вместо этого прогоняем ТОТ ЖЕ stdraw.Draw во
// временном RGBA-буфере: сначала "расколдовываем" в него текущее содержимое
// затрагиваемой области back (снова R↔B), затем — обычный Over, затем
// результат переносим назад с той же перестановкой. Дороже прямой записи,
// зато арифметика смешивания гарантированно та же, что и в FormatRGBA.
func (c *Canvas) blitOver(r image.Rectangle, src image.Image, offset image.Point) {
	if c.format != FormatBGRX {
		stdraw.Draw(c.back, r, src, offset, stdraw.Over)
		return
	}
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		return
	}
	tmp := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		bOff := c.back.PixOffset(r.Min.X, r.Min.Y+y)
		tOff := tmp.PixOffset(0, y)
		row := c.back.Pix[bOff : bOff+w*4]
		trow := tmp.Pix[tOff : tOff+w*4]
		for i := 0; i < len(row); i += 4 {
			trow[i+0], trow[i+1], trow[i+2], trow[i+3] = row[i+2], row[i+1], row[i+0], row[i+3]
		}
	}
	stdraw.Draw(tmp, tmp.Bounds(), src, offset, stdraw.Over)
	for y := 0; y < h; y++ {
		bOff := c.back.PixOffset(r.Min.X, r.Min.Y+y)
		tOff := tmp.PixOffset(0, y)
		row := c.back.Pix[bOff : bOff+w*4]
		trow := tmp.Pix[tOff : tOff+w*4]
		for i := 0; i < len(row); i += 4 {
			row[i+0], row[i+1], row[i+2], row[i+3] = trow[i+2], trow[i+1], trow[i+0], trow[i+3]
		}
	}
}

// ─── Кэш масштабирования ────────────────────────────────────────────────────

const (
	maxScaledEntries = 32       // записей в кэше DrawImageScaled
	maxScaledPixels  = 16 << 20 // суммарный бюджет пикселей кэша
)

// scaledKey — ключ кэша: идентичность источника плюс запрошенный размер.
type scaledKey struct {
	src  uintptr
	sb   image.Rectangle
	w, h int
}

type scaledEntry struct {
	img  *image.RGBA
	used uint64 // такт последнего обращения (LRU)
}

// DrawImageScaled рисует изображение масштабированным до (w × h) логических
// пикселей в позицию (x, y).
// Кэш — по идентичности источника; мутировали пиксели — InvalidateImageCache.
func (c *Canvas) DrawImageScaled(src image.Image, x, y, w, h int) {
	if w > 0 && h > 0 {
		c.markImage(c.sRect(image.Rect(x, y, x+w, y+h)))
	}
	px, py := c.sx(x), c.sx(y)
	pw, ph := c.sl(x, w), c.sl(y, h)
	dstRect := c.clampRect(image.Rect(px, py, px+pw, py+ph))
	if dstRect.Empty() {
		return
	}
	tmp := c.scaledFor(src, pw, ph)
	offset := image.Pt(dstRect.Min.X-px, dstRect.Min.Y-py)
	c.blitOver(dstRect, tmp, offset)
}

// scaledFor возвращает src, масштабированный до pw×ph физических пикселей.
func (c *Canvas) scaledFor(src image.Image, pw, ph int) *image.RGBA {
	need := image.Rect(0, 0, pw, ph)
	key, cacheable := scaledCacheKey(src, pw, ph)
	if cacheable {
		if ent, ok := c.scaledCache[key]; ok {
			c.scaledClock++
			ent.used = c.scaledClock
			return ent.img
		}
	} else {
		// Источник без стабильной идентичности — общий временный буфер.
		tmp := c.scaleTmp
		if tmp == nil || tmp.Bounds() != need {
			tmp = image.NewRGBA(need)
			c.scaleTmp = tmp
		} else {
			for i := range tmp.Pix {
				tmp.Pix[i] = 0
			}
		}
		draw.BiLinear.Scale(tmp, need, src, src.Bounds(), stdraw.Src, nil)
		return tmp
	}
	img := image.NewRGBA(need)
	draw.BiLinear.Scale(img, need, src, src.Bounds(), stdraw.Src, nil)
	c.putScaled(key, img, pw*ph)
	return img
}

// scaledCacheKey строит ключ кэша; false — источник кэшировать нельзя.
func scaledCacheKey(src image.Image, w, h int) (scaledKey, bool) {
	v := reflect.ValueOf(src)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return scaledKey{}, false
	}
	return scaledKey{src: v.Pointer(), sb: src.Bounds(), w: w, h: h}, true
}

// putScaled кладёт запись в кэш, вытесняя давние при переполнении.
func (c *Canvas) putScaled(key scaledKey, img *image.RGBA, pixels int) {
	if pixels > maxScaledPixels {
		return
	}
	if c.scaledCache == nil {
		c.scaledCache = make(map[scaledKey]*scaledEntry, maxScaledEntries)
	}
	for len(c.scaledCache) >= maxScaledEntries || c.scaledPixels+pixels > maxScaledPixels {
		if !c.evictScaled() {
			break
		}
	}
	c.scaledClock++
	c.scaledCache[key] = &scaledEntry{img: img, used: c.scaledClock}
	c.scaledPixels += pixels
}

// evictScaled выбрасывает самую давно не использованную запись.
func (c *Canvas) evictScaled() bool {
	var (
		oldKey scaledKey
		oldEnt *scaledEntry
	)
	for k, ent := range c.scaledCache {
		if oldEnt == nil || ent.used < oldEnt.used {
			oldKey, oldEnt = k, ent
		}
	}
	if oldEnt == nil {
		return false
	}
	delete(c.scaledCache, oldKey)
	c.scaledPixels -= oldEnt.img.Rect.Dx() * oldEnt.img.Rect.Dy()
	return true
}

// InvalidateImageCache сбрасывает кэш масштабирования для src (nil — целиком).
// Вызывать после мутации пикселей картинки на месте, из потока отрисовки.
func (c *Canvas) InvalidateImageCache(src image.Image) {
	if src == nil {
		c.scaledCache = nil
		c.scaledPixels = 0
		return
	}
	v := reflect.ValueOf(src)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return
	}
	ptr := v.Pointer()
	for k, ent := range c.scaledCache {
		if k.src == ptr {
			delete(c.scaledCache, k)
			c.scaledPixels -= ent.img.Rect.Dx() * ent.img.Rect.Dy()
		}
	}
}

// Snapshot возвращает КОПИЮ прямоугольной области уже отрисованного back-буфера
// как самостоятельный *image.RGBA. Прямоугольник r задаётся в ЛОГИЧЕСКИХ
// координатах холста (как остальной DrawContext) и клипится по его границам;
// возвращаемое изображение — в ФИЗИЧЕСКИХ пикселях (back-буфер физический) с
// началом координат (0,0). Пустое пересечение с холстом → nil.
//
// Копия независима от буфера (последующий рендер её не меняет), поэтому снимок
// можно хранить между кадрами — используется «призраком» drag&dock в
// widget.DockManager (снимок панели следует за курсором). Пиксели —
// premultiplied RGBA, как и весь back: снимок можно блиттить обратно через
// DrawImage/DrawImageScaled (Over) без преобразований.
func (c *Canvas) Snapshot(r image.Rectangle) *image.RGBA {
	pr := c.sRect(r).Intersect(c.back.Bounds())
	if pr.Empty() {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, pr.Dx(), pr.Dy()))
	rowBytes := pr.Dx() * 4
	for y := 0; y < pr.Dy(); y++ {
		src := c.back.PixOffset(pr.Min.X, pr.Min.Y+y)
		d := dst.PixOffset(0, y)
		copy(dst.Pix[d:d+rowBytes], c.back.Pix[src:src+rowBytes])
	}
	return dst
}

// ─── Tile diffing ───────────────────────────────────────────────────────────

// diffAndSync сравнивает back с front по тайлам и возвращает изменившиеся.
func (c *Canvas) diffAndSync() []output.DirtyTile {
	return c.diffTiles(0, 0, c.tilesX-1, c.tilesY-1)
}

// diffAndSyncIn — как diffAndSync, но сравнивает только тайлы, пересекающие
// region (damage-область из InvalidateRect). Тайлы вне region не сравниваются
// и НЕ синхронизируются — контракт: вызывающий заявил все изменённые области.
func (c *Canvas) diffAndSyncIn(region image.Rectangle) []output.DirtyTile {
	region = region.Intersect(image.Rect(0, 0, c.W, c.H))
	if region.Empty() {
		return nil
	}
	ts := output.TileSize
	tx0 := region.Min.X / ts
	ty0 := region.Min.Y / ts
	tx1 := (region.Max.X - 1) / ts
	ty1 := (region.Max.Y - 1) / ts
	return c.diffTiles(tx0, ty0, tx1, ty1)
}

// diffAndSyncInRects — diff по НЕСКОЛЬКИМ областям сразу.
//
// Области перебираются по одной, и общий тайл двух соседних областей не
// попадёт в выдачу дважды: diffTiles синхронизирует front сразу после
// извлечения тайла, поэтому при втором сравнении тайл уже равен и
// отбрасывается. Отдельная дедупликация не нужна.
func (c *Canvas) diffAndSyncInRects(regions []image.Rectangle) []output.DirtyTile {
	switch len(regions) {
	case 0:
		return nil
	case 1:
		return c.diffAndSyncIn(regions[0])
	}
	var out []output.DirtyTile
	for _, r := range regions {
		out = append(out, c.diffAndSyncIn(r)...)
	}
	return out
}

// diffTiles сравнивает тайлы в диапазоне индексов [tx0..tx1]×[ty0..ty1].
// При достаточном объёме работа распараллеливается по рядам тайлов: каждый
// воркер обрабатывает непересекающийся диапазон памяти (сравнение, извлечение
// и синхронизация front — всё в границах своих рядов), поэтому гонок нет.
func (c *Canvas) diffTiles(tx0, ty0, tx1, ty1 int) []output.DirtyTile {
	if ty1 >= c.tilesY {
		ty1 = c.tilesY - 1
	}
	if tx1 >= c.tilesX {
		tx1 = c.tilesX - 1
	}
	if ty0 > ty1 || tx0 > tx1 {
		return nil
	}

	rows := ty1 - ty0 + 1
	total := rows * (tx1 - tx0 + 1)
	workers := runtime.GOMAXPROCS(0)
	if workers > rows {
		workers = rows
	}
	// Порог: параллелить только заметный объём — сравнение одного тайла
	// это ~16 КБ memcmp, на мелких диффах горутины дороже выигрыша.
	if workers <= 1 || total < 64 {
		return c.diffTileRows(tx0, ty0, tx1, ty1)
	}

	results := make([][]output.DirtyTile, workers)
	var wg sync.WaitGroup
	chunk := (rows + workers - 1) / workers
	for i := 0; i < workers; i++ {
		r0 := ty0 + i*chunk
		r1 := r0 + chunk - 1
		if r1 > ty1 {
			r1 = ty1
		}
		if r0 > r1 {
			break
		}
		wg.Add(1)
		go func(idx, a, b int) {
			defer wg.Done()
			results[idx] = c.diffTileRows(tx0, a, tx1, b)
		}(i, r0, r1)
	}
	wg.Wait()

	var tiles []output.DirtyTile
	for _, part := range results {
		tiles = append(tiles, part...)
	}
	return tiles
}

// diffTileRows — последовательный diff тайлов в диапазоне рядов [ty0..ty1].
//
// Два прохода: сначала сравнение находит изменившиеся тайлы, затем их данные
// извлекаются в ОДИН слэб точного размера, а Data каждого тайла —
// трёхиндексный срез в него. Раньше на каждый тайл выделялся собственный
// буфер — при активной перерисовке это был крупнейший источник мусора движка
// (десятки МБ/с в профиле); слэб через append тоже не годился — рост с
// удвоением копирует данные и оставляет позади до слэба мусора за кадр.
func (c *Canvas) diffTileRows(tx0, ty0, tx1, ty1 int) []output.DirtyTile {
	ts := output.TileSize

	// Проход 1: какие тайлы изменились и сколько байт им нужно.
	var tiles []output.DirtyTile
	total := 0
	for ty := ty0; ty <= ty1; ty++ {
		for tx := tx0; tx <= tx1; tx++ {
			px := tx * ts
			py := ty * ts
			pw := min(ts, c.W-px)
			ph := min(ts, c.H-py)
			if !c.tilesEqual(px, py, pw, ph) {
				tiles = append(tiles, output.DirtyTile{X: px, Y: py, W: pw, H: ph})
				total += pw * ph * 4
			}
		}
	}
	if len(tiles) == 0 {
		return nil
	}

	// Проход 2: одна аллокация под все данные, извлечение и синхронизация.
	slab := make([]byte, 0, total)
	for i := range tiles {
		t := &tiles[i]
		start := len(slab)
		slab = c.appendTile(slab, t.X, t.Y, t.W, t.H)
		t.Data = slab[start:len(slab):len(slab)]
		c.syncTile(t.X, t.Y, t.W, t.H)
	}
	return tiles
}

func (c *Canvas) tilesEqual(x, y, w, h int) bool {
	rowBytes := w * 4
	for row := 0; row < h; row++ {
		fOff := c.front.PixOffset(x, y+row)
		bOff := c.back.PixOffset(x, y+row)
		if !bytes.Equal(
			c.front.Pix[fOff:fOff+rowBytes],
			c.back.Pix[bOff:bOff+rowBytes],
		) {
			return false
		}
	}
	return true
}

// appendTile дописывает пиксели тайла (x,y,w,h) из back-буфера в конец slab
// и возвращает выросший слэб. См. diffTileRows: одна аллокация на воркера
// вместо буфера на каждый тайл.
func (c *Canvas) appendTile(slab []byte, x, y, w, h int) []byte {
	rowBytes := w * 4
	for row := 0; row < h; row++ {
		src := c.back.PixOffset(x, y+row)
		slab = append(slab, c.back.Pix[src:src+rowBytes]...)
	}
	return slab
}

func (c *Canvas) syncTile(x, y, w, h int) {
	rowBytes := w * 4
	for row := 0; row < h; row++ {
		src := c.back.PixOffset(x, y+row)
		dst := c.front.PixOffset(x, y+row)
		copy(c.front.Pix[dst:dst+rowBytes], c.back.Pix[src:src+rowBytes])
	}
}

// OcclusionEnabled реализует widget.OcclusionScope: вычитать ли перекрытое.
func (c *Canvas) OcclusionEnabled() bool { return c.occlusionOn.Load() }
