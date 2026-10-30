package testhook

import "image"

// Точки для интеграционных тестов window/core, platform/a11y и platform/shell
// с живым окном Win32 (ядро и возможности поверх бэкенда platform/win32):
// тесты ядра не видят неэкспортированного состояния бэкенда, а публиковать
// его ради тестов незачем. Задаёт platform/win32 при инициализации (только
// Windows); до его загрузки и на других ОС nil. nw — *win32.Win32Window.

// Win32NewWindow создаёт окно бэкенда, закрытие которого не завершает цикл
// сообщений потока (тестам нужно несколько окон подряд). Уничтожить —
// DestroyWindow по HWND.
var Win32NewWindow func(title string, width, height int) (nw any, err error)

// Win32SetNoQuit — закрытие окна nw не завершает цикл сообщений потока.
var Win32SetNoQuit func(nw any)

// Win32WndProc — оконная процедура бэкенда: тест подаёт сообщение напрямую.
var Win32WndProc func(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr

// Win32WindowRect — внешний прямоугольник окна nw в координатах экрана.
var Win32WindowRect func(nw any) image.Rectangle

// Win32Cursor — курсор окна nw: показанный дескриптор, спрятан ли он, сколько
// курсоров из картинок создано.
var Win32Cursor func(nw any) (handle uintptr, hidden bool, created int)

// Win32CursorFor — дескриптор системного курсора формы c (base.Cursor).
var Win32CursorFor func(nw any, c int) uintptr

// Win32SetCursorMessage — ответ окна nw на WM_SETCURSOR в зоне hit.
var Win32SetCursorMessage func(nw any, hit uintptr) (handle uintptr, handled bool)

// Win32AfterResize ставит after перед обработчиком смены размера окна nw.
var Win32AfterResize func(nw any, after func(w, h int))
