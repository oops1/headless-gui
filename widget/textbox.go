package widget

import (
	"image"
	"image/color"
	"math"
	"sort"
	"sync"
	"time"
	"unicode/utf8"
)

// TextBox — многострочный текстовый редактор (WPF TextBox c
// AcceptsReturn="True"). Дополняет однострочный TextInput.
//
// Поддерживает:
//   - Перенос по словам (Wrap = TextWrapping="Wrap") либо горизонтальный скролл
//     с полосой прокрутки (колесо с Shift, горизонтальное колесо, перетаскивание)
//   - Вертикальный скролл: колесо мыши, PgUp/PgDn, тонкий индикатор
//   - Именованный шрифт (FontName), в том числе моноширинный для кода
//   - Табуляция: приём Tab как символа (AcceptTab) и табстопы (TabSize)
//   - Стили на диапазонах: подсветка синтаксиса, поиска, ошибок (SetStyler)
//   - Каретка, выделение мышью (drag), Shift+навигация, двойной клик — слово
//   - Стрелки, Ctrl+стрелки (по словам), Home/End, Ctrl+Home/End (документ)
//   - Ctrl+A/C/X/V, Ctrl+Z / Ctrl+Y (undo/redo)
//   - Enter — перевод строки; OnChange при каждом изменении
//   - Контекстное меню (Cut/Copy/Paste/Select All)
//   - Темизация (ApplyTheme), headless-ввод через engine.SendKeyEvent
//
// Компоновка текста (разбиение на строки) считается через MeasureUIText —
// работает и вне Draw (клавиатура/мышь), и в headless-режиме.
type TextBox struct {
	Base

	mu          sync.Mutex
	runes       []rune
	caret       int     // позиция вставки (индекс в runes)
	selAnchor   int     // якорь выделения (-1 = нет); выделение = [min,max)(anchor, caret)
	scrollY     int     // вертикальный сдвиг, px
	scrollX     int     // горизонтальный сдвиг, px (используется только при Wrap=false)
	scrollFrac  float64 // субпиксельный остаток плавной пиксельной прокрутки
	scrollFracX float64 // то же по горизонтали
	desiredX    int     // целевая X (px) для Up/Down; -1 = не задана

	// Кэш компоновки: границы строк для текущего текста и ширины.
	lines     []tbLine
	layoutW   int    // ширина текстовой области, для которой посчитан кэш
	layoutRev uint64 // ревизия метрик текста на момент компоновки
	dirty     bool   // текст изменился — кэш недействителен

	// Текст в байтах и смещения рун — замер префиксов без аллокаций.
	layoutBuf []byte
	runeOff   []int

	dragging bool
	capMgr   CaptureManager

	contextMenu *PopupMenu

	clicks clickSeries // номер нажатия в серии, если движок его не прислал

	// ime — диапазон незавершённого ввода внутри runes (ime.go). Набираемое
	// хранится прямо в тексте: так его видно без правок в отрисовке, а
	// раскладка строк считается с учётом набранного.
	ime imeState

	// История правок (textbox_undo.go): хранятся замены, а не снимки текста.
	undoStack []tbUndoEntry
	redoStack []tbUndoEntry
	pending   []tbEdit // замены текущего действия, ещё не закрытого commitUndo

	// Стили на диапазонах (textbox_style.go). Кэш — по номеру логической
	// строки, только для тех, что попадали в кадр; сбрасывается правкой текста.
	// styleGen растёт при каждом сбросе: ответ Styler, полученный без замка,
	// кладётся в кэш, только если за это время кэш не сбросили.
	styler     Styler
	styleCache map[int][]Span
	styleGen   uint64

	// Раскладка с учётом шрифта и табстопов. layoutKey — то, для чего она
	// посчитана: смена любой из частей делает кэш строк недействительным.
	pars       []int // начало каждого абзаца (логической строки) в runes
	layoutKey  tbLayoutKey
	layoutTabP int  // ширина табуляции в px для текущей раскладки (0 — табстопов нет)
	hasTabs    bool // в тексте есть табуляция (иначе меряем по-прежнему целиком)

	// Ширина самой длинной строки (для горизонтальной полосы): считается лениво
	// и только без переноса; widthCache — ширины строк по хешу текста, чтобы
	// правка одной строки не заставляла перемерять весь документ.
	cw         int
	cwOK       bool
	widthCache map[uint64]int
	widthSpare map[uint64]int

	// Перетаскивание ползунка горизонтальной полосы: hbarGrab — на сколько
	// правее левого края ползунка взялись.
	hbarDrag bool
	hbarGrab int

	// Wrap — переносить строки по словам (TextWrapping="Wrap").
	// false — длинные строки уходят вправо (горизонтальный скролл за кареткой).
	Wrap bool
	// ReadOnly — запрет редактирования (навигация и копирование работают).
	ReadOnly bool

	Placeholder string

	// FontName — именованный шрифт (RegisterFont); "" — шрифт по умолчанию.
	// Раскладка и отрисовка идут одним и тем же шрифтом. Для кода — моноширинный.
	FontName string
	// TabSize — ширина табуляции в пробелах. 0 — «как раньше»: табстопов нет,
	// пока не включён AcceptTab; с AcceptTab 0 означает 4.
	TabSize int
	// AcceptTab — Tab вставляет символ табуляции, а не уводит фокус (WPF:
	// AcceptsTab). По умолчанию выключено: приложения, собранные до появления
	// поля, продолжают переводить Tab в смену фокуса. Ctrl+Tab остаётся
	// навигацией.
	AcceptTab bool

	Background  color.RGBA
	BorderColor color.RGBA
	FocusBorder color.RGBA
	TextColor   color.RGBA
	PlaceColor  color.RGBA
	CaretColor  color.RGBA
	SelColor    color.RGBA

	PaddingX int
	PaddingY int

	FontSize float64 // pt (0 → DefaultFontSizePt)

	focused bool

	// caretPhase — фаза мигания каретки, ФАКТИЧЕСКИ отрисованная последним
	// Draw; caretPhaseKnown — рисовалась ли каретка вообще. См. NeedsAnimation.
	caretPhase      bool
	caretPhaseKnown bool

	// OnChange вызывается при каждом изменении текста.
	OnChange func(text string)
}

// tbLine — одна визуальная строка: полуинтервал рун [start, end).
// Завершающий '\n' (если есть) в интервал не входит.
type tbLine struct {
	start, end int
}

// tbLayoutKey — то, от чего зависит раскладка помимо текста и ширины: кегль,
// шрифт, ширина табуляции и перенос по словам. Раньше раскладка помнила только
// ширину и ревизию метрик, и смена шрифта (или включение переноса из меню
// «Формат») оставляла старые строки и старые позиции каретки до первой правки.
type tbLayoutKey struct {
	fs   float64
	font string
	tab  int // табуляция в пробелах; 0 — табстопов нет
	wrap bool
}

// NewTextBox создаёт многострочный редактор с переносом по словам.
func NewTextBox(placeholder string) *TextBox {
	return &TextBox{
		Placeholder: placeholder,
		Wrap:        true,
		Background:  win10.InputBG,
		BorderColor: win10.InputBorder,
		FocusBorder: win10.InputFocus,
		TextColor:   win10.InputText,
		PlaceColor:  win10.InputPlaceholder,
		CaretColor:  win10.InputCaret,
		SelColor:    premulAlpha(win10.Accent, 110),
		PaddingX:    6,
		PaddingY:    4,
		selAnchor:   -1,
		desiredX:    -1,
		dirty:       true,
	}
}

// premulAlpha возвращает корректный premultiplied-цвет: c с прозрачностью a.
// color.RGBA в Go — альфа-премультиплицированный; каналы, превышающие альфу,
// при Over-блендинге переполняются (артефакт «мадженты» на светлом фоне).
func premulAlpha(c color.RGBA, a uint8) color.RGBA {
	m := uint32(a)
	return color.RGBA{
		R: uint8(uint32(c.R) * m / 255),
		G: uint8(uint32(c.G) * m / 255),
		B: uint8(uint32(c.B) * m / 255),
		A: a,
	}
}

// ─── Текст ───────────────────────────────────────────────────────────────────

