//go:build windows

// a11y_windows.go — мост доступности UI Automation (Windows).
//
// Как это работает:
//
//  1. Скринридер (NVDA, «Экранный диктор», Инспектор UIA) шлёт окну
//     WM_GETOBJECT с lParam = UiaRootObjectId. Окно отвечает через
//     UiaReturnRawElementProvider, отдавая корневой провайдер.
//  2. Провайдер — обычный COM-объект: IRawElementProviderSimple (свойства),
//     IRawElementProviderFragment (навигация по дереву и границы) и
//     IRawElementProviderFragmentRoot (фокус и поиск по точке) у корня.
//     COM-обвязка собрана руками в a11y_uia_windows.go — CGO в проекте нет.
//  3. Семантику отдаёт движок (engine.AccessibilityTree), общий с Linux слой
//     a11y.go разворачивает её в плоский снимок с УСТОЙЧИВЫМИ id: клиент UIA
//     кэширует элементы по RuntimeId, и переиспользование id показало бы
//     скринридеру чужие имя и роль.
//  4. Изменения (фокус, имя, значение, структура) поднимаются событиями UIA,
//     но только когда клиенты действительно слушают (UiaClientsAreListening).
//
// Проверено на живом клиенте (.NET UIAutomationClient — та же сторона, что и у
// скринридера): дерево обходится TreeWalker'ом от окна до листьев с верными
// типами, именами, границами и фокусом; поиск по поддереву и по свойствам
// работает. Свойства, навигация, RuntimeId, границы и фокус покрыты тестами
// через НАСТОЯЩИЕ vtable (a11y_windows_test.go).
//
// ГРАБЛИ, на которые уже наступили (не наступать снова):
//
//   - НЕ отдавать UIA_NativeWindowHandlePropertyId из корня фрагмента. Это
//     свойство означает «элемент СОДЕРЖИТ вот это окно», и UIA идёт в него за
//     провайдером — то есть шлёт WM_GETOBJECT нам же, получает тот же корень и
//     делает его собственным ребёнком: обход уходит в бесконечное «окно внутри
//     окна». Закреплено тестом TestUIANoNativeWindowHandle.
//   - Каждый указатель, отданный в out-параметре, обязан быть учтён AddRef'ом
//     (см. outFragment/outRoot), а хост-провайдер должен быть ОДНИМ И ТЕМ ЖЕ
//     объектом при каждом запросе: UIA сверяет элементы по идентичности
//     COM-объекта.
//   - Разбирать поведение клиента можно журналом вызовов провайдера:
//     HEADLESS_GUI_UIA_LOG=<файл> (см. uiaLog).
//
// Не поддержано пока: паттерны управления (Invoke/Toggle/Value) — скринридер
// читает элементы, но не может нажимать их за пользователя; для этого движку
// нужен путь «активировать узел по семантическому id». И поиск по точке
// (ElementProviderFromPoint) берёт позицию курсора, а не переданные
// координаты — см. комментарий у этого метода.
package window

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/oops1/headless-gui/v3/widget"
	"golang.org/x/sys/windows"
)

// ─── Константы UI Automation ─────────────────────────────────────────────────

const (
	wmGetObject     = 0x003D
	uiaRootObjectID = -25 // UiaRootObjectId

	// Идентификаторы свойств (UIA_*PropertyId).
	uiaPropRuntimeID          = 30000
	uiaPropBoundingRectangle  = 30001
	uiaPropProcessID          = 30002
	uiaPropControlType        = 30003
	uiaPropName               = 30005
	uiaPropHasKeyboardFocus   = 30008
	uiaPropIsKeyboardFocusabl = 30009
	uiaPropIsEnabled          = 30010
	uiaPropAutomationID       = 30011
	uiaPropClassName          = 30012
	uiaPropHelpText           = 30013
	uiaPropIsControlElement   = 30016
	uiaPropIsContentElement   = 30017
	uiaPropIsPassword         = 30019
	uiaPropNativeWindowHandle = 30020
	uiaPropIsOffscreen        = 30022
	uiaPropFrameworkID        = 30024
	uiaPropValueValue         = 30045

	// Типы элементов управления (UIA_*ControlTypeId).
	uiaCtrlButton      = 50000
	uiaCtrlCheckBox    = 50002
	uiaCtrlComboBox    = 50003
	uiaCtrlEdit        = 50004
	uiaCtrlImage       = 50006
	uiaCtrlList        = 50008
	uiaCtrlMenuBar     = 50010
	uiaCtrlProgressBar = 50012
	uiaCtrlRadioButton = 50013
	uiaCtrlSlider      = 50015
	uiaCtrlSpinner     = 50016
	uiaCtrlTab         = 50018
	uiaCtrlText        = 50020
	uiaCtrlCustom      = 50025
	uiaCtrlGroup       = 50026
	uiaCtrlWindow      = 50032
	uiaCtrlPane        = 50033

	// Направления обхода (NavigateDirection).
	uiaNavParent      = 0
	uiaNavNextSibling = 1
	uiaNavPrevSibling = 2
	uiaNavFirstChild  = 3
	uiaNavLastChild   = 4

	// ProviderOptions_ServerSideProvider.
	uiaProviderServerSide = 1

	// UiaAppendRuntimeId — префикс RuntimeId провайдера внутри окна.
	uiaAppendRuntimeID = 3

	// События.
	uiaEventFocusChanged      = 20005
	uiaStructureChangeInvalid = 2 // StructureChangeType_ChildrenInvalidated
)

