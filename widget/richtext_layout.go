package widget

// richtext_layout.go — раскладка форматированного текста: чистая функция без
// окна, движка и замков.
//
// Почему вынесена отдельно. Раскладка — самая хрупкая часть rich text: здесь
// решается, где рвётся строка, какой она высоты и где стоит базовая линия. Её
// надо проверять числами, а не глазами, поэтому измерение сделано
// интерфейсом (richMeasurer): тесты подставляют моноширинного «измерителя» с
// предсказуемыми ширинами и метриками, а виджет — настоящий, через
// MeasureUITextFont и MeasureUIFontMetrics.
//
// Главное правило, ради которого всё затевалось: высота строки определяется
// ВСЕМИ ранами строки, а не базовым кеглем виджета. Крупный ран в строке с
// мелким раздвигает строку, а мелкий не «проваливается» — оба стоят на одной
// базовой линии. Блокнот на этом движке считал высоту по кеглю виджета, и
// крупный ран у него обрезался сверху.

import (
	"sort"
	"strings"
	"unicode"
)

// richMeasurer — всё, что раскладке нужно знать о шрифтах.
type richMeasurer interface {
	// TextWidth — ширина строки в пикселях именованным шрифтом ("" — шрифт по
	// умолчанию) и кеглем sizePt.
	TextWidth(text string, sizePt float64, font string) int
	// FontMetrics — вертикальные метрики шрифта (подъём, спуск, зазор).
	FontMetrics(sizePt float64, font string) FontMetrics
}

// richUIMeasurer — настоящий измеритель: тот же путь, которым движок потом
// рисует, поэтому раскладка и отрисовка не расходятся.
type richUIMeasurer struct{}

func (richUIMeasurer) TextWidth(text string, sizePt float64, font string) int {
	return MeasureUITextFont(text, sizePt, font)
}

func (richUIMeasurer) FontMetrics(sizePt float64, font string) FontMetrics {
	return MeasureUIFontMetrics(sizePt, font)
}

// richLayoutOpts — параметры раскладки.
type richLayoutOpts struct {
	// Width — ширина области текста, пиксели. Не больше нуля — без переноса
	// (строки ограничены только явными переводами): виджет без размера
	// ещё не знает, куда переносить.
	Width int
	// Font и Size — шрифт и кегль «по умолчанию» для ранов, где они не заданы.
	Font string
	Size float64
	// LineGap — добавка к высоте каждой строки, пиксели. Строка высотой «ровно
	// подъём + спуск» у большинства шрифтов выглядит слитно; добавка отделяет
	// строки, не меняя положения базовой линии.
	LineGap int
	// M — измеритель.
	M richMeasurer
}

// richSegment — кусок строки с единым оформлением: подряд идущие символы
// одного рана на одной строке. Пригоден и для отрисовки, и для попадания.
type richSegment struct {
	// RunIdx — номер рана в абзаце.
	RunIdx int
	// Run — копия рана; Text — только кусок, попавший на эту строку, а
	// Font и Size — ДЕЙСТВУЮЩИЕ (с учётом умолчаний виджета), по ним и
	// мерили, и будут рисовать.
	Run RichRun
	// Start, End — смещения куска в рунах документа (полуинтервал).
	Start, End int
	// X — левый край от левого края области текста (с отступом и
	// выравниванием); W — ширина БЕЗ хвостовых пробелов строки: они «висят»
	// за краем и не участвуют ни в центрировании, ни в попадании мышью.
	X, W int
	// Adv — полная ширина куска, с пробелами. Нужна выделению: подсветка
	// пробела в конце строки — это то, что человек ждёт увидеть.
	Adv int
	// Ascent, Descent — метрики шрифта рана: по ним ставится на базовую линию
	// и считается положение подчёркивания.
	Ascent, Descent int
}