// SetText устанавливает содержимое (каретка — в конец, скролл — в начало).
func (t *TextBox) SetText(text string) {
	t.mu.Lock()
	runes := []rune(text)
	changed := string(t.runes) != text
	if changed {
		// Записанные правки указывают в прежний документ: откат на новом
		// тексте испортил бы его.
		t.resetUndo()
	}
	t.runes = runes
	t.caret = len(runes)
	t.selAnchor = -1
	t.scrollY = 0
	t.scrollX = 0
	t.desiredX = -1
	t.dirty = true
	t.mu.Unlock()
	if changed {
		t.Invalidate()
	}
}

// GetText возвращает текущее содержимое.
func (t *TextBox) GetText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.runes)
}

// changeText — текст для OnChange. Копия документа нужна только тому, кто
// слушает: без обработчика она раньше делалась на каждое нажатие клавиши, и на
// большом документе это была копия в мегабайты впустую. Вызывать под t.mu.
func (t *TextBox) changeText(onCh func(string)) string {
	if onCh == nil {
		return ""
	}
	return string(t.runes)
}

// AcceptsTab — контракт TabAcceptor: при AcceptTab клавиша Tab вставляет символ
// табуляции, а не уводит фокус. Только для чтения вставлять некуда — Tab снова
// уходит обходу фокуса. Ctrl+Tab остаётся навигацией всегда.
func (t *TextBox) AcceptsTab() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.AcceptTab && !t.ReadOnly
}

// SetStyler задаёт источник стилей на диапазонах текста (подсветка синтаксиса,
// найденного, ошибок); nil — убрать, цвет один на весь виджет, как раньше.
// Прежние ответы приложения сбрасываются.
func (t *TextBox) SetStyler(s Styler) {
	t.mu.Lock()
	t.styler = s
	t.styleCache = nil
	t.styleGen++
	t.mu.Unlock()
	t.Invalidate()
}

// InvalidateStyles велит спросить стили заново: приложение сообщает, что
// правила подсветки изменились без правки текста (новая строка поиска, новые
// результаты разбора). Правку текста виджет замечает сам.
func (t *TextBox) InvalidateStyles() {
	t.mu.Lock()
	t.styleCache = nil
	t.styleGen++
	t.mu.Unlock()
	t.Invalidate()
}

// SelectedText возвращает выделенный фрагмент ("" если выделения нет).
func (t *TextBox) SelectedText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.selActive() {
		return ""
	}
	lo, hi := t.normSel()
	return string(t.runes[lo:hi])
}

// CaretPosition возвращает позицию каретки (индекс в рунах).
func (t *TextBox) CaretPosition() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.caret
}

// SetCaretPosition ставит каретку в pos (индекс в рунах), сбрасывая выделение.
func (t *TextBox) SetCaretPosition(pos int) {
	t.mu.Lock()
	t.caret = pos
	t.clampCaret()
	t.selAnchor = -1
	t.desiredX = -1
	t.ensureLayout()
	t.ensureCaretVisible()
	t.mu.Unlock()
	t.Invalidate()
}

// InsertAtCaret вставляет s в позицию каретки (заменяя выделение),
// сдвигает каретку за вставку; поддерживает Undo и вызывает OnChange.
func (t *TextBox) InsertAtCaret(s string) {
	if s == "" {
		return
	}
	t.mu.Lock()
	if t.ReadOnly {
		t.mu.Unlock()
		return
	}
	caret0 := t.caret
	t.insertRunes([]rune(s))
	t.commitUndo(caret0)
	t.desiredX = -1
	t.ensureLayout()
	t.ensureCaretVisible()
	onCh := t.OnChange
	text := t.changeText(onCh)
	t.mu.Unlock()
	t.Invalidate()
	if onCh != nil {
		onCh(text)
	}
}

// LineCount возвращает число визуальных строк при текущей ширине.
func (t *TextBox) LineCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureLayout()
	return len(t.lines)
}

// ScrollTop возвращает вертикальный сдвиг в пикселях (для автоматизации).
func (t *TextBox) ScrollTop() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scrollY
}

// Cursor — текстовый курсор (I-beam) над редактором.
func (t *TextBox) Cursor(x, y int) Cursor {
	if !t.IsEnabled() {
		return CursorArrow
	}
	// Над полосой прокрутки — обычная стрелка: I-beam обещал бы текст там,
	// где его нет.
	t.mu.Lock()
	t.ensureLayout()
	onBar := t.hbarShown() && image.Pt(x, y).In(tbHBarHit(t.bounds, t.PaddingX, t.textAreaW()))
	t.mu.Unlock()
	if onBar {
		return CursorArrow
	}
	return CursorIBeam
}

// ─── Focusable / Animated ───────────────────────────────────────────────────

func (t *TextBox) SetFocused(focused bool) {
	t.mu.Lock()
	changed := t.focused != focused
	t.focused = focused
	t.mu.Unlock()
	if changed {
		t.Invalidate()
	}
}

func (t *TextBox) IsFocused() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.focused
}

// NeedsAnimation — «нужен ли движку кадр ради мигающей каретки».
//
// Здесь возвращался просто IsFocused(), и движок в режиме отрисовки по
// запросу готовил ПОЛНЫЙ кадр всего дерева на целевой частоте — шестьдесят
// кадров в секунду ради каретки, меняющейся дважды. Полный, а не частичный:
// damage при этом пуст, а пустой damage уводит кадр по полному пути (блит
// фона во весь холст, обход дерева без клипа, сравнение всех тайлов). Смысл
// отрисовки по запросу терялся, стоило поставить курсор в текст.
//
// Теперь так же, как в TextInput (см. его NeedsAnimation): пока фаза мигания
// совпадает с уже нарисованной, кадр не нужен; когда сменилась — редактор
// заявляет ТОЛЬКО свой прямоугольник и всё равно отвечает «нет», а движок
// увидит новое поколение инвалидации на следующем тике и нарисует частичный
// кадр. Задержка в один тик на полупериод 530 мс глазу незаметна.
func (t *TextBox) NeedsAnimation() bool {
	phase := caretPhaseAt(time.Now().UnixMilli())
	t.mu.Lock()
	if !t.focused || (t.caretPhaseKnown && t.caretPhase == phase) {
		t.mu.Unlock()
		return false
	}
	// Фазу фиксируем здесь же: если кадр до Draw не дойдёт (редактор скрыт,
	// кадр отброшен), иначе инвалидировали бы на каждом тике. Пропущенное
	// мигание безобиднее непрерывной перерисовки.
	t.caretPhase, t.caretPhaseKnown = phase, true
	t.mu.Unlock()
	t.Invalidate()
	return false
}

// ─── Геометрия и компоновка ──────────────────────────────────────────────────

const tbScrollbarW = 7 // зона тонкого вертикального скроллбара

func (t *TextBox) fontSize() float64 {
	if t.FontSize > 0 {
		return t.FontSize
	}
	return DefaultFontSizePt
}

// lineHeight — высота визуальной строки в px.
func (t *TextBox) lineHeight() int {
	return int(t.fontSize()*1.6) + 3
}

// textAreaW — ширина области текста (внутри рамки, без скроллбара).
func (t *TextBox) textAreaW() int { return t.textAreaWAt(t.bounds) }

// textAreaWAt — то же для заданных границ: Draw работает с границами,
// снятыми в начале кадра, и не должен перечитывать их по ходу.
func (t *TextBox) textAreaWAt(b image.Rectangle) int {
	w := b.Dx() - 2*t.PaddingX - tbScrollbarW
	if w < 20 {
		w = 20
	}
	return w
}

// visibleLines — сколько строк помещается по высоте. Показанная полоса
// горизонтальной прокрутки отнимает у текста свою высоту.
func (t *TextBox) visibleLines() int {
	h := t.bounds.Dy() - 2*t.PaddingY
	if t.hbarShown() {
		h -= tbHBarH
	}
	n := h / t.lineHeight()
	if n < 1 {
		n = 1
	}
	return n
}

// effectiveTabSize — ширина табуляции в пробелах для раскладки; 0 — табстопов нет.
func (t *TextBox) effectiveTabSize() int {
	switch {
	case t.TabSize > 0:
		return t.TabSize
	case t.AcceptTab:
		return tbDefaultTabSize
	}
	return 0
}

