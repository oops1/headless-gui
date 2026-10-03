package widget

// richtext_caret.go — каретка RichText: положение, геометрия, мигание и
// навигация с клавиатуры.
//
// Что такое каретка здесь. Смещение в рунах документа (то же, что у выделения
// и скринридера) — и больше ничего: у текста нет «строки и столбца» как
// источника истины, они выводятся из раскладки. Каретка — это selCaret,
// подвижный конец выделения; якорь — selAnchor (см. richtext.go). Поэтому
// Shift+стрелка — это просто «подвинуть каретку, не трогая якорь».
//
// Две вещи, которые смещением не выразить, лежат рядом с ним:
//   - caretEOL — на стыке двух мягко перенесённых строк смещение у них общее
//     («после последнего слова» и «перед первым словом следующей»), а рисовать
//     каретку надо на одной из них. Без признака End на перенесённой строке
//     уносил бы каретку на следующую строку, а второй End «застревал» бы;
//   - wantX — «желаемый X» для Вверх/Вниз: при переходе через короткую строку
//     X надо помнить, иначе после двух нажатий каретка уезжает к левому краю.

import (
	"image"
	"sort"
	"time"
)

// richNowMs — часы для мигания; в тестах подменяются, чтобы фазу можно было
// проверять, не дожидаясь настоящих полусекунд.
var richNowMs = func() int64 { return time.Now().UnixMilli() }

// ─── Геометрия каретки по раскладке (чистые функции) ────────────────────────

// lineOfCaret — индекс строки, на которой стоит (рисуется) каретка off.
// Смещение, общее для конца мягко перенесённой строки и начала следующей,
// принадлежит следующей, если eol == false, и предыдущей — если true.
// Остальные смещения принадлежат строке однозначно: граница жёсткой строки
// (конец абзаца) занята разделителем, и следующая строка начинается за ним.
// Нет строк — -1.
func (l *richLayout) lineOfCaret(off int, eol bool) int {
	n := len(l.Lines)
	if n == 0 {
		return -1
	}
	// Конец строки растёт вместе с номером, поэтому ищем первую строку, чей
	// конец не левее каретки.
	k := sort.Search(n, func(i int) bool { return l.Lines[i].End >= off })
	if k >= n {
		return n - 1
	}
	if !eol && l.Lines[k].End == off && !l.Lines[k].HardEnd && k+1 < n {
		return k + 1
	}
	return k
}

// caretGeom — положение каретки off от левого верха содержимого: x, верх y и
// высота h. ok == false — раскладка пуста.
//
// Высота берётся по рану, который стоит СЛЕВА от каретки (на границе ранов —
// по левому, как в Word): печатаемый сейчас символ унаследует его оформление,
// и каретка показывает, какой кегль будет у набранного. Черта стоит на общей
// базовой линии строки, а не по верху строки: в строке с крупным словом
// каретка рядом с мелким словом не растягивается на всю высоту строки. В
// начале строки левого рана нет — берётся первый. В пустой строке сегментов
// нет, и высота — высота строки без добавки между строками.
func (l *richLayout) caretGeom(m richMeasurer, off int, eol bool) (x, y, h int, ok bool) {
	li := l.lineOfCaret(off, eol)
	if li < 0 {
		return 0, 0, 0, false
	}
	ln := &l.Lines[li]
	// Первый подходящий сегмент — левый на стыке двух.
	for i := range ln.Segments {
		s := &ln.Segments[i]
		if off >= s.Start && off <= s.End {
			x = s.X + prefixWidth(m, s, off-s.Start)
			return x, ln.Y + ln.Baseline - s.Ascent, s.Ascent + s.Descent, true
		}
	}
	h = ln.Height - l.LineGap
	if h < 1 {
		h = 1
	}
	return ln.X, ln.Y, h, true
}

// hitCaret — смещение под точкой (x, y) от левого верха содержимого и признак
// «конец мягко перенесённой строки». Тот же поиск, что у мыши (offsetAt):
// щелчок правее текста строки — её конец; но для мягко перенесённой строки это
// смещение совпадает с началом следующей, поэтому каретка должна остаться на
// той строке, по которой щёлкнули.
func (l *richLayout) hitCaret(m richMeasurer, x, y int) (off int, eol bool) {
	li := l.lineAt(y)
	off = l.offsetAt(m, x, y)
	if li >= 0 {
		ln := &l.Lines[li]
		eol = !ln.HardEnd && off == ln.End
	}
	return off, eol
}

// ─── Слова для навигации ────────────────────────────────────────────────────

