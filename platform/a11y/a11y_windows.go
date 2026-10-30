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
//     IRawElementProviderFragmentRoot (фокус и поиск по точке) у корня. Плюс
//     паттерны управления: IInvokeProvider у кнопок и IToggleProvider у
//     флажков/переключателей — через них скринридер нажимает элемент.
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
// Действия скринридера (SetFocus, Invoke, Toggle) уходят в движок через
// accessibilityNodeAction (a11y.go) и хост окна (native.AccessHost).
// Активация выполняется СИНТЕТИЧЕСКИМ КЛИКОМ по центру виджета, поэтому
// перекрытый другим виджетом элемент нажать не удастся (см. ActivateAccessible).
//
// Тексту отданы паттерны Value (содержимое строкой) и Text (чтение кусками,
// каретка, выделение — a11y_textpattern_windows.go).
//
// Радиокнопке отдан SelectionItem (IsSelected, Select), а не Invoke: клиент
// спрашивает «выбрана ли», а нажатие выбирает пункт. Пункту меню — Invoke
// (или Toggle, если он с отметкой) и ExpandCollapse, если у него подменю.
// Узлам, которые рисует сам виджет-владелец (панели сравнения, пункты меню),
// действия доставляются через base.AccessActionHandler владельца
// (accessibilityNodeAction), а не кликом в центр виджета.
//
// Прокрученный за край элемент отдаётся с IsOffscreen=true и пустой
// BoundingRectangle, частично видимый — с пересечением границ и окна
// прокрутки.
//
// Не поддержано пока: Value у Slider (значение отдаётся только свойством
// ValueValue, менять его клиент не может). Поиск по точке
// (ElementProviderFromPoint) берёт позицию курсора, а не переданные
// координаты — см. комментарий у этого метода. Всплывающее меню, вынесенное
// движком в отдельное окно ОС, UIA спрашивает у окна меню, а не у главного, и
// его пункты в дереве главного окна видны, но под курсором не находятся.

package a11y

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/platform/win32"
	"github.com/oops1/headless-gui/v4/widget/base"
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

	uiaPropAcceleratorKey = 30006
	// Свойства паттернов: UIA запрашивает их у провайдера и для кэширования.
	uiaPropExpandCollapseState = 30070
	uiaPropSelectionItemIsSel  = 30079
	uiaPropToggleState         = 30086
	uiaPropIsDialog            = 30174

	// Типы элементов управления (UIA_*ControlTypeId).
	uiaCtrlButton      = 50000
	uiaCtrlCheckBox    = 50002
	uiaCtrlComboBox    = 50003
	uiaCtrlEdit        = 50004
	uiaCtrlImage       = 50006
	uiaCtrlList        = 50008
	uiaCtrlMenu        = 50009
	uiaCtrlMenuBar     = 50010
	uiaCtrlMenuItem    = 50011
	uiaCtrlProgressBar = 50012
	uiaCtrlRadioButton = 50013
	uiaCtrlSlider      = 50015
	uiaCtrlSpinner     = 50016
	uiaCtrlTab         = 50018
	uiaCtrlText        = 50020
	uiaCtrlDocument    = 50030
	uiaCtrlCustom      = 50025
	uiaCtrlGroup       = 50026
	uiaCtrlWindow      = 50032
	uiaCtrlPane        = 50033

	// Паттерны управления (UIA_*PatternId).
	uiaPatternInvoke         = 10000
	uiaPatternValue          = 10002
	uiaPatternExpandCollapse = 10005
	uiaPatternSelectionItem  = 10010
	uiaPatternToggle         = 10015

	// ExpandCollapseState.
	uiaExpandCollapsed = 0
	uiaExpanded        = 1

	// ToggleState.
	uiaToggleOff = 0
	uiaToggleOn  = 1

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
func uiaControlType(r base.AccessRole) int32 {
	switch r {
	case base.RoleWindow:
		return uiaCtrlWindow
	case base.RolePanel:
		return uiaCtrlPane
	case base.RoleGroup:
		return uiaCtrlGroup
	case base.RoleButton:
		return uiaCtrlButton
	case base.RoleCheckBox, base.RoleSwitch:
		return uiaCtrlCheckBox
	case base.RoleRadioButton:
		return uiaCtrlRadioButton
	case base.RoleSlider:
		return uiaCtrlSlider
	case base.RoleProgressBar:
		return uiaCtrlProgressBar
	case base.RoleTextInput:
		return uiaCtrlEdit
	case base.RoleDocument:
		return uiaCtrlDocument
	case base.RoleLabel:
		return uiaCtrlText
	case base.RoleComboBox:
		return uiaCtrlComboBox
	case base.RoleList:
		return uiaCtrlList
	case base.RoleTabControl:
		return uiaCtrlTab
	case base.RoleMenuBar:
		return uiaCtrlMenuBar
	case base.RoleSpinner:
		return uiaCtrlSpinner
	case base.RoleImage:
		return uiaCtrlImage
	case base.RoleDialog:
		// Диалог в UIA — окно, отличаемое свойством IsDialog.
		return uiaCtrlWindow
	case base.RoleMenu:
		return uiaCtrlMenu
	case base.RoleMenuItem:
		return uiaCtrlMenuItem
	}
	return uiaCtrlCustom
}

// uiaFocusable — может ли элемент получить фокус клавиатуры.
func uiaFocusable(info base.AccessInfo) bool {
	if a11yHasState(info.States, base.StateDisabled) {
		return false
	}
	switch info.Role {
	case base.RoleButton, base.RoleCheckBox, base.RoleRadioButton, base.RoleSwitch,
		base.RoleSlider, base.RoleTextInput, base.RoleComboBox, base.RoleList,
		base.RoleTabControl, base.RoleSpinner, base.RoleMenuItem:
		return true
	}
	return false
}