// measureText — ширина строки без табуляций шрифтом виджета, в пределах
// раскладки (без Draw): тем же измерителем, что и всё остальное.
func (t *TextBox) measureText(text string, fs float64) int {
	if t.layoutKey.font == "" {
		return MeasureUIText(text, fs)
	}
	return MeasureUITextFont(text, fs, t.layoutKey.font)
}

// ensureLayout перекомпоновывает строки при изменении текста, ширины, метрик
// шрифта, самого шрифта или табстопов. Вызывать под t.mu.
func (t *TextBox) ensureLayout() {
	w := t.textAreaW()
	rev := TextMetricsRev()
	key := tbLayoutKey{fs: t.fontSize(), font: t.FontName, tab: t.effectiveTabSize(), wrap: t.Wrap}
	if !t.dirty && t.layoutW == w && t.layoutRev == rev && t.layoutKey == key && t.lines != nil {
		return
	}
	if t.dirty {
		// Текст изменился: стили, которые приложение отдало прежнему тексту,
		// больше не про него. Поколение растёт, чтобы запоздавший ответ
		// Styler (он зовётся без замка) не попал в кэш.
		t.styleCache = nil
		t.styleGen++
	}
	if t.layoutRev != rev || t.layoutKey != key {
		t.widthCache = nil // ширины строк посчитаны другим шрифтом
	}
	t.cwOK = false
	t.layoutW = w
	t.layoutRev = rev
	t.layoutKey = key
	t.layoutTabP = 0
	if key.tab > 0 {
		sp := t.measureText(" ", key.fs)
		if sp < 1 {
			sp = 1
		}
		t.layoutTabP = key.tab * sp
	}
	t.dirty = false
	t.buildText()
	t.lines = t.lines[:0]
	t.pars = t.pars[:0]

	fs := key.fs
	n := len(t.runes)
	parStart := 0
	for i := 0; i <= n; i++ {
		if i < n && t.runes[i] != '\n' {
			continue
		}
		// Параграф [parStart, i)
		t.pars = append(t.pars, parStart)
		if !t.Wrap {
			t.lines = append(t.lines, tbLine{start: parStart, end: i})
		} else {
			t.wrapParagraph(parStart, i, w, fs)
		}
		parStart = i + 1
	}
	if len(t.lines) == 0 {
		t.lines = []tbLine{{}}
	}
}

// buildText пересобирает байтовый текст и смещения рун. Вызывать под t.mu.
func (t *TextBox) buildText() {
	t.layoutBuf = t.layoutBuf[:0]
	t.runeOff = append(t.runeOff[:0], 0)
	t.hasTabs = false
	for _, r := range t.runes {
		if r == '\t' {
			t.hasTabs = true
		}
		t.layoutBuf = utf8.AppendRune(t.layoutBuf, r)
		t.runeOff = append(t.runeOff, len(t.layoutBuf))
	}
}

// measureRange возвращает ширину рун [a, b) в пикселях; a — начало видимой
// строки, табстопы считаются от него. Вызывать под t.mu.
func (t *TextBox) measureRange(a, b int, fs float64) int {
	fast := !t.dirty && len(t.runeOff) == len(t.runes)+1
	if t.layoutKey.font == "" && (t.layoutTabP <= 0 || !t.hasTabs) {
		// Прежний путь без изменений: кэшированный байтовый замер без
		// аллокаций. Им идут все редакторы без шрифта и табуляций.
		if fast {
			return measureUIBytes(t.layoutBuf[t.runeOff[a]:t.runeOff[b]], fs)
		}
		return MeasureUIText(string(t.runes[a:b]), fs)
	}
	var text string
	if fast {
		text = string(t.layoutBuf[t.runeOff[a]:t.runeOff[b]])
	} else {
		text = string(t.runes[a:b])
	}
	tabW := t.layoutTabP
	if !t.hasTabs {
		tabW = 0
	}
	return measureTabbed(text, tabW, func(s string) int { return t.measureText(s, fs) })
}

// contentWidth — ширина самой длинной строки без переноса, px. С переносом 0:
// строки не шире области, и полоса не нужна. Вызывать под t.mu.
//
// Считается лениво и держится до следующей раскладки: нужна лишь когда
// решается, показывать ли полосу. Ширины строк кэшируются по хешу текста:
// правка меняет одну строку, и перемерять остальные тысячи — это замер всего
// документа на каждое нажатие клавиши.
func (t *TextBox) contentWidth() int {
	if t.Wrap || t.dirty || t.lines == nil {
		return 0
	}
	if t.cwOK {
		return t.cw
	}
	fs := t.layoutKey.fs
	// Две карты по очереди: новую на каждую правку выделять — мегабайты на
	// документе в десятки тысяч строк, а очищенная старая отдаёт ту же ёмкость.
	next := t.widthSpare
	if next == nil {
		next = make(map[uint64]int, len(t.lines))
	}
	clear(next)
	best := 0
	for _, ln := range t.lines {
		if ln.end <= ln.start {
			continue
		}
		h := hashRunes(t.runes[ln.start:ln.end])
		w, ok := t.widthCache[h]
		if !ok {
			w = t.measureRange(ln.start, ln.end, fs)
		}
		next[h] = w
		if w > best {
			best = w
		}
	}
	t.widthSpare, t.widthCache, t.cw, t.cwOK = t.widthCache, next, best, true
	return best
}

// hashRunes — FNV-1a по рунам. Коллизия даст лишь неточную ширину полосы, а не
// порчу текста, так что криптостойкость не нужна.
func hashRunes(rs []rune) uint64 {
	h := uint64(14695981039346656037)
	for _, r := range rs {
		h ^= uint64(r)
		h *= 1099511628211
	}
	return h
}

// hbarShown — показана ли полоса горизонтальной прокрутки. Вызывать под t.mu.
func (t *TextBox) hbarShown() bool {
	return tbNeedHBar(t.Wrap, t.contentWidth(), t.textAreaW())
}

// wrapParagraph разбивает параграф [start, end) на строки шириной ≤ maxW px:
// перенос по пробелам, слишком длинные слова режутся по символам.
// Вызывать под t.mu (внутри ensureLayout).
func (t *TextBox) wrapParagraph(start, end, maxW int, fs float64) {
	if start >= end {
		t.lines = append(t.lines, tbLine{start: start, end: end})
		return
	}
	lineStart, from := start, start
	for from < end {
		i := t.firstOverflow(lineStart, from, end, maxW, fs)
		if i >= end {
			break
		}
		// Последний пробел строки; правее него пробелов уже нет.
		lastSpace := -1
		for k := i; k >= lineStart; k-- {
			if t.runes[k] == ' ' {
				lastSpace = k
				break
			}
		}
		switch {
		case lastSpace > lineStart:
			// Перенос по последнему пробелу; пробел остаётся в верхней строке.
			t.lines = append(t.lines, tbLine{start: lineStart, end: lastSpace})
			lineStart = lastSpace + 1
		case i > lineStart:
			// Одно слово шире области — режем по символам.
			t.lines = append(t.lines, tbLine{start: lineStart, end: i})
			lineStart = i
		default:
			// Даже один символ не влезает — кладём его целиком.
			t.lines = append(t.lines, tbLine{start: lineStart, end: i + 1})
			lineStart = i + 1
		}
		from = i + 1
	}
	t.lines = append(t.lines, tbLine{start: lineStart, end: end})
}

