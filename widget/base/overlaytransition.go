package base

// overlaytransition.go — что движку нужно знать об оверлее, чтобы
// анимировать его появление и исчезновение (engine.SetOverlayTransitions,
// задача 3.3 плана 4.0).
//
// Переход ведёт движок: пиксели оверлея он снимает сам, по OverlayBounds, и
// DrawOverlay на шагах перехода не зовёт. Оверлею ничего реализовывать не
// нужно; необязательные интерфейсы ниже уточняют поведение.

import "image"

// SelfAnimatedOverlay — оверлей, который сам анимирует своё появление и
// исчезновение (desktop.Flyout): переход движка к нему не применяется, иначе
// движение было бы двойным.
type SelfAnimatedOverlay interface {
	AnimatesOverlay() bool
}

// OverlayMarginer — на сколько логических точек рисунок оверлея (тень)
// выходит за OverlayBounds. Без него движок берёт DefaultOverlayMargin.
type OverlayMarginer interface {
	OverlayMargin() int
}

// OverlayAnchorer — точка, к которой привязан оверлей (в кадре владельца):
// оверлей выше неё раскрылся вверх, и переход появления ведёт его снизу.
// Без него движок сравнивает оверлей с границами владельца.
type OverlayAnchorer interface {
	OverlayAnchor() image.Point
}

// defaultOverlayMargin — запас, когда тема не объявила тень меню: покрывает
// плоскую тень со сдвигом и мягкую тень высоты до восьми.
const defaultOverlayMargin = 24

// DefaultOverlayMargin — запас под тень меню текущей темы (theme.ShadowSpec
// стиля "menu"), а без неё — запас под тени по умолчанию.
func DefaultOverlayMargin() int {
	if sp, ok := MenuShadow(); ok {
		if e := sp.Extent(); e > defaultOverlayMargin {
			return e
		}
	}
	return defaultOverlayMargin
}