// caretWordLeft — начало слова левее off. Классы символов те же, что у
// двойного щелчка (isWordRune): буквы и цифры — слово, остальное — «не слово».
// Через границу абзаца идём как через слово: с начала абзаца — на начало
// последнего слова предыдущего.
func (d *richDoc) caretWordLeft(off int) int {
	if len(d.paraStart) == 0 {
		return 0
	}
	pi := d.paraOf(off)
	if off <= d.paraStart[pi] {
		if pi == 0 {
			return d.paraStart[0]
		}
		pi--
		off = d.paraEnd(pi)
		if off == d.paraStart[pi] { // пустой абзац — слова в нём нет, встаём в него
			return off
		}
	}
	start := d.paraStart[pi]
	i := off
	for i > start && !isWordRune(d.runes[i-1]) {
		i--
	}
	for i > start && isWordRune(d.runes[i-1]) {
		i--
	}
	return i
}

// caretWordRight — начало следующего слова правее off. Если в абзаце слов
// больше нет — начало следующего абзаца (или конец текста): иначе на Ctrl+→
// приходилось бы дважды нажимать, чтобы пройти границу абзаца.
func (d *richDoc) caretWordRight(off int) int {
	n := len(d.runes)
	if len(d.paraStart) == 0 {
		return 0
	}
	pi := d.paraOf(off)
	end := d.paraEnd(pi)
	next := n
	if pi+1 < len(d.paraStart) {
		next = d.paraStart[pi+1]
	}
	if off >= end {
		return next
	}
	i := off
	for i < end && isWordRune(d.runes[i]) {
		i++
	}
	for i < end && !isWordRune(d.runes[i]) {
		i++
	}
	if i >= end {
		return next
	}
	return i
}

// ─── Состояние каретки ──────────────────────────────────────────────────────

// caretLocked — каретка, зажатая в документ.
func (t *RichText) caretLocked() int { return clampInt(t.selCaret, 0, len(t.doc.runes)) }

// caretJumpedLocked — каретка «прыгнула» не клавишами навигации (щелчок,
// программная установка): признак стыка строк и желаемый X больше не про неё,
// а мигание начинается заново — после движения каретка видна, а не в тёмной
// фазе.
func (t *RichText) caretJumpedLocked() {
	t.caretEOL = false
	t.wantXOk = false
	t.caretStamp = richNowMs()
	t.caretMovedLocked()
}

// caretMovedLocked — каретку двинули не правкой: набор прерывается (следующий
// символ начнёт новую запись отмены), а «стиль набора» — жирный, включённый
// Ctrl+B без выделения, — сбрасывается: он относился к тому месту, где каретка
// стояла. Правка текста, двигающая каретку сама, сюда не заходит.
func (t *RichText) caretMovedLocked() {
	t.pendOn = false
	t.rdoc.BreakUndoGroup()
}

// caretShownLocked — рисовать ли каретку и мигать ли ею (если есть фокус).
// У редактора она есть всегда, у просмотра — по ShowCaret.
func (t *RichText) caretShownLocked() bool { return t.ShowCaret || t.Editable }

// placeCaretLocked ставит каретку на pos. extend — не трогать якорь (Shift):
// если выделения не было, якорем становится прежняя каретка. keepX — не
// сбрасывать «желаемый X» (вертикальная навигация).
func (t *RichText) placeCaretLocked(pos int, eol, extend, keepX bool) {
	if extend {
		if t.selAnchor < 0 {
			t.selAnchor = t.caretLocked()
		}
	} else {
		t.selAnchor = -1
	}
	t.selCaret = clampInt(pos, 0, len(t.doc.runes))
	t.caretEOL = eol
	if !keepX {
		t.wantXOk = false
	}
	t.caretStamp = richNowMs()
	t.caretMovedLocked()
}

// CaretPosition — положение каретки в рунах документа (0 — перед первым
// символом, длина текста — после последнего). Это же подвижный конец
// выделения: при выделении Shift+← каретка слева, при Shift+→ справа.
func (t *RichText) CaretPosition() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.caretLocked()
}

// SetCaretPosition ставит каретку на pos (усекается в документ), снимает
// выделение и прокручивает текст так, чтобы каретка была видна: тот, кто
// ставит каретку программно, почти всегда хочет её показать.
func (t *RichText) SetCaretPosition(pos int) {
	t.mu.Lock()
	t.layoutLocked()
	t.placeCaretLocked(pos, false, false, false)
	t.ensureCaretVisibleLocked()
	t.mu.Unlock()
	t.Invalidate()
}

