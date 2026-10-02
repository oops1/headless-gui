//go:build windows

package window

import (
	"testing"
	"unicode/utf16"
)

// Windows шлёт текст в UTF-16: символ вне основной плоскости — эмодзи, редкие
// письменности — приходит ДВУМЯ сообщениями WM_CHAR, старшим и младшим
// суррогатом. По одному они не значат ничего, и раньше в приложение уходила
// пара мусорных рун вместо одного символа.

// feedChars прогоняет коды через обработчик WM_CHAR и возвращает то, что
// дошло до приложения.
func feedChars(w *Win32Window, codes ...rune) []rune {
	var got []rune
	for _, c := range codes {
		if r, ok := w.charFromUTF16(c); ok && r >= 32 {
			got = append(got, r)
		}
	}
	return got
}

func TestWmChar_SurrogatePair(t *testing.T) {
	w := &Win32Window{}
	hi, lo := utf16.EncodeRune('😀')

	got := feedChars(w, hi, lo)
	if len(got) != 1 || got[0] != '😀' {
		t.Errorf("пришло %q, ждал одну руну «😀»", string(got))
	}
}

func TestWmChar_PlainRune(t *testing.T) {
	w := &Win32Window{}
	got := feedChars(w, 'ф', 'x')
	if string(got) != "фx" {
		t.Errorf("пришло %q, ждал «фx»", string(got))
	}
}

// Непарный суррогат — мусор: лучше ничего, чем знак-замена в тексте.
func TestWmChar_LoneSurrogateDropped(t *testing.T) {
	w := &Win32Window{}
	hi, _ := utf16.EncodeRune('😀')

	got := feedChars(w, hi, 'a')
	if len(got) != 0 {
		t.Errorf("пришло %q, ждал пусто: пара не сложилась", string(got))
	}
	// После разбора пары обработчик снова принимает обычные символы.
	got = feedChars(w, 'b')
	if string(got) != "b" {
		t.Errorf("после непарного суррогата ввод сломался: %q", string(got))
	}
}
