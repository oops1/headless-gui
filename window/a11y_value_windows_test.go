//go:build windows

package window

import (
	"image"
	"syscall"
	"testing"
	"unsafe"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Содержимое поля ввода доезжало до скринридера одной строкой — «значением»
// элемента, как у ползунка. Для кнопки этого довольно, для поля ввода — нет:
// NVDA и «Экранный диктор» читают текст через паттерн Value, и без него поле
// объявлялось пустым.

// Слоты IValueProvider в таблице виртуальных методов: три метода IUnknown,
// затем SetValue, get_Value, get_IsReadOnly.
const (
	slotValueSet        = 3
	slotValueGet        = 4
	slotValueIsReadOnly = 5
)

// uiaValueScene — окно с полем ввода и многострочным текстом.
type uiaValueScene struct {
	b  *uiaBridge
	ti *widget.TextInput
	tb *widget.TextBox
}

func newUIAValueScene(t *testing.T) *uiaValueScene {
	t.Helper()
	root := widget.NewWindow("Текст", 320, 200)
	root.SetBounds(image.Rect(0, 0, 320, 200))

	ti := widget.NewTextInput("")
	ti.SetBounds(image.Rect(10, 10, 300, 40))
	ti.SetText("привет")
	root.AddChild(ti)

	tb := widget.NewTextBox("")
	tb.SetBounds(image.Rect(10, 50, 300, 180))
	tb.SetText("первая строка\nвторая строка")
	root.AddChild(tb)

	eng := engine.New(320, 200, 30)
	eng.SetRoot(root)
	eng.SetFocus(ti)

	win := New(eng, "Окно UIA")
	win.scale = 1
	b := &uiaBridge{win: win, elems: map[int32]*uiaElement{}}
	b.refresh(true)
	t.Cleanup(func() {
		b.mu.Lock()
		for _, e := range b.elems {
			e.forget()
		}
		b.mu.Unlock()
	})
	return &uiaValueScene{b: b, ti: ti, tb: tb}
}

// elemFor находит элемент моста по виджету.
func (sc *uiaValueScene) elemFor(t *testing.T, w widget.Widget) *uiaElement {
	t.Helper()
	v := sc.b.current()
	for i := range v.Snap.Nodes {
		if v.Snap.Nodes[i].Widget == w {
			return sc.b.element(v.id(int32(i)))
		}
	}
	t.Fatal("элемент виджета не найден")
	return nil
}

// valueOf читает Value через настоящую таблицу виртуальных методов — ровно
// так, как это делает UIA.
func valueOf(t *testing.T, e *uiaElement) string {
	t.Helper()
	var bstr uintptr
	if hr := comCall(e.valuePtr(), slotValueGet, uintptr(unsafe.Pointer(&bstr))); hr != sOK {
		t.Fatalf("get_Value: hr=%#x", hr)
	}
	defer procSysFreeString.Call(bstr)
	return bstrToString(bstr)
}

func TestUIAValue_ReadsText(t *testing.T) {
	sc := newUIAValueScene(t)

	if got := valueOf(t, sc.elemFor(t, sc.ti)); got != "привет" {
		t.Errorf("поле ввода читается как %q", got)
	}
	if got := valueOf(t, sc.elemFor(t, sc.tb)); got != "первая строка\nвторая строка" {
		t.Errorf("многострочный текст читается как %q", got)
	}
}

// Текст спрашивается у ЖИВОГО виджета: снимок семантики пересобирается раз в
// сто пятьдесят миллисекунд и отставал бы от набора.
func TestUIAValue_FollowsWidget(t *testing.T) {
	sc := newUIAValueScene(t)
	e := sc.elemFor(t, sc.ti)

	sc.ti.SetText("новое")
	if got := valueOf(t, e); got != "новое" {
		t.Errorf("прочитано %q — отдан устаревший снимок", got)
	}
}

func TestUIAValue_ReadOnly(t *testing.T) {
	sc := newUIAValueScene(t)

	readOnly := func(e *uiaElement) int32 {
		var ro int32
		if hr := comCall(e.valuePtr(), slotValueIsReadOnly, uintptr(unsafe.Pointer(&ro))); hr != sOK {
			t.Fatalf("get_IsReadOnly: hr=%#x", hr)
		}
		return ro
	}

	if ro := readOnly(sc.elemFor(t, sc.tb)); ro != 0 {
		t.Error("обычное поле объявлено доступным только для чтения")
	}
	sc.tb.ReadOnly = true
	if ro := readOnly(sc.elemFor(t, sc.tb)); ro != 1 {
		t.Error("поле только для чтения объявлено редактируемым")
	}
}

// Паттерн объявляется только там, где есть текст: пустой провайдер на кнопке
// заставил бы скринридер искать в ней содержимое.
func TestUIAValue_PatternByRole(t *testing.T) {
	sc := newUIAValueScene(t)
	e := sc.elemFor(t, sc.ti)

	var p uintptr
	if hr := comCall(e.simplePtr(), slotPatternProvider, uintptr(uiaPatternValue),
		uintptr(unsafe.Pointer(&p))); hr != sOK {
		t.Fatalf("GetPatternProvider: hr=%#x", hr)
	}
	if p != e.valuePtr() {
		t.Errorf("Value у поля ввода = %#x, ожидался %#x", p, e.valuePtr())
	}

	// QueryInterface обязан отвечать так же, как GetPatternProvider.
	var out uintptr
	if hr := comCall(e.simplePtr(), slotQueryInterface, uintptr(unsafe.Pointer(&iidValueProvider)),
		uintptr(unsafe.Pointer(&out))); hr != sOK || out != e.valuePtr() {
		t.Errorf("QI(Value) у поля ввода: hr=%#x out=%#x", hr, out)
	}
}

// Многострочный текст объявляется документом, однострочное поле — полем
// ввода: по этому различию скринридер выбирает, как его читать.
func TestUIAValue_DocumentControlType(t *testing.T) {
	if got := uiaControlType(widget.RoleDocument); got != uiaCtrlDocument {
		t.Errorf("тип документа %d, ждал %d", got, uiaCtrlDocument)
	}
	if got := uiaControlType(widget.RoleTextInput); got != uiaCtrlEdit {
		t.Errorf("тип поля ввода %d", got)
	}
}

// Поле только для чтения правке не поддаётся: скринридер должен получить
// отказ, а не тихое «готово».
func TestUIAValue_SetValueRefusedWhenReadOnly(t *testing.T) {
	sc := newUIAValueScene(t)
	sc.tb.ReadOnly = true
	e := sc.elemFor(t, sc.tb)

	text, _ := syscall.UTF16PtrFromString("правка")
	bstr, _, _ := procSysAllocString.Call(uintptr(unsafe.Pointer(text)))
	defer procSysFreeString.Call(bstr)

	if hr := comCall(e.valuePtr(), slotValueSet, bstr); hr != uiaEElementNotEnabled {
		t.Errorf("SetValue в поле только для чтения: hr=%#x, ждал E_ELEMENTNOTENABLED", hr)
	}
}