// firstOverflow — первая руна из [from, end), где префикс от lineStart шире
// maxW; end — влезает весь остаток. Ширина не убывает — делим пополам.
func (t *TextBox) firstOverflow(lineStart, from, end, maxW int, fs float64) int {
	if t.measureRange(lineStart, end, fs) <= maxW {
		return end
	}
	lo, hi := from, end-1
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if t.measureRange(lineStart, mid+1, fs) > maxW {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}

// caretLine возвращает индекс строки, содержащей каретку.
// Вызывать под t.mu (после ensureLayout).
//
// Делением пополам, а не обходом: концы строк не убывают, а на каждый кадр и
// каждое нажатие клавиши линейный обход по десяткам тысяч строк заметен.
// Каретка на стыке двух строк (разрез длинного слова) принадлежит первой —
// как и при обходе с начала.
func (t *TextBox) caretLine() int {
	return lineOfPos(t.lines, t.caret)
}

// lineOfPos — индекс первой строки, чей конец не левее pos; если такой нет —
// последняя.
func lineOfPos(lines []tbLine, pos int) int {
	i := sort.Search(len(lines), func(i int) bool { return lines[i].end >= pos })
	if i >= len(lines) {
		return len(lines) - 1
	}
	return i
}

// parOfLine — номер абзаца (логической строки), которому принадлежит видимая
// строка ln. Начала абзацев строго возрастают, а перенос по словам не пересекает
// '\n', так что хватает начала строки. Вызывать под t.mu.
func (t *TextBox) parOfLine(ln tbLine) int {
	i := sort.Search(len(t.pars), func(i int) bool { return t.pars[i] > ln.start })
	if i == 0 {
		return 0
	}
	return i - 1
}

// lineTextW возвращает ширину префикса строки li длиной col рун (px).
// Вызывать под t.mu.
func (t *TextBox) lineTextW(li, col int) int {
	ln := t.lines[li]
	if col < 0 {
		col = 0
	}
	if col > ln.end-ln.start {
		col = ln.end - ln.start
	}
	return t.measureRange(ln.start, ln.start+col, t.fontSize())
}

// colAtX возвращает колонку (0..len) строки li, ближайшую к px x.
// Рубежи колонок не убывают — ищем делением пополам. Вызывать под t.mu.
func (t *TextBox) colAtX(li, x int) int {
	ln := t.lines[li]
	length := ln.end - ln.start
	lo, hi := 1, length
	res := length + 1
	for lo <= hi {
		c := int(uint(lo+hi) >> 1)
		if x < (t.lineTextW(li, c-1)+t.lineTextW(li, c))/2 {
			res = c
			hi = c - 1
		} else {
			lo = c + 1
		}
	}
	return res - 1
}

// caretPoint возвращает (строка, x px) каретки. Вызывать под t.mu.
func (t *TextBox) caretPoint() (line, x int) {
	li := t.caretLine()
	return li, t.lineTextW(li, t.caret-t.lines[li].start)
}

// ensureCaretVisible прокручивает так, чтобы каретка была видима.
// Вызывать под t.mu (после ensureLayout).
func (t *TextBox) ensureCaretVisible() {
	lh := t.lineHeight()
	li, cx := t.caretPoint()
	top := li * lh
	viewH := t.visibleLines() * lh
	if top < t.scrollY {
		t.scrollY = top
	}
	if top+lh > t.scrollY+viewH {
		t.scrollY = top + lh - viewH
	}
	t.clampScroll()

	if !t.Wrap {
		w := t.textAreaW()
		if cx-t.scrollX > w-tbCaretRoom {
			t.scrollX = cx - w + tbCaretRoom
		}
		if cx-t.scrollX < 0 {
			t.scrollX = cx
		}
		t.clampScrollX()
	}
}

// clampScrollX ограничивает scrollX шириной самой длинной строки. Раньше
// предела не было: после удаления длинной строки смещение оставалось где было,
// и пустое место справа можно было накрутить колесом сколь угодно далеко.
// Вызывать под t.mu (после ensureLayout).
func (t *TextBox) clampScrollX() {
	if t.Wrap {
		t.scrollX = 0
		return
	}
	t.scrollX = tbClampScrollX(t.scrollX, t.contentWidth(), t.textAreaW())
}

// clampScroll ограничивает scrollY содержимым. Вызывать под t.mu.
func (t *TextBox) clampScroll() {
	lh := t.lineHeight()
	maxScroll := len(t.lines)*lh - t.visibleLines()*lh
	if maxScroll < 0 {
		maxScroll = 0
	}
	if t.scrollY > maxScroll {
		t.scrollY = maxScroll
	}
	if t.scrollY < 0 {
		t.scrollY = 0
	}
}

// charIndexAtPoint возвращает позицию каретки для точки (абс. координаты).
// Вызывать под t.mu.
func (t *TextBox) charIndexAtPoint(absX, absY int) int {
	t.ensureLayout()
	b := t.bounds
	lh := t.lineHeight()
	li := (absY - b.Min.Y - t.PaddingY + t.scrollY) / lh
	if li < 0 {
		li = 0
	}
	if li >= len(t.lines) {
		li = len(t.lines) - 1
	}
	x := absX - (b.Min.X + t.PaddingX) + t.scrollX
	col := t.colAtX(li, x)
	return t.lines[li].start + col
}

// ─── Выделение / правка (helpers) ────────────────────────────────────────────

func (t *TextBox) selActive() bool { return t.selAnchor >= 0 && t.selAnchor != t.caret }

func (t *TextBox) normSel() (lo, hi int) {
	if t.selAnchor <= t.caret {
		return t.selAnchor, t.caret
	}
	return t.caret, t.selAnchor
}

// deleteSel удаляет выделенное. Возвращает true, если выделение было.
func (t *TextBox) deleteSel() bool {
	if !t.selActive() {
		return false
	}
	lo, hi := t.normSel()
	t.splice(lo, hi-lo, nil)
	t.caret = lo
	t.selAnchor = -1
	return true
}

// insertRunes вставляет rs в позицию каретки (учитывая выделение).
func (t *TextBox) insertRunes(rs []rune) {
	t.deleteSel()
	if len(rs) == 0 {
		return
	}
	t.splice(t.caret, 0, rs)
	t.caret += len(rs)
}

func (t *TextBox) clampCaret() {
	if t.caret < 0 {
		t.caret = 0
	}
	if t.caret > len(t.runes) {
		t.caret = len(t.runes)
	}
}

// moveCaret переносит каретку в pos, поддерживая якорь при extend (Shift).
func (t *TextBox) moveCaret(pos int, extend bool) {
	if extend {
		if t.selAnchor < 0 {
			t.selAnchor = t.caret
		}
	} else {
		t.selAnchor = -1
	}
	t.caret = pos
	t.clampCaret()
}

// wordLeft возвращает позицию начала предыдущего слова.
func (t *TextBox) wordLeft(idx int) int {
	if idx <= 0 {
		return 0
	}
	i := idx - 1
	for i > 0 && !isWordRune(t.runes[i]) {
		i--
	}
	for i > 0 && isWordRune(t.runes[i-1]) {
		i--
	}
	return i
}

// wordRight возвращает позицию за концом следующего слова.
func (t *TextBox) wordRight(idx int) int {
	n := len(t.runes)
	i := idx
	for i < n && isWordRune(t.runes[i]) {
		i++
	}
	for i < n && !isWordRune(t.runes[i]) {
		i++
	}
	return i
}

// copySelection копирует выделение в буфер обмена (разрешено и при ReadOnly).
// Вызывать под t.mu.Lock().
func (t *TextBox) copySelection() {
	if t.selActive() {
		lo, hi := t.normSel()
		ClipboardSetText(string(t.runes[lo:hi]))
	}
}

// cutSelection копирует выделение в буфер обмена и удаляет его. При ReadOnly
// вырезание запрещено. Возвращает true, если что-то вырезано. Вызывать под t.mu.
func (t *TextBox) cutSelection() bool {
	if t.ReadOnly || !t.selActive() {
		return false
	}
	lo, hi := t.normSel()
	ClipboardSetText(string(t.runes[lo:hi]))
	t.deleteSel()
	return true
}

// pasteFromClipboard вставляет текст из буфера обмена в позицию каретки.
// При ReadOnly вставка запрещена. Возвращает true, если текст вставлен.
// Вызывать под t.mu.Lock().
func (t *TextBox) pasteFromClipboard() bool {
	if t.ReadOnly {
		return false
	}
	if clip := ClipboardGetText(); clip != "" {
		t.insertRunes([]rune(clip))
		return true
	}
	return false
}

// ─── KeyHandler ──────────────────────────────────────────────────────────────

func (t *TextBox) OnKeyEvent(e KeyEvent) {
	if !t.IsEnabled() || !e.Pressed {
		return
	}
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		t.contextMenu.OnKeyEvent(e)
		return
	}

	ctrl := e.Mod&ModCtrl != 0
	shift := e.Mod&ModShift != 0

	t.mu.Lock()
	t.ensureLayout()
	if !t.selActive() {
		// Выделение нулевой ширины (Shift+→ и сразу Shift+←) невидимо, но якорь
		// остаётся. Первая же правка двигала каретку от него прочь, и невидимое
		// выделение оживало на соседних знаках; а при удалении слова якорь
		// оказывался за концом текста и следующая вставка падала на срезе.
		t.selAnchor = -1
	}

	changed := false
	isUndoRedo := false
	keepDesiredX := false
	caret0, sel0, scr0, scr0x := t.caret, t.selAnchor, t.scrollY, t.scrollX

	switch e.Code {
	case KeyLeft:
		pos := t.caret
		switch {
		case ctrl:
			pos = t.wordLeft(t.caret)
		case !shift && t.selActive():
			pos, _ = t.normSel()
		case t.caret > 0:
			pos = t.caret - 1
		}
		t.moveCaret(pos, shift)

	case KeyRight:
		pos := t.caret
		switch {
		case ctrl:
			pos = t.wordRight(t.caret)
		case !shift && t.selActive():
			_, pos = t.normSel()
		case t.caret < len(t.runes):
			pos = t.caret + 1
		}
		t.moveCaret(pos, shift)

	case KeyUp, KeyDown:
		keepDesiredX = true
		li, cx := t.caretPoint()
		if t.desiredX < 0 {
			t.desiredX = cx
		}
		if e.Code == KeyUp {
			li--
		} else {
			li++
		}
		if li >= 0 && li < len(t.lines) {
			col := t.colAtX(li, t.desiredX)
			t.moveCaret(t.lines[li].start+col, shift)
		} else if !shift {
			t.selAnchor = -1
		}

	case KeyPageUp, KeyPageDown:
		keepDesiredX = true
		li, cx := t.caretPoint()
		if t.desiredX < 0 {
			t.desiredX = cx
		}
		page := t.visibleLines()
		if e.Code == KeyPageUp {
			li -= page
		} else {
			li += page
		}
		if li < 0 {
			li = 0
		}
		if li >= len(t.lines) {
			li = len(t.lines) - 1
		}
		col := t.colAtX(li, t.desiredX)
		t.moveCaret(t.lines[li].start+col, shift)

	case KeyHome:
		if ctrl {
			t.moveCaret(0, shift)
		} else {
			li := t.caretLine()
			t.moveCaret(t.lines[li].start, shift)
		}

	case KeyEnd:
		if ctrl {
			t.moveCaret(len(t.runes), shift)
		} else {
			li := t.caretLine()
			t.moveCaret(t.lines[li].end, shift)
		}

	case KeyBackspace:
		if t.ReadOnly {
			break
		}
		if t.deleteSel() {
			changed = true
		} else if ctrl {
			// Ctrl+Backspace — удалить слово НАЗАД от каретки.
			if t.caret > 0 {
				start := t.wordLeft(t.caret)
				if start < t.caret {
					t.splice(start, t.caret-start, nil)
					t.caret = start
					changed = true
				}
			}
		} else if t.caret > 0 {
			t.splice(t.caret-1, 1, nil)
			t.caret--
			changed = true
		}

	case KeyInsert:
		if ctrl {
			// Ctrl+Insert = копировать (как Ctrl+C).
			t.copySelection()
		} else if shift {
			// Shift+Insert = вставить (как Ctrl+V); запрещено при ReadOnly.
			if t.pasteFromClipboard() {
				changed = true
			}
		}

	case KeyDelete:
		if shift {
			// Shift+Delete = вырезать (как Ctrl+X) — приоритетнее обычного Delete
			// (cutSelection сам блокирует ReadOnly).
			if t.cutSelection() {
				changed = true
			}
			break
		}
		if t.ReadOnly {
			break
		}
		if t.deleteSel() {
			changed = true
		} else if ctrl {
			// Ctrl+Delete — удалить слово ВПЕРЁД от каретки.
			if t.caret < len(t.runes) {
				end := t.wordRight(t.caret)
				if end > t.caret {
					t.splice(t.caret, end-t.caret, nil)
					changed = true
				}
			}
		} else if t.caret < len(t.runes) {
			t.splice(t.caret, 1, nil)
			changed = true
		}

	case KeyEnter:
		if !t.ReadOnly {
			t.insertRunes([]rune{'\n'})
			changed = true
		}

	case KeyTab:
		// Tab до сюда доходит, только если AcceptsTab() сказал «да» (движок
		// иначе уводит его в смену фокуса) либо событие подали напрямую.
		// Вставляем сам символ: пробелы вместо него редактор кода выбирает
		// сам, а табстопы (TabSize) делают табуляцию нужной ширины. Shift+Tab
		// ничего не вставляет — в списке отступов он «убрать», а не «добавить».
		if t.AcceptTab && !t.ReadOnly && !ctrl && !shift {
			t.insertRunes([]rune{'\t'})
			changed = true
		}

	default:
		if ctrl {
			switch e.Code {
			case KeyZ:
				if t.ReadOnly {
					break
				}
				if shift {
					t.redo()
				} else {
					t.undo()
				}
				isUndoRedo = true
				changed = true
			case KeyY:
				if t.ReadOnly {
					break
				}
				t.redo()
				isUndoRedo = true
				changed = true
			case KeyA:
				t.selAnchor = 0
				t.caret = len(t.runes)
			case KeyC:
				t.copySelection()
			case KeyX:
				if t.cutSelection() {
					changed = true
				}
			case KeyV:
				if t.pasteFromClipboard() {
					changed = true
				}
			}
		} else if e.Rune >= 32 && !t.ReadOnly {
			t.insertRunes([]rune{e.Rune})
			changed = true
		}
	}

	if !isUndoRedo {
		// Закрываем действие всегда, а не только при changed: замены могли
		// остаться от набора через IME, и пусть лучше они станут своей записью,
		// чем прилипнут к чужому действию.
		t.commitUndo(caret0)
	}
	if !keepDesiredX {
		t.desiredX = -1
	}
	t.clampCaret()
	t.ensureLayout()
	t.ensureCaretVisible()

	visChanged := changed || t.caret != caret0 || t.selAnchor != sel0 ||
		t.scrollY != scr0 || t.scrollX != scr0x
	onCh := t.OnChange
	text := t.changeText(onCh)
	t.mu.Unlock()

	if visChanged {
		t.Invalidate()
	}
	if changed && onCh != nil {
		onCh(text)
	}
}

