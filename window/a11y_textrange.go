package window

import "unicode"

// a11y_textrange.go — чистая арифметика текстовых диапазонов UI Automation.
//
// Паттерн Text устроен вокруг «диапазона» — пары позиций в тексте. Скринридер
// не просит у документа строку номер пять: он берёт диапазон вокруг каретки,
// расширяет его до слова или строки, сдвигает на соседнюю и читает. Вся эта
// работа — сдвиг на единицы, расширение, сравнение концов, поиск — не имеет
// отношения к COM, поэтому живёт здесь, без платформенного суффикса, и
// проверяется в общем прогоне тестов. COM-обвязка (a11y_textpattern_windows.go)
// только разбирает аргументы и вызывает эти функции.
//
// Позиции — в РУНАХ, как и у widget.AccessTextProvider. Диапазон — полуинтервал
// [Start, End): пустой (Start == End) означает «точку» между символами, то есть
// каретку.
//
// Что и как считаем единицами — решено так (текст движка плоский, без
// форматирования и без знания о переносах по ширине виджета):
//
//   - Character — один символ; пара «\r\n» считается ОДНИМ символом, иначе
//     скринридер, шагая по символам, останавливался бы посреди перевода строки.
//   - Word — слово вместе с пробелами за ним (так слова считает системное поле
//     Edit: диктор читает «привет » и каретка после пробела уже на следующем
//     слове). Перевод строки — отдельное «слово». Знаки препинания в слово
//     входят: «привет,» — одно слово. Это упрощение: настоящего словаря границ
//     (UAX #29) у движка нет.
//   - Line и Paragraph — ЛОГИЧЕСКАЯ строка до перевода строки, вместе с ним.
//     Перенос по ширине виджета из публичного интерфейса не виден, поэтому
//     длинный абзац, который на экране занимает три строки, здесь одна «строка».
//     Перевод строки входит в единицу, чтобы единицы покрывали текст без
//     дыр: иначе при шаге по строкам терялась бы позиция каждого переноса.
//   - Format — в простом тексте формат один на весь документ, поэтому единица
//     совпадает с Document.
//   - Page — у движка нет страниц; считаем всю страницу документом (Document).
//   - Document — весь текст.

// a11yTextUnit — единица текста (UIA TextUnit).
type a11yTextUnit int

const (
	a11yUnitCharacter a11yTextUnit = 0
	a11yUnitFormat    a11yTextUnit = 1
	a11yUnitWord      a11yTextUnit = 2
	a11yUnitLine      a11yTextUnit = 3
	a11yUnitParagraph a11yTextUnit = 4
	a11yUnitPage      a11yTextUnit = 5
	a11yUnitDocument  a11yTextUnit = 6
)

// valid — значение входит в перечисление UIA. Клиент вправе прислать любое
// число, и молча трактовать чужое как «символ» значило бы сдвигать диапазон
// не на то, что он просил.
func (u a11yTextUnit) valid() bool { return u >= a11yUnitCharacter && u <= a11yUnitDocument }

// a11yEndpoint — какой конец диапазона (UIA TextPatternRangeEndpoint).
type a11yEndpoint int

const (
	a11yEndStart a11yEndpoint = 0
	a11yEndEnd   a11yEndpoint = 1
)

func (e a11yEndpoint) valid() bool { return e == a11yEndStart || e == a11yEndEnd }

// a11yRange — диапазон текста: полуинтервал [Start, End) в рунах.
type a11yRange struct{ Start, End int }

// Empty — диапазон пуст: это точка между символами (каретка).
func (r a11yRange) Empty() bool { return r.Start == r.End }

// endpoint возвращает позицию нужного конца.
func (r a11yRange) endpoint(e a11yEndpoint) int {
	if e == a11yEndEnd {
		return r.End
	}
	return r.Start
}

