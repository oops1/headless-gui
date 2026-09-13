package widget

import (
	"errors"
	"image"
	"math"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/widget/diffview"
)

// diffview.go — контрол сравнения и правки двух текстов.
//
// Две панели с карточками-абзацами, S-коннекторы между отличающимися блоками,
// кнопки-стрелки переноса блока на краях карточек и полоса-обзор изменений.
// Стороны разной длины прокручиваются синхронно: обе отображаются в общую
// виртуальную координату кусочно-линейно, и опорная точка едет от верха окна к
// низу по мере прокрутки — соответствующие блоки стоят рядом.
//
// Правка — полноценная, в обеих панелях: каретка, выделение, слова, буфер
// обмена, отмена и повтор с группировкой набора. Модель (алгоритм различий,
// файлы, подсветка) — в пакете widget/diffview; здесь — вид и ввод.

// DiffChange — участок сравнения: строки [LeftFrom, LeftTo) слева против
// [RightFrom, RightTo) справа, номера от нуля.
type DiffChange = diffview.Change

// DiffChangeKind — вид участка сравнения.
type DiffChangeKind = diffview.ChangeKind

const (
	DiffEqual   = diffview.Equal
	DiffReplace = diffview.Replace
	DiffDelete  = diffview.Delete
	DiffInsert  = diffview.Insert
)

// ErrDiffNoPath — у стороны нет файла: сохранять через SaveAs.
var ErrDiffNoPath = errors.New("diffview: side has no file path")

// Геометрия контрола в логических точках.
const (
	dvLineH     = 22 // высота строки кода
	dvTextDY    = 3  // смещение текста внутри строки
	dvTopPad    = 12 // поле над первой строкой
	dvBottomPad = 48 // поле под последней: последний блок не прилипает к краю
	dvHeaderH   = 50 // шапка с именами файлов
	dvOuterPad  = 14
	dvGutterW   = 72 // промежуток между панелями под коннекторы
	dvRulerW    = 12 // полоса-обзор изменений
	dvCircleR   = 10 // радиус кнопки-стрелки
	dvCardExt   = 6  // выступ карточки за строки
	dvUndoLimit = 500
)

// dvSpan — экранные строки участка на каждой стороне.
type dvSpan struct{ lr0, lr1, rr0, rr1 int }

// dvBP — точка излома отображения: виртуальная координата и координаты сторон.
type dvBP struct{ v, l, r float64 }

type dvGeom struct {
	b                  image.Rectangle
	lx0, lx1, rx0, rx1 int
	cy0, cy1           int
	rulerX             int
}

type dvEditKind int

const (
	dvEditNone dvEditKind = iota
	dvEditType
	dvEditDelete
	dvEditOther
)

type dvHist struct {
	lines  [2][]string
	caret  [2]dvPos
	anchor [2]dvPos
	rev    [2]int
	active DiffSide
}

// dvStamp — состояние до операции: по разнице с состоянием после неё
// рассылаются события.
type dvStamp struct {
	rev     [2]int
	mod     [2]bool
	caret   [2]dvPos
	active  DiffSide
	current int
	changes int
}

// DiffView — контрол сравнения и правки двух текстов.
type DiffView struct {
	Base
	mu sync.Mutex

	docs   [2]*dvDoc
	chunks []DiffChange
	spans  []dvSpan
	bps    []dvBP
	pal    dvPalette

	hide, syntax, ignoreWS bool
	showRO, headers        bool // отметка «только чтение» и шапки сторон (GG-71, GG-72)
	ctxLines               int
	expanded               map[[2]int]bool
	monoFont, boldFont     string
	fontSize               float64

	scroll, hscroll, charW float64

	active                        DiffSide
	current, hoverChunk, hoverBtn int
	rulerDrag, dragSel, focused   bool
	dragSide                      DiffSide
	lastClickAt                   time.Time
	lastClickPt                   image.Point
	clicks                        int
	anim                          *Animation

	undo, redo   []dvHist
	lastEdit     dvEditKind
	lastEditSide DiffSide
	lastEditAt   time.Time
	revSeq       int
	diffEvent    bool

	menu      *PopupMenu
	pending   []func()
	watchStop chan struct{}

	// post — отправка работы в поток движка. Движок раздаёт себя виджетам как
	// CaptureManager (SetCaptureManager), и у него же есть Post: событие
	// наблюдателя за файлами приходит с фоновой горутины, а обработчику нужен
	// поток UI. Приложению заполнять это ничего не нужно.
	post func(func())

	// commands — команды из разметки (SaveCommand="{Binding Save}"), см.
	// SetCommand.
	commands map[string]ICommand

	// События. Зовутся без блокировок контрола, в потоке ввода;
	// OnFileChangedOnDisk — в потоке движка (через его Post).
	OnTextChanged          func(side DiffSide)
	OnModifiedChanged      func(side DiffSide, modified bool)
	OnFileLoaded           func(side DiffSide, path string)
	OnFileSaved            func(side DiffSide, path string)
	OnFileChangedOnDisk    func(side DiffSide, path string, deleted bool)
	OnDiffChanged          func(changes int)
	OnCurrentChangeChanged func(index int)
	OnCaretMoved           func(side DiffSide, line, col int)
	OnActiveSideChanged    func(side DiffSide)
	OnBlockCopied          func(index int, toRight bool)
	OnSaveRequest          func(side DiffSide) // Ctrl+S
	OnError                func(side DiffSide, err error)
}

