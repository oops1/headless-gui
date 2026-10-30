package native

// dragdrop.go — возможность перетаскивания файлов (ADR-0013 §3, запасной вариант):
// сам приёмник и источник живут в бэкендах (OLE на Win32, XDND на X11,
// wl_data_device на Wayland) — они на внутренностях окна, — а включает их
// флаг, который ставит пакет platform/dnd. Без него (window/minimal) окно не
// объявляет себя приёмником (ни OLE, ни XdndAware, offer Wayland отклоняется),
// а BeginDragFiles отвечает false.

// dndSlot — флаг: значение не нужно, важна регистрация.
var dndSlot = newCapSlot[struct{}]("dnd")

// RegisterDragDrop включает перетаскивание файлов (зовёт init пакета
// platform/dnd; тесты бэкендов — свой init). restore возвращает прежнее
// состояние.
//
//facadegen:skip
func RegisterDragDrop() (restore func()) { return dndSlot.register(struct{}{}) }

// DragDropEnabled — перетаскивание включено (подключён platform/dnd). Читают
// бэкенды при создании окна и ядро при BeginDragFiles.
//
//facadegen:skip
func DragDropEnabled() bool { _, ok := dndSlot.get(); return ok }
