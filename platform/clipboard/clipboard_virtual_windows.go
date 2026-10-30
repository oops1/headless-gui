//go:build windows

package clipboard

// clipboard_virtual_windows.go — виртуальные файлы в буфере обмена Windows (v3.42).
//
// Буфер получает не копию данных, а объект IDataObject, который отдаёт их по
// запросу (OleSetClipboard). Проводник при вставке:
//
//  1. перечисляет форматы объекта и находит «FileGroupDescriptorW» (FILEGROUPDESCRIPTORW:
//     имена, размеры, время, атрибуты — widget/base/clipboard_virtual.go);
//  2. для каждого файла зовёт GetData("FileContents", lindex = номер файла,
//     TYMED_ISTREAM) и читает поток.
//
// Поток — наш IStream, читающий из io.ReadCloser, который отдаёт функция open;
// она зовётся при первом чтении потока, поэтому «скопировать» ничего не стоит,
// а скачивание начинается ровно при вставке. OleFlushClipboard не вызывается —
// иначе система запросила бы содержимое сразу, при копировании.
//
// OLE требует потока в квартире STA с циклом сообщений: к объекту, лежащему в
// буфере, чужие процессы обращаются через неё. Движок и приложение такого
// потока не гарантируют, поэтому у буфера свой — горутина, закреплённая за
// потоком ОС, с OleInitialize и GetMessage. Команды (положить, снять) ей
// передаются очередью и сообщением потока.
//
// CGO в проекте нет, поэтому COM-объекты собраны руками, как в platform/win32
// (com_windows.go, dnd_windows.go) и platform/a11y/a11y_uia_windows.go: объект — структура с указателем на таблицу виртуальных
// методов первым словом, таблица — массив адресов из syscall.NewCallback. Адрес
// объекта отдаётся ОС числом, а по числу объект находится в реестре — обратных
// преобразований uintptr в указатель на Go-память нет.
//
// Ограничения. Квартира STA — один поток: пока поток отдаёт кусок содержимого
// (Read ждёт open), остальные обращения к буферу из этого процесса
// (OleSetClipboard, замена буфера из другого потока) ждут конца куска.
// Блок Read заполняется целиком, если поток не кончился, — как делает
// CreateStreamOnHGlobal, чтобы короткое чтение не приняли за конец файла.

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/oops1/headless-gui/v4/widget/base"
)

// ─── Импорты и константы ─────────────────────────────────────────────────────

var (
	ole32Clip = syscall.NewLazyDLL("ole32.dll")

	procOleInitializeClip    = ole32Clip.NewProc("OleInitialize")
	procOleSetClipboard      = ole32Clip.NewProc("OleSetClipboard")
	procOleIsCurrentClip     = ole32Clip.NewProc("OleIsCurrentClipboard")
	procOleGetClipboard      = ole32Clip.NewProc("OleGetClipboard")
	procReleaseStgMediumClip = ole32Clip.NewProc("ReleaseStgMedium")
	procStdEnumFmtEtc        = shell32Clip.NewProc("SHCreateStdEnumFmtEtc")

	procGetMessageClip      = user32Clip.NewProc("GetMessageW")
	procPeekMessageClip     = user32Clip.NewProc("PeekMessageW")
	procTranslateMessage    = user32Clip.NewProc("TranslateMessage")
	procDispatchMessageClip = user32Clip.NewProc("DispatchMessageW")
	procPostThreadMessage   = user32Clip.NewProc("PostThreadMessageW")
	procGetCurrentThreadID  = kernel32.NewProc("GetCurrentThreadId")
)