// NewDiffView создаёт контрол сравнения. monoFont — шрифт кода (пусто —
// встроенный Go Mono), boldFont — шрифт имён файлов в шапке (пусто —
// встроенный жирный).
func NewDiffView(monoFont, boldFont string) *DiffView {
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
	d := &DiffView{
		docs:       [2]*dvDoc{newDvDoc(), newDvDoc()},
		pal:        dvPaletteFrom(t),
		syntax:     true,
		showRO:     true,
		headers:    true,
		ctxLines:   3,
		expanded:   map[[2]int]bool{},
		monoFont:   monoFont,
		boldFont:   boldFont,
		fontSize:   10,
		charW:      8,
		current:    -1,
		hoverChunk: -1,
		hoverBtn:   -1,
		menu:       NewPopupMenu(),
	}
	// Меню — ребёнок контрола: так движок рисует его оверлей и отдаёт ему
	// клики (контракт ContextMenuProvider).
	d.AddChild(d.menu)
	d.rebuildLocked()
	return d
}

// SetCaptureManager — движок раздаёт себя виджетам. Отсюда же берётся его Post
// для событий, приходящих с фоновой горутины.
func (d *DiffView) SetCaptureManager(cm CaptureManager) {
	d.mu.Lock()
	if p, ok := cm.(interface{ Post(func()) }); ok {
		d.post = p.Post
	}
	d.mu.Unlock()
}

// ─── События ────────────────────────────────────────────────────────────────

func (d *DiffView) stampLocked() dvStamp {
	st := dvStamp{active: d.active, current: d.current, changes: d.changeCountLocked()}
	for i, s := range d.docs {
		st.rev[i], st.mod[i], st.caret[i] = s.rev, s.modified(), s.caret
	}
	return st
}

func (d *DiffView) emit(f func()) { d.pending = append(d.pending, f) }

// queueEventsLocked сравнивает состояние до и после операции и ставит в
// очередь соответствующие события — вместе с командами разметки.
func (d *DiffView) queueEventsLocked(a dvStamp) {
	b := d.stampLocked()
	for i := range d.docs {
		side := DiffSide(i)
		if a.rev[i] != b.rev[i] {
			if f := d.OnTextChanged; f != nil {
				d.emit(func() { f(side) })
			}
			d.emitCommandLocked("TextChangedCommand", side)
		}
		if a.mod[i] != b.mod[i] {
			if f, m := d.OnModifiedChanged, b.mod[i]; f != nil {
				d.emit(func() { f(side, m) })
			}
		}
	}
	if a.active != b.active {
		if f, s := d.OnActiveSideChanged, b.active; f != nil {
			d.emit(func() { f(s) })
		}
	}
	if a.active != b.active || a.caret[b.active] != b.caret[b.active] {
		if f, s, c := d.OnCaretMoved, b.active, b.caret[b.active]; f != nil {
			d.emit(func() { f(s, c.line, c.col) })
		}
	}
	if a.current != b.current {
		if f, n := d.OnCurrentChangeChanged, d.ordinalLocked(b.current); f != nil {
			d.emit(func() { f(n) })
		}
	}
	if d.diffEvent || a.rev != b.rev || a.changes != b.changes {
		d.diffEvent = false
		if f, n := d.OnDiffChanged, b.changes; f != nil {
			d.emit(func() { f(n) })
		}
		d.emitCommandLocked("DiffChangedCommand", b.changes)
	}
}

// do выполняет fn под замком и рассылает события уже без него: обработчик
// вправе звать методы контрола, и замок, взятый второй раз, повесил бы его.
func (d *DiffView) do(fn func()) {
	d.mu.Lock()
	st := d.stampLocked()
	fn()
	d.queueEventsLocked(st)
	ev := d.pending
	d.pending = nil
	d.mu.Unlock()
	d.Invalidate()
	for _, f := range ev {
		f()
	}
}

func (d *DiffView) nextRev() int {
	d.revSeq++
	return d.revSeq
}

// ─── Команды разметки ───────────────────────────────────────────────────────