// ─── Мышь ────────────────────────────────────────────────────────────────────

// SetCaptureManager инжектится движком (drag-выделение за пределами bounds).
func (t *TextBox) SetCaptureManager(cm CaptureManager) { t.capMgr = cm }

// WantsCapture — захватываем мышь на ЛКМ внутри редактора.
func (t *TextBox) WantsCapture(e MouseEvent) bool {
	return e.Button == MouseLeft && e.Pressed
}

func (t *TextBox) OnMouseButton(e MouseEvent) bool {
	if !t.IsEnabled() {
		return false
	}

	// Контекстное меню.
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		if e.Button == MouseRight && !e.Pressed {
			return true
		}
		if image.Pt(e.X, e.Y).In(t.contextMenu.Bounds()) {
			return t.contextMenu.OnMouseButton(e)
		}
		t.contextMenu.Close()
	}
	if e.Button == MouseRight && e.Pressed {
		t.showContextMenu(e.X, e.Y)
		return true
	}

	inside := image.Pt(e.X, e.Y).In(t.bounds)

	// Колесо — вертикальная прокрутка.
	if inside && (e.Button == MouseWheelUp || e.Button == MouseWheelDown) && e.Pressed {
		t.mu.Lock()
		t.ensureLayout()
		oldY, oldX := t.scrollY, t.scrollX
		step := 3 * t.lineHeight()
		if e.Button == MouseWheelUp {
			step = -step
		}
		if e.Mod&ModShift != 0 && t.hbarShown() {
			// Shift+колесо — вбок: на мыши без горизонтального колеса это
			// единственный способ добраться до конца длинной строки без
			// захвата ползунка. Нет полосы — Shift ничего не меняет.
			t.scrollX += step
			t.clampScrollX()
		} else {
			t.scrollY += step
			t.clampScroll()
		}
		moved := t.scrollY != oldY || t.scrollX != oldX
		t.mu.Unlock()
		if moved {
			t.Invalidate()
		}
		return true
	}

	if e.Button != MouseLeft {
		return false
	}

	t.mu.Lock()
	caret0, sel0, scrX0 := t.caret, t.selAnchor, t.scrollX
	defer func() {
		changed := t.caret != caret0 || t.selAnchor != sel0 || t.scrollX != scrX0
		t.mu.Unlock()
		if changed {
			t.Invalidate()
		}
	}()

	if e.Pressed {
		// Полоса прокрутки — раньше текста: нажатие на неё не должно ставить
		// каретку в строку под ней.
		if t.hbarPressLocked(e.X, e.Y) {
			return true
		}
		idx := t.charIndexAtPoint(e.X, e.Y)

		// Двойной щелчок выделяет слово, тройной — строку.
		switch n := clicksOf(e, &t.clicks); {
		case n == 2:
			lo, hi := t.wordBounds(idx)
			t.selAnchor = lo
			t.caret = hi
			t.dragging = false
			return true
		case n >= 3:
			lo, hi := t.lineBounds(idx)
			t.selAnchor = lo
			t.caret = hi
			t.dragging = false
			return true
		}

		t.caret = idx
		t.selAnchor = idx
		t.dragging = true
		t.desiredX = -1
	} else {
		t.dragging = false
		t.hbarDrag = false
		if t.selAnchor == t.caret {
			t.selAnchor = -1
		}
		if t.capMgr != nil {
			t.capMgr.ReleaseCapture()
		}
	}
	return true
}