const (
	vfTymedHGlobal = 1 // TYMED_HGLOBAL
	vfTymedIStream = 4 // TYMED_ISTREAM
	vfAspect       = 1 // DVASPECT_CONTENT

	vfWMApp      = 0x8000 // WM_APP: команда потоку буфера
	vfPMNoRemove = 0

	// vfMaxHGlobal — наибольший файл, который отдаётся блоком HGLOBAL (потоком —
	// любой): блок читается целиком в память.
	vfMaxHGlobal = 16 << 20

	vfSFalse        = uintptr(1)
	vfEInvalidArg   = uintptr(0x80070057)
	vfENoInterface  = uintptr(0x80004002)
	vfEFail         = uintptr(0x80004005)
	vfENotImpl      = uintptr(0x80004001)
	vfDVFormatEtc   = uintptr(0x80040064) // DV_E_FORMATETC
	vfDVLindex      = uintptr(0x80040068) // DV_E_LINDEX
	vfDVTymed       = uintptr(0x80040069) // DV_E_TYMED
	vfDVAspect      = uintptr(0x8004006B) // DV_E_DVASPECT
	vfAdviseNoSup   = uintptr(0x80040003) // OLE_E_ADVISENOTSUPPORTED
	vfDataSameFmt   = uintptr(0x00040130) // DATA_S_SAMEFORMATETC
	vfSTGReadFault  = uintptr(0x8003001E) // STG_E_READFAULT
	vfSTGAccess     = uintptr(0x80030005) // STG_E_ACCESSDENIED
	vfSTGInvalidFn  = uintptr(0x80030001) // STG_E_INVALIDFUNCTION
	vfSTGInvalidPtr = uintptr(0x80030009) // STG_E_INVALIDPOINTER
	vfSTGSeekError  = uintptr(0x80030019) // STG_E_SEEKERROR
	vfClipCantOpen  = 0x800401D0          // CLIPBRD_E_CANT_OPEN
)

type vfGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	vfIIDUnknown    = vfGUID{0x00000000, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	vfIIDDataObject = vfGUID{0x0000010E, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	vfIIDStream     = vfGUID{0x0000000C, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	vfIIDSeqStream  = vfGUID{0x0C733A30, 0x2A1C, 0x11CE, [8]byte{0xAD, 0xE5, 0x00, 0xAA, 0x00, 0x44, 0x77, 0x3D}}
)

// vfFormatEtc — FORMATETC (32 байта на x64).
type vfFormatEtc struct {
	cfFormat uint16
	ptd      uintptr
	dwAspect uint32
	lindex   int32
	tymed    uint32
}

// vfStgMedium — STGMEDIUM (24 байта на x64).
type vfStgMedium struct {
	tymed          uint32
	data           uintptr
	pUnkForRelease uintptr
}

// vfStatStg — STATSTG (80 байт на x64).
type vfStatStg struct {
	pwcsName          uintptr
	typ               uint32
	cbSize            uint64
	mtime, ctime, atm uint64
	grfMode           uint32
	grfLocks          uint32
	clsid             vfGUID
	grfStateBits      uint32
	reserved          uint32
}

// vfMsg — MSG (48 байт на x64).
type vfMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
	priv    uint32
}

// vfPtrAt превращает число-адрес чужой памяти (аргумент COM-метода) в указатель.
// Обход через unsafe.Pointer(&p) — единственный способ сделать это так, чтобы
// go vet не видел здесь ошибки: адрес пришёл от ОС, а не из Go.
func vfPtrAt[T any](p uintptr) *T {
	return (*T)(*(*unsafe.Pointer)(unsafe.Pointer(&p)))
}

// vfInvoke вызывает метод COM-объекта по номеру слота таблицы. Аргументы-адреса
// обязаны остаться в куче на время вызова (метод может исполняться нашим же
// колбэком на этой горутине), отсюда uintptrescapes.
//
//go:uintptrescapes
func vfInvoke(obj uintptr, slot int, args ...uintptr) uintptr {
	vtbl := *vfPtrAt[uintptr](obj)
	fn := *vfPtrAt[uintptr](vtbl + uintptr(slot)*unsafe.Sizeof(uintptr(0)))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, obj)
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

// Слоты таблицы: IUnknown (0..2), затем методы интерфейса.
const (
	vfSlotRelease    = 2
	vfSlotStreamRead = 3
	vfSlotStreamWrit = 4
	vfSlotStreamSeek = 5
	vfSlotGetData    = 3
	vfSlotQueryGet   = 5
	vfSlotEnumFmt    = 8

	vfIStreamSlots     = 14 // IUnknown + ISequentialStream (2) + IStream (9)
	vfIDataObjectSlots = 12 // IUnknown + 9 методов
)

// ─── Форматы ─────────────────────────────────────────────────────────────────

var vfFmt struct {
	once                 sync.Once
	desc, contents, drop uint16
}

// vfFormats возвращает идентификаторы форматов «FileGroupDescriptorW»,
// «FileContents» и «Preferred DropEffect».
func vfFormats() (desc, contents, drop uint16) {
	vfFmt.once.Do(func() {
		reg := func(name string) uint16 {
			p, _ := syscall.UTF16PtrFromString(name)
			id, _, _ := procRegisterFormat.Call(uintptr(unsafe.Pointer(p)))
			return uint16(id)
		}
		vfFmt.desc = reg("FileGroupDescriptorW")
		vfFmt.contents = reg("FileContents")
		vfFmt.drop = reg("Preferred DropEffect")
	})
	return vfFmt.desc, vfFmt.contents, vfFmt.drop
}

// ─── Состояние обещания ──────────────────────────────────────────────────────

// vfData — то, что обещано буферу: список файлов и источник содержимого.
type vfData struct {
	files []base.VirtualFile
	open  func(int) (io.ReadCloser, error)
	desc  []byte // готовый FILEGROUPDESCRIPTORW
	obj   uintptr
	freed atomic.Bool // OLE выпустил объект: буфер заменён
	// release — колбэк приложения «обещание снято»; зовётся один раз.
	release func()
}

var vfCur atomic.Pointer[vfData] // обещание, лежащее в буфере сейчас

// ─── COM-объекты ─────────────────────────────────────────────────────────────

const (
	vfKindData = iota
	vfKindStream
)

// vfObj — COM-объект: IDataObject или IStream (различает kind). vtbl обязан
// быть первым словом.
type vfObj struct {
	vtbl uintptr
	refs atomic.Int32
	kind int
	data *vfData
	stm  *vfStream
}

func (o *vfObj) ptr() uintptr { return uintptr(unsafe.Pointer(o)) }

var (
	vfObjMu sync.Mutex
	vfObjs  = map[uintptr]*vfObj{} // адрес объекта → объект (держит его живым)
)

// newVFObj создаёт объект со счётчиком 1 (ссылка создателя).
func newVFObj(kind int) *vfObj {
	vfInitVTables()
	o := &vfObj{kind: kind}
	if kind == vfKindData {
		o.vtbl = vfVT.dataPtr
	} else {
		o.vtbl = vfVT.streamPtr
	}
	o.refs.Store(1)
	vfObjMu.Lock()
	vfObjs[o.ptr()] = o
	vfObjMu.Unlock()
	return o
}

func vfLookup(p uintptr) *vfObj {
	vfObjMu.Lock()
	defer vfObjMu.Unlock()
	return vfObjs[p]
}

// vfAlive — сколько объектов ещё в реестре (для тестов утечки ссылок).
func vfAlive() int {
	vfObjMu.Lock()
	defer vfObjMu.Unlock()
	return len(vfObjs)
}

func vfQueryInterface(this, riid, ppv uintptr) uintptr {
	if ppv == 0 {
		return vfEInvalidArg
	}
	*vfPtrAt[uintptr](ppv) = 0
	o := vfLookup(this)
	if o == nil || riid == 0 {
		return vfENoInterface
	}
	id := *vfPtrAt[vfGUID](riid)
	ok := id == vfIIDUnknown
	switch o.kind {
	case vfKindData:
		ok = ok || id == vfIIDDataObject
	case vfKindStream:
		ok = ok || id == vfIIDStream || id == vfIIDSeqStream
	}
	if !ok {
		return vfENoInterface
	}
	o.refs.Add(1)
	*vfPtrAt[uintptr](ppv) = this
	return 0
}

func vfAddRef(this uintptr) uintptr {
	if o := vfLookup(this); o != nil {
		return uintptr(o.refs.Add(1))
	}
	return 0
}

func vfRelease(this uintptr) uintptr {
	o := vfLookup(this)
	if o == nil {
		return 0
	}
	n := o.refs.Add(-1)
	if n > 0 {
		return uintptr(n)
	}
	vfObjMu.Lock()
	delete(vfObjs, this)
	vfObjMu.Unlock()
	switch o.kind {
	case vfKindData:
		o.data.freed.Store(true)
		vfCur.CompareAndSwap(o.data, nil)
		if fn := o.data.release; fn != nil {
			// Не здесь: мы внутри колбэка COM на потоке буфера, а приложение
			// вправе звать буфер, что на этом потоке не дождётся себя.
			go fn()
		}
	case vfKindStream:
		o.stm.close()
	}
	return 0
}

// vfVT — таблицы виртуальных методов. Срезы хранятся, чтобы тест видел их
// длину; адреса стабильны, пока срезы живы.
var vfVT struct {
	once               sync.Once
	data, stream       []uintptr
	dataPtr, streamPtr uintptr
}

func vfInitVTables() {
	vfVT.once.Do(func() {
		qi := syscall.NewCallback(vfQueryInterface)
		addRef := syscall.NewCallback(vfAddRef)
		release := syscall.NewCallback(vfRelease)
		// IDataObject: GetData, GetDataHere, QueryGetData, GetCanonicalFormatEtc,
		// SetData, EnumFormatEtc, DAdvise, DUnadvise, EnumDAdvise.
		vfVT.data = []uintptr{qi, addRef, release,
			syscall.NewCallback(vfDataGetData),
			syscall.NewCallback(vfDataGetDataHere),
			syscall.NewCallback(vfDataQueryGetData),
			syscall.NewCallback(vfDataGetCanonical),
			syscall.NewCallback(vfDataSetData),
			syscall.NewCallback(vfDataEnumFormatEtc),
			syscall.NewCallback(vfDataDAdvise),
			syscall.NewCallback(vfDataDUnadvise),
			syscall.NewCallback(vfDataEnumDAdvise),
		}
		// IStream: Read, Write (ISequentialStream), Seek, SetSize, CopyTo, Commit,
		// Revert, LockRegion, UnlockRegion, Stat, Clone.
		vfVT.stream = []uintptr{qi, addRef, release,
			syscall.NewCallback(vfStreamRead),
			syscall.NewCallback(vfStreamWrite),
			syscall.NewCallback(vfStreamSeek),
			syscall.NewCallback(vfStreamSetSize),
			syscall.NewCallback(vfStreamCopyTo),
			syscall.NewCallback(vfStreamCommit),
			syscall.NewCallback(vfStreamRevert),
			syscall.NewCallback(vfStreamLock),
			syscall.NewCallback(vfStreamLock),
			syscall.NewCallback(vfStreamStat),
			syscall.NewCallback(vfStreamClone),
		}
		vfVT.dataPtr = uintptr(unsafe.Pointer(&vfVT.data[0]))
		vfVT.streamPtr = uintptr(unsafe.Pointer(&vfVT.stream[0]))
	})
}

// ─── IDataObject ─────────────────────────────────────────────────────────────

// vfNewGlobal копирует b в HGLOBAL (GMEM_MOVEABLE). 0 — не хватило памяти.
func vfNewGlobal(b []byte) uintptr {
	n := uintptr(len(b))
	if n == 0 {
		n = 1
	}
	h, _, _ := procGlobalAlloc.Call(gmemMoveable, n)
	if h == 0 {
		return 0
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return 0
	}
	copy(unsafe.Slice(vfPtrAt[byte](p), len(b)), b)
	procGlobalUnlock.Call(h)
	return h
}

// check сверяет запрошенный формат с тем, что объект умеет отдать, и возвращает
// индекс файла (-1 для остальных форматов) и среду, в которой отдавать.
func (d *vfData) check(fe *vfFormatEtc) (idx int, tymed uint32, hr uintptr) {
	desc, contents, drop := vfFormats()
	if fe.dwAspect != vfAspect {
		return -1, 0, vfDVAspect
	}
	switch fe.cfFormat {
	case desc, drop:
		if fe.tymed&vfTymedHGlobal == 0 {
			return -1, 0, vfDVTymed
		}
		return -1, vfTymedHGlobal, 0
	case contents:
		idx = int(fe.lindex)
		if idx < -1 || idx >= len(d.files) {
			return -1, 0, vfDVLindex
		}
		if idx >= 0 && d.files[idx].IsDir {
			return -1, 0, vfDVLindex
		}
		switch {
		case fe.tymed&vfTymedIStream != 0:
			return idx, vfTymedIStream, 0
		case fe.tymed&vfTymedHGlobal != 0 && (idx < 0 || d.files[idx].Size <= vfMaxHGlobal):
			return idx, vfTymedHGlobal, 0
		}
		return -1, 0, vfDVTymed
	}
	return -1, 0, vfDVFormatEtc
}

func vfDataGetData(this, pfe, pmed uintptr) uintptr {
	o := vfLookup(this)
	if o == nil || o.kind != vfKindData || pfe == 0 || pmed == 0 {
		return vfEInvalidArg
	}
	d := o.data
	fe := vfPtrAt[vfFormatEtc](pfe)
	med := vfPtrAt[vfStgMedium](pmed)
	*med = vfStgMedium{}
	idx, tymed, hr := d.check(fe)
	if hr != 0 {
		return hr
	}
	desc, contents, _ := vfFormats()
	switch {
	case fe.cfFormat == desc:
		h := vfNewGlobal(d.desc)
		if h == 0 {
			return vfEFail
		}
		*med = vfStgMedium{tymed: vfTymedHGlobal, data: h}
	case fe.cfFormat == contents:
		if idx < 0 {
			return vfDVLindex // содержимое «всех сразу» не бывает
		}
		if tymed == vfTymedIStream {
			so := newVFObj(vfKindStream)
			so.stm = &vfStream{d: d, index: idx, size: d.files[idx].Size, mod: d.files[idx].ModTime}
			*med = vfStgMedium{tymed: vfTymedIStream, data: so.ptr()}
			return 0
		}
		rc, err := d.open(idx)
		if err != nil || rc == nil {
			return vfEFail
		}
		buf, err := io.ReadAll(io.LimitReader(rc, vfMaxHGlobal+1))
		rc.Close()
		if err != nil || len(buf) > vfMaxHGlobal {
			return vfEFail
		}
		h := vfNewGlobal(buf)
		if h == 0 {
			return vfEFail
		}
		*med = vfStgMedium{tymed: vfTymedHGlobal, data: h}
	default: // Preferred DropEffect: копирование
		h := vfNewGlobal([]byte{base.DropEffectCopy, 0, 0, 0})
		if h == 0 {
			return vfEFail
		}
		*med = vfStgMedium{tymed: vfTymedHGlobal, data: h}
	}
	return 0
}

func vfDataGetDataHere(this, pfe, pmed uintptr) uintptr { return vfENotImpl }

func vfDataQueryGetData(this, pfe uintptr) uintptr {
	o := vfLookup(this)
	if o == nil || o.kind != vfKindData || pfe == 0 {
		return vfEInvalidArg
	}
	_, _, hr := o.data.check(vfPtrAt[vfFormatEtc](pfe))
	return hr
}

func vfDataGetCanonical(this, pin, pout uintptr) uintptr {
	if pout != 0 {
		vfPtrAt[vfFormatEtc](pout).ptd = 0
	}
	return vfDataSameFmt
}

// vfDataSetData: Проводник после вставки сообщает источнику итог
// («Performed DropEffect», «Paste Succeeded»), но принимать это не обязательно.
func vfDataSetData(this, pfe, pmed, fRelease uintptr) uintptr { return vfENotImpl }

// vfDataEnumFormatEtc отдаёт стандартный перечислитель оболочки: свой
// IEnumFORMATETC писать незачем.
func vfDataEnumFormatEtc(this, direction, ppenum uintptr) uintptr {
	if ppenum == 0 {
		return vfEInvalidArg
	}
	*vfPtrAt[uintptr](ppenum) = 0
	if direction != 1 { // DATADIR_GET
		return vfENotImpl
	}
	desc, contents, drop := vfFormats()
	fes := []vfFormatEtc{
		{cfFormat: desc, dwAspect: vfAspect, lindex: -1, tymed: vfTymedHGlobal},
		{cfFormat: contents, dwAspect: vfAspect, lindex: 0, tymed: vfTymedIStream},
		{cfFormat: drop, dwAspect: vfAspect, lindex: -1, tymed: vfTymedHGlobal},
	}
	hr, _, _ := procStdEnumFmtEtc.Call(uintptr(len(fes)), uintptr(unsafe.Pointer(&fes[0])), ppenum)
	return hr
}

func vfDataDAdvise(this, pfe, advf, sink, conn uintptr) uintptr { return vfAdviseNoSup }
func vfDataDUnadvise(this, conn uintptr) uintptr                { return vfAdviseNoSup }
func vfDataEnumDAdvise(this, ppenum uintptr) uintptr            { return vfAdviseNoSup }

// ─── IStream ─────────────────────────────────────────────────────────────────

// vfStream — поток содержимого одного файла. Читатель открывается при первом
// чтении; Seek назад переоткрывает его (open зовётся снова), вперёд — читает и
// отбрасывает.
type vfStream struct {
	d     *vfData
	index int
	size  int64
	mod   time.Time

	mu   sync.Mutex
	rc   io.ReadCloser
	rpos int64 // позиция читателя rc
	pos  int64 // позиция потока (то, что видит вызывающий)
	err  error // ошибка чтения, сохранённая до следующего вызова
}

func (s *vfStream) close() {
	s.mu.Lock()
	if s.rc != nil {
		s.rc.Close()
		s.rc = nil
	}
	s.mu.Unlock()
}

// seekReader выставляет читателя в s.pos: открывает, переоткрывает или
// пропускает байты. s.mu держит вызывающий.
func (s *vfStream) seekReader() error {
	if s.rc == nil || s.pos < s.rpos {
		if s.rc != nil {
			s.rc.Close()
			s.rc = nil
		}
		rc, err := s.d.open(s.index)
		if err != nil {
			return err
		}
		if rc == nil {
			return errors.New("open вернул nil")
		}
		s.rc, s.rpos = rc, 0
	}
	if s.pos > s.rpos {
		n, err := io.CopyN(io.Discard, s.rc, s.pos-s.rpos)
		s.rpos += n
		if err != nil {
			return err
		}
	}
	return nil
}

// read читает в buf, пока не заполнит его или не кончится поток.
func (s *vfStream) read(buf []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		err := s.err
		s.err = nil
		return 0, err
	}
	if len(buf) == 0 {
		return 0, nil
	}
	if err := s.seekReader(); err != nil {
		if errors.Is(err, io.EOF) {
			return 0, io.EOF
		}
		return 0, err
	}
	n, err := io.ReadFull(s.rc, buf)
	s.rpos += int64(n)
	s.pos += int64(n)
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	if err == io.EOF && n > 0 {
		err = nil // конец потока, но данные есть: о конце скажет следующий вызов
	}
	if err != nil && err != io.EOF && n > 0 {
		s.err, err = err, nil // отдаём прочитанное, ошибку — следующим вызовом
	}
	return n, err
}

