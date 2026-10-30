//go:build windows

package win32

// dnd_windows.go — перетаскивание файлов на Win32 через OLE (v3.34).
//
// Раньше окно принимало файлы только через WM_DROPFILES (DragAcceptFiles):
// система сообщала лишь о самом сбросе, а войти/двигаться/выйти приёмник не
// знал. Теперь окно регистрирует IDropTarget (RegisterDragDrop) — отсюда и
// события DragEnter/DragOver/DragLeave, и сброс (Drop). WM_DROPFILES остаётся
// запасным путём: если OLE не поднялся или регистрация отказала, окно
// включает DragAcceptFiles, как раньше, и SetOnFilesDropped работает прежним
// образом. Поведение SetOnFilesDropped не меняется ни в том, ни в другом
// случае.
//
// Источник (BeginDragFiles) — DoDragDrop с двумя COM-объектами: IDataObject
// отдаёт список файлов форматом CF_HDROP, IDropSource решает, когда сбросить
// или отменить.
//
// CGO в проекте нет, поэтому COM-объекты собираются руками, как в
// platform/a11y/a11y_uia_windows.go: объект — структура с указателем на vtable первым
// словом, vtable — массив указателей из windows.NewCallback. Один тип
// объекта (dndComObj) служит всем трём интерфейсам; различает их таблица.
// Адрес объекта отдаётся ОС числом, а по числу объект находится в реестре —
// никаких обратных преобразований uintptr в указатель на Go-память.
//
// Значок перетаскивания (BeginDragFilesImage): оболочка рисует его сама, если
// объект IDataObject хранит «DragImageBits» и приёмник вызывает
// IDropTargetHelper. Первое делает IDragSourceHelper.InitializeFromBitmap —
// он кладёт картинку и служебные форматы в наш объект через SetData (поэтому
// SetData/GetData хранят произвольные блоки HGLOBAL), второе — приёмник окна
// (dropTargetHelper): так значок виден и над самим окном, не только над
// чужими программами.
//
// Ограничение: POINTL приходит в IDropTarget по значению, и разбор «одним
// машинным словом» верен для 64-разрядных Windows (amd64, arm64) — на них
// пакет и собирается.