// OnMouseWheelPixels — плавная прокрутка точной пиксельной дельтой
// (тачпад/колесо высокой точности). dy>0 — вниз, dx>0 — вправо. В отличие от
// тикового колеса (3 строки за тик) применяет дельту попиксельно с накоплением
// субпиксельного остатка. Возвращает false, если курсор вне поля или
// прокручивать нечего — чтобы событие всплыло к родителю.
func (t *TextBox) OnMouseWheelPixels(x, y int, dx, dy float64) bool {
	return t.OnMouseWheelPixelsMod(x, y, dx, dy, 0)
}

// OnMouseWheelPixelsMod — то же с модификаторами: Shift уводит вертикальную
// дельту вбок. Горизонтальная дельта раньше не принималась вовсе — dx
// молча терялся, и тачпад не двигал длинную строку.
func (t *TextBox) OnMouseWheelPixelsMod(x, y int, dx, dy float64, mod KeyMod) bool {
	if !image.Pt(x, y).In(t.bounds) {
		return false
	}
	t.mu.Lock()
	t.ensureLayout()
	oldY, oldX := t.scrollY, t.scrollX
	consumed := false

	hbar := t.hbarShown()
	if mod&ModShift != 0 && hbar && dx == 0 {
		dx, dy = dy, 0
	}
	if dx != 0 && hbar {
		maxX := tbScrollXMax(t.contentWidth(), t.textAreaW())
		if !((dx < 0 && t.scrollX <= 0) || (dx > 0 && t.scrollX >= maxX)) {
			t.scrollFracX += dx
			whole := math.Trunc(t.scrollFracX)
			t.scrollFracX -= whole
			t.scrollX += int(whole)
			t.clampScrollX()
			consumed = true
		}
	}

	lh := t.lineHeight()
	maxScroll := len(t.lines)*lh - t.visibleLines()*lh
	if dy != 0 && maxScroll > 0 &&
		!((dy < 0 && t.scrollY <= 0) || (dy > 0 && t.scrollY >= maxScroll)) {
		t.scrollFrac += dy
		whole := math.Trunc(t.scrollFrac)
		t.scrollFrac -= whole
		t.scrollY += int(whole)
		t.clampScroll()
		consumed = true
	}
	moved := t.scrollY != oldY || t.scrollX != oldX
	t.mu.Unlock()
	if moved {
		t.Invalidate()
	}
	return consumed
}

// hbarThumbLocked — ползунок горизонтальной полосы. Пустой — полосы нет.
// Вызывать под t.mu (после ensureLayout).
func (t *TextBox) hbarThumbLocked() image.Rectangle {
	if !t.hbarShown() {
		return image.Rectangle{}
	}
	vw := t.textAreaW()
	cw := t.contentWidth()
	return hbarThumb(tbHBarTrack(t.bounds, t.PaddingX, vw), float64(t.scrollX),
		float64(tbScrollXMax(cw, vw)), float64(vw), float64(cw+tbCaretRoom))
}

// hbarPressLocked — нажатие на полосу: на ползунке начинается перетаскивание
// без скачка, мимо — текст прыгает туда, куда ткнули. Возвращает true, если
// нажатие пришлось на полосу. Вызывать под t.mu.
func (t *TextBox) hbarPressLocked(x, y int) bool {
	t.ensureLayout()
	if !t.hbarShown() {
		return false
	}
	vw := t.textAreaW()
	if !image.Pt(x, y).In(tbHBarHit(t.bounds, t.PaddingX, vw)) {
		return false
	}
	th := t.hbarThumbLocked()
	t.dragging = false
	t.hbarDrag, t.hbarGrab = true, 0
	if !th.Empty() && x >= th.Min.X && x < th.Max.X {
		t.hbarGrab = x - th.Min.X
		return true
	}
	cw := t.contentWidth()
	t.scrollX = int(math.Round(hbarScrollAt(tbHBarTrack(t.bounds, t.PaddingX, vw), x,
		float64(tbScrollXMax(cw, vw)), float64(vw), float64(cw+tbCaretRoom))))
	t.clampScrollX()
	return true
}

// hbarDragToLocked ведёт ползунок за курсором. Вызывать под t.mu.
func (t *TextBox) hbarDragToLocked(x int) {
	t.ensureLayout()
	vw := t.textAreaW()
	cw := t.contentWidth()
	t.scrollX = int(math.Round(hbarScrollForThumbX(tbHBarTrack(t.bounds, t.PaddingX, vw), x-t.hbarGrab,
		float64(tbScrollXMax(cw, vw)), float64(vw), float64(cw+tbCaretRoom))))
	t.clampScrollX()
}

func (t *TextBox) OnMouseMove(x, y int) {
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		t.contextMenu.OnMouseMove(x, y)
	}
	t.mu.Lock()
	caret0, scr0, scrX0 := t.caret, t.scrollY, t.scrollX
	if t.hbarDrag {
		t.hbarDragToLocked(x)
	} else if t.dragging {
		t.caret = t.charIndexAtPoint(x, y)
		t.ensureCaretVisible()
	}
	changed := t.caret != caret0 || t.scrollY != scr0 || t.scrollX != scrX0
	t.mu.Unlock()
	if changed {
		t.Invalidate()
	}
}

// wordBounds — границы слова вокруг idx (как в TextInput).
func (t *TextBox) wordBounds(idx int) (int, int) {
	n := len(t.runes)
	if n == 0 {
		return 0, 0
	}
	if idx >= n {
		idx = n - 1
	}
	if idx < 0 {
		idx = 0
	}
	cls := isWordRune(t.runes[idx])
	lo := idx
	for lo > 0 && isWordRune(t.runes[lo-1]) == cls && t.runes[lo-1] != '\n' {
		lo--
	}
	hi := idx + 1
	for hi < n && isWordRune(t.runes[hi]) == cls && t.runes[hi] != '\n' {
		hi++
	}
	return lo, hi
}

// lineBounds — границы строки вокруг idx, без переноса на конце: выделять
// сам перенос значит выделять и начало следующей строки, а тройным щелчком
// просят именно эту.
func (t *TextBox) lineBounds(idx int) (int, int) {
	n := len(t.runes)
	if idx > n {
		idx = n
	}
	if idx < 0 {
		idx = 0
	}
	lo := idx
	for lo > 0 && t.runes[lo-1] != '\n' {
		lo--
	}
	hi := idx
	for hi < n && t.runes[hi] != '\n' {
		hi++
	}
	return lo, hi
}

