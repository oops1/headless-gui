package widget

import (
	"image"
	"strings"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/widget/diffview"
	"github.com/oops1/headless-gui/v3/widget/mergeview"
)

// mergeview.go — контрол трёхстороннего слияния: наше, база, их и итог.
//
// Пара к DiffView и его же родня по виду: те же карточки, шрифт, подсветка и
// палитра. Отличие в сути. DiffView сравнивает две стороны, и править можно
// обе; здесь три стороны только показываются, а правится итог — он и есть то,
// что приложение потом запишет в файл.
//
// Строки сторон выровнены по блокам: блок занимает в каждой панели одинаковое
// число экранных строк, недостающие — пустое место. Поэтому прокрутка у трёх
// верхних панелей одна, без кусочно-линейного отображения, как в DiffView.

// MergeSide — панель контрола слияния.
type MergeSide int

const (
	// MergeOurs — наша сторона (HEAD).
	MergeOurs MergeSide = iota
	// MergeBase — общий предок.
	MergeBase
	// MergeTheirs — их сторона (сливаемая ветка).
	MergeTheirs
	// MergeResult — итог: единственная панель, которую можно править.
	MergeResult
)

// MergeChunk — блок слияния. Псевдоним модели: контролу можно отдать блоки,
// посчитанные приложением (git merge-file), — см. SetChunks.
type MergeChunk = mergeview.Chunk

// MergeResolution — чем закрыт конфликтный блок.
type MergeResolution = mergeview.Resolution

// Решения по конфликту.
const (
	MergeUnresolved         = mergeview.Unresolved
	MergeTakeOurs           = mergeview.TakeOurs
	MergeTakeTheirs         = mergeview.TakeTheirs
	MergeTakeBase           = mergeview.TakeBase
	MergeTakeOursThenTheirs = mergeview.TakeOursThenTheirs
	MergeTakeTheirsThenOurs = mergeview.TakeTheirsThenOurs
)

// MergeStyle — как пишется нерешённый конфликт в итоге.
type MergeStyle = mergeview.Style

// Стили маркеров конфликта.
const (
	MergeStyleMerge = mergeview.StyleMerge
	MergeStyleDiff3 = mergeview.StyleDiff3
)

// MergeSideInfo — подписи панели: заголовок (обычно ветка) и примечание (обычно
// путь файла) в строке шапки, и пояснение — «что это за сторона» — второй
// строкой мелким приглушённым шрифтом.
type MergeSideInfo struct {
	Title, Note string
	// Hint — пояснение под заголовком. Шапки верхних панелей растут на строку,
	// когда пояснение есть хоть у одной видимой стороны: иначе код в соседних
	// панелях начинался бы на разной высоте. Шапка итога растёт по своему.
	Hint string
}

// mvSpan — экранные строки блока: [from, to) — одинаково во всех трёх верхних
// панелях, потому что блок выровнен по самой длинной стороне.
type mvSpan struct{ from, to int }

// mvHist — шаг истории: строки итога, решения и каретка.
type mvHist struct {
	lines  []string
	res    []MergeResolution
	spans  [][2]int
	caret  dvPos
	anchor dvPos
}

// mvStamp — состояние для сравнения «до и после» операции (события).
type mvStamp struct {
	rev, unresolved, current int
	caret                    dvPos
}

// MergeView — контрол трёхстороннего слияния.
type MergeView struct {
	Base
	mu sync.Mutex

	docs   [4]*dvDoc
	sides  [3]MergeSideInfo
	result MergeSideInfo // подписи итога: заголовок, примечание, пояснение
	chunks []MergeChunk
	res    []MergeResolution
	spans  []mvSpan  // блок → экранные строки верхних панелей
	rspan  [][2]int  // блок → строки итога
	rrows  []int     // строка итога → блок (-1 у строки вне блоков)
	rows   int       // всего экранных строк сверху
	pal    dvPalette // палитра DiffView: цвета и карточки общие

	showBase, syntax bool
	style            MergeStyle
	labels           mergeview.Labels
	markerSize       int // длина маркера конфликта; 0 — семь знаков

	monoFont, boldFont string
	fontSize           float64

	scroll, rscroll, hscroll, charW float64
	syncScroll                      bool    // верх и итог прокручиваются вместе, по блокам
	resultDrives                    bool    // ведущая часть — итог (её прокрутили или правили последней)
	split                           float64 // доля высоты под верхние панели

	active                    MergeSide
	current, hoverBtn         int
	hoverChunk                int
	focused, dragSel          bool
	dragSide                  MergeSide
	splitDrag                 bool
	rulerDrag                 bool
	rulerGrab                 int // точка захвата ползунка от его верха; -1 — нажали мимо
	lastClickAt               time.Time
	lastClickPt               image.Point
	clicks                    int
	undo, redo                []mvHist
	lastEdit                  dvEditKind
	lastEditAt                time.Time
	revSeq                    int
	resolvedEvent, editsEvent bool

	menu     *PopupMenu
	pending  []func()
	commands map[string]ICommand

	// События. Зовутся без замков контрола.
	OnResolvedChanged  func(unresolved int)
	OnResultEdited     func()
	OnCurrentConflict  func(index int)
	OnCaretMoved       func(line, col int)
	OnActiveSideChange func(side MergeSide)
	OnSaveRequest      func() // Ctrl+S
	OnError            func(err error)
}

