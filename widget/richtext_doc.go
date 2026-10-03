package widget

// richtext_doc.go — редактируемая модель форматированного текста: документ из
// абзацев, правки по смещениям в рунах и история отмены.
//
// Модель чистая: ни виджета, ни замков, ни отрисовки. Это сделано намеренно —
// правки текста с форматированием (вставка в середину рана, слияние абзацев,
// смена жирности на куске слова) — самая ошибкоопасная часть редактора, и
// проверять её надо без окна, на случайных последовательностях правок. Виджет
// держит RichDocument под своим замком и только вызывает методы.
//
// Адресация. Позиция — индекс руны в «документе как строке», где абзацы
// разделены одним '\n'. Так считают раскладка и выделение (richBuildDoc), и
// ровно так же здесь: ран с '\n' внутри (мягкий перевод строки) занимает в
// позиции столько же, сколько любая руна. Граница абзаца определяется не
// символом '\n' в тексте, а самой структурой документа: иначе мягкий перевод
// строки нельзя было бы отличить от границы абзаца.
//
// Инварианты после любой правки:
//   - в документе хотя бы один абзац;
//   - в абзаце нет пустых ранов, а соседние раны имеют разное оформление —
//     иначе «жирный» кусок, набранный по буквам, превратился бы в сотню ранов
//     по одной букве, а сравнение и выгрузка в HTML — в кашу из <span>;
//   - единственное исключение — абзац без текста: он хранит один пустой ран,
//     чтобы набор в нём начинался с запомненного оформления, а не «по умолчанию»
//     (так ведут себя Word и браузерные редакторы).

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// richDocUndoDepth — сколько действий помнит документ по умолчанию.
const richDocUndoDepth = 200

// RichDocSel — выделение, которое надо выставить после отмены или возврата.
// Anchor == Caret — просто каретка. Позиции — в рунах документа.
type RichDocSel struct {
	Anchor, Caret int
}

// richDocEntry — одна запись истории (одно действие пользователя).
//
// Запись хранит не весь документ, а только абзацы, которых коснулась правка:
// [first, first+count) — область документа в ТЕКУЩЕМ состоянии, saved — то, что
// стояло на её месте в противоположном. Так отмена и возврат — одна и та же
// операция «обменять область с saved», а цена записи зависит от правки, а не
// от размера документа. Дешёвой запись делает то, что строки текста не
// копируются (строки в Go неизменяемы и разделяются): копируются только
// заголовки ранов затронутых абзацев.
//
// Почему не «что вставили/что удалили» в рунах: обратная операция к удалению
// должна вернуть не только текст, но и оформление абзацев, слитых при удалении
// '\n', и «запомненный» стиль пустого абзаца. Точное обращение таких правок
// по описанию тянет за собой цепочку особых случаев, а обмен области
// восстанавливает всё побитно — а это то, что проверяет случайный тест.
type richDocEntry struct {
	first, count int
	saved        []RichParagraph
	selUndo      RichDocSel // куда поставить каретку при отмене
	selRedo      RichDocSel // и при возврате
}

// RichDocument — документ форматированного текста с историей правок.
//
// Не потокобезопасен: синхронизацию обеспечивает владелец.
type RichDocument struct {
	paras []RichParagraph

	undo, redo []richDocEntry
	depth      int

	// groupDepth > 0 — между BeginGroup и EndGroup: все правки идут в одну
	// запись. open — верхнюю запись истории можно дополнять (набор или
	// группа). typeOpen/typeEnd/typeStyle — набор, который продолжается, если
	// следующий символ набран ровно там, где закончился предыдущий, тем же
	// оформлением.
	groupDepth int
	open       bool
	typeOpen   bool
	typeEnd    int
	typeStyle  RichRun

	rev uint64
}