// ─── COM-элемент ─────────────────────────────────────────────────────────────

// uiaElement — COM-объект одного элемента дерева. Первые поля — указатели на
// vtable: их АДРЕСА и есть COM-указатели соответствующих интерфейсов
// (&e.simpleVT — IRawElementProviderSimple и т.д.). Паттерны Invoke и Toggle
// живут в том же объекте: держать их отдельными COM-объектами незачем — время
// жизни то же, а счётчик ссылок общий (так делает и стандартный UIA-провайдер).
type uiaElement struct {
	simpleVT   uintptr
	fragmentVT uintptr
	rootVT     uintptr
	invokeVT   uintptr
	toggleVT   uintptr
	valueVT    uintptr
	textVT     uintptr
	selItemVT  uintptr
	expandVT   uintptr

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
	e.invokeVT = uiaInvokeVTable()
	e.toggleVT = uiaToggleVTable()
	e.valueVT = uiaValueVTable()
	e.textVT = uiaTextVTable()
	e.selItemVT = uiaSelItemVTable()
	e.expandVT = uiaExpandVTable()
	uiaObjMu.Lock()
	uiaObjects[uintptr(unsafe.Pointer(&e.simpleVT))] = e
	uiaObjects[uintptr(unsafe.Pointer(&e.fragmentVT))] = e
	uiaObjects[uintptr(unsafe.Pointer(&e.rootVT))] = e
	uiaObjects[uintptr(unsafe.Pointer(&e.invokeVT))] = e
	uiaObjects[uintptr(unsafe.Pointer(&e.toggleVT))] = e
	uiaObjects[uintptr(unsafe.Pointer(&e.valueVT))] = e
	uiaObjects[uintptr(unsafe.Pointer(&e.textVT))] = e
	uiaObjects[uintptr(unsafe.Pointer(&e.selItemVT))] = e
	uiaObjects[uintptr(unsafe.Pointer(&e.expandVT))] = e
	uiaObjMu.Unlock()
	return e
}

// forget снимает регистрацию COM-указателей элемента.
func (e *uiaElement) forget() {
	uiaObjMu.Lock()
	delete(uiaObjects, uintptr(unsafe.Pointer(&e.simpleVT)))
	delete(uiaObjects, uintptr(unsafe.Pointer(&e.fragmentVT)))
	delete(uiaObjects, uintptr(unsafe.Pointer(&e.rootVT)))
	delete(uiaObjects, uintptr(unsafe.Pointer(&e.invokeVT)))
	delete(uiaObjects, uintptr(unsafe.Pointer(&e.toggleVT)))
	delete(uiaObjects, uintptr(unsafe.Pointer(&e.valueVT)))
	delete(uiaObjects, uintptr(unsafe.Pointer(&e.textVT)))
	delete(uiaObjects, uintptr(unsafe.Pointer(&e.selItemVT)))
	delete(uiaObjects, uintptr(unsafe.Pointer(&e.expandVT)))
	uiaObjMu.Unlock()
}

func (e *uiaElement) simplePtr() uintptr   { return uintptr(unsafe.Pointer(&e.simpleVT)) }
func (e *uiaElement) fragmentPtr() uintptr { return uintptr(unsafe.Pointer(&e.fragmentVT)) }
func (e *uiaElement) rootPtr() uintptr     { return uintptr(unsafe.Pointer(&e.rootVT)) }
func (e *uiaElement) invokePtr() uintptr   { return uintptr(unsafe.Pointer(&e.invokeVT)) }
func (e *uiaElement) togglePtr() uintptr   { return uintptr(unsafe.Pointer(&e.toggleVT)) }
func (e *uiaElement) valuePtr() uintptr    { return uintptr(unsafe.Pointer(&e.valueVT)) }
func (e *uiaElement) textPtr() uintptr     { return uintptr(unsafe.Pointer(&e.textVT)) }
func (e *uiaElement) selItemPtr() uintptr  { return uintptr(unsafe.Pointer(&e.selItemVT)) }
func (e *uiaElement) expandPtr() uintptr   { return uintptr(unsafe.Pointer(&e.expandVT)) }

// isRoot — корень фрагмента (окно): у него живёт FragmentRoot и хост-провайдер.
func (e *uiaElement) isRoot() bool { return e.id == e.b.rootID() }

// node возвращает узел снимка, которому соответствует элемент (nil — узел
// исчез из дерева после перестройки UI, а COM-объект клиент ещё держит).
func (e *uiaElement) node() *a11yNode {
	if e == nil || e.b == nil {
		return nil
	}
	return e.b.current().node(e.id)
}

// role — роль узла элемента (RoleUnknown, если узла уже нет).
func (e *uiaElement) role() base.AccessRole {
	if n := e.node(); n != nil {
		return n.Info.Role
	}
	return base.RoleUnknown
}

// info — семантика узла элемента (пустая, если узла уже нет).
func (e *uiaElement) info() base.AccessInfo {
	if n := e.node(); n != nil {
		return n.Info
	}
	return base.AccessInfo{Role: base.RoleUnknown}
}

// ─── Поддержка паттернов по роли ─────────────────────────────────────────────

