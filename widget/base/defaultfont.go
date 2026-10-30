// defaultfont.go — кегль по умолчанию, который может следовать теме.
//
// DefaultFontSizePt — константа (10 pt) и ею остаётся: на неё опираются
// потребители и тесты, в том числе в константных выражениях. Но виджеты,
// которым кегль не задан (заголовки окон, вкладки, пункты меню, подписи),
// берут его не из константы, а отсюда. Пока никто ничего не менял, значение то
// же — 10 pt, и ни раскладка, ни пиксели не отличаются.
//
// Менять кегль умеет тема: профиль с флагом theme.FlagFontDefaultGlobal просит,
// чтобы Fonts["default"] стал размером по умолчанию для виджетов вне
// компонентов оболочки (оболочка берёт шрифт из своих стилей и от этого не
// зависит). Применяет его ApplyGlobalTheme, то есть Engine.SetTheme,
// Engine.SetThemeProfile и Engine.ApplyThemeProfile; смена на тему без флага
// возвращает прежние 10 pt.
package base

import (
	"sync/atomic"

	hgdraw "github.com/oops1/headless-gui/v4/core/draw"
)

// defaultFontFromTheme — размер поставила тема, а не приложение. Только такой
// размер тема вправе снять при смене на профиль без флага: явный
// SetDefaultFontSize приложения ей не принадлежит. Сам размер хранит core/draw
// (его читает и холст движка).
var defaultFontFromTheme atomic.Bool

// DefaultFontSize возвращает размер шрифта (pt) для виджетов, которым свой не
// задан: DefaultFontSizePt, пока приложение или тема не назначили другой.
func DefaultFontSize() float64 { return hgdraw.DefaultFontSize() }

// SetDefaultFontSize назначает размер шрифта по умолчанию (pt). Ноль и
// отрицательное значения возвращают DefaultFontSizePt.
//
// Размер влияет на раскладку (ширины, высоты строк), поэтому ставить его надо
// до построения интерфейса; на готовом интерфейсе движок перерисует кадр, но
// размеры, уже посчитанные виджетами, останутся прежними до следующей
// раскладки. Явный вызов главнее темы: пока он не отменён нулём, смена темы
// размера не трогает.
func SetDefaultFontSize(pt float64) {
	defaultFontFromTheme.Store(false)
	storeDefaultFont(pt)
}

func storeDefaultFont(pt float64) { hgdraw.StoreDefaultFontSize(pt) }

// applyThemeFontSize — размер, о котором просит тема (ThemeStyle.DefaultFontSize).
//
// Положительный ставится и запоминается как «поставлен темой»; ноль снимает
// ТОЛЬКО то, что поставила сама тема, — иначе переключение пресетов стирало бы
// размер, назначенный приложением.
func applyThemeFontSize(pt float64) {
	if pt > 0 {
		if !defaultFontFromTheme.Load() && hgdraw.DefaultFontSizeSet() {
			return // явный выбор приложения главнее темы
		}
		storeDefaultFont(pt)
		defaultFontFromTheme.Store(true)
		return
	}
	if defaultFontFromTheme.Swap(false) {
		hgdraw.StoreDefaultFontSize(0)
	}
}