// SetCommand привязывает команду разметки к событию контрола:
//
//	SaveCommand          — Ctrl+S, параметр DiffSide;
//	TextChangedCommand   — текст стороны изменился, параметр DiffSide;
//	DiffChangedCommand   — пересчитаны различия, параметр — их число;
//	FileChangedCommand   — файл изменён на диске, параметр DiffFileChange.
//
// Возвращает false для неизвестного имени. Команда с CanExecute = false не
// выполняется. Обработчики On… и команды независимы — срабатывают оба.
func (d *DiffView) SetCommand(name string, cmd ICommand) bool {
	switch name {
	case "SaveCommand", "TextChangedCommand", "DiffChangedCommand", "FileChangedCommand":
	default:
		return false
	}
	d.mu.Lock()
	if d.commands == nil {
		d.commands = map[string]ICommand{}
	}
	d.commands[name] = cmd
	d.mu.Unlock()
	return true
}

// DiffFileChange — параметр FileChangedCommand.
type DiffFileChange struct {
	Side    DiffSide
	Path    string
	Deleted bool
}

func (d *DiffView) emitCommandLocked(name string, param any) {
	cmd := d.commands[name]
	if cmd == nil {
		return
	}
	d.emit(func() {
		if cmd.CanExecute(param) {
			cmd.Execute(param)
		}
	})
}

// ─── Публичное API: содержимое и файлы ──────────────────────────────────────

// LoadFile загружает файл в сторону. История правок сбрасывается.
func (d *DiffView) LoadFile(side DiffSide, path string) error {
	data, err := readDiffFile(path)
	if err != nil {
		d.fail(side, err)
		return err
	}
	d.do(func() {
		s := newDvDoc()
		s.setBytes(data)
		s.setPath(path)
		s.readOnly = d.docs[side].readOnly
		s.stampDisk()
		s.rev = d.nextRev()
		s.savedRev = s.rev
		d.docs[side] = s
		d.resetHistoryLocked()
		d.diffEvent = true
		d.rebuildLocked()
		if f := d.OnFileLoaded; f != nil {
			d.emit(func() { f(side, path) })
		}
	})
	return nil
}

// SetText задаёт текст стороны без файла; title и note — подписи шапки.
func (d *DiffView) SetText(side DiffSide, title, note, text string) {
	d.do(func() {
		s := newDvDoc()
		s.setBytes([]byte(text))
		s.title, s.note = title, note
		s.readOnly = d.docs[side].readOnly
		s.rev = d.nextRev()
		s.savedRev = s.rev
		d.docs[side] = s
		d.resetHistoryLocked()
		d.diffEvent = true
		d.rebuildLocked()
	})
}

// Reload перечитывает файл стороны с диска. Перечитывание отменяемо: правки,
// сделанные до него, возвращаются Undo.
func (d *DiffView) Reload(side DiffSide) error {
	path := d.FilePath(side)
	if path == "" {
		return ErrDiffNoPath
	}
	data, err := readDiffFile(path)
	if err != nil {
		d.fail(side, err)
		return err
	}
	d.do(func() {
		s := d.docs[side]
		d.pushUndoLocked(dvEditOther, side)
		caret := s.caret
		s.setBytes(data)
		s.caret = s.clamp(caret)
		s.anchor = s.caret
		s.stampDisk()
		s.rev = d.nextRev()
		s.savedRev = s.rev
		d.rebuildLocked()
		if f := d.OnFileLoaded; f != nil {
			d.emit(func() { f(side, path) })
		}
	})
	return nil
}

// Save сохраняет сторону в её файл; без файла — ErrDiffNoPath.
func (d *DiffView) Save(side DiffSide) error {
	path := d.FilePath(side)
	if path == "" {
		return ErrDiffNoPath
	}
	return d.SaveAs(side, path)
}

// SaveAs сохраняет сторону в path — с тем же переводом строки, BOM и переводом
// в конце, что были при чтении.
func (d *DiffView) SaveAs(side DiffSide, path string) error {
	d.mu.Lock()
	data := d.docs[side].bytes()
	d.mu.Unlock()
	if err := diffview.WriteFile(path, data); err != nil {
		d.fail(side, err)
		return err
	}
	d.do(func() {
		s := d.docs[side]
		s.setPath(path)
		s.stampDisk()
		s.savedRev = s.rev
		if f := d.OnFileSaved; f != nil {
			d.emit(func() { f(side, path) })
		}
	})
	return nil
}

func (d *DiffView) fail(side DiffSide, err error) {
	if d.OnError != nil {
		d.OnError(side, err)
	}
}

// Text — содержимое стороны в том виде, в каком оно будет сохранено.
func (d *DiffView) Text(side DiffSide) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return string(d.docs[side].bytes())
}

// Lines — строки стороны.
func (d *DiffView) Lines(side DiffSide) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.docs[side].text.Lines)
}

// FilePath — файл стороны (пусто — текст задан без файла).
func (d *DiffView) FilePath(side DiffSide) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.docs[side].path
}

// SetTitle задаёт подписи шапки стороны.
func (d *DiffView) SetTitle(side DiffSide, title, note string) {
	d.do(func() { d.docs[side].title, d.docs[side].note = title, note })
}