// withEndpoint ставит конец на позицию pos и чинит порядок концов.
//
// Если двигаемый конец перешагнул второй, второй переезжает вслед за ним, и
// диапазон становится пустым. Так велит спецификация UIA: без этого получился
// бы «отрицательный» диапазон, и GetText на нём вернул бы что угодно.
func (r a11yRange) withEndpoint(e a11yEndpoint, pos int) a11yRange {
	if e == a11yEndEnd {
		r.End = pos
		if r.Start > r.End {
			r.Start = r.End
		}
		return r
	}
	r.Start = pos
	if r.End < r.Start {
		r.End = r.Start
	}
	return r
}

// a11yClampRange приводит диапазон к границам текста длиной n.
//
// Диапазон переживает правки: клиент держит его, пока пользователь печатает,
// и текст за это время мог стать короче. Позиция за концом без усечения
// превратила бы GetText в панику на срезе.
func a11yClampRange(r a11yRange, n int) a11yRange {
	if r.Start < 0 {
		r.Start = 0
	}
	if r.Start > n {
		r.Start = n
	}
	if r.End > n {
		r.End = n
	}
	if r.End < r.Start {
		r.End = r.Start
	}
	return r
}

// a11yIsBlank — пробельный символ внутри строки (перевод строки сюда не
// входит: он ограничивает слова и строки).
func a11yIsBlank(r rune) bool { return r == ' ' || r == '\t' || r == '\r' }

// a11yUnitBounds — границы единицы, в которой лежит позиция off.
//
// Единицы одного вида покрывают текст без пропусков и наложений: для любой
// позиции внутри единицы возвращаются одни и те же границы. На этом держится
// шаг по единицам (a11yStepForward/a11yStepBack).
//
// Позиция в конце текста (каретка после последнего символа) — не ошибка:
// скринридер спрашивает именно там, когда каретка стоит в конце. Единицей
// тогда считается последняя (так делает и системное поле — иначе диктор,
// дойдя до конца строки, читал бы пустоту), а для строки после завершающего
// перевода — честная пустая последняя строка.
func a11yUnitBounds(rs []rune, off int, unit a11yTextUnit) (int, int) {
	n := len(rs)
	if off < 0 {
		off = 0
	}
	if off > n {
		off = n
	}
	switch unit {
	case a11yUnitFormat, a11yUnitPage, a11yUnitDocument:
		return 0, n
	}
	if n == 0 {
		return 0, 0
	}
	if off == n {
		if (unit == a11yUnitLine || unit == a11yUnitParagraph) && rs[n-1] == '\n' {
			return n, n
		}
		off = n - 1
	}
	switch unit {
	case a11yUnitCharacter:
		return a11yCharUnit(rs, off)
	case a11yUnitWord:
		return a11yWordUnit(rs, off)
	case a11yUnitLine, a11yUnitParagraph:
		from, to := a11yLineBounds(rs, off)
		if to < n { // перевод строки принадлежит строке, которую он завершает
			to++
		}
		return from, to
	}
	return off, off
}

// a11yCharUnit — символ в позиции off (off < len(rs)); «\r\n» — один символ.
func a11yCharUnit(rs []rune, off int) (int, int) {
	n := len(rs)
	if rs[off] == '\n' && off > 0 && rs[off-1] == '\r' {
		return off - 1, off + 1
	}
	if rs[off] == '\r' && off+1 < n && rs[off+1] == '\n' {
		return off, off + 2
	}
	return off, off + 1
}

// a11yWordUnit — слово в позиции off (off < len(rs)) вместе с пробелами за ним.
//
// Переиспользует a11yWordBounds: он находит границы «куска» без пробелов, а
// здесь к нему приклеиваются хвостовые пробелы. Пробелы в начале строки и
// перевод строки — самостоятельные единицы: им не к чему клеиться, а терять
// их нельзя, иначе единицы перестали бы покрывать текст.
func a11yWordUnit(rs []rune, off int) (int, int) {
	n := len(rs)
	if rs[off] == '\n' {
		return off, off + 1
	}
	if !a11yIsBlank(rs[off]) {
		from, to := a11yWordBounds(rs, off)
		for to < n && a11yIsBlank(rs[to]) {
			to++
		}
		return from, to
	}
	// Позиция в пробелах: они принадлежат слову перед ними, если оно есть на
	// этой же строке.
	from := off
	for from > 0 && a11yIsBlank(rs[from-1]) {
		from--
	}
	to := off + 1
	for to < n && a11yIsBlank(rs[to]) {
		to++
	}
	if from > 0 && rs[from-1] != '\n' {
		wordFrom, _ := a11yWordBounds(rs, from-1)
		return wordFrom, to
	}
	return from, to
}

