package widget

import (
	"encoding/binary"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func TestNormalizeVirtualFiles(t *testing.T) {
	ok, err := NormalizeVirtualFiles([]VirtualFile{
		{Name: "Отчёт/", IsDir: true, Size: 99},
		{Name: "Отчёт/итог.txt", Size: 5},
		{Name: "a b.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok[0].Name != "Отчёт" || ok[0].Size != 0 || ok[1].Name != "Отчёт/итог.txt" {
		t.Errorf("приведение имён: %+v", ok)
	}

	bad := map[string][]VirtualFile{
		"пусто":          nil,
		"пустое имя":     {{Name: ""}},
		"только слэш":    {{Name: "/"}},
		"абсолютный":     {{Name: "/etc/passwd"}},
		"родитель":       {{Name: "a/../b"}},
		"точка":          {{Name: "./a"}},
		"двойной слэш":   {{Name: "a//b"}},
		"обратный слэш":  {{Name: `a\b`}},
		"NUL":            {{Name: "a\x00b"}},
		"перевод строки": {{Name: "a\nb"}},
		"не UTF-8":       {{Name: "a\xffb"}},
		"отрицательный":  {{Name: "a", Size: -1}},
		"повтор":         {{Name: "a"}, {Name: "a/"}},
		"файл-каталог":   {{Name: "a"}, {Name: "a/b"}},
	}
	for name, files := range bad {
		if _, err := NormalizeVirtualFiles(files); err == nil {
			t.Errorf("%s: принято %v", name, files)
		} else if name != "пусто" && !errors.Is(err, ErrVirtualFileName) {
			t.Errorf("%s: ошибка не ErrVirtualFileName: %v", name, err)
		}
	}
	if _, err := NormalizeVirtualFiles(nil); !errors.Is(err, ErrClipboardVirtualEmpty) {
		t.Errorf("пустой список: %v", err)
	}
	// Вход не изменяется.
	in := []VirtualFile{{Name: "d/", IsDir: true, Size: 3}}
	NormalizeVirtualFiles(in)
	if in[0].Name != "d/" || in[0].Size != 3 {
		t.Errorf("вход изменён: %+v", in)
	}
}

func TestVirtualTopLevel(t *testing.T) {
	files := []VirtualFile{
		{Name: "a.txt"}, {Name: "d", IsDir: true}, {Name: "d/x"}, {Name: "d/e", IsDir: true}, {Name: "d/e/y"},
	}
	top, err := VirtualTopLevel(files)
	if err != nil || !reflect.DeepEqual(top, []int{0, 1}) {
		t.Errorf("верхний уровень: %v %v", top, err)
	}
	if _, err := VirtualTopLevel([]VirtualFile{{Name: "d/x"}}); !errors.Is(err, ErrVirtualFileName) {
		t.Errorf("каталог не объявлен: %v", err)
	}
}

func TestFileTime(t *testing.T) {
	if got := fileTime(time.Unix(0, 0)); got != 116444736000000000 {
		t.Errorf("эпоха Unix = %d", got)
	}
	if got := fileTime(time.Unix(1, 500)); got != 116444736000000000+10_000_000+5 {
		t.Errorf("секунда и 500 нс = %d", got)
	}
	if fileTime(time.Time{}) != 0 || fileTime(time.Date(1500, 1, 1, 0, 0, 0, 0, time.UTC)) != 0 {
		t.Error("нулевое и допотопное время должны давать 0")
	}
}

// FILEGROUPDESCRIPTORW байт в байт: cItems, флаги, атрибуты, время записи,
// размер двумя словами и имя UTF-16 с обратными косыми чертами.
func TestBuildFileGroupDescriptor(t *testing.T) {
	mod := time.Unix(1_700_000_000, 0)
	files := []VirtualFile{
		{Name: "Отчёт", IsDir: true, ModTime: mod},
		{Name: "Отчёт/итог.txt", Size: 0x1_0000_0007, ModTime: mod},
		{Name: "без времени.bin", Size: 3},
	}
	b, err := buildFileGroupDescriptor(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 4+3*fileDescriptorSize {
		t.Fatalf("размер блока %d", len(b))
	}
	if binary.LittleEndian.Uint32(b) != 3 {
		t.Errorf("cItems = %d", binary.LittleEndian.Uint32(b))
	}
	ent := func(i int) []byte { return b[4+i*fileDescriptorSize : 4+(i+1)*fileDescriptorSize] }
	u32 := func(e []byte, off int) uint32 { return binary.LittleEndian.Uint32(e[off:]) }
	name := func(e []byte) string {
		var u []uint16
		for j := 72; j+1 < len(e); j += 2 {
			v := binary.LittleEndian.Uint16(e[j:])
			if v == 0 {
				break
			}
			u = append(u, v)
		}
		return string(utf16.Decode(u))
	}

	// Каталог: атрибуты и время, размера нет.
	e := ent(0)
	if f := u32(e, 0); f != fdAttributes|fdWritesTime|fdProgressUI {
		t.Errorf("флаги каталога %#x", f)
	}
	if u32(e, 36) != fileAttributeDirectory || name(e) != "Отчёт" {
		t.Errorf("каталог: атрибуты %#x имя %q", u32(e, 36), name(e))
	}
	// Файл: размер, время, обратная черта.
	e = ent(1)
	if f := u32(e, 0); f != fdAttributes|fdWritesTime|fdFileSize|fdProgressUI {
		t.Errorf("флаги файла %#x", f)
	}
	if u32(e, 64) != 1 || u32(e, 68) != 7 {
		t.Errorf("размер: старшее %d младшее %d", u32(e, 64), u32(e, 68))
	}
	ft := fileTime(mod)
	if u32(e, 56) != uint32(ft) || u32(e, 60) != uint32(ft>>32) {
		t.Errorf("время записи: %d %d, ждал %d", u32(e, 56), u32(e, 60), ft)
	}
	if u32(e, 36) != fileAttributeNormal || name(e) != `Отчёт\итог.txt` {
		t.Errorf("файл: атрибуты %#x имя %q", u32(e, 36), name(e))
	}
	// Без времени флага WRITESTIME нет.
	e = ent(2)
	if f := u32(e, 0); f != fdAttributes|fdFileSize|fdProgressUI || u32(e, 56)|u32(e, 60) != 0 {
		t.Errorf("файл без времени: флаги %#x", f)
	}

	// Слишком длинное имя и запрещённые символы.
	for _, n := range []string{strings.Repeat("я", 260), "a:b", "a?b", "a|b", `a"b`} {
		if _, err := buildFileGroupDescriptor([]VirtualFile{{Name: n}}); !errors.Is(err, ErrVirtualFileName) {
			t.Errorf("имя %.12q принято: %v", n, err)
		}
	}
	if _, err := buildFileGroupDescriptor([]VirtualFile{{Name: strings.Repeat("я", 259)}}); err != nil {
		t.Errorf("имя в 259 символов: %v", err)
	}
}

// Буфер в памяти: обещание, вытеснение, колбэк снятия.
func TestClipboardVirtualFiles_Memory(t *testing.T) {
	prev := GetClipboardProvider()
	defer SetClipboardProvider(prev)
	UseMemoryClipboard()

	files := []VirtualFile{{Name: "a.txt", Size: 3}, {Name: "d", IsDir: true}}
	opened := 0
	open := func(i int) (io.ReadCloser, error) {
		opened++
		return io.NopCloser(strings.NewReader("abc")), nil
	}
	released := make(chan struct{}, 4)
	if err := ClipboardSetVirtualFiles(files, open, WithRelease(func() { released <- struct{}{} })); err != nil {
		t.Fatal(err)
	}
	if opened != 0 {
		t.Fatal("open вызван при копировании: файлы должны быть отложенными")
	}
	got, ok := ClipboardVirtualFiles()
	if !ok || !reflect.DeepEqual(got, files) {
		t.Errorf("ClipboardVirtualFiles = %v %v", got, ok)
	}
	if ClipboardGetText() != "a.txt\nd" {
		t.Errorf("текст-имена: %q", ClipboardGetText())
	}
	if !ClipboardHasFiles() {
		t.Errorf("обещанные файлы не видны ClipboardHasFiles: %v", ClipboardFormats())
	}
	rc, err := ClipboardOpenVirtualFile(0)
	if err != nil || opened != 1 {
		t.Fatalf("вставка: %v, open вызван %d раз", err, opened)
	}
	if b, _ := io.ReadAll(rc); string(b) != "abc" {
		t.Errorf("содержимое %q", b)
	}
	for _, i := range []int{1, 2, -1} { // каталог и выход за список
		if _, err := ClipboardOpenVirtualFile(i); !errors.Is(err, ErrVirtualFileNotFound) {
			t.Errorf("индекс %d: %v", i, err)
		}
	}
	select {
	case <-released:
		t.Fatal("обещание снято раньше времени")
	default:
	}

	ClipboardSetText("x") // чужое содержимое вытесняет обещание
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("после замены буфера WithRelease не вызван")
	}
	if _, ok := ClipboardVirtualFiles(); ok {
		t.Error("обещание пережило замену буфера")
	}
	if _, err := ClipboardOpenVirtualFile(0); !errors.Is(err, ErrVirtualFileNotFound) {
		t.Errorf("вставка после замены: %v", err)
	}

	// Cleanup снимает обещание и зовёт колбэк ровно один раз.
	ClipboardSetVirtualFiles(files, open, WithRelease(func() { released <- struct{}{} }))
	ClipboardCleanup()
	ClipboardCleanup()
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("ClipboardCleanup: WithRelease не вызван")
	}
	select {
	case <-released:
		t.Error("WithRelease вызван дважды")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestClipboardSetVirtualFiles_Errors(t *testing.T) {
	prev := GetClipboardProvider()
	defer SetClipboardProvider(prev)

	open := func(int) (io.ReadCloser, error) { return nil, nil }
	files := []VirtualFile{{Name: "a"}}

	// Провайдер приложения без расширения.
	to := &textOnlyClipboard{s: "до"}
	SetClipboardProvider(to)
	err := ClipboardSetVirtualFiles(files, open)
	if !errors.Is(err, ErrClipboardVirtualUnsupported) || !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("провайдер без расширения: %v", err)
	}
	if to.s != "до" {
		t.Error("отказ изменил буфер")
	}
	if _, ok := ClipboardVirtualFiles(); ok {
		t.Error("провайдер без расширения вернул файлы")
	}
	if _, err := ClipboardOpenVirtualFile(0); !errors.Is(err, ErrClipboardVirtualUnsupported) {
		t.Errorf("ClipboardOpenVirtualFile: %v", err)
	}
	ClipboardCleanup() // не должен паниковать

	UseMemoryClipboard()
	if err := ClipboardSetVirtualFiles(nil, open); !errors.Is(err, ErrClipboardVirtualEmpty) {
		t.Errorf("пустой список: %v", err)
	}
	if err := ClipboardSetVirtualFiles(files, nil); !errors.Is(err, ErrClipboardVirtualEmpty) {
		t.Errorf("нет ни open, ни Materialize: %v", err)
	}
	if err := ClipboardSetVirtualFiles([]VirtualFile{{Name: "../x"}}, open); !errors.Is(err, ErrVirtualFileName) {
		t.Errorf("плохое имя: %v", err)
	}
	// Только Materialize принимается (Linux), open в таком обещании нет.
	err = ClipboardSetVirtualFiles(files, nil, WithMaterialize(func(int) (string, error) { return "/x", nil }))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ClipboardOpenVirtualFile(0); !errors.Is(err, ErrVirtualFileNotFound) {
		t.Errorf("open не задан: %v", err)
	}
}
