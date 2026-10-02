package widget

// mnemonic.go — подчёркнутая буква в подписи меню («_Файл» → Ф с чертой).
//
// Мнемоника — второй способ дойти до пункта меню, кроме мыши: Alt открывает
// строку меню, подчёркнутая буква выбирает пункт. Меню движка этого не умело
// вовсе, и клавиатурой до пункта можно было добраться только стрелками.
//
// Разметка та же, что в WPF и Win32: подчёркивание перед буквой помечает её
// мнемоникой, двойное подчёркивание означает сам знак подчёркивания. Разбор
// включается флагом UseMnemonics — у приложения, которое пишет в подписях
// обычные подчёркивания (имена файлов в контекстном меню), ничего не меняется.

import (
	"image/color"
	"strings"
	"unicode"
)

// splitMnemonic разбирает подпись с мнемоникой.
//
// Возвращает подпись без служебных подчёркиваний, букву мнемоники в нижнем
// регистре (0 — её нет) и её смещение в БАЙТАХ внутри подписи: по нему
// меряется ширина текста слева, чтобы поставить черту ровно под буквой.
//
// Помечена первая встреченная мнемоника: вторая в той же подписи — почти
// наверняка опечатка, и подчёркивать обе значило бы показать человеку две
// клавиши там, где работает одна.
func splitMnemonic(s string) (label string, key rune, pos int) {
	if !strings.ContainsRune(s, '_') {
		return s, 0, -1
	}
	var b strings.Builder
	b.Grow(len(s))
	pos = -1
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '_' {
			b.WriteRune(rs[i])
			continue
		}
		if i+1 >= len(rs) {
			// Подчёркивание в конце подписи помечать нечего — оставляем как есть.
			b.WriteRune('_')
			continue
		}
		next := rs[i+1]
		if next == '_' {
			b.WriteRune('_') // «__» — сам знак подчёркивания
			i++
			continue
		}
		if key == 0 && next != ' ' {
			key = unicode.ToLower(next)
			pos = b.Len()
		}
		b.WriteRune(next)
		i++
	}
	return b.String(), key, pos
}

// mnemonicLabel — подпись без служебных подчёркиваний. Нужна там, где текст
// только меряют или рисуют: в расчёте ширины меню и в отрисовке.
func mnemonicLabel(s string, on bool) string {
	if !on {
		return s
	}
	label, _, _ := splitMnemonic(s)
	return label
}

// drawMnemonicText рисует подпись и черту под буквой мнемоники.
//
// Черта — там же, где её ставит Label: шрифт один и тот же, и подчёркивание
// в меню не должно выглядеть иначе, чем в остальном интерфейсе.
func drawMnemonicText(ctx DrawContext, text string, x, y int, col color.RGBA, on bool) {
	if !on {
		ctx.DrawText(text, x, y, col)
		return
	}
	label, key, pos := splitMnemonic(text)
	ctx.DrawText(label, x, y, col)
	if key == 0 || pos < 0 {
		return
	}
	lead := MeasureUIText(label[:pos], DefaultFontSizePt)
	w := MeasureUIText(string([]rune(label[pos:])[0]), DefaultFontSizePt)
	if w <= 0 {
		return
	}
	ctx.DrawHLine(x+lead, y+int(DefaultFontSizePt*1.35+0.5), w, col)
}

// matchesMnemonic — нажата ли клавиша этой мнемоники.
//
// Сопоставление идёт тремя способами подряд, от точного к догадке:
//
//  1. По символу события, если бэкенд его прислал.
//  2. По коду клавиши: коды движка — латинские буквы и цифры по физической
//     клавише, независимо от раскладки (так устроено, чтобы Ctrl+S работал
//     и в русской раскладке).
//  3. По русской букве на той же физической клавише. Кириллическую мнемонику
//     иначе не поймать: до приложения доходит код латинской буквы, а какая
//     раскладка включена, движок не знает. Раскладка здесь одна — ЙЦУКЕН;
//     других движок не знает, и угадывать их не берётся.
func matchesMnemonic(key rune, e KeyEvent) bool {
	if key == 0 {
		return false
	}
	if e.Rune != 0 && unicode.ToLower(e.Rune) == key {
		return true
	}
	latin, ok := latinFromKeyCode(e.Code)
	if !ok {
		return false
	}
	if latin == key {
		return true
	}
	return cyrillicOnKey(latin) == key
}

// latinFromKeyCode — латинская буква или цифра физической клавиши.
func latinFromKeyCode(c KeyCode) (rune, bool) {
	switch {
	case c >= KeyA && c <= KeyZ:
		return rune('a' + (c - KeyA)), true
	case c >= Key0 && c <= Key9:
		return rune('0' + (c - Key0)), true
	}
	return 0, false
}

// qwertyToJCUKEN — какая русская буква стоит на клавише с латинской буквой.
// Раскладка ЙЦУКЕН, стандартная русская: ряды сверху вниз.
const qwertyRow = "qwertyuiop[]asdfghjkl;'zxcvbnm,./"
const jcukenRow = "йцукенгшщзхъфывапролджэячсмитьбю."

// cyrillicOnKey возвращает русскую букву на клавише с данной латинской, или 0.
func cyrillicOnKey(latin rune) rune {
	i := strings.IndexRune(qwertyRow, latin)
	if i < 0 {
		return 0
	}
	return []rune(jcukenRow)[i]
}
