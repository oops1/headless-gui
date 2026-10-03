package widget

// clipboard_html.go — оформленный текст в буфере обмена: чистая логика без
// платформенных зависимостей (тестируется в общем прогоне).
//
// Windows хранит HTML в формате «HTML Format» — это не сам HTML, а текст со
// служебным заголовком, в котором байтовые смещения указывают, где начинается
// документ и где — вставляемый фрагмент. Смещения считаются в БАЙТАХ UTF-8 от
// начала всех данных. Стоит ошибиться на байт — Word вставит обрезанную
// разметку, кусок заголовка или мусор вместо текста, и самая частая причина —
// кириллица: в ней символ занимает два байта, а считать «в символах» так же
// привычно, как неверно. Поэтому сборка и разбор заголовка живут отдельно от
// вызовов Win32 и покрыты тестами.

import (
	"bytes"
	"strconv"
	"strings"
)

const (
	// cfHTMLStartMarker и cfHTMLEndMarker обрамляют вставляемый фрагмент:
	// потребитель берёт то, что между ними, а остальное — рамка документа.
	cfHTMLStartMarker = "<!--StartFragment-->"
	cfHTMLEndMarker   = "<!--EndFragment-->"

	// cfHTMLOffsetWidth — ширина каждого смещения в заголовке. Числа пишутся
	// с ведущими нулями фиксированной ширины: тогда длина заголовка известна
	// заранее и смещения не зависят от самих себя (иначе длина заголовка
	// зависит от числа цифр в смещениях, а смещения — от длины заголовка).
	cfHTMLOffsetWidth = 10
)

// BuildCFHTML собирает данные формата «HTML Format» из HTML-фрагмента.
//
// Если на входе целый документ (есть <body>), маркеры фрагмента ставятся
// внутри тела; иначе фрагмент оборачивается в минимальный документ. Результат
// не содержит завершающего NUL — его добавляет тот, кто кладёт данные в
// буфер, потому что NUL не входит ни в одно из смещений.
//
// Функция экспортирована, чтобы приложение могло проверить или переиспользовать
// формат (например, для перетаскивания), и потому что платформенный файл
// Windows — не единственный потребитель.
func BuildCFHTML(fragment string) []byte {
	doc := wrapCFHTMLDocument(fragment)

	header := func(sh, eh, sf, ef int) string {
		return "Version:0.9\r\n" +
			"StartHTML:" + padOffset(sh) + "\r\n" +
			"EndHTML:" + padOffset(eh) + "\r\n" +
			"StartFragment:" + padOffset(sf) + "\r\n" +
			"EndFragment:" + padOffset(ef) + "\r\n"
	}
	// Заголовок с нулями: его длина от значений не зависит.
	hlen := len(header(0, 0, 0, 0))

	// Смещения маркеров считаем в байтах внутри документа, а затем сдвигаем
	// на длину заголовка. strings.Index возвращает именно байтовый индекс, а
	// len(string) — длину в байтах, так что кириллица считается верно.
	startMark := strings.Index(doc, cfHTMLStartMarker)
	endMark := strings.LastIndex(doc, cfHTMLEndMarker)

	return []byte(header(
		hlen,
		hlen+len(doc),
		hlen+startMark+len(cfHTMLStartMarker),
		hlen+endMark,
	) + doc)
}