// uiaSupportsInvoke — узлы, у которых «нажатие» не имеет состояния: кнопка и
// пункт меню без отметки.
//
// Радиокнопке по спецификации положен SelectionItem, а не Invoke: клиент хочет
// уметь спросить «выбрана ли» (см. uiaSupportsSelectionItem). Флажок и
// переключатель сюда НЕ входят: у них Toggle.
func uiaSupportsInvoke(info base.AccessInfo) bool {
	switch info.Role {
	case base.RoleButton:
		return true
	case base.RoleMenuItem:
		return !a11yHasState(info.States, base.StateCheckable)
	}
	return false
}

// uiaSupportsToggle — узлы с двумя состояниями (Toggle вместо Invoke): флажок,
// переключатель и пункт меню с отметкой.
func uiaSupportsToggle(info base.AccessInfo) bool {
	switch info.Role {
	case base.RoleCheckBox, base.RoleSwitch:
		return true
	case base.RoleMenuItem:
		return a11yHasState(info.States, base.StateCheckable)
	}
	return false
}

// uiaSupportsSelectionItem — узлы, которые бывают «выбраны»: радиокнопка.
// Контейнера выбора (группы радиокнопок) движок не публикует, поэтому
// SelectionContainer отдаётся пустым — спецификация это допускает.
func uiaSupportsSelectionItem(info base.AccessInfo) bool {
	return info.Role == base.RoleRadioButton
}

// uiaSupportsExpandCollapse — узлы с раскрываемым содержимым: пункт меню с
// подменю. Раскрываемую группу без подменю (Expander) этим паттерном не
// объявляем: свёрнутое состояние у неё в дереве никак не помечено, а
// раскрыть её программно движок не умеет.
func uiaSupportsExpandCollapse(info base.AccessInfo) bool {
	return a11yHasState(info.States, base.StateHasPopup)
}

// ─── Таблицы виртуальных методов ─────────────────────────────────────────────

var (
	uiaVTOnce                             sync.Once
	uiaVTSimple, uiaVTFragment, uiaVTRoot uintptr
	uiaVTInvoke, uiaVTToggle, uiaVTValue  uintptr
	uiaVTSelItem, uiaVTExpand             uintptr
)

func uiaInitVTables() {
	uiaVTOnce.Do(func() {
		qi := windows.NewCallback(uiaQueryInterface)
		addRef := windows.NewCallback(uiaAddRef)
		release := windows.NewCallback(uiaRelease)

		uiaVTSimple = win32.NewVTable(qi, addRef, release,
			windows.NewCallback(uiaGetProviderOptions),
			windows.NewCallback(uiaGetPatternProvider),
			windows.NewCallback(uiaGetPropertyValue),
			windows.NewCallback(uiaGetHostProvider),
		)
		uiaVTFragment = win32.NewVTable(qi, addRef, release,
			windows.NewCallback(uiaNavigate),
			windows.NewCallback(uiaGetRuntimeID),
			windows.NewCallback(uiaGetBoundingRectangle),
			windows.NewCallback(uiaGetEmbeddedFragmentRoots),
			windows.NewCallback(uiaSetFocus),
			windows.NewCallback(uiaGetFragmentRoot),
		)
		uiaVTRoot = win32.NewVTable(qi, addRef, release,
			windows.NewCallback(uiaElementProviderFromPoint),
			windows.NewCallback(uiaGetFocus),
		)
		uiaVTInvoke = win32.NewVTable(qi, addRef, release,
			windows.NewCallback(uiaInvoke),
		)
		uiaVTToggle = win32.NewVTable(qi, addRef, release,
			windows.NewCallback(uiaToggle),
			windows.NewCallback(uiaGetToggleState),
		)
		uiaVTValue = win32.NewVTable(qi, addRef, release,
			windows.NewCallback(uiaSetValue),
			windows.NewCallback(uiaGetValue),
			windows.NewCallback(uiaGetValueIsReadOnly),
		)
		uiaVTSelItem = win32.NewVTable(qi, addRef, release,
			windows.NewCallback(uiaSelect),
			windows.NewCallback(uiaAddToSelection),
			windows.NewCallback(uiaRemoveFromSelection),
			windows.NewCallback(uiaGetIsSelected),
			windows.NewCallback(uiaGetSelectionContainer),
		)
		uiaVTExpand = win32.NewVTable(qi, addRef, release,
			windows.NewCallback(uiaExpand),
			windows.NewCallback(uiaCollapse),
			windows.NewCallback(uiaGetExpandCollapseState),
		)
	})
}

func uiaSimpleVTable() uintptr   { uiaInitVTables(); return uiaVTSimple }
func uiaFragmentVTable() uintptr { uiaInitVTables(); return uiaVTFragment }
func uiaRootVTable() uintptr     { uiaInitVTables(); return uiaVTRoot }
func uiaInvokeVTable() uintptr   { uiaInitVTables(); return uiaVTInvoke }
func uiaToggleVTable() uintptr   { uiaInitVTables(); return uiaVTToggle }
func uiaValueVTable() uintptr    { uiaInitVTables(); return uiaVTValue }
func uiaSelItemVTable() uintptr  { uiaInitVTables(); return uiaVTSelItem }
func uiaExpandVTable() uintptr   { uiaInitVTables(); return uiaVTExpand }

// ─── IUnknown ────────────────────────────────────────────────────────────────