// IsModified сообщает, отличается ли сторона от сохранённой. Считается по
// ревизиям, а не флагом: отмена до сохранённого состояния снимает «изменён».
func (d *DiffView) IsModified(side DiffSide) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.docs[side].modified()
}

func (d *DiffView) SetReadOnly(side DiffSide, v bool) {
	d.do(func() { d.docs[side].readOnly = v })
}

func (d *DiffView) IsReadOnly(side DiffSide) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.docs[side].readOnly
}

// ─── Публичное API: вид ─────────────────────────────────────────────────────

// SetHideUnchanged сворачивает одинаковые строки, оставляя по краям
// SetContextLines строк контекста.
func (d *DiffView) SetHideUnchanged(v bool) {
	d.do(func() {
		d.hide = v
		d.expanded = map[[2]int]bool{}
		d.rebuildLocked()
	})
}

// SetShowHeaders показывает или прячет шапки сторон — карточки с именем файла,
// примечанием и счётчиком. Без них код начинается от верхнего края контрола:
// там, где файл и что с чем сравнивается уже видно рядом (список изменений),
// шапки только отнимают место.
func (d *DiffView) SetShowHeaders(v bool) {
	d.do(func() {
		if d.headers == v {
			return
		}
		d.headers = v
		// Высота области кода изменилась — прокрутка могла выйти за край.
		d.setScrollLocked(d.scroll)
	})
}

// ShowHeaders сообщает, показаны ли шапки сторон.
func (d *DiffView) ShowHeaders() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.headers
}

// SetShowReadOnlyMark показывает или прячет отметку «только чтение» в шапке
// стороны. В окне, где сравнение по смыслу только для просмотра, отметка
// стоит на обеих сторонах, ничего не сообщает и отнимает место у примечания.
// Сама сторона остаётся только для чтения — меняется лишь надпись.
func (d *DiffView) SetShowReadOnlyMark(v bool) {
	d.do(func() { d.showRO = v })
}

// ShowReadOnlyMark сообщает, показывается ли отметка «только чтение».
func (d *DiffView) ShowReadOnlyMark() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.showRO
}

func (d *DiffView) HideUnchanged() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.hide
}

// SetContextLines — сколько одинаковых строк оставлять у свёрток.
func (d *DiffView) SetContextLines(n int) {
	d.do(func() {
		d.ctxLines = max(0, n)
		d.rebuildLocked()
	})
}

func (d *DiffView) SetIgnoreWhitespace(v bool) {
	d.do(func() {
		d.ignoreWS = v
		d.diffEvent = true
		d.rebuildLocked()
	})
}

func (d *DiffView) SetSyntaxHighlight(v bool) {
	d.do(func() {
		d.syntax = v
		d.rebuildLocked()
	})
}

// SetFont задаёт шрифт кода, шрифт шапки и кегль; пустые и нулевые значения
// оставляют прежние.
func (d *DiffView) SetFont(mono, bold string, sizePt float64) {
	d.do(func() {
		if mono != "" {
			d.monoFont = mono
		}
		if bold != "" {
			d.boldFont = bold
		}
		if sizePt > 0 {
			d.fontSize = sizePt
		}
	})
}

// ─── Публичное API: изменения и перенос ─────────────────────────────────────

func (d *DiffView) changeCountLocked() int {
	n := 0
	for _, c := range d.chunks {
		if c.Kind != DiffEqual {
			n++
		}
	}
	return n
}

func (d *DiffView) ordinalLocked(ci int) int {
	if ci < 0 {
		return -1
	}
	n := 0
	for i := 0; i < ci; i++ {
		if d.chunks[i].Kind != DiffEqual {
			n++
		}
	}
	return n
}

func (d *DiffView) chunkOfOrdinalLocked(ord int) int {
	for ci, c := range d.chunks {
		if c.Kind != DiffEqual {
			if ord == 0 {
				return ci
			}
			ord--
		}
	}
	return -1
}

// ChangeCount — число отличий.
func (d *DiffView) ChangeCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.changeCountLocked()
}

// Changes — отличия (совпадения в список не входят).
func (d *DiffView) Changes() []DiffChange {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []DiffChange
	for _, c := range d.chunks {
		if c.Kind != DiffEqual {
			out = append(out, c)
		}
	}
	return out
}

// CurrentChange — номер текущего отличия (-1 — нет).
func (d *DiffView) CurrentChange() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ordinalLocked(d.current)
}

func (d *DiffView) GoToChange(index int) {
	d.do(func() {
		if ci := d.chunkOfOrdinalLocked(index); ci >= 0 {
			d.goToChunkLocked(ci)
		}
	})
}

func (d *DiffView) NextChange() { d.do(func() { d.navigateLocked(+1) }) }
func (d *DiffView) PrevChange() { d.do(func() { d.navigateLocked(-1) }) }

// CopyBlock переносит отличие index на другую сторону.
func (d *DiffView) CopyBlock(index int, toRight bool) {
	d.do(func() { d.applyLocked(d.chunkOfOrdinalLocked(index), toRight) })
}

