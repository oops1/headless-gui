package core

// cursorimage.go — курсор из картинки (Engine.SetCursorImage): общая часть
// всех бэкендов.
//
// Движок хранит картинку как widget.CursorImage с ключом-хешем содержимого.
// Окно отдаёт её ОС через необязательный cursorImageSetter, и только при смене
// ключа. Системный курсор из картинки — дорогой ресурс (Win32 — пара
// GDI-объектов, X11 — pixmap, picture и курсор на сервере, Wayland —
// shm-буфер), поэтому каждый бэкенд держит готовые курсоры в кэше по ключу,
// не более pointer.CursorCacheSize штук: сервер удалённого рабочего стола шлёт то один
// указатель, то другой, и пересоздавать их на каждую смену нельзя.
//
// Файл без платформенного суффикса: кэш и пересчёт пикселей — чистые функции.

import (
	"github.com/oops1/headless-gui/v4/widget"
)

// cursorImageSource — движок знает курсор-картинку (реализует *engine.Engine).
type cursorImageSource interface {
	CursorImageOverride() (*widget.CursorImage, bool)
}

// cursorImageSetter — окно ОС умеет показать курсор из картинки. Зовётся на
// потоке окна, как SetCursor; последующий SetCursor заменяет картинку формой.
type cursorImageSetter interface {
	SetCursorImage(ci *widget.CursorImage)
}

// cursorImageOf возвращает курсор-картинку движка или nil.
func cursorImageOf(eng any) *widget.CursorImage {
	src, ok := eng.(cursorImageSource)
	if !ok {
		return nil
	}
	ci, _ := src.CursorImageOverride()
	return ci
}

// sameCursorImage — одна и та же картинка (по ключу; обе nil — тоже одна).
func sameCursorImage(a, b *widget.CursorImage) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Key == b.Key
}