// NewRichDocument строит документ из абзацев. Абзацы копируются (владелец
// исходного среза может продолжать им пользоваться), текст нормализуется, раны
// приводятся к инвариантам. Пустой список — один пустой абзац.
func NewRichDocument(paras []RichParagraph) *RichDocument {
	d := &RichDocument{depth: richDocUndoDepth}
	d.setParas(paras)
	return d
}

// NewRichDocumentFromText строит документ из простого текста: '\n' и "\r\n"
// разбивают абзацы.
func NewRichDocumentFromText(text string) *RichDocument {
	return NewRichDocument(RichParagraphsFromText(text, RichRun{}))
}

// RichParagraphsFromText раскладывает простой текст на абзацы: '\n', "\r\n" и
// одиночный '\r' (старые Mac и часть буферов обмена) — границы абзацев. Каждый
// абзац получает style (его Text игнорируется). Пустая строка даёт абзац с
// одним пустым раном — запомненным оформлением. Пустой текст — один пустой
// абзац, а не ноль абзацев: документу без абзацев некуда ставить каретку.
func RichParagraphsFromText(text string, style RichRun) []RichParagraph {
	text = richNormalizeText(text)
	lines := strings.Split(text, "\n")
	out := make([]RichParagraph, len(lines))
	for i, ln := range lines {
		r := style
		r.Text = ln
		out[i] = RichParagraph{Runs: []RichRun{r}}
	}
	return out
}

func (d *RichDocument) setParas(paras []RichParagraph) {
	d.paras = richCopyParagraphs(paras)
	if len(d.paras) == 0 {
		d.paras = []RichParagraph{{}}
	}
	for i := range d.paras {
		d.paras[i].Runs = richTidyRuns(d.paras[i].Runs)
	}
}

// SetParagraphs заменяет весь документ и забывает историю: записанные области
// указывали бы в чужой документ, и отмена испортила бы текст.
func (d *RichDocument) SetParagraphs(paras []RichParagraph) {
	d.setParas(paras)
	d.ClearHistory()
	d.rev++
}

// Paragraphs возвращает глубокую копию абзацев: изменения копии документ не
// затронут, а правки документа не изменят копию.
func (d *RichDocument) Paragraphs() []RichParagraph { return richCloneParas(d.paras) }

// ParagraphCount — число абзацев (не меньше одного).
func (d *RichDocument) ParagraphCount() int { return len(d.paras) }

// Revision растёт при каждом изменении документа (правка, отмена, возврат,
// замена целиком); по нему виджет понимает, что раскладку пора строить заново.
func (d *RichDocument) Revision() uint64 { return d.rev }

// Text — документ как строка: абзацы через '\n'.
func (d *RichDocument) Text() string {
	var sb strings.Builder
	for i, p := range d.paras {
		if i > 0 {
			sb.WriteByte('\n')
		}
		for _, r := range p.Runs {
			sb.WriteString(r.Text)
		}
	}
	return sb.String()
}

// Len — длина документа в рунах, с разделителями абзацев.
func (d *RichDocument) Len() int {
	n := len(d.paras) - 1
	for _, p := range d.paras {
		n += richParaLen(p)
	}
	return n
}

// ---------------------------------------------------------------------------
// Вспомогательные функции

// richParaLen — длина абзаца в рунах.
func richParaLen(p RichParagraph) int {
	n := 0
	for _, r := range p.Runs {
		n += utf8.RuneCountInString(r.Text)
	}
	return n
}

// richSameStyle — оформление ранов одинаково (текст не сравнивается). RichRun
// состоит из сравнимых полей, поэтому достаточно ==: список полей не надо
// дублировать, и новое поле оформления автоматически учтётся при слиянии.
func richSameStyle(a, b RichRun) bool {
	a.Text, b.Text = "", ""
	return a == b
}

