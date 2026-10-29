//go:build windows

package core

import (
	"reflect"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Тесты COM-объектов перетаскивания. Вызывают методы по номерам слотов vtable
// (comCall из a11y_windows_test.go) — ровно так, как их вызывает OLE: проверяют
// и логику, и раскладку таблиц, и счётчики ссылок.

const (
	dropEffectCopy = 1
	dropEffectMove = 2
	dropEffectLink = 4

	// слоты IDropTarget
	slotDragEnter = 3
	slotDragOver  = 4
	slotDragLeave = 5
	slotDrop      = 6

	// слоты IDataObject (3 и 5, 8 — те же, что в слотах рабочего кода)
	slotDataGetDataHere = 4
	slotDataGetCanon    = 6
	slotDataSetData     = 7
	slotDataDAdvise     = 9
	slotDataDUnadvise   = 10
	slotDataEnumDAdvise = 11

	// слоты IDropSource
	slotQueryContinue = 3
	slotGiveFeedback  = 4
)

// comCallP — comCall, который не даёт указателям, превращённым в uintptr при
// вызове, остаться на стеке горутины: метод объекта выполняется Go-колбэком
// на этой же горутине, стек может вырасти и переехать, а ответ записался бы
// по старому адресу (так отказывал бы и настоящий вызов из OLE).
//
//go:uintptrescapes
func comCallP(iface uintptr, slot int, args ...uintptr) uintptr {
	return comCall(iface, slot, args...)
}

func packPt(x, y int32) uintptr { return uintptr(uint32(x)) | uintptr(uint32(y))<<32 }

// newTestData создаёт объект-список файлов, как BeginDragFiles.
func newTestData(paths []string) *dndComObj {
	dndInitVTables()
	o := newDndObj(dndKindData, dndVTData)
	o.hdrop = buildHDrop(paths)
	return o
}

func TestDnd_IUnknown(t *testing.T) {
	dndInitVTables()
	before := dndAlive()
	kinds := []struct {
		name string
		kind dndKind
		vt   uintptr
		iid  *comGUID
	}{
		{"IDropTarget", dndKindTarget, dndVTTarget, &iidIDropTarget},
		{"IDataObject", dndKindData, dndVTData, &iidIDataObject},
		{"IDropSource", dndKindSource, dndVTSource, &iidIDropSource},
	}
	for _, k := range kinds {
		o := newDndObj(k.kind, k.vt)
		this := o.ptr()

		// QueryInterface: IUnknown и собственный интерфейс — да, чужой — нет.
		for _, iid := range []*comGUID{&iidIUnknown, k.iid} {
			var out uintptr
			hr := comCallP(this, slotQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
			if hr != sOK || out != this {
				t.Errorf("%s: QueryInterface(%s) = %#x, out=%#x", k.name, iid, hr, out)
			}
			comCallP(this, slotRelease) // ссылка, выданная QueryInterface
		}
		out := uintptr(0xdead)
		hr := comCallP(this, slotQueryInterface, uintptr(unsafe.Pointer(&iidProviderSimple)), uintptr(unsafe.Pointer(&out)))
		if hr != eNoInterface || out != 0 {
			t.Errorf("%s: чужой IID: hr=%#x out=%#x, ждал E_NOINTERFACE и 0", k.name, hr, out)
		}
		if hr := comCallP(this, slotQueryInterface, uintptr(unsafe.Pointer(k.iid)), 0); hr != eInvalidArg {
			t.Errorf("%s: QueryInterface с нулевым ppv = %#x, ждал E_INVALIDARG", k.name, hr)
		}

		// Счётчик ссылок: создание даёт 1, AddRef — 2, Release — 1, ещё один — 0.
		if n := comCallP(this, slotAddRef); n != 2 {
			t.Errorf("%s: AddRef = %d, ждал 2", k.name, n)
		}
		if n := comCallP(this, slotRelease); n != 1 {
			t.Errorf("%s: Release = %d, ждал 1", k.name, n)
		}
		if n := comCallP(this, slotRelease); n != 0 {
			t.Errorf("%s: последний Release = %d, ждал 0", k.name, n)
		}
	}
	if after := dndAlive(); after != before {
		t.Errorf("объекты утекли из реестра: было %d, стало %d", before, after)
	}
}

func TestDnd_DataObjectGetData(t *testing.T) {
	paths := []string{`C:\temp\a.txt`, `D:\Фото\б в.png`, `\\srv\share\f`}
	o := newTestData(paths)
	defer comCallP(o.ptr(), slotRelease)

	fe := hdropFormat()
	var stg stgMedium
	if hr := comCallP(o.ptr(), slotDataGetData, uintptr(unsafe.Pointer(&fe)), uintptr(unsafe.Pointer(&stg))); hr != sOK {
		t.Fatalf("GetData(CF_HDROP) = %#x", hr)
	}
	if stg.tymed != tymedHGlobal || stg.data == 0 || stg.pUnkForRelease != 0 {
		t.Fatalf("STGMEDIUM = %+v", stg)
	}
	// HGLOBAL читает сама оболочка — как приёмник в чужом процессе.
	if got := hdropPaths(stg.data); !reflect.DeepEqual(got, paths) {
		t.Errorf("DragQueryFileW прочитал %q, ждал %q", got, paths)
	}
	// Тело — тот же блок, что собрали.
	if size, _, _ := procGlobalSize.Call(stg.data); int(size) < len(o.hdrop) {
		t.Errorf("HGLOBAL %d байт, блок %d", size, len(o.hdrop))
	}
	procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&stg)))

	// Каждый GetData отдаёт СВОЮ копию: приёмник освобождает её сам.
	var a, b stgMedium
	comCallP(o.ptr(), slotDataGetData, uintptr(unsafe.Pointer(&fe)), uintptr(unsafe.Pointer(&a)))
	comCallP(o.ptr(), slotDataGetData, uintptr(unsafe.Pointer(&fe)), uintptr(unsafe.Pointer(&b)))
	if a.data == b.data {
		t.Error("два GetData вернули один и тот же HGLOBAL")
	}
	procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&a)))
	procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&b)))
}

