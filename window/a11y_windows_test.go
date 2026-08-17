//go:build windows

package window

import (
	"image"
	"image/color"
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
//
// Объекты тестов живут в Go-куче — checkptr здесь неприменим.
//
//go:nocheckptr
func comCall(iface uintptr, slot int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(unsafe.Pointer(iface))
	fn := *(*uintptr)(unsafe.Add(vtbl, slot*int(unsafe.Sizeof(uintptr(0)))))
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

	slotInvoke      = 3 // IInvokeProvider::Invoke
	slotToggle      = 3 // IToggleProvider::Toggle
	slotToggleState = 4 // IToggleProvider::get_ToggleState
)

var (
	procSafeArrayGetElement  = oleaut32.NewProc("SafeArrayGetElement")
	procSafeArrayGetUBound   = oleaut32.NewProc("SafeArrayGetUBound")
	procSafeArrayDestroyTest = oleaut32.NewProc("SafeArrayDestroy")
)

// variantString читает BSTR из VARIANT и освобождает его.
//
//go:nocheckptr
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
	if hr := comCall(simple, slotPatternProvider, uiaPatternInvoke, uintptr(unsafe.Pointer(&pattern))); hr != sOK ||
		pattern != 0 {
		t.Errorf("GetPatternProvider(Invoke) у окна: hr=%#x ptr=%#x (окно не нажимается)", hr, pattern)
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

// ─── Паттерны управления ─────────────────────────────────────────────────────

// uiaActionScene — сцена для проверки Invoke/Toggle/SetFocus.
type uiaActionScene struct {
	b      *uiaBridge
	eng    *engine.Engine
	btn    *widget.Button
	cb     *widget.CheckBox
	lbl    *widget.Label
	clicks int
}

// newUIAActionScene собирает мост поверх кнопки, флажка и надписи.
//
// Два требования к раскладке, оба обязательны: виджеты НЕ перекрываются и лежат
// НИЖЕ полосы заголовка (32 px у widget.Window). Активация выполняется
// синтетическим кликом по центру границ через настоящий путь ввода — клик в
// заголовок перехватил бы сам widget.Window (перетаскивание), а перекрытый
// виджет получил бы чужое нажатие.
func newUIAActionScene(t *testing.T) *uiaActionScene {
	t.Helper()
	sc := &uiaActionScene{}
	sc.eng = engine.New(400, 300, 20)
	root := widget.NewWindow("Действия UIA", 400, 300)

	sc.btn = widget.NewButton("Сохранить")
	sc.btn.SetBounds(image.Rect(10, 50, 120, 80))
	sc.btn.OnClick = func() { sc.clicks++ }
	root.AddChild(sc.btn)

	sc.cb = widget.NewCheckBox("Запомнить")
	sc.cb.SetBounds(image.Rect(10, 100, 160, 130))
	root.AddChild(sc.cb)

	sc.lbl = widget.NewLabel("Подпись", color.RGBA{A: 255})
	sc.lbl.SetBounds(image.Rect(10, 150, 160, 170))
	root.AddChild(sc.lbl)

	sc.eng.SetRoot(root)

	win := New(sc.eng, "Действия UIA")
	win.scale = 1
	sc.b = &uiaBridge{win: win, elems: map[int32]*uiaElement{}}
	sc.b.refresh(true)
	t.Cleanup(func() {
		sc.b.mu.Lock()
		for _, e := range sc.b.elems {
			e.forget()
		}
		sc.b.mu.Unlock()
	})
	return sc
}

// elem возвращает COM-объект узла по его индексу в снимке (1 — кнопка,
// 2 — флажок, 3 — надпись).
func (sc *uiaActionScene) elem(t *testing.T, idx int32) *uiaElement {
	t.Helper()
	e := sc.b.element(sc.b.current().id(idx))
	if e == nil {
		t.Fatalf("элемент с индексом %d не создан", idx)
	}
	return e
}

// TestUIAPatternProviderByRole — GetPatternProvider отдаёт паттерн только тем
// ролям, которые его реально поддерживают: надпись нельзя ни нажать, ни
// переключить, кнопку нельзя переключить, флажок нажимается через Toggle.
func TestUIAPatternProviderByRole(t *testing.T) {
	sc := newUIAActionScene(t)
	btn, cb, lbl := sc.elem(t, 1), sc.elem(t, 2), sc.elem(t, 3)

	pattern := func(e *uiaElement, id int32) uintptr {
		t.Helper()
		var p uintptr
		if hr := comCall(e.simplePtr(), slotPatternProvider, uintptr(id),
			uintptr(unsafe.Pointer(&p))); hr != sOK {
			t.Fatalf("GetPatternProvider(%d): hr=%#x", id, hr)
		}
		return p
	}

	if got := pattern(btn, uiaPatternInvoke); got != btn.invokePtr() {
		t.Errorf("Invoke у кнопки = %#x, ожидался %#x", got, btn.invokePtr())
	}
	if got := pattern(btn, uiaPatternToggle); got != 0 {
		t.Errorf("Toggle у кнопки = %#x, ожидался NULL", got)
	}
	if got := pattern(cb, uiaPatternToggle); got != cb.togglePtr() {
		t.Errorf("Toggle у флажка = %#x, ожидался %#x", got, cb.togglePtr())
	}
	if got := pattern(cb, uiaPatternInvoke); got != 0 {
		t.Errorf("Invoke у флажка = %#x, ожидался NULL (у него Toggle)", got)
	}
	if got := pattern(lbl, uiaPatternInvoke); got != 0 {
		t.Errorf("Invoke у надписи = %#x, ожидался NULL", got)
	}
	if got := pattern(lbl, uiaPatternToggle); got != 0 {
		t.Errorf("Toggle у надписи = %#x, ожидался NULL", got)
	}
	if got := pattern(btn, 10002); got != 0 { // ExpandCollapse — не реализован
		t.Errorf("неизвестный паттерн = %#x, ожидался NULL", got)
	}

	// QueryInterface обязан отвечать так же, как GetPatternProvider: иначе
	// клиент, получивший паттерн одним путём, не найдёт его другим.
	var out uintptr
	if hr := comCall(btn.simplePtr(), slotQueryInterface, uintptr(unsafe.Pointer(&iidInvokeProvider)),
		uintptr(unsafe.Pointer(&out))); hr != sOK || out != btn.invokePtr() {
		t.Errorf("QI(Invoke) у кнопки: hr=%#x out=%#x", hr, out)
	}
	if hr := comCall(lbl.simplePtr(), slotQueryInterface, uintptr(unsafe.Pointer(&iidInvokeProvider)),
		uintptr(unsafe.Pointer(&out))); hr != eNoInterface || out != 0 {
		t.Errorf("QI(Invoke) у надписи: hr=%#x out=%#x, ожидался E_NOINTERFACE", hr, out)
	}
	if hr := comCall(cb.simplePtr(), slotQueryInterface, uintptr(unsafe.Pointer(&iidToggleProvider)),
		uintptr(unsafe.Pointer(&out))); hr != sOK || out != cb.togglePtr() {
		t.Errorf("QI(Toggle) у флажка: hr=%#x out=%#x", hr, out)
	}
	if hr := comCall(btn.simplePtr(), slotQueryInterface, uintptr(unsafe.Pointer(&iidToggleProvider)),
		uintptr(unsafe.Pointer(&out))); hr != eNoInterface {
		t.Errorf("QI(Toggle) у кнопки: hr=%#x, ожидался E_NOINTERFACE", hr)
	}
}

// TestUIAInvokeClicksButton — вызов Invoke() через настоящую vtable доходит до
// OnClick кнопки. Это и есть «скринридер нажал кнопку за пользователя».
func TestUIAInvokeClicksButton(t *testing.T) {
	sc := newUIAActionScene(t)
	btn := sc.elem(t, 1)

	var invoke uintptr
	comCall(btn.simplePtr(), slotPatternProvider, uiaPatternInvoke, uintptr(unsafe.Pointer(&invoke)))
	if invoke == 0 {
		t.Fatal("кнопка не отдала IInvokeProvider")
	}
	if hr := comCall(invoke, slotInvoke); hr != sOK {
		t.Fatalf("Invoke: hr=%#x", hr)
	}
	if sc.clicks != 1 {
		t.Fatalf("нажатий %d, ожидалось 1", sc.clicks)
	}
	if hr := comCall(invoke, slotInvoke); hr != sOK || sc.clicks != 2 {
		t.Errorf("повторный Invoke: hr=%#x clicks=%d", hr, sc.clicks)
	}

	// Выключенную кнопку нажать нельзя — клиент получает UIA_E_ELEMENTNOTENABLED.
	sc.btn.SetEnabled(false)
	if hr := comCall(invoke, slotInvoke); hr != uiaEElementNotEnabled || sc.clicks != 2 {
		t.Errorf("Invoke выключенной кнопки: hr=%#x clicks=%d", hr, sc.clicks)
	}
}

// TestUIAToggleCheckBox — Toggle() переключает флажок, а get_ToggleState
// показывает его текущее состояние.
func TestUIAToggleCheckBox(t *testing.T) {
	sc := newUIAActionScene(t)
	cb := sc.elem(t, 2)

	var toggle uintptr
	comCall(cb.simplePtr(), slotPatternProvider, uiaPatternToggle, uintptr(unsafe.Pointer(&toggle)))
	if toggle == 0 {
		t.Fatal("флажок не отдал IToggleProvider")
	}

	// state читает состояние ПОСЛЕ принудительного пересбора снимка: провайдер
	// отвечает по снимку, а он обновляется не чаще a11yRefreshEvery.
	state := func() int32 {
		t.Helper()
		sc.b.refresh(true)
		var s int32
		if hr := comCall(toggle, slotToggleState, uintptr(unsafe.Pointer(&s))); hr != sOK {
			t.Fatalf("get_ToggleState: hr=%#x", hr)
		}
		return s
	}

	if got := state(); got != uiaToggleOff {
		t.Fatalf("исходное состояние = %d, ожидался Off", got)
	}
	if hr := comCall(toggle, slotToggle); hr != sOK {
		t.Fatalf("Toggle: hr=%#x", hr)
	}
	if !sc.cb.IsChecked() {
		t.Fatal("Toggle не отметил флажок")
	}
	if got := state(); got != uiaToggleOn {
		t.Errorf("после Toggle состояние = %d, ожидался On", got)
	}
	if hr := comCall(toggle, slotToggle); hr != sOK || sc.cb.IsChecked() {
		t.Errorf("обратный Toggle: hr=%#x checked=%v", hr, sc.cb.IsChecked())
	}
	if got := state(); got != uiaToggleOff {
		t.Errorf("после обратного Toggle состояние = %d, ожидался Off", got)
	}
}

// TestUIASetFocusThroughVTable — SetFocus из IRawElementProviderFragment реально
// переводит фокус ввода в движке (раньше метод был заглушкой).
func TestUIASetFocusThroughVTable(t *testing.T) {
	sc := newUIAActionScene(t)
	btn, cb, lbl := sc.elem(t, 1), sc.elem(t, 2), sc.elem(t, 3)

	if hr := comCall(btn.fragmentPtr(), slotSetFocus); hr != sOK {
		t.Fatalf("SetFocus у кнопки: hr=%#x", hr)
	}
	if !sc.btn.IsFocused() {
		t.Fatal("кнопка не получила фокус")
	}
	// Снимок должен показать фокус там же, где его видит движок.
	sc.b.refresh(true)
	var v comVariant
	comCall(btn.simplePtr(), slotPropertyValue, uiaPropHasKeyboardFocus, uintptr(unsafe.Pointer(&v)))
	if v.vt != vtBool || v.val[0] == 0 {
		t.Errorf("HasKeyboardFocus у кнопки после SetFocus: vt=%d val=%d", v.vt, v.val[0])
	}

	if hr := comCall(cb.fragmentPtr(), slotSetFocus); hr != sOK {
		t.Fatalf("SetFocus у флажка: hr=%#x", hr)
	}
	if !sc.cb.IsFocused() || sc.btn.IsFocused() {
		t.Errorf("фокус не переехал на флажок: cb=%v btn=%v", sc.cb.IsFocused(), sc.btn.IsFocused())
	}

	// Надпись фокус не принимает, но HRESULT всё равно S_OK: ошибка отсюда
	// заставляет «Экранный диктор» считать провайдер сломанным.
	if hr := comCall(lbl.fragmentPtr(), slotSetFocus); hr != sOK {
		t.Errorf("SetFocus у надписи: hr=%#x, ожидался S_OK", hr)
	}
	if !sc.cb.IsFocused() {
		t.Error("отказной SetFocus не должен был сдвинуть фокус")
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

// TestUIAPasswordHidden — пароль не отдаётся UIA, IsPassword=true.
func TestUIAPasswordHidden(t *testing.T) {
	eng := engine.New(200, 100, 20)
	root := widget.NewWindow("T", 200, 100)
	ti := widget.NewTextInput("")
	ti.SetBounds(image.Rect(0, 40, 150, 70))
	ti.SetPasswordMode(true)
	ti.SetText("hunter2")
	root.AddChild(ti)
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
		t.Fatal("элемент поля не создан")
	}
	var out comVariant
	comCall(e.simplePtr(), slotPropertyValue, uiaPropIsPassword, uintptr(unsafe.Pointer(&out)))
	if out.vt != vtBool || out.val[0] == 0 {
		t.Errorf("IsPassword: vt=%d val=%d, ожидалось TRUE", out.vt, out.val[0])
	}
	comCall(e.simplePtr(), slotPropertyValue, uiaPropValueValue, uintptr(unsafe.Pointer(&out)))
	if out.vt == vtBSTR {
		if s := variantString(t, &out); s != "" {
			t.Errorf("Value раскрывает пароль: %q", s)
		}
	}
	if snap := eng.AccessibilityTree(); snap != nil {
		var walk func(n *widget.AccessNode)
		walk = func(n *widget.AccessNode) {
			if n.Value == "hunter2" {
				t.Error("пароль в семантическом дереве")
			}
			for _, c := range n.Children {
				walk(c)
			}
		}
		walk(snap)
	}
}