// richLine — визуальная строка.
type richLine struct {
	// Para — номер абзаца.
	Para int
	// Y — верх строки от верха содержимого; Height — высота строки.
	Y, Height int
	// Baseline — расстояние от верха строки до базовой линии: наибольший
	// подъём среди ранов строки. Все сегменты строки стоят на ней.
	Baseline int
	// Start, End — смещения строки в рунах документа. Хвостовые пробелы входят
	// в строку; перевод строки (или разделитель абзацев) — нет, он лежит на
	// позиции End.
	Start, End int
	// X — левый край содержимого строки, Width — его ширина без хвостовых
	// пробелов.
	X, Width int
	// HardEnd — строка закончилась явным переводом или концом абзаца, а не
	// переносом: следом идёт символ-разделитель, который можно выделить.
	HardEnd  bool
	Segments []richSegment
}

// richLayout — результат раскладки.
type richLayout struct {
	// Width — ширина, для которой посчитано; Height — полная высота.
	Width, Height int
	// LineGap — добавка к высоте строки, с которой считали: каретке пустой
	// строки нужна высота шрифта без неё.
	LineGap int
	Lines   []richLine
	// ParaStart — смещение начала каждого абзаца; TextLen — длина текста.
	ParaStart []int
	TextLen   int
}

// Виды кусков текста при разборе абзаца.
const (
	rpWord  = iota // слово — единица, внутри которой строку не рвут
	rpSpace        // пробелы — висят в конце строки
	rpBreak        // явный перевод строки
)

// richIsSpace — пробельный символ, на котором строку можно разорвать.
// Неразрывный пробел (U+00A0) намеренно не считается: он для того и нужен,
// чтобы держать слова вместе.
func richIsSpace(r rune) bool { return r != ' ' && unicode.IsSpace(r) }

// richPiece — кусок абзаца: слово, пробелы или перевод строки, целиком из
// одного рана. Слово, разбитое границей ранов («Ма»+жирное «ма»), — несколько
// кусков: рвать его на этой границе нельзя, и разбор ниже собирает кусок в
// слово, а не рвёт по ранам.
type richPiece struct {
	kind     int
	run      int
	from, to int // руны в тексте рана
	off      int // смещение from от начала абзаца, в рунах
	text     string
	w        int
}

func (p richPiece) n() int { return p.to - p.from }

// layoutRich раскладывает абзацы по строкам шириной opt.Width.
func layoutRich(paras []RichParagraph, opt richLayoutOpts) *richLayout {
	lay := &richLayout{Width: opt.Width, LineGap: opt.LineGap, ParaStart: make([]int, len(paras))}
	b := &richBuilder{opt: opt, mcache: map[richMKey]FontMetrics{}}
	if b.opt.Size <= 0 {
		b.opt.Size = DefaultFontSizePt
	}
	y, base := 0, 0
	for pi, p := range paras {
		if pi > 0 {
			base++ // разделитель абзацев занимает одну позицию
		}
		lay.ParaStart[pi] = base
		if p.SpaceBefore > 0 {
			y += p.SpaceBefore
		}
		lines, n := b.paragraph(pi, p, base)
		for i := range lines {
			lines[i].Y = y
			y += lines[i].Height
		}
		lay.Lines = append(lay.Lines, lines...)
		if p.SpaceAfter > 0 {
			y += p.SpaceAfter
		}
		base += n
	}
	lay.Height = y
	lay.TextLen = base
	return lay
}

// richMKey — ключ кэша метрик внутри одной раскладки: метрики спрашиваются на
// каждый сегмент, а ответов — по числу различных шрифтов и кеглей.
type richMKey struct {
	size float64
	font string
}

// richBuilder — состояние одной раскладки.
type richBuilder struct {
	opt    richLayoutOpts
	mcache map[richMKey]FontMetrics
}

func (b *richBuilder) metrics(size float64, font string) FontMetrics {
	k := richMKey{size, font}
	if m, ok := b.mcache[k]; ok {
		return m
	}
	m := b.opt.M.FontMetrics(size, font)
	b.mcache[k] = m
	return m
}