func TestDnd_DataObjectRefusals(t *testing.T) {
	o := newTestData([]string{`C:\a`})
	defer comCallP(o.ptr(), slotRelease)

	text := formatEtc{cfFormat: 13, dwAspect: dvAspectContent, lindex: -1, tymed: tymedHGlobal}
	badTymed := hdropFormat()
	badTymed.tymed = 4 // TYMED_ISTREAM
	badAspect := hdropFormat()
	badAspect.dwAspect = 2 // DVASPECT_THUMBNAIL
	good := hdropFormat()

	cases := []struct {
		name string
		fe   *formatEtc
		want uintptr
	}{
		{"другой формат", &text, dvEFormatEtc},
		{"другой носитель", &badTymed, dvETymed},
		{"другой аспект", &badAspect, dvEAspect},
		{"подходит", &good, sOK},
	}
	for _, tc := range cases {
		if hr := comCallP(o.ptr(), slotDataQueryGetData, uintptr(unsafe.Pointer(tc.fe))); hr != tc.want {
			t.Errorf("QueryGetData(%s) = %#x, ждал %#x", tc.name, hr, tc.want)
		}
		var stg stgMedium
		hr := comCallP(o.ptr(), slotDataGetData, uintptr(unsafe.Pointer(tc.fe)), uintptr(unsafe.Pointer(&stg)))
		if hr != tc.want {
			t.Errorf("GetData(%s) = %#x, ждал %#x", tc.name, hr, tc.want)
		}
		if tc.want == sOK {
			procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&stg)))
		} else if stg.data != 0 {
			t.Errorf("GetData(%s) выдал носитель при отказе", tc.name)
		}
	}
	if hr := comCallP(o.ptr(), slotDataGetDataHere, uintptr(unsafe.Pointer(&good)), 0); hr != eNotImpl {
		t.Errorf("GetDataHere = %#x, ждал E_NOTIMPL", hr)
	}
	if hr := comCallP(o.ptr(), slotDataSetData, uintptr(unsafe.Pointer(&good)), 0, 0); hr != eNotImpl {
		t.Errorf("SetData = %#x, ждал E_NOTIMPL", hr)
	}
	if hr := comCallP(o.ptr(), slotDataGetCanon, 0, 0); hr != dataSSameFmt {
		t.Errorf("GetCanonicalFormatEtc = %#x, ждал DATA_S_SAMEFORMATETC", hr)
	}
	for name, slot := range map[string]int{"DAdvise": slotDataDAdvise, "DUnadvise": slotDataDUnadvise, "EnumDAdvise": slotDataEnumDAdvise} {
		if hr := comCallP(o.ptr(), slot, 0, 0, 0, 0); hr != oleEAdviseNoSup {
			t.Errorf("%s = %#x, ждал OLE_E_ADVISENOTSUPPORTED", name, hr)
		}
	}
}