// CopyCurrent переносит текущее отличие.
func (d *DiffView) CopyCurrent(toRight bool) {
	d.do(func() { d.applyLocked(d.current, toRight) })
}

// CopyAll делает одну сторону копией другой.
func (d *DiffView) CopyAll(toRight bool) {
	d.do(func() { d.copyAllLocked(toRight) })
}

// ─── Публичное API: каретка ─────────────────────────────────────────────────

func (d *DiffView) ActiveSide() DiffSide {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.active
}

func (d *DiffView) SetActiveSide(side DiffSide) { d.do(func() { d.active = side }) }

// Caret — позиция каретки стороны (строка и колонка в рунах, от нуля).
func (d *DiffView) Caret(side DiffSide) (line, col int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	c := d.docs[side].caret
	return c.line, c.col
}

func (d *DiffView) SetCaret(side DiffSide, line, col int) {
	d.do(func() {
		s := d.docs[side]
		s.caret = s.clamp(dvPos{line, col})
		s.anchor = s.caret
		d.active = side
		d.caretMovedLocked()
	})
}

func (d *DiffView) GoToLine(side DiffSide, line int) { d.SetCaret(side, line, 0) }

// Selection возвращает строки стороны, задетые выделением: [fromLine, toLine),
// номера от нуля. ok=false — выделения на этой стороне нет.
//
// Текст выделения (SelectedText) не говорит, какие это строки: одинаковые
// строки встречаются в файле много раз, а приложению, которое добавляет в
// индекс выделенные строки, нужны именно номера. Выделение, кончающееся в
// самом начале строки, эту строку не задевает — так считают строки редакторы:
// Shift+↓ от начала строки выделяет одну строку, а не две.
func (d *DiffView) Selection(side DiffSide) (fromLine, toLine int, ok bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if side != DiffLeft && side != DiffRight {
		return 0, 0, false
	}
	s := d.docs[side]
	if !s.hasSel() {
		return 0, 0, false
	}
	a, b := s.sel()
	to := b.line + 1
	if b.col == 0 && b.line > a.line {
		to = b.line
	}
	return a.line, to, true
}

// ─── Модель отображения ─────────────────────────────────────────────────────

func (d *DiffView) prepareSide(s *dvDoc) { dvPrepareDoc(s, d.syntax) }

// dvPrepareDoc пересобирает кэши отображения буфера: раскрытые табуляции,
// подсветку, пустые отметки внутристрочной разницы и карты строк. Свободная
// функция: тем же способом готовит свои буферы контрол слияния.
func dvPrepareDoc(s *dvDoc, syntax bool) {
	L := s.text.Lines
	s.disp = make([][]rune, len(L))
	s.hl = make([][2]int, len(L))
	s.toks = make([][]diffview.Token, len(L))
	s.lineRow = make([]int, len(L))
	for i, l := range L {
		s.disp[i] = diffview.ExpandTabs(l)
		s.hl[i] = [2]int{-1, -1}
		s.lineRow[i] = -1
		if syntax {
			s.toks[i] = diffview.Tokenize(s.disp[i])
		}
	}
	s.rows = s.rows[:0]
	s.cards = s.cards[:0]
}

func dvLineKind(s string) dvRowKind {
	if diffview.IsBlank(s) {
		return dvRowGap
	}
	return dvRowLine
}

