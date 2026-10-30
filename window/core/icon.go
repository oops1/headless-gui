// icon.go — значок окна во время работы.
//
// На Win32 значок окна и кнопки на панели задач брался ТОЛЬКО из ресурса
// RT_GROUP_ICON исполняемого файла: поменять его на лету было нечем, а на X11
// свойство _NET_WM_ICON не выставлялось вовсе — под Linux у окна значка не
// было в принципе, сколько бы ресурсов ни лежало в сборке.
//
// Здесь значок задаётся картинкой: и на старте, и в любой момент работы —
// например, чтобы показать ход длительной операции.
package core

import (
	"image"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// SetIcon задаёт значок окна.
//
// Принимает несколько размеров: система выбирает подходящий сама — Windows
// берёт разные для заголовка (16×16) и для Alt+Tab (32×32), оконные менеджеры
// X11 — свой под каждое место. Один размер тоже годится: недостающие система
// отмасштабирует, просто хуже, чем это сделал бы художник.
//
// Вызывать можно в любой момент после создания окна. Пустой список снимает
// значок, заданный ранее.
func (w *Window) SetIcon(icons ...image.Image) error {
	out := make([]image.Image, 0, len(icons))
	for _, ic := range icons {
		if ic == nil || ic.Bounds().Empty() {
			continue
		}
		out = append(out, ic)
	}
	w.pendingIcons, w.iconWant = out, true

	// До Run() бэкенда ещё нет: поле native заполняет первая же строка Run, и
	// приведение типа на nil-интерфейсе всегда даёт «не поддерживается».
	// Значок откладывается и ставится при создании окна — ровно так же, как
	// это делает SetTrayIcon.
	if w.native == nil {
		return nil
	}
	setter, ok := w.native.(native.IconSetter)
	if !ok {
		return native.ErrIconUnsupported
	}
	return setter.SetIcon(out)
}

// applyPendingIcon отдаёт бэкенду значок, заданный до Run().
//
// Зовётся сразу после создания объекта бэкенда и ДО создания окна ОС: сам
// бэкенд подержит значок у себя и выставит его в нужный момент — на X11 до
// MapWindow, иначе оконный менеджер успевает показать окно без значка.
func (w *Window) applyPendingIcon() {
	if !w.iconWant || w.native == nil {
		return
	}
	if setter, ok := w.native.(native.IconSetter); ok {
		_ = setter.SetIcon(w.pendingIcons)
	}
}
