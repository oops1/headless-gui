package window

import "testing"

// Тесты арифметики текстовых диапазонов. Она чистая — без COM, — поэтому
// работает в общем прогоне на любой платформе; COM-обвязка проверяется
// отдельно (a11y_textpattern_windows_test.go).

// textSamples — тексты для проверок: кириллица, пробелы подряд, пустые строки,
// перевод строки в конце, «\r\n».
var textSamples = []string{
	"",
	"a",
	"привет мир",
	"привет мир\nвторая  строка\n\nконец",
	"строка с переводом в конце\n",
	"  ведущие пробелы\n\tи табуляция\n",
	"win\r\nстроки\r\nтут",
	"\n\n",
}

var allUnits = []a11yTextUnit{
	a11yUnitCharacter, a11yUnitFormat, a11yUnitWord, a11yUnitLine,
	a11yUnitParagraph, a11yUnitPage, a11yUnitDocument,
}

// Единицы одного вида обязаны покрывать текст без пропусков и наложений:
// шаг по единицам, начатый с нуля, доходит до конца, а любая позиция внутри
// единицы даёт одни и те же границы. Иначе скринридер, идущий по словам или
// строкам, терял бы куски текста или читал бы их дважды.
func TestA11yUnitBounds_TileText(t *testing.T) {
	for _, text := range textSamples {
		rs := []rune(text)
		for _, unit := range allUnits {
			pos := 0
			for pos < len(rs) {
				from, to := a11yUnitBounds(rs, pos, unit)
				if from != pos {
					t.Fatalf("%q unit=%d: единица с позиции %d начинается с %d — дыра или наложение", text, unit, pos, from)
				}
				if to <= from {
					t.Fatalf("%q unit=%d: пустая единица [%d,%d) в середине текста", text, unit, from, to)
				}
				for i := from; i < to; i++ {
					f2, t2 := a11yUnitBounds(rs, i, unit)
					if f2 != from || t2 != to {
						t.Fatalf("%q unit=%d: позиция %d даёт [%d,%d), а позиция %d — [%d,%d)", text, unit, i, f2, t2, pos, from, to)
					}
				}
				pos = to
			}
			if pos != len(rs) {
				t.Errorf("%q unit=%d: покрытие кончилось на %d из %d", text, unit, pos, len(rs))
			}
		}
	}
}

func TestA11yUnitBounds_Samples(t *testing.T) {
	rs := []rune("привет мир\nвторая  строка\n\nконец")
	cases := []struct {
		off      int
		unit     a11yTextUnit
		from, to int
	}{
		{0, a11yUnitCharacter, 0, 1},
		{3, a11yUnitCharacter, 3, 4},
		{0, a11yUnitWord, 0, 7},    // «привет » — слово с пробелом за ним
		{6, a11yUnitWord, 0, 7},    // позиция в пробеле — всё то же слово
		{7, a11yUnitWord, 7, 10},   // «мир»
		{10, a11yUnitWord, 10, 11}, // перевод строки — отдельная единица
		{0, a11yUnitLine, 0, 11},   // строка вместе с переводом
		{10, a11yUnitLine, 0, 11},  // сам перевод принадлежит своей строке
		{11, a11yUnitLine, 11, 26},
		{26, a11yUnitLine, 26, 27}, // пустая строка — это один перевод
		{27, a11yUnitLine, 27, 32}, // последняя — без перевода в конце
		{5, a11yUnitParagraph, 0, 11},
		{5, a11yUnitDocument, 0, 32},
		{5, a11yUnitFormat, 0, 32},
		{5, a11yUnitPage, 0, 32},
	}
	for _, c := range cases {
		from, to := a11yUnitBounds(rs, c.off, c.unit)
		if from != c.from || to != c.to {
			t.Errorf("off=%d unit=%d: [%d,%d), ждал [%d,%d)", c.off, c.unit, from, to, c.from, c.to)
		}
	}
}

// Пара «\r\n» — один символ: шаг по символам не должен останавливаться
// посреди перевода строки.
func TestA11yCharUnit_CRLF(t *testing.T) {
	rs := []rune("a\r\nb")
	for _, off := range []int{1, 2} {
		if from, to := a11yUnitBounds(rs, off, a11yUnitCharacter); from != 1 || to != 3 {
			t.Errorf("off=%d: [%d,%d), ждал [1,3)", off, from, to)
		}
	}
}

