//go:build windows

package widget

import (
	"bytes"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"
	"unsafe"
)

// Тесты COM-объектов виртуальных файлов. Первые не трогают системный буфер: они
// вызывают наши объекты по таблицам виртуальных методов, как это делает ОС.
// Последний — на настоящем буфере, поэтому ему нужен
// HEADLESS_GUI_TEST_SYSCLIPBOARD=1 (на CI запускается сам).

// vfTestSetText кладёт текст и проверяет, что он лёг: сразу после изменения
// буфер на миг занимают слушатели (история буфера), и SetText молча уходит
// впустую — тест не должен падать из-за этого.
func vfTestSetText(t *testing.T, c *winClipboard, s string) {
	t.Helper()
	for i := 0; i < 50; i++ {
		c.SetText(s)
		if c.GetText() == s {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Skip("буфер обмена занят другим процессом")
}

// vfTestGetText читает текст, повторяя попытку: GetText на занятом буфере
// отвечает пустотой, как и на пустом, а затёртый буфер пуст стабильно.
func vfTestGetText(c *winClipboard) string {
	var s string
	for i := 0; i < 25; i++ {
		if s = c.GetText(); s != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return s
}

// vfTestReader — читатель, который знает, закрыт ли он.
type vfTestReader struct {
	io.Reader
	closed *atomic.Int32
}

func (r vfTestReader) Close() error { r.closed.Add(1); return nil }

// vfTestSource — тестовый источник: три файла и счётчики open/close.
type vfTestSource struct {
	opens, closes atomic.Int32
	files         []VirtualFile
	body          map[int]string
}

func newVFTestSource() *vfTestSource {
	return &vfTestSource{
		files: []VirtualFile{
			{Name: "Отчёт", IsDir: true, ModTime: time.Unix(1_700_000_000, 0)},
			{Name: "Отчёт/итог.txt", Size: 11, ModTime: time.Unix(1_700_000_000, 0)},
			{Name: "пусто.bin"},
			{Name: "большой.bin", Size: vfMaxHGlobal + 1},
		},
		body: map[int]string{1: "hello world", 2: "", 3: "x"},
	}
}

func (s *vfTestSource) open(i int) (io.ReadCloser, error) {
	s.opens.Add(1)
	b, ok := s.body[i]
	if !ok {
		return nil, errors.New("нет такого файла")
	}
	return vfTestReader{strings.NewReader(b), &s.closes}, nil
}

// newObj собирает объект данных без обращения к буферу обмена.
func (s *vfTestSource) newObj(t *testing.T) (obj uintptr, d *vfData) {
	t.Helper()
	desc, err := buildFileGroupDescriptor(s.files)
	if err != nil {
		t.Fatal(err)
	}
	vfFormats()
	d = &vfData{files: s.files, open: s.open, desc: desc}
	o := newVFObj(vfKindData)
	o.data = d
	return o.ptr(), d
}

func vfFE(cf uint16, lindex int32, tymed uint32) *vfFormatEtc {
	return &vfFormatEtc{cfFormat: cf, dwAspect: vfAspect, lindex: lindex, tymed: tymed}
}

func vfGetData(obj uintptr, fe *vfFormatEtc) (vfStgMedium, uintptr) {
	var med vfStgMedium
	hr := vfInvoke(obj, vfSlotGetData, uintptr(unsafe.Pointer(fe)), uintptr(unsafe.Pointer(&med)))
	return med, hr
}

// vfGetDataRetry — vfGetData для объекта из OleGetClipboard: его обёртка
// открывает буфер на каждый вызов, и тот на миг бывает занят слушателем
// (CLIPBRD_E_CANT_OPEN, а иногда и DV_E_FORMATETC, пока формат «не виден» из-за
// занятого буфера) — настоящий вставщик повторяет попытку.
func vfGetDataRetry(obj uintptr, fe *vfFormatEtc) (med vfStgMedium, hr uintptr) {
	for i := 0; i < 50; i++ {
		if med, hr = vfGetData(obj, fe); uint32(hr) != vfClipCantOpen && hr != vfDVFormatEtc {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return med, hr
}

func vfQuery(obj uintptr, fe *vfFormatEtc) uintptr {
	return vfInvoke(obj, vfSlotQueryGet, uintptr(unsafe.Pointer(fe)))
}

func vfReleaseMedium(med *vfStgMedium) {
	procReleaseStgMediumClip.Call(uintptr(unsafe.Pointer(med)))
}

// Раскладка таблиц: 12 слотов у IDataObject (3 + 9 методов) и 14 у IStream
// (3 + Read/Write + 9 методов IStream); общий IUnknown, остальное — свои
// колбэки, и ни один слот не пуст.
func TestVirtualCOM_VTableLayout(t *testing.T) {
	vfInitVTables()
	if len(vfVT.data) != vfIDataObjectSlots || len(vfVT.stream) != vfIStreamSlots {
		t.Fatalf("слотов: IDataObject %d (ждал %d), IStream %d (ждал %d)",
			len(vfVT.data), vfIDataObjectSlots, len(vfVT.stream), vfIStreamSlots)
	}
	for _, tbl := range [][]uintptr{vfVT.data, vfVT.stream} {
		seen := map[uintptr]int{}
		for i, fn := range tbl {
			if fn == 0 {
				t.Errorf("слот %d пуст", i)
			}
			seen[fn] = i
		}
		// Каждый метод — свой колбэк; допустимо совпадение лишь у Lock/Unlock
		// потока (общий отказ) — отсюда на один адрес меньше.
		if len(seen) < len(tbl)-1 {
			t.Errorf("в таблице из %d слотов лишь %d разных колбэков", len(tbl), len(seen))
		}
	}
	for i := 0; i < 3; i++ {
		if vfVT.data[i] != vfVT.stream[i] {
			t.Errorf("слот IUnknown %d у таблиц разный", i)
		}
	}
	// У объекта таблица — первое слово, и в ней лежит ровно то, что мы собрали.
	src := newVFTestSource()
	obj, _ := src.newObj(t)
	if got := *vfPtrAt[uintptr](obj); got != vfVT.dataPtr {
		t.Error("первое слово объекта данных — не таблица IDataObject")
	}
	vfRelease(obj)
}

// QueryInterface / AddRef / Release: свои интерфейсы отвечают, чужие — нет;
// объект освобождается ровно на последнем Release и тогда зовёт release.
func TestVirtualCOM_Unknown(t *testing.T) {
	src := newVFTestSource()
	before := vfAlive()
	obj, d := src.newObj(t)
	released := make(chan struct{}, 2)
	d.release = func() { released <- struct{}{} }

	var out uintptr
	for _, iid := range []*vfGUID{&vfIIDUnknown, &vfIIDDataObject} {
		out = 0
		if hr := vfInvoke(obj, 0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out))); hr != 0 || out != obj {
			t.Errorf("QueryInterface(%v) = %#x out=%#x", iid, hr, out)
		}
	}
	// Два успешных QueryInterface дали по ссылке: счётчик 1 + 2.
	if n := vfInvoke(obj, 1); n != 4 {
		t.Errorf("AddRef = %d, ждал 4", n)
	}
	out = 1
	if hr := vfInvoke(obj, 0, uintptr(unsafe.Pointer(&vfIIDStream)), uintptr(unsafe.Pointer(&out))); hr != vfENoInterface || out != 0 {
		t.Errorf("QueryInterface(IStream) у объекта данных = %#x out=%#x", hr, out)
	}
	if hr := vfInvoke(obj, 0, uintptr(unsafe.Pointer(&vfIIDUnknown)), 0); hr != vfEInvalidArg {
		t.Errorf("QueryInterface без out = %#x", hr)
	}
	for want := uintptr(3); want >= 1; want-- {
		if n := vfInvoke(obj, vfSlotRelease); n != want {
			t.Errorf("Release = %d, ждал %d", n, want)
		}
	}
	select {
	case <-released:
		t.Fatal("release вызван, пока ссылки остались")
	case <-time.After(20 * time.Millisecond):
	}
	if vfAlive() != before+1 {
		t.Error("объект исчез из реестра раньше последнего Release")
	}
	if n := vfInvoke(obj, vfSlotRelease); n != 0 {
		t.Errorf("последний Release = %d", n)
	}
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("release не вызван на последнем Release")
	}
	if vfAlive() != before || !d.freed.Load() {
		t.Errorf("объект остался в реестре (%d, было %d) или freed=%v", vfAlive(), before, d.freed.Load())
	}
}

// GetData(FileGroupDescriptorW) отдаёт ровно собранный блок; форматы и
// среды проверяются так, как это делает Проводник через QueryGetData.
func TestVirtualCOM_GetDataDescriptor(t *testing.T) {
	src := newVFTestSource()
	obj, d := src.newObj(t)
	defer vfRelease(obj)
	desc, contents, drop := vfFormats()

	med, hr := vfGetData(obj, vfFE(desc, -1, vfTymedHGlobal))
	if hr != 0 || med.tymed != vfTymedHGlobal || med.data == 0 {
		t.Fatalf("GetData(descriptor) = %#x %+v", hr, med)
	}
	if got := clipboardBlock(med.data); !bytes.Equal(got, d.desc) {
		t.Errorf("блок дескрипторов не совпал: %d байт против %d", len(got), len(d.desc))
	}
	vfReleaseMedium(&med)

	med, hr = vfGetData(obj, vfFE(drop, -1, vfTymedHGlobal))
	if blk := clipboardBlock(med.data); hr != 0 || len(blk) != 4 || blk[0] != dropEffectCopy || dropEffectIsCut(blk) {
		t.Errorf("Preferred DropEffect: %#x %v", hr, blk)
	}
	vfReleaseMedium(&med)

	bad := &vfFormatEtc{cfFormat: desc, dwAspect: 2 /*THUMBNAIL*/, lindex: -1, tymed: vfTymedHGlobal}
	for name, c := range map[string]struct {
		fe   *vfFormatEtc
		want uintptr
	}{
		"дескриптор":              {vfFE(desc, -1, vfTymedHGlobal), 0},
		"дескриптор в потоке":     {vfFE(desc, -1, vfTymedIStream), vfDVTymed},
		"дескриптор и оба вида":   {vfFE(desc, -1, vfTymedHGlobal|vfTymedIStream), 0},
		"чужой аспект":            {bad, vfDVAspect},
		"чужой формат":            {vfFE(cfUnicodeText, -1, vfTymedHGlobal), vfDVFormatEtc},
		"содержимое в потоке":     {vfFE(contents, 1, vfTymedIStream), 0},
		"содержимое lindex -1":    {vfFE(contents, -1, vfTymedIStream), 0},
		"содержимое вне списка":   {vfFE(contents, 4, vfTymedIStream), vfDVLindex},
		"содержимое lindex -2":    {vfFE(contents, -2, vfTymedIStream), vfDVLindex},
		"содержимое каталога":     {vfFE(contents, 0, vfTymedIStream), vfDVLindex},
		"содержимое без среды":    {vfFE(contents, 1, 0), vfDVTymed},
		"малый файл в HGLOBAL":    {vfFE(contents, 1, vfTymedHGlobal), 0},
		"большой файл в HGLOBAL":  {vfFE(contents, 3, vfTymedHGlobal), vfDVTymed},
		"большой файл в потоке":   {vfFE(contents, 3, vfTymedIStream), 0},
		"содержимое drop-effect":  {vfFE(drop, -1, vfTymedHGlobal), 0},
		"содержимое drop-effect2": {vfFE(drop, -1, vfTymedIStream), vfDVTymed},
	} {
		if got := vfQuery(obj, c.fe); got != c.want {
			t.Errorf("QueryGetData %s = %#x, ждал %#x", name, got, c.want)
		}
	}
	if src.opens.Load() != 0 {
		t.Errorf("QueryGetData и GetData дескриптора позвали open %d раз", src.opens.Load())
	}
}

// Остальные методы IDataObject: перечислитель форматов, отказы, реклама.
func TestVirtualCOM_EnumFormatsAndStubs(t *testing.T) {
	src := newVFTestSource()
	obj, _ := src.newObj(t)
	defer vfRelease(obj)
	desc, contents, drop := vfFormats()

	var en uintptr
	if hr := vfInvoke(obj, vfSlotEnumFmt, 1 /*DATADIR_GET*/, uintptr(unsafe.Pointer(&en))); hr != 0 || en == 0 {
		t.Fatalf("EnumFormatEtc = %#x", hr)
	}
	defer vfInvoke(en, vfSlotRelease)
	var fes [4]vfFormatEtc
	var n uint32
	vfInvoke(en, 3 /*Next*/, 4, uintptr(unsafe.Pointer(&fes[0])), uintptr(unsafe.Pointer(&n)))
	if n != 3 || fes[0].cfFormat != desc || fes[1].cfFormat != contents || fes[2].cfFormat != drop {
		t.Errorf("форматы: %d шт., %v %v %v", n, fes[0].cfFormat, fes[1].cfFormat, fes[2].cfFormat)
	}
	if fes[1].tymed != vfTymedIStream {
		t.Errorf("FileContents перечисляется в среде %d, ждал TYMED_ISTREAM", fes[1].tymed)
	}

	var en2 uintptr
	if hr := vfInvoke(obj, vfSlotEnumFmt, 2 /*DATADIR_SET*/, uintptr(unsafe.Pointer(&en2))); hr != vfENotImpl || en2 != 0 {
		t.Errorf("EnumFormatEtc(SET) = %#x", hr)
	}
	// 4 GetDataHere, 6 GetCanonical, 7 SetData, 9 DAdvise, 10 DUnadvise, 11 EnumDAdvise.
	if hr := vfInvoke(obj, 4, 0, 0); hr != vfENotImpl {
		t.Errorf("GetDataHere = %#x", hr)
	}
	if hr := vfInvoke(obj, 6, 0, 0); hr != vfDataSameFmt {
		t.Errorf("GetCanonicalFormatEtc = %#x", hr)
	}
	if hr := vfInvoke(obj, 7, 0, 0, 0); hr != vfENotImpl {
		t.Errorf("SetData = %#x", hr)
	}
	if hr := vfInvoke(obj, 9, 0, 0, 0, 0); hr != vfAdviseNoSup {
		t.Errorf("DAdvise = %#x", hr)
	}
	if vfInvoke(obj, vfSlotGetData, 0, 0) != vfEInvalidArg {
		t.Error("GetData без аргументов должен отвечать E_INVALIDARG")
	}
}

// FILECONTENTS потоком: open вызывается при первом чтении, а не при GetData;
// читается кусками; Seek назад переоткрывает; Release закрывает читателя.
func TestVirtualCOM_ContentsStream(t *testing.T) {
	src := newVFTestSource()
	obj, _ := src.newObj(t)
	defer vfRelease(obj)
	_, contents, _ := vfFormats()

	med, hr := vfGetData(obj, vfFE(contents, 1, vfTymedIStream))
	if hr != 0 || med.tymed != vfTymedIStream || med.data == 0 {
		t.Fatalf("GetData(contents) = %#x %+v", hr, med)
	}
	stm := med.data
	if src.opens.Load() != 0 {
		t.Fatalf("open вызван при GetData (%d): содержимое должно читаться лениво", src.opens.Load())
	}

	// Stat не читает содержимого.
	var st vfStatStg
	if hr := vfInvoke(stm, 12 /*Stat*/, uintptr(unsafe.Pointer(&st)), 1); hr != 0 ||
		st.cbSize != 11 || st.typ != 2 || st.mtime != fileTime(time.Unix(1_700_000_000, 0)) {
		t.Errorf("Stat = %#x %+v", hr, st)
	}
	if src.opens.Load() != 0 {
		t.Error("Stat открыл файл")
	}
	// IStream и ISequentialStream — наши, IDataObject — нет.
	var q uintptr
	if hr := vfInvoke(stm, 0, uintptr(unsafe.Pointer(&vfIIDSeqStream)), uintptr(unsafe.Pointer(&q))); hr != 0 || q != stm {
		t.Errorf("QueryInterface(ISequentialStream) = %#x", hr)
	}
	vfInvoke(stm, vfSlotRelease)
	if hr := vfInvoke(stm, 0, uintptr(unsafe.Pointer(&vfIIDDataObject)), uintptr(unsafe.Pointer(&q))); hr != vfENoInterface {
		t.Errorf("QueryInterface(IDataObject) у потока = %#x", hr)
	}

	read := func(n int) (string, uint32, uintptr) {
		buf := make([]byte, n)
		var got uint32
		hr := vfInvoke(stm, vfSlotStreamRead, uintptr(unsafe.Pointer(&buf[0])), uintptr(n), uintptr(unsafe.Pointer(&got)))
		return string(buf[:got]), got, hr
	}
	if s, n, hr := read(4); s != "hell" || n != 4 || hr != 0 {
		t.Errorf("Read(4) = %q %d %#x", s, n, hr)
	}
	if src.opens.Load() != 1 {
		t.Errorf("open вызван %d раз после первого чтения, ждал 1", src.opens.Load())
	}
	if s, n, hr := read(4); s != "o wo" || n != 4 || hr != 0 {
		t.Errorf("Read(4) #2 = %q %d %#x", s, n, hr)
	}
	// Блок заполняется целиком, пока поток не кончился; в конце — остаток.
	if s, n, hr := read(64); s != "rld" || n != 3 || hr != 0 {
		t.Errorf("Read(64) = %q %d %#x", s, n, hr)
	}
	if s, n, hr := read(8); s != "" || n != 0 || hr != 0 {
		t.Errorf("Read на конце = %q %d %#x", s, n, hr)
	}

	// Seek: позиция, назад (переоткрытие), вперёд (пропуск), от конца.
	seek := func(move int64, origin uintptr) (uint64, uintptr) {
		var pos uint64
		hr := vfInvoke(stm, vfSlotStreamSeek, uintptr(move), origin, uintptr(unsafe.Pointer(&pos)))
		return pos, hr
	}
	if pos, hr := seek(0, 1); pos != 11 || hr != 0 {
		t.Errorf("Seek(CUR,0) = %d %#x", pos, hr)
	}
	if pos, hr := seek(0, 0); pos != 0 || hr != 0 {
		t.Errorf("Seek(SET,0) = %d %#x", pos, hr)
	}
	if s, _, _ := read(5); s != "hello" {
		t.Errorf("чтение после Seek назад: %q", s)
	}
	if src.opens.Load() != 2 || src.closes.Load() != 1 {
		t.Errorf("после Seek назад: open %d (ждал 2), close %d (ждал 1)", src.opens.Load(), src.closes.Load())
	}
	if pos, _ := seek(1, 1); pos != 6 {
		t.Errorf("Seek(CUR,1) = %d", pos)
	}
	if s, _, _ := read(5); s != "world" {
		t.Errorf("чтение после Seek вперёд: %q", s)
	}
	if src.opens.Load() != 2 {
		t.Errorf("Seek вперёд переоткрыл файл (%d open)", src.opens.Load())
	}
	if pos, hr := seek(-5, 2); pos != 6 || hr != 0 {
		t.Errorf("Seek(END,-5) = %d %#x", pos, hr)
	}
	if _, hr := seek(-1, 0); hr != vfSTGSeekError {
		t.Errorf("Seek в отрицательную позицию = %#x", hr)
	}
	if _, hr := seek(0, 9); hr != vfEInvalidArg {
		t.Errorf("Seek с неизвестным началом = %#x", hr)
	}

	// Запись запрещена, Clone не поддержан.
	if hr := vfInvoke(stm, vfSlotStreamWrit, uintptr(unsafe.Pointer(&q)), 1, 0); hr != vfSTGAccess {
		t.Errorf("Write = %#x", hr)
	}
	var cl uintptr = 5
	if hr := vfInvoke(stm, 13, uintptr(unsafe.Pointer(&cl))); hr != vfENotImpl || cl != 0 {
		t.Errorf("Clone = %#x", hr)
	}

	closes := src.closes.Load()
	if n := vfInvoke(stm, vfSlotRelease); n != 0 {
		t.Errorf("Release потока = %d", n)
	}
	if src.closes.Load() != closes+1 {
		t.Error("Release потока не закрыл читателя")
	}
}

// Пустой файл, ошибка open, ошибка чтения на полпути.
func TestVirtualCOM_StreamEdgeCases(t *testing.T) {
	src := newVFTestSource()
	obj, _ := src.newObj(t)
	defer vfRelease(obj)
	_, contents, _ := vfFormats()

	// Пустой файл: Read даёт 0 байт и S_OK.
	med, hr := vfGetData(obj, vfFE(contents, 2, vfTymedIStream))
	if hr != 0 {
		t.Fatal(hr)
	}
	var buf [4]byte
	var got uint32 = 77
	if hr := vfInvoke(med.data, vfSlotStreamRead, uintptr(unsafe.Pointer(&buf[0])), 4, uintptr(unsafe.Pointer(&got))); hr != 0 || got != 0 {
		t.Errorf("Read пустого файла = %#x, %d байт", hr, got)
	}
	vfReleaseMedium(&med)

	// open вернул ошибку: Read — STG_E_READFAULT, а не паника и не EOF.
	delete(src.body, 1)
	med, _ = vfGetData(obj, vfFE(contents, 1, vfTymedIStream))
	got = 77
	if hr := vfInvoke(med.data, vfSlotStreamRead, uintptr(unsafe.Pointer(&buf[0])), 4, uintptr(unsafe.Pointer(&got))); hr != vfSTGReadFault || got != 0 {
		t.Errorf("Read при ошибке open = %#x, %d байт", hr, got)
	}
	vfReleaseMedium(&med)

	// Читатель, отдающий по байту: блок Read всё равно заполняется целиком,
	// иначе короткое чтение приняли бы за конец файла.
	descAll, _ := buildFileGroupDescriptor(src.files)
	dd := &vfData{files: src.files, desc: descAll, open: func(int) (io.ReadCloser, error) {
		return io.NopCloser(iotest.OneByteReader(strings.NewReader("hello world"))), nil
	}}
	oo := newVFObj(vfKindData)
	oo.data = dd
	defer vfRelease(oo.ptr())
	med, _ = vfGetData(oo.ptr(), vfFE(contents, 1, vfTymedIStream))
	var eight [8]byte
	if hr := vfInvoke(med.data, vfSlotStreamRead, uintptr(unsafe.Pointer(&eight[0])), 8, uintptr(unsafe.Pointer(&got))); hr != 0 || got != 8 || string(eight[:]) != "hello wo" {
		t.Errorf("Read по байту = %#x, %d байт %q", hr, got, eight[:got])
	}
	vfReleaseMedium(&med)

	// Ошибка в середине: прочитанное отдаётся, ошибка — следующим вызовом.
	boom := errors.New("обрыв")
	desc, _ := buildFileGroupDescriptor(src.files)
	d := &vfData{files: src.files, desc: desc, open: func(int) (io.ReadCloser, error) {
		return io.NopCloser(io.MultiReader(strings.NewReader("abc"), errReader{boom})), nil
	}}
	o := newVFObj(vfKindData)
	o.data = d
	defer vfRelease(o.ptr())
	med, _ = vfGetData(o.ptr(), vfFE(contents, 1, vfTymedIStream))
	defer vfReleaseMedium(&med)
	var big [16]byte
	if hr := vfInvoke(med.data, vfSlotStreamRead, uintptr(unsafe.Pointer(&big[0])), 16, uintptr(unsafe.Pointer(&got))); hr != 0 || got != 3 || string(big[:3]) != "abc" {
		t.Errorf("Read до обрыва = %#x, %d байт %q", hr, got, big[:got])
	}
	if hr := vfInvoke(med.data, vfSlotStreamRead, uintptr(unsafe.Pointer(&big[0])), 16, uintptr(unsafe.Pointer(&got))); hr != vfSTGReadFault || got != 0 {
		t.Errorf("Read после обрыва = %#x, %d байт", hr, got)
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

// Малый файл по запросу TYMED_HGLOBAL читается целиком при GetData.
func TestVirtualCOM_ContentsHGlobal(t *testing.T) {
	src := newVFTestSource()
	obj, _ := src.newObj(t)
	defer vfRelease(obj)
	_, contents, _ := vfFormats()

	med, hr := vfGetData(obj, vfFE(contents, 1, vfTymedHGlobal))
	if hr != 0 || med.tymed != vfTymedHGlobal {
		t.Fatalf("GetData(HGLOBAL) = %#x %+v", hr, med)
	}
	if got := clipboardBlock(med.data); string(got) != "hello world" {
		t.Errorf("содержимое %q", got)
	}
	vfReleaseMedium(&med)
	if src.opens.Load() != 1 || src.closes.Load() != 1 {
		t.Errorf("open %d, close %d", src.opens.Load(), src.closes.Load())
	}
	// Ошибка open → E_FAIL, а не пустой блок.
	delete(src.body, 1)
	if _, hr := vfGetData(obj, vfFE(contents, 1, vfTymedHGlobal)); hr != vfEFail {
		t.Errorf("GetData(HGLOBAL) при ошибке open = %#x", hr)
	}
}

// CopyTo переливает содержимое в чужой поток (CreateStreamOnHGlobal).
func TestVirtualCOM_StreamCopyTo(t *testing.T) {
	src := newVFTestSource()
	obj, _ := src.newObj(t)
	defer vfRelease(obj)
	_, contents, _ := vfFormats()
	med, _ := vfGetData(obj, vfFE(contents, 1, vfTymedIStream))
	defer vfReleaseMedium(&med)

	var dest uintptr
	if hr, _, _ := ole32Clip.NewProc("CreateStreamOnHGlobal").Call(0, 1, uintptr(unsafe.Pointer(&dest))); hr != 0 || dest == 0 {
		t.Fatalf("CreateStreamOnHGlobal = %#x", hr)
	}
	defer vfInvoke(dest, vfSlotRelease)

	var rd, wr uint64
	if hr := vfInvoke(med.data, 7 /*CopyTo*/, dest, 1<<20, uintptr(unsafe.Pointer(&rd)), uintptr(unsafe.Pointer(&wr))); hr != 0 || rd != 11 || wr != 11 {
		t.Fatalf("CopyTo = %#x, прочитано %d, записано %d", hr, rd, wr)
	}
	vfInvoke(dest, vfSlotStreamSeek, 0, 0, 0)
	buf := make([]byte, 32)
	var n uint32
	vfInvoke(dest, vfSlotStreamRead, uintptr(unsafe.Pointer(&buf[0])), 32, uintptr(unsafe.Pointer(&n)))
	if string(buf[:n]) != "hello world" {
		t.Errorf("в приёмнике %q", buf[:n])
	}
}

// ─── Живой буфер обмена ──────────────────────────────────────────────────────

// vfOleThread выполняет fn на потоке ОС, закреплённом за горутиной, с OleInitialize
// — как поток приложения-вставщика (Проводника).
func vfOleThread(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan any, 1)
	go func() {
		runtime.LockOSThread()
		if hr, _, _ := procOleInitializeClip.Call(0); int32(hr) < 0 {
			done <- errors.New("OleInitialize")
			return
		}
		defer func() { done <- recover() }()
		fn()
	}()
	if p := <-done; p != nil {
		t.Fatalf("поток вставки: %v", p)
	}
}

// Настоящий буфер: кладём виртуальные файлы и читаем их через OleGetClipboard,
// как Проводник при вставке. До чтения потока open не вызывается ни разу.
func TestWinClipboard_VirtualFilesLive(t *testing.T) {
	c := needSysClipboard(t)
	src := newVFTestSource()
	released := make(chan struct{}, 2)
	if err := c.SetVirtualFiles(src.files, src.open, VirtualFilesOptions{Release: func() { released <- struct{}{} }}); err != nil {
		t.Fatal(err)
	}
	defer c.Cleanup()
	desc, contents, _ := vfFormats()

	if r, _, _ := user32Clip.NewProc("IsClipboardFormatAvailable").Call(uintptr(desc)); r == 0 {
		t.Error("FileGroupDescriptorW не объявлен в буфере")
	}
	if r, _, _ := user32Clip.NewProc("IsClipboardFormatAvailable").Call(uintptr(contents)); r == 0 {
		t.Error("FileContents не объявлен в буфере")
	}
	if got, ok := c.GetVirtualFiles(); !ok || len(got) != len(src.files) {
		t.Errorf("GetVirtualFiles = %v %v", got, ok)
	}
	if src.opens.Load() != 0 {
		t.Fatalf("open вызван при копировании (%d)", src.opens.Load())
	}

	vfOleThread(t, func() {
		var pdo uintptr
		var hr uintptr
		for i := 0; i < 50; i++ { // буфер на миг занят слушателем: Проводник тоже повторяет
			hr, _, _ = procOleGetClipboard.Call(uintptr(unsafe.Pointer(&pdo)))
			if uint32(hr) != vfClipCantOpen {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if int32(hr) < 0 || pdo == 0 {
			t.Errorf("OleGetClipboard = %#x", hr)
			return
		}
		defer vfInvoke(pdo, vfSlotRelease)

		if hr := vfQuery(pdo, vfFE(desc, -1, vfTymedHGlobal)); hr != 0 {
			t.Errorf("QueryGetData(descriptor) через OLE = %#x", hr)
		}
		med, hr := vfGetDataRetry(pdo, vfFE(desc, -1, vfTymedHGlobal))
		if hr != 0 {
			t.Errorf("GetData(descriptor) через OLE = %#x", hr)
			return
		}
		blk := clipboardBlock(med.data)
		vfReleaseMedium(&med)
		if want, _ := buildFileGroupDescriptor(src.files); !bytes.Equal(blk, want) {
			t.Errorf("дескрипторы через OLE: %d байт, ждал %d", len(blk), len(want))
		}
		if src.opens.Load() != 0 {
			t.Errorf("чтение дескрипторов позвало open (%d)", src.opens.Load())
		}

		med, hr = vfGetDataRetry(pdo, vfFE(contents, 1, vfTymedIStream))
		if hr != 0 || med.tymed != vfTymedIStream {
			t.Errorf("GetData(contents) через OLE = %#x %+v", hr, med)
			return
		}
		var out []byte
		buf := make([]byte, 5)
		for {
			var n uint32
			if hr := vfInvoke(med.data, vfSlotStreamRead, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n))); int32(hr) < 0 {
				t.Errorf("Read через OLE = %#x", hr)
				break
			}
			if n == 0 {
				break
			}
			out = append(out, buf[:n]...)
		}
		if string(out) != "hello world" {
			t.Errorf("содержимое через OLE: %q", out)
		}
		if src.opens.Load() != 1 {
			t.Errorf("open вызван %d раз при вставке одного файла, ждал 1", src.opens.Load())
		}
		vfReleaseMedium(&med)
	})
	if src.closes.Load() != 1 {
		t.Errorf("читатель закрыт %d раз, ждал 1", src.closes.Load())
	}

	// Чужое содержимое вытесняет обещание: OLE отпускает объект, release зовётся.
	vfTestSetText(t, c, "после")
	select {
	case <-released:
	case <-time.After(3 * time.Second):
		t.Fatal("после замены буфера release не вызван")
	}
	if _, ok := c.GetVirtualFiles(); ok {
		t.Error("обещание пережило замену буфера")
	}
	if _, err := c.OpenVirtualFile(1); !errors.Is(err, ErrVirtualFileNotFound) {
		t.Errorf("OpenVirtualFile после замены: %v", err)
	}
}

// Cleanup снимает обещание и зовёт release; чужое содержимое он не трогает.
func TestWinClipboard_VirtualFilesCleanup(t *testing.T) {
	c := needSysClipboard(t)
	src := newVFTestSource()
	var once sync.Once
	released := make(chan struct{})
	if err := c.SetVirtualFiles(src.files, src.open, VirtualFilesOptions{Release: func() { once.Do(func() { close(released) }) }}); err != nil {
		t.Fatal(err)
	}
	if rc, err := c.OpenVirtualFile(1); err != nil {
		t.Fatalf("OpenVirtualFile: %v", err)
	} else {
		b, _ := io.ReadAll(rc)
		rc.Close()
		if string(b) != "hello world" {
			t.Errorf("OpenVirtualFile: %q", b)
		}
	}
	c.Cleanup()
	select {
	case <-released:
	case <-time.After(6 * time.Second):
		t.Fatal("Cleanup не снял обещание")
	}
	if r, _, _ := user32Clip.NewProc("IsClipboardFormatAvailable").Call(uintptr(func() uint16 { d, _, _ := vfFormats(); return d }())); r != 0 {
		t.Error("после Cleanup формат остался в буфере")
	}

	// Чужое содержимое Cleanup не затирает.
	vfTestSetText(t, c, "чужое")
	c.Cleanup()
	if got := vfTestGetText(c); got != "чужое" {
		t.Errorf("Cleanup затёр чужое содержимое: %q", got)
	}
}

// Без open (одно Materialize) Windows отказывает и не трогает буфер.
func TestWinClipboard_VirtualFilesNeedsOpen(t *testing.T) {
	c := needSysClipboard(t)
	vfTestSetText(t, c, "до")
	err := c.SetVirtualFiles([]VirtualFile{{Name: "a"}}, nil, VirtualFilesOptions{Materialize: func(int) (string, error) { return "", nil }})
	if !errors.Is(err, ErrClipboardVirtualNoSource) {
		t.Errorf("без open: %v", err)
	}
	if got := vfTestGetText(c); got != "до" {
		t.Errorf("отказ изменил буфер: %q", got)
	}
	if err := c.SetVirtualFiles([]VirtualFile{{Name: "a:b"}}, func(int) (io.ReadCloser, error) { return nil, nil }, VirtualFilesOptions{}); !errors.Is(err, ErrVirtualFileName) {
		t.Errorf("имя с двоеточием: %v", err)
	}
}