func vfStreamOf(this uintptr) *vfStream {
	if o := vfLookup(this); o != nil && o.kind == vfKindStream {
		return o.stm
	}
	return nil
}

func vfStreamRead(this, pv, cb, pcbRead uintptr) uintptr {
	s := vfStreamOf(this)
	if s == nil {
		return vfEInvalidArg
	}
	n := int(uint32(cb))
	if pcbRead != 0 {
		*vfPtrAt[uint32](pcbRead) = 0
	}
	if n > 0 && pv == 0 {
		return vfSTGInvalidPtr
	}
	var buf []byte
	if n > 0 {
		buf = unsafe.Slice(vfPtrAt[byte](pv), n)
	}
	got, err := s.read(buf)
	if pcbRead != 0 {
		*vfPtrAt[uint32](pcbRead) = uint32(got)
	}
	if err != nil && err != io.EOF {
		return vfSTGReadFault
	}
	return 0
}

func vfStreamWrite(this, pv, cb, pcbWritten uintptr) uintptr { return vfSTGAccess }

func vfStreamSeek(this, move, origin, pnew uintptr) uintptr {
	s := vfStreamOf(this)
	if s == nil {
		return vfEInvalidArg
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var origPos int64
	switch uint32(origin) {
	case 0: // STREAM_SEEK_SET
	case 1: // STREAM_SEEK_CUR
		origPos = s.pos
	case 2: // STREAM_SEEK_END
		origPos = s.size
	default:
		return vfEInvalidArg
	}
	target := origPos + int64(move)
	if target < 0 {
		return vfSTGSeekError
	}
	s.pos = target
	if pnew != 0 {
		*vfPtrAt[uint64](pnew) = uint64(target)
	}
	return 0
}

func vfStreamSetSize(this, size uintptr) uintptr { return vfSTGAccess }
func vfStreamCommit(this, flags uintptr) uintptr { return 0 }
func vfStreamRevert(this uintptr) uintptr        { return 0 }
func vfStreamLock(this, off, cb, typ uintptr) uintptr {
	return vfSTGInvalidFn
}
func vfStreamClone(this, ppstm uintptr) uintptr {
	if ppstm != 0 {
		*vfPtrAt[uintptr](ppstm) = 0
	}
	return vfENotImpl
}

// vfStreamCopyTo копирует cb байт в другой поток (его Write — слот 4).
func vfStreamCopyTo(this, dest, cb, pcbRead, pcbWritten uintptr) uintptr {
	s := vfStreamOf(this)
	if s == nil || dest == 0 {
		return vfEInvalidArg
	}
	var read, written uint64
	left := uint64(cb)
	buf := make([]byte, 64<<10)
	hr := uintptr(0)
	for left > 0 {
		chunk := buf
		if uint64(len(chunk)) > left {
			chunk = chunk[:left]
		}
		n, err := s.read(chunk)
		read += uint64(n)
		if n > 0 {
			var w uint32
			if h := vfInvoke(dest, vfSlotStreamWrit, uintptr(unsafe.Pointer(&chunk[0])), uintptr(n), uintptr(unsafe.Pointer(&w))); int32(h) < 0 {
				hr = h
				break
			}
			written += uint64(w)
			left -= uint64(n)
		}
		if err != nil {
			if err != io.EOF {
				hr = vfSTGReadFault
			}
			break
		}
	}
	if pcbRead != 0 {
		*vfPtrAt[uint64](pcbRead) = read
	}
	if pcbWritten != 0 {
		*vfPtrAt[uint64](pcbWritten) = written
	}
	return hr
}

func vfStreamStat(this, pstat, flag uintptr) uintptr {
	s := vfStreamOf(this)
	if s == nil || pstat == 0 {
		return vfEInvalidArg
	}
	*vfPtrAt[vfStatStg](pstat) = vfStatStg{
		typ:    2, // STGTY_STREAM
		cbSize: uint64(s.size),
		mtime:  base.FileTime(s.mod),
	}
	return 0
}

// ─── Поток буфера: STA с циклом сообщений ────────────────────────────────────

// vfThread — поток ОС, обслуживающий объект в буфере: OLE-квартира STA с
// GetMessage. Запускается при первом обращении и живёт до конца процесса.
type vfThread struct {
	once    sync.Once
	started atomic.Bool
	tid     uint32
	err     error

	mu sync.Mutex
	q  []func()
}

var vfT vfThread

func (t *vfThread) start() error {
	t.once.Do(func() {
		ready := make(chan error, 1)
		go t.loop(ready)
		t.err = <-ready
		t.started.Store(t.err == nil)
	})
	return t.err
}

func (t *vfThread) loop(ready chan<- error) {
	runtime.LockOSThread() // поток не отпускаем: квартира STA живёт вместе с ним
	if hr, _, _ := procOleInitializeClip.Call(0); int32(hr) < 0 {
		ready <- fmt.Errorf("widget: OleInitialize: 0x%08X", uint32(hr))
		return
	}
	var msg vfMsg
	// Холостой PeekMessage заводит очередь сообщений потока: до этого
	// PostThreadMessage ему не доставить.
	procPeekMessageClip.Call(uintptr(unsafe.Pointer(&msg)), 0, vfWMApp, vfWMApp, vfPMNoRemove)
	id, _, _ := procGetCurrentThreadID.Call()
	t.tid = uint32(id)
	ready <- nil
	for {
		r, _, _ := procGetMessageClip.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		if msg.hwnd == 0 && msg.message == vfWMApp {
			t.drain()
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageClip.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (t *vfThread) drain() {
	for {
		t.mu.Lock()
		if len(t.q) == 0 {
			t.mu.Unlock()
			return
		}
		fn := t.q[0]
		t.q = t.q[1:]
		t.mu.Unlock()
		fn()
	}
}

// run выполняет fn на потоке буфера и ждёт конца. Из самого потока (колбэка
// COM) звать нельзя — вечное ожидание.
func (t *vfThread) run(fn func()) error {
	if err := t.start(); err != nil {
		return err
	}
	done := make(chan any, 1)
	t.mu.Lock()
	t.q = append(t.q, func() {
		defer func() { done <- recover() }()
		fn()
	})
	t.mu.Unlock()
	if r, _, e := procPostThreadMessage.Call(uintptr(t.tid), vfWMApp, 0, 0); r == 0 {
		return fmt.Errorf("widget: PostThreadMessage: %v", e)
	}
	if p := <-done; p != nil {
		return fmt.Errorf("widget: сбой потока буфера: %v", p)
	}
	return nil
}

// ─── winClipboard ────────────────────────────────────────────────────────────

// SetVirtualFiles кладёт обещанные файлы в буфер объектом IDataObject.
//
// Реализует widget.ClipboardVirtualFilesProvider.
func (c *winClipboard) SetVirtualFiles(files []base.VirtualFile, open func(int) (io.ReadCloser, error), opts base.VirtualFilesOptions) error {
	norm, err := base.NormalizeVirtualFiles(files)
	if err != nil {
		return err
	}
	if open == nil {
		return base.ErrClipboardVirtualNoSource // Проводнику нужно содержимое: Materialize здесь не годится
	}
	desc, err := base.BuildFileGroupDescriptor(norm)
	if err != nil {
		return err
	}
	vfFormats()
	d := &vfData{files: norm, open: open, desc: desc, release: opts.Release}

	var setErr error
	if err := vfT.run(func() { setErr = vfSetClipboard(d) }); err != nil {
		return err
	}
	return setErr
}

// vfSetClipboard исполняется на потоке буфера.
func vfSetClipboard(d *vfData) error {
	o := newVFObj(vfKindData)
	o.data = d
	d.obj = o.ptr()
	hr := vfOleSetClipboard(o.ptr())
	if int32(hr) < 0 {
		d.release = nil // обещание не состоялось — приложению сообщать нечего
		vfRelease(o.ptr())
		return fmt.Errorf("widget: OleSetClipboard: 0x%08X", uint32(hr))
	}
	vfCur.Store(d)
	vfRelease(o.ptr()) // ссылка создателя; своя у OLE
	return nil
}

// vfOleSetClipboard зовёт OleSetClipboard, повторяя попытку: сразу после
// изменения буфер на миг занимают слушатели (история буфера, удалённый рабочий
// стол), и вызов отвечает CLIPBRD_E_CANT_OPEN, хотя через десяток миллисекунд
// всё открылось бы. Исполняется на потоке буфера.
func vfOleSetClipboard(dataObj uintptr) uintptr {
	var hr uintptr
	for i := 0; i < 300; i++ {
		hr, _, _ = procOleSetClipboard.Call(dataObj)
		if uint32(hr) != vfClipCantOpen {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return hr
}

// GetVirtualFiles возвращает обещанные нами файлы, пока они в буфере.
func (c *winClipboard) GetVirtualFiles() ([]base.VirtualFile, bool) {
	d := vfCur.Load()
	if d == nil || d.freed.Load() {
		return nil, false
	}
	return append([]base.VirtualFile(nil), d.files...), true
}

// OpenVirtualFile открывает содержимое нашего обещанного файла.
func (c *winClipboard) OpenVirtualFile(index int) (io.ReadCloser, error) {
	d := vfCur.Load()
	if d == nil || d.freed.Load() || index < 0 || index >= len(d.files) || d.files[index].IsDir {
		return nil, base.ErrVirtualFileNotFound
	}
	return d.open(index)
}

// Cleanup снимает обещание, если оно ещё в буфере. Чужое содержимое не трогает.
func (c *winClipboard) Cleanup() {
	d := vfCur.Load()
	if d == nil || !vfT.started.Load() {
		return
	}
	vfT.run(func() {
		if hr, _, _ := procOleIsCurrentClip.Call(d.obj); hr == 0 {
			vfOleSetClipboard(0)
		}
	})
}
