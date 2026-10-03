//go:build windows

package window

import (
	"image"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Паттерн Text: скринридер читает многострочный текст кусками. Тесты вызывают
// провайдер через НАСТОЯЩИЕ таблицы виртуальных методов, как это делает UIA:
// перепутанный слот виден сразу.

// Слоты ITextProvider: три метода IUnknown, затем по порядку интерфейса.
const (
	slotTextGetSelection     = 3
	slotTextVisibleRanges    = 4
	slotTextRangeFromChild   = 5
	slotTextRangeFromPoint   = 6
	slotTextDocumentRange    = 7
	slotTextSupportedSelect  = 8
	slotRangeClone           = 3
	slotRangeCompare         = 4
	slotRangeCompareEndpts   = 5
	slotRangeExpand          = 6
	slotRangeFindAttribute   = 7
	slotRangeFindText        = 8
	slotRangeGetAttribute    = 9
	slotRangeBoundingRects   = 10
	slotRangeEnclosing       = 11
	slotRangeGetText         = 12
	slotRangeMove            = 13
	slotRangeMoveEndpoint    = 14
	slotRangeMoveEndpointRng = 15
	slotRangeSelect          = 16
	slotRangeAddToSelection  = 17
	slotRangeRemoveFromSel   = 18
	slotRangeScrollIntoView  = 19
	slotRangeGetChildren     = 20
)

// textScene — сцена из value-теста плюс удобные обёртки вызовов.
type textScene struct {
	*uiaValueScene
}

// flush выполняет правки, которые провайдер поставил на горутину движка:
// движок в тесте не запущен, и очередь Post иначе осталась бы нетронутой.
func (sc *textScene) flush() {
	sc.b.win.eng.(interface{ Flush() }).Flush()
}

func newTextScene(t *testing.T) *textScene {
	t.Helper()
	return &textScene{newUIAValueScene(t)}
}

// docRange берёт у элемента диапазон всего документа через ITextProvider.
func (sc *textScene) docRange(t *testing.T, e *uiaElement) uintptr {
	t.Helper()
	// Ячейка для ответа — в куче, а не на стеке (см. comCall): помощник
	// зовут тысячи раз подряд, и именно здесь рост стека под -race ловил
	// ответ мимо переменной.
	r := new(uintptr)
	if hr := comCall(e.textPtr(), slotTextDocumentRange, uintptr(unsafe.Pointer(r))); hr != sOK || *r == 0 {
		t.Fatalf("get_DocumentRange: hr=%#x r=%#x", hr, *r)
	}
	return *r
}

func rangeText(t *testing.T, r uintptr) string {
	t.Helper()
	return rangeTextMax(t, r, -1)
}

func rangeTextMax(t *testing.T, r uintptr, maxLen int32) string {
	t.Helper()
	var bstr uintptr
	if hr := comCall(r, slotRangeGetText, uintptr(uint32(maxLen)), uintptr(unsafe.Pointer(&bstr))); hr != sOK {
		t.Fatalf("GetText: hr=%#x", hr)
	}
	if bstr == 0 {
		return ""
	}
	defer procSysFreeString.Call(bstr)
	return bstrToString(bstr)
}

func rangeMove(t *testing.T, r uintptr, unit, count int32) int32 {
	t.Helper()
	var moved int32
	if hr := comCall(r, slotRangeMove, uintptr(unit), uintptr(uint32(count)), uintptr(unsafe.Pointer(&moved))); hr != sOK {
		t.Fatalf("Move: hr=%#x", hr)
	}
	return moved
}

func rangeExpand(t *testing.T, r uintptr, unit int32) {
	t.Helper()
	if hr := comCall(r, slotRangeExpand, uintptr(unit)); hr != sOK {
		t.Fatalf("ExpandToEnclosingUnit: hr=%#x", hr)
	}
}

func rangeRelease(r uintptr) { comCall(r, slotRelease) }

// safeArrayLen — длина одномерного SAFEARRAY.
func safeArrayLen(t *testing.T, sa uintptr) int {
	t.Helper()
	var ub int32
	if hr, _, _ := procSafeArrayGetUBound.Call(sa, 1, uintptr(unsafe.Pointer(&ub))); hr != 0 {
		t.Fatalf("SafeArrayGetUBound: hr=%#x", hr)
	}
	return int(ub) + 1
}

// ─── Объявление паттерна ─────────────────────────────────────────────────────

// Паттерн объявляется там, где есть текст, и только там; QueryInterface и
// GetPatternProvider обязаны отвечать одинаково.
func TestUIAText_PatternByRole(t *testing.T) {
	sc := newTextScene(t)
	for _, w := range []widget.Widget{sc.ti, sc.tb} {
		e := sc.elemFor(t, w)
		var p uintptr
		if hr := comCall(e.simplePtr(), slotPatternProvider, uintptr(uiaPatternText),
			uintptr(unsafe.Pointer(&p))); hr != sOK || p != e.textPtr() {
			t.Errorf("GetPatternProvider(Text): hr=%#x p=%#x, ждал %#x", hr, p, e.textPtr())
		}
		comCall(e.textPtr(), slotRelease) // GetPatternProvider отдал ссылку
		var out uintptr
		if hr := comCall(e.simplePtr(), slotQueryInterface, uintptr(unsafe.Pointer(&iidTextProvider)),
			uintptr(unsafe.Pointer(&out))); hr != sOK || out != e.textPtr() {
			t.Errorf("QI(Text): hr=%#x out=%#x", hr, out)
		}
		comCall(e.textPtr(), slotRelease)
	}
}

// У надписи и кнопки паттерна нет: пустой ITextProvider заставил бы скринридер
// искать в них текст для чтения.
func TestUIAText_NotOnButton(t *testing.T) {
	b, _ := newUIATestBridge(t)
	v := b.current()
	for i := range v.Snap.Nodes {
		n := &v.Snap.Nodes[i]
		if n.Info.Role != widget.RoleButton {
			continue
		}
		e := b.element(v.id(int32(i)))
		var p uintptr
		if hr := comCall(e.simplePtr(), slotPatternProvider, uintptr(uiaPatternText),
			uintptr(unsafe.Pointer(&p))); hr != sOK || p != 0 {
			t.Errorf("у кнопки паттерн Text: hr=%#x p=%#x", hr, p)
		}
		var out uintptr
		if hr := comCall(e.simplePtr(), slotQueryInterface, uintptr(unsafe.Pointer(&iidTextProvider)),
			uintptr(unsafe.Pointer(&out))); hr != eNoInterface || out != 0 {
			t.Errorf("QI(Text) у кнопки: hr=%#x", hr)
		}
		return
	}
	t.Fatal("в сцене нет кнопки")
}

// У поля пароля паттерна нет: системное поле с ES_PASSWORD его тоже не
// отдаёт, а иначе скринридер прочитал бы пароль по символам.
func TestUIAText_NotOnPassword(t *testing.T) {
	root := widget.NewWindow("Пароль", 320, 100)
	root.SetBounds(image.Rect(0, 0, 320, 100))
	pw := widget.NewPasswordInput("")
	pw.SetBounds(image.Rect(10, 10, 300, 40))
	pw.SetText("секрет")
	root.AddChild(pw)
	eng := engine.New(320, 100, 30)
	eng.SetRoot(root)

	win := New(eng, "Окно UIA")
	win.scale = 1
	b := &uiaBridge{win: win, elems: map[int32]*uiaElement{}}
	b.refresh(true)
	t.Cleanup(func() {
		b.mu.Lock()
		for _, el := range b.elems {
			el.forget()
		}
		b.mu.Unlock()
	})
	sc := &uiaValueScene{b: b}

	e := sc.elemFor(t, pw)
	var p uintptr
	if hr := comCall(e.simplePtr(), slotPatternProvider, uintptr(uiaPatternText),
		uintptr(unsafe.Pointer(&p))); hr != sOK || p != 0 {
		t.Errorf("у поля пароля паттерн Text: hr=%#x p=%#x", hr, p)
	}
	var out uintptr
	if hr := comCall(e.simplePtr(), slotQueryInterface, uintptr(unsafe.Pointer(&iidTextProvider)),
		uintptr(unsafe.Pointer(&out))); hr != eNoInterface {
		t.Errorf("QI(Text) у поля пароля: hr=%#x", hr)
	}
}

func TestUIAText_SupportedSelection(t *testing.T) {
	sc := newTextScene(t)
	e := sc.elemFor(t, sc.tb)
	var v int32
	if hr := comCall(e.textPtr(), slotTextSupportedSelect, uintptr(unsafe.Pointer(&v))); hr != sOK || v != uiaSelectionSingle {
		t.Errorf("SupportedTextSelection: hr=%#x v=%d", hr, v)
	}
}

// ─── Чтение ──────────────────────────────────────────────────────────────────

func TestUIAText_DocumentRangeAndGetText(t *testing.T) {
	sc := newTextScene(t)
	r := sc.docRange(t, sc.elemFor(t, sc.tb))
	defer rangeRelease(r)

	if got := rangeText(t, r); got != "первая строка\nвторая строка" {
		t.Errorf("документ читается как %q", got)
	}
	if got := rangeTextMax(t, r, 6); got != "первая" {
		t.Errorf("усечённый до 6 символов: %q", got)
	}
	var bstr uintptr
	if hr := comCall(r, slotRangeGetText, uintptr(^uintptr(1)), uintptr(unsafe.Pointer(&bstr))); hr != eInvalidArg {
		t.Errorf("maxLength = -2: hr=%#x, ждал E_INVALIDARG", hr)
	}
}

// Скринридер идёт по документу так: берёт диапазон, расширяет до строки,
// читает, сдвигает на следующую.
func TestUIAText_ReadByLines(t *testing.T) {
	sc := newTextScene(t)
	r := sc.docRange(t, sc.elemFor(t, sc.tb))
	defer rangeRelease(r)

	// Схлопываем в начало: сдвиг конца назад на документ.
	var moved int32
	comCall(r, slotRangeMoveEndpoint, uintptr(a11yEndEnd), uintptr(a11yUnitDocument),
		uintptr(^uintptr(0)), uintptr(unsafe.Pointer(&moved)))
	if got := rangeText(t, r); got != "" {
		t.Fatalf("после схлопывания: %q", got)
	}

	rangeExpand(t, r, int32(a11yUnitLine))
	if got := rangeText(t, r); got != "первая строка\n" {
		t.Errorf("первая строка: %q", got)
	}
	if n := rangeMove(t, r, int32(a11yUnitLine), 1); n != 1 {
		t.Errorf("сдвиг на строку вперёд: moved=%d", n)
	}
	if got := rangeText(t, r); got != "вторая строка" {
		t.Errorf("вторая строка: %q", got)
	}
	if n := rangeMove(t, r, int32(a11yUnitLine), 1); n != 0 {
		t.Errorf("дальше конца: moved=%d, ждал 0", n)
	}
	if n := rangeMove(t, r, int32(a11yUnitLine), -5); n != -1 {
		t.Errorf("назад с упором в начало: moved=%d, ждал -1", n)
	}
	if got := rangeText(t, r); got != "первая строка\n" {
		t.Errorf("снова первая: %q", got)
	}

	rangeExpand(t, r, int32(a11yUnitWord))
	if got := rangeText(t, r); got != "первая " {
		t.Errorf("слово: %q", got)
	}
	rangeMove(t, r, int32(a11yUnitWord), 1)
	if got := rangeText(t, r); got != "строка" {
		t.Errorf("следующее слово: %q", got)
	}
	rangeExpand(t, r, int32(a11yUnitCharacter))
	if got := rangeText(t, r); got != "с" {
		t.Errorf("символ: %q", got)
	}

	// Чужое значение TextUnit — ошибка аргумента, а не молчаливый сдвиг.
	if hr := comCall(r, slotRangeExpand, 99); hr != eInvalidArg {
		t.Errorf("ExpandToEnclosingUnit(99): hr=%#x", hr)
	}
}

// Выделение и каретка читаются у ЖИВОГО виджета.
func TestUIAText_SelectionFollowsWidget(t *testing.T) {
	sc := newTextScene(t)
	e := sc.elemFor(t, sc.tb)

	selection := func() (string, a11yRange) {
		var sa uintptr
		if hr := comCall(e.textPtr(), slotTextGetSelection, uintptr(unsafe.Pointer(&sa))); hr != sOK || sa == 0 {
			t.Fatalf("GetSelection: hr=%#x", hr)
		}
		defer procSafeArrayDestroyTest.Call(sa)
		if n := safeArrayLen(t, sa); n != 1 {
			t.Fatalf("диапазонов в выделении %d, ждал 1", n)
		}
		var idx int32
		var r uintptr
		procSafeArrayGetElement.Call(sa, uintptr(unsafe.Pointer(&idx)), uintptr(unsafe.Pointer(&r)))
		defer rangeRelease(r)
		return rangeText(t, r), uiaLookupRange(r).get()
	}

	sc.tb.SetCaretPosition(3)
	if text, r := selection(); text != "" || r != (a11yRange{3, 3}) {
		t.Errorf("каретка: %q %+v", text, r)
	}
	sc.tb.AccessSetSelection(7, 13)
	if text, r := selection(); text != "строка" || r != (a11yRange{7, 13}) {
		t.Errorf("выделение: %q %+v", text, r)
	}
}

func TestUIAText_CompareAndClone(t *testing.T) {
	sc := newTextScene(t)
	e := sc.elemFor(t, sc.tb)
	a := sc.docRange(t, e)
	defer rangeRelease(a)

	var c uintptr
	if hr := comCall(a, slotRangeClone, uintptr(unsafe.Pointer(&c))); hr != sOK || c == 0 || c == a {
		t.Fatalf("Clone: hr=%#x c=%#x", hr, c)
	}
	defer rangeRelease(c)

	var eq int32
	comCall(a, slotRangeCompare, c, uintptr(unsafe.Pointer(&eq)))
	if eq != 1 {
		t.Error("копия не равна оригиналу")
	}
	// Копия независима: её сдвиг не трогает оригинал.
	rangeExpand(t, c, int32(a11yUnitWord))
	comCall(a, slotRangeCompare, c, uintptr(unsafe.Pointer(&eq)))
	if eq != 0 {
		t.Error("после сдвига копии диапазоны всё ещё «равны»")
	}
	if got := rangeText(t, a); got != "первая строка\nвторая строка" {
		t.Errorf("оригинал изменился: %q", got)
	}

	// Концы: начало копии совпадает с началом документа, конец — раньше.
	var cmp int32
	comCall(c, slotRangeCompareEndpts, uintptr(a11yEndStart), a, uintptr(a11yEndStart), uintptr(unsafe.Pointer(&cmp)))
	if cmp != 0 {
		t.Errorf("начала: %d, ждал 0", cmp)
	}
	comCall(c, slotRangeCompareEndpts, uintptr(a11yEndEnd), a, uintptr(a11yEndEnd), uintptr(unsafe.Pointer(&cmp)))
	if cmp != -1 {
		t.Errorf("концы: %d, ждал -1", cmp)
	}
	if hr := comCall(c, slotRangeCompareEndpts, 5, a, 0, uintptr(unsafe.Pointer(&cmp))); hr != eInvalidArg {
		t.Errorf("чужой конец диапазона: hr=%#x", hr)
	}

	// Диапазоны разных элементов не равны, а сравнение концов — ошибка.
	other := sc.docRange(t, sc.elemFor(t, sc.ti))
	defer rangeRelease(other)
	comCall(a, slotRangeCompare, other, uintptr(unsafe.Pointer(&eq)))
	if eq != 0 {
		t.Error("диапазоны разных элементов признаны равными")
	}
	if hr := comCall(a, slotRangeCompareEndpts, 0, other, 0, uintptr(unsafe.Pointer(&cmp))); hr != eInvalidArg {
		t.Errorf("концы диапазонов разных элементов: hr=%#x", hr)
	}
}

func TestUIAText_MoveEndpoints(t *testing.T) {
	sc := newTextScene(t)
	e := sc.elemFor(t, sc.tb)
	a := sc.docRange(t, e)
	defer rangeRelease(a)
	b := sc.docRange(t, e)
	defer rangeRelease(b)

	// Двигаем конец b назад на два слова: «строка» и «вторая ».
	var moved int32
	comCall(b, slotRangeMoveEndpoint, uintptr(a11yEndEnd), uintptr(a11yUnitWord),
		uintptr(^uintptr(1)), uintptr(unsafe.Pointer(&moved))) // -2
	if moved != -2 {
		t.Fatalf("moved=%d", moved)
	}
	if got := rangeText(t, b); got != "первая строка\n" {
		t.Errorf("после сдвига конца: %q", got)
	}
	// Начало a — на конец b: MoveEndpointByRange.
	if hr := comCall(a, slotRangeMoveEndpointRng, uintptr(a11yEndStart), b, uintptr(a11yEndEnd)); hr != sOK {
		t.Fatalf("MoveEndpointByRange: hr=%#x", hr)
	}
	if got := rangeText(t, a); got != "вторая строка" {
		t.Errorf("хвост документа: %q", got)
	}
}

func TestUIAText_FindText(t *testing.T) {
	sc := newTextScene(t)
	r := sc.docRange(t, sc.elemFor(t, sc.tb))
	defer rangeRelease(r)

	find := func(s string, backward, ignoreCase int32) uintptr {
		text, _ := syscall.UTF16PtrFromString(s)
		bstr, _, _ := procSysAllocString.Call(uintptr(unsafe.Pointer(text)))
		defer procSysFreeString.Call(bstr)
		var out uintptr
		if hr := comCall(r, slotRangeFindText, bstr, uintptr(backward), uintptr(ignoreCase),
			uintptr(unsafe.Pointer(&out))); hr != sOK {
			t.Fatalf("FindText(%q): hr=%#x", s, hr)
		}
		return out
	}

	f := find("СТРОКА", 0, 1)
	if f == 0 {
		t.Fatal("строка не найдена без учёта регистра")
	}
	defer rangeRelease(f)
	if got := uiaLookupRange(f).get(); got != (a11yRange{7, 13}) {
		t.Errorf("первое вхождение: %+v", got)
	}
	b := find("строка", 1, 0)
	defer rangeRelease(b)
	if got := uiaLookupRange(b).get(); got != (a11yRange{21, 27}) {
		t.Errorf("последнее вхождение: %+v", got)
	}
	if find("СТРОКА", 0, 0) != 0 {
		t.Error("найдено с учётом регистра")
	}
	if find("нет такого", 0, 0) != 0 {
		t.Error("найдено несуществующее")
	}
}

// ─── Атрибуты, границы, элемент, дети ────────────────────────────────────────

// На любой атрибут — «не поддерживается»: особый объект UIA, а не пустое
// значение, которое означало бы «у атрибута пустое значение».
func TestUIAText_AttributesNotSupported(t *testing.T) {
	sc := newTextScene(t)
	r := sc.docRange(t, sc.elemFor(t, sc.tb))
	defer rangeRelease(r)

	var v comVariant
	if hr := comCall(r, slotRangeGetAttribute, 40005, uintptr(unsafe.Pointer(&v))); hr != sOK {
		t.Fatalf("GetAttributeValue: hr=%#x", hr)
	}
	if v.vt != vtUnknown || v.val[0] == 0 {
		t.Errorf("ответ vt=%d val=%#x, ждал VT_UNKNOWN с NotSupportedValue", v.vt, v.val[0])
	}

	var in comVariant
	in.setI4(1)
	var out uintptr = 0xDEAD
	if hr := comCall(r, slotRangeFindAttribute, 40005, uintptr(unsafe.Pointer(&in)), 0,
		uintptr(unsafe.Pointer(&out))); hr != sOK || out != 0 {
		t.Errorf("FindAttribute: hr=%#x out=%#x, ждал S_OK и NULL", hr, out)
	}
}

func TestUIAText_BoundingRectanglesAreElementRect(t *testing.T) {
	sc := newTextScene(t)
	e := sc.elemFor(t, sc.tb)
	r := sc.docRange(t, e)
	defer rangeRelease(r)

	var sa uintptr
	if hr := comCall(r, slotRangeBoundingRects, uintptr(unsafe.Pointer(&sa))); hr != sOK || sa == 0 {
		t.Fatalf("GetBoundingRectangles: hr=%#x", hr)
	}
	defer procSafeArrayDestroyTest.Call(sa)
	if n := safeArrayLen(t, sa); n != 4 {
		t.Fatalf("в массиве %d чисел, ждал 4 (один прямоугольник)", n)
	}
	var got [4]float64
	for i := range got {
		idx := int32(i)
		procSafeArrayGetElement.Call(sa, uintptr(unsafe.Pointer(&idx)), uintptr(unsafe.Pointer(&got[i])))
	}
	want := e.b.boundsOf(e.id)
	if got != [4]float64{want.left, want.top, want.width, want.height} || want.width == 0 {
		t.Errorf("прямоугольник %v, ждал рамку элемента %+v", got, want)
	}
}

func TestUIAText_EnclosingElementAndChildren(t *testing.T) {
	sc := newTextScene(t)
	e := sc.elemFor(t, sc.tb)
	r := sc.docRange(t, e)
	defer rangeRelease(r)

	before := e.refs.Load()
	var el uintptr
	if hr := comCall(r, slotRangeEnclosing, uintptr(unsafe.Pointer(&el))); hr != sOK || el != e.simplePtr() {
		t.Fatalf("GetEnclosingElement: hr=%#x el=%#x, ждал %#x", hr, el, e.simplePtr())
	}
	if e.refs.Load() != before+1 {
		t.Error("указатель на элемент отдан без AddRef")
	}
	comCall(el, slotRelease)

	var sa uintptr
	if hr := comCall(r, slotRangeGetChildren, uintptr(unsafe.Pointer(&sa))); hr != sOK || sa == 0 {
		t.Fatalf("GetChildren: hr=%#x", hr)
	}
	defer procSafeArrayDestroyTest.Call(sa)
	if n := safeArrayLen(t, sa); n != 0 {
		t.Errorf("у текста %d детей, ждал 0", n)
	}

	// RangeFromChild: детей нет — отказ аргумента.
	var out uintptr
	if hr := comCall(e.textPtr(), slotTextRangeFromChild, e.simplePtr(), uintptr(unsafe.Pointer(&out))); hr != eInvalidArg || out != 0 {
		t.Errorf("RangeFromChild: hr=%#x out=%#x", hr, out)
	}
}

func TestUIAText_RangeFromPointAndVisible(t *testing.T) {
	sc := newTextScene(t)
	e := sc.elemFor(t, sc.tb)
	sc.tb.SetCaretPosition(5)

	// Точка приходит структурой по ссылке (16 байт): колбэк принимает её как
	// адрес и не разыменовывает.
	pt := [2]float64{20, 60}
	var r uintptr
	if hr := comCall(e.textPtr(), slotTextRangeFromPoint, uintptr(unsafe.Pointer(&pt)), uintptr(unsafe.Pointer(&r))); hr != sOK || r == 0 {
		t.Fatalf("RangeFromPoint: hr=%#x", hr)
	}
	defer rangeRelease(r)
	if got := uiaLookupRange(r).get(); got != (a11yRange{5, 5}) {
		t.Errorf("RangeFromPoint: %+v, ждал пустой диапазон на каретке", got)
	}

	var sa uintptr
	if hr := comCall(e.textPtr(), slotTextVisibleRanges, uintptr(unsafe.Pointer(&sa))); hr != sOK || sa == 0 {
		t.Fatalf("GetVisibleRanges: hr=%#x", hr)
	}
	defer procSafeArrayDestroyTest.Call(sa)
	if n := safeArrayLen(t, sa); n != 1 {
		t.Errorf("видимых диапазонов %d, ждал 1", n)
	}
}

// ─── Select и выделение ──────────────────────────────────────────────────────

// Select выделяет диапазон в виджете — на горутине движка.
func TestUIAText_Select(t *testing.T) {
	sc := newTextScene(t)
	e := sc.elemFor(t, sc.tb)
	r := sc.docRange(t, e)
	defer rangeRelease(r)

	// Находим «строка» первой строки: диапазон [7,13).
	text, _ := syscall.UTF16PtrFromString("строка")
	bstr, _, _ := procSysAllocString.Call(uintptr(unsafe.Pointer(text)))
	defer procSysFreeString.Call(bstr)
	var found uintptr
	if hr := comCall(r, slotRangeFindText, bstr, 0, 0, uintptr(unsafe.Pointer(&found))); hr != sOK || found == 0 {
		t.Fatalf("FindText: hr=%#x", hr)
	}
	defer rangeRelease(found)

	if hr := comCall(found, slotRangeSelect); hr != sOK {
		t.Fatalf("Select: hr=%#x", hr)
	}
	sc.flush()
	if from, to := sc.tb.AccessSelection(); from != 7 || to != 13 {
		t.Errorf("выделение в виджете [%d,%d), ждал [7,13)", from, to)
	}
	if got := sc.tb.SelectedText(); got != "строка" {
		t.Errorf("выделен %q", got)
	}

	// Пустой диапазон — просто каретка: схлопываем найденное в начало.
	var moved int32
	comCall(found, slotRangeMoveEndpoint, uintptr(a11yEndEnd), uintptr(a11yUnitDocument),
		uintptr(^uintptr(0)), uintptr(unsafe.Pointer(&moved)))
	if hr := comCall(found, slotRangeSelect); hr != sOK {
		t.Fatalf("Select (каретка): hr=%#x", hr)
	}
	sc.flush()
	if from, to := sc.tb.AccessSelection(); from != to || from != 0 {
		t.Errorf("каретка [%d,%d), ждал в 0", from, to)
	}
}

// Выделение единственное: добавить можно, лишь пока его нет; снять — только
// то, что и есть выделение.
func TestUIAText_AddRemoveSelection(t *testing.T) {
	sc := newTextScene(t)
	e := sc.elemFor(t, sc.tb)
	flush := sc.flush

	r := sc.docRange(t, e)
	defer rangeRelease(r)

	sc.tb.SetCaretPosition(0)
	if hr := comCall(r, slotRangeAddToSelection); hr != sOK {
		t.Fatalf("AddToSelection без выделения: hr=%#x", hr)
	}
	flush()
	if from, to := sc.tb.AccessSelection(); from != 0 || to != 27 {
		t.Fatalf("выделение после Add: [%d,%d)", from, to)
	}
	// Теперь выделение есть: ещё один диапазон добавить некуда.
	if hr := comCall(r, slotRangeAddToSelection); hr != uiaEInvalidOperation {
		t.Errorf("второй AddToSelection: hr=%#x, ждал UIA_E_INVALIDOPERATION", hr)
	}
	// Диапазон, не совпадающий с выделением, ничего не снимает.
	other := sc.docRange(t, e)
	defer rangeRelease(other)
	var moved int32
	comCall(other, slotRangeMoveEndpoint, uintptr(a11yEndEnd), uintptr(a11yUnitWord),
		uintptr(^uintptr(0)), uintptr(unsafe.Pointer(&moved)))
	if hr := comCall(other, slotRangeRemoveFromSel); hr != sOK {
		t.Fatalf("RemoveFromSelection чужого диапазона: hr=%#x", hr)
	}
	flush()
	if from, to := sc.tb.AccessSelection(); from != 0 || to != 27 {
		t.Errorf("выделение изменилось: [%d,%d)", from, to)
	}
	// Совпадающий — снимает.
	if hr := comCall(r, slotRangeRemoveFromSel); hr != sOK {
		t.Fatalf("RemoveFromSelection: hr=%#x", hr)
	}
	flush()
	if from, to := sc.tb.AccessSelection(); from != to {
		t.Errorf("выделение не снято: [%d,%d)", from, to)
	}
	if hr := comCall(r, slotRangeScrollIntoView, 1); hr != sOK {
		t.Errorf("ScrollIntoView: hr=%#x", hr)
	}
}

// Диапазон переживает правку текста: позиции усекаются, а не роняют провайдер.
func TestUIAText_RangeSurvivesTextChange(t *testing.T) {
	sc := newTextScene(t)
	r := sc.docRange(t, sc.elemFor(t, sc.tb))
	defer rangeRelease(r)

	sc.tb.SetText("кратко")
	if got := rangeText(t, r); got != "кратко" {
		t.Errorf("после правки: %q", got)
	}
	sc.tb.SetText("")
	if got := rangeText(t, r); got != "" {
		t.Errorf("после очистки: %q", got)
	}
}

// ─── Время жизни диапазонов ──────────────────────────────────────────────────

// Release до нуля НАСТОЯЩИЙ: тысячи диапазонов, которые создаёт скринридер,
// не копятся до закрытия окна.
func TestUIAText_RangeReleaseFrees(t *testing.T) {
	sc := newTextScene(t)
	e := sc.elemFor(t, sc.tb)
	base := uiaLiveRanges()

	r := sc.docRange(t, e)
	tr0 := uiaLookupRange(r)
	if got := uiaLiveRanges(); got != base+1 {
		t.Fatalf("после создания зарегистрировано %d, ждал %d", got, base+1)
	}
	if n := comCall(r, slotAddRef); n != 2 {
		t.Errorf("AddRef вернул %d, ждал 2", n)
	}
	if n := comCall(r, slotRelease); n != 1 {
		t.Errorf("Release вернул %d, ждал 1", n)
	}
	if got := uiaLiveRanges(); got != base+1 {
		t.Errorf("объект снят при живой ссылке: %d", got)
	}
	if n := comCall(r, slotRelease); n != 0 {
		t.Errorf("последний Release вернул %d", n)
	}
	if got := uiaLiveRanges(); got != base {
		t.Fatalf("после последнего Release осталось %d, ждал %d", got, base)
	}

	// Вызов на освобождённом указателе — ошибка клиента — не должен трогать
	// память: объект не найдётся, ответ — отказ. Go-ссылку держим, чтобы
	// сборщик не забрал память под ногами у самого теста (comCall читает
	// vtable через этот адрес).
	defer runtime.KeepAlive(tr0)
	var bstr uintptr
	if hr := comCall(r, slotRangeGetText, uintptr(^uintptr(0)), uintptr(unsafe.Pointer(&bstr))); hr != eFail {
		t.Errorf("GetText на освобождённом: hr=%#x, ждал E_FAIL", hr)
	}
	// Повторные Release и AddRef не воскрешают и не падают.
	if n := comCall(r, slotRelease); n != 0 {
		t.Errorf("повторный Release: %d", n)
	}
	if n := comCall(r, slotAddRef); n != 0 {
		t.Errorf("AddRef на мёртвом: %d", n)
	}
	if got := uiaLiveRanges(); got != base {
		t.Errorf("мёртвый объект вернулся в реестр: %d", got)
	}

	// Тысячи диапазонов — как у скринридера, читающего документ: реестр
	// возвращается к исходному размеру.
	for i := 0; i < 2000; i++ {
		x := sc.docRange(t, e)
		var c uintptr
		comCall(x, slotRangeClone, uintptr(unsafe.Pointer(&c)))
		rangeExpand(t, c, int32(a11yUnitWord))
		comCall(c, slotRelease)
		comCall(x, slotRelease)
	}
	if got := uiaLiveRanges(); got != base {
		t.Errorf("после 2000 циклов зарегистрировано %d, ждал %d — течёт", got, base)
	}
}

// QueryInterface диапазона: свои интерфейсы с AddRef, чужие — отказ.
func TestUIAText_RangeQueryInterface(t *testing.T) {
	sc := newTextScene(t)
	r := sc.docRange(t, sc.elemFor(t, sc.tb))
	defer rangeRelease(r)

	for _, iid := range []*comGUID{&iidIUnknown, &iidTextRangeProvider} {
		var out uintptr
		if hr := comCall(r, slotQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out))); hr != sOK || out != r {
			t.Errorf("QI(%s): hr=%#x out=%#x", iid, hr, out)
		}
		comCall(r, slotRelease)
	}
	var out uintptr
	if hr := comCall(r, slotQueryInterface, uintptr(unsafe.Pointer(&iidTextProvider)), uintptr(unsafe.Pointer(&out))); hr != eNoInterface || out != 0 {
		t.Errorf("QI(чужой): hr=%#x out=%#x", hr, out)
	}
}

