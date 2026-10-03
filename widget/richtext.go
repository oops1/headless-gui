package widget

// richtext.go — виджет RichText: показ форматированного текста (без правки).
//
// Зачем он нужен. Label — один шрифт и один цвет на всю надпись, TextBox — один
// кегль на всё поле. Приложениям же нужен текст «с оформлением»: предпросмотр
// Markdown, справка, подписи с выделенным словом, журналы с цветными уровнями.
// Виджет принимает готовые абзацы из ранов (см. RichRun) и ничего не знает про
// исходную разметку.
//
// Устройство. Всё, что можно посчитать без окна, лежит в чистых файлах:
// richtext_layout.go (перенос, высота строки, базовая линия, выравнивание,
// попадание), richtext_html.go (выделение → HTML), richtext_model.go (абзацы,
// документ как строка рун). Здесь — состояние, замок, кэш раскладки и
// отрисовка; события мыши и клавиатуры — в richtext_input.go.
//
// Потоки. Отрисовка идёт из потока кадра, события мыши и вызовы приложения —
// из других горутин. Всё изменяемое состояние (содержимое, выделение,
// прокрутка, кэш раскладки) лежит под t.mu. Раскладка после построения
// неизменяема, поэтому Draw берёт под замком только указатель на неё и
// числа, а рисует уже без замка: приложение, дописывающее журнал, не ждёт
// кадра, а кадр — приложения.

import (
	"image"
	"image/color"
	"strings"
	"sync"
)

const (
	// richBarW — ширина зоны вертикальной полосы прокрутки.
	richBarW = 10
	// richDefaultPad — внутренний отступ по умолчанию.
	richDefaultPad = 4
	// richSpill — хвост выделения за концом строки: отметка «выделен и перевод
	// строки», как у TextBox.
	richSpill = 4
)

// RichText — показ форматированного текста: абзацы из ранов с разными
// шрифтами, кеглями, цветами, ссылками. Текст нельзя править, но можно
// выделять мышью и копировать (Ctrl+C кладёт в буфер и простой текст, и HTML,
// так что в Word вставится с оформлением).
//
// Если содержимое выше виджета, справа появляется полоса прокрутки; колесо,
// PgUp/PgDn, стрелки и Home/End листают текст.
type RichText struct {
	Base

	mu sync.Mutex

	// Содержимое. doc — то же самое одной строкой (абзацы через '\n'): по
	// смещениям в ней живут выделение, копирование и скринридер.
	paras []RichParagraph
	doc   *richDoc
	// rev растёт при каждом изменении содержимого: по нему кэш раскладки
	// узнаёт, что текст стал другим.
	rev uint64

	// measurer — измеритель шрифтов; в тестах подменяется на предсказуемый.
	measurer richMeasurer

	// Кэш раскладки: одна последняя, вместе с тем, для чего она посчитана.
	lay    *richLayout
	layKey richLayoutKey
	barOn  bool // по высоте нужна полоса прокрутки

	scrollY    int
	scrollFrac float64 // субпиксельный остаток плавной прокрутки

	// Выделение и каретка в рунах документа. Каретка — selCaret, она всегда
	// есть и является подвижным («активным») концом выделения; якорь —
	// selAnchor, неподвижный конец. Выделение — [min, max) этой пары; anchor < 0
	// (или равный каретке) — выделения нет, и тогда якорь «совпадает» с кареткой.
	// Одна пара вместо двух отдельных состояний: выделение мышью и клавиатурой
	// (Shift+стрелки) и положение каретки не могут разойтись.
	selAnchor, selCaret int
	dragging            bool
	pressLink           string // ссылка под курсором в момент нажатия

	// Состояние каретки, не сводимое к смещению (см. richtext_caret.go).
	// caretEOL — каретка на стыке мягко перенесённых строк: смещение одно и то
	// же («конец строки» и «начало следующей»), а рисовать надо на одной из
	// них. wantX/wantXOk — «желаемый X» вертикальной навигации.
	// caretStamp — момент последнего перемещения (мс): мигание начинается с
	// показанной каретки. caretPhase/caretPhaseKnown — фаза, нарисованная
	// последним Draw (по ней NeedsAnimation решает, нужен ли кадр).
	caretEOL        bool
	wantX           int
	wantXOk         bool
	caretStamp      int64
	caretPhase      bool
	caretPhaseKnown bool

	// Полоса прокрутки: перетаскивание ползунка и подсветка под курсором.
	barDrag, barHover bool
	barGrab           int

	capMgr  CaptureManager
	clicks  clickSeries
	focused bool

	// Оформление. Нулевая альфа цвета — «взять из темы» не умеет: цвета
	// задаются конструктором и ApplyTheme; меняйте их после и вызывайте
	// Invalidate.
	Background  color.RGBA // фон; с нулевой альфой фона нет
	TextColor   color.RGBA // цвет текста ранов без собственного цвета
	LinkColor   color.RGBA // цвет ссылок (по умолчанию — акцент темы)
	SelColor    color.RGBA // фон выделения
	FocusBorder color.RGBA // рамка при фокусе
	CaretColor  color.RGBA // цвет каретки (по умолчанию — каретка полей ввода темы)

	// ShowCaret — рисовать мигающую каретку (в фокусе). По умолчанию false:
	// виджет — средство ПРОСМОТРА, мигающая черта в справке или журнале только
	// отвлекала бы, и кадр на каждое мигание не нужен. Каретка при этом всё
	// равно существует и ходит: Влево/Вправо, Ctrl+стрелки, Shift+стрелки и
	// Ctrl+Home/End двигают её и выделение (это нужно и клавиатурному
	// выделению, и скринридеру, у которого есть AccessCaret). Отличие только
	// в простых Вверх/Вниз/PgUp/PgDn/Home/End: в режиме просмотра они, как
	// раньше, листают текст, а в режиме с кареткой двигают её. Редактору
	// (этап 2) нужно true.
	ShowCaret bool

	PaddingX, PaddingY int

	// FontSize и FontName — кегль (pt) и шрифт для ранов, у которых свои не
	// заданы. 0 и "" — кегль и шрифт по умолчанию.
	FontSize float64
	FontName string

	// LineSpacing — добавка к высоте строки, пиксели. Высота строки — это
	// «подъём + спуск» самого высокого рана; без добавки строки у многих
	// шрифтов слипаются.
	LineSpacing int

	// LinkUnderline — подчёркивать ссылки. Цвет одного не годится: человек с
	// нарушением цветовосприятия не отличит ссылку от текста.
	LinkUnderline bool

	// OnLinkClick вызывается по щелчку на ссылке (без протяжки): url — Link
	// рана. Виджет сам ничего не открывает — это решает приложение (в пакете
	// window есть OpenURL). Вызывается без замков виджета, в потоке событий.
	OnLinkClick func(url string)
}