// uiaControlType переводит роль движка в тип элемента UIA.
func uiaControlType(r widget.AccessRole) int32 {
	switch r {
	case widget.RoleWindow:
		return uiaCtrlWindow
	case widget.RolePanel:
		return uiaCtrlPane
	case widget.RoleGroup:
		return uiaCtrlGroup
	case widget.RoleButton:
		return uiaCtrlButton
	case widget.RoleCheckBox, widget.RoleSwitch:
		return uiaCtrlCheckBox
	case widget.RoleRadioButton:
		return uiaCtrlRadioButton
	case widget.RoleSlider:
		return uiaCtrlSlider
	case widget.RoleProgressBar:
		return uiaCtrlProgressBar
	case widget.RoleTextInput:
		return uiaCtrlEdit
	case widget.RoleLabel:
		return uiaCtrlText
	case widget.RoleComboBox:
		return uiaCtrlComboBox
	case widget.RoleList:
		return uiaCtrlList
	case widget.RoleTabControl:
		return uiaCtrlTab
	case widget.RoleMenuBar:
		return uiaCtrlMenuBar
	case widget.RoleSpinner:
		return uiaCtrlSpinner
	case widget.RoleImage:
		return uiaCtrlImage
	}
	return uiaCtrlCustom
}

// uiaFocusable — может ли элемент получить фокус клавиатуры.
func uiaFocusable(info widget.AccessInfo) bool {
	if a11yHasState(info.States, widget.StateDisabled) {
		return false
	}
	switch info.Role {
	case widget.RoleButton, widget.RoleCheckBox, widget.RoleRadioButton, widget.RoleSwitch,
		widget.RoleSlider, widget.RoleTextInput, widget.RoleComboBox, widget.RoleList,
		widget.RoleTabControl, widget.RoleSpinner:
		return true
	}
	return false
}

// ─── COM-элемент ─────────────────────────────────────────────────────────────

// uiaElement — COM-объект одного элемента дерева. Три первых поля — указатели
// на vtable: их АДРЕСА и есть COM-указатели соответствующих интерфейсов
// (&e.simpleVT — IRawElementProviderSimple и т.д.).
type uiaElement struct {
	simpleVT   uintptr
	fragmentVT uintptr
	rootVT     uintptr

	// refs — счётчик ссылок COM. Атомарный: AddRef/Release приходят из
	// потоков UIA, а не только из UI-потока окна.
	refs atomic.Int32
	b    *uiaBridge
	id   int32
}

// outFragment/outRoot отдают интерфейс наружу по правилам COM: каждый
// указатель, попавший в out-параметр, обязан быть учтён AddRef'ом — иначе
// клиент освободит объект, которым мы продолжаем пользоваться.
func (e *uiaElement) outFragment() uintptr {
	e.refs.Add(1)
	return e.fragmentPtr()
}

func (e *uiaElement) outRoot() uintptr {
	e.refs.Add(1)
	return e.rootPtr()
}

var (
	uiaObjMu   sync.RWMutex
	uiaObjects = map[uintptr]*uiaElement{} // адрес поля-vtable → элемент
)

// uiaLookup находит элемент по COM-указателю.
func uiaLookup(this uintptr) *uiaElement {
	uiaObjMu.RLock()
	e := uiaObjects[this]
	uiaObjMu.RUnlock()
	return e
}