// Массив диапазонов владеет ими: пока массив жив, диапазоны живы, а его
// уничтожение отпускает их — иначе каждый вызов GetSelection тек бы.
func TestUIAText_SafeArrayOwnsRanges(t *testing.T) {
	sc := newTextScene(t)
	e := sc.elemFor(t, sc.tb)
	base := uiaLiveRanges()

	var sa uintptr
	if hr := comCall(e.textPtr(), slotTextGetSelection, uintptr(unsafe.Pointer(&sa))); hr != sOK {
		t.Fatalf("GetSelection: hr=%#x", hr)
	}
	if got := uiaLiveRanges(); got != base+1 {
		t.Errorf("с живым массивом зарегистрировано %d, ждал %d", got, base+1)
	}
	procSafeArrayDestroyTest.Call(sa)
	if got := uiaLiveRanges(); got != base {
		t.Errorf("после уничтожения массива осталось %d, ждал %d", got, base)
	}
}

// Остановка моста снимает диапазоны, которые клиент не отпустил.
func TestUIAText_BridgeStopForgetsRanges(t *testing.T) {
	sc := newTextScene(t)
	e := sc.elemFor(t, sc.tb)
	base := uiaLiveRanges()
	_ = sc.docRange(t, e)
	_ = sc.docRange(t, e)
	if got := uiaLiveRanges(); got != base+2 {
		t.Fatalf("зарегистрировано %d, ждал %d", got, base+2)
	}
	uiaForgetRangesOf(sc.b)
	if got := uiaLiveRanges(); got != base {
		t.Errorf("после остановки моста осталось %d, ждал %d", got, base)
	}
}

// ─── События ─────────────────────────────────────────────────────────────────

// Состояние текста запоминается между проходами: без этого нельзя понять,
// что изменилось. Сами события уходят клиентам UIA, которых в тесте нет, —
// проверяем учёт и отсутствие падений.
func TestUIAText_EmitTextEventsTracksState(t *testing.T) {
	sc := newTextScene(t) // фокус в поле ввода (см. newUIAValueScene)
	sc.b.markDirty()
	sc.b.refresh(true)

	sc.b.emitTextEvents() // первое обращение: только запоминаем
	if sc.b.textSeen == nil || sc.b.textSeen.Text != "привет" {
		t.Fatalf("состояние не запомнено: %+v", sc.b.textSeen)
	}
	sc.ti.AccessSetCaret(2)
	sc.b.emitTextEvents()
	if sc.b.textSeen.Caret != 2 {
		t.Errorf("каретка не обновлена: %+v", sc.b.textSeen)
	}
	sc.ti.SetText("новое")
	sc.b.emitTextEvents()
	if sc.b.textSeen.Text != "новое" {
		t.Errorf("текст не обновлён: %+v", sc.b.textSeen)
	}
}
