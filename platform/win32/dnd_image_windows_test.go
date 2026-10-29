//go:build windows

package win32

import (
	"encoding/binary"
	"image"
	"image/color"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Значок перетаскивания Win32: хранилище SetData/GetData в нашем IDataObject и
// живой IDragSourceHelper оболочки (CLSID_DragDropHelper есть в любой
// настольной Windows). Курсор не двигается.

var procRegisterClipboardFormatW = user32.NewProc("RegisterClipboardFormatW")

func dndFormat(name string) uint16 {
	p, _ := windows.UTF16PtrFromString(name)
	id, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(p)))
	return uint16(id)
}

// dndLockedOLE поднимает OLE на потоке теста (как поток окна).
func dndLockedOLE(t *testing.T) {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	if !oleInit() {
		t.Skip("OLE на этом потоке не поднялся")
	}
}

func dndStg(blob []byte) stgMedium {
	return stgMedium{tymed: tymedHGlobal, data: newGlobalCopy(blob)}
}

// SetData кладёт блок, GetData/QueryGetData отдают его обратно; перечислитель
// видит и CF_HDROP, и блок.
func TestDnd_DataObjectSetData(t *testing.T) {
	o := newTestData([]string{`C:\a.txt`})
	defer comCallP(o.ptr(), slotRelease)
	cf := dndFormat("HeadlessGuiTestBlock")

	fe := formatEtc{cfFormat: cf, dwAspect: dvAspectContent, lindex: -1, tymed: tymedHGlobal}
	if hr := comCallP(o.ptr(), slotDataQueryGetData, uintptr(unsafe.Pointer(&fe))); hr != dvEFormatEtc {
		t.Fatalf("до SetData QueryGetData = %#x, ждал DV_E_FORMATETC", hr)
	}

	blob := []byte{1, 2, 3, 4, 5, 6, 7}
	stg := dndStg(blob)
	// fRelease = 1: объект отвечает за среду, а значит, освобождает её.
	if hr := comCallP(o.ptr(), slotDataSetData, uintptr(unsafe.Pointer(&fe)), uintptr(unsafe.Pointer(&stg)), 1); hr != SOK {
		t.Fatalf("SetData = %#x", hr)
	}
	if hr := comCallP(o.ptr(), slotDataQueryGetData, uintptr(unsafe.Pointer(&fe))); hr != SOK {
		t.Errorf("после SetData QueryGetData = %#x", hr)
	}
	var got stgMedium
	if hr := comCallP(o.ptr(), slotDataGetData, uintptr(unsafe.Pointer(&fe)), uintptr(unsafe.Pointer(&got))); hr != SOK {
		t.Fatalf("GetData блока = %#x", hr)
	}
	if b := globalBytes(got.data); string(b) != string(blob) {
		t.Errorf("GetData вернул %v, положили %v", b, blob)
	}
	procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&got)))

	// Тот же блок в виде IStream: оболочка просит и так.
	asStream := fe
	asStream.tymed = tymedIStream
	var sg stgMedium
	if hr := comCallP(o.ptr(), slotDataGetData, uintptr(unsafe.Pointer(&asStream)), uintptr(unsafe.Pointer(&sg))); hr != SOK || sg.tymed != tymedIStream || sg.data == 0 {
		t.Fatalf("GetData как IStream = %#x, среда %+v", hr, sg)
	}
	if b := streamBytes(sg.data); string(b) != string(blob) {
		t.Errorf("IStream отдал %v, положили %v", b, blob)
	}
	procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&sg)))

	// Чужая среда и аспект отвергаются, список файлов по-прежнему на месте.
	bad := fe
	bad.tymed = 2 // TYMED_FILE
	if hr := comCallP(o.ptr(), slotDataQueryGetData, uintptr(unsafe.Pointer(&bad))); hr != dvETymed {
		t.Errorf("QueryGetData с файлом = %#x, ждал DV_E_TYMED", hr)
	}
	hd := hdropFormat()
	if hr := comCallP(o.ptr(), slotDataQueryGetData, uintptr(unsafe.Pointer(&hd))); hr != SOK {
		t.Errorf("CF_HDROP пропал после SetData: %#x", hr)
	}
	// CF_HDROP через SetData не подменить.
	stg2 := dndStg([]byte{9})
	if hr := comCallP(o.ptr(), slotDataSetData, uintptr(unsafe.Pointer(&hd)), uintptr(unsafe.Pointer(&stg2)), 0); hr != ENotImpl {
		t.Errorf("SetData(CF_HDROP) = %#x, ждал E_NOTIMPL", hr)
	}
	procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&stg2)))

	var en uintptr
	if hr := comCallP(o.ptr(), slotDataEnumFormatEtc, dataDirGet, uintptr(unsafe.Pointer(&en))); hr != SOK || en == 0 {
		t.Fatalf("EnumFormatEtc = %#x", hr)
	}
	defer comCallP(en, slotRelease)
	var fes [3]formatEtc
	var fetched uint32
	comCallP(en, 3, 3, uintptr(unsafe.Pointer(&fes[0])), uintptr(unsafe.Pointer(&fetched)))
	if fetched != 2 || fes[0].cfFormat != cfHDropFormat || fes[1].cfFormat != cf || fes[1].tymed != tymedHGlobal|tymedIStream {
		t.Errorf("перечислитель: %d форматов %+v", fetched, fes[:fetched])
	}
}

