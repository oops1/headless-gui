//go:build windows

package clipboard

// clipboard_formats_windows.go — какие виды содержимого лежат в буфере
// обмена Windows (см. widget/base/clipboard_formats.go).
//
// IsClipboardFormatAvailable не открывает буфер и не просит данные у
// владельца: система хранит список форматов сама. Поэтому ответ мгновенный и
// не зависит от занятости буфера другим процессом (OpenClipboard там
// отказывает, пока слушатель держит буфер) и от медленного отложенного
// рендеринга.

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/oops1/headless-gui/v4/widget/base"
)

var procFmtsAvailable = user32Clip.NewProc("IsClipboardFormatAvailable")

// fmtsFileDescFormat — «FileGroupDescriptorW» (CFSTR_FILEDESCRIPTORW):
// «обещанные» файлы — имена, размеры, время; содержимое владелец отдаёт при
// вставке («FileContents»). Так объявляют файлы удалённые рабочие столы,
// архиваторы и почтовые клиенты: на диске их ещё нет, поэтому CF_HDROP у
// них нет.
var (
	fmtsFileDescOnce sync.Once
	fmtsFileDescID   uintptr
)

func fmtsFileDescFormat() uintptr {
	fmtsFileDescOnce.Do(func() {
		name, _ := syscall.UTF16PtrFromString("FileGroupDescriptorW")
		fmtsFileDescID, _, _ = procRegisterFormat.Call(uintptr(unsafe.Pointer(name)))
	})
	return fmtsFileDescID
}

// fmtsAvailable — формат есть в буфере. 0 — формат не зарегистрирован.
func fmtsAvailable(format uintptr) bool {
	if format == 0 {
		return false
	}
	r, _, _ := procFmtsAvailable.Call(format)
	return r != 0
}

// ClipboardFormats возвращает виды содержимого буфера по списку форматов
// системы. Реализует widget.ClipboardFormatsProvider.
func (c *winClipboard) ClipboardFormats() base.ClipboardFormatSet {
	var s base.ClipboardFormatSet
	if fmtsAvailable(cfUnicodeText) {
		s |= base.ClipboardFormatText
	}
	if fmtsAvailable(htmlFormat()) {
		s |= base.ClipboardFormatHTML
	}
	// Картинку система сама показывает несколькими форматами: DIB «выводится»
	// из DIBV5 и наоборот, так что достаточно проверить любой, но «PNG»
	// приходит отдельно (Chrome, Telegram).
	if fmtsAvailable(pngFormat()) || fmtsAvailable(cfDibV5) || fmtsAvailable(cfDib) {
		s |= base.ClipboardFormatImage
	}
	if fmtsAvailable(cfHDrop) || fmtsAvailable(fmtsFileDescFormat()) {
		s |= base.ClipboardFormatFiles
	}
	return s
}