// rebuildLocked пересчитывает различия и всё, что от них зависит: экранные
// строки, карточки, точки излома синхронной прокрутки.
func (d *DiffView) rebuildLocked() {
	L, R := d.docs[0], d.docs[1]
	d.chunks = diffview.Lines(L.text.Lines, R.text.Lines, d.ignoreWS)
	d.prepareSide(L)
	d.prepareSide(R)

	for _, c := range d.chunks {
		if c.Kind != DiffReplace {
			continue
		}
		for i := 0; i < min(c.LeftTo-c.LeftFrom, c.RightTo-c.RightFrom); i++ {
			li, ri := c.LeftFrom+i, c.RightFrom+i
			if a0, a1, b0, b1, ok := diffview.InlineRange(L.disp[li], R.disp[ri]); ok {
				L.hl[li] = [2]int{a0, a1}
				R.hl[ri] = [2]int{b0, b1}
			}
		}
	}

	d.spans = d.spans[:0]
	for ci, c := range d.chunks {
		sp := dvSpan{lr0: len(L.rows), rr0: len(R.rows)}
		if c.Kind == DiffEqual {
			n := c.LeftTo - c.LeftFrom
			head, tail := n, 0
			if d.hide && !d.expanded[[2]int{c.LeftFrom, c.RightFrom}] {
				head, tail = d.ctxLines, d.ctxLines
				if ci == 0 {
					head = 0
				}
				if ci == len(d.chunks)-1 {
					tail = 0
				}
				if n-head-tail < 2 {
					head, tail = n, 0
				}
			}
			rows := func(from, to int) {
				for i := from; i < to; i++ {
					k := dvLineKind(L.text.Lines[c.LeftFrom+i])
					L.rows = append(L.rows, dvRow{k, c.LeftFrom + i, ci})
					R.rows = append(R.rows, dvRow{k, c.RightFrom + i, ci})
				}
			}
			rows(0, head)
			if head < n {
				hidden := n - head - tail
				L.rows = append(L.rows, dvRow{dvRowFold, hidden, ci})
				R.rows = append(R.rows, dvRow{dvRowFold, hidden, ci})
				rows(n-tail, n)
			}
		} else {
			for i := c.LeftFrom; i < c.LeftTo; i++ {
				L.rows = append(L.rows, dvRow{dvLineKind(L.text.Lines[i]), i, ci})
			}
			for i := c.RightFrom; i < c.RightTo; i++ {
				R.rows = append(R.rows, dvRow{dvLineKind(R.text.Lines[i]), i, ci})
			}
		}
		sp.lr1, sp.rr1 = len(L.rows), len(R.rows)
		d.spans = append(d.spans, sp)
	}
	for _, s := range d.docs {
		for r, row := range s.rows {
			if row.kind != dvRowFold {
				s.lineRow[row.line] = r
			}
		}
		dvBuildCards(s)
		s.caret, s.anchor = s.clamp(s.caret), s.clamp(s.anchor)
		if s.lineRow[s.caret.line] < 0 {
			s.caret = s.clamp(dvPos{d.visibleLineLocked(s, s.caret.line, +1), s.caret.col})
			s.anchor = s.caret
		}
	}

	d.bps = append(d.bps[:0], dvBP{0, 0, 0}, dvBP{dvTopPad, dvTopPad, dvTopPad})
	v := float64(dvTopPad)
	for _, sp := range d.spans {
		v += float64(max(sp.lr1-sp.lr0, sp.rr1-sp.rr0) * dvLineH)
		d.bps = append(d.bps, dvBP{v, float64(dvTopPad + sp.lr1*dvLineH), float64(dvTopPad + sp.rr1*dvLineH)})
	}
	last := d.bps[len(d.bps)-1]
	d.bps = append(d.bps, dvBP{last.v + dvBottomPad, last.l + dvBottomPad, last.r + dvBottomPad})

	if d.current >= len(d.chunks) || (d.current >= 0 && d.chunks[d.current].Kind == DiffEqual) {
		d.current = -1
	}
	d.hoverChunk, d.hoverBtn = -1, -1
}

// visibleLineLocked — ближайшая показанная строка (сначала в сторону dir).
func (d *DiffView) visibleLineLocked(s *dvDoc, line, dir int) int {
	for _, step := range []int{dir, -dir} {
		for l := line; l >= 0 && l < len(s.text.Lines); l += step {
			if s.lineRow[l] >= 0 {
				return l
			}
		}
	}
	return line
}

// dvBuildCards собирает карточки-абзацы: подряд идущие непустые строки.
func dvBuildCards(s *dvDoc) {
	start := -1
	for i, r := range s.rows {
		if r.kind == dvRowLine {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			s.cards = append(s.cards, [2]int{start, i})
			start = -1
		}
	}
	if start >= 0 {
		s.cards = append(s.cards, [2]int{start, len(s.rows)})
	}
}

func (d *DiffView) chunkAtLocked(side DiffSide, line int) int {
	for ci, c := range d.chunks {
		a, b := c.LeftFrom, c.LeftTo
		if side == DiffRight {
			a, b = c.RightFrom, c.RightTo
		}
		if line >= a && line < b {
			if c.Kind == DiffEqual {
				return -1
			}
			return ci
		}
	}
	return -1
}

// ─── Геометрия и прокрутка ──────────────────────────────────────────────────

func (d *DiffView) geom() dvGeom {
	b := d.Bounds()
	g := dvGeom{b: b}
	g.rulerX = b.Max.X - dvRulerW - 6
	pw := max((g.rulerX-b.Min.X-2*dvOuterPad-dvGutterW)/2, 80)
	g.lx0 = b.Min.X + dvOuterPad
	g.lx1 = g.lx0 + pw
	g.rx0 = g.lx1 + dvGutterW
	g.rx1 = g.rx0 + pw
	g.cy0 = b.Min.Y
	if d.headers {
		g.cy0 += dvHeaderH
	}
	g.cy1 = b.Max.Y
	return g
}

// paneGeom — края панели и области кода стороны.
func (d *DiffView) paneGeom(g dvGeom, side DiffSide) (x0, x1, codeX, codeR int) {
	x0, x1 = g.lx0, g.lx1
	if side == DiffRight {
		x0, x1 = g.rx0, g.rx1
	}
	digits := max(2, len(strconv.Itoa(max(len(d.docs[0].text.Lines), len(d.docs[1].text.Lines)))))
	return x0, x1, x0 + int(float64(digits)*d.charW) + 36, x1 - 8
}