// newUIAElement создаёт COM-объект элемента и регистрирует его указатели.
// Объект живёт до закрытия окна: держать его в карте дешевле, чем городить
// освобождение по счётчику ссылок из чужого потока.
func newUIAElement(b *uiaBridge, id int32) *uiaElement {
	e := &uiaElement{b: b, id: id}
	e.refs.Store(1)
	e.simpleVT = uiaSimpleVTable()
	e.fragmentVT = uiaFragmentVTable()
	e.rootVT = uiaRootVTable()
	uiaObjMu.Lock()
	uiaObjects[uintptr(unsafe.Pointer(&e.simpleVT))] = e
	uiaObjects[uintptr(unsafe.Pointer(&e.fragmentVT))] = e
	uiaObjects[uintptr(unsafe.Pointer(&e.rootVT))] = e
	uiaObjMu.Unlock()
	return e
}

// forget снимает регистрацию COM-указателей элемента.
func (e *uiaElement) forget() {
	uiaObjMu.Lock()
	delete(uiaObjects, uintptr(unsafe.Pointer(&e.simpleVT)))
	delete(uiaObjects, uintptr(unsafe.Pointer(&e.fragmentVT)))
	delete(uiaObjects, uintptr(unsafe.Pointer(&e.rootVT)))
	uiaObjMu.Unlock()
}

func (e *uiaElement) simplePtr() uintptr   { return uintptr(unsafe.Pointer(&e.simpleVT)) }
func (e *uiaElement) fragmentPtr() uintptr { return uintptr(unsafe.Pointer(&e.fragmentVT)) }
func (e *uiaElement) rootPtr() uintptr     { return uintptr(unsafe.Pointer(&e.rootVT)) }

// isRoot — корень фрагмента (окно): у него живёт FragmentRoot и хост-провайдер.
func (e *uiaElement) isRoot() bool { return e.id == e.b.rootID() }

// ─── Таблицы виртуальных методов ─────────────────────────────────────────────

var (
	uiaVTOnce                             sync.Once
	uiaVTSimple, uiaVTFragment, uiaVTRoot uintptr
)

func uiaInitVTables() {
	uiaVTOnce.Do(func() {
		qi := windows.NewCallback(uiaQueryInterface)
		addRef := windows.NewCallback(uiaAddRef)
		release := windows.NewCallback(uiaRelease)

		uiaVTSimple = newVTable(qi, addRef, release,
			windows.NewCallback(uiaGetProviderOptions),
			windows.NewCallback(uiaGetPatternProvider),
			windows.NewCallback(uiaGetPropertyValue),
			windows.NewCallback(uiaGetHostProvider),
		)
		uiaVTFragment = newVTable(qi, addRef, release,
			windows.NewCallback(uiaNavigate),
			windows.NewCallback(uiaGetRuntimeID),
			windows.NewCallback(uiaGetBoundingRectangle),
			windows.NewCallback(uiaGetEmbeddedFragmentRoots),
			windows.NewCallback(uiaSetFocus),
			windows.NewCallback(uiaGetFragmentRoot),
		)
		uiaVTRoot = newVTable(qi, addRef, release,
			windows.NewCallback(uiaElementProviderFromPoint),
			windows.NewCallback(uiaGetFocus),
		)
	})
}

func uiaSimpleVTable() uintptr   { uiaInitVTables(); return uiaVTSimple }
func uiaFragmentVTable() uintptr { uiaInitVTables(); return uiaVTFragment }
func uiaRootVTable() uintptr     { uiaInitVTables(); return uiaVTRoot }

// ─── IUnknown ────────────────────────────────────────────────────────────────

func uiaQueryInterface(this uintptr, riid *comGUID, ppv *uintptr) uintptr {
	if ppv == nil {
		return eInvalidArg
	}
	*ppv = 0
	e := uiaLookup(this)
	if e == nil {
		return eNoInterface
	}
	switch {
	case riid.equals(&iidIUnknown), riid.equals(&iidProviderSimple):
		uiaLog("QI(%d, Simple)", e.id)
		*ppv = e.simplePtr()
	case riid.equals(&iidProviderFragment):
		uiaLog("QI(%d, Fragment)", e.id)
		*ppv = e.fragmentPtr()
	case riid.equals(&iidProviderFragmentRoot):
		if !e.isRoot() {
			uiaLog("QI(%d, FragmentRoot) → не корень, E_NOINTERFACE", e.id)
			return eNoInterface
		}
		uiaLog("QI(%d, FragmentRoot)", e.id)
		*ppv = e.rootPtr()
	default:
		uiaLog("QI(%d, %s) → E_NOINTERFACE", e.id, riid)
		return eNoInterface
	}
	e.refs.Add(1)
	return sOK
}