// runFace — действующие шрифт и кегль рана.
func (b *richBuilder) runFace(r RichRun) (string, float64) {
	font, size := r.Font, r.Size
	if font == "" {
		font = b.opt.Font
	}
	if size <= 0 {
		size = b.opt.Size
	}
	return font, size
}

// pieces режет абзац на куски. Тексты ранов уже нормализованы ('\n' — перевод).
func (b *richBuilder) pieces(runs []RichRun) []richPiece {
	var out []richPiece
	off := 0
	for ri, r := range runs {
		font, size := b.runFace(r)
		rs := []rune(r.Text)
		for i := 0; i < len(rs); {
			kind := rpWord
			switch {
			case rs[i] == '\n':
				kind = rpBreak
			case richIsSpace(rs[i]):
				kind = rpSpace
			}
			j := i + 1
			if kind != rpBreak { // каждый перевод строки — отдельный кусок
				for j < len(rs) && rs[j] != '\n' && richIsSpace(rs[j]) == (kind == rpSpace) {
					j++
				}
			}
			p := richPiece{kind: kind, run: ri, from: i, to: j, off: off + i, text: string(rs[i:j])}
			if kind != rpBreak {
				p.w = b.opt.M.TextWidth(p.text, size, font)
			}
			out = append(out, p)
			i = j
		}
		off += len(rs)
	}
	return out
}

// richParaState — состояние раскладки одного абзаца: строка, которая сейчас
// набирается, и уже готовые.
type richParaState struct {
	b         *richBuilder
	pi        int
	p         RichParagraph
	base      int // смещение начала абзаца в документе
	indent    int
	avail     int // ширина под текст строки
	lines     []richLine
	placed    []richPiece // куски набираемой строки
	curW      int         // их суммарная ширина
	lineStart int         // смещение начала набираемой строки от начала абзаца
	lastRun   int         // ран, на котором строка возникла: берётся для пустой
}

// place кладёт кусок в набираемую строку.
func (st *richParaState) place(pc richPiece) {
	st.placed = append(st.placed, pc)
	st.curW += pc.w
	st.lastRun = pc.run
}

// finish закрывает набираемую строку; следующая начнётся со смещения nextStart.
func (st *richParaState) finish(hard bool, nextStart int) {
	st.lines = append(st.lines, st.b.buildLine(st, hard))
	st.placed = st.placed[:0]
	st.curW = 0
	st.lineStart = nextStart
}

// paragraph раскладывает один абзац; base — смещение его начала в документе.
// Возвращает строки (Y ещё не проставлен) и длину абзаца в рунах.
func (b *richBuilder) paragraph(pi int, p RichParagraph, base int) ([]richLine, int) {
	st := &richParaState{b: b, pi: pi, p: p, base: base, lastRun: -1}
	if st.indent = p.Indent; st.indent < 0 {
		st.indent = 0
	}
	st.avail = 1 << 30
	if b.opt.Width > 0 {
		if st.avail = b.opt.Width - st.indent; st.avail < 1 {
			st.avail = 1 // узкая область: слово рвётся посимвольно, а не исчезает
		}
	}
	if len(p.Runs) > 0 {
		st.lastRun = 0
	}

	pcs := b.pieces(p.Runs)
	total := 0
	for _, pc := range pcs {
		total += pc.n()
	}

	for i := 0; i < len(pcs); {
		if pcs[i].kind == rpBreak {
			st.lastRun = pcs[i].run
			st.finish(true, pcs[i].off+1)
			i++
			continue
		}
		// Единица переноса: слово (возможно, из нескольких ранов) и пробелы за ним.
		j, wordW := i, 0
		for j < len(pcs) && pcs[j].kind == rpWord {
			wordW += pcs[j].w
			j++
		}
		k := j
		for k < len(pcs) && pcs[k].kind == rpSpace {
			k++
		}

		if j > i {
			if len(st.placed) > 0 && st.curW+wordW > st.avail {
				// Слово не влезает в остаток строки — переносим его целиком.
				st.finish(false, pcs[i].off)
				continue
			}
			if st.curW+wordW <= st.avail {
				for _, pc := range pcs[i:j] {
					st.place(pc)
				}
			} else {
				// Слово шире пустой строки — жёсткий перенос по символам.
				st.hardBreak(pcs[i:j])
			}
		}
		// Пробелы после слова не проверяются на ширину: хвост строки «висит».
		for _, pc := range pcs[j:k] {
			st.place(pc)
		}
		i = k
	}
	// Последняя строка есть всегда: пустой абзац и абзац, оканчивающийся на
	// перевод строки, дают пустую строку, как и в редакторе.
	st.finish(true, total)
	return st.lines, total
}

