package window

import "testing"

// Скринридер не читает документ целиком: он просит символ, слово или строку
// вокруг каретки и следом соседние. Движок отдавал ему только одну строку —
// «значение» элемента, как у ползунка, — и редактор звучал как поле ввода,
// объявляющее весь текст заново на каждое нажатие.

const a11ySample = "первая строка\nвторая строка\nтретья"

func TestA11yTextSlice_Line(t *testing.T) {
	// Смещение внутри второй строки.
	s, from, to := a11yTextSlice(a11ySample, 20, a11yGranLine)
	if s != "вторая строка" {
		t.Errorf("строка %q", s)
	}
	if from != 14 || to != 27 {
		t.Errorf("границы %d..%d, ждал 14..27", from, to)
	}
	// Перенос в кусок не входит: прочитав его, скринридер объявил бы лишнюю
	// пустую строку.
	if len(s) > 0 && s[len(s)-1] == '\n' {
		t.Error("в строку попал перенос")
	}
}

func TestA11yTextSlice_Word(t *testing.T) {
	s, from, to := a11yTextSlice(a11ySample, 2, a11yGranWord)
	if s != "первая" || from != 0 || to != 6 {
		t.Errorf("слово %q (%d..%d), ждал «первая» 0..6", s, from, to)
	}
	// Пробел между словами — тоже кусок: иначе шаг по словам терял бы
	// смещения и следующий начинался не там.
	s, from, to = a11yTextSlice(a11ySample, 6, a11yGranWord)
	if s != " " || from != 6 || to != 7 {
		t.Errorf("промежуток %q (%d..%d)", s, from, to)
	}
}

func TestA11yTextSlice_Char(t *testing.T) {
	s, from, to := a11yTextSlice(a11ySample, 0, a11yGranChar)
	if s != "п" || from != 0 || to != 1 {
		t.Errorf("символ %q (%d..%d)", s, from, to)
	}
}

// Смещение за концом текста — не ошибка: там стоит каретка после последнего
// символа, и скринридер спрашивает и о нём.
func TestA11yTextSlice_PastEnd(t *testing.T) {
	n := len([]rune(a11ySample))
	s, from, to := a11yTextSlice(a11ySample, n+5, a11yGranChar)
	if s != "" || from != n || to != n {
		t.Errorf("за концом вернулось %q (%d..%d), ждал пустое в %d", s, from, to, n)
	}
	if s, _, _ := a11yTextSlice("", 0, a11yGranLine); s != "" {
		t.Errorf("в пустом тексте нашлась строка %q", s)
	}
}

// Старый вид запроса (вид границы) переводится в зернистость: им до сих пор
// пользуется libatspi.
func TestA11yTextBoundaryToGran(t *testing.T) {
	cases := map[uint32]a11yGranularity{
		0: a11yGranChar,
		1: a11yGranWord,
		2: a11yGranWord,
		5: a11yGranLine,
		6: a11yGranLine,
	}
	for boundary, want := range cases {
		if got := a11yTextBoundaryToGran(boundary); got != want {
			t.Errorf("граница %d → %d, ждал %d", boundary, got, want)
		}
	}
}

func TestA11yLineAt(t *testing.T) {
	line, start := a11yLineAt(a11ySample, 20)
	if line != 1 || start != 14 {
		t.Errorf("строка %d начинается с %d, ждал 1 и 14", line, start)
	}
	if line, _ := a11yLineAt(a11ySample, 0); line != 0 {
		t.Errorf("в начале текста строка %d", line)
	}
}
