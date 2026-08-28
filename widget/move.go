// move.go — виджет сообщает, что его содержимое переехало.
//
// Перетаскивание окна не меняет пиксели: та же картинка оказывается в другом
// месте. Движку об этом знать неоткуда — он видит две изменившиеся области и
// честно отправляет обе. Потребитель платит за то, что у него уже есть.
//
// Здесь виджет объявляет перенос: «взять отсюда, положить туда». Движок
// складывает такие объявления в кадр, а потребитель выполняет копирование
// вместо перерисовки — в RDP это пара команд через кэш поверхности, локально
// обычный блит.
package widget

import (
	"image"
	"sync"
)

// MoveNotice — объявленный перенос: откуда взять и куда положить.
type MoveNotice struct {
	From image.Point
	Rect image.Rectangle
}

var (
	moveMu sync.Mutex
	// moveSinks — приёмники объявлений, по одному на движок. Список, а не
	// одна переменная: движков в процессе может быть несколько (окно и
	// вынесенный в своё окно попап), и перенос, объявленный деревом одного,
	// не должен попадать в кадр другого. Кому какое дерево принадлежит,
	// решает приёмник — он же и есть движок.
	moveSinks   []moveSink
	moveSinkSeq uint64
)

// moveSink — зарегистрированный приёмник объявлений о переносе.
type moveSink struct {
	handle uint64
	notify func(MoveNotice)
}

// RegisterMoveSink регистрирует приёмник объявлений о переносе и возвращает
// дескриптор для UnregisterMoveSink. Движок вызывает это при создании.
//
// Приёмник получает КАЖДОЕ объявление процесса и сам решает, относится ли
// оно к его дереву. Виджет об этом ничего не знает: он говорит «эти пиксели
// переехали», а не «переехали у такого-то движка».
func RegisterMoveSink(notify func(MoveNotice)) uint64 {
	if notify == nil {
		return 0
	}
	moveMu.Lock()
	moveSinkSeq++
	h := moveSinkSeq
	moveSinks = append(moveSinks, moveSink{handle: h, notify: notify})
	moveMu.Unlock()
	return h
}

// UnregisterMoveSink снимает приёмник по дескриптору. Идемпотентно.
func UnregisterMoveSink(handle uint64) {
	if handle == 0 {
		return
	}
	moveMu.Lock()
	for i := range moveSinks {
		if moveSinks[i].handle == handle {
			moveSinks = append(moveSinks[:i], moveSinks[i+1:]...)
			break
		}
	}
	moveMu.Unlock()
}

// NotifyMove объявляет, что содержимое области src целиком переехало в dst.
//
// Размер берётся у dst; если он не совпадает с размером источника, это уже
// не перенос, а масштабирование — такое объявление отбрасывается, потому что
// потребитель выполнит его блитом и получит искажение.
//
// Объявление НЕ отменяет обычную инвалидацию: виджет по-прежнему обязан
// заявить изменившиеся области. Перенос — подсказка, как дешевле получить тот
// же результат, а не замена damage.
func NotifyMove(src, dst image.Rectangle) {
	if src.Empty() || dst.Empty() {
		return
	}
	if src.Dx() != dst.Dx() || src.Dy() != dst.Dy() {
		return
	}
	if src.Min == dst.Min {
		return // никуда не переехало
	}
	notice := MoveNotice{From: src.Min, Rect: dst}

	moveMu.Lock()
	sinks := make([]func(MoveNotice), 0, len(moveSinks))
	for _, s := range moveSinks {
		sinks = append(sinks, s.notify)
	}
	moveMu.Unlock()

	// Приёмники зовём ВНЕ замка: движок в ответ трогает свои структуры, и
	// держать на это время общий замок пакета незачем.
	for _, notify := range sinks {
		notify(notice)
	}
}
