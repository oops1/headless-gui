package theme

import (
	"image/color"
	"math"
)

// Токены акцента.
//
// Акцент — единственный цвет темы, который пользователь меняет сам (Windows
// «Параметры → Персонализация → Цвета»), причём на лету. Поэтому он живёт не
// в готовых цветах стилей, а в плоском токене, на который стили ссылаются
// (StyleDelta.FillFrom и соседи): смена акцента — это пересборка темы с
// другим значением токена, а не обход всех стилей.
//
// От базового акцента считаются производные. Профиль может объявить их сам
// (SetColor("accent.hover", …)), и тогда его значение побеждает; но после
// Manager.SetAccent производные считаются заново ВСЕГДА — иначе у нового
// акцента осталось бы наведение от старого.
const (
	// KeyAccent — базовый цвет акцента.
	KeyAccent Key = "accent"
	// KeyAccentHover — акцент под курсором (чуть светлее).
	KeyAccentHover Key = "accent.hover"
	// KeyAccentPressed — акцент нажатого элемента (темнее).
	KeyAccentPressed Key = "accent.pressed"
	// KeyAccentDark — тёмный оттенок: рамки и подчёркивания на светлом.
	KeyAccentDark Key = "accent.dark"
	// KeyAccentLight — светлый оттенок: ссылки и акценты на тёмном.
	KeyAccentLight Key = "accent.light"
	// KeyAccentText — цвет текста на акцентной заливке: белый или чёрный,
	// смотря что читается лучше. Светло-жёлтый акцент с белым текстом —
	// частая ошибка, которую токен снимает.
	KeyAccentText Key = "accent.text"
	// KeySelection — цвет выделения; по умолчанию равен акценту.
	KeySelection Key = "selection"
)

// AccentSet — акцент вместе с производными.
type AccentSet struct {
	Base, Hover, Pressed, Dark, Light, Text color.RGBA
}

// DeriveAccent считает производные от базового цвета.
//
// Прозрачность базы отбрасывается: акцент всегда непрозрачен, а цвет с альфой
// (premultiplied) пришлось бы сначала восстанавливать. Правила простые и
// одинаковы для любого цвета: наведение — подмешать 12 % белого, нажатие —
// 15 % чёрного, тёмный и светлый оттенки — 25 % чёрного и 30 % белого. Текст
// на акценте — белый, пока цвет не станет заметно светлым (относительная
// яркость больше 0,35): акценты Windows, включая синий по умолчанию, остаются
// с белым текстом.
func DeriveAccent(base color.RGBA) AccentSet {
	base.A = 255
	white, black := RGB(255, 255, 255), RGB(0, 0, 0)
	text := white
	if relativeLuminance(base) > 0.35 {
		text = black
	}
	return AccentSet{
		Base:    base,
		Hover:   mixOpaque(base, white, 0.12),
		Pressed: mixOpaque(base, black, 0.15),
		Dark:    mixOpaque(base, black, 0.25),
		Light:   mixOpaque(base, white, 0.30),
		Text:    text,
	}
}

// tokens раскладывает набор по ключам токенов.
func (a AccentSet) tokens() map[Key]color.RGBA {
	return map[Key]color.RGBA{
		KeyAccent:        a.Base,
		KeyAccentHover:   a.Hover,
		KeyAccentPressed: a.Pressed,
		KeyAccentDark:    a.Dark,
		KeyAccentLight:   a.Light,
		KeyAccentText:    a.Text,
	}
}

// mixOpaque смешивает два непрозрачных цвета: t — доля to.
func mixOpaque(from, to color.RGBA, t float64) color.RGBA {
	m := func(a, b uint8) uint8 {
		return uint8(float64(a)*(1-t) + float64(b)*t + 0.5)
	}
	return color.RGBA{R: m(from.R, to.R), G: m(from.G, to.G), B: m(from.B, to.B), A: 255}
}

// relativeLuminance — относительная яркость sRGB по WCAG (0 — чёрный, 1 —
// белый).
func relativeLuminance(c color.RGBA) float64 {
	lin := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}