// ─── Контекстное меню ────────────────────────────────────────────────────────

func (t *TextBox) showContextMenu(x, y int) {
	t.mu.Lock()
	hasSel := t.selActive()
	hasText := len(t.runes) > 0
	ro := t.ReadOnly
	t.mu.Unlock()
	hasClip := ClipboardGetText() != ""

	edit := func(action func()) func() {
		return func() {
			t.mu.Lock()
			caret0 := t.caret
			t.mu.Unlock()
			action()
			t.mu.Lock()
			// Правки меню раньше в историю не попадали: Ctrl+Z после «Вырезать»
			// откатывал что-то более раннее. Теперь это обычное действие.
			t.commitUndo(caret0)
			t.clampCaret()
			t.ensureLayout()
			t.ensureCaretVisible()
			onCh := t.OnChange
			text := t.changeText(onCh)
			t.mu.Unlock()
			t.Invalidate()
			if onCh != nil {
				onCh(text)
			}
		}
	}

	menu := NewPopupMenu()
	menu.SetItems([]MenuItem{
		{Text: "Cut", Disabled: !hasSel || ro, OnClick: edit(func() {
			t.mu.Lock()
			if t.selActive() {
				lo, hi := t.normSel()
				ClipboardSetText(string(t.runes[lo:hi]))
				t.deleteSel()
			}
			t.mu.Unlock()
		})},
		{Text: "Copy", Disabled: !hasSel, OnClick: func() {
			t.mu.Lock()
			if t.selActive() {
				lo, hi := t.normSel()
				ClipboardSetText(string(t.runes[lo:hi]))
			}
			t.mu.Unlock()
		}},
		{Text: "Paste", Disabled: !hasClip || ro, OnClick: edit(func() {
			if clip := ClipboardGetText(); clip != "" {
				t.mu.Lock()
				t.insertRunes([]rune(clip))
				t.mu.Unlock()
			}
		})},
		{Separator: true},
		{Text: "Select All", Disabled: !hasText, OnClick: func() {
			t.mu.Lock()
			t.selAnchor = 0
			t.caret = len(t.runes)
			t.mu.Unlock()
			t.Invalidate()
		}},
	})
	menu.Show(x, y)
	t.contextMenu = menu
}

// ─── Draw ────────────────────────────────────────────────────────────────────

// tbVisLine — видимая строка, снятая под замком для отрисовки: границы, её
// руны и откуда она в абзаце (для стилей).
type tbVisLine struct {
	ln  tbLine
	rs  []rune
	par int // номер абзаца (логической строки)
	off int // смещение начала строки в абзаце, руны
}

// tbDrawState — всё, что Draw берёт у виджета под замком. Вынесено в
// структуру, чтобы рисовать без замка и не читать поля виджета гонкой.
type tbDrawState struct {
	vis      []tbVisLine // видимые строки с first по last включительно
	first    int         // индекс первой видимой строки среди всех
	nLines   int
	empty    bool // текста нет вовсе — рисуем подсказку
	caret    int
	caretLi  int
	selLo    int
	selHi    int
	scrollX  int
	scrollY  int
	imeFrom  int
	imeTo    int
	imeOn    bool
	focused  bool
	fs       float64
	lh       int
	font     string
	tabPx    int
	hbar     bool
	contentW int
	thumb    image.Rectangle
	spans    map[int][]Span // стили абзацев видимых строк (из кэша или от Styler)
	need     map[int]string // абзацы, стилей которых в кэше нет: номер -> текст
	styler   Styler
	gen      uint64
}

// snapshotDraw снимает состояние для кадра. Копируется ТОЛЬКО видимый диапазон
// строк: раньше каждый кадр копировал весь срез рун и все строки документа
// (на документе в мегабайты — копия в мегабайты на кадр, даже когда мигает одна
// каретка), а рисовалось из них десяток строк.
func (t *TextBox) snapshotDraw(viewH int) tbDrawState {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureLayout()
	t.clampScrollX() // перенос включили, или текст стал короче — смещение не должно «висеть»

	st := tbDrawState{
		nLines:  len(t.lines),
		empty:   len(t.runes) == 0,
		caret:   t.caret,
		selLo:   -1,
		selHi:   -1,
		scrollX: t.scrollX,
		scrollY: t.scrollY,
		focused: t.focused,
		fs:      t.fontSize(),
		lh:      t.lineHeight(),
		font:    t.layoutKey.font,
		styler:  t.styler,
		gen:     t.styleGen,
	}
	if t.selActive() {
		st.selLo, st.selHi = t.normSel()
	}
	st.imeFrom, st.imeTo, st.imeOn = t.ime.imeRange()
	if t.hasTabs {
		st.tabPx = t.layoutTabP
	}
	st.hbar = t.hbarShown()
	if st.hbar {
		st.contentW = t.contentWidth()
		st.thumb = t.hbarThumbLocked()
	}
	st.caretLi = t.caretLine()

	if st.empty {
		return st
	}
	first := st.scrollY / st.lh
	last := (st.scrollY + viewH) / st.lh
	if first < 0 {
		first = 0
	}
	if last >= st.nLines {
		last = st.nLines - 1
	}
	st.first = first
	for li := first; li <= last; li++ {
		ln := t.lines[li]
		par := t.parOfLine(ln)
		st.vis = append(st.vis, tbVisLine{
			ln:  ln,
			rs:  append([]rune(nil), t.runes[ln.start:ln.end]...),
			par: par,
			off: ln.start - t.pars[par],
		})
		if st.styler == nil {
			continue
		}
		if _, ok := st.spans[par]; ok {
			continue
		}
		if _, ok := st.need[par]; ok {
			continue
		}
		if sp, ok := t.styleCache[par]; ok {
			if st.spans == nil {
				st.spans = make(map[int][]Span)
			}
			st.spans[par] = sp
			continue
		}
		end := len(t.runes)
		if par+1 < len(t.pars) {
			end = t.pars[par+1] - 1
		}
		if st.need == nil {
			st.need = make(map[int]string)
		}
		st.need[par] = string(t.runes[t.pars[par]:end])
	}
	return st
}

// fetchStyles спрашивает у приложения стили абзацев, которых нет в кэше. Зовёт
// БЕЗ замка: Styler — чужой код, и он вправе обратиться к самому виджету
// (GetText, позиция каретки) или долго считать; под замком это тупик или
// заморозка ввода. Ответ кладётся в кэш, только если за время вызова текст и
// правила не менялись: иначе устаревшие стили осели бы в кэше.
func (t *TextBox) fetchStyles(st *tbDrawState) {
	if len(st.need) == 0 || st.styler == nil {
		return
	}
	got := make(map[int][]Span, len(st.need))
	for par, text := range st.need {
		got[par] = append([]Span(nil), st.styler.LineSpans(par, text)...)
	}
	if st.spans == nil {
		st.spans = make(map[int][]Span, len(got))
	}
	for par, sp := range got {
		st.spans[par] = sp
	}

	t.mu.Lock()
	if t.styleGen == st.gen && !t.dirty {
		if t.styleCache == nil || len(t.styleCache) > tbStyleCacheMax {
			t.styleCache = make(map[int][]Span, len(got))
		}
		for par, sp := range got {
			t.styleCache[par] = sp
		}
	}
	t.mu.Unlock()
}

// tbStyleCacheMax — сколько абзацев держит кэш стилей. Кэш пополняется только
// видимыми строками, а чистится правкой текста; предел страхует от прокрутки
// по документу в сотни тысяч строк без единой правки.
const tbStyleCacheMax = 4096