import (
	"image"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// ─── Импорты и константы ─────────────────────────────────────────────────────

var (
	ole32 = windows.NewLazySystemDLL("ole32.dll")

	procOleInitialize    = ole32.NewProc("OleInitialize")
	procRegisterDragDrop = ole32.NewProc("RegisterDragDrop")
	procRevokeDragDrop   = ole32.NewProc("RevokeDragDrop")
	procDoDragDrop       = ole32.NewProc("DoDragDrop")
	procReleaseStgMedium = ole32.NewProc("ReleaseStgMedium")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCreateStreamOnHG = ole32.NewProc("CreateStreamOnHGlobal")

	procSHCreateStdEnumFmtEtc = shell32.NewProc("SHCreateStdEnumFmtEtc")

	kernel32dnd       = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalAlloc   = kernel32dnd.NewProc("GlobalAlloc")
	procGlobalLock    = kernel32dnd.NewProc("GlobalLock")
	procGlobalUnlock  = kernel32dnd.NewProc("GlobalUnlock")
	procGlobalSize    = kernel32dnd.NewProc("GlobalSize")
	procGlobalFree    = kernel32dnd.NewProc("GlobalFree")
	procGetAsyncKeyst = user32.NewProc("GetAsyncKeyState")
)

const (
	cfHDropFormat   = 15 // CF_HDROP
	tymedHGlobal    = 1  // TYMED_HGLOBAL
	tymedIStream    = 4  // TYMED_ISTREAM
	dvAspectContent = 1  // DVASPECT_CONTENT
	dataDirGet      = 1  // DATADIR_GET
	gmemMoveableDnd = 0x0002

	sFalse          = uintptr(1)
	dvEFormatEtc    = uintptr(0x80040064) // DV_E_FORMATETC
	dvETymed        = uintptr(0x80040069) // DV_E_TYMED
	dvEAspect       = uintptr(0x8004006B) // DV_E_DVASPECT
	oleEAdviseNoSup = uintptr(0x80040003) // OLE_E_ADVISENOTSUPPORTED
	dataSSameFmt    = uintptr(0x00040130) // DATA_S_SAMEFORMATETC

	clsctxInprocServer = 1 // CLSCTX_INPROC_SERVER

	smSwapButton = 23 // SM_SWAPBUTTON
	vkLButton    = 0x01
	vkRButton    = 0x02
	vkMButton    = 0x04
)

// IID интерфейсов, на которые отвечают наши объекты.
var (
	iidIDropTarget = GUID{0x00000122, 0x0000, 0x0000, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIDropSource = GUID{0x00000121, 0x0000, 0x0000, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIDataObject = GUID{0x0000010E, 0x0000, 0x0000, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}

	// Помощники оболочки для значка перетаскивания.
	clsidDragDropHelper  = GUID{0x4657278A, 0x411B, 0x11D2, [8]byte{0x83, 0x9A, 0x00, 0xC0, 0x4F, 0xD9, 0x18, 0xD0}}
	iidIDragSourceHelper = GUID{0xDE5BF786, 0x477A, 0x11D2, [8]byte{0x83, 0x9D, 0x00, 0xC0, 0x4F, 0xD9, 0x18, 0xD0}}
	iidIDropTargetHelper = GUID{0x4657278B, 0x411B, 0x11D2, [8]byte{0x83, 0x9A, 0x00, 0xC0, 0x4F, 0xD9, 0x18, 0xD0}}
)

// formatEtc — FORMATETC (32 байта на x64: поля раскладываются как в C).
type formatEtc struct {
	cfFormat uint16
	ptd      uintptr
	dwAspect uint32
	lindex   int32
	tymed    uint32
}

// stgMedium — STGMEDIUM (24 байта на x64).
type stgMedium struct {
	tymed          uint32
	data           uintptr
	pUnkForRelease uintptr
}

// ptrAt превращает число-адрес чужой памяти (аргумент COM-метода) в указатель.
// Обход через unsafe.Pointer(&p) — единственный способ сделать это так, чтобы
// go vet не видел здесь ошибки: адрес пришёл от ОС, а не из Go.
func ptrAt[T any](p uintptr) *T {
	return (*T)(*(*unsafe.Pointer)(unsafe.Pointer(&p)))
}

// comMethod возвращает адрес метода COM-объекта по номеру слота vtable.
func comMethod(obj uintptr, slot int) uintptr {
	vtbl := *ptrAt[uintptr](obj)
	return *ptrAt[uintptr](vtbl + uintptr(slot)*unsafe.Sizeof(uintptr(0)))
}

// comInvoke вызывает метод чужого COM-объекта по номеру слота vtable.
//
// uintptrescapes: аргументы-адреса (uintptr(unsafe.Pointer(&x))) обязаны
// остаться в куче, пока идёт вызов, — метод может выполняться Go-колбэком на
// этой же горутине (приёмник и источник в одном процессе), а рост её стека
// унёс бы переменную из-под записи ответа.
//
//go:uintptrescapes
func comInvoke(obj uintptr, slot int, args ...uintptr) uintptr {
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, obj)
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(comMethod(obj, slot), all...)
	return r
}

// Слоты vtable: IUnknown (0..2), затем методы интерфейсов.
const (
	slotUnkRelease = 2

	slotStreamRead = 3 // ISequentialStream::Read
	slotStreamSeek = 5 // IStream::Seek

	slotDataGetData       = 3 // IDataObject::GetData
	slotDataQueryGetData  = 5 // IDataObject::QueryGetData
	slotDataEnumFormatEtc = 8 // IDataObject::EnumFormatEtc

	slotDragSrcFromBitmap = 3 // IDragSourceHelper::InitializeFromBitmap

	slotDropHelperEnter = 3 // IDropTargetHelper::DragEnter
	slotDropHelperLeave = 4 // IDropTargetHelper::DragLeave
	slotDropHelperOver  = 5 // IDropTargetHelper::DragOver
	slotDropHelperDrop  = 6 // IDropTargetHelper::Drop
)

// ─── COM-объект ──────────────────────────────────────────────────────────────

type dndKind int

const (
	dndKindTarget dndKind = iota // IDropTarget окна
	dndKindData                  // IDataObject со списком файлов
	dndKindSource                // IDropSource
)

// dndComObj — один COM-объект перетаскивания. vtbl обязан быть первым словом.
type dndComObj struct {
	vtbl uintptr
	refs atomic.Int32
	kind dndKind

	// IDropTarget
	win      *Win32Window
	hasFiles bool // в текущем перетаскивании есть CF_HDROP
	helperOn bool // IDropTargetHelper получил DragEnter и ждёт Leave/Drop

	// IDataObject
	hdrop []byte // готовый блок CF_HDROP
	// extra — блоки, положенные в объект через SetData (формат → HGLOBAL-
	// содержимое): помощник значка хранит в них картинку и служебные флаги.
	// Защищён extraMu: SetData и GetData приходят и от оболочки, и из потока
	// модального цикла.
	extraMu sync.Mutex
	extra   map[uint16]dndBlock

	// IDropSource
	mask uint32 // MK_*-кнопка(и), с которой начали перетаскивание
}

// dndBlock — блок, положенный в объект данных через SetData: содержимое и среда
// (HGLOBAL или IStream), в которой его положили. Отдаём в той же среде, а на
// просьбу о другой — преобразуем: оба вида хранят одни и те же байты.
type dndBlock struct {
	data  []byte
	tymed uint32
}

var (
	dndObjMu sync.Mutex
	dndObjs  = map[uintptr]*dndComObj{} // адрес объекта → объект (держит его живым)
)

func (o *dndComObj) ptr() uintptr { return uintptr(unsafe.Pointer(o)) }

// newDndObj создаёт объект со счётчиком 1 (ссылка создателя) и регистрирует
// его.
func newDndObj(kind dndKind, vtbl uintptr) *dndComObj {
	o := &dndComObj{vtbl: vtbl, kind: kind}
	o.refs.Store(1)
	dndObjMu.Lock()
	dndObjs[o.ptr()] = o
	dndObjMu.Unlock()
	return o
}

func dndLookup(p uintptr) *dndComObj {
	dndObjMu.Lock()
	defer dndObjMu.Unlock()
	return dndObjs[p]
}

// dndAlive — сколько объектов ещё в реестре (для тестов утечки ссылок).
func dndAlive() int {
	dndObjMu.Lock()
	defer dndObjMu.Unlock()
	return len(dndObjs)
}

// ─── IUnknown ────────────────────────────────────────────────────────────────

func dndQueryInterface(this, riid, ppv uintptr) uintptr {
	if ppv == 0 {
		return EInvalidArg
	}
	out := ptrAt[uintptr](ppv)
	*out = 0
	o := dndLookup(this)
	if o == nil || riid == 0 {
		return ENoInterface
	}
	id := ptrAt[GUID](riid)
	var own *GUID
	switch o.kind {
	case dndKindTarget:
		own = &iidIDropTarget
	case dndKindData:
		own = &iidIDataObject
	case dndKindSource:
		own = &iidIDropSource
	}
	if !id.Equals(&IIDIUnknown) && !id.Equals(own) {
		return ENoInterface
	}
	o.refs.Add(1)
	*out = this
	return SOK
}

func dndAddRef(this uintptr) uintptr {
	if o := dndLookup(this); o != nil {
		return uintptr(o.refs.Add(1))
	}
	return 0
}

func dndRelease(this uintptr) uintptr {
	o := dndLookup(this)
	if o == nil {
		return 0
	}
	n := o.refs.Add(-1)
	if n <= 0 {
		dndObjMu.Lock()
		delete(dndObjs, this)
		dndObjMu.Unlock()
		return 0
	}
	return uintptr(n)
}

// ─── Таблицы виртуальных методов ─────────────────────────────────────────────

var (
	dndVTOnce     sync.Once
	dndVTTarget   uintptr
	dndVTData     uintptr
	dndVTSource   uintptr
	dndVTInitDone bool
)

func dndInitVTables() {
	dndVTOnce.Do(func() {
		qi := windows.NewCallback(dndQueryInterface)
		addRef := windows.NewCallback(dndAddRef)
		release := windows.NewCallback(dndRelease)

		// IDropTarget: DragEnter, DragOver, DragLeave, Drop.
		dndVTTarget = NewVTable(qi, addRef, release,
			windows.NewCallback(dndTargetDragEnter),
			windows.NewCallback(dndTargetDragOver),
			windows.NewCallback(dndTargetDragLeave),
			windows.NewCallback(dndTargetDrop),
		)
		// IDataObject: GetData, GetDataHere, QueryGetData, GetCanonicalFormatEtc,
		// SetData, EnumFormatEtc, DAdvise, DUnadvise, EnumDAdvise.
		dndVTData = NewVTable(qi, addRef, release,
			windows.NewCallback(dndDataGetData),
			windows.NewCallback(dndDataGetDataHere),
			windows.NewCallback(dndDataQueryGetData),
			windows.NewCallback(dndDataGetCanonical),
			windows.NewCallback(dndDataSetData),
			windows.NewCallback(dndDataEnumFormatEtc),
			windows.NewCallback(dndDataDAdvise),
			windows.NewCallback(dndDataDUnadvise),
			windows.NewCallback(dndDataEnumDAdvise),
		)
		// IDropSource: QueryContinueDrag, GiveFeedback.
		dndVTSource = NewVTable(qi, addRef, release,
			windows.NewCallback(dndSourceQueryContinue),
			windows.NewCallback(dndSourceGiveFeedback),
		)
		dndVTInitDone = true
	})
}

// ─── IDropTarget (приёмник: окно) ────────────────────────────────────────────

// dropTargetState — состояние регистрации окна: объект и признак успеха.
type dropTargetState struct {
	obj *dndComObj
	// helper — IDropTargetHelper оболочки (0 — не создан или недоступен):
	// рисует значок перетаскивания над окном. Создаётся при первом входе.
	helper     uintptr
	helperFail bool
}

// oleInit поднимает OLE на текущем потоке (STA). RegisterDragDrop и
// DoDragDrop требуют именно OLE, а не просто COM. Вызывать на потоке окна.
func oleInit() bool {
	hr, _, _ := procOleInitialize.Call(0)
	// S_OK (0) и S_FALSE (1, уже поднят) — успех; RPC_E_CHANGED_MODE — поток
	// занят COM в другом режиме, OLE на нём не будет.
	return int32(hr) >= 0
}

// registerDropTarget регистрирует окно как приёмник OLE. true — события
// приходят через IDropTarget; false — вызывающий включает запасной
// WM_DROPFILES.
func (w *Win32Window) registerDropTarget() bool {
	if w.hwnd == 0 || !oleInit() {
		return false
	}
	dndInitVTables()
	o := newDndObj(dndKindTarget, dndVTTarget)
	o.win = w
	hr, _, _ := procRegisterDragDrop.Call(uintptr(w.hwnd), o.ptr())
	if int32(hr) < 0 {
		dndRelease(o.ptr())
		return false
	}
	w.dropTarget = &dropTargetState{obj: o}
	w.oleReady = true
	return true
}

// revokeDropTarget снимает регистрацию (при уничтожении окна).
func (w *Win32Window) revokeDropTarget() {
	st := w.dropTarget
	if st == nil {
		return
	}
	w.dropTarget = nil
	procRevokeDragDrop.Call(uintptr(w.hwnd))
	if st.helper != 0 {
		comInvoke(st.helper, slotUnkRelease)
		st.helper = 0
	}
	dndRelease(st.obj.ptr())
}

// SetOnDragEnter / Over / Leave — события приёмника (dragTargetEvents).
// Колбэки вызываются на потоке окна; координаты — клиентские физические.
func (w *Win32Window) SetOnDragEnter(fn func(x, y int))     { w.onDragEnter = fn }
func (w *Win32Window) SetOnDragOver(fn func(x, y int) bool) { w.onDragOver = fn }
func (w *Win32Window) SetOnDragLeave(fn func())             { w.onDragLeave = fn }

// dndClientPoint переводит POINTL (экранные координаты, упакованные в слово)
// в клиентские координаты окна.
func (w *Win32Window) dndClientPoint(pt uintptr) (int, int) {
	p := point{X: int32(uint32(pt)), Y: int32(uint32(pt >> 32))}
	if w.hwnd != 0 {
		procScreenToClient.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&p)))
	}
	return int(p.X), int(p.Y)
}