// richLayoutKey — то, для чего посчитана раскладка. Совпадение ключа значит,
// что кэш годится; любая из частей изменилась — считаем заново.
type richLayoutKey struct {
	w, h       int // размер виджета
	rev        uint64
	metricsRev uint64 // смена DPI меняет ширины текста
	font       string
	size       float64
	gap        int
	padX, padY int
}

// NewRichText создаёт пустой виджет с цветами активной темы.
func NewRichText() *RichText {
	t := &RichText{
		measurer:      richUIMeasurer{},
		doc:           richBuildDoc(nil),
		selAnchor:     -1,
		PaddingX:      richDefaultPad,
		PaddingY:      richDefaultPad,
		LineSpacing:   2,
		LinkUnderline: true,
	}
	t.applyColors(&win10)
	return t
}

// applyColors берёт цвета из темы.
func (t *RichText) applyColors(th *Theme) {
	t.TextColor = th.LabelText
	t.LinkColor = th.Accent
	t.FocusBorder = th.InputFocus
	t.CaretColor = th.InputCaret
	if th.TextSelectionBG.A != 0 {
		t.SelColor = th.TextSelectionBG
	} else {
		t.SelColor = premulAlpha(th.Accent, 110)
	}
}

// ApplyTheme обновляет цвета по теме.
func (t *RichText) ApplyTheme(th *Theme) {
	t.applyColors(th)
}

// ─── Содержимое ─────────────────────────────────────────────────────────────

// SetParagraphs заменяет содержимое. Срезы копируются: после вызова приложение
// вправе менять свои.
//
// Выделение сбрасывается — его смещения указывали бы в прежний текст.
// Прокрутка сохраняется (зажимается по новой высоте при следующей раскладке),
// чтобы живой предпросмотр не прыгал наверх при каждой правке документа.
func (t *RichText) SetParagraphs(paras []RichParagraph) {
	cp := richCopyParagraphs(paras)
	doc := richBuildDoc(cp)
	t.mu.Lock()
	t.paras, t.doc = cp, doc
	t.rev++
	t.selAnchor, t.selCaret = -1, 0
	t.caretEOL, t.wantXOk = false, false
	t.dragging = false
	t.mu.Unlock()
	t.Invalidate()
}

