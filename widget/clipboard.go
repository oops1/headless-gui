package widget

// clipboard.go — кросс-платформенный буфер обмена.
//
// Абстракция для работы с системным буфером обмена.
// Платформенные реализации: clipboard_windows.go, clipboard_linux.go, clipboard_darwin.go.

// ClipboardProvider — интерфейс доступа к системному буферу обмена.
type ClipboardProvider interface {
	// GetText возвращает текст из буфера обмена. Пустая строка если буфер пуст или ошибка.
	GetText() string
	// SetText записывает текст в буфер обмена.
	SetText(s string)
}

// defaultClipboard — глобальный провайдер буфера обмена.
// Инициализируется платформенной реализацией.
var defaultClipboard ClipboardProvider = &memoryClipboard{}

// SetClipboardProvider устанавливает глобальный провайдер буфера обмена.
func SetClipboardProvider(p ClipboardProvider) {
	if p != nil {
		defaultClipboard = p
	}
}

// GetClipboardProvider возвращает текущий провайдер буфера обмена.
func GetClipboardProvider() ClipboardProvider {
	return defaultClipboard
}

// ClipboardGetText возвращает текст из системного буфера обмена.
func ClipboardGetText() string {
	return defaultClipboard.GetText()
}

// ClipboardSetText записывает текст в системный буфер обмена.
func ClipboardSetText(s string) {
	defaultClipboard.SetText(s)
}

// memoryClipboard — fallback реализация (в памяти, без OS интеграции).
type memoryClipboard struct {
	text string
}

func (c *memoryClipboard) GetText() string  { return c.text }
func (c *memoryClipboard) SetText(s string) { c.text = s }