// Живая оболочка: IDragSourceHelper принимает картинку и раскладывает её по
// нашему объекту через SetData; блок «DragImageBits» читается обратно, в нём
// размер значка.
func TestDnd_InitDragImage_Live(t *testing.T) {
	dndLockedOLE(t)
	o := newTestData([]string{`C:\a.txt`})
	defer comCallP(o.ptr(), slotRelease)

	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 200, G: 80, B: 20, A: 255})
		}
	}
	if h := createDragHelper(&iidIDragSourceHelper); h == 0 {
		t.Skip("IDragSourceHelper недоступен в этой среде")
	} else {
		comInvoke(h, slotUnkRelease)
	}
	if !initDragImage(o.ptr(), img, image.Pt(10, 12)) {
		t.Fatal("помощник есть, а картинку не принял")
	}

	cf := dndFormat("DragImageBits")
	fe := formatEtc{cfFormat: cf, dwAspect: dvAspectContent, lindex: -1, tymed: tymedHGlobal}
	if hr := comCallP(o.ptr(), slotDataQueryGetData, uintptr(unsafe.Pointer(&fe))); hr != SOK {
		t.Fatalf("после InitializeFromBitmap в объекте нет «DragImageBits»: %#x (форматы: %v)", hr, o.extraFormats())
	}
	var stg stgMedium
	if hr := comCallP(o.ptr(), slotDataGetData, uintptr(unsafe.Pointer(&fe)), uintptr(unsafe.Pointer(&stg))); hr != SOK {
		t.Fatalf("GetData(DragImageBits) = %#x", hr)
	}
	defer procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&stg)))
	blob := globalBytes(stg.data)
	if len(blob) < 8+64*48*4 {
		t.Fatalf("блок «DragImageBits» %d байт, ждал не меньше пикселей картинки (%d)", len(blob), 64*48*4)
	}
	// SHDRAGIMAGE в начале блока: размер, затем точка захвата.
	cx, cy := int32(binary.LittleEndian.Uint32(blob[0:4])), int32(binary.LittleEndian.Uint32(blob[4:8]))
	if cx != 64 || cy != 48 {
		t.Errorf("значок в объекте %dx%d, отдали 64x48", cx, cy)
	}
	if o.refs.Load() != 1 {
		t.Errorf("счётчик ссылок объекта %d: помощник не отпустил его", o.refs.Load())
	}
}

// Не принятая оболочкой картинка не должна утекать: пустой/нулевой размер и
// пустой объект — отказ без падения.
func TestDnd_InitDragImage_Refusals(t *testing.T) {
	dndLockedOLE(t)
	o := newTestData([]string{`C:\a.txt`})
	defer comCallP(o.ptr(), slotRelease)
	if initDragImage(o.ptr(), nil, image.Point{}) {
		t.Error("nil-картинка принята")
	}
	if initDragImage(0, image.NewRGBA(image.Rect(0, 0, 4, 4)), image.Point{}) {
		t.Error("нулевой объект данных принят")
	}
	if initDragImage(o.ptr(), image.NewRGBA(image.Rect(0, 0, 0, 0)), image.Point{}) {
		t.Error("пустая картинка принята")
	}
}

// Приёмник окна подключает IDropTargetHelper: вход создаёт помощника, выход и
// сброс его «снимают». Помощник живой, значок над настоящим окном.
func TestDnd_TargetHelper_Live(t *testing.T) {
	dndLockedOLE(t)
	w := newFsWindow(t)
	dndInitVTables()
	target := newDndObj(dndKindTarget, dndVTTarget)
	target.win = w
	w.dropTarget = &dropTargetState{obj: target}
	t.Cleanup(func() {
		if w.dropTarget.helper != 0 {
			comInvoke(w.dropTarget.helper, slotUnkRelease)
		}
		w.dropTarget = nil
		comCallP(target.ptr(), slotRelease)
	})
	w.SetOnDragOver(func(x, y int) bool { return true })

	data := newTestData([]string{`C:\a.png`})
	defer comCallP(data.ptr(), slotRelease)
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for i := range img.Pix {
		img.Pix[i] = 0xFF
	}
	initDragImage(data.ptr(), img, image.Pt(4, 4))

	eff := new(uint32)
	*eff = dropEffectCopy
	if hr := comCallP(target.ptr(), slotDragEnter, data.ptr(), 0, packPt(10, 20), uintptr(unsafe.Pointer(eff))); hr != SOK {
		t.Fatalf("DragEnter = %#x", hr)
	}
	if h := createDragHelper(&iidIDropTargetHelper); h == 0 {
		t.Skip("IDropTargetHelper недоступен в этой среде")
	} else {
		comInvoke(h, slotUnkRelease)
	}
	if w.dropTarget.helper == 0 {
		t.Fatal("вход не создал IDropTargetHelper окна")
	}
	if !target.helperOn {
		t.Error("помощник не получил DragEnter")
	}
	*eff = dropEffectCopy
	comCallP(target.ptr(), slotDragOver, 0, packPt(12, 22), uintptr(unsafe.Pointer(eff)))
	if !target.helperOn {
		t.Error("DragOver снял помощника")
	}
	comCallP(target.ptr(), slotDragLeave)
	if target.helperOn {
		t.Error("после DragLeave помощник остался включённым")
	}

	// Сброс тоже снимает.
	comCallP(target.ptr(), slotDragEnter, data.ptr(), 0, packPt(10, 20), uintptr(unsafe.Pointer(eff)))
	if !target.helperOn {
		t.Fatal("повторный вход не включил помощника")
	}
	*eff = dropEffectCopy
	comCallP(target.ptr(), slotDrop, data.ptr(), 0, packPt(10, 20), uintptr(unsafe.Pointer(eff)))
	if target.helperOn {
		t.Error("после Drop помощник остался включённым")
	}
}
