//go:build windows

// a11y_uia_windows.go — COM-обвязка моста UI Automation: таблицы виртуальных
// методов, IUnknown, VARIANT, SAFEARRAY и импорты uiautomationcore.
//
// CGO в проекте нет, поэтому COM-объекты собираются руками: объект — это
// структура, первым словом которой лежит указатель на vtable, а vtable —
// массив указателей на функции из windows.NewCallback. Клиент (UIA) получает
// адрес поля-vtable и вызывает методы по смещениям, как и для C++-объекта.
//
// Ограничение платформы: windows.NewCallback не умеет принимать аргументы с
// плавающей точкой (они приходят в XMM-регистрах, до которых Go-колбэк не
// добирается). Это касается единственного метода — ElementProviderFromPoint;
// как с ним поступаем, описано в a11y_windows.go.

package a11y

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oops1/headless-gui/v3/platform/win32"
)

// errUIAUnavailable — мост поднять не на чем: нет uiautomationcore.dll или
// окно ещё не создано.
var errUIAUnavailable = errors.New("window: UI Automation недоступна")

// ─── Журнал вызовов провайдера ───────────────────────────────────────────────
//
// Клиент UIA живёт в чужом процессе, отладчиком его не прощупать: единственный
// способ понять, ЧТО именно он спрашивает у провайдера и в каком порядке —
// писать вызовы в файл. Включается переменной окружения:
//
//	HEADLESS_GUI_UIA_LOG=C:\путь\uia.log
//
// Без неё не стоит ничего (проверка одного указателя).

var (
	uiaLogMu   sync.Mutex
	uiaLogFile *os.File
	uiaLogOnce sync.Once
	uiaLogOn   bool
	uiaLogT0   time.Time
)

// uiaLog пишет строку в журнал вызовов провайдера, если он включён.
func uiaLog(format string, args ...any) {
	uiaLogOnce.Do(func() {
		path := os.Getenv("HEADLESS_GUI_UIA_LOG")
		if path == "" {
			return
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return
		}
		uiaLogFile, uiaLogOn, uiaLogT0 = f, true, time.Now()
	})
	if !uiaLogOn {
		return
	}
	uiaLogMu.Lock()
	defer uiaLogMu.Unlock()
	fmt.Fprintf(uiaLogFile, "%8.3f [%d] %s\n", time.Since(uiaLogT0).Seconds(),
		windows.GetCurrentThreadId(), fmt.Sprintf(format, args...))
}

// ─── HRESULT ─────────────────────────────────────────────────────────────────

const (
	uiaEBadPoint = uintptr(0x80131515) // не используется, оставлено для полноты

	// UIA_E_ELEMENTNOTENABLED / UIA_E_ELEMENTNOTAVAILABLE — штатные отказы
	// паттернов управления: «элемент выключен» и «элемента больше нет».
	// Клиенты их ждут и обрабатывают (в отличие от ошибок из SetFocus).
	uiaEElementNotEnabled   = uintptr(0x80040200)
	uiaEElementNotAvailable = uintptr(0x80040201)
)

// ─── GUID ────────────────────────────────────────────────────────────────────

// comAddRef вызывает IUnknown::AddRef чужого COM-объекта (слот 1 в vtable).
// Нужен, когда мы отдаём клиенту КЭШИРОВАННЫЙ указатель: без этого клиент
// освободит объект, которым мы продолжаем пользоваться.
//
// go vet ругается здесь на «possible misuse of unsafe.Pointer»: чтение чужой
// таблицы методов иначе и не делается — адрес приходит из ОС, а не из Go.
func comAddRef(obj uintptr) {
	if obj == 0 {
		return
	}
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + unsafe.Sizeof(uintptr(0))))
	syscall.SyscallN(fn, obj)
}