func uiaQueryInterface(this uintptr, riid *win32.GUID, ppv *uintptr) uintptr {
	if ppv == nil {
		return win32.EInvalidArg
	}
	*ppv = 0
	e := uiaLookup(this)
	if e == nil {
		return win32.ENoInterface
	}
	switch {
	case riid.Equals(&win32.IIDIUnknown), riid.Equals(&iidProviderSimple):
		uiaLog("QI(%d, Simple)", e.id)
		*ppv = e.simplePtr()
	case riid.Equals(&iidProviderFragment):
		uiaLog("QI(%d, Fragment)", e.id)
		*ppv = e.fragmentPtr()
	case riid.Equals(&iidProviderFragmentRoot):
		if !e.isRoot() {
			uiaLog("QI(%d, FragmentRoot) → не корень, E_NOINTERFACE", e.id)
			return win32.ENoInterface
		}
		uiaLog("QI(%d, FragmentRoot)", e.id)
		*ppv = e.rootPtr()
	case riid.Equals(&iidInvokeProvider):
		// Отвечаем ТОЛЬКО если роль реально поддерживает паттерн: пустой
		// IInvokeProvider на надписи заставил бы скринридер объявить её
		// нажимаемой, а нажатие ничего бы не делало.
		if !uiaSupportsInvoke(e.info()) {
			uiaLog("QI(%d, Invoke) → роль не поддерживает, E_NOINTERFACE", e.id)
			return win32.ENoInterface
		}
		uiaLog("QI(%d, Invoke)", e.id)
		*ppv = e.invokePtr()
	case riid.Equals(&iidToggleProvider):
		if !uiaSupportsToggle(e.info()) {
			uiaLog("QI(%d, Toggle) → роль не поддерживает, E_NOINTERFACE", e.id)
			return win32.ENoInterface
		}
		uiaLog("QI(%d, Toggle)", e.id)
		*ppv = e.togglePtr()
	case riid.Equals(&iidSelectionItemProvider):
		if !uiaSupportsSelectionItem(e.info()) {
			uiaLog("QI(%d, SelectionItem) → роль не поддерживает, E_NOINTERFACE", e.id)
			return win32.ENoInterface
		}
		uiaLog("QI(%d, SelectionItem)", e.id)
		*ppv = e.selItemPtr()
	case riid.Equals(&iidExpandCollapseProvider):
		if !uiaSupportsExpandCollapse(e.info()) {
			uiaLog("QI(%d, ExpandCollapse) → не поддерживается, E_NOINTERFACE", e.id)
			return win32.ENoInterface
		}
		uiaLog("QI(%d, ExpandCollapse)", e.id)
		*ppv = e.expandPtr()
	case riid.Equals(&iidValueProvider):
		if !uiaSupportsValue(e.role()) {
			uiaLog("QI(%d, Value) → роль не поддерживает, E_NOINTERFACE", e.id)
			return win32.ENoInterface
		}
		uiaLog("QI(%d, Value)", e.id)
		*ppv = e.valuePtr()
	case riid.Equals(&iidTextProvider):
		// Как и Value: только там, где у виджета есть текст, и не у пароля.
		if !uiaSupportsText(e) {
			uiaLog("QI(%d, Text) → не поддерживается, E_NOINTERFACE", e.id)
			return win32.ENoInterface
		}
		uiaLog("QI(%d, Text)", e.id)
		*ppv = e.textPtr()
	default:
		uiaLog("QI(%d, %s) → E_NOINTERFACE", e.id, riid)
		return win32.ENoInterface
	}
	e.refs.Add(1)
	return win32.SOK
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
		return win32.EInvalidArg
	}
	*out = uiaProviderServerSide
	return win32.SOK
}

// uiaGetPatternProvider отдаёт провайдер паттерна управления по его id.
//
// Возвращаемый указатель уходит клиенту во владение, поэтому обязателен AddRef
// (как и в outFragment/outRoot). Неизвестный или неподдерживаемый ролью
// паттерн — NULL и S_OK: для UIA это штатный ответ «такого у меня нет».
func uiaGetPatternProvider(this uintptr, patternID int32, out *uintptr) uintptr {
	if out == nil {
		return win32.EInvalidArg
	}
	*out = 0
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	info := e.info()
	role := info.Role
	switch patternID {
	case uiaPatternInvoke:
		if uiaSupportsInvoke(info) {
			e.refs.Add(1)
			*out = e.invokePtr()
		}
	case uiaPatternToggle:
		if uiaSupportsToggle(info) {
			e.refs.Add(1)
			*out = e.togglePtr()
		}
	case uiaPatternSelectionItem:
		if uiaSupportsSelectionItem(info) {
			e.refs.Add(1)
			*out = e.selItemPtr()
		}
	case uiaPatternExpandCollapse:
		if uiaSupportsExpandCollapse(info) {
			e.refs.Add(1)
			*out = e.expandPtr()
		}
	case uiaPatternValue:
		if uiaSupportsValue(role) {
			e.refs.Add(1)
			*out = e.valuePtr()
		}
	case uiaPatternText:
		if uiaSupportsText(e) {
			e.refs.Add(1)
			*out = e.textPtr()
		}
	}
	uiaLog("Pattern(%d, %d, role=%s) → %#x", e.id, patternID, role, *out)
	return win32.SOK
}

func uiaGetPropertyValue(this uintptr, propID int32, out *comVariant) uintptr {
	if out == nil {
		return win32.EInvalidArg
	}
	out.setEmpty()
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	e.b.fillProperty(e.id, propID, out)
	uiaLog("Property(%d, %d) → vt=%d", e.id, propID, out.vt)
	return win32.SOK
}