// caretRectLocked — прямоугольник каретки в координатах холста: черта в один
// пиксель шириной. Вызывать под t.mu.
func (t *RichText) caretRectLocked() image.Rectangle {
	lay := t.layoutLocked()
	b := t.Base.Bounds()
	cx, cy, ch, ok := lay.caretGeom(t.measurer, t.caretLocked(), t.caretEOL)
	if !ok {
		// Документ без абзацев: каретка стоит там, где встанет первая строка,
		// высотой в шрифт виджета.
		fm := t.measurer.FontMetrics(fontSizeOrDefault(t.FontSize), t.FontName)
		cx, cy, ch = 0, 0, fm.Ascent+fm.Descent
	}
	x := b.Min.X + t.PaddingX + cx
	y := b.Min.Y + t.PaddingY + cy - t.scrollY
	return image.Rect(x, y, x+1, y+ch)
}

// IMECaretRect — место каретки в координатах холста: по нему система ставит
// окно кандидатов ввода под кареткой, а не в углу экрана. Как у
// TextBox.IMECaretRect; часть контракта IMEComposer (richtext_ime.go).
func (t *RichText) IMECaretRect() image.Rectangle {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.caretRectLocked()
}

// ensureCaretVisibleLocked прокручивает так, чтобы строка каретки целиком
// помещалась в видимую область. true — прокрутка сместилась.
//
// Границы считаются с отступами виджета: у первой строки это прокрутка 0, у
// последней — максимальная, и верхний и нижний отступы остаются на месте.
// Если каретка выше видимой области (крошечный виджет), выигрывает верх.
func (t *RichText) ensureCaretVisibleLocked() bool {
	lay := t.layoutLocked()
	_, cy, ch, ok := lay.caretGeom(t.measurer, t.caretLocked(), t.caretEOL)
	if !ok {
		return false
	}
	hi := cy
	lo := cy + ch + 2*t.PaddingY - t.Base.Bounds().Dy()
	if lo > hi {
		lo = hi
	}
	s := t.scrollY
	if s > hi {
		s = hi
	} else if s < lo {
		s = lo
	}
	return t.setScrollLocked(s)
}

// ─── Мигание и отрисовка ────────────────────────────────────────────────────

// drawCaretLocked вызывается из Draw под замком: возвращает прямоугольник
// каретки и «рисовать ли сейчас» (режим с кареткой, фокус, светлая фаза
// мигания). Заодно запоминает отрисованную фазу — по ней NeedsAnimation
// понимает, что фаза сменилась и нужен кадр.
func (t *RichText) drawCaretLocked(focused bool) (image.Rectangle, bool) {
	if !t.caretShownLocked() || !focused {
		t.caretPhaseKnown = false
		return image.Rectangle{}, false
	}
	phase := t.caretPhaseNow()
	t.caretPhase, t.caretPhaseKnown = phase, true
	return t.caretRectLocked(), phase
}

// caretPhaseNow — светлая ли сейчас фаза мигания. Отсчёт от последнего
// движения каретки, а не от стенных часов: сдвинули каретку — она сразу видна.
func (t *RichText) caretPhaseNow() bool {
	ms := richNowMs() - t.caretStamp
	if ms < 0 {
		ms = 0
	}
	return caretPhaseAt(ms)
}

// NeedsAnimation — «нужен ли движку кадр ради мигающей каретки». Устроено как
// у TextBox: пока фаза совпадает с уже нарисованной, кадр не нужен; сменилась
// — заявляется ТОЛЬКО прямоугольник каретки (частичный кадр, а не полный
// перебор всего дерева), а ответ всё равно «нет»: движок увидит новое
// поколение инвалидации на следующем тике. Задержка в тик на полупериод
// 530 мс незаметна. Без ShowCaret и Editable и без фокуса — всегда false.
func (t *RichText) NeedsAnimation() bool {
	t.mu.Lock()
	if !t.caretShownLocked() || !t.focused {
		t.mu.Unlock()
		return false
	}
	phase := t.caretPhaseNow()
	if t.caretPhaseKnown && t.caretPhase == phase {
		t.mu.Unlock()
		return false
	}
	// Фазу фиксируем здесь же: если кадр до Draw не дойдёт (виджет скрыт),
	// иначе инвалидировали бы на каждом тике.
	t.caretPhase, t.caretPhaseKnown = phase, true
	r := t.caretRectLocked().Intersect(t.Base.Bounds())
	t.mu.Unlock()
	if !r.Empty() {
		t.invalidateRect(r)
	}
	return false
}

// ─── Навигация с клавиатуры ─────────────────────────────────────────────────

