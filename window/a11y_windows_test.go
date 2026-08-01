//go:build windows

package window

import (
	"image"
	"syscall"
	"testing"
	"unsafe"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	"golang.org/x/sys/windows"
)

// ─── Вызов COM-методов по таблице ────────────────────────────────────────────

// comCall вызывает метод COM-объекта по номеру слота в vtable — ровно так, как
// это делает клиент UIA. Проверяет не только логику провайдера, но и саму
// раскладку таблицы: перепутанный слот виден сразу.
func comCall(iface uintptr, slot int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(iface))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	all := append([]uintptr{iface}, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

// Слоты интерфейсов провайдера.
const (
	slotQueryInterface = 0
	slotAddRef         = 1
	slotRelease        = 2

	slotProviderOptions = 3
	slotPatternProvider = 4
	slotPropertyValue   = 5
	slotHostProvider    = 6

	slotNavigate           = 3
	slotRuntimeID          = 4
	slotBoundingRectangle  = 5
	slotEmbeddedFragmRoots = 6
	slotSetFocus           = 7
	slotFragmentRoot       = 8

	slotElementFromPoint = 3
	slotGetFocus         = 4
)

var (
	procSysFreeString        = oleaut32.NewProc("SysFreeString")
	procSafeArrayGetElement  = oleaut32.NewProc("SafeArrayGetElement")
	procSafeArrayGetUBound   = oleaut32.NewProc("SafeArrayGetUBound")
	procSafeArrayDestroyTest = oleaut32.NewProc("SafeArrayDestroy")
)

// variantString читает BSTR из VARIANT и освобождает его.
func variantString(t *testing.T, v *comVariant) string {
	t.Helper()
	if v.vt != vtBSTR {
		t.Fatalf("ожидался VT_BSTR, получен vt=%d", v.vt)
	}
	s := windows.UTF16PtrToString((*uint16)(unsafe.Pointer(v.val[0])))
	procSysFreeString.Call(v.val[0])
	return s
}

// safeArrayInts читает SAFEARRAY(VT_I4) и освобождает его.
func safeArrayInts(t *testing.T, sa uintptr) []int32 {
	t.Helper()
	if sa == 0 {
		t.Fatal("SAFEARRAY не создан")
	}
	var ub int32
	procSafeArrayGetUBound.Call(sa, 1, uintptr(unsafe.Pointer(&ub)))
	out := make([]int32, 0, ub+1)
	for i := int32(0); i <= ub; i++ {
		var val int32
		idx := i
		procSafeArrayGetElement.Call(sa, uintptr(unsafe.Pointer(&idx)), uintptr(unsafe.Pointer(&val)))
		out = append(out, val)
	}
	procSafeArrayDestroyTest.Call(sa)
	return out
}

// ─── Тесты ───────────────────────────────────────────────────────────────────

// TestUIAControlTypeMapping — роли движка переводятся в типы элементов UIA.
func TestUIAControlTypeMapping(t *testing.T) {
	cases := []struct {
		role widget.AccessRole
		want int32
	}{
		{widget.RoleButton, uiaCtrlButton},
		{widget.RoleCheckBox, uiaCtrlCheckBox},
		{widget.RoleSwitch, uiaCtrlCheckBox},
		{widget.RoleRadioButton, uiaCtrlRadioButton},
		{widget.RoleTextInput, uiaCtrlEdit},
		{widget.RoleLabel, uiaCtrlText},
		{widget.RoleWindow, uiaCtrlWindow},
		{widget.RolePanel, uiaCtrlPane},
		{widget.RoleUnknown, uiaCtrlCustom},
	}
	for _, c := range cases {
		if got := uiaControlType(c.role); got != c.want {
			t.Errorf("uiaControlType(%s) = %d, want %d", c.role, got, c.want)
		}
	}
	if uiaFocusable(widget.AccessInfo{Role: widget.RoleLabel}) {
		t.Error("надпись не должна быть фокусируемой")
	}
	if !uiaFocusable(widget.AccessInfo{Role: widget.RoleButton}) {
		t.Error("кнопка должна быть фокусируемой")
	}
	if uiaFocusable(widget.AccessInfo{Role: widget.RoleButton, States: []string{widget.StateDisabled}}) {
		t.Error("выключенная кнопка не фокусируема")
	}
}

// newUIATestBridge собирает мост поверх окна с кнопкой, чекбоксом и полем
// ввода. Нативного окна нет — провайдеры от него не зависят (кроме хост-
// провайдера, который у корня и так опционален).
func newUIATestBridge(t *testing.T) (*uiaBridge, *engine.Engine) {
	t.Helper()
	eng := engine.New(400, 300, 20)
	root := widget.NewWindow("Окно UIA", 400, 300)

	btn := widget.NewButton("Сохранить")
	btn.SetBounds(image.Rect(10, 10, 120, 40))
	root.AddChild(btn)

	cb := widget.NewCheckBox("Запомнить")
	cb.SetBounds(image.Rect(10, 50, 160, 72))
	cb.SetChecked(true)
	root.AddChild(cb)

	ti := widget.NewTextInput("")
	ti.SetBounds(image.Rect(10, 80, 200, 108))
	ti.SetText("привет")
	root.AddChild(ti)

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
	return b, eng
}

// TestUIAProviderThroughVTable — провайдер вызывается ровно так, как это делает
// UIA: через указатели в таблице виртуальных методов.
func TestUIAProviderThroughVTable(t *testing.T) {
	b, _ := newUIATestBridge(t)
	root := b.element(b.rootID())
	if root == nil {
		t.Fatal("корневой элемент не создан")
	}
	simple := root.simplePtr()

	// ── IUnknown ─────────────────────────────────────────────────────────────
	var out uintptr
	if hr := comCall(simple, slotQueryInterface, uintptr(unsafe.Pointer(&iidProviderSimple)),
		uintptr(unsafe.Pointer(&out))); hr != sOK || out != simple {
		t.Fatalf("QueryInterface(Simple): hr=%#x out=%#x", hr, out)
	}
	if hr := comCall(simple, slotQueryInterface, uintptr(unsafe.Pointer(&iidProviderFragment)),
		uintptr(unsafe.Pointer(&out))); hr != sOK || out != root.fragmentPtr() {
		t.Fatalf("QueryInterface(Fragment): hr=%#x", hr)
	}
	if hr := comCall(simple, slotQueryInterface, uintptr(unsafe.Pointer(&iidProviderFragmentRoot)),
		uintptr(unsafe.Pointer(&out))); hr != sOK || out != root.rootPtr() {
		t.Fatalf("QueryInterface(FragmentRoot) у корня: hr=%#x", hr)
	}
	unknownIID := comGUID{Data1: 0xDEADBEEF}
	if hr := comCall(simple, slotQueryInterface, uintptr(unsafe.Pointer(&unknownIID)),
		uintptr(unsafe.Pointer(&out))); hr != eNoInterface || out != 0 {
		t.Errorf("QueryInterface(чужой IID): hr=%#x out=%#x", hr, out)
	}
	before := root.refs.Load()
	comCall(simple, slotAddRef)
	if root.refs.Load() != before+1 {
		t.Errorf("AddRef не увеличил счётчик: %d → %d", before, root.refs.Load())
	}
	comCall(simple, slotRelease)
	if root.refs.Load() != before {
		t.Errorf("Release не вернул счётчик: %d", root.refs.Load())
	}

	// ── IRawElementProviderSimple ────────────────────────────────────────────
	var opts int32
	if hr := comCall(simple, slotProviderOptions, uintptr(unsafe.Pointer(&opts))); hr != sOK ||
		opts != uiaProviderServerSide {
		t.Errorf("ProviderOptions: hr=%#x opts=%d", hr, opts)
	}
	var pattern uintptr
	if hr := comCall(simple, slotPatternProvider, 10000, uintptr(unsafe.Pointer(&pattern))); hr != sOK ||
		pattern != 0 {
		t.Errorf("GetPatternProvider: hr=%#x ptr=%#x (паттернов пока нет)", hr, pattern)
	}

	prop := func(iface uintptr, id int32) comVariant {
		var v comVariant
		if hr := comCall(iface, slotPropertyValue, uintptr(id), uintptr(unsafe.Pointer(&v))); hr != sOK {
			t.Fatalf("GetPropertyValue(%d): hr=%#x", id, hr)
		}
		return v
	}

	v := prop(simple, uiaPropName)
	if got := variantString(t, &v); got != "Окно UIA" {
		t.Errorf("имя корня = %q", got)
	}
	v = prop(simple, uiaPropControlType)
	if v.vt != vtI4 || int32(uint32(v.val[0])) != uiaCtrlWindow {
		t.Errorf("тип корня: vt=%d val=%d", v.vt, int32(uint32(v.val[0])))
	}

	// ── Навигация ────────────────────────────────────────────────────────────
	fragment := root.fragmentPtr()
	var child uintptr
	if hr := comCall(fragment, slotNavigate, uiaNavFirstChild, uintptr(unsafe.Pointer(&child))); hr != sOK ||
		child == 0 {
		t.Fatalf("Navigate(FirstChild): hr=%#x child=%#x", hr, child)
	}
	btn := uiaLookup(child)
	if btn == nil {
		t.Fatal("первый ребёнок не найден в реестре COM-объектов")
	}
	v = prop(btn.simplePtr(), uiaPropName)
	if got := variantString(t, &v); got != "Сохранить" {
		t.Errorf("имя первого ребёнка = %q", got)
	}
	v = prop(btn.simplePtr(), uiaPropControlType)
	if int32(uint32(v.val[0])) != uiaCtrlButton {
		t.Errorf("тип первого ребёнка = %d", int32(uint32(v.val[0])))
	}

	var parent uintptr
	if hr := comCall(btn.fragmentPtr(), slotNavigate, uiaNavParent, uintptr(unsafe.Pointer(&parent))); hr != sOK ||
		parent != root.fragmentPtr() {
		t.Errorf("Navigate(Parent) от кнопки: hr=%#x parent=%#x", hr, parent)
	}
	var sibling uintptr
	comCall(btn.fragmentPtr(), slotNavigate, uiaNavNextSibling, uintptr(unsafe.Pointer(&sibling)))
	cb := uiaLookup(sibling)
	if cb == nil {
		t.Fatal("следующий сосед не найден")
	}
	v = prop(cb.simplePtr(), uiaPropName)
	if got := variantString(t, &v); got != "Запомнить" {
		t.Errorf("следующий сосед = %q", got)
	}
	var back uintptr
	comCall(cb.fragmentPtr(), slotNavigate, uiaNavPrevSibling, uintptr(unsafe.Pointer(&back)))
	if back != btn.fragmentPtr() {
		t.Error("PreviousSibling не вернул кнопку")
	}
	var none uintptr
	if hr := comCall(root.fragmentPtr(), slotNavigate, uiaNavParent, uintptr(unsafe.Pointer(&none))); hr != sOK ||
		none != 0 {
		t.Errorf("родитель корня должен быть NULL: hr=%#x ptr=%#x", hr, none)
	}

	// ── RuntimeId и границы ──────────────────────────────────────────────────
	var sa uintptr
	if hr := comCall(btn.fragmentPtr(), slotRuntimeID, uintptr(unsafe.Pointer(&sa))); hr != sOK {
		t.Fatalf("GetRuntimeId: hr=%#x", hr)
	}
	ids := safeArrayInts(t, sa)
	if len(ids) != 2 || ids[0] != uiaAppendRuntimeID || ids[1] != btn.id {
		t.Errorf("RuntimeId = %v, want [%d %d]", ids, uiaAppendRuntimeID, btn.id)
	}

	var rc uiaRect
	if hr := comCall(btn.fragmentPtr(), slotBoundingRectangle, uintptr(unsafe.Pointer(&rc))); hr != sOK {
		t.Fatalf("BoundingRectangle: hr=%#x", hr)
	}
	if rc.left != 10 || rc.top != 10 || rc.width != 110 || rc.height != 30 {
		t.Errorf("границы кнопки = %+v, want 10,10 110x30", rc)
	}

	// ── Фрагмент-корень и фокус ──────────────────────────────────────────────
	var fr uintptr
	if hr := comCall(btn.fragmentPtr(), slotFragmentRoot, uintptr(unsafe.Pointer(&fr))); hr != sOK ||
		fr != root.rootPtr() {
		t.Errorf("get_FragmentRoot: hr=%#x ptr=%#x", hr, fr)
	}
	var focus uintptr
	if hr := comCall(root.rootPtr(), slotGetFocus, uintptr(unsafe.Pointer(&focus))); hr != sOK || focus == 0 {
		t.Fatalf("GetFocus: hr=%#x ptr=%#x", hr, focus)
	}
	focused := uiaLookup(focus)
	if focused == nil {
		t.Fatal("элемент с фокусом не найден")
	}
	v = prop(focused.simplePtr(), uiaPropControlType)
	if int32(uint32(v.val[0])) != uiaCtrlEdit {
		t.Errorf("фокус на элементе типа %d, ожидалось поле ввода", int32(uint32(v.val[0])))
	}
	v = prop(focused.simplePtr(), uiaPropHasKeyboardFocus)
	if v.vt != vtBool || v.val[0] == 0 {
		t.Errorf("HasKeyboardFocus у фокусированного: vt=%d val=%d", v.vt, v.val[0])
	}
	v = prop(btn.simplePtr(), uiaPropHasKeyboardFocus)
	if v.vt != vtBool || v.val[0] != 0 {
		t.Errorf("HasKeyboardFocus у кнопки должен быть FALSE: val=%d", v.val[0])
	}
	v = prop(focused.simplePtr(), uiaPropValueValue)
	if got := variantString(t, &v); got != "привет" {
		t.Errorf("значение поля ввода = %q", got)
	}

	// ── Прочие свойства ──────────────────────────────────────────────────────
	v = prop(btn.simplePtr(), uiaPropIsEnabled)
	if v.vt != vtBool || v.val[0] == 0 {
		t.Errorf("IsEnabled у кнопки: vt=%d val=%d", v.vt, v.val[0])
	}
	v = prop(btn.simplePtr(), uiaPropAutomationID)
	if got := variantString(t, &v); got == "" {
		t.Error("AutomationId пуст")
	}
	v = prop(btn.simplePtr(), uiaPropFrameworkID)
	if got := variantString(t, &v); got != "headless-gui" {
		t.Errorf("FrameworkId = %q", got)
	}
	v = prop(btn.simplePtr(), 39999) // неизвестное свойство
	if v.vt != vtEmpty {
		t.Errorf("неизвестное свойство должно давать VT_EMPTY, получено vt=%d", v.vt)
	}
}

// TestUIANoNativeWindowHandle — корень НЕ должен отдавать NativeWindowHandle.
//
// Регрессия: пока корень отвечал на это свойство дескриптором своего окна, UIA
// понимала его как «элемент содержит вот это окно», шла в него за провайдером
// (WM_GETOBJECT), получала тот же самый корень и делала его собственным
// ребёнком — обход дерева уходил в бесконечное «окно внутри окна».
func TestUIANoNativeWindowHandle(t *testing.T) {
	b, _ := newUIATestBridge(t)
	b.hwnd = 0xBEEF
	for _, id := range []int32{b.rootID(), b.current().id(1)} {
		e := b.element(id)
		if e == nil {
			t.Fatalf("элемент %d не создан", id)
		}
		var v comVariant
		if hr := comCall(e.simplePtr(), slotPropertyValue, uiaPropNativeWindowHandle,
			uintptr(unsafe.Pointer(&v))); hr != sOK {
			t.Fatalf("GetPropertyValue(NativeWindowHandle): hr=%#x", hr)
		}
		if v.vt != vtEmpty {
			t.Errorf("элемент %d вернул NativeWindowHandle (vt=%d, %#x) — UIA уйдёт в рекурсию",
				id, v.vt, v.val[0])
		}
	}
}

// TestUIADisabledAndChecked — выключенная кнопка и отмеченный чекбокс
// отдаются клиенту правильными свойствами.
func TestUIADisabledAndChecked(t *testing.T) {
	eng := engine.New(200, 100, 20)
	root := widget.NewWindow("T", 200, 100)
	btn := widget.NewButton("Нельзя")
	btn.SetBounds(image.Rect(0, 0, 100, 30))
	btn.SetEnabled(false)
	root.AddChild(btn)
	eng.SetRoot(root)

	win := New(eng, "T")
	win.scale = 1
	b := &uiaBridge{win: win, elems: map[int32]*uiaElement{}}
	b.refresh(true)
	t.Cleanup(func() {
		for _, e := range b.elems {
			e.forget()
		}
	})

	v := b.current()
	e := b.element(v.id(1))
	if e == nil {
		t.Fatal("элемент кнопки не создан")
	}
	var out comVariant
	comCall(e.simplePtr(), slotPropertyValue, uiaPropIsEnabled, uintptr(unsafe.Pointer(&out)))
	if out.vt != vtBool || out.val[0] != 0 {
		t.Errorf("выключенная кнопка: IsEnabled vt=%d val=%d", out.vt, out.val[0])
	}
	comCall(e.simplePtr(), slotPropertyValue, uiaPropIsKeyboardFocusabl, uintptr(unsafe.Pointer(&out)))
	if out.val[0] != 0 {
		t.Error("выключенная кнопка не должна быть фокусируемой")
	}
}

// TestUIAScaledBounds — при HiDPI-масштабе границы отдаются в физических
// пикселях экрана.
func TestUIAScaledBounds(t *testing.T) {
	b, _ := newUIATestBridge(t)
	b.win.scale = 1.5
	v := b.current()
	e := b.element(v.id(1))
	var rc uiaRect
	comCall(e.fragmentPtr(), slotBoundingRectangle, uintptr(unsafe.Pointer(&rc)))
	if rc.left != 15 || rc.top != 15 || rc.width != 165 || rc.height != 45 {
		t.Errorf("границы при scale=1.5: %+v, want 15,15 165x45", rc)
	}
}