// NewMergeView создаёт контрол слияния. monoFont — шрифт кода (пусто —
// встроенный Go Mono), boldFont — шрифт заголовков панелей (пусто —
// встроенный жирный).
func NewMergeView(monoFont, boldFont string) *MergeView {
	if monoFont == "" {
		monoFont = BuiltinFontMono
	}
	if boldFont == "" {
		boldFont = BuiltinFontBold
	}
	t := CurrentTheme()
	if t == nil {
		t = Win11LightTheme()
	}
	m := &MergeView{
		pal:        dvPaletteFrom(t),
		showBase:   true,
		syntax:     true,
		monoFont:   monoFont,
		boldFont:   boldFont,
		fontSize:   10,
		charW:      8,
		split:      0.58,
		syncScroll: true,
		current:    -1,
		hoverChunk: -1,
		hoverBtn:   -1,
		active:     MergeResult,
		menu:       NewPopupMenu(),
	}
	for i := range m.docs {
		m.docs[i] = newDvDoc()
	}
	m.docs[MergeOurs].readOnly = true
	m.docs[MergeBase].readOnly = true
	m.docs[MergeTheirs].readOnly = true
	// Итог по умолчанию кончается переводом строки: так кончаются почти все
	// файлы в репозитории, а блоки (SetChunks) приходят строками без переводов,
	// и без этого решённый конфликт записывался бы с «No newline at end of
	// file». Другой вид задаёт SetResultEOL; SetTexts берёт его у нашей стороны.
	m.docs[MergeResult].text.EOL = "\n"
	m.docs[MergeResult].text.FinalNL = true
	m.sides = [3]MergeSideInfo{{Title: "ours"}, {Title: "base"}, {Title: "theirs"}}
	// Меню — ребёнок контрола: так движок рисует его оверлей и отдаёт ему
	// клики (контракт ContextMenuProvider).
	m.AddChild(m.menu)
	m.rebuildLocked()
	return m
}

// ─── События ────────────────────────────────────────────────────────────────

func (m *MergeView) stampLocked() mvStamp {
	return mvStamp{
		rev:        m.docs[MergeResult].rev,
		unresolved: mergeview.UnresolvedCount(m.chunks, m.res),
		current:    m.current,
		caret:      m.docs[m.active].caret,
	}
}

func (m *MergeView) emit(f func()) { m.pending = append(m.pending, f) }

func (m *MergeView) queueEventsLocked(a mvStamp) {
	b := m.stampLocked()
	if a.rev != b.rev || m.editsEvent {
		m.editsEvent = false
		if f := m.OnResultEdited; f != nil {
			m.emit(f)
		}
		m.emitCommandLocked("ResultEditedCommand", nil)
	}
	if a.unresolved != b.unresolved || m.resolvedEvent {
		m.resolvedEvent = false
		if f, n := m.OnResolvedChanged, b.unresolved; f != nil {
			m.emit(func() { f(n) })
		}
		m.emitCommandLocked("ResolvedCommand", b.unresolved)
	}
	if a.current != b.current {
		if f, n := m.OnCurrentConflict, m.ordinalLocked(b.current); f != nil {
			m.emit(func() { f(n) })
		}
	}
	if a.caret != b.caret {
		if f, c := m.OnCaretMoved, b.caret; f != nil {
			m.emit(func() { f(c.line, c.col) })
		}
	}
}

// do выполняет fn под замком и рассылает события уже без него: обработчик
// вправе звать методы контрола, и замок, взятый второй раз, повесил бы его.
func (m *MergeView) do(fn func()) {
	m.mu.Lock()
	st := m.stampLocked()
	fn()
	m.queueEventsLocked(st)
	ev := m.pending
	m.pending = nil
	m.mu.Unlock()
	m.Invalidate()
	for _, f := range ev {
		f()
	}
}

func (m *MergeView) nextRev() int {
	m.revSeq++
	return m.revSeq
}

// SetCommand привязывает команду разметки к событию контрола:
//
//	SaveCommand          — Ctrl+S, без параметра;
//	ResultEditedCommand  — итог изменился, без параметра;
//	ResolvedCommand      — решён или отменён конфликт, параметр — сколько
//	                       нерешённых осталось.
//
// Возвращает false для неизвестного имени.
func (m *MergeView) SetCommand(name string, cmd ICommand) bool {
	switch name {
	case "SaveCommand", "ResultEditedCommand", "ResolvedCommand":
	default:
		return false
	}
	m.mu.Lock()
	if m.commands == nil {
		m.commands = map[string]ICommand{}
	}
	m.commands[name] = cmd
	m.mu.Unlock()
	return true
}

func (m *MergeView) emitCommandLocked(name string, param any) {
	cmd := m.commands[name]
	if cmd == nil {
		return
	}
	m.emit(func() {
		if cmd.CanExecute(param) {
			cmd.Execute(param)
		}
	})
}

// ─── Содержимое ─────────────────────────────────────────────────────────────

// SetSides задаёт подписи панелей: заголовок (обычно ветка) и примечание
// (обычно имя файла). Подписи уходят и в маркеры конфликта — там git пишет
// имена ветвей.
func (m *MergeView) SetSides(ours, base, theirs MergeSideInfo) {
	m.do(func() {
		m.sides = [3]MergeSideInfo{ours, base, theirs}
		for i, s := range m.sides {
			m.docs[i].title, m.docs[i].note = s.Title, s.Note
		}
		m.labels = mergeview.Labels{Ours: ours.Title, Base: base.Title, Theirs: theirs.Title}
		m.respliceUnresolvedLocked()
		m.rebuildLocked()
	})
}

// Sides возвращает подписи трёх сторон, заданные SetSides.
func (m *MergeView) Sides() [3]MergeSideInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sides
}

// SetResultInfo задаёт подписи панели итога — как SetSides у сторон.
// Пустой заголовок — прежний, из ключа merge.side.result. В маркеры конфликта
// подписи итога не уходят: там git пишет стороны, а не итог.
func (m *MergeView) SetResultInfo(info MergeSideInfo) {
	m.do(func() {
		m.result = info
		r := m.docs[MergeResult]
		r.title, r.note = info.Title, info.Note
		// Пояснение меняет высоту шапки итога, а с ней — сколько строк видно.
		m.clampScrollLocked()
	})
}