// navigateLocked выполняет клавишу навигации. true — клавиша обработана
// (что-то могло измениться). Вызывать под t.mu, раскладка уже построена.
//
// Правила:
//   - ←/→ по символу; без Shift при выделении каретка схлопывается в его край
//     (как в редакторах: ← — в левый, → — в правый), а не шагает от активного
//     конца; с Ctrl — по словам;
//   - ↑/↓ по визуальным строкам с «желаемым X»;
//   - Home/End — начало и конец ВИЗУАЛЬНОЙ строки, с Ctrl — документа;
//   - PgUp/PgDn — на высоту видимой области, текст листается вместе с кареткой.
//
// В режиме просмотра (ни ShowCaret, ни Editable) простые ↑/↓/PgUp/PgDn/Home/End
// без модификаторов листают текст, как до появления каретки; с Shift или Ctrl
// они всегда двигают каретку.
func (t *RichText) navigateLocked(code KeyCode, ctrl, shift bool) bool {
	lay := t.lay
	line := t.wheelStepLocked() / 3
	page := t.Base.Bounds().Dy() - line
	if page < line {
		page = line
	}
	scrollOnly := !t.caretShownLocked() && !ctrl && !shift

	caret := t.caretLocked()
	lo, hi := t.selRangeLocked()
	hasSel := lo != hi

	switch code {
	case KeyLeft:
		pos := caret - 1
		switch {
		case ctrl:
			pos = t.doc.caretWordLeft(caret)
		case hasSel && !shift:
			pos = lo
		}
		t.placeCaretLocked(pos, false, shift, false)
	case KeyRight:
		pos := caret + 1
		switch {
		case ctrl:
			pos = t.doc.caretWordRight(caret)
		case hasSel && !shift:
			pos = hi
		}
		t.placeCaretLocked(pos, false, shift, false)

	case KeyUp, KeyDown:
		dir := 1
		if code == KeyUp {
			dir = -1
		}
		if scrollOnly {
			t.setScrollLocked(t.scrollY + dir*line)
			return true
		}
		t.verticalLocked(dir, 0, shift)
	case KeyPageUp, KeyPageDown:
		dir := 1
		if code == KeyPageUp {
			dir = -1
		}
		if scrollOnly {
			t.setScrollLocked(t.scrollY + dir*page)
			return true
		}
		// Листаем сразу: каретка уезжает на ту же высоту и остаётся на том же
		// месте экрана, а не оказывается у края.
		t.setScrollLocked(t.scrollY + dir*page)
		t.verticalLocked(dir, dir*page, shift)

	case KeyHome, KeyEnd:
		if scrollOnly {
			target := 0
			if code == KeyEnd {
				target = 1 << 30
			}
			t.setScrollLocked(target)
			return true
		}
		switch {
		case ctrl && code == KeyHome:
			t.placeCaretLocked(0, false, shift, false)
		case ctrl:
			t.placeCaretLocked(len(t.doc.runes), false, shift, false)
		default:
			li := lay.lineOfCaret(caret, t.caretEOL)
			if li < 0 {
				return true
			}
			ln := &lay.Lines[li]
			if code == KeyHome {
				t.placeCaretLocked(ln.Start, false, shift, false)
			} else {
				// Конец мягко перенесённой строки — это же смещение, что и
				// начало следующей: прижимаем каретку к своей строке.
				t.placeCaretLocked(ln.End, !ln.HardEnd, shift, false)
			}
		}
	default:
		return false
	}
	t.ensureCaretVisibleLocked()
	return true
}

// verticalLocked двигает каретку на строку выше/ниже (page == 0) или на page
// пикселей (листание). «Желаемый X» запоминается на первом вертикальном шаге
// и живёт, пока каретку двигают только вертикально.
//
// На первой строке ↑ уходит в начало документа, на последней ↓ — в конец,
// как в TextBox: иначе клавиша «ничего не делала» бы у края.
func (t *RichText) verticalLocked(dir, page int, extend bool) {
	lay := t.lay
	caret := t.caretLocked()
	cur := lay.lineOfCaret(caret, t.caretEOL)
	if cur < 0 {
		return
	}
	if !t.wantXOk {
		x, _, _, _ := lay.caretGeom(t.measurer, caret, t.caretEOL)
		t.wantX, t.wantXOk = x, true
	}
	ti := cur + dir
	if page != 0 {
		ti = lay.lineAt(lay.Lines[cur].Y + page)
		if ti == cur { // лист короче строки — всё равно сдвигаемся
			ti = cur + dir
		}
	}
	switch {
	case ti < 0:
		t.placeCaretLocked(0, false, extend, false)
	case ti >= len(lay.Lines):
		t.placeCaretLocked(len(t.doc.runes), false, extend, false)
	default:
		ln := &lay.Lines[ti]
		// Точка — в середине строки по высоте: между абзацами с отступами
		// граничная ордината попала бы в соседнюю строку.
		off, eol := lay.hitCaret(t.measurer, t.wantX, ln.Y+ln.Height/2)
		t.placeCaretLocked(off, eol, extend, true)
	}
}
