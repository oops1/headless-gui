package base

// clipboard_files.go — список файлов в буфере обмена.
//
// Проводник на движке копировал файлы в собственный список внутри программы:
// в системный буфер уходил только текст. Файлы не доходили ни до другого
// файлового менеджера, ни до клиента удалённого рабочего стола, и обратно
// тоже не вставлялись. Здесь буфер получает настоящий список файлов —
// тот, что понимают Проводник Windows, Nautilus, Dolphin и RDP.
//
// Устроено так же, как оформленный текст: необязательное расширение
// провайдера, которое реализуют системные буферы (Windows, Wayland, X11) и
// буфер в памяти; провайдер приложения может его не поддерживать — тогда в
// буфер попадают пути простым текстом.

import (
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

// ClipboardFilesProvider — необязательное расширение ClipboardProvider: буфер,
// умеющий список файлов.
type ClipboardFilesProvider interface {
	// SetFiles кладёт в буфер список файлов (абсолютные пути) и рядом — те же
	// пути простым текстом. cut — файлы вырезали, а не скопировали.
	SetFiles(paths []string, cut bool)
	// GetFiles возвращает список файлов из буфера; ok == false, если в буфере
	// не файлы.
	GetFiles() (paths []string, cut bool, ok bool)
}

// ClipboardSetFiles кладёт в системный буфер обмена список файлов (абсолютные
// пути), заменяя его содержимое, как ClipboardSetText. cut — вырезать, а не
// копировать: файловый менеджер, куда вставят, переместит файлы.
//
// Рядом в буфер ложатся те же пути простым текстом, через перевод строки, —
// для программ, которые берут только текст. Если текущий провайдер файлы не
// умеет, ложится только этот текст.
func ClipboardSetFiles(paths []string, cut bool) {
	if p, ok := defaultClipboard.(ClipboardFilesProvider); ok {
		p.SetFiles(paths, cut)
		return
	}
	defaultClipboard.SetText(strings.Join(paths, "\n"))
}

// ClipboardFiles читает список файлов из системного буфера обмена. ok == false,
// если в буфере не файлы или провайдер их не умеет. cut — файлы вырезали.
//
// Вызов может ЖДАТЬ: владелец буфера отдаёт данные, когда готов, а на
// удалённом рабочем столе файлы, скопированные у клиента, скачиваются в
// момент вставки — секунды и дольше. Поэтому звать его стоит не на горутине
// движка (там рисуется кадр), а в своей, с результатом через Engine.Post.
func ClipboardFiles() (paths []string, cut bool, ok bool) {
	if p, isFiles := defaultClipboard.(ClipboardFilesProvider); isFiles {
		return p.GetFiles()
	}
	return nil, false, false
}

// SetFiles — реализация ClipboardFilesProvider в памяти.
func (c *memoryClipboard) SetFiles(paths []string, cut bool) {
	c.mu.Lock()
	old := c.virt
	c.text, c.html = strings.Join(paths, "\n"), ""
	c.files, c.cut, c.img, c.virt = append([]string(nil), paths...), cut, nil, nil
	c.mu.Unlock()
	old.drop()
}

// GetFiles — реализация ClipboardFilesProvider в памяти.
func (c *memoryClipboard) GetFiles() ([]string, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.files) == 0 {
		return nil, false, false
	}
	return append([]string(nil), c.files...), c.cut, true
}

// ─── CF_HDROP (Windows) ─────────────────────────────────────────────────────
//
// Сборка и разбор блока DROPFILES — без системных вызовов, чтобы их проверял
// общий прогон тестов. Блок: заголовок DROPFILES (смещение списка, точка
// сброса, флаг «вне клиентской области», флаг «пути в UTF-16») и пути,
// каждый с завершающим нулём, плюс ещё один ноль в конце списка.

// dropFilesHeaderSize — sizeof(DROPFILES): DWORD pFiles, POINT pt, BOOL fNC,
// BOOL fWide.
const dropFilesHeaderSize = 20

// BuildDropFiles собирает CF_HDROP с путями в UTF-16.
//
// Служебная: для подпакетов widget и platform/clipboard, в фасад не входит.
//
//facadegen:skip
func BuildDropFiles(paths []string) []byte {
	var u []uint16
	for _, p := range paths {
		u = append(u, utf16.Encode([]rune(p))...)
		u = append(u, 0)
	}
	u = append(u, 0)
	if len(paths) == 0 {
		u = append(u, 0) // пустой список — два нуля подряд
	}
	b := make([]byte, dropFilesHeaderSize+len(u)*2)
	binary.LittleEndian.PutUint32(b[0:4], dropFilesHeaderSize)
	binary.LittleEndian.PutUint32(b[16:20], 1) // fWide
	for i, v := range u {
		binary.LittleEndian.PutUint16(b[dropFilesHeaderSize+i*2:], v)
	}
	return b
}

// ParseDropFiles разбирает CF_HDROP с путями в UTF-16. ok == false, если блок
// битый или пути в однобайтовой кодировке (их разбирает система, см.
// platform/clipboard/clipboard_windows.go).
//
// Блок чужой, поэтому каждое смещение сверяется с его размером: прочитать
// за границей нельзя, сколько бы ни обещал заголовок.
//
// Служебная: для подпакетов widget и platform/clipboard, в фасад не входит.
//
//facadegen:skip
func ParseDropFiles(b []byte) (paths []string, ok bool) {
	if len(b) < dropFilesHeaderSize {
		return nil, false
	}
	off := int(binary.LittleEndian.Uint32(b[0:4]))
	wide := binary.LittleEndian.Uint32(b[16:20]) != 0
	if !wide || off < dropFilesHeaderSize || off > len(b) {
		return nil, false
	}
	var cur []uint16
	for i := off; i+1 < len(b); i += 2 {
		v := binary.LittleEndian.Uint16(b[i:])
		if v != 0 {
			cur = append(cur, v)
			continue
		}
		if len(cur) == 0 {
			break // второй ноль подряд — конец списка
		}
		paths = append(paths, string(utf16.Decode(cur)))
		cur = cur[:0]
	}
	if len(cur) > 0 {
		// Блок оборвался без завершающего нуля — последний путь не теряем.
		paths = append(paths, string(utf16.Decode(cur)))
	}
	return paths, len(paths) > 0
}

// Preferred DropEffect — DWORD рядом с CF_HDROP: что сделать при вставке.
//
// Служебные: для platform/clipboard, в фасад не входят.
//
//facadegen:skip
const (
	DropEffectCopy = 1
	DropEffectMove = 2
)

// DropEffectBytes — значение Preferred DropEffect для cut и copy.
//
// Служебная: для подпакетов widget и platform/clipboard, в фасад не входит.
//
//facadegen:skip
func DropEffectBytes(cut bool) []byte {
	v := uint32(DropEffectCopy)
	if cut {
		v = DropEffectMove
	}
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

// DropEffectIsCut — Preferred DropEffect говорит «переместить».
//
// Служебная: для подпакетов widget и platform/clipboard, в фасад не входит.
//
//facadegen:skip
func DropEffectIsCut(b []byte) bool {
	return len(b) >= 4 && binary.LittleEndian.Uint32(b)&DropEffectMove != 0
}
