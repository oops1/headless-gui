// Package pointer — переносимая часть мыши и курсора нативных бэкендов окна
// (ADR-0013): слой ниже бэкендов Win32, X11, Wayland и Cocoa.
//
// Здесь:
//   - кнопки мыши по ОС → коды NativeWindow.SetOnMouseButton (ButtonLeft…
//     ButtonX2; коды evdev Wayland — WlBtn*), уход указателя из окна
//     (Outside, LeaveTracker), колесо в единицах оборудования, разбор
//     RAWINPUT мыши;
//   - картинки курсоров по формам: CursorArrow… — числовые значения
//     base.Cursor (не тип), CursorImage и CursorPixels — их картинки (не
//     base.CursorImage, пользовательская картинка курсора);
//   - пользовательские курсоры: кэш CursorLRU, пиксели и маски по ОС.
//
// Открытых имён window пакет не даёт; программы его не импортируют.
// Эмуляция мыши касанием и состояние захвата указателя — обвязка окна
// (surface) и остаются в window/core.
package pointer
