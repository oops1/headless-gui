package tests

import (
	"testing"
)

// GG-63: итог и верхние панели прокручиваются вместе, по блокам. Отдельно
// прокручивать их было путано: пользователь листал итог к конфликту, а наверху
// оставалось другое место файла.

// Колесо над одной частью двигает и вторую — в обе стороны.
func TestMergeView_SyncScrollWheel(t *testing.T) {
	mv := mergeLongView(t)
	if !mv.SyncScroll() {
		t.Fatal("синхронная прокрутка по умолчанию выключена")
	}

	// Верхние панели: x в «нашей» панели, y посреди её строк.
	mv.OnMouseWheelPixels(200, 200, 0, -300)
	if mv.Scroll() <= 0 {
		t.Fatalf("колесо над верхом не прокрутило его: %v", mv.Scroll())
	}
	if mv.ResultScroll() <= 0 {
		t.Fatal("итог не пошёл за верхними панелями")
	}

	// Итог: y внизу контрола, в панели итога.
	mv.SetScroll(0)
	if mv.ResultScroll() != 0 {
		t.Fatalf("верх в начале, а итог прокручен на %v", mv.ResultScroll())
	}
	mv.OnMouseWheelPixels(200, 520, 0, -300)
	if mv.ResultScroll() <= 0 || mv.Scroll() <= 0 {
		t.Fatalf("колесо над итогом: итог %v, верх %v — верх должен пойти следом",
			mv.ResultScroll(), mv.Scroll())
	}
}

// Края совпадают: верх в начале — итог в начале, верх в конце — итог в конце,
// хотя итог длиннее на строки маркеров.
func TestMergeView_SyncScrollEnds(t *testing.T) {
	mv := mergeLongView(t)

	mv.SetSyncScroll(false)
	mv.SetResultScroll(1e9)
	resultMax := mv.ResultScroll()
	mv.SetResultScroll(0)
	mv.SetSyncScroll(true)

	mv.SetScroll(1e9)
	if got := mv.ResultScroll(); got != resultMax {
		t.Fatalf("верх в конце, итог на %v, а его конец — %v", got, resultMax)
	}
	mv.SetScroll(0)
	if got := mv.ResultScroll(); got != 0 {
		t.Fatalf("верх в начале, итог на %v", got)
	}
}

// Посреди файла напротив друг друга стоят одни и те же строки: между первым и
// вторым конфликтом верхняя строка итога сдвинута ровно на лишние строки
// маркеров первого конфликта (у нерешённого их на четыре больше, чем строк у
// сторон), с допуском на то, что опорная точка у частей разной высоты.
func TestMergeView_SyncScrollAlignsChunks(t *testing.T) {
	mv := mergeLongView(t)
	const lineH = 22

	mv.SetScroll(600) // верх окна около строки 27 — между конфликтами 10 и 60
	topLine := (mv.Scroll() - 12) / lineH
	resLine := (mv.ResultScroll() - 12) / lineH
	if d := resLine - (topLine + 4); d < -4 || d > 4 {
		t.Fatalf("напротив строки %.1f верха итог показывает строку %.1f, ждал около %.1f",
			topLine, resLine, topLine+4)
	}
}

// Выключенная синхронизация — части порознь; включение ставит итог вровень.
func TestMergeView_SyncScrollOff(t *testing.T) {
	mv := mergeLongView(t)
	mv.SetSyncScroll(false)
	mv.SetScroll(900)
	if got := mv.ResultScroll(); got != 0 {
		t.Fatalf("без синхронизации итог пошёл за верхом: %v", got)
	}
	mv.SetSyncScroll(true)
	if mv.ResultScroll() <= 0 {
		t.Fatal("включение синхронизации не поставило итог вровень с верхом")
	}
}

// Правка итога его прокрутку не дёргает: ведущей становится сама правка.
func TestMergeView_SyncScrollStableWhileTyping(t *testing.T) {
	mv := mergeLongView(t)
	mv.SetResultScroll(800)
	before := mv.ResultScroll()
	// Каретка в видимой строке итога, ниже первого конфликта.
	line := int(before/22) + 3
	mv.SetCaret(line, 0)
	after := mv.ResultScroll()
	for i := 0; i < 3; i++ {
		mv.InsertText("x\n")
	}
	if got := mv.ResultScroll(); got != after {
		t.Fatalf("итог сдвинулся под пишущим: было %v, стало %v", after, got)
	}
}
