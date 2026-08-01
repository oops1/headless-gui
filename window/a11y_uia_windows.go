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
package window

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
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
	sOK          = uintptr(0)
	eNoInterface = uintptr(0x80004002)
	eInvalidArg  = uintptr(0x80070057)
	eFail        = uintptr(0x80004005)
	eNotImpl     = uintptr(0x80004001)
	uiaEBadPoint = uintptr(0x80131515) // не используется, оставлено для полноты
)

// ─── GUID ────────────────────────────────────────────────────────────────────

// comGUID — структура GUID в раскладке Windows.
type comGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

func (g *comGUID) equals(o *comGUID) bool {
	if g == nil || o == nil {
		return false
	}
	return *g == *o
}

// String форматирует GUID как {XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX} —
// нужно, чтобы в журнале было видно, какой интерфейс просит клиент.
func (g *comGUID) String() string {
	if g == nil {
		return "<nil>"
	}
	return fmt.Sprintf("{%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X}",
		g.Data1, g.Data2, g.Data3, g.Data4[0], g.Data4[1],
		g.Data4[2], g.Data4[3], g.Data4[4], g.Data4[5], g.Data4[6], g.Data4[7])
}

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
	iidIUnknown = comGUID{0x00000000, 0x0000, 0x0000, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	// IRawElementProviderSimple {D6DD68D1-86FD-4332-8666-9ABEDEA2D24C}
	iidProviderSimple = comGUID{0xD6DD68D1, 0x86FD, 0x4332, [8]byte{0x86, 0x66, 0x9A, 0xBE, 0xDE, 0xA2, 0xD2, 0x4C}}
	// IRawElementProviderFragment {F7063DA8-8359-439C-9297-BBC5299A7D87}
	iidProviderFragment = comGUID{0xF7063DA8, 0x8359, 0x439C, [8]byte{0x92, 0x97, 0xBB, 0xC5, 0x29, 0x9A, 0x7D, 0x87}}
	// IRawElementProviderFragmentRoot {620CE2A5-AB8F-40A9-86CB-DE3C75599B58}
	iidProviderFragmentRoot = comGUID{0x620CE2A5, 0xAB8F, 0x40A9, [8]byte{0x86, 0xCB, 0xDE, 0x3C, 0x75, 0x59, 0x9B, 0x58}}
)

// ─── Импорты ─────────────────────────────────────────────────────────────────

var (
	oleaut32 = windows.NewLazySystemDLL("oleaut32.dll")
	uiaCore  = windows.NewLazySystemDLL("uiautomationcore.dll")

	procSysAllocString        = oleaut32.NewProc("SysAllocString")
	procSafeArrayCreateVector = oleaut32.NewProc("SafeArrayCreateVector")
	procSafeArrayPutElement   = oleaut32.NewProc("SafeArrayPutElement")

	procUiaReturnRawElementProvider = uiaCore.NewProc("UiaReturnRawElementProvider")
	procUiaHostProviderFromHwnd     = uiaCore.NewProc("UiaHostProviderFromHwnd")
	procUiaRaiseAutomationEvent     = uiaCore.NewProc("UiaRaiseAutomationEvent")
	procUiaRaiseStructureChanged    = uiaCore.NewProc("UiaRaiseStructureChangedEvent")
	procUiaRaisePropertyChanged     = uiaCore.NewProc("UiaRaiseAutomationPropertyChangedEvent")
	procUiaDisconnectAllProviders   = uiaCore.NewProc("UiaDisconnectAllProviders")
	procUiaClientsAreListening      = uiaCore.NewProc("UiaClientsAreListening")
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

// setString кладёт BSTR. Строку выделяет SysAllocString — владение переходит
// вызывающей стороне (UIA освободит через VariantClear).
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

var (
	vtableMu   sync.Mutex
	vtableKeep [][]uintptr // держим таблицы живыми: их адреса ушли в чужой код
)

// newVTable собирает vtable из указателей на функции и возвращает адрес
// первого элемента. Слайс сохраняется глобально — сборщик мусора не должен
// освободить память, на которую смотрит UIA.
func newVTable(fns ...uintptr) uintptr {
	tbl := make([]uintptr, len(fns))
	copy(tbl, fns)
	vtableMu.Lock()
	vtableKeep = append(vtableKeep, tbl)
	vtableMu.Unlock()
	return uintptr(unsafe.Pointer(&tbl[0]))
}