func (d *DiffView) viewH() float64 {
	g := d.geom()
	return float64(max(g.cy1-g.cy0, 1))
}

func (d *DiffView) maxScroll(viewH float64) float64 {
	return math.Max(0, d.bps[len(d.bps)-1].v-viewH)
}

// mapAt — координаты сторон для виртуальной координаты ref.
func (d *DiffView) mapAt(ref float64) (l, r float64) {
	bps := d.bps
	if ref <= 0 {
		return 0, 0
	}
	for i := 1; i < len(bps); i++ {
		a, b := bps[i-1], bps[i]
		if ref <= b.v {
			if b.v == a.v {
				return b.l, b.r
			}
			t := (ref - a.v) / (b.v - a.v)
			return a.l + t*(b.l-a.l), a.r + t*(b.r-a.r)
		}
	}
	last := bps[len(bps)-1]
	return last.l, last.r
}

// vForSide — обратное отображение: виртуальная координата точки стороны.
func (d *DiffView) vForSide(side DiffSide, y float64) float64 {
	for i := 1; i < len(d.bps); i++ {
		a, b := d.bps[i-1], d.bps[i]
		pa, pb := a.l, b.l
		if side == DiffRight {
			pa, pb = a.r, b.r
		}
		if y <= pb && pb > pa {
			return a.v + math.Max(0, y-pa)/(pb-pa)*(b.v-a.v)
		}
	}
	return d.bps[len(d.bps)-1].v
}

// offsets — верх видимой области каждой стороны. Опорная точка едет от верха
// окна к низу по мере прокрутки: соответствующие блоки стоят рядом, и в конце
// прокрутки обе стороны доходят до своего конца, какой бы длины они ни были.
func (d *DiffView) offsets(viewH float64) (lt, rt float64) {
	ms := d.maxScroll(viewH)
	s := math.Max(0, math.Min(d.scroll, ms))
	ref := s
	if ms > 0 {
		ref = s + viewH*s/ms
	}
	l, r := d.mapAt(ref)
	o := ref - s
	last := d.bps[len(d.bps)-1]
	lt = math.Max(0, math.Min(l-o, last.l-viewH))
	rt = math.Max(0, math.Min(r-o, last.r-viewH))
	return
}

func (d *DiffView) sideTop(side DiffSide) float64 {
	lt, rt := d.offsets(d.viewH())
	if side == DiffRight {
		return rt
	}
	return lt
}

func (d *DiffView) setScrollLocked(v float64) bool {
	v = math.Max(0, math.Min(v, d.maxScroll(d.viewH())))
	if v == d.scroll {
		return false
	}
	d.scroll = v
	return true
}

func (d *DiffView) stopAnim() {
	if d.anim != nil {
		d.anim.Stop()
		d.anim = nil
	}
}

func (d *DiffView) scrollBy(dy float64) {
	d.mu.Lock()
	d.stopAnim()
	ch := d.setScrollLocked(d.scroll + dy)
	d.mu.Unlock()
	if ch {
		d.Invalidate()
	}
}

func (d *DiffView) maxHScrollLocked() float64 {
	maxRunes := 0
	for _, s := range d.docs {
		for _, r := range s.disp {
			maxRunes = max(maxRunes, len(r))
		}
	}
	g := d.geom()
	_, _, codeX, codeR := d.paneGeom(g, DiffLeft)
	return math.Max(0, float64(maxRunes+2)*d.charW-float64(codeR-codeX))
}

func (d *DiffView) hscrollBy(dx float64) {
	d.mu.Lock()
	v := math.Max(0, math.Min(d.hscroll+dx, d.maxHScrollLocked()))
	ch := v != d.hscroll
	d.hscroll = v
	d.mu.Unlock()
	if ch {
		d.Invalidate()
	}
}

func (d *DiffView) chunkCenterV(ci int) float64 {
	return (d.bps[ci+1].v + d.bps[ci+2].v) / 2
}

// scrollForV — прокрутка, при которой точка v стоит в опорной точке.
func (d *DiffView) scrollForV(v float64) float64 {
	vh := d.viewH()
	ms := d.maxScroll(vh)
	if ms <= 0 {
		return 0
	}
	return v * ms / (ms + vh)
}

func (d *DiffView) animateToLocked(target float64) {
	d.stopAnim()
	from := d.scroll
	d.anim = AnimateOwned(d, "scroll", 220*time.Millisecond, EaseOutCubic, func(t float64) {
		d.mu.Lock()
		d.setScrollLocked(from + (target-from)*t)
		d.mu.Unlock()
		d.Invalidate()
	})
}