// uiaGetHostProvider — у корня это провайдер самого окна (UIA доберёт из него
// свойства HWND), у остальных элементов — NULL.
func uiaGetHostProvider(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return win32.EInvalidArg
	}
	*out = 0
	e := uiaLookup(this)
	if e == nil || !e.isRoot() {
		return win32.SOK
	}
	// Хост-провайдер должен быть ОДНИМ И ТЕМ ЖЕ объектом при каждом запросе:
	// UIA сверяет элементы по идентичности COM-объекта, и новый экземпляр на
	// каждый вызов она принимает за новый элемент окна. Отдаём кэшированный —
	// с обязательным AddRef, потому что владение переходит клиенту.
	*out = e.b.hostProvider()
	uiaLog("HostProvider(%d) → %#x", e.id, *out)
	return win32.SOK
}

// ─── IRawElementProviderFragment ─────────────────────────────────────────────

func uiaNavigate(this uintptr, direction int32, out *uintptr) uintptr {
	if out == nil {
		return win32.EInvalidArg
	}
	*out = 0
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	target := e.b.navigate(e.id, direction)
	if target != nil {
		*out = target.outFragment()
	}
	uiaLog("Navigate(%d, dir=%d) → %s", e.id, direction, uiaTargetName(target))
	return win32.SOK
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
		return win32.EInvalidArg
	}
	*out = 0
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	*out = safeArrayOfInts([]int32{uiaAppendRuntimeID, e.id})
	uiaLog("RuntimeId(%d) → [%d %d] sa=%#x", e.id, uiaAppendRuntimeID, e.id, *out)
	return win32.SOK
}

// uiaRect — UiaRect (левый верхний угол, ширина, высота; экранные пиксели).
type uiaRect struct {
	left, top, width, height float64
}

func uiaGetBoundingRectangle(this uintptr, out *uiaRect) uintptr {
	if out == nil {
		return win32.EInvalidArg
	}
	*out = uiaRect{}
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	*out = e.b.boundsOf(e.id)
	return win32.SOK
}

func uiaGetEmbeddedFragmentRoots(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return win32.EInvalidArg
	}
	*out = 0
	return win32.SOK
}

// uiaSetFocus — программная передача фокуса: скринридер просит сделать элемент
// фокусированным (NVDA делает это при переходе по объектной навигации).
//
// Отказ (виджета нет, он скрыт/выключен или не принимает фокус) НЕ превращаем в
// ошибку: клиенты UIA плохо переносят HRESULT из SetFocus — «Экранный диктор»
// после него считает провайдер сломанным и перестаёт ходить по дереву. Причину
// видно в журнале (HEADLESS_GUI_UIA_LOG).
func uiaSetFocus(this uintptr) uintptr {
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	node := e.node()
	if node == nil {
		uiaLog("SetFocus(%d) → узла нет в снимке", e.id)
		return win32.SOK
	}
	// Узел из AccessChildren получает фокус сам (AccessActionHandler владельца),
	// а не его владелец целиком.
	if !accessibilityNodeAction(e.b.host, node, base.AccessFocus) {
		uiaLog("SetFocus(%d) → отказ (виджет не принимает фокус)", e.id)
		return win32.SOK
	}
	e.b.markDirty() // фокус переехал — снимок устарел
	uiaLog("SetFocus(%d) → ок", e.id)
	return win32.SOK
}

func uiaGetFragmentRoot(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return win32.EInvalidArg
	}
	*out = 0
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	root := e.b.element(e.b.rootID())
	if root != nil {
		*out = root.outRoot()
	}
	uiaLog("FragmentRoot(%d) → %s", e.id, uiaTargetName(root))
	return win32.SOK
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
		return win32.EInvalidArg
	}
	*out = 0
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	x, y, ok := cursorScreenPoint()
	if !ok {
		return win32.SOK
	}
	if hit := e.b.hitTestScreen(x, y); hit != nil {
		*out = hit.outFragment()
	}
	return win32.SOK
}

func uiaGetFocus(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return win32.EInvalidArg
	}
	*out = 0
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	f := e.b.focusElement()
	if f != nil {
		*out = f.outFragment()
	}
	uiaLog("GetFocus(%d) → %s", e.id, uiaTargetName(f))
	return win32.SOK
}

// ─── Паттерны управления: IInvokeProvider и IToggleProvider ──────────────────

// uiaDoNode — общая часть паттернов управления: выполнить над узлом действие
// kind (accessibilityNodeAction).
//
// Invoke, Toggle и SelectionItem.Select для обычного виджета ведут в одно и то
// же — синтетический клик по центру (engine.ActivateAccessible). Разница
// между ними только в том, ЧТО клиент об элементе думает: Invoke — действие
// без состояния, Toggle — переключение, после которого клиент перечитает
// ToggleState. Узел из AccessChildren получает действие от владельца
// (base.AccessActionHandler), а не кликом в центр владельца.
func uiaDoNode(this uintptr, what string, kind base.AccessActionKind) uintptr {
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	node := e.node()
	if node == nil {
		uiaLog("%s(%d) → узла нет в снимке", what, e.id)
		return uiaEElementNotAvailable
	}
	if !accessibilityNodeAction(e.b.host, node, kind) {
		uiaLog("%s(%d) → отказ (виджет выключен, скрыт или без границ)", what, e.id)
		return uiaEElementNotEnabled
	}
	// Состояние узла изменилось (флажок переключился, кнопка что-то сделала):
	// помечаем снимок устаревшим, чтобы следующий пересбор поднял события.
	// Штатный уведомитель UI сделал бы это и сам, но он подключается только
	// вместе с циклом событий (ensureEventLoop).
	e.b.markDirty()
	uiaLog("%s(%d) → ок", what, e.id)
	return win32.SOK
}

// uiaInvoke — IInvokeProvider::Invoke.
func uiaInvoke(this uintptr) uintptr { return uiaDoNode(this, "Invoke", base.AccessInvoke) }

