// textinput_secret.go — чтение содержимого поля пароля без создания строки.
//
// Единственным способом прочитать введённое был GetText() string. Внутренний
// буфер рун виджет затирает при SetText, но само значение уходит наружу
// НЕИЗМЕНЯЕМОЙ Go-строкой: занулить её нельзя ничем, она живёт в куче до
// сборки мусора и попадает в дампы памяти. Приложению, у которого правило
// «секрет не превращается в string», это правило приходилось нарушать ровно
// на границе с движком.
//
// Здесь секрет отдаётся байтами, которые вызывающий волен занулить, а буфер
// виджета зачищается тем же вызовом.
package base

import (
	"unicode/utf8"
)

// TakeSecret возвращает содержимое поля в UTF-8 и ОЧИЩАЕТ поле.
//
// Строка не создаётся ни разу: руны кодируются прямо в возвращаемый срез.
// Вызывающий отвечает за то, чтобы занулить его после использования
// (clear(b) или подобное) — движок держать этот срез не будет.
//
// «Take», а не «Get»: чтение и очистка — одно действие. Пароль читают один
// раз, чтобы отдать в хранилище или в транспорт, и оставлять его в поле после
// этого незачем; разделять на два вызова значит оставить окно, в котором
// секрет лежит в двух местах.
//
// Работает на любом поле, а не только на созданном NewPasswordInput: режим
// пароля скрывает символы на экране, а к тому, как значение читают, отношения
// не имеет.
//
// Пустое поле даёт пустой, не nil, срез — вызывающему не приходится различать
// «ничего не ввели» и «что-то пошло не так».
func (t *TextInput) TakeSecret() []byte {
	t.mu.Lock()
	out := make([]byte, 0, len(t.runes)*utf8.UTFMax)
	for _, r := range t.runes {
		out = utf8.AppendRune(out, r)
	}
	clear(t.runes)
	t.runes = nil
	t.caretPos = 0
	t.selStart = -1
	t.selEnd = 0
	t.scrollX = 0
	t.wipeHistory()
	t.mu.Unlock()

	t.Invalidate()
	return out
}

// WipeSecret очищает поле, ничего не возвращая.
//
// Нужен на отмене диалога: секрет никому не понадобился, но и оставаться в
// памяти он не должен. SetText("") делает то же самое, но только в режиме
// пароля — а поле могло быть обычным.
func (t *TextInput) WipeSecret() {
	t.mu.Lock()
	empty := len(t.runes) == 0
	clear(t.runes)
	t.runes = nil
	t.caretPos = 0
	t.selStart = -1
	t.selEnd = 0
	t.scrollX = 0
	t.wipeHistory()
	t.mu.Unlock()

	if !empty {
		t.Invalidate()
	}
}

// SecretBytes возвращает содержимое поля в UTF-8 КОПИЕЙ, не очищая поле.
// Строка не создаётся; занулить срез после использования — забота
// вызывающего. Чтобы прочитать и сразу очистить — TakeSecret.
func (t *TextInput) SecretBytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]byte, 0, len(t.runes)*utf8.UTFMax)
	for _, r := range t.runes {
		out = utf8.AppendRune(out, r)
	}
	return out
}

// Clear очищает поле, затирая буфер в памяти, — то же, что WipeSecret.
func (t *TextInput) Clear() { t.WipeSecret() }

// ─── Буфер без брошенных копий (режим пароля) ───────────────────────────────
//
// Буфер рун — единственное место, где пароль лежит в памяти, и его можно
// затереть. Но правка текста раньше оставляла копии: каждое нажатие клавиши
// заводило новый массив, а старый, с уже набранным началом пароля, уходил
// сборщику мусора как есть; удаление сдвигало руны влево и оставляло хвост
// массива нетронутым. В режиме пароля старые массивы и хвосты затираются.

// cutRunes удаляет руны [a, b), затирая освободившийся хвост массива.
// Вызывать под t.mu.
func (t *TextInput) cutRunes(a, b int) {
	old := len(t.runes)
	t.runes = append(t.runes[:a], t.runes[b:]...)
	if t.isPassword {
		clear(t.runes[len(t.runes):old])
	}
}

// adoptRunes заменяет буфер СВЕЖЕВЫДЕЛЕННЫМ срезом ins, затирая прежний
// массив. ins не должен делить память с t.runes. Вызывать под t.mu.
func (t *TextInput) adoptRunes(ins []rune) {
	if t.isPassword {
		clear(t.runes[:cap(t.runes)])
	}
	t.runes = ins
}

// growRunes дописывает n нулевых рун; если массив пришлось перевыделить,
// прежний затирается. Вызывать под t.mu.
func (t *TextInput) growRunes(n int) {
	old := t.runes
	t.runes = append(t.runes, make([]rune, n)...)
	if t.isPassword && cap(old) < len(old)+n {
		clear(old[:cap(old)])
	}
}

// changeNotifyLocked готовит уведомление об изменении текста — вызывать его
// после Unlock. В режиме пароля с OnSecretChange строка не создаётся вовсе:
// уходит только число символов. Вызывать под t.mu; nil — уведомлять некого.
func (t *TextInput) changeNotifyLocked() func() {
	if t.isPassword && t.OnSecretChange != nil {
		n, f := len(t.runes), t.OnSecretChange
		return func() { f(n) }
	}
	if t.OnChange == nil {
		return nil
	}
	text, f := string(t.runes), t.OnChange
	return func() { f(text) }
}
