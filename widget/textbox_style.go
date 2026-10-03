package widget

import (
	"image/color"
	"sort"
)

// textbox_style.go — стили на диапазонах текста многострочного редактора.
//
// Цвет у редактора был один на весь виджет, а подсветка синтаксиса, найденных
// слов и ошибок — обычная работа редактора кода. Хранить стили внутри виджета
// нельзя: правила знает приложение (разбор языка, строка поиска), а виджету
// достаточно спросить «что у этой строки». Поэтому приложение даёт Styler, а
// виджет зовёт его построчно, только для видимых строк, и кэширует ответ до
// первой правки текста: без кэша разбор языка шёл бы на каждый кадр.

// Style — оформление куска текста. Нулевое поле означает «как у виджета»:
// так приложению не нужно знать цвет темы, чтобы подсветить одно слово.
type Style struct {
	// Color — цвет букв; с нулевой альфой — TextColor виджета.
	Color color.RGBA
	// BG — фон за буквами (подсветка найденного, волна ошибки и т.п.); с
	// нулевой альфой фона нет. Цвет — как у остальных виджетов: RGBA с
	// предумноженной альфой.
	BG color.RGBA
	// Face — ИМЯ зарегистрированного шрифта (RegisterFont), которым рисуется
	// кусок; "" — шрифт виджета (FontName). Начертание — обычное, жирное,
	// курсив — выражается именно именем отдельного шрифта: синтетического
	// «утолщения» букв в движке нет.
	//
	// Раскладка (положение каретки, выделения, перенос) считается шрифтом
	// виджета, а не Face, поэтому гарнитуры должны совпадать по ширине знака —
	// жирный и курсивный вариант того же моноширинного шрифта. Иначе буквы
	// кусков уйдут относительно каретки.
	Face string
}

// isZero — стиль ничего не меняет.
func (s Style) isZero() bool { return s == Style{} }

// Span — стиль на диапазоне рун [From, To) внутри текста строки.
type Span struct {
	From, To int
	Style    Style
}

// Styler отдаёт стили строк приложению. Реализует приложение.
//
// LineSpans вызывается из потока отрисовки без блокировок виджета — внутри
// можно звать методы TextBox (GetText и т.п.). Метод должен быть быстрым: он
// стоит на пути кадра. Результат кэшируется до правки текста либо до
// InvalidateStyles, поэтому он обязан зависеть только от (line, text) и от
// состояния самого приложения, о смене которого оно сообщает InvalidateStyles.
type Styler interface {
	// LineSpans возвращает стили строки. line — номер ЛОГИЧЕСКОЙ строки (по
	// символам '\n', с нуля, перенос по словам на неё не влияет), text — её
	// текст без перевода строки. Границы From/To — в рунах от начала text.
	// Диапазоны могут перекрываться: более поздний в срезе рисуется поверх,
	// заданные им поля перекрывают прежние (цвет — от разбора синтаксиса, фон —
	// от поиска). Выходящие за строку обрезаются, пустые и перевёрнутые
	// игнорируются.
	LineSpans(line int, text string) []Span
}

// mergeStyle накладывает over на base: заданные поля over побеждают, остальные
// остаются от base.
func mergeStyle(base, over Style) Style {
	if over.Color.A != 0 {
		base.Color = over.Color
	}
	if over.BG.A != 0 {
		base.BG = over.BG
	}
	if over.Face != "" {
		base.Face = over.Face
	}
	return base
}

// tbPiece — кусок видимой строки с единым оформлением: руны [From, To) строки.
type tbPiece struct {
	From, To int
	Style    Style
}

// splitStylePieces режет строку длиной n рун на куски с единым оформлением.
// spans заданы в рунах ЛОГИЧЕСКОЙ строки, а видимая строка при переносе по
// словам — лишь её часть, начинающаяся в off: куски возвращаются в координатах
// видимой строки.
//
// Куски покрывают строку целиком и без дыр: промежутки между диапазонами —
// куски с нулевым стилем (рисуются цветом виджета). Соседние куски с равным
// стилем склеиваются: иначе строка, разрезанная на сотню одинаковых диапазонов,
// выводилась бы сотней вызовов отрисовки вместо одного.
//
// Работа линейна по числу диапазонов (с сортировкой), а не квадратична: строка
// минифицированного кода может нести тысячи диапазонов, и квадрат на каждый
// кадр заметен.
func splitStylePieces(n, off int, spans []Span) []tbPiece {
	if n <= 0 {
		return nil
	}
	type kept struct {
		f, t int
		st   Style
	}
	ks := make([]kept, 0, len(spans))
	for _, sp := range spans {
		f, t := sp.From-off, sp.To-off
		if f < 0 {
			f = 0
		}
		if t > n {
			t = n
		}
		if sp.From >= sp.To || f >= t || sp.Style.isZero() {
			continue
		}
		ks = append(ks, kept{f, t, sp.Style})
	}
	if len(ks) == 0 {
		return []tbPiece{{From: 0, To: n}}
	}

	// Границы, в которых набор действующих диапазонов может поменяться.
	bounds := make([]int, 0, 2*len(ks)+2)
	bounds = append(bounds, 0, n)
	for _, k := range ks {
		bounds = append(bounds, k.f, k.t)
	}
	sort.Ints(bounds)

	// Диапазоны по началу: при движении слева направо добавляем те, что
	// начались, и убираем закончившиеся. Порядок в active — порядок во входном
	// срезе: он определяет, кто «поверх».
	order := make([]int, len(ks))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return ks[order[a]].f < ks[order[b]].f })

	var (
		out    []tbPiece
		active []int
		next   int
	)
	for bi := 0; bi+1 < len(bounds); bi++ {
		p, q := bounds[bi], bounds[bi+1]
		if p == q {
			continue
		}
		for next < len(order) && ks[order[next]].f <= p {
			idx := order[next]
			at := sort.SearchInts(active, idx)
			active = append(active, 0)
			copy(active[at+1:], active[at:])
			active[at] = idx
			next++
		}
		w := active[:0]
		for _, idx := range active {
			if ks[idx].t > p {
				w = append(w, idx)
			}
		}
		active = w

		var st Style
		for _, idx := range active {
			st = mergeStyle(st, ks[idx].st)
		}
		if m := len(out); m > 0 && out[m-1].Style == st {
			out[m-1].To = q
			continue
		}
		out = append(out, tbPiece{From: p, To: q, Style: st})
	}
	return out
}

// tbSelOnLine — часть выделения [selLo, selHi) на видимой строке ln: границы в
// рунах и признак «выделение продолжается на следующей строке» (там рисуется
// небольшой хвост — отметка перевода строки). ok == false — строки выделение
// не касается.
//
// Условие selLo < ln.end+1, а не selLo < ln.end: выделение, начинающееся
// ровно на конце строки, захватывает её перевод и должно дать этот хвост, хотя
// ни одной руны строки в нём нет.
func tbSelOnLine(ln tbLine, selLo, selHi int) (lo, hi int, spill, ok bool) {
	if selLo < 0 || selLo >= ln.end+1 || selHi <= ln.start {
		return 0, 0, false, false
	}
	lo, hi = selLo, selHi
	if lo < ln.start {
		lo = ln.start
	}
	if hi > ln.end {
		hi = ln.end
	}
	return lo, hi, selHi > ln.end, true
}
