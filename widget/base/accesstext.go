package base

// accesstext.go — текстовая семантика для скринридера.
//
// До этого содержимое поля ввода уезжало к скринридеру одной строкой —
// «значением» элемента, как у ползунка. Для кнопки или переключателя этого
// довольно, для текста — нет: скринридер читает документ по строкам и
// словам, следует за кареткой, озвучивает выделение, а ничего этого в одной
// строке нет. В редакторе он объявлял бы весь текст заново на каждое
// нажатие клавиши.
//
// Платформенные мосты (UI Automation на Windows, AT-SPI на Linux) спрашивают
// эти данные у виджета напрямую, а не из снимка семантики: текст и каретка
// меняются на каждое нажатие, и снимок, снятый раз в сто пятьдесят
// миллисекунд, отставал бы от курсора.

// AccessTextProvider — виджет, чьё содержимое читается как ТЕКСТ.
//
// Смещения — в РУНАХ, а не в байтах: скринридер считает позиции в символах,
// и кириллическая строка в байтах дала бы вдвое большие числа.
type AccessTextProvider interface {
	// AccessText — всё содержимое целиком.
	AccessText() string
	// AccessCaret — позиция каретки.
	AccessCaret() int
	// AccessSelection — выделение; равные границы означают, что его нет.
	AccessSelection() (from, to int)
	// AccessReadOnly — текст доступен только для чтения.
	AccessReadOnly() bool
}

// AccessTextSetter — виджет, которому скринридер (или другое средство
// автоматизации) может задать текст целиком: UIA ValuePattern.SetValue,
// AT-SPI EditableText.SetTextContents.
//
// Отдельно от AccessTextProvider: читать текст можно у всякого поля, а
// править — не у всякого.
type AccessTextSetter interface {
	// AccessSetText заменяет содержимое; false — виджет отказался (только
	// для чтения или выключен).
	AccessSetText(s string) bool
}

// AccessCaretSetter — скринридер может переставить каретку: так он водит
// человека по тексту, оставаясь согласованным с тем, что видно на экране.
type AccessCaretSetter interface {
	// AccessSetCaret ставит каретку на позицию в рунах; false — виджет
	// отказался.
	AccessSetCaret(pos int) bool
}

// AccessSelectionSetter — скринридер может выделить диапазон: паттерн Text
// UI Automation (ITextRangeProvider::Select) выделяет не точку, а отрезок.
//
// Отдельно от AccessCaretSetter: каретку ставят все поля, а выделение — не
// обязательно. Без него Select на непустом диапазоне свёлся бы к постановке
// каретки, и диктор, прочитав слово, не смог бы его подсветить.
type AccessSelectionSetter interface {
	// AccessSetSelection выделяет [from, to) в рунах (from <= to), каретка
	// встаёт в to; from == to снимает выделение и ставит каретку. false —
	// виджет отказался.
	AccessSetSelection(from, to int) bool
}

// ─── Общее ──────────────────────────────────────────────────────────────────

// AccessTextOf возвращает текстовую семантику виджета, если он её отдаёт.
func AccessTextOf(w Widget) (AccessTextProvider, bool) {
	tp, ok := w.(AccessTextProvider)
	return tp, ok
}

// AccessClampPair усекает границы выделения до длины текста n и упорядочивает
// их. Границы приходят от скринридера, который вправе промахнуться за конец
// текста (он держит диапазон, пока пользователь печатает), а выход за массив
// рун без усечения — паника в горутине движка.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func AccessClampPair(from, to, n int) (int, int) {
	if from < 0 {
		from = 0
	}
	if to < 0 {
		to = 0
	}
	if from > n {
		from = n
	}
	if to > n {
		to = n
	}
	if to < from {
		from, to = to, from
	}
	return from, to
}

// AccessRuneLen — длина текста в рунах: столько позиций у каретки минус одна.
func AccessRuneLen(s string) int { return len([]rune(s)) }

// AccessSubstring — отрезок текста по смещениям в рунах, с усечением до
// границ. Границы приходят от скринридера, и он вправе спросить больше, чем
// есть: за концом текста он ожидает пустую строку, а не отказ.
func AccessSubstring(s string, from, to int) string {
	rs := []rune(s)
	if from < 0 {
		from = 0
	}
	if to > len(rs) || to < 0 {
		to = len(rs)
	}
	if from >= to {
		return ""
	}
	return string(rs[from:to])
}