func uiaAddRef(this uintptr) uintptr {
	if e := uiaLookup(this); e != nil {
		return uintptr(e.refs.Add(1))
	}
	return 1
}

// uiaRelease уменьшает счётчик. Память не освобождаем: элементы живут вместе с
// мостом (см. newUIAElement) — иначе пришлось бы синхронизировать удаление с
// возможными вызовами из потоков UIA.
func uiaRelease(this uintptr) uintptr {
	if e := uiaLookup(this); e != nil {
		if n := e.refs.Add(-1); n >= 0 {
			return uintptr(n)
		}
		e.refs.Store(0)
	}
	return 0
}

// ─── IRawElementProviderSimple ───────────────────────────────────────────────

func uiaGetProviderOptions(this uintptr, out *int32) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = uiaProviderServerSide
	return sOK
}

// uiaGetPatternProvider — паттернов управления пока нет (см. шапку файла).
func uiaGetPatternProvider(this uintptr, patternID int32, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	return sOK
}

func uiaGetPropertyValue(this uintptr, propID int32, out *comVariant) uintptr {
	if out == nil {
		return eInvalidArg
	}
	out.setEmpty()
	e := uiaLookup(this)
	if e == nil {
		return eFail
	}
	e.b.fillProperty(e.id, propID, out)
	uiaLog("Property(%d, %d) → vt=%d", e.id, propID, out.vt)
	return sOK
}

// uiaGetHostProvider — у корня это провайдер самого окна (UIA доберёт из него
// свойства HWND), у остальных элементов — NULL.
func uiaGetHostProvider(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	e := uiaLookup(this)
	if e == nil || !e.isRoot() {
		return sOK
	}
	// Хост-провайдер должен быть ОДНИМ И ТЕМ ЖЕ объектом при каждом запросе:
	// UIA сверяет элементы по идентичности COM-объекта, и новый экземпляр на
	// каждый вызов она принимает за новый элемент окна. Отдаём кэшированный —
	// с обязательным AddRef, потому что владение переходит клиенту.
	*out = e.b.hostProvider()
	uiaLog("HostProvider(%d) → %#x", e.id, *out)
	return sOK
}

// ─── IRawElementProviderFragment ─────────────────────────────────────────────

func uiaNavigate(this uintptr, direction int32, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	e := uiaLookup(this)
	if e == nil {
		return eFail
	}
	target := e.b.navigate(e.id, direction)
	if target != nil {
		*out = target.outFragment()
	}
	uiaLog("Navigate(%d, dir=%d) → %s", e.id, direction, uiaTargetName(target))
	return sOK
}

// uiaTargetName — компактное описание элемента для журнала.
func uiaTargetName(e *uiaElement) string {
	if e == nil {
		return "NULL"
	}
	return "id=" + strconv.Itoa(int(e.id))
}

// uiaGetRuntimeID — идентификатор элемента для клиента: [UiaAppendRuntimeId, id].
func uiaGetRuntimeID(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	e := uiaLookup(this)
	if e == nil {
		return eFail
	}
	*out = safeArrayOfInts([]int32{uiaAppendRuntimeID, e.id})
	uiaLog("RuntimeId(%d) → [%d %d] sa=%#x", e.id, uiaAppendRuntimeID, e.id, *out)
	return sOK
}

// uiaRect — UiaRect (левый верхний угол, ширина, высота; экранные пиксели).
type uiaRect struct {
	left, top, width, height float64
}

func uiaGetBoundingRectangle(this uintptr, out *uiaRect) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = uiaRect{}
	e := uiaLookup(this)
	if e == nil {
		return eFail
	}
	*out = e.b.boundsOf(e.id)
	return sOK
}

func uiaGetEmbeddedFragmentRoots(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	return sOK
}

// uiaSetFocus — программная передача фокуса пока не поддержана: у движка нет
// публичного «сфокусировать узел по семантическому id». Возвращаем S_OK,
// чтобы клиент не считал провайдер сломанным.
func uiaSetFocus(this uintptr) uintptr { return sOK }