// Каретка после последнего символа: единицей считается последняя — иначе
// скринридер, дойдя до конца строки, читал бы пустоту. Исключение — строка
// после завершающего перевода: она и правда пуста.
func TestA11yUnitBounds_EndOfText(t *testing.T) {
	rs := []rune("раз два")
	if from, to := a11yUnitBounds(rs, 7, a11yUnitWord); from != 4 || to != 7 {
		t.Errorf("слово в конце текста [%d,%d), ждал [4,7)", from, to)
	}
	if from, to := a11yUnitBounds(rs, 7, a11yUnitCharacter); from != 6 || to != 7 {
		t.Errorf("символ в конце текста [%d,%d), ждал [6,7)", from, to)
	}
	nl := []rune("раз\n")
	if from, to := a11yUnitBounds(nl, 4, a11yUnitLine); from != 4 || to != 4 {
		t.Errorf("последняя строка после перевода [%d,%d), ждал пустую [4,4)", from, to)
	}
	if from, to := a11yUnitBounds(nil, 0, a11yUnitWord); from != 0 || to != 0 {
		t.Errorf("слово в пустом тексте [%d,%d)", from, to)
	}
}

func TestA11yExpandToUnit(t *testing.T) {
	rs := []rune("привет мир\nвторая")
	// Каретка в середине слова — диапазон становится словом.
	if got := a11yExpandToUnit(rs, a11yRange{2, 2}, a11yUnitWord); got != (a11yRange{0, 7}) {
		t.Errorf("слово: %+v", got)
	}
	// Диапазон больше единицы укорачивается до единицы с его началом.
	if got := a11yExpandToUnit(rs, a11yRange{2, 15}, a11yUnitCharacter); got != (a11yRange{2, 3}) {
		t.Errorf("символ: %+v", got)
	}
	// Диапазон меньше единицы растёт.
	if got := a11yExpandToUnit(rs, a11yRange{12, 13}, a11yUnitLine); got != (a11yRange{11, 17}) {
		t.Errorf("строка: %+v", got)
	}
	// Позиции за текстом усекаются, а не роняют.
	if got := a11yExpandToUnit(rs, a11yRange{100, 200}, a11yUnitDocument); got != (a11yRange{0, 17}) {
		t.Errorf("документ при позициях за концом: %+v", got)
	}
}

func TestA11yMoveRange_Forward(t *testing.T) {
	rs := []rune("раз два три")
	r := a11yRange{0, 4} // «раз »
	got, moved := a11yMoveRange(rs, r, a11yUnitWord, 1)
	if got != (a11yRange{4, 8}) || moved != 1 {
		t.Errorf("на слово вперёд: %+v moved=%d", got, moved)
	}
	// Двух слов осталось одно: сдвиг останавливается, а клиенту сообщается,
	// сколько реально пройдено.
	got, moved = a11yMoveRange(rs, a11yRange{4, 8}, a11yUnitWord, 5)
	if got != (a11yRange{8, 11}) || moved != 1 {
		t.Errorf("с упором в конец: %+v moved=%d", got, moved)
	}
	// Дальше идти некуда.
	got, moved = a11yMoveRange(rs, a11yRange{8, 11}, a11yUnitWord, 1)
	if got != (a11yRange{8, 11}) || moved != 0 {
		t.Errorf("за концом: %+v moved=%d", got, moved)
	}
}

func TestA11yMoveRange_Backward(t *testing.T) {
	rs := []rune("раз два три")
	got, moved := a11yMoveRange(rs, a11yRange{8, 11}, a11yUnitWord, -1)
	if got != (a11yRange{4, 8}) || moved != -1 {
		t.Errorf("на слово назад: %+v moved=%d", got, moved)
	}
	got, moved = a11yMoveRange(rs, a11yRange{4, 8}, a11yUnitWord, -9)
	if got != (a11yRange{0, 4}) || moved != -1 {
		t.Errorf("с упором в начало: %+v moved=%d", got, moved)
	}
	got, moved = a11yMoveRange(rs, a11yRange{0, 4}, a11yUnitWord, -1)
	if got != (a11yRange{0, 4}) || moved != 0 {
		t.Errorf("до начала: %+v moved=%d", got, moved)
	}
}

// Непустой диапазон перед сдвигом расширяется до единицы.
func TestA11yMoveRange_ExpandsFirst(t *testing.T) {
	rs := []rune("раз два три")
	got, moved := a11yMoveRange(rs, a11yRange{1, 2}, a11yUnitWord, 1)
	if got != (a11yRange{4, 8}) || moved != 1 {
		t.Errorf("%+v moved=%d", got, moved)
	}
	// Нулевой сдвиг — только расширение.
	got, moved = a11yMoveRange(rs, a11yRange{5, 6}, a11yUnitWord, 0)
	if got != (a11yRange{4, 8}) || moved != 0 {
		t.Errorf("Move(0): %+v moved=%d", got, moved)
	}
}