// richTidyRuns восстанавливает инварианты ранов абзаца: выбрасывает пустые,
// сливает соседние с одинаковым оформлением. Если текста нет совсем, остаётся
// первый из пустых ранов: он помнит оформление абзаца. Исходный срез не
// меняется.
func richTidyRuns(runs []RichRun) []RichRun {
	var (
		out         []RichRun
		parts       []string // куски текста текущего рана: склейка один раз
		placeholder RichRun
		havePlace   bool
	)
	flush := func() {
		if len(out) > 0 {
			out[len(out)-1].Text = strings.Join(parts, "")
		}
		parts = parts[:0]
	}
	for _, r := range runs {
		if r.Text == "" {
			if !havePlace {
				placeholder, havePlace = r, true
			}
			continue
		}
		if len(out) > 0 && richSameStyle(out[len(out)-1], r) {
			parts = append(parts, r.Text)
			continue
		}
		flush()
		out = append(out, r)
		parts = append(parts[:0], r.Text)
	}
	flush()
	if len(out) == 0 && havePlace && placeholder != (RichRun{}) {
		// Пустой ран без оформления ничего не помнит: хранить его незачем, и
		// «абзац без ранов» и «абзац с пустым безликим раном» — одно и то
		// же. Каноническая форма — без ранов, иначе сравнения (отмена,
		// разбор HTML) отличали бы одинаковые абзацы.
		return []RichRun{placeholder}
	}
	return out
}

// richCloneParas — глубокая копия абзацев (раны копируются, строки делятся).
func richCloneParas(src []RichParagraph) []RichParagraph {
	out := make([]RichParagraph, len(src))
	for i, p := range src {
		p.Runs = slices.Clone(p.Runs)
		out[i] = p
	}
	return out
}

// richParasEqual — абзацы равны по оформлению и содержимому.
func richParasEqual(a, b []RichParagraph) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.Align != y.Align || x.Indent != y.Indent ||
			x.SpaceBefore != y.SpaceBefore || x.SpaceAfter != y.SpaceAfter ||
			!slices.Equal(x.Runs, y.Runs) {
			return false
		}
	}
	return true
}

// richSplitAtRune делит строку на две по индексу руны n. Некорректные байты
// UTF-8 считаются по одному на руну — как при преобразовании в []rune и как
// считает richBuildDoc, иначе смещения «поплыли» бы.
func richSplitAtRune(s string, n int) (string, string) {
	if n <= 0 {
		return "", s
	}
	i := 0
	for ; n > 0 && i < len(s); n-- {
		_, w := utf8.DecodeRuneInString(s[i:])
		i += w
	}
	return s[:i], s[i:]
}

// richSplitRuns делит раны по смещению off (в рунах от начала абзаца). Ран на
// границе разрезается на два с тем же оформлением. Возвращаемые срезы новые.
func richSplitRuns(runs []RichRun, off int) (left, right []RichRun) {
	for i, r := range runs {
		if off <= 0 {
			right = append(right, runs[i:]...)
			return
		}
		n := utf8.RuneCountInString(r.Text)
		if n <= off {
			left = append(left, r)
			off -= n
			continue
		}
		h, t := richSplitAtRune(r.Text, off)
		hr, tr := r, r
		hr.Text, tr.Text = h, t
		left = append(left, hr)
		right = append(right, tr)
		right = append(right, runs[i+1:]...)
		return
	}
	return
}

// richStyleAtOffset — оформление, которое получит текст, набранный на
// смещении off абзаца: символа слева, а в начале абзаца — его первого символа
// (так поступают текстовые редакторы: набор в начале слова не меняет вид).
// У абзаца без текста — запомненный пустой ран.
func richStyleAtOffset(runs []RichRun, off int) RichRun {
	if len(runs) == 0 {
		return RichRun{}
	}
	if off <= 0 {
		r := runs[0]
		r.Text = ""
		return r
	}
	acc := 0
	for _, r := range runs {
		acc += utf8.RuneCountInString(r.Text)
		if off <= acc {
			r.Text = ""
			return r
		}
	}
	r := runs[len(runs)-1]
	r.Text = ""
	return r
}

