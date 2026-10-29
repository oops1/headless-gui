package core

// popup_fade.go — прозрачность окна-попапа на переходах появления и
// исчезновения (engine.PopupTransition, задача 3.4 плана 4.0).
//
// Окно ОС умеет её двумя способами, и какой есть — узнаётся после создания:
//   - прозрачность окна целиком (popupAlphaSetter): Win32 — слоистое окно
//     WS_EX_LAYERED с SetLayeredWindowAttributes. Картинка остаётся прежней,
//     шаг перехода — один вызов без перекладки пикселей;
//   - альфа точек, которую смешивает композитор (popupPixelAlpha): X11 с
//     32-битным visual'ом и композитным менеджером. Шаг — копия кадра,
//     умноженная на непрозрачность (кадр premultiplied, множитель — на все
//     четыре канала).
//
// Без обоих (X11 без композитора: форма окна — только сплошные точки, и
// полупрозрачное окно было бы невидимым) переход вырождается: окно
// показывается сразу, исчезающее закрывается сразу.

import "image"

// popupFadeKind — как окно-попап умеет прозрачность.
type popupFadeKind uint8

const (
	popupFadeNone   popupFadeKind = iota // не умеет: без переходов
	popupFadeWindow                      // прозрачность окна целиком
	popupFadeCopy                        // альфа точек: копия кадра с множителем
)

// popupAlphaSetter — окно ОС умеет прозрачность целиком. Можно звать до
// CreatePopup: окно родится уже с ней.
type popupAlphaSetter interface {
	SetPopupAlpha(a float64)
}

// popupPixelAlpha — окно ОС показывает альфу точек кадра смешанной с тем,
// что под ним (решается при создании окна).
type popupPixelAlpha interface {
	PopupPixelAlpha() bool
}

// popupFadeOf — как созданное окно умеет прозрачность.
func popupFadeOf(native NativeWindow) popupFadeKind {
	if _, ok := native.(popupAlphaSetter); ok {
		return popupFadeWindow
	}
	if pa, ok := native.(popupPixelAlpha); ok && pa.PopupPixelAlpha() {
		return popupFadeCopy
	}
	return popupFadeNone
}

// fadedCopy — копия кадра src, умноженная на a, для окна с альфой точек.
// Копия на каждый шаг новая: окно (X11) держит последнюю выведенную для
// Expose, а блит идёт без замка хоста — переписывать на месте нельзя.
// Шагов на переход — десяток, и только у таких окон.
func fadedCopy(src *image.RGBA, a float64) *image.RGBA {
	dst := image.NewRGBA(src.Rect)
	fadeRGBA(dst, src, a)
	return dst
}

// fadeRGBA кладёт в dst кадр src (premultiplied), умноженный на a.
func fadeRGBA(dst, src *image.RGBA, a float64) {
	k := uint32(a*256 + 0.5)
	if a <= 0 {
		k = 0
	}
	if k > 256 {
		k = 256
	}
	b := src.Rect
	row := b.Dx() * 4
	for y := b.Min.Y; y < b.Max.Y; y++ {
		s := src.Pix[src.PixOffset(b.Min.X, y):][:row]
		d := dst.Pix[dst.PixOffset(b.Min.X, y):][:row]
		for i, v := range s {
			d[i] = byte(uint32(v) * k >> 8)
		}
	}
}