var (
	// IRawElementProviderSimple {D6DD68D1-86FD-4332-8666-9ABEDEA2D24C}
	iidProviderSimple = win32.GUID{Data1: 0xD6DD68D1, Data2: 0x86FD, Data3: 0x4332, Data4: [8]byte{0x86, 0x66, 0x9A, 0xBE, 0xDE, 0xA2, 0xD2, 0x4C}}
	// IRawElementProviderFragment {F7063DA8-8359-439C-9297-BBC5299A7D87}
	iidProviderFragment = win32.GUID{Data1: 0xF7063DA8, Data2: 0x8359, Data3: 0x439C, Data4: [8]byte{0x92, 0x97, 0xBB, 0xC5, 0x29, 0x9A, 0x7D, 0x87}}
	// IRawElementProviderFragmentRoot {620CE2A5-AB8F-40A9-86CB-DE3C75599B58}
	iidProviderFragmentRoot = win32.GUID{Data1: 0x620CE2A5, Data2: 0xAB8F, Data3: 0x40A9, Data4: [8]byte{0x86, 0xCB, 0xDE, 0x3C, 0x75, 0x59, 0x9B, 0x58}}
	// IInvokeProvider {54FCB24B-E18E-47A2-B4D3-ECCBE77599A2} — паттерн Invoke.
	iidInvokeProvider = win32.GUID{Data1: 0x54FCB24B, Data2: 0xE18E, Data3: 0x47A2, Data4: [8]byte{0xB4, 0xD3, 0xEC, 0xCB, 0xE7, 0x75, 0x99, 0xA2}}
	// IToggleProvider {56D00BD0-C4F4-433C-A836-1A52A57E0892} — паттерн Toggle.
	iidToggleProvider = win32.GUID{Data1: 0x56D00BD0, Data2: 0xC4F4, Data3: 0x433C, Data4: [8]byte{0xA8, 0x36, 0x1A, 0x52, 0xA5, 0x7E, 0x08, 0x92}}
	// ISelectionItemProvider {2ACAD808-B2D4-452D-A407-91FF1AD167B2} — паттерн
	// SelectionItem: «выбрана ли» радиокнопка и выбор её.
	iidSelectionItemProvider = win32.GUID{Data1: 0x2ACAD808, Data2: 0xB2D4, Data3: 0x452D, Data4: [8]byte{0xA4, 0x07, 0x91, 0xFF, 0x1A, 0xD1, 0x67, 0xB2}}
	// IExpandCollapseProvider {D847D3A5-CAB0-4A98-8C32-ECB45C59AD24} — паттерн
	// ExpandCollapse: раскрытие подменю.
	iidExpandCollapseProvider = win32.GUID{Data1: 0xD847D3A5, Data2: 0xCAB0, Data3: 0x4A98, Data4: [8]byte{0x8C, 0x32, 0xEC, 0xB4, 0x5C, 0x59, 0xAD, 0x24}}
	// IValueProvider {C7935180-6FB3-4201-B174-7DF73ADBF64A} — паттерн Value:
	// содержимое поля ввода как строка.
	iidValueProvider = win32.GUID{Data1: 0xC7935180, Data2: 0x6FB3, Data3: 0x4201, Data4: [8]byte{0xB1, 0x74, 0x7D, 0xF7, 0x3A, 0xDB, 0xF6, 0x4A}}
	// ITextProvider {3589C92C-63F3-4367-99BB-ADA653B77CF2} — паттерн Text:
	// содержимое как документ, читаемый кусками.
	iidTextProvider = win32.GUID{Data1: 0x3589C92C, Data2: 0x63F3, Data3: 0x4367, Data4: [8]byte{0x99, 0xBB, 0xAD, 0xA6, 0x53, 0xB7, 0x7C, 0xF2}}
	// ITextRangeProvider {5347AD7B-C355-46F8-AFF5-909033582F63} — диапазон
	// текста внутри паттерна Text.
	iidTextRangeProvider = win32.GUID{Data1: 0x5347AD7B, Data2: 0xC355, Data3: 0x46F8, Data4: [8]byte{0xAF, 0xF5, 0x90, 0x90, 0x33, 0x58, 0x2F, 0x63}}
)

// ─── Импорты ─────────────────────────────────────────────────────────────────

var (
	oleaut32 = windows.NewLazySystemDLL("oleaut32.dll")
	uiaCore  = windows.NewLazySystemDLL("uiautomationcore.dll")

	procSysAllocString        = oleaut32.NewProc("SysAllocString")
	procSysFreeString         = oleaut32.NewProc("SysFreeString")
	procSafeArrayCreateVector = oleaut32.NewProc("SafeArrayCreateVector")
	procSafeArrayPutElement   = oleaut32.NewProc("SafeArrayPutElement")

	procUiaReturnRawElementProvider = uiaCore.NewProc("UiaReturnRawElementProvider")
	procUiaHostProviderFromHwnd     = uiaCore.NewProc("UiaHostProviderFromHwnd")
	procUiaRaiseAutomationEvent     = uiaCore.NewProc("UiaRaiseAutomationEvent")
	procUiaRaiseStructureChanged    = uiaCore.NewProc("UiaRaiseStructureChangedEvent")
	procUiaRaisePropertyChanged     = uiaCore.NewProc("UiaRaiseAutomationPropertyChangedEvent")
	procUiaDisconnectAllProviders   = uiaCore.NewProc("UiaDisconnectAllProviders")
	procUiaClientsAreListening      = uiaCore.NewProc("UiaClientsAreListening")
	procUiaGetReservedNotSupported  = uiaCore.NewProc("UiaGetReservedNotSupportedValue")
)