func (t *TextBox) Draw(ctx DrawContext) {
	b := t.bounds
	if b.Empty() {
		return
	}

	st := t.snapshotDraw(b.Dy())
	t.fetchStyles(&st)
	fs, lh := st.fs, st.lh
	focused := st.focused

	sty := currentStyle()

	// Фон и рамка — в стиле TextInput активной темы.
	switch {
	case sty.Classic3D:
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.Background)
		drawBevelSunken(ctx, b.Min.X, b.Min.Y, b.Dx(), b.Dy(), sty)
	case sty.ControlCorner > 0:
		cr := sty.ControlCorner
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, t.Background)
		if focused {
			ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, t.FocusBorder)
		} else {
			ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, t.BorderColor)
		}
	default:
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.Background)
		if focused {
			ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.FocusBorder)
		} else {
			ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.BorderColor)
		}
	}

	inner := image.Rect(b.Min.X+1, b.Min.Y+1, b.Max.X-1, b.Max.Y-1)
	// Текст не должен заходить на полосу прокрутки: нижняя строка обрезается по
	// её верхнему краю. Сужение — пересечением с внешней областью, а в конце
	// она возвращается: поле может стоять внутри прокрутки.
	textClip := inner
	if st.hbar {
		textClip.Max.Y = b.Max.Y - tbHBarH
	}
	outer := ctx.Clip()
	ctx.SetClip(textClip.Intersect(outer))

	textX := b.Min.X + t.PaddingX - st.scrollX
	topY := b.Min.Y + t.PaddingY

	// Измерение и вывод — тем же шрифтом, каким считалась раскладка: иначе
	// каретка и выделение разойдутся с буквами.
	measure := func(s string) int { return ctx.MeasureText(s, fs) }
	if st.font != "" {
		measure = func(s string) int { return ctx.MeasureTextFont(s, fs, st.font) }
	}
	drawText := func(s string, x, y int, face string, col color.RGBA) {
		if face == "" {
			face = st.font
		}
		if face == "" {
			ctx.DrawTextSize(s, x, y, fs, col)
			return
		}
		ctx.DrawTextFont(s, x, y, fs, face, col)
	}
	// xAt — x колонки col строки rs. Колонка 0 — ровно начало: меряем только
	// непустой префикс.
	xAt := func(rs []rune, col int) int {
		if col <= 0 {
			return textX
		}
		return textX + measureTabbed(string(rs[:col]), st.tabPx, measure)
	}

	if st.empty {
		drawText(t.Placeholder, b.Min.X+t.PaddingX, topY+2, "", t.PlaceColor)
	} else {
		for i := range st.vis {
			vl := &st.vis[i]
			ln, rs := vl.ln, vl.rs
			li := st.first + i
			y := topY + li*lh - st.scrollY

			pieces := splitStylePieces(len(rs), vl.off, st.spans[vl.par])

			// Фон кусков — под выделением и текстом.
			for _, pc := range pieces {
				bg := pc.Style.BG
				if bg.A == 0 {
					continue
				}
				x0, x1 := xAt(rs, pc.From), xAt(rs, pc.To)
				if x1 <= x0 {
					continue
				}
				if bg.A == 255 {
					ctx.FillRect(x0, y, x1-x0, lh, bg)
				} else {
					ctx.FillRectAlpha(x0, y, x1-x0, lh, bg)
				}
			}

			// Подсветка выделения в пределах строки.
			if lo, hi, spill, ok := tbSelOnLine(ln, st.selLo, st.selHi); ok {
				x0 := xAt(rs, lo-ln.start)
				x1 := xAt(rs, hi-ln.start)
				if spill { // выделение уходит на следующую строку
					x1 += 5
				}
				if x1 > x0 {
					ctx.FillRectAlpha(x0, y, x1-x0, lh, t.SelColor)
				}
			}

			for _, pc := range pieces {
				col := t.TextColor
				if pc.Style.Color.A != 0 {
					col = pc.Style.Color
				}
				if st.tabPx <= 0 {
					if pc.To > pc.From {
						drawText(string(rs[pc.From:pc.To]), xAt(rs, pc.From), y+2, pc.Style.Face, col)
					}
					continue
				}
				// Табуляция сама ничего не рисует: куски между табуляциями
				// выводятся каждый в своём табстопе.
				for _, run := range splitTabRuns(rs, pc.From, pc.To) {
					drawText(string(rs[run.From:run.To]), xAt(rs, run.From), y+2, pc.Style.Face, col)
				}
			}

			// Набираемый, но ещё не введённый текст подчёркивается: иначе
			// человек не отличит его от уже введённого.
			if st.imeOn && st.imeFrom < ln.end+1 && st.imeTo > ln.start {
				lo, hi := st.imeFrom, st.imeTo
				if lo < ln.start {
					lo = ln.start
				}
				if hi > ln.end {
					hi = ln.end
				}
				x0 := xAt(rs, lo-ln.start)
				x1 := xAt(rs, hi-ln.start)
				if x1 > x0 {
					ctx.DrawHLine(x0, y+lh-3, x1-x0, t.TextColor)
				}
			}
		}
	}

	// Каретка. Рисуется, только если её строка в кадре: остальные всё равно
	// срезал бы клип, а снимать для них текст строки незачем.
	if focused && caretPhaseAt(time.Now().UnixMilli()) {
		if i := st.caretLi - st.first; i >= 0 && i < len(st.vis) {
			vl := &st.vis[i]
			cx := xAt(vl.rs, st.caret-vl.ln.start)
			cy := topY + st.caretLi*lh - st.scrollY
			ctx.DrawVLine(cx, cy+1, lh-2, t.CaretColor)
		}
	}

	// Тонкий вертикальный скроллбар при переполнении.
	contentH := st.nLines * lh
	viewH := b.Dy() - 2*t.PaddingY
	trackH := b.Dy() - 8
	if st.hbar {
		viewH -= tbHBarH
		trackH -= tbHBarH
	}
	if contentH > viewH {
		thumbH := trackH * viewH / contentH
		if thumbH < 20 {
			thumbH = 20
		}
		maxScroll := contentH - viewH
		ty := b.Min.Y + 4 + (trackH-thumbH)*st.scrollY/maxScroll
		ctx.FillRoundRect(b.Max.X-6, ty, 4, thumbH, 2, win10.ScrollThumbBG)
	}

	// Горизонтальная полоса прокрутки — геометрия общая со сравнением файлов.
	if st.hbar && !st.thumb.Empty() {
		tr := tbHBarTrack(b, t.PaddingX, t.textAreaWAt(b))
		ctx.SetClip(tr.Inset(-2).Intersect(inner).Intersect(outer))
		drawHBar(ctx, tr, st.thumb, win10.ScrollTrackBG, win10.ScrollThumbBG)
	}

	// Возвращаем внешнюю область, а не снимаем отсечение: иначе соседи поля
	// внутри прокрутки рисовали бы мимо неё.
	ctx.SetClip(outer)
	t.drawChildren(ctx)
	t.drawDisabledOverlay(ctx)
}

// ─── Bounds / Overlay (контекстное меню) ─────────────────────────────────────

// Bounds включает открытое контекстное меню (для hit-теста движка).
func (t *TextBox) Bounds() image.Rectangle {
	b := t.Base.Bounds()
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		return b.Union(t.contextMenu.Bounds())
	}
	return b
}

// HasOverlay — контекстное меню открыто.
func (t *TextBox) HasOverlay() bool {
	return t.contextMenu != nil && t.contextMenu.IsOpen()
}

// DrawOverlay рисует контекстное меню поверх всего UI.
func (t *TextBox) DrawOverlay(ctx DrawContext) {
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		t.contextMenu.DrawOverlay(ctx)
	}
}

// OverlayBounds возвращает прямоугольник открытого контекстного меню
// (для выноса в нативное окно). Реализует widget.OverlayBoundsProvider.
func (t *TextBox) OverlayBounds() image.Rectangle {
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		return t.contextMenu.OverlayBounds()
	}
	return image.Rectangle{}
}

// Dismiss закрывает контекстное меню (Dismissable).
func (t *TextBox) Dismiss() {
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		t.contextMenu.Close()
	}
}

// ─── Themeable ───────────────────────────────────────────────────────────────

func (t *TextBox) ApplyTheme(th *Theme) {
	t.Background = th.InputBG
	t.BorderColor = th.InputBorder
	t.FocusBorder = th.InputFocus
	t.TextColor = th.InputText
	t.PlaceColor = th.InputPlaceholder
	t.CaretColor = th.InputCaret
	t.SelColor = premulAlpha(th.Accent, 110)
}
