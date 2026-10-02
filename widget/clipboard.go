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

// ClipboardHTMLProvider — необязательное расширение ClipboardProvider: буфер,
// умеющий оформленный текст (HTML вместе с простым).
//
// Отдельный интерфейс, а не новые методы в ClipboardProvider: тот реализуют
// приложения, и добавление метода сломало бы каждую их реализацию. Провайдеры
// системного буфера (Windows, Wayland, X11) это расширение реализуют; свой
// провайдер приложения может его не поддерживать — тогда работает запасной
// путь SetClipboardHTML/ClipboardHTML.
type ClipboardHTMLProvider interface {
	// SetHTML кладёт в буфер оформленный текст и его простую версию рядом.
	// plain нужен всем, кто HTML не понимает (Блокнот, терминал): без него
	// вставка туда дала бы пустоту или сырую разметку.
	SetHTML(html, plain string)
	// GetHTML возвращает HTML-фрагмент из буфера; ok == false, если в буфере
	// HTML нет (там простой текст, изображение или пусто).
	GetHTML() (html string, ok bool)
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

// SetClipboardHTML кладёт в буфер обмена оформленный текст: html для тех, кто
// его понимает (Word, почтовик, браузерный редактор), и plain — для остальных
// (Блокнот, терминал).
//
// Если текущий провайдер HTML не умеет (приложение подставило свой, реализующий
// только ClipboardProvider), в буфер попадает только plain: оформление теряется,
// но скопированный текст не пропадает.
func SetClipboardHTML(html, plain string) {
	if p, ok := defaultClipboard.(ClipboardHTMLProvider); ok {
		p.SetHTML(html, plain)
		return
	}
	defaultClipboard.SetText(plain)
}

// ClipboardHTML возвращает HTML из буфера обмена. ok == false, если HTML там
// нет, а также если текущий провайдер HTML не умеет: честно «нет», а не простой
// текст под видом разметки (его пришлось бы экранировать, и вызывающий не
// отличил бы одно от другого). Запасной путь для вызывающего — ClipboardGetText.
func ClipboardHTML() (html string, ok bool) {
	if p, isHTML := defaultClipboard.(ClipboardHTMLProvider); isHTML {
		return p.GetHTML()
	}
	return "", false
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
	html string // оформленная версия того же содержимого; пусто — только текст
}

func (c *memoryClipboard) GetText() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.text
}

func (c *memoryClipboard) SetText(s string) {
	c.mu.Lock()
	c.text = s
	// Новый простой текст вытесняет прежнее оформление: иначе GetHTML отдал бы
	// разметку от предыдущего копирования, не связанную с текущим содержимым.
	c.html = ""
	c.mu.Unlock()
}

// SetHTML — реализация ClipboardHTMLProvider в памяти.
func (c *memoryClipboard) SetHTML(html, plain string) {
	c.mu.Lock()
	c.text, c.html = plain, html
	c.mu.Unlock()
}

// GetHTML — реализация ClipboardHTMLProvider в памяти.
func (c *memoryClipboard) GetHTML() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.html, c.html != ""
}