// hardBreak укладывает слово шире строки, разрезая его по символам: строка
// заполняется до отказа, остаток уходит на следующую.
func (st *richParaState) hardBreak(word []richPiece) {
	b := st.b
	for _, wp := range word {
		font, size := b.runFace(st.p.Runs[wp.run])
		rest := wp
		for {
			if st.curW+rest.w <= st.avail {
				st.place(rest)
				break
			}
			k := b.fitPrefix(rest, font, size, st.avail-st.curW)
			if k == 0 {
				if len(st.placed) > 0 {
					st.finish(false, rest.off) // место кончилось — с новой строки
					continue
				}
				k = 1 // даже один символ не лезет: берём его, иначе зациклимся
			}
			if k >= rest.n() {
				st.place(rest)
				break
			}
			head, tail := b.split(rest, k, font, size)
			st.place(head)
			st.finish(false, tail.off)
			rest = tail
		}
	}
}

// fitPrefix — наибольшее число рун от начала куска, ширина которых не больше
// avail. Ширина префикса растёт с длиной, поэтому ищем двоичным поиском: на
// строке в тысячу символов это десять измерений, а не тысяча.
func (b *richBuilder) fitPrefix(pc richPiece, font string, size float64, avail int) int {
	if avail <= 0 {
		return 0
	}
	rs := []rune(pc.text)
	lo, hi := 0, len(rs)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if b.opt.M.TextWidth(string(rs[:mid]), size, font) <= avail {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// split разрезает кусок на два по k рун от начала.
func (b *richBuilder) split(pc richPiece, k int, font string, size float64) (richPiece, richPiece) {
	rs := []rune(pc.text)
	head, tail := pc, pc
	head.text, head.to = string(rs[:k]), pc.from+k
	tail.text, tail.from, tail.off = string(rs[k:]), pc.from+k, pc.off+k
	head.w = b.opt.M.TextWidth(head.text, size, font)
	tail.w = b.opt.M.TextWidth(tail.text, size, font)
	return head, tail
}

// buildLine собирает строку из уложенных кусков: склеивает соседние куски
// одного рана в сегменты, считает метрики, хвостовые пробелы и выравнивание.
func (b *richBuilder) buildLine(st *richParaState, hard bool) richLine {
	p, placed, base := st.p, st.placed, st.base
	lineStart, lastRun, indent, avail := st.lineStart, st.lastRun, st.indent, st.avail

	ln := richLine{Para: st.pi, HardEnd: hard}
	m := b.opt.M

	// Сегменты: подряд идущие куски одного рана без разрыва в тексте.
	type acc struct {
		seg    richSegment
		runes  int
		localS int // начало от начала абзаца
	}
	var accs []acc
	for _, pc := range placed {
		if n := len(accs); n > 0 && accs[n-1].seg.RunIdx == pc.run &&
			accs[n-1].localS+accs[n-1].runes == pc.off {
			accs[n-1].seg.Run.Text += pc.text
			accs[n-1].runes += pc.n()
			continue
		}
		run := p.Runs[pc.run]
		run.Text = pc.text
		accs = append(accs, acc{seg: richSegment{RunIdx: pc.run, Run: run}, runes: pc.n(), localS: pc.off})
	}

	maxAsc, maxDesc := 0, 0
	segs := make([]richSegment, len(accs))
	for i, a := range accs {
		s := a.seg
		s.Run.Font, s.Run.Size = b.runFace(s.Run)
		s.Start = base + a.localS
		s.End = s.Start + a.runes
		s.Adv = m.TextWidth(s.Run.Text, s.Run.Size, s.Run.Font)
		s.W = s.Adv
		fm := b.metrics(s.Run.Size, s.Run.Font)
		s.Ascent, s.Descent = fm.Ascent, fm.Descent
		if fm.Ascent > maxAsc {
			maxAsc = fm.Ascent
		}
		if fm.Descent > maxDesc {
			maxDesc = fm.Descent
		}
		segs[i] = s
	}
	if len(segs) == 0 {
		// Пустая строка: высота по рану, на котором она возникла (перевод
		// строки, пустой абзац), иначе по умолчанию виджета.
		var run RichRun
		if lastRun >= 0 && lastRun < len(p.Runs) {
			run = p.Runs[lastRun]
		}
		font, size := b.runFace(run)
		fm := b.metrics(size, font)
		maxAsc, maxDesc = fm.Ascent, fm.Descent
	}

	ln.Start = base + lineStart
	ln.End = ln.Start
	if n := len(segs); n > 0 {
		ln.End = segs[n-1].End
	}
	ln.Baseline = maxAsc
	ln.Height = maxAsc + maxDesc + b.opt.LineGap

	// Хвостовые пробелы строки висят: убираем их из ширины, иначе центрирование
	// и выравнивание вправо сдвигали бы текст на невидимую величину.
	for i := len(segs) - 1; i >= 0; i-- {
		t := strings.TrimRightFunc(segs[i].Run.Text, richIsSpace)
		if t == segs[i].Run.Text {
			break
		}
		if t == "" {
			segs[i].W = 0
			continue
		}
		segs[i].W = m.TextWidth(t, segs[i].Run.Size, segs[i].Run.Font)
		break
	}
	x := 0
	for i := range segs {
		segs[i].X = x
		x += segs[i].W
	}
	ln.Width = x

	x0 := indent
	if b.opt.Width > 0 && p.Align != TextAlignLeft {
		if free := avail - ln.Width; free > 0 {
			if p.Align == TextAlignCenter {
				x0 += free / 2
			} else {
				x0 += free
			}
		}
	}
	for i := range segs {
		segs[i].X += x0
	}
	ln.X = x0
	ln.Segments = segs
	return ln
}

// ─── Попадание и геометрия выделения ────────────────────────────────────────

// lineAt — индекс строки по y (от верха содержимого). Выше первой — первая,
// ниже последней — последняя, в промежутке между абзацами — следующая:
// щелчок в пустом поле между абзацами не должен промахиваться в никуда.
func (l *richLayout) lineAt(y int) int {
	n := len(l.Lines)
	if n == 0 {
		return -1
	}
	i := sort.Search(n, func(i int) bool { return l.Lines[i].Y+l.Lines[i].Height > y })
	if i >= n {
		i = n - 1
	}
	return i
}

// visibleLines — полуинтервал индексов строк, пересекающих [top, bottom).
func (l *richLayout) visibleLines(top, bottom int) (first, last int) {
	n := len(l.Lines)
	first = sort.Search(n, func(i int) bool { return l.Lines[i].Y+l.Lines[i].Height > top })
	last = sort.Search(n, func(i int) bool { return l.Lines[i].Y >= bottom })
	return first, last
}

// prefixWidth — ширина первых k рун сегмента.
func prefixWidth(m richMeasurer, s *richSegment, k int) int {
	if k <= 0 {
		return 0
	}
	rs := []rune(s.Run.Text)
	if k >= len(rs) {
		return s.Adv
	}
	return m.TextWidth(string(rs[:k]), s.Run.Size, s.Run.Font)
}

// richCharAt — ближайшая к rx (от левого края сегмента) граница между рунами
// сегмента, в рунах от его начала.
func richCharAt(m richMeasurer, s *richSegment, rx int) int {
	n := len([]rune(s.Run.Text))
	lo, hi := 0, n
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if prefixWidth(m, s, mid) <= rx {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	k := lo
	if k < n && rx-prefixWidth(m, s, k) >= prefixWidth(m, s, k+1)-rx {
		k++ // правая граница ближе
	}
	return k
}

// offsetAt — смещение (в рунах документа) под точкой (x, y), заданной от
// левого верха содержимого. Всегда возвращает осмысленную границу: щелчок
// левее текста — начало строки, правее — её конец, ниже всего — конец текста.
func (l *richLayout) offsetAt(m richMeasurer, x, y int) int {
	li := l.lineAt(y)
	if li < 0 {
		return 0
	}
	ln := &l.Lines[li]
	if len(ln.Segments) == 0 || x < ln.Segments[0].X {
		return ln.Start
	}
	for i := range ln.Segments {
		s := &ln.Segments[i]
		if x < s.X+s.W {
			return s.Start + richCharAt(m, s, x-s.X)
		}
	}
	return ln.End
}

// segmentAt — сегмент ровно под точкой (без «ближайшего»): для ссылок важно,
// что курсор именно над буквами, а не рядом с ними. nil — там ничего нет.
func (l *richLayout) segmentAt(x, y int) *richSegment {
	if y < 0 || len(l.Lines) == 0 {
		return nil
	}
	li := l.lineAt(y)
	ln := &l.Lines[li]
	if y < ln.Y || y >= ln.Y+ln.Height {
		return nil
	}
	for i := range ln.Segments {
		s := &ln.Segments[i]
		if x >= s.X && x < s.X+s.W {
			return s
		}
	}
	return nil
}

// selectionX — горизонтальный отрезок строки, занятый выделением [lo, hi).
// ok == false — строки выделение не касается.
func (l *richLayout) selectionX(m richMeasurer, ln *richLine, lo, hi int) (x0, x1 int, ok bool) {
	for i := range ln.Segments {
		s := &ln.Segments[i]
		a, b := lo, hi
		if a < s.Start {
			a = s.Start
		}
		if b > s.End {
			b = s.End
		}
		if a >= b {
			continue
		}
		sx0 := s.X
		if a > s.Start {
			sx0 += prefixWidth(m, s, a-s.Start)
		}
		sx1 := s.X + s.Adv
		if b < s.End {
			sx1 = s.X + prefixWidth(m, s, b-s.Start)
		}
		if !ok {
			x0 = sx0
		}
		x1, ok = sx1, true
	}
	return x0, x1, ok
}

// richDecoration — положение и толщина подчёркивания и зачёркивания от
// метрик шрифта: ul — верхний ряд подчёркивания ниже базовой линии (>0),
// strike — верхний ряд зачёркивания относительно базовой линии (<0 — выше).
//
// Метрики TrueType (положение и толщина линии) наружу не выходят, поэтому
// берутся доли спуска и подъёма. У типичных шрифтов подчёркивание лежит на
// ~0.11 em ниже базовой, толщина ~0.06 em, а спуск ~0.24 em: доля спуска
// (45% и 25%) даёт эти же значения при любом кегле, а для зачёркивания берём
// 30% подъёма — середину строчных букв.
func richDecoration(ascent, descent int) (ul, strike, thick int) {
	thick = (descent + 2) / 4
	if thick < 1 {
		thick = 1
	}
	ul = (descent*45 + 50) / 100
	if ul+thick > descent { // линия не должна выходить за низ строки
		ul = descent - thick
	}
	if ul < 1 {
		ul = 1
	}
	strike = -((ascent*30+50)/100 + thick/2)
	return ul, strike, thick
}