// IEnumFORMATETC от оболочки: единственный формат — CF_HDROP.
func TestDnd_DataObjectEnumFormatEtc(t *testing.T) {
	o := newTestData([]string{`C:\a`})
	defer comCallP(o.ptr(), slotRelease)

	var en uintptr
	if hr := comCallP(o.ptr(), slotDataEnumFormatEtc, dataDirGet, uintptr(unsafe.Pointer(&en))); hr != sOK || en == 0 {
		t.Fatalf("EnumFormatEtc(GET) = %#x, en=%#x", hr, en)
	}
	// IEnumFORMATETC::Next — слот 3.
	var fe formatEtc
	var fetched uint32
	if hr := comCallP(en, 3, 1, uintptr(unsafe.Pointer(&fe)), uintptr(unsafe.Pointer(&fetched))); hr != sOK || fetched != 1 {
		t.Fatalf("Next = %#x, fetched=%d", hr, fetched)
	}
	if fe.cfFormat != cfHDropFormat || fe.tymed != tymedHGlobal {
		t.Errorf("формат перечислителя: %+v", fe)
	}
	if hr := comCallP(en, 3, 1, uintptr(unsafe.Pointer(&fe)), uintptr(unsafe.Pointer(&fetched))); hr != sFalse {
		t.Errorf("второй Next = %#x, ждал S_FALSE (формат один)", hr)
	}
	comCallP(en, slotRelease)

	// Направление SET не поддержано, ppenum обнуляется.
	en = 0xdead
	if hr := comCallP(o.ptr(), slotDataEnumFormatEtc, 2, uintptr(unsafe.Pointer(&en))); hr != eNotImpl || en != 0 {
		t.Errorf("EnumFormatEtc(SET) = %#x, en=%#x", hr, en)
	}
}

func TestDnd_DropSource(t *testing.T) {
	dndInitVTables()
	o := newDndObj(dndKindSource, dndVTSource)
	o.mask = mkLButton
	defer comCallP(o.ptr(), slotRelease)

	cases := []struct {
		esc, keys uintptr
		want      uintptr
	}{
		{0, mkLButton, sOK},
		{0, 0, dragdropSDrop},
		{1, mkLButton, dragdropSCancel},
	}
	for _, tc := range cases {
		if got := comCallP(o.ptr(), slotQueryContinue, tc.esc, tc.keys); got != tc.want {
			t.Errorf("QueryContinueDrag(esc=%d, keys=%#x) = %#x, ждал %#x", tc.esc, tc.keys, got, tc.want)
		}
	}
	if got := comCallP(o.ptr(), slotGiveFeedback, dropEffectCopy); got != dragdropSUseDef {
		t.Errorf("GiveFeedback = %#x, ждал DRAGDROP_S_USEDEFAULTCURSORS", got)
	}
}

// targetFixture — окно без hwnd (координаты не переводятся) и его IDropTarget.
type targetFixture struct {
	w      *Win32Window
	target *dndComObj
	log    []string
	drops  [][]string
	dropAt [][2]int
	accept bool
}

func newTargetFixture(t *testing.T) *targetFixture {
	t.Helper()
	dndInitVTables()
	f := &targetFixture{w: &Win32Window{}, accept: true}
	f.target = newDndObj(dndKindTarget, dndVTTarget)
	f.target.win = f.w
	f.w.SetOnDragEnter(func(x, y int) { f.log = append(f.log, "enter") })
	f.w.SetOnDragOver(func(x, y int) bool { f.log = append(f.log, "over"); return f.accept })
	f.w.SetOnDragLeave(func() { f.log = append(f.log, "leave") })
	f.w.SetOnFilesDropped(nil)
	f.w.onFilesDropped = func(p []string, x, y int) {
		f.drops = append(f.drops, p)
		f.dropAt = append(f.dropAt, [2]int{x, y})
	}
	t.Cleanup(func() { comCallP(f.target.ptr(), slotRelease) })
	return f
}

