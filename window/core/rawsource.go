package core

// rawsource.go — необязательные возможности бэкенда и движка: сырые клавиши и колесо.

import "github.com/oops1/headless-gui/v3/platform/keyboard"

// rawKeySource — бэкенд умеет сообщать клавиши вместе со скан-кодом, в том
// числе те, которых нет в таблице VK.
//
// Отдельный интерфейс по той же причине, что и keyRepeatSource: менять
// сигнатуры SetOnKeyDown/SetOnKeyUp значит ломать сборку каждому, кто
// реализует NativeWindow. Бэкенд, зарегистрировавший приёмник, больше НЕ
// вызывает SetOnKeyDown/SetOnKeyUp/SetOnKeyDownRepeat: всё идёт сюда. Текстовый
// ввод (SetOnChar) от этого не зависит и идёт как прежде.
type rawKeySource interface {
	SetOnRawKey(fn func(k keyboard.RawKey))
}

// wheelRawSource — бэкенд умеет сообщать колесо в единицах оборудования
// (120 на щелчок) до перевода в пиксели.
//
// Приёмник получает событие и функцию legacy: прежний путь доставки этого же
// события (пиксели или тики кнопок). Окно вызывает legacy само, если виджеты
// сырое колесо не приняли, — так одно движение колеса не прокручивает дерево
// дважды. Бэкенд без приёмника зовёт legacy сразу, как раньше.
type wheelRawSource interface {
	SetOnWheelRaw(fn func(x, y int, dx, dy float64, precise bool, legacy func()))
}

// wheelRawEngine — движок умеет принять сырое колесо (реализует *engine.Engine).
// Возвращает true, если какой-то виджет событие принял.
type wheelRawEngine interface {
	SendMouseWheelRaw(x, y int, dx, dy float64, precise bool) bool
}
