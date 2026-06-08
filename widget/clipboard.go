package widget

import "sync"

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

// UseMemoryClipboard переключает буфер обмена на детерминированную in-memory
// реализацию (без интеграции с ОС). Предназначено для тестов и headless-сценариев,
// где зависимость от глобального системного буфера обмена даёт нестабильность.
func UseMemoryClipboard() {
	SetClipboardProvider(&memoryClipboard{})
}

// memoryClipboard — реализация в памяти (без OS интеграции). Потокобезопасна.
type memoryClipboard struct {
	mu   sync.Mutex
	text string
}

func (c *memoryClipboard) GetText() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.text
}

func (c *memoryClipboard) SetText(s string) {
	c.mu.Lock()
	c.text = s
	c.mu.Unlock()
}