// Настоящий цикл приёмника: вход с файлами, движение, выход.
func TestDnd_TargetEnterOverLeave(t *testing.T) {
	f := newTargetFixture(t)
	data := newTestData([]string{`C:\a.png`})
	defer comCallP(data.ptr(), slotRelease)

	eff := new(uint32)
	*eff = dropEffectCopy | dropEffectMove | dropEffectLink
	if hr := comCallP(f.target.ptr(), slotDragEnter, data.ptr(), 0, packPt(10, 20), uintptr(unsafe.Pointer(eff))); hr != sOK {
		t.Fatalf("DragEnter = %#x", hr)
	}
	if *eff != dropEffectCopy {
		t.Errorf("действие на входе %d, ждал копирование", *eff)
	}

	// Shift — перемещение, Ctrl+Shift — ссылка (как в Проводнике).
	for _, tc := range []struct {
		keys uintptr
		want uint32
	}{{0, dropEffectCopy}, {mkShift, dropEffectMove}, {mkControl, dropEffectCopy}, {mkControl | mkShift, dropEffectLink}} {
		*eff = dropEffectCopy | dropEffectMove | dropEffectLink
		comCallP(f.target.ptr(), slotDragOver, tc.keys, packPt(11, 21), uintptr(unsafe.Pointer(eff)))
		if *eff != tc.want {
			t.Errorf("DragOver keys=%#x: действие %d, ждал %d", tc.keys, *eff, tc.want)
		}
	}

	// Приложение отказало: действие «нельзя».
	f.accept = false
	*eff = dropEffectCopy
	comCallP(f.target.ptr(), slotDragOver, 0, packPt(12, 22), uintptr(unsafe.Pointer(eff)))
	if *eff != 0 {
		t.Errorf("при отказе приложения действие %d, ждал 0", *eff)
	}

	if hr := comCallP(f.target.ptr(), slotDragLeave); hr != sOK {
		t.Errorf("DragLeave = %#x", hr)
	}
	want := []string{"enter", "over", "over", "over", "over", "over", "over", "leave"}
	if !reflect.DeepEqual(f.log, want) {
		t.Errorf("события %v, ждал %v", f.log, want)
	}
}

// Без файлов (текст, изображение из браузера) окно сброс не принимает и
// событий приложению не шлёт.
func TestDnd_TargetIgnoresNonFiles(t *testing.T) {
	f := newTargetFixture(t)
	empty := newDndObj(dndKindData, dndVTData) // hdrop пуст: CF_HDROP нет
	defer comCallP(empty.ptr(), slotRelease)

	eff := new(uint32)
	*eff = dropEffectCopy
	comCallP(f.target.ptr(), slotDragEnter, empty.ptr(), 0, packPt(1, 1), uintptr(unsafe.Pointer(eff)))
	if *eff != 0 {
		t.Errorf("действие для не-файлов: %d, ждал 0", *eff)
	}
	comCallP(f.target.ptr(), slotDragOver, 0, packPt(2, 2), uintptr(unsafe.Pointer(eff)))
	comCallP(f.target.ptr(), slotDragLeave)
	if len(f.log) != 0 {
		t.Errorf("приложение получило события для не-файлов: %v", f.log)
	}
}

// Сброс: пути из CF_HDROP доходят до SetOnFilesDropped с точкой сброса.
func TestDnd_TargetDrop(t *testing.T) {
	f := newTargetFixture(t)
	paths := []string{`C:\Фото\a.png`, `C:\b.jpg`}
	data := newTestData(paths)
	defer comCallP(data.ptr(), slotRelease)

	eff := new(uint32)
	*eff = dropEffectCopy | dropEffectMove
	comCallP(f.target.ptr(), slotDragEnter, data.ptr(), 0, packPt(5, 6), uintptr(unsafe.Pointer(eff)))
	*eff = dropEffectCopy | dropEffectMove
	if hr := comCallP(f.target.ptr(), slotDrop, data.ptr(), 0, packPt(30, 40), uintptr(unsafe.Pointer(eff))); hr != sOK {
		t.Fatalf("Drop = %#x", hr)
	}
	if *eff != dropEffectCopy {
		t.Errorf("действие при сбросе %d, ждал копирование", *eff)
	}
	if !reflect.DeepEqual(f.drops, [][]string{paths}) {
		t.Errorf("SetOnFilesDropped получил %q, ждал %q", f.drops, paths)
	}
	if !reflect.DeepEqual(f.dropAt, [][2]int{{30, 40}}) {
		t.Errorf("точка сброса %v, ждал [30 40]", f.dropAt)
	}
	// После сброса leave не приходит: перетаскивание кончилось.
	if n := len(f.log); n == 0 || f.log[n-1] == "leave" {
		t.Errorf("события %v: после Drop не должно быть leave", f.log)
	}
}

