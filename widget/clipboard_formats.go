package widget

// clipboard_formats.go — «что лежит в буфере обмена», без чтения содержимого.
//
// ClipboardFiles, ClipboardImage и ClipboardHTML ЧИТАЮТ данные, а читать
// приходится у владельца буфера: на X11 и Wayland это чужой процесс, который
// вправе отвечать секунды и минуты (удалённый рабочий стол отдаёт список
// файлов, когда они скачаны). Приложению же часто нужно только знать, есть ли
// в буфере файлы — например, чтобы сообщить об этом удалённой стороне при
// каждом изменении буфера. Звать для этого ClipboardFiles нельзя: проверка
// зависала бы вместе с владельцем.
//
// ClipboardFormats и ClipboardHasFiles отвечают на этот вопрос без чтения
// данных и без ожидания чужого владельца дольше короткого предела:
//   - Windows: IsClipboardFormatAvailable — мгновенно, буфер не открывается;
//   - macOS: список типов NSPasteboard — мгновенно;
//   - Wayland: список типов выбранного wl_data_offer уже пришёл событиями;
//   - X11: один запрос TARGETS (до 250 мс), ответ запоминается до смены
//     владельца буфера (XFixes) либо на полсекунды, если расширения нет.
//
// Устроено так же, как остальные возможности буфера: необязательное
// расширение провайдера (ClipboardFormatsProvider). Провайдер приложения,
// которое его не реализует, считается «не знающим»: ответ — пустой набор, а не
// чтение через GetFiles, потому что тот может ждать неограниченно.

import "strings"

// ClipboardFormatSet — набор видов содержимого, которые есть в буфере обмена.
// Один и тот же буфер несёт сразу несколько видов: скопированные файлы лежат
// и списком, и текстом путей; оформленный текст — и HTML, и простым.
type ClipboardFormatSet uint8

const (
	// ClipboardFormatText — простой текст (ClipboardGetText).
	ClipboardFormatText ClipboardFormatSet = 1 << iota
	// ClipboardFormatHTML — оформленный текст (ClipboardHTML).
	ClipboardFormatHTML
	// ClipboardFormatImage — изображение (ClipboardImage).
	ClipboardFormatImage
	// ClipboardFormatFiles — файлы (ClipboardFiles): готовый список или
	// «обещанные» файлы, которые владелец отдаст при вставке.
	ClipboardFormatFiles
)

// Has сообщает, что в наборе есть ВСЕ перечисленные виды.
func (s ClipboardFormatSet) Has(f ClipboardFormatSet) bool { return f != 0 && s&f == f }

// Empty — в буфере нет ни одного из известных видов.
func (s ClipboardFormatSet) Empty() bool { return s == 0 }

// String — для логов и сообщений: «text|html», «files», «-» для пустого набора.
func (s ClipboardFormatSet) String() string {
	var parts []string
	for _, n := range []struct {
		f    ClipboardFormatSet
		name string
	}{
		{ClipboardFormatText, "text"}, {ClipboardFormatHTML, "html"},
		{ClipboardFormatImage, "image"}, {ClipboardFormatFiles, "files"},
	} {
		if s&n.f != 0 {
			parts = append(parts, n.name)
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, "|")
}

// ClipboardFormatsProvider — необязательное расширение ClipboardProvider:
// буфер, умеющий быстро сказать, какие виды содержимого в нём лежат.
type ClipboardFormatsProvider interface {
	// ClipboardFormats возвращает набор видов содержимого, не читая сами
	// данные. Не должен ждать чужого владельца буфера дольше короткого предела
	// (доли секунды); безопасен из любой горутины.
	ClipboardFormats() ClipboardFormatSet
}

// ClipboardFormats возвращает набор видов содержимого системного буфера
// обмена: текст, HTML, изображение, файлы. Содержимое НЕ читается, а чужой
// владелец буфера не ждётся дольше короткого предела (на X11 — 250 мс на
// первый вопрос, дальше ответ берётся из памяти), так что вызов можно делать
// при каждом изменении буфера. Зовите вне замков: в худшем случае (X11,
// владелец молчит) вызов занимает эти 250 мс.
//
// Если провайдер приложения расширения ClipboardFormatsProvider не реализует,
// возвращается пустой набор: честное «не знаю», а не чтение через
// ClipboardFiles и ClipboardImage, которое могло бы ждать неограниченно.
func ClipboardFormats() ClipboardFormatSet {
	if p, ok := defaultClipboard.(ClipboardFormatsProvider); ok {
		return p.ClipboardFormats()
	}
	return 0
}

// ClipboardHasFiles сообщает, лежат ли в системном буфере обмена файлы
// (ClipboardFiles вернул бы ok == true), не читая их список и не дожидаясь
// владельца буфера. Это ClipboardFormats().Has(ClipboardFormatFiles); оговорки
// те же.
//
// На Windows «файлы» — это и готовый список (CF_HDROP), и «обещанные» файлы
// (FileGroupDescriptorW), которые владелец отдаёт при вставке. Положительный
// ответ говорит, что владелец ОБЪЯВИЛ файлы, но не что ClipboardFiles сумеет их
// прочитать: список может не разобраться, а владелец — не ответить.
func ClipboardHasFiles() bool {
	return ClipboardFormats().Has(ClipboardFormatFiles)
}

// ClipboardFormats — реализация ClipboardFormatsProvider в памяти.
func (c *memoryClipboard) ClipboardFormats() ClipboardFormatSet {
	c.mu.Lock()
	defer c.mu.Unlock()
	var s ClipboardFormatSet
	if c.text != "" {
		s |= ClipboardFormatText
	}
	if c.html != "" {
		s |= ClipboardFormatHTML
	}
	if c.img != nil {
		s |= ClipboardFormatImage
	}
	if len(c.files) > 0 || c.virt != nil {
		s |= ClipboardFormatFiles
	}
	return s
}