// Paragraphs возвращает копию содержимого.
func (t *RichText) Paragraphs() []RichParagraph {
	t.mu.Lock()
	defer t.mu.Unlock()
	return richCopyParagraphs(t.paras)
}

// SetText заменяет содержимое простым текстом без оформления: строка — абзац.
func (t *RichText) SetText(text string) {
	lines := strings.Split(richNormalizeText(text), "\n")
	paras := make([]RichParagraph, len(lines))
	for i, ln := range lines {
		paras[i] = RichParagraph{Runs: []RichRun{{Text: ln}}}
	}
	t.SetParagraphs(paras)
}

// Text возвращает содержимое простым текстом: абзацы через '\n'.
func (t *RichText) Text() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.doc.runes)
}

// Clear удаляет всё содержимое.
func (t *RichText) Clear() { t.SetParagraphs(nil) }

// AppendParagraph добавляет абзац в конец.
//
// Документ дополняется на месте, без пересборки: журнал, в который строки
// дописывают по одной, иначе стоил бы времени, растущего с квадратом числа
// строк. Выделение и прокрутка сохраняются — смещения прежнего текста не
// сдвигаются.
func (t *RichText) AppendParagraph(p RichParagraph) {
	cp := richCopyParagraphs([]RichParagraph{p})[0]
	t.mu.Lock()
	if len(t.paras) > 0 {
		t.doc.runes = append(t.doc.runes, '\n')
	}
	t.doc.paraStart = append(t.doc.paraStart, len(t.doc.runes))
	for _, r := range cp.Runs {
		t.doc.runes = append(t.doc.runes, []rune(r.Text)...)
	}
	t.paras = append(t.paras, cp)
	t.rev++
	t.mu.Unlock()
	t.Invalidate()
}

// AppendRun добавляет ран в конец последнего абзаца; если абзацев нет —
// создаёт его. Так простой случай — «текст, несколько слов жирным» — не
// требует собирать RichParagraph вручную.
func (t *RichText) AppendRun(r RichRun) {
	r.Text = richNormalizeText(r.Text)
	t.mu.Lock()
	if len(t.paras) == 0 {
		t.paras = append(t.paras, RichParagraph{})
		t.doc.paraStart = append(t.doc.paraStart, 0)
	}
	last := len(t.paras) - 1
	t.paras[last].Runs = append(t.paras[last].Runs, r)
	t.doc.runes = append(t.doc.runes, []rune(r.Text)...)
	t.rev++
	t.mu.Unlock()
	t.Invalidate()
}

// ─── Раскладка и прокрутка ──────────────────────────────────────────────────

// layoutLocked возвращает раскладку для текущих размеров, строя её при
// необходимости; заодно зажимает прокрутку. Вызывать под t.mu.
//
// Полоса прокрутки отнимает ширину у текста, а нужна она тогда, когда текст
// не помещается по высоте — то есть зависит от ширины. Цикл решается просто:
// сначала считаем на полной ширине; если не влезло по высоте, считаем на
// суженной. Сужение только удлиняет текст, так что полоса, однажды понадобившись,
// остаётся нужной, и колебания нет.
func (t *RichText) layoutLocked() *richLayout {
	b := t.Base.Bounds()
	key := richLayoutKey{
		w: b.Dx(), h: b.Dy(), rev: t.rev, metricsRev: TextMetricsRev(),
		font: t.FontName, size: t.FontSize, gap: t.LineSpacing,
		padX: t.PaddingX, padY: t.PaddingY,
	}
	if t.lay == nil || key != t.layKey {
		opts := richLayoutOpts{
			Width: b.Dx() - 2*t.PaddingX,
			Font:  t.FontName, Size: fontSizeOrDefault(t.FontSize),
			LineGap: t.LineSpacing, M: t.measurer,
		}
		lay := layoutRich(t.paras, opts)
		bar := b.Dy() > 0 && lay.Height+2*t.PaddingY > b.Dy()
		if bar {
			if opts.Width > 0 {
				if opts.Width -= richBarW; opts.Width < 1 {
					opts.Width = 1
				}
			}
			lay = layoutRich(t.paras, opts)
		}
		t.lay, t.layKey, t.barOn = lay, key, bar
	}
	t.scrollY = clampInt(t.scrollY, 0, t.maxScrollLocked())
	return t.lay
}

func clampInt(v, lo, hi int) int {
	if v > hi {
		v = hi
	}
	if v < lo {
		v = lo
	}
	return v
}