// ResultInfo возвращает подписи панели итога.
func (m *MergeView) ResultInfo() MergeSideInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.result
}

// SetChunks задаёт блоки слияния готовыми.
//
// Это главный путь: у приложения, которое уже слило файл (git merge-file),
// итог обязан совпасть с записанным git байт в байт, а пересчёт своим
// алгоритмом дал бы другое разбиение. Решения по конфликтам сбрасываются.
func (m *MergeView) SetChunks(chunks []MergeChunk) {
	m.do(func() {
		m.chunks = append([]MergeChunk(nil), chunks...)
		m.res = make([]MergeResolution, len(m.chunks))
		m.resetHistoryLocked()
		m.setSideTextsLocked()
		m.buildResultLocked()
		m.rebuildLocked()
		m.resolvedEvent = true
		// Текущего конфликта нет: первый F7 (или NextConflict) встаёт на
		// первый, как переход к изменению в сравнении.
		m.current = -1
	})
}

// SetTexts считает блоки сам — для демонстраций и простых случаев; тексты
// разбиваются на строки и сравниваются с базой (см. mergeview.Merge).
func (m *MergeView) SetTexts(base, ours, theirs string) {
	b := diffview.Decode([]byte(base))
	o := diffview.Decode([]byte(ours))
	t := diffview.Decode([]byte(theirs))
	m.SetChunks(mergeview.Merge(b.Lines, o.Lines, t.Lines, false))
	m.do(func() {
		// Перевод строки и BOM итога берём у нашей стороны: файл в рабочем
		// каталоге — наш, и записан он будет по-нашему.
		r := m.docs[MergeResult]
		r.text.EOL, r.text.BOM, r.text.FinalNL = o.EOL, o.BOM, o.FinalNL
	})
}

// Chunks возвращает копию блоков слияния.
func (m *MergeView) Chunks() []MergeChunk {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MergeChunk(nil), m.chunks...)
}

// setSideTextsLocked собирает тексты трёх верхних панелей из блоков.
func (m *MergeView) setSideTextsLocked() {
	var ours, base, theirs []string
	for _, c := range m.chunks {
		ours = append(ours, c.Ours...)
		base = append(base, c.Base...)
		theirs = append(theirs, c.Theirs...)
	}
	m.docs[MergeOurs].text.Lines = nonEmptyLines(ours)
	m.docs[MergeBase].text.Lines = nonEmptyLines(base)
	m.docs[MergeTheirs].text.Lines = nonEmptyLines(theirs)
}

func nonEmptyLines(l []string) []string {
	if len(l) == 0 {
		return []string{""}
	}
	return l
}

// buildResultLocked собирает итог из блоков и решений, запоминая, какие строки
// какому блоку принадлежат.
func (m *MergeView) buildResultLocked() {
	var lines []string
	m.rspan = m.rspan[:0]
	for i, c := range m.chunks {
		from := len(lines)
		lines = append(lines, m.chunkResultLocked(i, c)...)
		m.rspan = append(m.rspan, [2]int{from, len(lines)})
	}
	r := m.docs[MergeResult]
	r.text.Lines = nonEmptyLines(lines)
	r.caret, r.anchor = r.clamp(r.caret), r.clamp(r.anchor)
	r.rev = m.nextRev()
}

// chunkResultLocked — строки блока в итоге: решение, а у нерешённого
// конфликта — маркеры git.
func (m *MergeView) chunkResultLocked(i int, c MergeChunk) []string {
	if !c.Conflict {
		return c.Result()
	}
	var res []MergeResolution
	if i < len(m.res) {
		res = []MergeResolution{m.res[i]}
	}
	return mergeview.Render([]MergeChunk{c}, res, m.formatLocked())
}

// ─── Решения ────────────────────────────────────────────────────────────────

// Resolve закрывает конфликтный блок выбранной стороной. MergeUnresolved
// возвращает блок в нерешённое состояние — в итоге снова появятся маркеры.
// Индекс — по списку блоков (Chunks); неконфликтный блок решения не требует.
func (m *MergeView) Resolve(index int, how MergeResolution) {
	m.do(func() { m.resolveLocked(index, how) })
}

func (m *MergeView) resolveLocked(index int, how MergeResolution) {
	if index < 0 || index >= len(m.chunks) || !m.chunks[index].Conflict {
		return
	}
	if m.res[index] == how {
		return
	}
	m.pushUndoLocked(dvEditOther)
	m.res[index] = how

	// Заменяем строки блока на месте: остальной итог (в том числе правки
	// руками) остаётся как есть.
	r := m.docs[MergeResult]
	sp := m.rspan[index]
	ins := m.chunkResultLocked(index, m.chunks[index])
	r.text.Lines = dvSplice(r.text.Lines, sp[0], sp[1], ins)
	if len(r.text.Lines) == 0 {
		r.text.Lines = []string{""}
	}
	delta := len(ins) - (sp[1] - sp[0])
	m.rspan[index][1] = sp[1] + delta
	for i := index + 1; i < len(m.rspan); i++ {
		m.rspan[i][0] += delta
		m.rspan[i][1] += delta
	}
	r.rev = m.nextRev()
	r.caret, r.anchor = r.clamp(dvPos{sp[0], 0}), r.clamp(dvPos{sp[0], 0})
	m.current = index
	m.resolvedEvent = true
	m.rebuildLocked()
	m.ensureResultVisibleLocked()
}

// ResolveCurrent закрывает текущий конфликт (тот, на который встала навигация).
func (m *MergeView) ResolveCurrent(how MergeResolution) {
	m.do(func() { m.resolveLocked(m.current, how) })
}

// ResolveAll закрывает ВСЕ нерешённые конфликты одной стороной.
func (m *MergeView) ResolveAll(how MergeResolution) {
	m.do(func() {
		for i, c := range m.chunks {
			if c.Conflict && !m.res[i].Resolved() {
				m.resolveLocked(i, how)
			}
		}
	})
}