// dataObjectHasHDrop спрашивает у чужого IDataObject, есть ли в нём CF_HDROP.
func dataObjectHasHDrop(dataObj uintptr) bool {
	if dataObj == 0 {
		return false
	}
	fe := hdropFormat()
	return int32(comInvoke(dataObj, slotDataQueryGetData, uintptr(unsafe.Pointer(&fe)))) >= 0
}

func hdropFormat() formatEtc {
	return formatEtc{cfFormat: cfHDropFormat, dwAspect: dvAspectContent, lindex: -1, tymed: tymedHGlobal}
}

// dataObjectFiles достаёт из чужого IDataObject список путей CF_HDROP.
func dataObjectFiles(dataObj uintptr) []string {
	if dataObj == 0 {
		return nil
	}
	fe := hdropFormat()
	var stg stgMedium
	hr := comInvoke(dataObj, slotDataGetData, uintptr(unsafe.Pointer(&fe)), uintptr(unsafe.Pointer(&stg)))
	if int32(hr) < 0 || stg.tymed != tymedHGlobal || stg.data == 0 {
		return nil
	}
	defer procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&stg)))
	// HGLOBAL с блоком DROPFILES — это и есть HDROP: DragQueryFileW читает его
	// напрямую (так же, как из WM_DROPFILES).
	return hdropPaths(stg.data)
}