// Сброс без файлов: колбэк не вызывается, действие «нельзя».
func TestDnd_TargetDropWithoutFiles(t *testing.T) {
	f := newTargetFixture(t)
	empty := newDndObj(dndKindData, dndVTData)
	defer comCallP(empty.ptr(), slotRelease)
	eff := new(uint32)
	*eff = dropEffectCopy
	comCallP(f.target.ptr(), slotDrop, empty.ptr(), 0, packPt(1, 1), uintptr(unsafe.Pointer(eff)))
	if len(f.drops) != 0 || *eff != 0 {
		t.Errorf("drops=%v eff=%d", f.drops, *eff)
	}
}

// Мусор в аргументах не роняет процесс.
func TestDnd_NilArguments(t *testing.T) {
	f := newTargetFixture(t)
	comCallP(f.target.ptr(), slotDragEnter, 0, 0, 0, 0)
	comCallP(f.target.ptr(), slotDragOver, 0, 0, 0)
	comCallP(f.target.ptr(), slotDrop, 0, 0, 0, 0)
	comCallP(f.target.ptr(), slotDragLeave)
	var stg stgMedium
	o := newTestData([]string{`C:\a`})
	defer comCallP(o.ptr(), slotRelease)
	if hr := comCallP(o.ptr(), slotDataGetData, 0, uintptr(unsafe.Pointer(&stg))); hr != eInvalidArg {
		t.Errorf("GetData(nil) = %#x", hr)
	}
	if hr := comCallP(o.ptr(), slotDataQueryGetData, 0); hr != eInvalidArg {
		t.Errorf("QueryGetData(nil) = %#x", hr)
	}
}

// createTestWindow создаёт настоящее окно STATIC на потоке теста (поток
// закрепляется за горутиной: RegisterDragDrop и OLE привязаны к потоку).
func createTestWindow(t *testing.T) uintptr {
	t.Helper()
	runtime.LockOSThread()
	class, _ := windows.UTF16PtrFromString("STATIC")
	hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(class)), 0,
		0, 0, 0, 10, 10, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Skipf("CreateWindowEx: %v", err)
	}
	t.Cleanup(func() { procDestroyWindow.Call(hwnd) })
	return hwnd
}

// Окно регистрируется приёмником OLE и снимает регистрацию: на потоке
// теста (OLE поднимается сам).
func TestDnd_RegisterRevoke(t *testing.T) {
	hwnd := createTestWindow(t)
	w := &Win32Window{hwnd: windows.HWND(hwnd)}
	before := dndAlive()
	if !w.registerDropTarget() {
		t.Skip("RegisterDragDrop недоступен на этом потоке")
	}
	if !w.oleReady || w.dropTarget == nil {
		t.Fatal("после регистрации oleReady/dropTarget не выставлены")
	}
	// Повторная регистрация того же окна отказывает: запасной путь включится.
	w2 := &Win32Window{hwnd: windows.HWND(hwnd)}
	if w2.registerDropTarget() {
		t.Error("второй приёмник на то же окно принят")
	}
	w.revokeDropTarget()
	if w.dropTarget != nil {
		t.Error("dropTarget не обнулён после revoke")
	}
	if after := dndAlive(); after != before {
		t.Errorf("объекты утекли: %d -> %d", before, after)
	}
	// Теперь регистрация снова возможна.
	w3 := &Win32Window{hwnd: windows.HWND(hwnd)}
	if !w3.registerDropTarget() {
		t.Error("после revoke регистрация отказала")
	}
	w3.revokeDropTarget()
}
