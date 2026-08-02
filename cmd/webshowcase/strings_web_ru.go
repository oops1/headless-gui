package main

import "github.com/oops1/headless-gui/v3/widget"

// webShowcaseRU — строки, которые есть только у браузерной витрины: пояснения
// про отсутствие окна ОС и записи её журнала. Общие с оконной витриной строки
// живут в cmd/internal/showcasestrings.
//
// Ключ — английская строка, поэтому таблицы для английского не нужно: Tr не
// найдёт перевода и вернёт сам ключ.
var webShowcaseRU = map[string]string{
	"Web showcase started: the server has no OS window":                                                                 "Витрина запущена в браузере: у сервера нет ни одного окна ОС",
	"This needs a real OS window: the server runs headless and has no tray.":                                            "Для этого нужно настоящее окно ОС: сервер работает headless, трея у него нет.",
	"Window-only feature requested in the browser":                                                                      "В браузере запрошена возможность окна ОС",
	"In the browser a large dialog is drawn over the dimmed canvas: there is no OS window to be larger than.":           "В браузере большой диалог рисуется поверх затемнённого холста: окна ОС, которое он мог бы перерасти, попросту нет.",
	"Drag and drop from the OS works in the native window; in the browser the canvas receives mouse and keyboard only.": "Перетаскивание файлов из ОС работает в нативном окне; в браузере холст принимает только мышь и клавиатуру.",
	"Dialogs: opened the large dialog over the stream":                                                                  "Диалоги: большой диалог открыт поверх стрима",
}

// registerWebStrings подключает таблицу браузерной витрины.
func registerWebStrings() {
	widget.RegisterStrings("RU", webShowcaseRU)
}