// Пустой диапазон (каретка) остаётся пустым и вправе дойти до самого конца.
func TestA11yMoveRange_Degenerate(t *testing.T) {
	rs := []rune("раз два")
	got, moved := a11yMoveRange(rs, a11yRange{0, 0}, a11yUnitCharacter, 3)
	if got != (a11yRange{3, 3}) || moved != 3 {
		t.Errorf("%+v moved=%d", got, moved)
	}
	got, moved = a11yMoveRange(rs, a11yRange{0, 0}, a11yUnitCharacter, 100)
	if got != (a11yRange{7, 7}) || moved != 7 {
		t.Errorf("до конца: %+v moved=%d", got, moved)
	}
	got, moved = a11yMoveRange(rs, a11yRange{7, 7}, a11yUnitCharacter, -2)
	if got != (a11yRange{5, 5}) || moved != -2 {
		t.Errorf("назад: %+v moved=%d", got, moved)
	}
}

// Document не сдвигается: он один.
func TestA11yMoveRange_Document(t *testing.T) {
	rs := []rune("раз два")
	got, moved := a11yMoveRange(rs, a11yRange{0, 7}, a11yUnitDocument, 1)
	if got != (a11yRange{0, 7}) || moved != 0 {
		t.Errorf("%+v moved=%d", got, moved)
	}
}

func TestA11yMoveRange_EmptyText(t *testing.T) {
	got, moved := a11yMoveRange(nil, a11yRange{0, 0}, a11yUnitWord, 3)
	if got != (a11yRange{0, 0}) || moved != 0 {
		t.Errorf("%+v moved=%d", got, moved)
	}
}

func TestA11yMoveEndpoint(t *testing.T) {
	rs := []rune("раз два три")
	// Конец вперёд на слово.
	got, moved := a11yMoveEndpoint(rs, a11yRange{0, 4}, a11yEndEnd, a11yUnitWord, 1)
	if got != (a11yRange{0, 8}) || moved != 1 {
		t.Errorf("конец вперёд: %+v moved=%d", got, moved)
	}
	// Начало назад на слово.
	got, moved = a11yMoveEndpoint(rs, a11yRange{4, 8}, a11yEndStart, a11yUnitWord, -1)
	if got != (a11yRange{0, 8}) || moved != -1 {
		t.Errorf("начало назад: %+v moved=%d", got, moved)
	}
	// Упор в конец текста.
	got, moved = a11yMoveEndpoint(rs, a11yRange{0, 4}, a11yEndEnd, a11yUnitWord, 9)
	if got != (a11yRange{0, 11}) || moved != 2 {
		t.Errorf("упор в конец: %+v moved=%d", got, moved)
	}
}

// Конец, перешагнувший второй, тянет его за собой — диапазон схлопывается в
// точку, а не становится «отрицательным».
func TestA11yMoveEndpoint_Crossing(t *testing.T) {
	rs := []rune("раз два три")
	got, moved := a11yMoveEndpoint(rs, a11yRange{4, 8}, a11yEndStart, a11yUnitWord, 2)
	if moved != 2 || !got.Empty() || got.Start != 11 {
		t.Errorf("начало за конец: %+v moved=%d", got, moved)
	}
	got, moved = a11yMoveEndpoint(rs, a11yRange{4, 8}, a11yEndEnd, a11yUnitWord, -2)
	if moved != -2 || !got.Empty() || got.Start != 0 {
		t.Errorf("конец за начало: %+v moved=%d", got, moved)
	}
}

func TestA11yMoveEndpointToRange(t *testing.T) {
	r := a11yRange{2, 6}
	o := a11yRange{8, 10}
	if got := a11yMoveEndpointToRange(r, a11yEndEnd, o, a11yEndEnd); got != (a11yRange{2, 10}) {
		t.Errorf("конец на конец: %+v", got)
	}
	if got := a11yMoveEndpointToRange(r, a11yEndStart, o, a11yEndStart); got != (a11yRange{8, 8}) {
		t.Errorf("начало за конец должно схлопнуть диапазон: %+v", got)
	}
	if got := a11yMoveEndpointToRange(r, a11yEndEnd, o, a11yEndStart); got != (a11yRange{2, 8}) {
		t.Errorf("конец на начало другого: %+v", got)
	}
}

func TestA11yCompareEndpoints(t *testing.T) {
	a, b := a11yRange{2, 6}, a11yRange{4, 6}
	cases := []struct {
		ep, oep a11yEndpoint
		want    int
	}{
		{a11yEndStart, a11yEndStart, -1},
		{a11yEndEnd, a11yEndEnd, 0},
		{a11yEndEnd, a11yEndStart, 1},
		{a11yEndStart, a11yEndEnd, -1},
	}
	for _, c := range cases {
		if got := a11yCompareEndpoints(a, c.ep, b, c.oep); got != c.want {
			t.Errorf("ep=%d oep=%d: %d, ждал %d", c.ep, c.oep, got, c.want)
		}
	}
}

