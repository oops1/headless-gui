// cursor.go — типы курсора мыши и интерфейс CursorProvider.
package widget

// Cursor — форма курсора мыши.
type Cursor int

const (
	CursorArrow  Cursor = iota // обычная стрелка (по умолчанию)
	CursorIBeam                // текстовый курсор (для полей ввода)
	CursorHand                 // «рука» (для кликабельных ссылок)
	CursorSizeWE               // изменение размера ↔ (вертикальный сплиттер/границы)
	CursorSizeNS               // изменение размера ↕ (горизонтальный сплиттер/границы)
	CursorSizeNWSE             // изменение размера ⤡ (диагональ NW↔SE, углы окна)
	CursorSizeNESW             // изменение размера ⤢ (диагональ NE↔SW, углы окна)
)

// CursorProvider реализуется виджетами, которые хотят задать форму курсора
// под точкой (x, y) в своих bounds. Движок опрашивает виджет под курсором.
type CursorProvider interface {
	Cursor(x, y int) Cursor
}