// Resolution возвращает решение по блоку.
func (m *MergeView) Resolution(index int) MergeResolution {
	m.mu.Lock()
	defer m.mu.Unlock()
	if index < 0 || index >= len(m.res) {
		return MergeUnresolved
	}
	return m.res[index]
}

// Unresolved — сколько конфликтов ещё не закрыто.
func (m *MergeView) Unresolved() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return mergeview.UnresolvedCount(m.chunks, m.res)
}

// ConflictCount — сколько конфликтных блоков всего (решённые тоже).
func (m *MergeView) ConflictCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return mergeview.Conflicts(m.chunks)
}

// Result возвращает итог одним текстом: перевод строки, BOM и перевод в конце —
// как задано SetResultEOL (у SetTexts — как у нашей стороны). Нерешённые
// конфликты записаны маркерами git.
func (m *MergeView) Result() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.docs[MergeResult].text
	// Пустое слияние — пустой файл, а не одинокий перевод строки: буфер держит
	// одну пустую строку только потому, что каретке нужно где-то стоять.
	if len(t.Lines) == 1 && t.Lines[0] == "" && m.resultLinesLocked() == 0 {
		t.FinalNL = false
	}
	return string(t.Encode())
}

// resultLinesLocked — сколько строк итога принадлежит блокам.
func (m *MergeView) resultLinesLocked() int {
	n := 0
	for _, sp := range m.rspan {
		n += sp[1] - sp[0]
	}
	return n
}

// SetResultEOL задаёт, как записывается итог: перевод строки ("\n" или
// "\r\n"; пусто — "\n"), BOM в начале и перевод строки в конце файла.
//
// Блоки (SetChunks) приходят строками без переводов, и сказать контролу, чем
// кончался файл, больше нечем. Настройка переживает SetChunks — её задают
// один раз на файл; SetTexts берёт её у нашей стороны сам.
func (m *MergeView) SetResultEOL(eol string, bom, finalNL bool) {
	if eol == "" {
		eol = "\n"
	}
	m.do(func() {
		r := m.docs[MergeResult]
		r.text.EOL, r.text.BOM, r.text.FinalNL = eol, bom, finalNL
	})
}

// ResultEOL возвращает настройку записи итога (см. SetResultEOL).
func (m *MergeView) ResultEOL() (eol string, bom, finalNL bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.docs[MergeResult].text
	return t.EOL, t.BOM, t.FinalNL
}

// ResultLines возвращает строки итога.
func (m *MergeView) ResultLines() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.docs[MergeResult].text.Lines...)
}

// SetStyle задаёт стиль маркеров нерешённого конфликта: merge или diff3.
func (m *MergeView) SetStyle(s MergeStyle) {
	m.do(func() {
		if m.style == s {
			return
		}
		m.style = s
		m.respliceUnresolvedLocked()
	})
}

// formatLocked — настройки сборки итога: стиль, подписи и длина маркеров.
func (m *MergeView) formatLocked() mergeview.Format {
	return mergeview.Format{Style: m.style, Labels: m.labels, MarkerSize: m.markerSize}
}

// SetMarkerSize задаёт длину маркеров конфликта. git берёт её из атрибута
// conflict-marker-size, и репозиторий вправе поставить другую — нерешённый
// конфликт, записанный из окна, должен совпасть с тем, что написал бы git.
// 0 и меньше — семь знаков, как у git по умолчанию.
func (m *MergeView) SetMarkerSize(n int) {
	m.do(func() {
		n = max(0, n)
		if m.markerSize == n {
			return
		}
		m.markerSize = n
		m.respliceUnresolvedLocked()
	})
}

// MarkerSize возвращает длину маркеров конфликта (семь, если не задана).
func (m *MergeView) MarkerSize() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.markerSize <= 0 {
		return len(mergeview.MarkerOurs)
	}
	return m.markerSize
}

// respliceUnresolvedLocked переписывает в итоге строки НЕРЕШЁННЫХ конфликтов:
// маркеры зависят от стиля, подписей и длины. Остальной итог, в том числе
// правки руками, не трогается — пересборка целиком их стёрла бы, а подписи
// сторон, например, меняются при каждой смене языка.
func (m *MergeView) respliceUnresolvedLocked() {
	r := m.docs[MergeResult]
	changed := false
	for i, c := range m.chunks {
		if !c.Conflict || i >= len(m.res) || m.res[i].Resolved() || i >= len(m.rspan) {
			continue
		}
		sp := m.rspan[i]
		ins := m.chunkResultLocked(i, c)
		r.text.Lines = dvSplice(r.text.Lines, sp[0], sp[1], ins)
		delta := len(ins) - (sp[1] - sp[0])
		m.rspan[i][1] = sp[1] + delta
		for k := i + 1; k < len(m.rspan); k++ {
			m.rspan[k][0] += delta
			m.rspan[k][1] += delta
		}
		changed = true
	}
	if !changed {
		return
	}
	if len(r.text.Lines) == 0 {
		r.text.Lines = []string{""}
	}
	r.caret, r.anchor = r.clamp(r.caret), r.clamp(r.anchor)
	r.rev = m.nextRev()
	m.rebuildLocked()
}

// ─── Навигация по конфликтам ────────────────────────────────────────────────

// CurrentConflict — порядковый номер текущего конфликта (от 0) или -1.
func (m *MergeView) CurrentConflict() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ordinalLocked(m.current)
}

// ordinalLocked переводит индекс блока в номер конфликта по порядку.
func (m *MergeView) ordinalLocked(index int) int {
	if index < 0 || index >= len(m.chunks) || !m.chunks[index].Conflict {
		return -1
	}
	n := 0
	for i := 0; i < index; i++ {
		if m.chunks[i].Conflict {
			n++
		}
	}
	return n
}