func uiaGetFragmentRoot(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	e := uiaLookup(this)
	if e == nil {
		return eFail
	}
	root := e.b.element(e.b.rootID())
	if root != nil {
		*out = root.outRoot()
	}
	uiaLog("FragmentRoot(%d) → %s", e.id, uiaTargetName(root))
	return sOK
}

// ─── IRawElementProviderFragmentRoot ─────────────────────────────────────────

// uiaElementProviderFromPoint — поиск элемента под точкой.
//
// ВАЖНО: x и y приходят как double в XMM-регистрах, а windows.NewCallback
// аргументы с плавающей точкой не передаёт — до Go-функции они не доходят.
// Поэтому берём позицию курсора у системы: этот метод клиенты доступности
// вызывают именно для «что под мышью», и координаты совпадают.
func uiaElementProviderFromPoint(this uintptr, _, _ uintptr, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	e := uiaLookup(this)
	if e == nil {
		return eFail
	}
	x, y, ok := cursorScreenPoint()
	if !ok {
		return sOK
	}
	if hit := e.b.hitTestScreen(x, y); hit != nil {
		*out = hit.outFragment()
	}
	return sOK
}

func uiaGetFocus(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	e := uiaLookup(this)
	if e == nil {
		return eFail
	}
	f := e.b.focusElement()
	if f != nil {
		*out = f.outFragment()
	}
	uiaLog("GetFocus(%d) → %s", e.id, uiaTargetName(f))
	return sOK
}

// cursorScreenPoint — позиция курсора в экранных координатах.
func cursorScreenPoint() (int32, int32, bool) {
	var pt struct{ X, Y int32 }
	r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	if r == 0 {
		return 0, 0, false
	}
	return pt.X, pt.Y, true
}

// ─── Мост ────────────────────────────────────────────────────────────────────

func init() {
	newA11yBridge = func(win *Window) a11yBridge { return &uiaBridge{win: win} }
}

// uiaBridge — состояние моста для одного окна.
type uiaBridge struct {
	win *Window

	mu      sync.RWMutex
	ids     a11yIDPool
	view    *a11yView
	prev    *a11yView
	stamp   time.Time
	dirty   bool
	elems   map[int32]*uiaElement
	hwnd    uintptr
	hostPtr atomic.Uintptr // кэш провайдера окна (см. hostProvider)

	notifier uint64
	stopCh   chan struct{}
	stopOnce sync.Once
	loopOnce sync.Once
}

// uiaBridges — мосты по hwnd: wndProc обрабатывает WM_GETOBJECT без правки
// структуры Win32Window.
var (
	uiaBridgeMu sync.RWMutex
	uiaBridges  = map[uintptr]*uiaBridge{}
)

// start поднимает мост: он ничего не занимает в системе — просто становится
// готов отвечать на WM_GETOBJECT.
func (b *uiaBridge) start() error {
	if !uiaAvailable() || !b.enabled() {
		return errUIAUnavailable
	}
	nw, ok := b.win.native.(*Win32Window)
	if !ok || nw.hwnd == 0 {
		return errUIAUnavailable
	}
	b.hwnd = uintptr(nw.hwnd)
	b.createHostProvider()
	b.elems = map[int32]*uiaElement{}
	b.refresh(true)

	uiaBridgeMu.Lock()
	uiaBridges[b.hwnd] = b
	uiaBridgeMu.Unlock()

	b.stopCh = make(chan struct{})
	return nil
}

// ensureEventLoop поднимает слежение за изменениями UI при ПЕРВОМ обращении
// клиента доступности. Пока никто не спрашивал, мост не стоит приложению
// ничего: ни горутины, ни подписки, ни снимков семантики.
func (b *uiaBridge) ensureEventLoop() {
	b.loopOnce.Do(func() {
		b.notifier = widget.RegisterUINotifier(b.markDirty, nil)
		go b.eventLoop()
	})
}

// stop снимает мост и отключает провайдеры от клиентов.
func (b *uiaBridge) stop() {
	b.stopOnce.Do(func() {
		if b.stopCh != nil {
			close(b.stopCh)
		}
		widget.UnregisterUINotifier(b.notifier)
		uiaBridgeMu.Lock()
		delete(uiaBridges, b.hwnd)
		uiaBridgeMu.Unlock()

		b.mu.Lock()
		for _, e := range b.elems {
			e.forget()
		}
		b.elems = map[int32]*uiaElement{}
		b.mu.Unlock()

		if uiaCore.Load() == nil {
			procUiaDisconnectAllProviders.Call()
		}
	})
}