// locate переводит позицию в (абзац, смещение в абзаце) и возвращает
// позицию, приведённую к границам документа. Позиция конца абзаца
// принадлежит этому абзацу, а не следующему (как paraOf у richDoc). Это
// линейный проход по абзацам: на правку это микросекунды даже для документа
// в десятки тысяч абзацев, а кэш длин потребовал бы согласованности с каждой
// правкой и отменой.
func (d *RichDocument) locate(pos int) (pi, off, abs int) {
	if pos < 0 {
		pos = 0
	}
	abs = pos
	last := len(d.paras) - 1
	for i := range d.paras {
		n := richParaLen(d.paras[i])
		if pos <= n || i == last {
			if pos > n {
				abs -= pos - n
				pos = n
			}
			return i, pos, abs
		}
		pos -= n + 1
	}
	return 0, 0, 0
}

// richOrdered упорядочивает границы диапазона.
func richOrdered(a, b int) (int, int) {
	if a > b {
		return b, a
	}
	return a, b
}

// ---------------------------------------------------------------------------
// Запросы

// StyleAt — оформление (поле Text пусто), которое получит текст, набранный в
// позиции pos: стиль символа слева, в начале абзаца — первого символа абзаца,
// в пустом абзаце — запомненное оформление.
func (d *RichDocument) StyleAt(pos int) RichRun {
	pi, off, _ := d.locate(pos)
	return richStyleAtOffset(d.paras[pi].Runs, off)
}

// ParagraphFormatAt — выравнивание и отступы абзаца, в котором стоит pos (Runs
// пусты). Для панели инструментов: показать, какое выравнивание у абзаца под
// кареткой.
func (d *RichDocument) ParagraphFormatAt(pos int) RichParagraph {
	pi, _, _ := d.locate(pos)
	p := d.paras[pi]
	p.Runs = nil
	return p
}

// Fragment — копия диапазона [from, to) как абзацев со всем оформлением. Для
// копирования внутри редактора без потери оформления и для InsertParagraphs.
// Формат первого абзаца — формат абзаца, где начинается диапазон.
func (d *RichDocument) Fragment(from, to int) []RichParagraph {
	from, to = richOrdered(from, to)
	pa, offa, _ := d.locate(from)
	pb, offb, _ := d.locate(to)
	out := make([]RichParagraph, 0, pb-pa+1)
	for i := pa; i <= pb; i++ {
		p := d.paras[i]
		runs := p.Runs
		// Сначала отрезается хвост, потом голова: смещение начала считается
		// от начала абзаца, а отрезание хвоста его не сдвигает — поэтому
		// порядок безопасен и для диапазона внутри одного абзаца.
		if i == pb {
			runs, _ = richSplitRuns(runs, offb)
		}
		if i == pa {
			_, runs = richSplitRuns(runs, offa)
		}
		p.Runs = richTidyRuns(runs)
		out = append(out, p)
	}
	return out
}

// ---------------------------------------------------------------------------
// История