// uiaAvailable — есть ли в системе uiautomationcore.dll (нет — мост молчит).
func uiaAvailable() bool { return uiaCore.Load() == nil }

// uiaClientsListening — подключён ли хоть один клиент доступности.
// Пока никто не слушает, события не рассылаем.
func uiaClientsListening() bool {
	if uiaCore.Load() != nil || procUiaClientsAreListening.Find() != nil {
		return false
	}
	r, _, _ := procUiaClientsAreListening.Call()
	return r != 0
}

// ─── VARIANT ─────────────────────────────────────────────────────────────────

// Типы VARIANT, используемые провайдером.
const (
	vtEmpty = 0
	vtI4    = 3
	vtR8    = 5
	vtBSTR  = 8
	vtBool  = 11
)

// comVariant — VARIANT (24 байта на x64: заголовок 8 + объединение 16).
type comVariant struct {
	vt         uint16
	r1, r2, r3 uint16
	val        [2]uintptr
}

func (v *comVariant) setEmpty() { v.vt = vtEmpty; v.val = [2]uintptr{} }

func (v *comVariant) setI4(n int32) {
	v.vt = vtI4
	v.val = [2]uintptr{uintptr(uint32(n)), 0}
}

func (v *comVariant) setR8(f float64) {
	v.vt = vtR8
	v.val = [2]uintptr{uintptr(*(*uint64)(unsafe.Pointer(&f))), 0}
}

// setBool кладёт VARIANT_BOOL (-1 = TRUE, 0 = FALSE).
func (v *comVariant) setBool(b bool) {
	v.vt = vtBool
	var n uint16
	if b {
		n = 0xFFFF
	}
	v.val = [2]uintptr{uintptr(n), 0}
}

// setString кладёт BSTR; out-параметр освобождает UIA, in — clear().
func (v *comVariant) setString(s string) {
	if s == "" {
		v.setEmpty()
		return
	}
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		v.setEmpty()
		return
	}
	bstr, _, _ := procSysAllocString.Call(uintptr(unsafe.Pointer(p)))
	if bstr == 0 {
		v.setEmpty()
		return
	}
	v.vt = vtBSTR
	v.val = [2]uintptr{bstr, 0}
}

// clear освобождает BSTR входного VARIANT (in-параметры — наши).
func (v *comVariant) clear() {
	if v.vt == vtBSTR && v.val[0] != 0 {
		procSysFreeString.Call(v.val[0])
	}
	v.setEmpty()
}

// ─── SAFEARRAY ───────────────────────────────────────────────────────────────

// safeArrayOfInts создаёт SAFEARRAY(VT_I4) — так UIA принимает RuntimeId.
// Возвращает 0, если выделить массив не удалось.
func safeArrayOfInts(vals []int32) uintptr {
	sa, _, _ := procSafeArrayCreateVector.Call(uintptr(vtI4), 0, uintptr(len(vals)))
	if sa == 0 {
		return 0
	}
	for i := range vals {
		idx := int32(i)
		procSafeArrayPutElement.Call(sa, uintptr(unsafe.Pointer(&idx)),
			uintptr(unsafe.Pointer(&vals[i])))
	}
	return sa
}

// ─── Таблицы виртуальных методов ─────────────────────────────────────────────

// bstrToString читает строку COM (BSTR) — указатель на UTF-16 с длиной перед
// ним. Нужен для входных параметров: SetValue приходит именно в таком виде.
//
// Длину берём из заголовка BSTR (четыре байта перед данными), а не ищем NUL:
// строка вправе содержать нули внутри, и поиск оборвал бы её на первом.
func bstrToString(bstr uintptr) string {
	if bstr == 0 {
		return ""
	}
	nBytes := *(*uint32)(unsafe.Pointer(bstr - 4))
	n := int(nBytes / 2)
	if n <= 0 || n > 1<<20 {
		return ""
	}
	buf := unsafe.Slice((*uint16)(unsafe.Pointer(bstr)), n)
	return windows.UTF16ToString(buf)
}