// uiaToggle — IToggleProvider::Toggle. Отдельного «установить в состояние X» в
// паттерне нет: клиент переключает и перечитывает ToggleState.
func uiaToggle(this uintptr) uintptr { return uiaDoNode(this, "Toggle", base.AccessToggle) }

// ─── ISelectionItemProvider: радиокнопка ────────────────────────────────────

// uiaSelect — ISelectionItemProvider::Select: выбрать радиокнопку.
func uiaSelect(this uintptr) uintptr { return uiaDoNode(this, "Select", base.AccessSelect) }

// uiaSelected — выбран ли узел. Спрашивается у самого виджета, если он умеет
// отвечать (радиокнопка), а не берётся из снимка: снимок обновляется раз в
// сто пятьдесят миллисекунд, а клиент, выбравший пункт, тут же спросит о нём
// снова — и услышал бы прежний ответ.
func uiaSelected(node *a11yNode) bool {
	if s, ok := node.Widget.(interface{ IsSelected() bool }); ok && node.ActionID == "" {
		return s.IsSelected()
	}
	return a11yHasState(node.Info.States, base.StateSelected)
}

// uiaAddToSelection — ISelectionItemProvider::AddToSelection. Группа
// радиокнопок выбирает ровно одну, поэтому «добавить» значит выбрать; уже
// выбранную трогать незачем.
func uiaAddToSelection(this uintptr) uintptr {
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	node := e.node()
	if node == nil {
		return uiaEElementNotAvailable
	}
	if uiaSelected(node) {
		return win32.SOK
	}
	return uiaDoNode(this, "AddToSelection", base.AccessSelect)
}

// uiaRemoveFromSelection — ISelectionItemProvider::RemoveFromSelection.
// Снять выбор с радиокнопки нельзя — он переходит только на другую; для
// невыбранной это ничего не меняет.
func uiaRemoveFromSelection(this uintptr) uintptr {
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	node := e.node()
	if node == nil {
		return uiaEElementNotAvailable
	}
	if uiaSelected(node) {
		return uiaEInvalidOperation
	}
	return win32.SOK
}

// uiaGetIsSelected — ISelectionItemProvider::get_IsSelected.
func uiaGetIsSelected(this uintptr, out *int32) uintptr {
	if out == nil {
		return win32.EInvalidArg
	}
	*out = 0
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	node := e.node()
	if node == nil {
		return uiaEElementNotAvailable
	}
	if uiaSelected(node) {
		*out = 1 // здесь BOOL, а не VARIANT_BOOL: 1
	}
	uiaLog("IsSelected(%d) → %d", e.id, *out)
	return win32.SOK
}

// uiaGetSelectionContainer — ISelectionItemProvider::get_SelectionContainer.
// Группы радиокнопок в дереве нет — контейнер пуст (допустимо спецификацией).
func uiaGetSelectionContainer(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return win32.EInvalidArg
	}
	*out = 0
	return win32.SOK
}

// ─── IExpandCollapseProvider: пункт меню с подменю ──────────────────────────

// uiaExpand — IExpandCollapseProvider::Expand: раскрыть подменю пункта.
func uiaExpand(this uintptr) uintptr { return uiaDoNode(this, "Expand", base.AccessExpand) }

// uiaCollapse — IExpandCollapseProvider::Collapse: закрыть подменю пункта.
func uiaCollapse(this uintptr) uintptr { return uiaDoNode(this, "Collapse", base.AccessCollapse) }

// uiaGetExpandCollapseState — get_ExpandCollapseState.
func uiaGetExpandCollapseState(this uintptr, out *int32) uintptr {
	if out == nil {
		return win32.EInvalidArg
	}
	*out = uiaExpandCollapsed
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	node := e.node()
	if node == nil {
		return uiaEElementNotAvailable
	}
	if a11yHasState(node.Info.States, base.StateExpanded) {
		*out = uiaExpanded
	}
	uiaLog("ExpandCollapseState(%d) → %d", e.id, *out)
	return win32.SOK
}

// uiaGetToggleState — IToggleProvider::get_ToggleState.
// Третьего состояния (ToggleState_Indeterminate) движок не публикует.
func uiaGetToggleState(this uintptr, out *int32) uintptr {
	if out == nil {
		return win32.EInvalidArg
	}
	*out = uiaToggleOff
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	node := e.node()
	if node == nil {
		return uiaEElementNotAvailable
	}
	if a11yHasState(node.Info.States, base.StateChecked) {
		*out = uiaToggleOn
	}
	uiaLog("ToggleState(%d) → %d", e.id, *out)
	return win32.SOK
}

// cursorScreenPoint — позиция курсора в экранных координатах.
func cursorScreenPoint() (int32, int32, bool) {
	var pt struct{ X, Y int32 }
	r, _, _ := procUIAGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	if r == 0 {
		return 0, 0, false
	}
	return pt.X, pt.Y, true
}

// ─── Мост ────────────────────────────────────────────────────────────────────

func init() {
	native.RegisterAccessibility(func(h native.AccessHost) native.AccessBridge { return &uiaBridge{host: h} })
	// Запрос клиента доступности (скринридер, Инспектор UIA) приходит окну
	// бэкенда сообщением WM_GETOBJECT; бэкенд спрашивает об этом нас.
	win32.AddMessageHook(wmGetObject, uiaHandleGetObject)
}