// hdropPaths перечисляет пути HDROP (DragQueryFileW).
func hdropPaths(hDrop uintptr) []string {
	n, _, _ := procDragQueryFileW.Call(hDrop, 0xFFFFFFFF, 0, 0)
	count := int(n)
	if count <= 0 {
		return nil
	}
	paths := make([]string, 0, count)
	for i := 0; i < count; i++ {
		l, _, _ := procDragQueryFileW.Call(hDrop, uintptr(i), 0, 0)
		if l == 0 {
			continue
		}
		buf := make([]uint16, int(l)+1)
		procDragQueryFileW.Call(hDrop, uintptr(i),
			uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		paths = append(paths, windows.UTF16ToString(buf))
	}
	return paths
}

// dndTargetAnswer — общая часть DragEnter/DragOver: решает действие и пишет
// его в *pdwEffect. enter — вызвать ещё и колбэк входа.
func dndTargetAnswer(o *dndComObj, enter bool, keys uint32, pt, pdwEffect uintptr) {
	if pdwEffect == 0 {
		return
	}
	eff := ptrAt[uint32](pdwEffect)
	allowed := native.DragEffect(*eff) & native.DragEffectAll
	w := o.win
	if !o.hasFiles || w == nil || allowed == 0 {
		*eff = 0
		return
	}
	x, y := w.dndClientPoint(pt)
	if enter && w.onDragEnter != nil {
		w.onDragEnter(x, y)
	}
	if w.onDragOver != nil && !w.onDragOver(x, y) {
		*eff = 0
		return
	}
	*eff = uint32(native.DragEffectForKeys(allowed,
		keys&mkControl != 0, keys&mkShift != 0, keys&mkAlt != 0))
}

func dndTargetDragEnter(this, dataObj, keys, pt, pdwEffect uintptr) uintptr {
	o := dndLookup(this)
	if o == nil {
		return EFail
	}
	o.hasFiles = dataObjectHasHDrop(dataObj)
	dndTargetAnswer(o, true, uint32(keys), pt, pdwEffect)
	o.helperEnter(dataObj, pt, pdwEffect)
	return SOK
}

func dndTargetDragOver(this, keys, pt, pdwEffect uintptr) uintptr {
	o := dndLookup(this)
	if o == nil {
		return EFail
	}
	dndTargetAnswer(o, false, uint32(keys), pt, pdwEffect)
	o.helperCall(slotDropHelperOver, pt, pdwEffect)
	return SOK
}

func dndTargetDragLeave(this uintptr) uintptr {
	o := dndLookup(this)
	if o == nil {
		return EFail
	}
	if o.hasFiles && o.win != nil && o.win.onDragLeave != nil {
		o.win.onDragLeave()
	}
	o.hasFiles = false
	o.helperLeave()
	return SOK
}

func dndTargetDrop(this, dataObj, keys, pt, pdwEffect uintptr) uintptr {
	o := dndLookup(this)
	if o == nil {
		return EFail
	}
	w := o.win
	o.hasFiles = false
	var eff *uint32
	if pdwEffect != 0 {
		eff = ptrAt[uint32](pdwEffect)
	}
	o.helperDrop(dataObj, pt, pdwEffect)
	paths := dataObjectFiles(dataObj)
	if w == nil || len(paths) == 0 {
		if eff != nil {
			*eff = 0
		}
		return SOK
	}
	if eff != nil {
		allowed := native.DragEffect(*eff) & native.DragEffectAll
		*eff = uint32(native.DragEffectForKeys(allowed,
			keys&mkControl != 0, keys&mkShift != 0, keys&mkAlt != 0))
	}
	if w.onFilesDropped != nil {
		x, y := w.dndClientPoint(pt)
		w.onFilesDropped(paths, x, y)
	}
	return SOK
}

// ─── IDataObject (источник: список файлов) ───────────────────────────────────

// newGlobalCopy копирует b в HGLOBAL (GMEM_MOVEABLE). 0 — не хватило памяти.
func newGlobalCopy(b []byte) uintptr {
	h, _, _ := procGlobalAlloc.Call(gmemMoveableDnd, uintptr(len(b)))
	if h == 0 {
		return 0
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return 0
	}
	copy(unsafe.Slice(ptrAt[byte](p), len(b)), b)
	procGlobalUnlock.Call(h)
	return h
}

func dndDataGetData(this, pformatetc, pmedium uintptr) uintptr {
	o := dndLookup(this)
	if o == nil || pformatetc == 0 || pmedium == 0 {
		return EInvalidArg
	}
	fe := ptrAt[formatEtc](pformatetc)
	blob, out := o.hdrop, uint32(tymedHGlobal)
	if fe.cfFormat != cfHDropFormat {
		// Блок, положенный через SetData (значок перетаскивания).
		blk, tymed, hr := o.checkExtra(fe)
		if hr != SOK {
			return hr
		}
		blob, out = blk.data, tymed
	} else if hr := o.checkFormat(fe); hr != SOK {
		return hr
	}
	h := newGlobalCopy(blob)
	if h == 0 {
		return EFail
	}
	if out == tymedIStream {
		// Поток владеет HGLOBAL (fDeleteOnRelease) и освободит его сам.
		var stm uintptr
		if hr, _, _ := procCreateStreamOnHG.Call(h, 1, uintptr(unsafe.Pointer(&stm))); int32(hr) < 0 || stm == 0 {
			procGlobalFree.Call(h)
			return EFail
		}
		*ptrAt[stgMedium](pmedium) = stgMedium{tymed: tymedIStream, data: stm}
		return SOK
	}
	*ptrAt[stgMedium](pmedium) = stgMedium{tymed: tymedHGlobal, data: h}
	return SOK
}

func dndDataGetDataHere(this, pformatetc, pmedium uintptr) uintptr { return ENotImpl }

func dndDataQueryGetData(this, pformatetc uintptr) uintptr {
	o := dndLookup(this)
	if o == nil || pformatetc == 0 {
		return EInvalidArg
	}
	fe := ptrAt[formatEtc](pformatetc)
	if fe.cfFormat != cfHDropFormat {
		_, _, hr := o.checkExtra(fe)
		return hr
	}
	return o.checkFormat(fe)
}

// checkExtra сверяет запрошенный формат с блоками, положенными через SetData,
// и возвращает найденный блок и среду, в которой его отдать: та, в которой
// положили, если запрошена она, иначе другая из запрошенных (HGLOBAL или IStream).
func (o *dndComObj) checkExtra(fe *formatEtc) (dndBlock, uint32, uintptr) {
	o.extraMu.Lock()
	blk, ok := o.extra[fe.cfFormat]
	o.extraMu.Unlock()
	want := fe.tymed & (tymedHGlobal | tymedIStream)
	switch {
	case !ok:
		return dndBlock{}, 0, dvEFormatEtc
	case fe.dwAspect != dvAspectContent:
		return dndBlock{}, 0, dvEAspect
	case want == 0:
		return dndBlock{}, 0, dvETymed
	case want&blk.tymed != 0:
		return blk, blk.tymed, SOK
	case want&tymedHGlobal != 0:
		return blk, tymedHGlobal, SOK
	}
	return blk, tymedIStream, SOK
}

// putExtra запоминает блок формата cf (перезаписывает прежний).
func (o *dndComObj) putExtra(cf uint16, blk dndBlock) {
	o.extraMu.Lock()
	if o.extra == nil {
		o.extra = map[uint16]dndBlock{}
	}
	o.extra[cf] = blk
	o.extraMu.Unlock()
}

// extraFormats — форматы блоков SetData по возрастанию (для перечислителя).
func (o *dndComObj) extraFormats() []uint16 {
	o.extraMu.Lock()
	defer o.extraMu.Unlock()
	out := make([]uint16, 0, len(o.extra))
	for cf := range o.extra {
		out = append(out, cf)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// globalBytes копирует содержимое HGLOBAL (nil — не удалось).
func globalBytes(h uintptr) []byte {
	size, _, _ := procGlobalSize.Call(h)
	if size == 0 || size > 1<<26 {
		return nil
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return nil
	}
	defer procGlobalUnlock.Call(h)
	out := make([]byte, size)
	copy(out, unsafe.Slice(ptrAt[byte](p), int(size)))
	return out
}

// streamBytes читает IStream целиком с начала (nil — не вышло или слишком
// большой).
func streamBytes(stm uintptr) []byte {
	comInvoke(stm, slotStreamSeek, 0, 0, 0) // в начало
	var out []byte
	buf := make([]byte, 64<<10)
	for len(out) <= 1<<26 {
		var n uint32
		hr := comInvoke(stm, slotStreamRead, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n)))
		if int32(hr) < 0 {
			return nil
		}
		out = append(out, buf[:n]...)
		if n == 0 {
			break
		}
	}
	return out
}

// checkFormat сверяет запрошенный формат с тем, что объект умеет отдать:
// CF_HDROP, содержимое, через HGLOBAL. Пустой список файлов — «такого
// формата нет».
func (o *dndComObj) checkFormat(fe *formatEtc) uintptr {
	switch {
	case fe.cfFormat != cfHDropFormat || len(o.hdrop) == 0:
		return dvEFormatEtc
	case fe.dwAspect != dvAspectContent:
		return dvEAspect
	case fe.tymed&tymedHGlobal == 0:
		return dvETymed
	}
	return SOK
}

func dndDataGetCanonical(this, pformatetcIn, pformatetcOut uintptr) uintptr {
	if pformatetcOut != 0 {
		ptrAt[formatEtc](pformatetcOut).ptd = 0
	}
	return dataSSameFmt
}

// dndDataSetData принимает блоки (HGLOBAL и IStream) от помощника значка
// перетаскивания (картинка «DragImageBits» и служебные флаги) и отдаёт их
// обратно в GetData. Остальное — E_NOTIMPL, как раньше; формат CF_HDROP
// перезаписать нельзя.
func dndDataSetData(this, pformatetc, pmedium, fRelease uintptr) uintptr {
	o := dndLookup(this)
	if o == nil || o.kind != dndKindData || pformatetc == 0 || pmedium == 0 {
		return ENotImpl
	}
	fe := ptrAt[formatEtc](pformatetc)
	st := ptrAt[stgMedium](pmedium)
	if fe.cfFormat == cfHDropFormat || st.data == 0 || (st.tymed != tymedHGlobal && st.tymed != tymedIStream) {
		return ENotImpl
	}
	var blob []byte
	if st.tymed == tymedHGlobal {
		blob = globalBytes(st.data)
	} else {
		blob = streamBytes(st.data)
	}
	if blob == nil {
		return EFail
	}
	o.putExtra(fe.cfFormat, dndBlock{data: blob, tymed: st.tymed})
	if fRelease != 0 {
		procReleaseStgMedium.Call(pmedium) // хозяин среды — мы
	}
	return SOK
}

// dndDataEnumFormatEtc отдаёт стандартный перечислитель оболочки с единственным
// форматом CF_HDROP — свой IEnumFORMATETC писать незачем.
func dndDataEnumFormatEtc(this, direction, ppenum uintptr) uintptr {
	if ppenum == 0 {
		return EInvalidArg
	}
	*ptrAt[uintptr](ppenum) = 0
	if direction != dataDirGet {
		return ENotImpl
	}
	// CF_HDROP и блоки, положенные через SetData.
	o := dndLookup(this)
	fes := []formatEtc{hdropFormat()}
	if o != nil {
		for _, cf := range o.extraFormats() {
			fes = append(fes, formatEtc{cfFormat: cf, dwAspect: dvAspectContent, lindex: -1, tymed: tymedHGlobal | tymedIStream})
		}
	}
	hr, _, _ := procSHCreateStdEnumFmtEtc.Call(uintptr(len(fes)), uintptr(unsafe.Pointer(&fes[0])), ppenum)
	return hr
}

func dndDataDAdvise(this, pformatetc, advf, pAdvSink, pdwConnection uintptr) uintptr {
	return oleEAdviseNoSup
}
func dndDataDUnadvise(this, dwConnection uintptr) uintptr { return oleEAdviseNoSup }
func dndDataEnumDAdvise(this, ppenum uintptr) uintptr     { return oleEAdviseNoSup }

// ─── IDropSource ─────────────────────────────────────────────────────────────

func dndSourceQueryContinue(this, escape, keyState uintptr) uintptr {
	o := dndLookup(this)
	if o == nil {
		return uintptr(dragdropSCancel)
	}
	return uintptr(dragContinueResult(escape != 0, uint32(keyState), o.mask))
}

func dndSourceGiveFeedback(this, effect uintptr) uintptr { return uintptr(dragdropSUseDef) }

// ─── BeginDragFiles ──────────────────────────────────────────────────────────

// physicalButtons возвращает маски зажатых кнопок мыши: в битах MK_* (для
// IDropSource) и в идентификаторах движка (0 левая, 1 правая, 2 средняя) —
// последние нужны, чтобы после DoDragDrop отпустить кнопки в движке: модальный
// цикл съедает отпускание.
func physicalButtons() (mk uint32, held []int) {
	swapped, _, _ := user32.NewProc("GetSystemMetrics").Call(smSwapButton)
	l, r := vkLButton, vkRButton
	if swapped != 0 {
		l, r = r, l
	}
	// Логическая левая кнопка — та, что клавиша VK_LBUTTON после перестановки.
	down := func(vk int) bool {
		s, _, _ := procGetAsyncKeyst.Call(uintptr(vk))
		return int16(s) < 0
	}
	if down(l) {
		mk |= mkLButton
		held = append(held, 0)
	}
	if down(r) {
		mk |= mkRButton
		held = append(held, 1)
	}
	if down(vkMButton) {
		mk |= mkMButton
		held = append(held, 2)
	}
	return mk, held
}

// BeginDragFiles начинает перетаскивание файлов (fileDragSource): DoDragDrop
// на потоке окна. Кнопка мыши должна быть зажата прямо сейчас.
func (w *Win32Window) BeginDragFiles(paths []string, effect native.DragEffect, done func(native.DragEffect)) bool {
	return w.beginDrag(paths, effect, nil, image.Point{}, done)
}

// BeginDragFilesImage — BeginDragFiles со значком (fileDragImageSource): img
// уходит оболочке через IDragSourceHelper. Без помощника (его нет в системе)
// перетаскивание идёт без значка.
func (w *Win32Window) BeginDragFilesImage(paths []string, effect native.DragEffect, img *image.RGBA, hotspot image.Point, done func(native.DragEffect)) bool {
	return w.beginDrag(paths, effect, img, hotspot, done)
}

// DragImageSupported — значок рисует оболочка, когда поднят OLE.
func (w *Win32Window) DragImageSupported() bool { return w.oleReady }

func (w *Win32Window) beginDrag(paths []string, effect native.DragEffect, img *image.RGBA, hot image.Point, done func(native.DragEffect)) bool {
	if w.hwnd == 0 || !w.oleReady || len(paths) == 0 || effect&native.DragEffectAll == 0 {
		return false
	}
	mask, held := physicalButtons()
	if mask == 0 {
		return false
	}
	block := buildHDrop(paths)
	w.InvokeOnUIThread(func() {
		res := runDragDropImage(block, effect, mask, img, hot)
		// Модальный цикл ОС забрал отпускание кнопки: сообщаем движку сами.
		if w.onMouseButton != nil {
			var pt point
			procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			procScreenToClient.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&pt)))
			for _, id := range held {
				w.onMouseButton(int(pt.X), int(pt.Y), id, false)
			}
		}
		if done != nil {
			done(res)
		}
	})
	return true
}

// runDragDrop выполняет DoDragDrop с объектами списка файлов; возвращает
// выбранное приёмником действие (0 — отмена или ошибка). Блокирует до конца
// перетаскивания; только на потоке с OLE.
func runDragDrop(block []byte, effect native.DragEffect, mask uint32) native.DragEffect {
	return runDragDropImage(block, effect, mask, nil, image.Point{})
}

// runDragDropImage — runDragDrop со значком (img == nil — без него).
func runDragDropImage(block []byte, effect native.DragEffect, mask uint32, img *image.RGBA, hot image.Point) native.DragEffect {
	dndInitVTables()
	data := newDndObj(dndKindData, dndVTData)
	data.hdrop = block
	if img != nil {
		initDragImage(data.ptr(), img, hot)
	}
	src := newDndObj(dndKindSource, dndVTSource)
	src.mask = mask
	defer dndRelease(data.ptr())
	defer dndRelease(src.ptr())

	var final uint32
	hr, _, _ := procDoDragDrop.Call(data.ptr(), src.ptr(), uintptr(effect&native.DragEffectAll),
		uintptr(unsafe.Pointer(&final)))
	if uint32(hr) != dragdropSDrop {
		return 0
	}
	return native.DragEffect(final) & native.DragEffectAll
}