// nextConflictLocked ищет ближайший конфликтный блок от from в сторону dir.
func (m *MergeView) nextConflictLocked(from, dir int) int {
	for i := from + dir; i >= 0 && i < len(m.chunks); i += dir {
		if m.chunks[i].Conflict {
			return i
		}
	}
	return from
}

// NextConflict переходит к следующему конфликту.
func (m *MergeView) NextConflict() {
	m.do(func() { m.goToChunkLocked(m.nextConflictLocked(m.current, +1)) })
}

// PrevConflict переходит к предыдущему конфликту.
func (m *MergeView) PrevConflict() {
	m.do(func() {
		from := m.current
		if from < 0 {
			from = len(m.chunks)
		}
		m.goToChunkLocked(m.nextConflictLocked(from, -1))
	})
}

// GoToConflict переходит к конфликту по порядковому номеру.
func (m *MergeView) GoToConflict(n int) {
	m.do(func() {
		k := 0
		for i, c := range m.chunks {
			if !c.Conflict {
				continue
			}
			if k == n {
				m.goToChunkLocked(i)
				return
			}
			k++
		}
	})
}

func (m *MergeView) goToChunkLocked(index int) {
	if index < 0 || index >= len(m.chunks) {
		return
	}
	m.current = index
	sp := m.spans[index]
	// Блок встаёт на треть высоты панели — так видно и то, что было до него.
	viewH := float64(m.geom().ty1 - m.geom().ty0)
	m.setScrollLocked(float64(dvTopPad+sp.from*dvLineH) - viewH/3)
	r := m.docs[MergeResult]
	r.caret = r.clamp(dvPos{m.rspan[index][0], 0})
	r.anchor = r.caret
	m.ensureResultVisibleLocked()
}

// ─── Вид ────────────────────────────────────────────────────────────────────

// SetShowBase показывает или прячет панель базы: без неё остаются «наше», «их»
// и итог — так смотрят конфликт, когда предок не нужен.
func (m *MergeView) SetShowBase(v bool) {
	m.do(func() {
		if m.showBase == v {
			return
		}
		m.showBase = v
		m.rebuildLocked()
	})
}

// ShowBase сообщает, показана ли панель базы.
func (m *MergeView) ShowBase() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.showBase
}

// SetSyntaxHighlight включает подсветку синтаксиса.
func (m *MergeView) SetSyntaxHighlight(v bool) {
	m.do(func() {
		if m.syntax == v {
			return
		}
		m.syntax = v
		m.rebuildLocked()
	})
}

// SetFont задаёт шрифты и кегль кода.
func (m *MergeView) SetFont(monoFont, boldFont string, sizePt float64) {
	m.do(func() {
		if monoFont != "" {
			m.monoFont = monoFont
		}
		if boldFont != "" {
			m.boldFont = boldFont
		}
		if sizePt > 0 {
			m.fontSize = sizePt
		}
	})
}

// SetReadOnly запрещает или разрешает правку итога.
func (m *MergeView) SetReadOnly(v bool) {
	m.do(func() { m.docs[MergeResult].readOnly = v })
}

// IsReadOnly сообщает, запрещена ли правка итога.
func (m *MergeView) IsReadOnly() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.docs[MergeResult].readOnly
}