// wrapCFHTMLDocument приводит вход к документу с маркерами фрагмента.
func wrapCFHTMLDocument(fragment string) string {
	// Маркеры в самом входе убираем: иначе их окажется два набора, а потребитель
	// возьмёт первый — и вставит часть содержимого или его обёртку.
	fragment = strings.ReplaceAll(fragment, cfHTMLStartMarker, "")
	fragment = strings.ReplaceAll(fragment, cfHTMLEndMarker, "")

	// Ищем теги в копии, где приведены к нижнему регистру только ASCII-буквы:
	// strings.ToLower способна поменять длину в байтах на некоторых символах и
	// сдвинула бы индексы относительно исходной строки.
	lower := asciiLower(fragment)
	if bi := strings.Index(lower, "<body"); bi >= 0 {
		if gt := strings.IndexByte(lower[bi:], '>'); gt >= 0 {
			open := bi + gt + 1
			if ci := strings.LastIndex(lower, "</body"); ci >= open {
				return fragment[:open] + cfHTMLStartMarker + fragment[open:ci] +
					cfHTMLEndMarker + fragment[ci:]
			}
			return fragment[:open] + cfHTMLStartMarker + fragment[open:] + cfHTMLEndMarker
		}
	}
	return "<html><body>\r\n" + cfHTMLStartMarker + fragment + cfHTMLEndMarker + "\r\n</body></html>"
}

// asciiLower приводит к нижнему регистру только ASCII, сохраняя длину в байтах.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// padOffset форматирует смещение фиксированной ширины с ведущими нулями.
func padOffset(n int) string {
	s := strconv.Itoa(n)
	if len(s) < cfHTMLOffsetWidth {
		s = strings.Repeat("0", cfHTMLOffsetWidth-len(s)) + s
	}
	return s
}

// ParseCFHTML достаёт HTML-фрагмент из данных формата «HTML Format».
//
// Чужие приложения пишут этот формат по-разному: смещения бывают без ведущих
// нулей, строки заканчиваются и \n, и \r\n, в конце может стоять NUL, а
// иногда в буфер кладут вовсе голый HTML без заголовка. Всё это принимается.
// Недостоверные смещения (за пределами данных, начало позже конца) не
// доверяются: данные приходят извне, и слепой срез по чужому числу — паника.
// ok == false, если фрагмента нет или он пуст.
func ParseCFHTML(data []byte) (fragment string, ok bool) {
	// Завершающие NUL — часть соглашения о буфере, не содержимого.
	data = bytes.TrimRight(data, "\x00")
	if len(data) == 0 {
		return "", false
	}

	// Голый HTML без заголовка: отдаём как есть.
	if !bytes.HasPrefix(data, []byte("Version:")) {
		return string(data), true
	}

	sh, eh, sf, ef := -1, -1, -1, -1
	rest := data
	// Заголовок — это строки «Ключ:значение» в самом начале; число разбираемых
	// строк ограничено, чтобы не гулять по всему документу.
headerLoop:
	for i := 0; i < 16 && len(rest) > 0; i++ {
		var line []byte
		if nl := bytes.IndexByte(rest, '\n'); nl < 0 {
			line, rest = rest, nil
		} else {
			line, rest = rest[:nl], rest[nl+1:]
		}
		line = bytes.TrimRight(line, "\r")
		colon := bytes.IndexByte(line, ':')
		if colon < 0 {
			break // заголовок закончился
		}
		val := strings.TrimSpace(string(line[colon+1:]))
		switch string(line[:colon]) {
		case "StartHTML":
			sh = parseOffset(val)
		case "EndHTML":
			eh = parseOffset(val)
		case "StartFragment":
			sf = parseOffset(val)
		case "EndFragment":
			ef = parseOffset(val)
		case "Version", "SourceURL":
			// известные, но для разбора не нужные
		default:
			break headerLoop // не наш ключ — значит, дальше уже документ
		}
	}

	if offsetsValid(sf, ef, len(data)) {
		frag := string(data[sf:ef])
		return frag, frag != ""
	}
	// Фрагмент не указан — берём весь документ целиком.
	if offsetsValid(sh, eh, len(data)) {
		doc := string(data[sh:eh])
		return doc, doc != ""
	}
	return "", false
}

// parseOffset разбирает смещение; -1 — отсутствует или не число.
func parseOffset(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return -1
	}
	return n
}

// offsetsValid проверяет, что [start:end) лежит внутри данных длиной n.
func offsetsValid(start, end, n int) bool {
	return start >= 0 && end >= start && end <= n
}