// a11yStepForward — начало следующей единицы после позиции pos; false, если
// шагать некуда (pos уже в конце текста).
//
// Конец текста возвращается как допустимый результат: это конец последней
// единицы, и концевая позиция диапазона вправе на нём стоять.
func a11yStepForward(rs []rune, pos int, unit a11yTextUnit) (int, bool) {
	if pos >= len(rs) {
		return pos, false
	}
	_, to := a11yUnitBounds(rs, pos, unit)
	return to, true
}

// a11yStepBack — начало единицы, в которой (или сразу перед которой) стоит
// pos; false, если pos уже в начале текста. Из середины единицы шаг назад
// ведёт к её началу — как и в системном поле.
func a11yStepBack(rs []rune, pos int, unit a11yTextUnit) (int, bool) {
	if pos <= 0 {
		return pos, false
	}
	if pos > len(rs) {
		pos = len(rs)
	}
	from, _ := a11yUnitBounds(rs, pos-1, unit)
	return from, true
}

// a11yExpandToUnit — ExpandToEnclosingUnit: диапазон становится ровно единицей,
// в которой лежит его начало. Больший диапазон укорачивается, меньший
// растёт — так велит спецификация.
func a11yExpandToUnit(rs []rune, r a11yRange, unit a11yTextUnit) a11yRange {
	r = a11yClampRange(r, len(rs))
	from, to := a11yUnitBounds(rs, r.Start, unit)
	return a11yRange{from, to}
}

// a11yMoveRange — Move: диапазон целиком сдвигается на count единиц (отрицательное
// число — назад). Возвращает новый диапазон и сколько единиц реально пройдено
// (со знаком): у границ текста сдвиг останавливается, и клиент по этому числу
// узнаёт, что дальше идти некуда.
//
// Непустой диапазон сначала расширяется до единицы, затем переезжает целиком.
// Пустой (каретка) остаётся пустым и переезжает как точка; он единственный
// вправе оказаться в самом конце текста — непустому там места нет.
func a11yMoveRange(rs []rune, r a11yRange, unit a11yTextUnit, count int) (a11yRange, int) {
	n := len(rs)
	r = a11yClampRange(r, n)
	degenerate := r.Empty()
	if !degenerate {
		r = a11yExpandToUnit(rs, r, unit)
	}
	if count == 0 {
		return r, 0
	}
	steps := count
	if steps < 0 {
		steps = -steps
	}
	pos := r.Start
	moved := 0
	for moved < steps {
		var next int
		var ok bool
		if count > 0 {
			next, ok = a11yStepForward(rs, pos, unit)
			// Непустой диапазон не может начаться в конце текста.
			if ok && !degenerate && next >= n {
				ok = false
			}
		} else {
			next, ok = a11yStepBack(rs, pos, unit)
		}
		if !ok {
			break
		}
		pos = next
		moved++
	}
	if count < 0 {
		moved = -moved
	}
	if degenerate {
		return a11yRange{pos, pos}, moved
	}
	if moved == 0 {
		return r, 0
	}
	_, to := a11yUnitBounds(rs, pos, unit)
	return a11yRange{pos, to}, moved
}

// a11yMoveEndpoint — MoveEndpointByUnit: двигается ОДИН конец диапазона.
// Результат и счёт — как у a11yMoveRange. Если конец перешагнул второй,
// диапазон схлопывается в точку (см. withEndpoint).
func a11yMoveEndpoint(rs []rune, r a11yRange, ep a11yEndpoint, unit a11yTextUnit, count int) (a11yRange, int) {
	r = a11yClampRange(r, len(rs))
	steps := count
	if steps < 0 {
		steps = -steps
	}
	pos := r.endpoint(ep)
	moved := 0
	for moved < steps {
		var next int
		var ok bool
		if count > 0 {
			next, ok = a11yStepForward(rs, pos, unit)
		} else {
			next, ok = a11yStepBack(rs, pos, unit)
		}
		if !ok {
			break
		}
		pos = next
		moved++
	}
	if count < 0 {
		moved = -moved
	}
	if moved == 0 {
		return r, 0
	}
	return r.withEndpoint(ep, pos), moved
}