// maxScrollLocked — наибольшее смещение. Раскладка уже должна быть построена.
func (t *RichText) maxScrollLocked() int {
	if t.lay == nil {
		return 0
	}
	if m := t.lay.Height + 2*t.PaddingY - t.Base.Bounds().Dy(); m > 0 {
		return m
	}
	return 0
}

// setScrollLocked ставит прокрутку с зажимом; true — сместилась.
func (t *RichText) setScrollLocked(y int) bool {
	t.layoutLocked()
	y = clampInt(y, 0, t.maxScrollLocked())
	if y == t.scrollY {
		return false
	}
	t.scrollY = y
	return true
}

// ScrollY — текущее смещение прокрутки, пиксели.
func (t *RichText) ScrollY() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.layoutLocked()
	return t.scrollY
}

// SetScrollY прокручивает к смещению y (зажимается в допустимые пределы).
func (t *RichText) SetScrollY(y int) {
	t.mu.Lock()
	changed := t.setScrollLocked(y)
	t.mu.Unlock()
	if changed {
		t.Invalidate()
	}
}

// ScrollToEnd прокручивает к концу — для журнала, который растёт вниз.
func (t *RichText) ScrollToEnd() { t.SetScrollY(1 << 30) }

// ContentHeight — полная высота содержимого при текущей ширине виджета с
// учётом отступов. Нужна приложению, которое хочет подогнать высоту виджета
// под текст.
func (t *RichText) ContentHeight() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.layoutLocked().Height + 2*t.PaddingY
}

// barGeomLocked — геометрия полосы прокрутки: вся полоса, рабочая зона (в
// классической теме — между кнопками ▲▼) и ползунок. Пустой ползунок — полосы
// нет. Вызывать под t.mu, раскладка должна быть построена.
func (t *RichText) barGeomLocked() (track, work, thumb image.Rectangle) {
	if !t.barOn || t.lay == nil {
		return
	}
	b := t.Base.Bounds()
	track = image.Rect(b.Max.X-richBarW, b.Min.Y, b.Max.X, b.Max.Y)
	top, h := sbWorkArea(track, richBarW)
	work = image.Rect(track.Min.X, top, track.Max.X, top+h)
	thumb = richVThumb(work, t.scrollY, t.maxScrollLocked(), b.Dy(), t.lay.Height+2*t.PaddingY)
	return
}

// Вертикальная полоса считается теми же функциями, что горизонтальная в
// сравнении файлов и редакторе (hbarThumb и др.): математика одна — трек,
// доля видимого, положение. Чтобы не копировать её, прямоугольники
// транспонируются (X↔Y) на входе и на выходе.

func transposeRect(r image.Rectangle) image.Rectangle {
	return image.Rect(r.Min.Y, r.Min.X, r.Max.Y, r.Max.X)
}

// richVThumb — ползунок на рабочей зоне work.
func richVThumb(work image.Rectangle, scroll, maxScroll, viewH, contentH int) image.Rectangle {
	return transposeRect(hbarThumb(transposeRect(work), float64(scroll), float64(maxScroll),
		float64(viewH), float64(contentH)))
}

// ─── Отрисовка ──────────────────────────────────────────────────────────────

// Draw рисует только видимые строки.
func (t *RichText) Draw(ctx DrawContext) {
	b := t.Base.Bounds()
	if b.Empty() {
		return
	}

	// Всё, что кадр берёт из изменяемого состояния, снимается одним захватом
	// замка: раздельные чтения дали бы кадр, где текст уже уехал, а ползунок
	// ещё стоит на старом месте. Раскладка неизменяема — её можно держать
	// после разблокировки.
	t.mu.Lock()
	lay := t.layoutLocked()
	scrollY := t.scrollY
	selLo, selHi := t.selRangeLocked()
	focused := t.focused
	barOn := t.barOn
	caret, caretOn := t.drawCaretLocked(focused)
	track, _, thumb := t.barGeomLocked()
	barActive := t.barDrag || t.barHover
	m := t.measurer
	t.mu.Unlock()

	if t.Background.A > 0 {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.Background)
	}

	// Содержимое отсекается по виджету (минус полоса): текст уезжает под
	// верхний край плавно, а не обрывается на отступе. Сужение — пересечением
	// с внешней областью, и она возвращается: виджет может стоять в прокрутке.
	clip := b
	if barOn {
		clip.Max.X -= richBarW
	}
	restore := PushClip(ctx, clip)

	originX := b.Min.X + t.PaddingX
	originY := b.Min.Y + t.PaddingY - scrollY
	first, last := lay.visibleLines(scrollY-t.PaddingY, scrollY-t.PaddingY+b.Dy())
	for li := first; li < last; li++ {
		ln := &lay.Lines[li]
		ly := originY + ln.Y
		t.drawLine(ctx, m, lay, ln, originX, ly, selLo, selHi)
	}
	// Каретка — поверх букв, но в том же отсечении: уехавшая за край строка
	// не должна оставлять черту над полосой прокрутки.
	if caretOn {
		fillColor(ctx, caret.Min.X, caret.Min.Y, caret.Dx(), caret.Dy(), t.CaretColor)
	}
	restore()

	if barOn {
		t.drawBar(ctx, track, thumb, barActive)
	}
	if focused {
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.FocusBorder)
	}
	t.drawChildren(ctx)
	t.drawDisabledOverlay(ctx)
}