// ActiveSide — панель, в которую идёт ввод.
func (m *MergeView) ActiveSide() MergeSide {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

// SetActiveSide переводит ввод в панель.
func (m *MergeView) SetActiveSide(side MergeSide) {
	m.do(func() {
		if side < MergeOurs || side > MergeResult || m.active == side {
			return
		}
		m.active = side
		if f := m.OnActiveSideChange; f != nil {
			m.emit(func() { f(side) })
		}
	})
}

// Caret — положение каретки в итоге (строка и колонка от нуля).
func (m *MergeView) Caret() (line, col int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.docs[MergeResult].caret
	return c.line, c.col
}

// SetCaret ставит каретку в итоге.
func (m *MergeView) SetCaret(line, col int) {
	m.do(func() {
		r := m.docs[MergeResult]
		r.caret = r.clamp(dvPos{line, col})
		r.anchor = r.caret
		m.active = MergeResult
		m.ensureResultVisibleLocked()
	})
}

// ─── Модель отображения ─────────────────────────────────────────────────────

// rebuildLocked пересобирает экранные строки: блок занимает в каждой верхней
// панели одинаковое число строк, недостающие — пустое место (dvRowPad).
func (m *MergeView) rebuildLocked() {
	for i := 0; i < 4; i++ {
		dvPrepareDoc(m.docs[i], m.syntax)
	}
	m.spans = m.spans[:0]
	var line [3]int
	rows := 0
	for ci, c := range m.chunks {
		n := max(len(c.Ours), len(c.Theirs))
		if m.showBase {
			n = max(n, len(c.Base))
		}
		sp := mvSpan{from: rows, to: rows + n}
		for si, sideLines := range [3][]string{c.Ours, c.Base, c.Theirs} {
			s := m.docs[si]
			for i := 0; i < n; i++ {
				if i < len(sideLines) {
					s.rows = append(s.rows, dvRow{dvLineKind(sideLines[i]), line[si] + i, ci})
					continue
				}
				s.rows = append(s.rows, dvRow{dvRowPad, 0, ci})
			}
			line[si] += len(sideLines)
		}
		m.spans = append(m.spans, sp)
		rows += n
	}
	m.rows = rows

	for i := 0; i < 3; i++ {
		s := m.docs[i]
		for r, row := range s.rows {
			if row.kind != dvRowPad {
				s.lineRow[row.line] = r
			}
		}
		dvBuildCards(s)
	}

	// Итог: строка на строку, плюс карта «строка → блок» для подкраски.
	r := m.docs[MergeResult]
	m.rrows = m.rrows[:0]
	for i := range r.text.Lines {
		ci := -1
		for k, sp := range m.rspan {
			if i >= sp[0] && i < sp[1] {
				ci = k
				break
			}
		}
		m.rrows = append(m.rrows, ci)
		r.rows = append(r.rows, dvRow{dvLineKind(r.text.Lines[i]), i, ci})
		r.lineRow[i] = i
	}
	dvBuildCards(r)

	if m.current >= len(m.chunks) {
		m.current = -1
	}
	m.hoverChunk, m.hoverBtn = -1, -1
	m.clampScrollLocked()
}

// resultChunkAtLocked — блок, которому принадлежит строка итога.
func (m *MergeView) resultChunkAtLocked(line int) int {
	if line < 0 || line >= len(m.rrows) {
		return -1
	}
	return m.rrows[line]
}

// adjustSpansLocked разносит правку итога по блокам: строки, добавленные или
// убранные вручную, достаются блоку, внутри которого стояла каретка.
func (m *MergeView) adjustSpansLocked(atLine, delta int) {
	if delta == 0 || len(m.rspan) == 0 {
		return
	}
	ci := m.resultChunkAtLocked(atLine)
	if ci < 0 {
		// Правка вне блоков (в самом конце) — растягиваем последний.
		ci = len(m.rspan) - 1
	}
	m.rspan[ci][1] += delta
	for i := ci + 1; i < len(m.rspan); i++ {
		m.rspan[i][0] += delta
		m.rspan[i][1] += delta
	}
}

// ─── Геометрия и прокрутка ──────────────────────────────────────────────────

// mvGeom — раскладка: x-границы панелей, полосы содержимого верха и итога.
type mvGeom struct {
	b          image.Rectangle
	px         [3][2]int // x0,x1 верхних панелей; у скрытой базы — пустой
	ty0, ty1   int       // содержимое верхних панелей
	ry0, ry1   int       // содержимое итога
	splitY     int       // разделитель между верхом и итогом
	rulerX     int
	panes      int
	headerTopY int
	headerResY int
	headerTopH int // высота шапок верхних панелей (с пояснением выше)
	headerResH int // высота шапки итога
}

const (
	mvSplitH  = 8  // толщина разделителя
	mvHeaderH = 34 // высота заголовка панели
	mvGutter  = 10 // промежуток между панелями
	mvHintH   = 16 // строка пояснения в шапке
)

// mvHeaderHeight — высота шапки панели: с пояснением на строку выше.
func mvHeaderHeight(hint bool) int {
	if hint {
		return mvHeaderH + mvHintH
	}
	return mvHeaderH
}

// topHeaderH — высота шапок верхних панелей: общая для всех видимых сторон,
// чтобы код в соседних панелях начинался на одной высоте.
func (m *MergeView) topHeaderH() int {
	for i, s := range m.sides {
		if i == int(MergeBase) && !m.showBase {
			continue
		}
		if s.Hint != "" {
			return mvHeaderHeight(true)
		}
	}
	return mvHeaderH
}

func (m *MergeView) geom() mvGeom {
	b := m.Bounds()
	g := mvGeom{b: b}
	g.panes = 3
	if !m.showBase {
		g.panes = 2
	}
	g.rulerX = b.Max.X - dvOuterPad - dvRulerW

	inner := image.Rect(b.Min.X+dvOuterPad, b.Min.Y+dvOuterPad, g.rulerX-6, b.Max.Y-dvOuterPad)
	splitY := inner.Min.Y + int(float64(inner.Dy())*m.split)
	g.splitY = splitY

	w := (inner.Dx() - mvGutter*(g.panes-1)) / g.panes
	x := inner.Min.X
	for i := 0; i < 3; i++ {
		if i == int(MergeBase) && !m.showBase {
			g.px[i] = [2]int{0, 0}
			continue
		}
		g.px[i] = [2]int{x, x + w}
		x += w + mvGutter
	}
	g.headerTopY = inner.Min.Y + 4
	g.headerTopH = m.topHeaderH()
	g.ty0 = g.headerTopY + g.headerTopH + 6
	g.ty1 = splitY - mvSplitH/2
	g.headerResY = splitY + mvSplitH/2 + 4
	g.headerResH = mvHeaderHeight(m.result.Hint != "")
	g.ry0 = g.headerResY + g.headerResH + 6
	g.ry1 = inner.Max.Y
	return g
}

// resultBounds — прямоугольник панели итога.
func (g mvGeom) resultBounds() image.Rectangle {
	return image.Rect(g.px[0][0], g.ry0, g.b.Max.X-dvOuterPad-dvRulerW-6, g.ry1)
}

func (m *MergeView) topViewH() float64 {
	g := m.geom()
	return float64(max(0, g.ty1-g.ty0))
}

func (m *MergeView) maxScrollLocked() float64 {
	h := float64(dvTopPad + m.rows*dvLineH + dvBottomPad)
	return max(0, h-m.topViewH())
}

func (m *MergeView) maxResultScrollLocked() float64 {
	g := m.geom()
	h := float64(dvTopPad + len(m.docs[MergeResult].rows)*dvLineH + dvBottomPad)
	return max(0, h-float64(max(0, g.ry1-g.ry0)))
}

// setScrollLocked прокручивает верхние панели; при синхронной прокрутке итог
// следует за ними.
func (m *MergeView) setScrollLocked(v float64) {
	m.scroll = min(max(0, v), m.maxScrollLocked())
	m.resultDrives = false
	if m.syncScroll {
		m.followLocked()
	}
}

// setResultScrollLocked прокручивает итог; при синхронной прокрутке верхние
// панели следуют за ним.
func (m *MergeView) setResultScrollLocked(v float64) {
	m.rscroll = min(max(0, v), m.maxResultScrollLocked())
	m.resultDrives = true
	if m.syncScroll {
		m.followLocked()
	}
}

// clampScrollLocked держит обе прокрутки в пределах после перестройки (решение,
// правка, смена раскладки) и заново ставит ведомую часть вровень с ведущей.
func (m *MergeView) clampScrollLocked() {
	m.scroll = min(max(0, m.scroll), m.maxScrollLocked())
	m.rscroll = min(max(0, m.rscroll), m.maxResultScrollLocked())
	if m.syncScroll {
		m.followLocked()
	}
}

// resultViewHLocked — высота видимой области итога.
func (m *MergeView) resultViewHLocked() float64 {
	g := m.geom()
	return float64(max(0, g.ry1-g.ry0))
}

// scrollBPsLocked — точки излома соответствия «точка верха ↔ точка итога»:
// начала и концы блоков. Между ними соответствие линейное — блок, который
// сверху занимает строку, а в итоге пять строк маркеров, растягивается, и
// соседние блоки всё равно встают вровень.
func (m *MergeView) scrollBPsLocked() (top, res []float64) {
	top = append(top, 0, dvTopPad)
	res = append(res, 0, dvTopPad)
	for ci, sp := range m.spans {
		if ci >= len(m.rspan) {
			break
		}
		top = append(top, float64(dvTopPad+sp.to*dvLineH))
		res = append(res, float64(dvTopPad+m.rspan[ci][1]*dvLineH))
	}
	top = append(top, m.virtualHLocked())
	res = append(res, float64(dvTopPad+len(m.docs[MergeResult].rows)*dvLineH+dvBottomPad))
	return top, res
}

// mvMapPoint переводит точку v по ломаной from → to. Отрезки нулевой длины на
// стороне from пропускаются: блок без строк сверху (правка только в итоге)
// точкой соответствия не служит.
func mvMapPoint(v float64, from, to []float64) float64 {
	n := min(len(from), len(to))
	if n == 0 {
		return v
	}
	if v <= from[0] {
		return to[0]
	}
	for i := 0; i+1 < n; i++ {
		if v > from[i+1] || from[i+1] <= from[i] {
			continue
		}
		t := (v - from[i]) / (from[i+1] - from[i])
		return to[i] + t*(to[i+1]-to[i])
	}
	return to[n-1]
}

// followLocked ставит ведомую часть вровень с ведущей.
//
// Опорная точка едет от верха окна к низу вместе с прокруткой (доля f), как в
// сравнении: у начала файла вровень стоят верхние края, у конца — нижние, и
// обе части доходят до своих концов одновременно, хотя итог длиннее на строки
// маркеров.
func (m *MergeView) followLocked() {
	top, res := m.scrollBPsLocked()
	vh, rvh := m.topViewH(), m.resultViewHLocked()
	if m.resultDrives {
		maxR := m.maxResultScrollLocked()
		f := 0.0
		if maxR > 0 {
			f = m.rscroll / maxR
		}
		p := mvMapPoint(m.rscroll+f*rvh, res, top)
		m.scroll = min(max(0, p-f*vh), m.maxScrollLocked())
		return
	}
	maxT := m.maxScrollLocked()
	f := 0.0
	if maxT > 0 {
		f = m.scroll / maxT
	}
	p := mvMapPoint(m.scroll+f*vh, top, res)
	m.rscroll = min(max(0, p-f*rvh), m.maxResultScrollLocked())
}

// SetSyncScroll включает или выключает синхронную прокрутку верха и итога.
// По умолчанию включена: листая итог к конфликту, человек ждёт наверху те же
// строки сторон. Выключенная — части прокручиваются порознь.
func (m *MergeView) SetSyncScroll(v bool) {
	m.do(func() {
		if m.syncScroll == v {
			return
		}
		m.syncScroll = v
		if v {
			m.followLocked()
		}
	})
}

// SyncScroll сообщает, прокручиваются ли верх и итог вместе.
func (m *MergeView) SyncScroll() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.syncScroll
}