func TestA11yRangeText(t *testing.T) {
	rs := []rune("привет мир")
	if got := a11yRangeText(rs, a11yRange{0, 6}, -1); got != "привет" {
		t.Errorf("%q", got)
	}
	if got := a11yRangeText(rs, a11yRange{0, 6}, 3); got != "при" {
		t.Errorf("усечение: %q", got)
	}
	if got := a11yRangeText(rs, a11yRange{0, 6}, 0); got != "" {
		t.Errorf("maxLen=0: %q", got)
	}
	// Диапазон переживает правку текста: позиции за концом усекаются.
	if got := a11yRangeText(rs, a11yRange{7, 500}, -1); got != "мир" {
		t.Errorf("усечение до длины текста: %q", got)
	}
	if got := a11yRangeText(rs, a11yRange{50, 60}, -1); got != "" {
		t.Errorf("диапазон целиком за концом: %q", got)
	}
}

func TestA11yClampRange(t *testing.T) {
	cases := []struct{ in, want a11yRange }{
		{a11yRange{-3, 4}, a11yRange{0, 4}},
		{a11yRange{2, 99}, a11yRange{2, 5}},
		{a11yRange{9, 12}, a11yRange{5, 5}},
		{a11yRange{4, 1}, a11yRange{4, 4}}, // перевёрнутый диапазон не бывает
	}
	for _, c := range cases {
		if got := a11yClampRange(c.in, 5); got != c.want {
			t.Errorf("%+v → %+v, ждал %+v", c.in, got, c.want)
		}
	}
}

func TestA11yFindText(t *testing.T) {
	rs := []rune("Раз два раз ДВА три")
	all := a11yRange{0, len(rs)}

	if got, ok := a11yFindText(rs, all, "два", false, false); !ok || got != (a11yRange{4, 7}) {
		t.Errorf("вперёд: %+v ok=%v", got, ok)
	}
	// Вхождение с другим регистром без ignoreCase не находится.
	if got, ok := a11yFindText(rs, all, "раз", false, false); !ok || got != (a11yRange{8, 11}) {
		t.Errorf("с учётом регистра: %+v ok=%v", got, ok)
	}
	if got, ok := a11yFindText(rs, all, "раз", false, true); !ok || got != (a11yRange{0, 3}) {
		t.Errorf("без учёта регистра, вперёд: %+v ok=%v", got, ok)
	}
	if got, ok := a11yFindText(rs, all, "два", true, true); !ok || got != (a11yRange{12, 15}) {
		t.Errorf("без учёта регистра, назад: %+v ok=%v", got, ok)
	}
	// Поиск ограничен диапазоном.
	if _, ok := a11yFindText(rs, a11yRange{0, 6}, "три", false, false); ok {
		t.Error("найдено за пределами диапазона")
	}
	if got, ok := a11yFindText(rs, a11yRange{5, 15}, "раз", false, true); !ok || got != (a11yRange{8, 11}) {
		t.Errorf("в середине диапазона: %+v ok=%v", got, ok)
	}
	// Вхождение, торчащее за правый край диапазона, не считается.
	if _, ok := a11yFindText(rs, a11yRange{0, 2}, "раз", false, true); ok {
		t.Error("вхождение вылезло за диапазон")
	}
	if _, ok := a11yFindText(rs, all, "", false, false); ok {
		t.Error("пустая строка не должна находиться")
	}
	if _, ok := a11yFindText(rs, all, "нету", false, false); ok {
		t.Error("найдено несуществующее")
	}
}

func TestA11yTextDiff(t *testing.T) {
	base := a11yTextState{Text: "раз", Caret: 3, SelFrom: 3, SelTo: 3}
	if tc, sc := a11yTextDiff(base, base); tc || sc {
		t.Error("одинаковые состояния признаны разными")
	}
	moved := base
	moved.Caret, moved.SelFrom, moved.SelTo = 1, 1, 1
	if tc, sc := a11yTextDiff(base, moved); tc || !sc {
		t.Errorf("сдвиг каретки: text=%v sel=%v", tc, sc)
	}
	sel := base
	sel.SelFrom = 0
	if tc, sc := a11yTextDiff(base, sel); tc || !sc {
		t.Errorf("выделение: text=%v sel=%v", tc, sc)
	}
	typed := base
	typed.Text = "раза"
	if tc, sc := a11yTextDiff(base, typed); !tc || sc {
		t.Errorf("правка текста: text=%v sel=%v", tc, sc)
	}
}

func TestA11yUnitValid(t *testing.T) {
	for _, u := range allUnits {
		if !u.valid() {
			t.Errorf("единица %d объявлена недопустимой", u)
		}
	}
	if a11yTextUnit(-1).valid() || a11yTextUnit(7).valid() {
		t.Error("чужое значение TextUnit принято")
	}
	if !a11yEndStart.valid() || !a11yEndEnd.valid() || a11yEndpoint(2).valid() {
		t.Error("проверка концов диапазона неверна")
	}
}