// uiaBridge — состояние моста для одного окна.
type uiaBridge struct {
	host native.AccessHost

	mu      sync.RWMutex
	ids     a11yIDPool
	view    *a11yView
	prev    *a11yView
	stamp   time.Time
	dirty   bool
	elems   map[int32]*uiaElement
	hwnd    uintptr
	hostPtr atomic.Uintptr // кэш провайдера окна (см. hostProvider)

	// Что клиентам уже известно о тексте поля с фокусом (см. emitTextEvents).
	// Трогает только горутина цикла событий — замок не нужен.
	textSeen   *a11yTextState
	textSeenID int32

	notifier uint64
	stopCh   chan struct{}
	stopOnce sync.Once
	loopOnce sync.Once
}

// procUIAGetCursorPos — GetCursorPos для моста (свой LazyProc: user32 бэкенда
// Win32 в его пакете).
var procUIAGetCursorPos = windows.NewLazySystemDLL("user32.dll").NewProc("GetCursorPos")

// uiaBridges — мосты по hwnd: обработчик WM_GETOBJECT (win32.AddMessageHook)
// находит мост окна без правки структуры Win32Window.
var (
	uiaBridgeMu sync.RWMutex
	uiaBridges  = map[uintptr]*uiaBridge{}
)

// Start поднимает мост: он ничего не занимает в системе — просто становится
// готов отвечать на WM_GETOBJECT.
func (b *uiaBridge) Start() error {
	if !uiaAvailable() || !b.enabled() {
		return errUIAUnavailable
	}
	nw, ok := b.host.Native().(*win32.Win32Window)
	if !ok || nw.HWND() == 0 {
		return errUIAUnavailable
	}
	b.hwnd = nw.HWND()
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
		b.notifier = base.RegisterUINotifier(b.markDirty, nil)
		go b.eventLoop()
	})
}