// ensureResultVisibleLocked подкручивает итог так, чтобы каретка была видна.
func (m *MergeView) ensureResultVisibleLocked() {
	g := m.geom()
	viewH := float64(max(0, g.ry1-g.ry0))
	if viewH <= 0 {
		return
	}
	y := float64(dvTopPad + m.docs[MergeResult].caret.line*dvLineH)
	switch {
	case y < m.rscroll:
		m.setResultScrollLocked(y)
	case y+dvLineH > m.rscroll+viewH:
		m.setResultScrollLocked(y + dvLineH - viewH)
	}
}

// ─── Прочее ─────────────────────────────────────────────────────────────────

// ─── Полоса-обзор и прокрутка снаружи ───────────────────────────────────────

// virtualHLocked — полная высота содержимого верхних панелей в точках.
func (m *MergeView) virtualHLocked() float64 {
	return float64(dvTopPad + m.rows*dvLineH + dvBottomPad)
}

// rulerTrackLocked — полоса-обзор справа: от верхних панелей до низа итога.
func (m *MergeView) rulerTrackLocked(g mvGeom) image.Rectangle {
	return image.Rect(g.rulerX, g.ty0, g.rulerX+dvRulerW, g.ry1)
}

// rulerThumbLocked — ползунок видимой области верхних панелей. Пустой —
// прокручивать нечего.
func (m *MergeView) rulerThumbLocked(g mvGeom) image.Rectangle {
	tr := m.rulerTrackLocked(g)
	h := m.topViewH()
	if tr.Dy() <= 0 || h <= 0 || m.maxScrollLocked() <= 0 {
		return image.Rectangle{}
	}
	vs := float64(tr.Dy()) / m.virtualHLocked()
	ty := tr.Min.Y + int(m.scroll*vs)
	return image.Rect(tr.Min.X, ty, tr.Max.X, min(ty+max(18, int(h*vs)), tr.Max.Y))
}