// enabled решает, поднимать ли мост. По умолчанию ВКЛЮЧЁН: сам по себе он
// пассивен (пока никто не прислал WM_GETOBJECT, нет ни горутин, ни снимков —
// см. ensureEventLoop), а без него скринридер не видит в borderless-окне
// ничего, кроме пустого прямоугольника. Выключить: SetAccessibilityEnabled(false)
// или HEADLESS_GUI_A11Y=0.
func (b *uiaBridge) enabled() bool {
	if v := os.Getenv("HEADLESS_GUI_A11Y"); v != "" {
		return v != "0" && !strings.EqualFold(v, "false")
	}
	if b.win.a11yForce != nil {
		return *b.win.a11yForce
	}
	return true
}

func (b *uiaBridge) markDirty() {
	b.mu.Lock()
	b.dirty = true
	b.mu.Unlock()
}

// hostProvider — провайдер самого окна (UiaHostProviderFromHwnd). Создаётся
// РОВНО ОДИН раз (в start, на UI-потоке) и отдаётся клиенту с AddRef: каждый
// вызов UiaHostProviderFromHwnd возвращает НОВЫЙ COM-объект, а UIA сверяет
// элементы по идентичности объекта — новый экземпляр на каждый запрос она
// принимает за другой элемент окна.
func (b *uiaBridge) hostProvider() uintptr {
	host := b.hostPtr.Load()
	comAddRef(host) // владение отданным указателем переходит клиенту
	return host
}

// createHostProvider создаёт хост-провайдер один раз при подъёме моста.
func (b *uiaBridge) createHostProvider() {
	if b.hwnd == 0 || uiaCore.Load() != nil {
		return
	}
	var host uintptr
	procUiaHostProviderFromHwnd.Call(b.hwnd, uintptr(unsafe.Pointer(&host)))
	b.hostPtr.Store(host)
}

func (b *uiaBridge) hwndValue() uintptr {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.hwnd
}

// rootID — устойчивый id корня снимка (окна).
func (b *uiaBridge) rootID() int32 { return b.current().id(a11yRootID) }

// refresh пересобирает снимок семантики и возвращает изменения.
func (b *uiaBridge) refresh(force bool) a11yChanges {
	b.mu.Lock()
	if !force && !b.dirty && time.Since(b.stamp) < a11yRefreshEvery {
		b.mu.Unlock()
		return a11yChanges{FocusLost: -1, FocusGained: -1}
	}
	old := b.view
	b.mu.Unlock()

	snap := b.win.accessibilitySnapshot()
	var oldSnap *a11ySnapshot
	if old != nil {
		oldSnap = old.Snap
	}
	ch := a11yDiff(oldSnap, snap)

	b.mu.Lock()
	b.prev = b.view
	b.view = b.ids.assign(snap)
	b.stamp = time.Now()
	b.dirty = false
	b.mu.Unlock()
	return ch
}

// current возвращает актуальное представление, при необходимости пересобрав его.
func (b *uiaBridge) current() *a11yView {
	b.mu.RLock()
	v, stamp, dirty := b.view, b.stamp, b.dirty
	b.mu.RUnlock()
	if v == nil || (dirty && time.Since(stamp) >= a11yRefreshEvery) {
		b.refresh(v == nil)
		b.mu.RLock()
		v = b.view
		b.mu.RUnlock()
	}
	return v
}

