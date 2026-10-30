package text

import (
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
)

func secTypeRunes(t *TextInput, s string) {
	for _, r := range s {
		t.OnKeyEvent(base.KeyEvent{Rune: r, Pressed: true})
	}
}

func allZero(rs []rune) bool {
	for _, r := range rs {
		if r != 0 {
			return false
		}
	}
	return true
}

// Ввод пароля не оставляет в памяти брошенных массивов с его началом: каждое
// нажатие заводит новый массив, и старый затирается (замечание WinLine после
// v3.38.0 — окно контроля учётных записей).
func TestPassword_TypingWipesOldBuffers(t *testing.T) {
	p := NewPasswordInput("")
	secTypeRunes(p, "sec")
	old := p.runes[:cap(p.runes)]
	secTypeRunes(p, "r")
	if &old[0] == &p.runes[:1][0] {
		t.Skip("буфер не перевыделен — проверять нечего")
	}
	if !allZero(old) {
		t.Fatalf("брошенный массив не затёрт: %q", string(old))
	}
	if string(p.runes) != "secr" {
		t.Fatalf("поле содержит %q", string(p.runes))
	}
}

// Удаление затирает освободившийся хвост массива.
func TestPassword_DeleteWipesTail(t *testing.T) {
	p := NewPasswordInput("")
	secTypeRunes(p, "abcd")
	p.OnKeyEvent(base.KeyEvent{Code: base.KeyBackspace, Pressed: true})
	if string(p.runes) != "abc" {
		t.Fatalf("поле содержит %q", string(p.runes))
	}
	if tail := p.runes[len(p.runes):4]; !allZero(tail) {
		t.Fatalf("хвост после удаления не затёрт: %q", string(tail))
	}
}

// С OnSecretChange поле пароля сообщает об изменении числом символов, а
// OnChange со строкой-копией не вызывается.
func TestPassword_SecretChangeWithoutString(t *testing.T) {
	p := NewPasswordInput("")
	var got []int
	strCalls := 0
	p.OnSecretChange = func(n int) { got = append(got, n) }
	p.OnChange = func(string) { strCalls++ }
	secTypeRunes(p, "ab")
	if strCalls != 0 {
		t.Errorf("OnChange со строкой вызван %d раз", strCalls)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("OnSecretChange: %v, ждали [1 2]", got)
	}
	// Обычное поле с тем же колбэком получает строку, как раньше.
	q := NewTextInput("")
	var text string
	q.OnSecretChange = func(int) { t.Error("OnSecretChange у обычного поля") }
	q.OnChange = func(s string) { text = s }
	secTypeRunes(q, "x")
	if text != "x" {
		t.Errorf("OnChange обычного поля: %q", text)
	}
}

// SecretBytes отдаёт копию и не очищает поле; Clear затирает буфер.
func TestPassword_SecretBytesAndClear(t *testing.T) {
	p := NewPasswordInput("")
	secTypeRunes(p, "пароль")
	b := p.SecretBytes()
	if string(b) != "пароль" {
		t.Fatalf("SecretBytes: %q", b)
	}
	b[0] = 0 // это копия: поле не задето
	if string(p.runes) != "пароль" {
		t.Fatalf("SecretBytes изменил поле: %q", string(p.runes))
	}
	buf := p.runes[:cap(p.runes)]
	p.Clear()
	if len(p.runes) != 0 || !allZero(buf) {
		t.Fatalf("Clear не затёр буфер: %q", string(buf))
	}
}
