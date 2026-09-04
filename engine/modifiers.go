// modifiers.go — модификаторы клавиатуры, зажатые в момент события мыши.
//
// Ctrl+Click и Shift+Click — обычный способ выделить несколько строк, и
// DataGrid его давно умеет: selectRow(row, shift, ctrl) честно делает и
// toggle, и диапазон. Вызвать это было нечем: widget.MouseEvent нёс только
// координаты и кнопку, и в движке на месте вызова стояло `false, false`.
//
// Взять модификаторы из клавиатурных событий движок не может: отдельной
// клавиши-модификатора в KeyCode нет, и человек, зажавший Ctrl и щёлкнувший
// мышью, не порождает НИ ОДНОГО клавиатурного события. Состояние знает только
// бэкенд окна — он и сообщает его сюда.
package engine

import "github.com/oops1/headless-gui/v3/widget"

// SetModifiers сообщает движку, какие модификаторы зажаты сейчас.
//
// Вызывает бэкенд окна при каждом нажатии и отпускании Ctrl/Shift/Alt
// (window.Run делает это сам). Всё, что движок разошлёт после этого —
// нажатия, движения, колесо — уедет с этими модификаторами в MouseEvent.Mod.
//
// Приложению, которое кормит движок событиями само (headless, тесты, свой
// бэкенд), достаточно позвать это перед SendMouseButton:
//
//	eng.SetModifiers(widget.ModCtrl)
//	eng.SendMouseButton(x, y, widget.MouseLeft, true)
//
// Состояние, а не параметр события: у SendMouseButton, SendMouseMove и
// SendMouseWheelPixels пришлось бы менять сигнатуры — все три, — и всякое
// приложение, собранное с прежней версией, перестало бы компилироваться ради
// параметра, который в большинстве вызовов равен нулю.
func (e *Engine) SetModifiers(mod widget.KeyMod) {
	e.mouseMod.Store(int32(mod))
}

// Modifiers возвращает модификаторы, которыми движок помечает события мыши.
func (e *Engine) Modifiers() widget.KeyMod {
	return widget.KeyMod(e.mouseMod.Load())
}