// element возвращает (создавая при необходимости) COM-объект элемента.
func (b *uiaBridge) element(id int32) *uiaElement {
	if id < 0 {
		return nil
	}
	b.mu.RLock()
	e := b.elems[id]
	b.mu.RUnlock()
	if e != nil {
		return e
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if e = b.elems[id]; e != nil {
		return e
	}
	e = newUIAElement(b, id)
	if b.elems == nil {
		b.elems = map[int32]*uiaElement{}
	}
	b.elems[id] = e
	return e
}

// eventLoop раз в a11yRefreshEvery пересобирает снимок и поднимает события.
func (b *uiaBridge) eventLoop() {
	t := time.NewTicker(a11yRefreshEvery)
	defer t.Stop()
	for {
		select {
		case <-b.stopCh:
			return
		case <-t.C:
			b.mu.RLock()
			dirty := b.dirty
			b.mu.RUnlock()
			// Пока клиентов доступности нет, снимок не пересобираем: мост
			// не должен стоить ничего приложению, которое никто не читает.
			if !dirty || !uiaClientsListening() {
				continue
			}
			b.emitChanges(b.refresh(true))
		}
	}
}

// emitChanges поднимает события UIA по результатам диффа снимков.
func (b *uiaBridge) emitChanges(ch a11yChanges) {
	b.mu.RLock()
	curV, prevV := b.view, b.prev
	b.mu.RUnlock()
	if curV == nil {
		return
	}
	if ch.Structural {
		if root := b.element(curV.id(a11yRootID)); root != nil {
			rid := []int32{uiaAppendRuntimeID, root.id}
			if procUiaRaiseStructureChanged.Find() == nil {
				procUiaRaiseStructureChanged.Call(root.simplePtr(), uiaStructureChangeInvalid,
					uintptr(unsafe.Pointer(&rid[0])), uintptr(len(rid)))
			}
		}
		return
	}
	if ch.FocusGained >= 0 {
		if e := b.element(curV.id(ch.FocusGained)); e != nil {
			procUiaRaiseAutomationEvent.Call(e.simplePtr(), uiaEventFocusChanged)
		}
	}
	raiseProp := func(idx int32, prop int32, set func(v *comVariant, n *a11yNode)) {
		node := curV.Snap.node(idx)
		e := b.element(curV.id(idx))
		if node == nil || e == nil {
			return
		}
		var oldV, newV comVariant
		if prevV != nil {
			if p := prevV.Snap.node(idx); p != nil {
				set(&oldV, p)
			}
		}
		set(&newV, node)
		procUiaRaisePropertyChanged.Call(e.simplePtr(), uintptr(prop),
			uintptr(unsafe.Pointer(&oldV)), uintptr(unsafe.Pointer(&newV)))
	}
	for _, idx := range ch.NameChanged {
		raiseProp(idx, uiaPropName, func(v *comVariant, n *a11yNode) { v.setString(n.Info.Name) })
	}
	for _, idx := range ch.ValueChanged {
		raiseProp(idx, uiaPropValueValue, func(v *comVariant, n *a11yNode) { v.setString(n.Info.Value) })
	}
	for _, idx := range ch.StateChanged {
		raiseProp(idx, uiaPropIsEnabled, func(v *comVariant, n *a11yNode) {
			v.setBool(!a11yHasState(n.Info.States, widget.StateDisabled))
		})
	}
}

// ─── Данные для провайдеров ──────────────────────────────────────────────────

// fillProperty кладёт в VARIANT значение запрошенного свойства.
func (b *uiaBridge) fillProperty(id int32, prop int32, out *comVariant) {
	v := b.current()
	node := v.node(id)
	if node == nil {
		return
	}
	isRoot := id == v.id(a11yRootID)
	switch prop {
	case uiaPropName:
		name := node.Info.Name
		if name == "" && isRoot {
			name = b.win.title
		}
		out.setString(name)
	case uiaPropControlType:
		out.setI4(uiaControlType(node.Info.Role))
	case uiaPropAutomationID:
		out.setString("hg-" + strconv.Itoa(int(id)))
	case uiaPropClassName:
		out.setString("HeadlessGui")
	case uiaPropFrameworkID:
		out.setString("headless-gui")
	case uiaPropHelpText:
		out.setString(node.Info.Description)
	case uiaPropIsEnabled:
		out.setBool(!a11yHasState(node.Info.States, widget.StateDisabled))
	case uiaPropHasKeyboardFocus:
		out.setBool(v.Snap.Focus >= 0 && v.id(v.Snap.Focus) == id)
	case uiaPropIsKeyboardFocusabl:
		out.setBool(uiaFocusable(node.Info))
	case uiaPropIsControlElement, uiaPropIsContentElement:
		out.setBool(true)
	case uiaPropIsOffscreen:
		out.setBool(false)
	case uiaPropIsPassword:
		out.setBool(false)
	case uiaPropValueValue:
		out.setString(node.Info.Value)
	case uiaPropNativeWindowHandle:
		// НЕ отдаём HWND. Это свойство означает «элемент СОДЕРЖИТ вот это
		// окно», и UIA послушно идёт в него за провайдером — то есть шлёт
		// WM_GETOBJECT нашему же окну и получает этот же корень, делая его
		// собственным ребёнком (бесконечное «окно внутри окна»). Дескриптор
		// окна клиент и так получает из хост-провайдера.
	case uiaPropProcessID:
		out.setI4(int32(windows.GetCurrentProcessId()))
	}
}

// navigate возвращает соседний элемент в заданном направлении.
func (b *uiaBridge) navigate(id int32, direction int32) *uiaElement {
	v := b.current()
	idx, ok := v.Index[id]
	if !ok {
		return nil
	}
	node := v.Snap.node(idx)
	if node == nil {
		return nil
	}
	switch direction {
	case uiaNavParent:
		if node.Parent < 0 {
			return nil // корень фрагмента: выше — окно, его подставит UIA
		}
		return b.element(v.id(node.Parent))
	case uiaNavFirstChild:
		if len(node.Children) == 0 {
			return nil
		}
		return b.element(v.id(node.Children[0]))
	case uiaNavLastChild:
		if len(node.Children) == 0 {
			return nil
		}
		return b.element(v.id(node.Children[len(node.Children)-1]))
	case uiaNavNextSibling, uiaNavPrevSibling:
		if node.Parent < 0 {
			return nil
		}
		siblings := v.Snap.node(node.Parent).Children
		pos := int(node.Index)
		if pos < 0 || pos >= len(siblings) {
			return nil
		}
		if direction == uiaNavNextSibling {
			pos++
		} else {
			pos--
		}
		if pos < 0 || pos >= len(siblings) {
			return nil
		}
		return b.element(v.id(siblings[pos]))
	}
	return nil
}

// boundsOf переводит логические границы узла в экранные физические пиксели.
func (b *uiaBridge) boundsOf(id int32) uiaRect {
	v := b.current()
	node := v.node(id)
	if node == nil {
		return uiaRect{}
	}
	scale := b.win.scale
	if scale <= 0 {
		scale = 1
	}
	r := node.Info.Bounds
	ox, oy := 0, 0
	if b.win.native != nil {
		ox, oy = b.win.native.GetPosition()
	}
	return uiaRect{
		left:   float64(ox) + float64(r.Min.X)*scale,
		top:    float64(oy) + float64(r.Min.Y)*scale,
		width:  float64(r.Dx()) * scale,
		height: float64(r.Dy()) * scale,
	}
}

// hitTestScreen ищет элемент под точкой экрана.
func (b *uiaBridge) hitTestScreen(x, y int32) *uiaElement {
	v := b.current()
	scale := b.win.scale
	if scale <= 0 {
		scale = 1
	}
	ox, oy := 0, 0
	if b.win.native != nil {
		ox, oy = b.win.native.GetPosition()
	}
	lx := int(float64(int(x)-ox)/scale + 0.5)
	ly := int(float64(int(y)-oy)/scale + 0.5)
	hit := v.Snap.hitTest(lx, ly)
	if hit < 0 {
		return nil
	}
	return b.element(v.id(hit))
}

// focusElement — элемент с фокусом клавиатуры (или корень).
func (b *uiaBridge) focusElement() *uiaElement {
	v := b.current()
	if v.Snap.Focus >= 0 {
		return b.element(v.id(v.Snap.Focus))
	}
	return b.element(v.id(a11yRootID))
}

// ─── WM_GETOBJECT ────────────────────────────────────────────────────────────

// uiaHandleGetObject отвечает на WM_GETOBJECT корневым провайдером окна.
// ok=false — сообщение не про UI Automation, обрабатывает DefWindowProc.
func uiaHandleGetObject(hwnd, wparam, lparam uintptr) (uintptr, bool) {
	if int32(lparam) != uiaRootObjectID {
		return 0, false
	}
	uiaBridgeMu.RLock()
	b := uiaBridges[hwnd]
	uiaBridgeMu.RUnlock()
	if b == nil {
		return 0, false
	}
	b.ensureEventLoop() // клиент доступности пришёл — начинаем следить за UI
	root := b.element(b.rootID())
	if root == nil {
		return 0, false
	}
	root.refs.Add(1) // UIA владеет отданным указателем и освободит его сама
	ret, _, _ := procUiaReturnRawElementProvider.Call(hwnd, wparam, lparam, root.simplePtr())
	uiaLog("WM_GETOBJECT → корень id=%d, ret=%#x", root.id, ret)
	return ret, true
}