// rulerMarkLocked — отметка блока на полосе. Масштаб тот же, что у ползунка, —
// точки содержимого: отметки по числу строк, а ползунок по точкам расходились
// бы на поля сверху и снизу, и щелчок по отметке попадал бы мимо блока.
func (m *MergeView) rulerMarkLocked(g mvGeom, ci int) image.Rectangle {
	tr := m.rulerTrackLocked(g)
	vs := float64(tr.Dy()) / m.virtualHLocked()
	sp := m.spans[ci]
	y0 := tr.Min.Y + int(float64(dvTopPad+sp.from*dvLineH)*vs)
	return image.Rect(tr.Min.X, y0, tr.Max.X, y0+max(3, int(float64((sp.to-sp.from)*dvLineH)*vs)))
}

// onRulerLocked — точка над полосой-обзором, с запасом: в тонкую полосу иначе
// трудно попасть.
func (m *MergeView) onRulerLocked(x, y int) bool {
	return image.Pt(x, y).In(m.rulerTrackLocked(m.geom()).Inset(-4))
}

// rulerPressLocked — нажатие на полосу-обзор. На ползунке — захват без скачка:
// панели сдвигаются ровно на столько, на сколько протянули. На отметке
// конфликта — переход к нему. Мимо — видимая область встаёт туда серединой.
func (m *MergeView) rulerPressLocked(y int) {
	g := m.geom()
	m.rulerDrag, m.rulerGrab = true, -1
	if th := m.rulerThumbLocked(g); !th.Empty() && y >= th.Min.Y && y < th.Max.Y {
		m.rulerGrab = y - th.Min.Y
		return
	}
	for ci, c := range m.chunks {
		if !c.Conflict {
			continue
		}
		if r := m.rulerMarkLocked(g, ci); y >= r.Min.Y-2 && y < r.Max.Y+2 {
			m.goToChunkLocked(ci)
			return
		}
	}
	m.scrollToRulerLocked(y)
}

// rulerDragLocked — протяжка по полосе после нажатия.
func (m *MergeView) rulerDragLocked(y int) {
	tr := m.rulerTrackLocked(m.geom())
	if tr.Dy() <= 0 {
		return
	}
	if m.rulerGrab >= 0 {
		vs := float64(tr.Dy()) / m.virtualHLocked()
		m.setScrollLocked(float64(y-m.rulerGrab-tr.Min.Y) / vs)
		return
	}
	m.scrollToRulerLocked(y)
}

// scrollToRulerLocked ставит серединой видимой области место полосы под y.
func (m *MergeView) scrollToRulerLocked(y int) {
	tr := m.rulerTrackLocked(m.geom())
	if tr.Dy() <= 0 {
		return
	}
	v := float64(y-tr.Min.Y) / float64(tr.Dy()) * m.virtualHLocked()
	m.setScrollLocked(v - m.topViewH()/2)
}

// Scroll — прокрутка верхних панелей в точках от начала.
func (m *MergeView) Scroll() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.scroll
}

// SetScroll прокручивает верхние панели на y точек от начала; значение за
// краями ограничивается.
func (m *MergeView) SetScroll(y float64) { m.do(func() { m.setScrollLocked(y) }) }

// ResultScroll — прокрутка итога в точках от начала.
func (m *MergeView) ResultScroll() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rscroll
}

// SetResultScroll прокручивает итог на y точек от начала.
func (m *MergeView) SetResultScroll(y float64) { m.do(func() { m.setResultScrollLocked(y) }) }

// ScrollToLine прокручивает панель так, чтобы строка буфера (от нуля) стояла на
// трети высоты: у сторон двигаются верхние панели, у итога — итог.
func (m *MergeView) ScrollToLine(side MergeSide, line int) {
	m.do(func() {
		if side < MergeOurs || side > MergeResult {
			return
		}
		s := m.docs[side]
		if line < 0 || line >= len(s.lineRow) {
			return
		}
		if side == MergeResult {
			g := m.geom()
			m.setResultScrollLocked(float64(dvTopPad+line*dvLineH) - float64(g.ry1-g.ry0)/3)
			return
		}
		if row := s.lineRow[line]; row >= 0 {
			m.setScrollLocked(float64(dvTopPad+row*dvLineH) - m.topViewH()/3)
		}
	})
}

// AccessInfo — контракт доступности: контрол как группа.
func (m *MergeView) AccessInfo() AccessInfo {
	return AccessInfo{
		Role:   RoleGroup,
		Name:   Tr("merge.a11y.view"),
		Bounds: m.Bounds(),
	}
}

// AccessChildren отдаёт скринридеру четыре панели: контрол рисует их сам, и
// без этого дерево доступности видело бы одну безымянную группу.
func (m *MergeView) AccessChildren() []AccessInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.geom()
	keys := [4]string{"merge.a11y.ours", "merge.a11y.base", "merge.a11y.theirs", "merge.a11y.result"}
	out := make([]AccessInfo, 0, 4)
	for i := 0; i < 4; i++ {
		var r image.Rectangle
		if i == int(MergeResult) {
			r = g.resultBounds()
		} else {
			if i == int(MergeBase) && !m.showBase {
				continue
			}
			r = image.Rect(g.px[i][0], g.ty0, g.px[i][1], g.ty1)
		}
		s := m.docs[i]
		name := s.title
		if name == "" {
			// Как в шапке: скринридер называет панель тем же словом, что видно
			// на экране, а не «(пусто)».
			name = Tr(mvSideKey(MergeSide(i)))
		}
		info := AccessInfo{
			Role:   RoleTextInput,
			Name:   Trf(keys[i], name),
			Value:  strings.Join(s.text.Lines, "\n"),
			Bounds: r,
		}
		if i == int(MergeResult) {
			info.Description = m.result.Hint
		} else {
			info.Description = m.sides[i].Hint
		}
		if s.readOnly {
			info.States = append(info.States, StateReadOnly)
		}
		if m.focused && m.active == MergeSide(i) {
			info.States = append(info.States, StateFocused)
		}
		out = append(out, info)
	}
	return out
}
