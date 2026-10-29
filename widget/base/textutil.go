package base

// textutil.go — помощники ввода текста, общие для полей widget/text и
// RichText: премультиплицированный цвет выделения, фаза мигания каретки,
// символы слова.

import "image/color"

// PremulAlpha возвращает корректный premultiplied-цвет: c с прозрачностью a.
// color.RGBA в Go — альфа-премультиплицированный; каналы, превышающие альфу,
// при Over-блендинге переполняются (артефакт «мадженты» на светлом фоне).
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func PremulAlpha(c color.RGBA, a uint8) color.RGBA {
	m := uint32(a)
	return color.RGBA{
		R: uint8(uint32(c.R) * m / 255),
		G: uint8(uint32(c.G) * m / 255),
		B: uint8(uint32(c.B) * m / 255),
		A: a,
	}
}

// CaretBlinkHalfPeriodMs — полупериод мигания каретки (мс, стиль Windows).
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
const CaretBlinkHalfPeriodMs = 530

// CaretPhaseAt возвращает фазу мигания каретки (true — каретка видима)
// для момента времени ms (Unix-миллисекунды).
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func CaretPhaseAt(ms int64) bool { return (ms/CaretBlinkHalfPeriodMs)%2 == 0 }

// IsWordRune — true для букв/цифр/подчёркивания (символы одного «слова»).
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func IsWordRune(r rune) bool {
	if r == '_' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
		return true
	}
	// Не-ASCII (кириллица и т.п.) считаем частью слова, пробелы — нет.
	return r > 127 && r != ' '
}

// ClampInt — служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func ClampInt(v, lo, hi int) int {
	if v > hi {
		v = hi
	}
	if v < lo {
		v = lo
	}
	return v
}