// a11yMoveEndpointToRange — MoveEndpointByRange: конец ep диапазона r
// становится на позицию конца oep диапазона other.
func a11yMoveEndpointToRange(r a11yRange, ep a11yEndpoint, other a11yRange, oep a11yEndpoint) a11yRange {
	return r.withEndpoint(ep, other.endpoint(oep))
}

// a11yCompareEndpoints — CompareEndpoints: знак разности позиций концов
// (-1: у r раньше, 0: совпадают, 1: у r позже).
func a11yCompareEndpoints(r a11yRange, ep a11yEndpoint, other a11yRange, oep a11yEndpoint) int {
	a, b := r.endpoint(ep), other.endpoint(oep)
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// a11yRangeText — GetText: текст диапазона, усечённый до maxLen рун
// (maxLen < 0 — без ограничения, как -1 в UIA).
func a11yRangeText(rs []rune, r a11yRange, maxLen int) string {
	r = a11yClampRange(r, len(rs))
	seg := rs[r.Start:r.End]
	if maxLen >= 0 && len(seg) > maxLen {
		seg = seg[:maxLen]
	}
	return string(seg)
}

// a11yFoldRune — упрощённое сопоставление регистров для поиска без учёта
// регистра. Сопоставление ПО РУНАМ, а не через strings.ToLower всей строки:
// у некоторых букв нижний регистр занимает другое число рун, и после
// преобразования найденные позиции перестали бы совпадать с позициями в
// исходном тексте — скринридер выделил бы не то слово.
func a11yFoldRune(r rune) rune { return unicode.ToLower(r) }

// a11yFindText — FindText: ищет needle внутри диапазона r; backward — с конца.
// Найденное вхождение возвращается диапазоном; ok=false — не найдено (для UIA
// это штатный ответ NULL, а не ошибка). Пустая строка не находится: что
// считать её вхождением, спецификация не определяет.
func a11yFindText(rs []rune, r a11yRange, needle string, backward, ignoreCase bool) (a11yRange, bool) {
	nd := []rune(needle)
	r = a11yClampRange(r, len(rs))
	if len(nd) == 0 || r.End-r.Start < len(nd) {
		return a11yRange{}, false
	}
	match := func(at int) bool {
		for i, c := range nd {
			h := rs[at+i]
			if h == c {
				continue
			}
			if !ignoreCase || a11yFoldRune(h) != a11yFoldRune(c) {
				return false
			}
		}
		return true
	}
	last := r.End - len(nd)
	if backward {
		for at := last; at >= r.Start; at-- {
			if match(at) {
				return a11yRange{at, at + len(nd)}, true
			}
		}
		return a11yRange{}, false
	}
	for at := r.Start; at <= last; at++ {
		if match(at) {
			return a11yRange{at, at + len(nd)}, true
		}
	}
	return a11yRange{}, false
}

// a11yTextState — то, что клиент доступности знает о текстовом поле: по
// сравнению двух таких состояний решается, какие события поднимать.
type a11yTextState struct {
	Text           string
	Caret          int
	SelFrom, SelTo int
}

// a11yTextDiff — что изменилось между двумя состояниями: текст и/или
// каретка с выделением.
//
// Нужна пара флагов, а не один «изменилось»: смена текста и сдвиг каретки —
// разные события UIA (TextChanged и TextSelectionChanged), и клиенты
// подписываются на них раздельно. Диктор, например, на TextChanged читает
// вставленное, а на TextSelectionChanged — слово под кареткой.
func a11yTextDiff(prev, cur a11yTextState) (textChanged, selChanged bool) {
	textChanged = prev.Text != cur.Text
	selChanged = prev.Caret != cur.Caret || prev.SelFrom != cur.SelFrom || prev.SelTo != cur.SelTo
	return textChanged, selChanged
}
