package widget

// richtext_model.go — модель форматированного текста: раны и абзацы.
//
// Модель намеренно плоская: абзац — список ранов, ран — кусок текста с одним
// оформлением. Дерева (как в HTML) нет: вложенность «жирный внутри ссылки»
// выражается раном с несколькими признаками сразу, а плоский список
// раскладывать, копировать и показывать скринридеру в разы проще. Приложение,
// у которого своя разметка (Markdown, справка), раскладывает её в раны само —
// в этом и смысл виджета: он ничего не знает про разметку, только показывает
// результат.

import (
	"image/color"
	"strings"
)

// RichRun — кусок текста с единым оформлением.
//
// Нулевое значение поля означает «как у виджета»: приложению не нужно знать
// цвет и кегль темы, чтобы выделить одно слово.
type RichRun struct {
	// Text — текст рана. Символ '\n' внутри — явный перевод строки (мягкий:
	// абзац остаётся тем же, меняется только строка).
	Text string
	// Font — ИМЯ зарегистрированного шрифта (RegisterFont); "" — шрифт виджета.
	// Жирное и курсив выражаются именем отдельного шрифта (BuiltinFontBold и
	// т.д.): синтетического «утолщения» букв в движке нет.
	Font string
	// Size — кегль в пунктах; 0 — кегль виджета.
	Size float64
	// Color — цвет букв; с нулевой альфой — цвет текста виджета (у ссылки —
	// цвет ссылки темы).
	Color color.RGBA
	// BG — фон под раном; с нулевой альфой фона нет.
	BG color.RGBA
	// Underline и Strike — подчёркивание и зачёркивание. Толщина и положение
	// линий считаются по метрикам шрифта рана, а не константой: при кегле 9 и
	// при кегле 32 одна и та же «1 пиксель» выглядела бы то жирной, то
	// невидимой.
	Underline, Strike bool
	// Link — не пусто: ран является ссылкой с этим адресом. Виджет ничего не
	// открывает, а сообщает адрес через OnLinkClick.
	Link string
}

// RichParagraph — абзац: ряд ранов, выравнивание и отступы.
type RichParagraph struct {
	Runs []RichRun
	// Align — выравнивание строк абзаца в ширине виджета.
	Align TextAlign
	// Indent — отступ слева в пикселях, для всех строк абзаца (список, цитата).
	Indent int
	// SpaceBefore и SpaceAfter — пустое место над и под абзацем, пиксели.
	// Отступы складываются, а не «схлопываются», как поля в HTML: правило
	// «большее из двух» приложение выразит само, а скрытая арифметика
	// раскладки удивляла бы.
	SpaceBefore, SpaceAfter int
}

// richNormalizeText приводит переводы строки к '\n'.
//
// Текст из файла Windows приходит с "\r\n", а '\r' — управляющий символ: при
// отрисовке он давал бы «коробку» вместо пустоты, а в смещениях выделения —
// лишний символ, которого человек не видит.
func richNormalizeText(s string) string {
	if !strings.Contains(s, "\r") {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// richCopyParagraphs глубоко копирует абзацы с нормализацией текста.
//
// Копия обязательна: виджет читает содержимое из потока отрисовки, а
// вызывающий мог бы дописывать в свой срез ранов уже после SetParagraphs —
// это гонка данных на ровном месте. Заодно здесь же нормализуется текст.
func richCopyParagraphs(src []RichParagraph) []RichParagraph {
	out := make([]RichParagraph, len(src))
	for i, p := range src {
		runs := make([]RichRun, len(p.Runs))
		for j, r := range p.Runs {
			r.Text = richNormalizeText(r.Text)
			runs[j] = r
		}
		p.Runs = runs
		out[i] = p
	}
	return out
}

// richDoc — документ как одна строка: абзацы через '\n'. Смещения выделения,
// скринридера и раскладки — индексы рун именно этой строки.
type richDoc struct {
	runes     []rune
	paraStart []int // смещение начала каждого абзаца; paraStart[i+1]-1 — его '\n'
}

// richBuildDoc собирает richDoc из абзацев (текст уже нормализован).
func richBuildDoc(paras []RichParagraph) *richDoc {
	d := &richDoc{paraStart: make([]int, len(paras))}
	for i, p := range paras {
		if i > 0 {
			d.runes = append(d.runes, '\n')
		}
		d.paraStart[i] = len(d.runes)
		for _, r := range p.Runs {
			d.runes = append(d.runes, []rune(r.Text)...)
		}
	}
	return d
}

// paraEnd — смещение конца абзаца i (позиция его разделителя или конец текста).
func (d *richDoc) paraEnd(i int) int {
	if i+1 < len(d.paraStart) {
		return d.paraStart[i+1] - 1
	}
	return len(d.runes)
}

// paraOf — номер абзаца, которому принадлежит смещение off (позиция конца
// абзаца принадлежит ему же, а не следующему).
func (d *richDoc) paraOf(off int) int {
	lo, hi := 0, len(d.paraStart)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if d.paraStart[mid] <= off {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo < 0 {
		return 0
	}
	return lo
}

// wordBounds — границы слова вокруг off, не выходящие за абзац. Пробелы и
// знаки препинания образуют свои «слова» (двойной щелчок на пробеле выделяет
// пробелы) — так же, как в TextBox.
func (d *richDoc) wordBounds(off int) (int, int) {
	n := len(d.runes)
	if len(d.paraStart) == 0 {
		return 0, 0
	}
	if off < 0 {
		off = 0
	}
	if off > n {
		off = n
	}
	pi := d.paraOf(off)
	// Пустой абзац — слова в нём нет: выделяется пустое место.
	if d.paraStart[pi] == d.paraEnd(pi) {
		return d.paraStart[pi], d.paraStart[pi]
	}
	// Щелчок в самом конце абзаца (правее последней буквы): берём его
	// последний символ, а не разделитель перевода строки.
	if off >= d.paraEnd(pi) {
		off = d.paraEnd(pi) - 1
	}
	cls := isWordRune(d.runes[off])
	lo, hi := off, off+1
	end := d.paraEnd(pi)
	for lo > d.paraStart[pi] && isWordRune(d.runes[lo-1]) == cls {
		lo--
	}
	for hi < end && isWordRune(d.runes[hi]) == cls {
		hi++
	}
	return lo, hi
}
