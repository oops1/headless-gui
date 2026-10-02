//go:build windows

package window

// ime_windows.go — редактор метода ввода (IME) на Windows.
//
// Китайский, японский и корейский набираются не по букве: человек печатает
// слоги, система показывает список кандидатов, и только выбранный вариант
// становится текстом. Окно движка об этом не знало — сообщения WM_IME_* не
// обрабатывались вовсе, — и набрать иероглиф в поле ввода было нельзя.
//
// Набираемое («композицию») окно забирает у системы и отдаёт движку, а тот —
// полю ввода: оно показывает набранное на месте каретки, подчёркнутым, и
// заменяет на следующее, пока ввод не завершится. Системное окно композиции
// при этом подавляется: своё поле рисует само приложение, и второе
// «плавающее» поверх него выглядело бы ошибкой.
//
// Список кандидатов остаётся за системой — его рисует она, нам остаётся
// сказать, ГДЕ его показать: под кареткой, а не в углу экрана.

import (
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmImeSetContext       = 0x0281
	wmImeStartComposition = 0x010D
	wmImeEndComposition   = 0x010E
	wmImeComposition      = 0x010F

	// GCS_* — что именно просить у контекста ввода.
	gcsCompStr   = 0x0008 // набираемое (ещё не введено)
	gcsCursorPos = 0x0080 // позиция курсора внутри набираемого
	gcsResultStr = 0x0800 // готовый результат: человек выбрал вариант

	// ISC_SHOWUICOMPOSITIONWINDOW — бит, которым система спрашивает, рисовать
	// ли ей своё окно композиции. Снимаем: поле рисует приложение.
	iscShowUICompositionWindow = 0x80000000

	// CFS_* — вид формы окна кандидатов и композиции.
	cfsPoint        = 0x0002
	cfsCandidatePos = 0x0040
)

var (
	imm32                      = windows.NewLazySystemDLL("imm32.dll")
	procImmGetContext          = imm32.NewProc("ImmGetContext")
	procImmReleaseContext      = imm32.NewProc("ImmReleaseContext")
	procImmGetCompositionStrW  = imm32.NewProc("ImmGetCompositionStringW")
	procImmSetCandidateWindow  = imm32.NewProc("ImmSetCandidateWindow")
	procImmSetCompositionWndow = imm32.NewProc("ImmSetCompositionWindow")
	procImmNotifyIME           = imm32.NewProc("ImmNotifyIME")
)

// candidateForm — CANDIDATEFORM: где показать список кандидатов.
type candidateForm struct {
	index        uint32
	style        uint32
	ptCurrentPos point
	rcArea       rect
}

// compositionForm — COMPOSITIONFORM: где система считает каретку.
type compositionForm struct {
	style        uint32
	ptCurrentPos point
	rcArea       rect
}

// SetOnIMEComposition подключает приёмник набираемого текста.
func (w *Win32Window) SetOnIMEComposition(fn func(text string, caret int)) {
	w.onIMEComposition = fn
}

// SetOnIMECommit подключает приёмник готового текста.
func (w *Win32Window) SetOnIMECommit(fn func(text string)) { w.onIMECommit = fn }

// SetIMECaretProvider подключает источник места каретки (физические пиксели
// клиентской области): по нему ставится окно кандидатов.
func (w *Win32Window) SetIMECaretProvider(fn func() (image.Rectangle, bool)) {
	w.imeCaret = fn
}

// handleIMEMessage разбирает сообщения редактора метода ввода. Возвращает
// (результат, true), если сообщение обработано целиком.
func (w *Win32Window) handleIMEMessage(hwnd uintptr, umsg uint32, wparam, lparam uintptr) (uintptr, bool) {
	switch umsg {
	case wmImeSetContext:
		// Своё окно композиции системе рисовать не нужно: поле рисует
		// приложение, и второе поверх него выглядело бы ошибкой. Остальные
		// биты (список кандидатов) оставляем ей.
		lparam &^= iscShowUICompositionWindow
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wparam, lparam)
		return ret, true

	case wmImeStartComposition:
		w.imePlaceWindows(hwnd)
		return 0, true // системное окно композиции подавлено

	case wmImeComposition:
		w.imeHandleComposition(hwnd, lparam)
		w.imePlaceWindows(hwnd)
		return 0, true

	case wmImeEndComposition:
		// Набранное к этому моменту либо введено (пришло результатом), либо
		// брошено. Пустая композиция убирает остаток из поля.
		if w.onIMEComposition != nil {
			w.onIMEComposition("", 0)
		}
		return 0, true
	}
	return 0, false
}