// commit применяет замену области [first, first+n) на repl и записывает её в
// историю. merge — дополнить верхнюю запись вместо новой.
//
// Дополнение запись расширяет до объединения областей: абзацы вне прежней
// области правка не трогала, поэтому их состояние «до» совпадает с текущим и
// берётся прямо из документа.
func (d *RichDocument) commit(first, n int, repl []RichParagraph, selUndo, selRedo RichDocSel, typing bool, typeEnd int, typeStyle RichRun, merge bool) {
	d.rev++
	if d.depth <= 0 {
		d.undo, d.redo = nil, nil
		d.paras = slices.Replace(d.paras, first, first+n, repl...)
		d.open, d.typeOpen = false, false
		return
	}
	var e *richDocEntry
	if merge && len(d.undo) > 0 {
		e = &d.undo[len(d.undo)-1]
		u0 := min(e.first, first)
		u1 := max(e.first+e.count, first+n)
		if u0 < e.first {
			head := richCloneParas(d.paras[u0:e.first])
			e.saved = append(head, e.saved...)
		}
		if end := e.first + e.count; u1 > end {
			e.saved = append(e.saved, richCloneParas(d.paras[end:u1])...)
		}
		e.first, e.count = u0, u1-u0
	} else {
		d.undo = append(d.undo, richDocEntry{
			first:   first,
			count:   n,
			saved:   richCloneParas(d.paras[first : first+n]),
			selUndo: selUndo,
		})
		if over := len(d.undo) - d.depth; over > 0 {
			copy(d.undo, d.undo[over:])
			clear(d.undo[d.depth:])
			d.undo = d.undo[:d.depth]
		}
		e = &d.undo[len(d.undo)-1]
		d.redo = nil
	}
	d.paras = slices.Replace(d.paras, first, first+n, repl...)
	e.count += len(repl) - n
	e.selRedo = selRedo

	d.open = d.groupDepth > 0 || typing
	d.typeOpen = typing
	if typing {
		d.typeEnd, d.typeStyle = typeEnd, typeStyle
	}
}

// swap меняет область записи с сохранённой: отмена и возврат — одно действие.
// Вытесненные абзацы больше нигде не используются, поэтому их можно забрать
// без глубокого копирования.
func (d *RichDocument) swap(e *richDocEntry) bool {
	end := e.first + e.count
	if e.first < 0 || end > len(d.paras) || len(e.saved) == 0 {
		return false
	}
	cur := slices.Clone(d.paras[e.first:end])
	d.paras = slices.Replace(d.paras, e.first, end, e.saved...)
	e.count, e.saved = len(e.saved), cur
	d.rev++
	return true
}

// Undo отменяет последнее действие. ok == false, если отменять нечего. sel —
// куда поставить каретку и выделение: после отмены вставки — на место вставки,
// после отмены удаления — выделенным возвращённый текст.
func (d *RichDocument) Undo() (sel RichDocSel, ok bool) {
	if len(d.undo) == 0 {
		return RichDocSel{}, false
	}
	d.open, d.typeOpen = false, false
	e := d.undo[len(d.undo)-1]
	d.undo = d.undo[:len(d.undo)-1]
	if !d.swap(&e) {
		// История разошлась с документом — лучше потерять её, чем текст.
		d.ClearHistory()
		return RichDocSel{}, false
	}
	d.redo = append(d.redo, e)
	return e.selUndo, true
}

// Redo возвращает последнее отменённое действие.
func (d *RichDocument) Redo() (sel RichDocSel, ok bool) {
	if len(d.redo) == 0 {
		return RichDocSel{}, false
	}
	d.open, d.typeOpen = false, false
	e := d.redo[len(d.redo)-1]
	d.redo = d.redo[:len(d.redo)-1]
	if !d.swap(&e) {
		d.ClearHistory()
		return RichDocSel{}, false
	}
	d.undo = append(d.undo, e)
	return e.selRedo, true
}

// CanUndo — есть ли что отменять.
func (d *RichDocument) CanUndo() bool { return len(d.undo) > 0 }

// CanRedo — есть ли что возвращать.
func (d *RichDocument) CanRedo() bool { return len(d.redo) > 0 }

// ClearHistory забывает историю.
func (d *RichDocument) ClearHistory() {
	d.undo, d.redo = nil, nil
	d.open, d.typeOpen = false, false
}

// SetUndoDepth задаёт, сколько действий помнит документ; 0 и меньше —
// история выключена. Лишние старые записи сразу отбрасываются.
func (d *RichDocument) SetUndoDepth(n int) {
	if n < 0 {
		n = 0
	}
	d.depth = n
	if over := len(d.undo) - n; over > 0 {
		copy(d.undo, d.undo[over:])
		clear(d.undo[n:])
		d.undo = d.undo[:n]
	}
	if n == 0 {
		d.undo, d.redo = nil, nil
	}
}