func (d *DiffView) navigateLocked(dir int) {
	n := len(d.chunks)
	i := d.current
	if i < 0 {
		// Без текущего — от каретки активной стороны.
		line := d.docs[d.active].caret.line
		i = -1
		for ci, c := range d.chunks {
			a := c.LeftFrom
			if d.active == DiffRight {
				a = c.RightFrom
			}
			if a <= line {
				i = ci
			}
		}
		if dir < 0 {
			i++
		}
	}
	for {
		i += dir
		if i < 0 || i >= n {
			return
		}
		if d.chunks[i].Kind != DiffEqual {
			break
		}
	}
	d.goToChunkLocked(i)
}

func (d *DiffView) goToChunkLocked(ci int) {
	c := d.chunks[ci]
	d.current = ci
	for i, s := range d.docs {
		line := c.LeftFrom
		if i == 1 {
			line = c.RightFrom
		}
		s.caret = s.clamp(dvPos{line, 0})
		s.anchor = s.caret
	}
	d.lastEdit = dvEditNone
	d.animateToLocked(d.scrollForV(d.chunkCenterV(ci)))
}

// ─── Попадания ──────────────────────────────────────────────────────────────

func (d *DiffView) circleCenters(g dvGeom, ci int, lt, rt float64) (lx, ly, rx, ry float64) {
	sp := d.spans[ci]
	lx, rx = float64(g.lx1+2), float64(g.rx0-2)
	ly = float64(g.cy0+dvTopPad) + float64(sp.lr0+sp.lr1)/2*dvLineH - lt
	ry = float64(g.cy0+dvTopPad) + float64(sp.rr0+sp.rr1)/2*dvLineH - rt
	return
}

// hitButtonLocked — кнопка-стрелка под точкой: 2·участок (левая) или
// 2·участок+1 (правая); -1 — мимо.
func (d *DiffView) hitButtonLocked(x, y int) int {
	g := d.geom()
	if y < g.cy0 || y >= g.cy1 {
		return -1
	}
	lt, rt := d.offsets(float64(g.cy1 - g.cy0))
	fx, fy := float64(x), float64(y)
	rr := float64((dvCircleR + 3) * (dvCircleR + 3))
	for ci, c := range d.chunks {
		if c.Kind == DiffEqual {
			continue
		}
		lx, ly, rx, ry := d.circleCenters(g, ci, lt, rt)
		if (fx-lx)*(fx-lx)+(fy-ly)*(fy-ly) <= rr {
			return ci * 2
		}
		if (fx-rx)*(fx-rx)+(fy-ry)*(fy-ry) <= rr {
			return ci*2 + 1
		}
	}
	return -1
}

// sideAt — сторона под точкой (ok=false — не над панелью).
func (d *DiffView) sideAt(x, y int) (DiffSide, bool) {
	g := d.geom()
	if y < g.cy0 || y >= g.cy1 {
		return 0, false
	}
	switch {
	case x >= g.lx0 && x < g.lx1:
		return DiffLeft, true
	case x >= g.rx0 && x < g.rx1:
		return DiffRight, true
	}
	return 0, false
}

func (d *DiffView) rowAtLocked(side DiffSide, y int) int {
	g := d.geom()
	return int(math.Floor((float64(y-g.cy0-dvTopPad) + d.sideTop(side)) / dvLineH))
}

func (d *DiffView) hitChunkLocked(x, y int) int {
	side, ok := d.sideAt(x, y)
	if !ok {
		return -1
	}
	s := d.docs[side]
	r := d.rowAtLocked(side, y)
	if r < 0 || r >= len(s.rows) {
		return -1
	}
	if ci := s.rows[r].chunk; d.chunks[ci].Kind != DiffEqual {
		return ci
	}
	return -1
}

// hitPosLocked — позиция в буфере под точкой (с зажимом к краям).
func (d *DiffView) hitPosLocked(side DiffSide, x, y int) dvPos {
	s := d.docs[side]
	if len(s.rows) == 0 {
		return dvPos{}
	}
	r := max(0, min(d.rowAtLocked(side, y), len(s.rows)-1))
	if s.rows[r].kind == dvRowFold {
		switch {
		case r+1 < len(s.rows):
			r++
		case r > 0:
			r--
		default:
			return s.clamp(dvPos{})
		}
	}
	line := s.rows[r].line
	_, _, codeX, _ := d.paneGeom(d.geom(), side)
	dc := int(math.Round((float64(x-codeX) + d.hscroll) / d.charW))
	return s.clamp(dvPos{line, diffview.RawCol([]rune(s.text.Lines[line]), max(0, dc))})
}

func (d *DiffView) rulerTrack(g dvGeom) image.Rectangle {
	return image.Rect(g.rulerX, g.cy0+4, g.rulerX+dvRulerW, g.cy1-8)
}

func (d *DiffView) scrollToRulerLocked(y int) {
	g := d.geom()
	tr := d.rulerTrack(g)
	if tr.Dy() <= 0 {
		return
	}
	V := d.bps[len(d.bps)-1].v
	d.stopAnim()
	d.setScrollLocked(float64(y-tr.Min.Y)/float64(tr.Dy())*V - d.viewH()/2)
}