// imeHandleComposition забирает из контекста ввода готовый результат и
// набираемое.
//
// Результат читается ПЕРВЫМ: в одном сообщении приходят оба, и если сперва
// показать набираемое, введённый текст встал бы после него.
func (w *Win32Window) imeHandleComposition(hwnd uintptr, lparam uintptr) {
	himc, _, _ := procImmGetContext.Call(hwnd)
	if himc == 0 {
		return
	}
	defer procImmReleaseContext.Call(hwnd, himc)

	if lparam&gcsResultStr != 0 && w.onIMECommit != nil {
		if s, ok := immCompositionString(himc, gcsResultStr); ok && s != "" {
			w.onIMECommit(s)
		}
	}
	if lparam&gcsCompStr != 0 && w.onIMEComposition != nil {
		s, _ := immCompositionString(himc, gcsCompStr)
		caret := 0
		if lparam&gcsCursorPos != 0 {
			// Позиция курсора приходит значением, а не строкой: длина
			// возвращается самим вызовом.
			n, _, _ := procImmGetCompositionStrW.Call(himc, gcsCursorPos, 0, 0)
			caret = int(int32(n))
		}
		w.onIMEComposition(s, caret)
	}
}

// immCompositionString читает строку из контекста ввода.
//
// Вызов сперва спрашивает ДЛИНУ (нулевой буфер), потом забирает данные:
// длина в БАЙТАХ, а строка — UTF-16, и делить надо на два.
func immCompositionString(himc uintptr, what uint32) (string, bool) {
	n, _, _ := procImmGetCompositionStrW.Call(himc, uintptr(what), 0, 0)
	size := int(int32(n))
	if size <= 0 || size > 1<<20 {
		return "", false
	}
	buf := make([]uint16, size/2)
	r, _, _ := procImmGetCompositionStrW.Call(himc, uintptr(what),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(size))
	if int(int32(r)) <= 0 {
		return "", false
	}
	return windows.UTF16ToString(buf), true
}

// imePlaceWindows ставит окно кандидатов под кареткой.
//
// Без этого система показывает его там, где считает нужным, — обычно в углу
// окна: человек печатает в одном месте, а варианты выбирает в другом.
func (w *Win32Window) imePlaceWindows(hwnd uintptr) {
	if w.imeCaret == nil {
		return
	}
	r, ok := w.imeCaret()
	if !ok {
		return
	}
	himc, _, _ := procImmGetContext.Call(hwnd)
	if himc == 0 {
		return
	}
	defer procImmReleaseContext.Call(hwnd, himc)

	pt := point{X: int32(r.Min.X), Y: int32(r.Max.Y)}
	area := rect{Left: int32(r.Min.X), Top: int32(r.Min.Y),
		Right: int32(r.Max.X), Bottom: int32(r.Max.Y)}

	cand := candidateForm{style: cfsCandidatePos, ptCurrentPos: pt, rcArea: area}
	procImmSetCandidateWindow.Call(himc, uintptr(unsafe.Pointer(&cand)))

	comp := compositionForm{style: cfsPoint, ptCurrentPos: point{X: int32(r.Min.X), Y: int32(r.Min.Y)}}
	procImmSetCompositionWndow.Call(himc, uintptr(unsafe.Pointer(&comp)))
}

// imeCancelComposition просит систему бросить начатый набор (смена фокуса,
// Escape): иначе набранное зависло бы в поле.
func (w *Win32Window) imeCancelComposition(hwnd uintptr) {
	const (
		niiCompositionStr = 0x0015
		cpsCancel         = 0x0004
	)
	himc, _, _ := procImmGetContext.Call(hwnd)
	if himc == 0 {
		return
	}
	defer procImmReleaseContext.Call(hwnd, himc)
	procImmNotifyIME.Call(himc, niiCompositionStr, cpsCancel, 0)
}