// BreakUndoGroup закрывает набор: следующий символ начнёт новую запись. Виджет
// вызывает при движении каретки, щелчке, смене фокуса — набор после них
// отменяется отдельным шагом (как в Word и в браузере).
func (d *RichDocument) BreakUndoGroup() {
	d.typeOpen = false
	if d.groupDepth == 0 {
		d.open = false
	}
}

// BeginGroup и EndGroup объединяют правки между ними в одну запись истории
// (вставка поверх выделения: удаление плюс вставка — одна отмена). Вызовы
// вкладываются.
func (d *RichDocument) BeginGroup() {
	if d.groupDepth == 0 {
		d.open, d.typeOpen = false, false
	}
	d.groupDepth++
}

// EndGroup закрывает группу, открытую BeginGroup.
func (d *RichDocument) EndGroup() {
	if d.groupDepth == 0 {
		return
	}
	d.groupDepth--
	if d.groupDepth == 0 {
		d.open, d.typeOpen = false, false
	}
}

// ---------------------------------------------------------------------------
// Правки

// Insert вставляет text в позицию at, оформляя его style (поле style.Text
// игнорируется). Каждый '\n' в тексте разбивает абзац; новые абзацы наследуют
// выравнивание и отступы текущего. Возвращает позицию сразу за вставленным.
// at за границами документа приводится к его границам.
func (d *RichDocument) Insert(at int, text string, style RichRun) int {
	return d.insertText(at, text, style, false)
}

// Type — Insert для набора с клавиатуры: подряд набранные символы (каждый
// следующий ровно там, где закончился предыдущий, с тем же оформлением)
// попадают в одну запись истории — один Undo убирает всё набранное слово или
// фразу, а не по букве. Текст с '\n' (Enter) — отдельный шаг и набор
// прерывает. Это отдельный метод, а не эвристика в Insert, потому что вставка
// из буфера и набор буквы неотличимы по аргументам, а склеивать вставки с
// набором нельзя.
func (d *RichDocument) Type(at int, text string, style RichRun) int {
	return d.insertText(at, text, style, true)
}