// Stop снимает мост и отключает провайдеры от клиентов.
func (b *uiaBridge) Stop() {
	b.stopOnce.Do(func() {
		if b.stopCh != nil {
			close(b.stopCh)
		}
		base.UnregisterUINotifier(b.notifier)
		uiaBridgeMu.Lock()
		delete(uiaBridges, b.hwnd)
		uiaBridgeMu.Unlock()

		b.mu.Lock()
		for _, e := range b.elems {
			e.forget()
		}
		b.elems = map[int32]*uiaElement{}
		b.mu.Unlock()
		uiaForgetRangesOf(b) // диапазоны, которые клиент так и не отпустил

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
	if on, set := b.host.AccessibilityForced(); set {
		return on
	}
	return true
}

func (b *uiaBridge) markDirty() {
	b.mu.Lock()
	b.dirty = true
	b.mu.Unlock()
}

// hostProvider — провайдер самого окна (UiaHostProviderFromHwnd). Создаётся
// РОВНО ОДИН раз (в Start, на UI-потоке) и отдаётся клиенту с AddRef: каждый
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

	snap := accessibilitySnapshot(b.host)
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
			b.emitTextEvents()
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
		oldV.clear()
		newV.clear()
	}
	for _, idx := range ch.NameChanged {
		raiseProp(idx, uiaPropName, func(v *comVariant, n *a11yNode) { v.setString(n.Info.Name) })
	}
	for _, idx := range ch.ValueChanged {
		raiseProp(idx, uiaPropValueValue, func(v *comVariant, n *a11yNode) { v.setString(n.Info.Value) })
	}
	for _, idx := range ch.StateChanged {
		// Каждое изменившееся состояние поднимает своё свойство: клиенту,
		// слушающему IsSelected радиокнопки, незачем будить его из-за
		// IsEnabled, и наоборот.
		prev, cur := prevV.Snap.node(idx), curV.Snap.node(idx)
		if prev == nil || cur == nil {
			continue
		}
		differs := func(state string) bool {
			return a11yHasState(prev.Info.States, state) != a11yHasState(cur.Info.States, state)
		}
		if differs(base.StateDisabled) {
			raiseProp(idx, uiaPropIsEnabled, func(v *comVariant, n *a11yNode) {
				v.setBool(!a11yHasState(n.Info.States, base.StateDisabled))
			})
		}
		if differs(base.StateOffscreen) {
			raiseProp(idx, uiaPropIsOffscreen, func(v *comVariant, n *a11yNode) {
				v.setBool(a11yHasState(n.Info.States, base.StateOffscreen))
			})
		}
		if differs(base.StateSelected) && uiaSupportsSelectionItem(cur.Info) {
			raiseProp(idx, uiaPropSelectionItemIsSel, func(v *comVariant, n *a11yNode) {
				v.setBool(a11yHasState(n.Info.States, base.StateSelected))
			})
		}
		if differs(base.StateChecked) && uiaSupportsToggle(cur.Info) {
			raiseProp(idx, uiaPropToggleState, func(v *comVariant, n *a11yNode) {
				state := int32(uiaToggleOff)
				if a11yHasState(n.Info.States, base.StateChecked) {
					state = uiaToggleOn
				}
				v.setI4(state)
			})
		}
		if differs(base.StateExpanded) && uiaSupportsExpandCollapse(cur.Info) {
			raiseProp(idx, uiaPropExpandCollapseState, func(v *comVariant, n *a11yNode) {
				state := int32(uiaExpandCollapsed)
				if a11yHasState(n.Info.States, base.StateExpanded) {
					state = uiaExpanded
				}
				v.setI4(state)
			})
		}
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
			name = b.host.Title()
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
		// У пункта меню «описание» — сочетание клавиш: оно отдаётся как
		// AcceleratorKey, а не как справка.
		if node.Info.Role != base.RoleMenuItem {
			out.setString(node.Info.Description)
		}
	case uiaPropAcceleratorKey:
		if node.Info.Role == base.RoleMenuItem {
			out.setString(node.Info.Description)
		}
	case uiaPropIsDialog:
		out.setBool(node.Info.Role == base.RoleDialog)
	case uiaPropSelectionItemIsSel:
		out.setBool(a11yHasState(node.Info.States, base.StateSelected))
	case uiaPropToggleState:
		if uiaSupportsToggle(node.Info) {
			state := int32(uiaToggleOff)
			if a11yHasState(node.Info.States, base.StateChecked) {
				state = uiaToggleOn
			}
			out.setI4(state)
		}
	case uiaPropExpandCollapseState:
		if uiaSupportsExpandCollapse(node.Info) {
			state := int32(uiaExpandCollapsed)
			if a11yHasState(node.Info.States, base.StateExpanded) {
				state = uiaExpanded
			}
			out.setI4(state)
		}
	case uiaPropIsEnabled:
		out.setBool(!a11yHasState(node.Info.States, base.StateDisabled))
	case uiaPropHasKeyboardFocus:
		out.setBool(v.Snap.Focus >= 0 && v.id(v.Snap.Focus) == id)
	case uiaPropIsKeyboardFocusabl:
		out.setBool(uiaFocusable(node.Info))
	case uiaPropIsControlElement, uiaPropIsContentElement:
		out.setBool(true)
	case uiaPropIsOffscreen:
		// Прокручен за край окна прокрутки или закрыт (меню, которое не
		// показано): на экране элемента нет.
		out.setBool(a11yHasState(node.Info.States, base.StateOffscreen))
	case uiaPropIsPassword:
		out.setBool(a11yHasState(node.Info.States, base.StatePassword))
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
	scale := b.host.Scale()
	if scale <= 0 {
		scale = 1
	}
	// Видимая часть: у элемента, прокрученного за край, границ на экране нет
	// (пустой прямоугольник — вместе с IsOffscreen), у выступающего — его
	// пересечение с окном прокрутки.
	r := a11yVisibleRect(node.Info)
	if r.Empty() {
		return uiaRect{}
	}
	ox, oy := 0, 0
	if b.host.Native() != nil {
		ox, oy = b.host.Native().GetPosition()
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
	scale := b.host.Scale()
	if scale <= 0 {
		scale = 1
	}
	ox, oy := 0, 0
	if b.host.Native() != nil {
		ox, oy = b.host.Native().GetPosition()
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

// ─── IValueProvider ──────────────────────────────────────────────────────────
//
// Содержимое поля ввода скринридер читает через этот паттерн. Без него текст
// доезжал до него одной строкой-«значением» элемента, как у ползунка: для
// кнопки довольно, для поля ввода — нет, и NVDA с «Экранным диктором»
// объявляли поле пустым.

// uiaSupportsValue — роли, у которых есть текстовое содержимое.
func uiaSupportsValue(r base.AccessRole) bool {
	switch r {
	case base.RoleTextInput, base.RoleDocument:
		return true
	}
	return false
}

// uiaTextOf возвращает текстовую семантику виджета элемента.
func uiaTextOf(e *uiaElement) (base.AccessTextProvider, bool) {
	node := e.node()
	if node == nil || node.Widget == nil {
		return nil, false
	}
	return base.AccessTextOf(node.Widget)
}

// uiaGetValue — IValueProvider::get_Value.
//
// Текст спрашивается у виджета, а не берётся из снимка семантики: снимок
// пересобирается раз в сто пятьдесят миллисекунд и отставал бы от набора.
func uiaGetValue(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return win32.EInvalidArg
	}
	*out = 0
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	text := ""
	if tp, ok := uiaTextOf(e); ok {
		text = tp.AccessText()
	} else if node := e.node(); node != nil {
		text = node.Info.Value
	} else {
		return uiaEElementNotAvailable
	}
	var v comVariant
	v.setString(text)
	*out = v.val[0] // BSTR уходит клиенту во владение
	uiaLog("get_Value(%d) → %d симв.", e.id, len([]rune(text)))
	return win32.SOK
}

// uiaGetValueIsReadOnly — IValueProvider::get_IsReadOnly.
func uiaGetValueIsReadOnly(this uintptr, out *int32) uintptr {
	if out == nil {
		return win32.EInvalidArg
	}
	*out = 1
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	if tp, ok := uiaTextOf(e); ok {
		if !tp.AccessReadOnly() {
			*out = 0
		}
		return win32.SOK
	}
	node := e.node()
	if node == nil {
		return uiaEElementNotAvailable
	}
	if !a11yHasState(node.Info.States, base.StateReadOnly) &&
		!a11yHasState(node.Info.States, base.StateDisabled) {
		*out = 0
	}
	return win32.SOK
}

// uiaSetValue — IValueProvider::SetValue: средство автоматизации (или сам
// скринридер) заменяет содержимое поля.
//
// Правка идёт на горутине движка: дерево виджетов принадлежит ей, а вызов
// приходит из потока UIA.
func uiaSetValue(this uintptr, bstr uintptr) uintptr {
	e := uiaLookup(this)
	if e == nil {
		return win32.EFail
	}
	tp, ok := uiaTextOf(e)
	if !ok {
		return uiaEElementNotAvailable
	}
	setter, ok := tp.(base.AccessTextSetter)
	if !ok || tp.AccessReadOnly() {
		return uiaEElementNotEnabled
	}
	text := bstrToString(bstr)
	e.b.host.PostToEngine(func() { setter.AccessSetText(text) })
	uiaLog("SetValue(%d, %d симв.)", e.id, len([]rune(text)))
	return win32.SOK
}
