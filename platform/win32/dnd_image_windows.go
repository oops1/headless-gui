//go:build windows

package win32

// dnd_image_windows.go — значок перетаскивания на Win32 (BeginDragFilesImage).
//
// Значок рисует оболочка, а не мы. Источник отдаёт ей картинку через
// IDragSourceHelper.InitializeFromBitmap: помощник кладёт в наш IDataObject
// блок «DragImageBits» (и служебные форматы) методом SetData. Приёмник —
// чужая программа или это же окно — вызывает IDropTargetHelper, тот читает
// блок из объекта и рисует многослойное окно под курсором. Поэтому у окна
// есть и приёмный помощник: без него значок был бы виден только над чужими
// окнами.
//
// Всё, что здесь не получилось (помощника нет в системе, картинка не
// принята), молча сводится к перетаскиванию без значка: оно важнее картинки.

import (
	"image"
	"unsafe"
)

// shDragImage — SHDRAGIMAGE (32 байта на x64).
type shDragImage struct {
	cx, cy     int32   // размер картинки
	offX, offY int32   // точка картинки под курсором
	hbmp       uintptr // HBITMAP (32bpp, премультиплицированный); после успеха — собственность оболочки
	colorKey   uint32  // COLORREF прозрачного цвета; CLR_NONE — альфа-канал
}

const clrNone = 0xFFFFFFFF

// createDragHelper создаёт помощника оболочки (CLSID_DragDropHelper) с
// интерфейсом iid. 0 — не вышло.
func createDragHelper(iid *GUID) uintptr {
	var h uintptr
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidDragDropHelper)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&h)))
	if int32(hr) < 0 {
		return 0
	}
	return h
}

// initDragImage отдаёт оболочке значок для объекта данных dataObj (наш COM-
// объект). true — оболочка приняла картинку и владеет теперь её HBITMAP.
// Вызывать на потоке с OLE.
func initDragImage(dataObj uintptr, img *image.RGBA, hotspot image.Point) bool {
	if img == nil || dataObj == 0 {
		return false
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	hbmp := scaledDIB(img, w, h)
	if hbmp == 0 {
		return false
	}
	helper := createDragHelper(&iidIDragSourceHelper)
	if helper == 0 {
		procDeleteObject.Call(hbmp)
		return false
	}
	defer comInvoke(helper, slotUnkRelease)

	sd := shDragImage{
		cx: int32(w), cy: int32(h),
		offX: int32(hotspot.X), offY: int32(hotspot.Y),
		hbmp: hbmp, colorKey: clrNone,
	}
	hr := comInvoke(helper, slotDragSrcFromBitmap, uintptr(unsafe.Pointer(&sd)), dataObj)
	if int32(hr) < 0 {
		procDeleteObject.Call(hbmp) // не принят — картинка по-прежнему наша
		return false
	}
	return true
}

// ─── Приёмный помощник окна ──────────────────────────────────────────────────

// targetHelper возвращает IDropTargetHelper окна, создавая его при первом
// обращении; 0 — недоступен (повторно не пробуем).
func (o *dndComObj) targetHelper() uintptr {
	if o.win == nil || o.win.dropTarget == nil {
		return 0
	}
	st := o.win.dropTarget
	if st.helper == 0 && !st.helperFail {
		if st.helper = createDragHelper(&iidIDropTargetHelper); st.helper == 0 {
			st.helperFail = true
		}
	}
	return st.helper
}

func dndScreenPoint(pt uintptr) point {
	return point{X: int32(uint32(pt)), Y: int32(uint32(pt >> 32))}
}

func dndEffectOf(pdwEffect uintptr) uintptr {
	if pdwEffect == 0 {
		return 0
	}
	return uintptr(*ptrAt[uint32](pdwEffect))
}

// helperEnter — DragEnter помощника: перетаскивание вошло в окно.
func (o *dndComObj) helperEnter(dataObj, pt, pdwEffect uintptr) {
	h := o.targetHelper()
	if h == 0 || dataObj == 0 || o.win == nil {
		return
	}
	p := dndScreenPoint(pt)
	comInvoke(h, slotDropHelperEnter, uintptr(o.win.hwnd), dataObj,
		uintptr(unsafe.Pointer(&p)), dndEffectOf(pdwEffect))
	o.helperOn = true
}

// helperCall — DragOver помощника (slot — slotDropHelperOver).
func (o *dndComObj) helperCall(slot int, pt, pdwEffect uintptr) {
	if !o.helperOn {
		return
	}
	h := o.targetHelper()
	if h == 0 {
		return
	}
	p := dndScreenPoint(pt)
	comInvoke(h, slot, uintptr(unsafe.Pointer(&p)), dndEffectOf(pdwEffect))
}

// helperLeave — DragLeave помощника: значок убирается.
func (o *dndComObj) helperLeave() {
	if !o.helperOn {
		return
	}
	o.helperOn = false
	if h := o.targetHelper(); h != 0 {
		comInvoke(h, slotDropHelperLeave)
	}
}

// helperDrop — Drop помощника: сброс состоялся, значок убирается.
func (o *dndComObj) helperDrop(dataObj, pt, pdwEffect uintptr) {
	if !o.helperOn {
		return
	}
	o.helperOn = false
	h := o.targetHelper()
	if h == 0 || dataObj == 0 {
		return
	}
	p := dndScreenPoint(pt)
	comInvoke(h, slotDropHelperDrop, dataObj, uintptr(unsafe.Pointer(&p)), dndEffectOf(pdwEffect))
}