// fillColor заливает прямоугольник с учётом прозрачности цвета.
func fillColor(ctx DrawContext, x, y, w, h int, c color.RGBA) {
	if w <= 0 || h <= 0 || c.A == 0 {
		return
	}
	if c.A == 255 {
		ctx.FillRect(x, y, w, h, c)
	} else {
		ctx.FillRectAlpha(x, y, w, h, c)
	}
}

// drawLine рисует одну строку: фон ранов, выделение, буквы, линии.
// Порядок слоёв — как у редактора: выделение под буквами, но над фоном рана,
// иначе подсветка найденного скрывала бы выделение.
func (t *RichText) drawLine(ctx DrawContext, m richMeasurer, lay *richLayout, ln *richLine,
	originX, ly, selLo, selHi int) {

	// Фон ранов — полосой высотой в сам ран (подъём+спуск), а не во всю
	// строку: мелкое слово с подсветкой в строке с крупным не растягивается
	// на её высоту.
	for i := range ln.Segments {
		s := &ln.Segments[i]
		if s.Run.BG.A != 0 {
			fillColor(ctx, originX+s.X, ly+ln.Baseline-s.Ascent, s.W, s.Ascent+s.Descent, s.Run.BG)
		}
	}

	// Выделение — полосой на всю высоту строки.
	if selLo != selHi {
		x0, x1, ok := lay.selectionX(m, ln, selLo, selHi)
		spill := ln.HardEnd && selLo <= ln.End && selHi > ln.End
		endX := ln.X + ln.Width
		if !ok && spill {
			x0, x1, ok = endX, endX, true
		}
		if ok {
			if spill {
				x1 += richSpill
			}
			fillColor(ctx, originX+x0, ly, x1-x0, ln.Height, t.SelColor)
		}
	}

	for i := range ln.Segments {
		s := &ln.Segments[i]
		col := s.Run.Color
		if col.A == 0 {
			col = t.TextColor
			if s.Run.Link != "" {
				col = t.LinkColor
			}
		}
		sx := originX + s.X
		base := ly + ln.Baseline
		ctx.DrawTextFont(s.Run.Text, sx, base-s.Ascent, s.Run.Size, s.Run.Font, col)

		underline := s.Run.Underline || (s.Run.Link != "" && t.LinkUnderline)
		if (underline || s.Run.Strike) && s.W > 0 {
			ul, strike, thick := richDecoration(s.Ascent, s.Descent)
			if underline {
				ctx.FillRect(sx, base+ul, s.W, thick, col)
			}
			if s.Run.Strike {
				ctx.FillRect(sx, base+strike, s.W, thick, col)
			}
		}
	}
}

// drawBar рисует полосу прокрутки: в классической теме — с кнопками ▲▼, в
// остальных — тонкий ползунок поверх содержимого.
func (t *RichText) drawBar(ctx DrawContext, track, thumb image.Rectangle, active bool) {
	if thumb.Empty() {
		return
	}
	thumbCol := win10.ScrollThumbBG
	if active {
		thumbCol = win10.Accent
	}
	if st := currentStyle(); st.Classic3D {
		ctx.FillRect(track.Min.X, track.Min.Y, track.Dx(), track.Dy(), win10.ScrollTrackBG)
		drawClassicScrollbar(ctx, track, thumb, st, win10.ScrollThumbBG, win10.LabelText)
		return
	}
	ctx.FillRoundRect(thumb.Min.X+2, thumb.Min.Y+1, thumb.Dx()-4, thumb.Dy()-2, 3, thumbCol)
}