func (d *RichDocument) insertText(at int, text string, style RichRun, typing bool) int {
	text = richNormalizeText(text)
	if text == "" {
		_, _, abs := d.locate(at)
		return abs
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 1 {
		typing = false
	}
	frag := make([]RichParagraph, len(lines))
	for i, ln := range lines {
		r := style
		r.Text = ln
		frag[i] = RichParagraph{Runs: []RichRun{r}}
	}
	return d.insert(at, frag, true, typing, style)
}

// InsertInline вставляет text в один абзац: '\n' остаётся мягким переводом
// строки внутри рана (Shift+Enter), абзац не разбивается.
func (d *RichDocument) InsertInline(at int, text string, style RichRun) int {
	text = richNormalizeText(text)
	if text == "" {
		_, _, abs := d.locate(at)
		return abs
	}
	r := style
	r.Text = text
	return d.insert(at, []RichParagraph{{Runs: []RichRun{r}}}, true, false, style)
}

// InsertParagraphs вставляет в позицию at готовые абзацы со своим
// оформлением (разобранный HTML, Fragment). Первый абзац вливается в текущий и
// его выравнивание не меняет, следом идут абзацы со своим выравниванием, а
// хвост текущего абзаца присоединяется к последнему вставленному. Исключение:
// если текущий абзац пуст, а вставляется несколько, выравнивание берётся от
// первого вставляемого — иначе вставка маркированного списка в пустую строку
// теряла бы отступ первого пункта. Возвращает позицию за вставленным.
func (d *RichDocument) InsertParagraphs(at int, paras []RichParagraph) int {
	if len(paras) == 0 {
		_, _, abs := d.locate(at)
		return abs
	}
	return d.insert(at, richCopyParagraphs(paras), false, false, RichRun{})
}

// insert — общая вставка. inherit: оформление всех абзацев после первого
// берётся у текущего абзаца (обычный ввод), иначе — из frag.
func (d *RichDocument) insert(at int, frag []RichParagraph, inherit, typing bool, style RichRun) int {
	pi, off, abs := d.locate(at)
	P := d.paras[pi]
	left, right := richSplitRuns(P.Runs, off)

	pf := P
	pf.Runs = nil
	first := pf
	if !inherit && len(frag) > 1 && richParaLen(P) == 0 {
		first = frag[0]
		first.Runs = nil
	}

	total := len(frag) - 1
	repl := make([]RichParagraph, len(frag))
	for i := range frag {
		var runs []RichRun
		if i == 0 {
			runs = append(runs, left...)
		}
		runs = append(runs, frag[i].Runs...)
		if i == len(frag)-1 {
			runs = append(runs, right...)
		}
		f := first
		if i > 0 {
			if inherit {
				f = pf
			} else {
				f = frag[i]
			}
		}
		f.Runs = richTidyRuns(runs)
		repl[i] = f
		total += richParaLen(frag[i])
	}
	end := abs + total

	merge := d.open && (d.groupDepth > 0 ||
		(typing && d.typeOpen && abs == d.typeEnd && richSameStyle(d.typeStyle, style)))
	d.commit(pi, 1, repl,
		RichDocSel{abs, abs}, RichDocSel{end, end},
		typing, end, style, merge)
	return end
}

// Delete удаляет диапазон [from, to) (границы в любом порядке). Удаление '\n'
// сливает абзацы; оформление абзаца берётся у первого. Если удалён весь текст
// абзаца, он помнит оформление удалённого начала — набор в нём продолжится
// тем же шрифтом, а не «по умолчанию».
func (d *RichDocument) Delete(from, to int) {
	from, to = richOrdered(from, to)
	pa, offa, a := d.locate(from)
	pb, offb, b := d.locate(to)
	if a == b {
		return
	}
	Pa, Pb := d.paras[pa], d.paras[pb]
	hold := richStyleAtOffset(Pa.Runs, offa)
	left, _ := richSplitRuns(Pa.Runs, offa)
	_, right := richSplitRuns(Pb.Runs, offb)

	merged := Pa
	merged.Runs = richTidyRuns(append(left, right...))
	if len(merged.Runs) == 0 {
		hold.Text = ""
		merged.Runs = richTidyRuns([]RichRun{hold})
	}
	d.commit(pa, pb-pa+1, []RichParagraph{merged},
		RichDocSel{a, b}, RichDocSel{a, a},
		false, 0, RichRun{}, d.open && d.groupDepth > 0)
}

// Replace заменяет диапазон [from, to) текстом text одним действием истории:
// набор поверх выделения и вставка из буфера отменяются одним Undo. Возвращает
// позицию за вставленным.
func (d *RichDocument) Replace(from, to int, text string, style RichRun) int {
	from, to = richOrdered(from, to)
	d.BeginGroup()
	defer d.EndGroup()
	d.Delete(from, to)
	return d.Insert(from, text, style)
}

// TypeReplace — Type поверх выделения: первый набранный символ замещает
// [from, to) одним действием (удаление и вставка — один Undo), а дальнейший
// набор подряд дописывается в ту же запись, как после обычного Type. Так в
// Word: выделили слово, напечатали другое — один Ctrl+Z возвращает исходное.
//
// Отдельно от Replace, потому что Replace после себя набор не продолжает:
// EndGroup закрывает запись, и вторая буква нового слова начала бы свою, а
// отмена вернула бы «слово без первой буквы». Здесь признак набора после
// закрытия группы возвращается, если набор не был прерван переводом абзаца.
func (d *RichDocument) TypeReplace(from, to int, text string, style RichRun) int {
	from, to = richOrdered(from, to)
	_, _, a := d.locate(from)
	_, _, b := d.locate(to)
	if a == b {
		return d.Type(from, text, style)
	}
	d.BeginGroup()
	d.Delete(from, to)
	end := d.Type(from, text, style)
	typing := d.typeOpen
	d.EndGroup()
	if typing && d.groupDepth == 0 && len(d.undo) > 0 {
		d.open, d.typeOpen = true, true
	}
	return end
}

// ApplyStyle применяет fn к оформлению каждого рана диапазона [from, to):
// раны на границах разрезаются, к серединам применяется fn. Так делаются
// жирный, курсив, цвет, кегль, шрифт, подчёркивание и ссылка. fn не должна
// менять Text: если изменит, текст восстанавливается — правка оформления не
// имеет права сдвигать смещения.
//
// Пустой диапазон ничего не меняет, кроме одного случая: каретка в пустом
// абзаце — тогда fn применяется к запомненному пустому рану абзаца, то есть
// задаёт, как будет оформлен набранный в нём текст.
func (d *RichDocument) ApplyStyle(from, to int, fn func(*RichRun)) {
	if fn == nil {
		return
	}
	from, to = richOrdered(from, to)
	pa, offa, a := d.locate(from)
	pb, _, b := d.locate(to)

	region := richCloneParas(d.paras[pa : pb+1])
	base := a - offa // абсолютное начало абзаца pa
	for i := pa; i <= pb; i++ {
		p := &region[i-pa]
		n := richParaLen(*p)
		s := max(a-base, 0)
		e := min(b-base, n)
		switch {
		case n == 0:
			// Пустой абзац стилизуется, если выделение захватывает его
			// конец (разделитель после него), либо это пустой диапазон,
			// либо абзац последний и выделение доходит до конца документа.
			touched := a <= base && (b > base || (b == base && (a == b || i == len(d.paras)-1)))
			if touched {
				if len(p.Runs) == 0 {
					p.Runs = []RichRun{{}}
				}
				fn(&p.Runs[0])
				p.Runs[0].Text = ""
				p.Runs = richTidyRuns(p.Runs)
			}
		case s < e:
			l, rest := richSplitRuns(p.Runs, s)
			m, r := richSplitRuns(rest, e-s)
			for k := range m {
				t := m[k].Text
				fn(&m[k])
				m[k].Text = t
			}
			p.Runs = richTidyRuns(append(append(l, m...), r...))
		}
		base += n + 1
	}
	if richParasEqual(region, d.paras[pa:pb+1]) {
		return
	}
	d.commit(pa, pb-pa+1, region, RichDocSel{a, b}, RichDocSel{a, b},
		false, 0, RichRun{}, d.open && d.groupDepth > 0)
}

// SetParagraphFormat применяет fn к абзацам, задетым диапазоном: выравнивание
// и отступы. Абзац, до начала которого диапазон лишь доходит (выделили строку
// вместе с её '\n'), не считается задетым. Пустой диапазон — абзац каретки.
// fn не должна менять Runs: если изменит, раны восстанавливаются.
func (d *RichDocument) SetParagraphFormat(from, to int, fn func(*RichParagraph)) {
	if fn == nil {
		return
	}
	from, to = richOrdered(from, to)
	pa, _, a := d.locate(from)
	pb, offb, b := d.locate(to)
	if b > a && pb > pa && offb == 0 {
		pb--
	}
	region := richCloneParas(d.paras[pa : pb+1])
	for i := range region {
		runs := region[i].Runs
		fn(&region[i])
		region[i].Runs = runs
	}
	if richParasEqual(region, d.paras[pa:pb+1]) {
		return
	}
	d.commit(pa, pb-pa+1, region, RichDocSel{a, b}, RichDocSel{a, b},
		false, 0, RichRun{}, d.open && d.groupDepth > 0)
}
