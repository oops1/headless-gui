package base

// clipboard_virtual.go — «виртуальные» файлы в буфере обмена: имена, размеры и
// время есть, содержимого нет, оно запрашивается у приложения, когда кто-то
// вставляет.
//
// ClipboardSetFiles принимает только готовые пути: файлы приходится сначала
// создать на диске. Клиенту удалённого рабочего стола это не годится — файлы
// сервера должны скачиваться при вставке на этом компьютере, а не каждый раз
// при копировании внутри сеанса. Здесь буфер получает «обещание»: список
// VirtualFile и колбэки приложения, которые зовут только при вставке.
//
// Движок держит только МЕХАНИЗМ: он говорит системе «файлы есть» и зовёт
// колбэк приложения в момент вставки. Политики в нём нет — ни временных папок,
// ни скачивания, ни очистки: откуда брать байты, куда их складывать и когда
// убирать, решает приложение.
//
// На проводе:
//   - Windows: IDataObject с CFSTR_FILEDESCRIPTORW и CFSTR_FILECONTENTS
//     (TYMED_ISTREAM, поток читает из open по требованию), OleSetClipboard;
//     Проводник просит содержимое файла при вставке (platform/clipboard/clipboard_virtual_windows.go);
//   - X11 и Wayland: стандартного протокола «виртуальных файлов» нет, а
//     вставляющему нужны настоящие пути (text/uri-list,
//     x-special/gnome-copied-files). Поэтому в ответ на запрос этих форматов
//     владелец зовёт WithMaterialize приложения и отдаёт пути, которые оно
//     вернуло (platform/native/clipboard_virtual.go). Без WithMaterialize этих форматов
//     на Linux нет, и ClipboardSetVirtualFiles отвечает отказом;
//   - macOS и провайдеры приложения без расширения: ErrClipboardVirtualUnsupported.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// VirtualFile — файл, обещанный буферу обмена без содержимого.
type VirtualFile struct {
	// Name — относительный путь с «/» в качестве разделителя («Отчёт/итог.txt»).
	// Каталоги вложенных файлов на Windows можно не перечислять — они
	// подразумеваются, но пустой каталог обязан быть в списке (IsDir). На X11 и
	// Wayland каталог верхнего уровня должен быть в списке всегда.
	Name string
	// Size — размер содержимого в байтах (у каталога игнорируется).
	Size int64
	// ModTime — время изменения; нулевое — «неизвестно».
	ModTime time.Time
	// IsDir — каталог: содержимого у него нет, open для него не зовётся.
	IsDir bool
}

var (
	// ErrClipboardVirtualUnsupported возвращает ClipboardSetVirtualFiles, когда
	// текущий буфер виртуальных файлов не умеет (macOS, провайдер приложения без
	// расширения, Linux без окна). Совпадает с errors.ErrUnsupported.
	ErrClipboardVirtualUnsupported = fmt.Errorf("widget: буфер обмена не поддерживает виртуальные файлы: %w", errors.ErrUnsupported)
	// ErrClipboardVirtualEmpty — список пуст или не задан ни open, ни
	// WithMaterialize: класть в буфер нечего.
	ErrClipboardVirtualEmpty = errors.New("widget: пустой список виртуальных файлов или нет ни open, ни WithMaterialize")
	// ErrClipboardVirtualNoSource — платформе нужна функция, которой приложение
	// не дало: на Windows это open, на X11 и Wayland — WithMaterialize. Буфер
	// при этом не меняется. Совпадает с errors.ErrUnsupported.
	ErrClipboardVirtualNoSource = fmt.Errorf("widget: для этой платформы не задан источник содержимого виртуальных файлов (Windows — open, X11/Wayland — WithMaterialize): %w", errors.ErrUnsupported)
	// ErrVirtualFileName — недопустимое имя виртуального файла или список, где
	// имена противоречат друг другу.
	ErrVirtualFileName = errors.New("widget: недопустимое имя виртуального файла")
	// ErrVirtualFileNotFound — обещания в буфере нет или индекс вне списка.
	ErrVirtualFileNotFound = errors.New("widget: нет такого виртуального файла в буфере")
)

// maxVirtualFiles — предел на число записей: список приходит извне (с
// удалённого рабочего стола), а дескрипторы Windows занимают по 592 байта.
const maxVirtualFiles = 1 << 16

// NormalizeVirtualFiles проверяет список и возвращает его копию с
// приведёнными именами (без завершающего «/»).
//
// Имя — относительный путь из непустых компонентов, без «.», «..», обратной
// косой черты, нулевого и управляющих символов, в корректном UTF-8. Одно имя
// дважды и «файл, он же каталог» (имя файла как префикс пути другого) — отказ:
// приёмник создал бы либо перезапись, либо несуществующий каталог. Размер не
// отрицателен. Ошибка оборачивает ErrVirtualFileName.
func NormalizeVirtualFiles(files []VirtualFile) ([]VirtualFile, error) {
	if len(files) == 0 {
		return nil, ErrClipboardVirtualEmpty
	}
	if len(files) > maxVirtualFiles {
		return nil, fmt.Errorf("%w: записей больше %d", ErrVirtualFileName, maxVirtualFiles)
	}
	out := make([]VirtualFile, len(files))
	kind := make(map[string]bool, len(files)) // имя → это каталог
	for i, f := range files {
		name, err := cleanVirtualName(f.Name)
		if err != nil {
			return nil, fmt.Errorf("%w: запись %d (%q): %v", ErrVirtualFileName, i, f.Name, err)
		}
		if f.Size < 0 {
			return nil, fmt.Errorf("%w: запись %d (%q): отрицательный размер", ErrVirtualFileName, i, f.Name)
		}
		if _, dup := kind[name]; dup {
			return nil, fmt.Errorf("%w: запись %d: имя %q повторено", ErrVirtualFileName, i, name)
		}
		kind[name] = f.IsDir
		if f.IsDir {
			f.Size = 0
		}
		f.Name = name
		out[i] = f
	}
	// Предок записи, объявленный файлом, — противоречие.
	for name := range kind {
		for j := strings.LastIndexByte(name, '/'); j > 0; j = strings.LastIndexByte(name[:j], '/') {
			if isDir, ok := kind[name[:j]]; ok && !isDir {
				return nil, fmt.Errorf("%w: %q лежит внутри файла %q", ErrVirtualFileName, name, name[:j])
			}
		}
	}
	return out, nil
}

// cleanVirtualName проверяет один относительный путь и убирает завершающий «/».
func cleanVirtualName(name string) (string, error) {
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		return "", errors.New("пустое имя")
	}
	if !utf8.ValidString(name) {
		return "", errors.New("не UTF-8")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f || r == '\\' {
			return "", errors.New("недопустимый символ")
		}
	}
	for _, part := range strings.Split(name, "/") {
		switch part {
		case "":
			return "", errors.New("пустой компонент пути")
		case ".", "..":
			return "", errors.New("компонент «.» или «..»")
		}
	}
	return name, nil
}

// VirtualTopLevel возвращает индексы записей верхнего уровня (имя без «/»).
// Ошибка (ErrVirtualFileName), если вложенная запись лежит в каталоге верхнего
// уровня, которого нет в списке явной записью IsDir. Нужна X11 и Wayland: пути
// просят только у записей верхнего уровня.
func VirtualTopLevel(files []VirtualFile) ([]int, error) {
	declared := make(map[string]bool, len(files))
	for _, f := range files {
		if f.IsDir {
			declared[f.Name] = true
		}
	}
	var top []int
	for i, f := range files {
		j := strings.IndexByte(f.Name, '/')
		if j < 0 {
			top = append(top, i)
			continue
		}
		if !declared[f.Name[:j]] {
			return nil, fmt.Errorf("%w: каталог верхнего уровня %q не объявлен записью IsDir", ErrVirtualFileName, f.Name[:j])
		}
	}
	return top, nil
}

// VirtualFilesOptions — необязательные колбэки приложения для
// ClipboardSetVirtualFiles; заполняются VirtualFilesOption.
type VirtualFilesOptions struct {
	// Materialize — «положи файл на диск и верни путь»: нужен X11 и Wayland.
	Materialize func(index int) (path string, err error)
	// Release — обещание снято (буфер заменён, окно закрыто, ClipboardCleanup).
	Release func()
}

// VirtualFilesOption настраивает ClipboardSetVirtualFiles.
type VirtualFilesOption func(*VirtualFilesOptions)

// WithMaterialize задаёт колбэк X11 и Wayland: вставляющему нужен настоящий
// путь, и движок просит его у приложения в момент запроса text/uri-list или
// x-special/gnome-copied-files.
//
// fn(index) зовётся для каждой записи верхнего уровня списка (имя без «/»):
// для файла приложение кладёт его на диск и возвращает абсолютный путь, для
// каталога — каталог со всем содержимым (вложенные записи сами в колбэк не
// попадают). Каталог верхнего уровня обязан быть в списке явной записью IsDir —
// иначе спросить не о чем, и ClipboardSetVirtualFiles отвечает
// ErrVirtualFileName. Вызов задерживает ответ владельца выделения, вставляющий
// ждёт; зовётся не на горутине движка, последовательно для одного обещания.
// Результат запоминается до снятия обещания: второй запрос (uri-list после
// gnome-copied-files) повторно не зовёт. Ошибка не запоминается и значит
// отказ вставляющему. На Windows игнорируется.
func WithMaterialize(fn func(index int) (path string, err error)) VirtualFilesOption {
	return func(o *VirtualFilesOptions) { o.Materialize = fn }
}

// WithRelease задаёт колбэк «обещание снято»: буфер заменили другим
// содержимым, закрыли окно или вызвали ClipboardCleanup. Здесь приложение
// убирает то, что положило на диск. Зовётся один раз, в отдельной горутине,
// из него нельзя ждать другие вызовы буфера обмена.
func WithRelease(fn func()) VirtualFilesOption {
	return func(o *VirtualFilesOptions) { o.Release = fn }
}

// ClipboardVirtualFilesProvider — необязательное расширение ClipboardProvider:
// буфер, умеющий виртуальные файлы.
type ClipboardVirtualFilesProvider interface {
	// SetVirtualFiles кладёт в буфер обещанные файлы, заменяя его содержимое.
	// files уже проверен NormalizeVirtualFiles. open может быть nil (тогда
	// платформе нужен opts.Materialize).
	SetVirtualFiles(files []VirtualFile, open func(index int) (io.ReadCloser, error), opts VirtualFilesOptions) error
	// GetVirtualFiles возвращает обещанные нами файлы, пока они в буфере;
	// ok == false, если буфер заменён.
	GetVirtualFiles() (files []VirtualFile, ok bool)
	// OpenVirtualFile открывает содержимое нашего обещанного файла — так же,
	// как это сделал бы вставляющий.
	OpenVirtualFile(index int) (io.ReadCloser, error)
}

// ClipboardSetVirtualFiles кладёт в системный буфер обмена «обещанные» файлы:
// имена, размеры и время изменения — без содержимого. Содержимое запрашивается
// у приложения только тогда, когда файлы вставляют.
//
// Windows: open(index) вернёт читателя файла files[index] (для каталогов не
// зовётся); Проводник вставляет файлы как обычные, а содержимое читает потоком
// IStream, поэтому open вызывается при первом чтении, и читатель закрывается
// после него. Чтение идёт на отдельном потоке ОС, обслуживающем буфер, куском
// за раз; пока кусок не прочитан, замена буфера из другого потока ждёт.
// WithMaterialize игнорируется.
//
// X11 и Wayland: стандартного протокола «виртуальных файлов» нет, поэтому в
// ответ на запрос text/uri-list или x-special/gnome-copied-files владелец
// зовёт WithMaterialize и отдаёт вернувшиеся пути. Ограничение: такой запрос
// иногда делают и менеджеры буфера обмена просто при просмотре — тогда
// колбэк вызовется без вставки; часть программ (Qt — 5 секунд) ждёт ответ
// владельца ограниченное время. Простым текстом ложатся только относительные
// имена. open на этих платформах нужен лишь ClipboardOpenVirtualFile.
// Движок ничего не создаёт и не удаляет на диске.
//
// Колбэки зовутся не на горутине движка. Буфер заменяется так же, как в
// ClipboardSetText; когда его заменят (или вызовут ClipboardCleanup),
// обещание снимается и зовётся WithRelease.
//
// Ошибки: ErrClipboardVirtualUnsupported (macOS, провайдер без расширения),
// ErrClipboardVirtualEmpty (пустой список, нет ни open, ни WithMaterialize),
// ErrClipboardVirtualNoSource (платформе не хватает своей функции; буфер не
// меняется), ErrVirtualFileName (недопустимые имена).
func ClipboardSetVirtualFiles(files []VirtualFile, open func(index int) (io.ReadCloser, error), opts ...VirtualFilesOption) error {
	norm, err := NormalizeVirtualFiles(files)
	if err != nil {
		return err
	}
	var o VirtualFilesOptions
	for _, f := range opts {
		if f != nil {
			f(&o)
		}
	}
	if open == nil && o.Materialize == nil {
		return ErrClipboardVirtualEmpty
	}
	if p, ok := defaultClipboard.(ClipboardVirtualFilesProvider); ok {
		return p.SetVirtualFiles(norm, open, o)
	}
	return ErrClipboardVirtualUnsupported
}

// ClipboardVirtualFiles возвращает файлы, обещанные ClipboardSetVirtualFiles,
// пока они в буфере. ok == false, если буфер уже заменён или провайдер
// виртуальных файлов не умеет.
func ClipboardVirtualFiles() (files []VirtualFile, ok bool) {
	if p, isV := defaultClipboard.(ClipboardVirtualFilesProvider); isV {
		return p.GetVirtualFiles()
	}
	return nil, false
}

// ClipboardOpenVirtualFile открывает содержимое файла index из нашего же
// обещания — как вставка внутри программы. Ошибка, если обещания в буфере нет,
// индекс вне списка или open не задан.
func ClipboardOpenVirtualFile(index int) (io.ReadCloser, error) {
	if p, ok := defaultClipboard.(ClipboardVirtualFilesProvider); ok {
		return p.OpenVirtualFile(index)
	}
	return nil, ErrClipboardVirtualUnsupported
}

// ClipboardCleanup снимает обещание виртуальных файлов, если оно ещё в буфере
// (зовётся WithRelease). Звать при выходе из программы. Провайдер без такой
// возможности игнорируется.
func ClipboardCleanup() {
	if p, ok := defaultClipboard.(interface{ Cleanup() }); ok {
		p.Cleanup()
	}
}

// memVirtual — обещание виртуальных файлов в буфере в памяти.
type memVirtual struct {
	files   []VirtualFile
	open    func(int) (io.ReadCloser, error)
	release func()
}

// drop сообщает приложению, что обещание снято (один раз, в своей горутине).
func (v *memVirtual) drop() {
	if v != nil && v.release != nil {
		go v.release()
	}
}

// SetVirtualFiles — реализация ClipboardVirtualFilesProvider в памяти.
func (c *memoryClipboard) SetVirtualFiles(files []VirtualFile, open func(int) (io.ReadCloser, error), opts VirtualFilesOptions) error {
	norm, err := NormalizeVirtualFiles(files)
	if err != nil {
		return err
	}
	if open == nil && opts.Materialize == nil {
		return ErrClipboardVirtualEmpty
	}
	names := make([]string, len(norm))
	for i, f := range norm {
		names[i] = f.Name
	}
	c.mu.Lock()
	old := c.virt
	c.text, c.html = strings.Join(names, "\n"), ""
	c.files, c.cut, c.img = nil, false, nil
	c.virt = &memVirtual{files: norm, open: open, release: opts.Release}
	c.mu.Unlock()
	old.drop()
	return nil
}

// GetVirtualFiles — реализация ClipboardVirtualFilesProvider в памяти.
func (c *memoryClipboard) GetVirtualFiles() ([]VirtualFile, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.virt == nil {
		return nil, false
	}
	return append([]VirtualFile(nil), c.virt.files...), true
}

// OpenVirtualFile — реализация ClipboardVirtualFilesProvider в памяти.
func (c *memoryClipboard) OpenVirtualFile(index int) (io.ReadCloser, error) {
	c.mu.Lock()
	v := c.virt
	c.mu.Unlock()
	if v == nil || index < 0 || index >= len(v.files) || v.files[index].IsDir || v.open == nil {
		return nil, ErrVirtualFileNotFound
	}
	return v.open(index)
}

// Cleanup снимает обещание в памяти.
func (c *memoryClipboard) Cleanup() {
	c.mu.Lock()
	old := c.virt
	if old != nil {
		c.virt, c.text = nil, ""
	}
	c.mu.Unlock()
	old.drop()
}

// ─── FILEGROUPDESCRIPTORW (Windows) ─────────────────────────────────────────
//
// Сборка блока — без системных вызовов, чтобы её проверял общий прогон тестов.
// FILEGROUPDESCRIPTORW: UINT cItems и cItems записей FILEDESCRIPTORW по 592
// байта: флаги, CLSID, SIZEL, POINTL, атрибуты, три FILETIME, размер (старшее и
// младшее слово) и имя из 260 WCHAR.

const (
	fileDescriptorSize = 592
	fileDescNameChars  = 260

	fdAttributes = 0x00000004 // FD_ATTRIBUTES
	fdWritesTime = 0x00000020 // FD_WRITESTIME
	fdFileSize   = 0x00000040 // FD_FILESIZE
	fdProgressUI = 0x00004000 // FD_PROGRESSUI

	fileAttributeDirectory = 0x10
	fileAttributeNormal    = 0x80
)

// fileTimeEpochDiff — секунд между 1601-01-01 (начало FILETIME) и 1970-01-01.
const fileTimeEpochDiff = 11644473600

// FileTime переводит время в FILETIME (100-нс интервалы с 1601 года); время
// до 1601 года и нулевое дают 0.
//
// Служебная: для подпакетов widget и platform/clipboard, в фасад не входит.
//
//facadegen:skip
func FileTime(t time.Time) uint64 {
	if t.IsZero() {
		return 0
	}
	sec := t.Unix() + fileTimeEpochDiff
	if sec < 0 {
		return 0
	}
	return uint64(sec)*10_000_000 + uint64(t.Nanosecond()/100)
}

// BuildFileGroupDescriptor собирает FILEGROUPDESCRIPTORW. Имена — с обратной
// косой чертой; недопустимые в Windows символы и пути длиннее MAX_PATH-1
// (259 единиц UTF-16) — ErrVirtualFileName: Проводник отказал бы во всей
// вставке.
//
// Служебная: для подпакетов widget и platform/clipboard, в фасад не входит.
//
//facadegen:skip
func BuildFileGroupDescriptor(files []VirtualFile) ([]byte, error) {
	b := make([]byte, 4+len(files)*fileDescriptorSize)
	putU32(b[0:], uint32(len(files)))
	for i, f := range files {
		if strings.ContainsAny(f.Name, `<>:"|?*`) {
			return nil, fmt.Errorf("%w: %q содержит символ, недопустимый в Windows", ErrVirtualFileName, f.Name)
		}
		name := utf16Encode(strings.ReplaceAll(f.Name, "/", `\`))
		if len(name) >= fileDescNameChars {
			return nil, fmt.Errorf("%w: %q длиннее %d символов", ErrVirtualFileName, f.Name, fileDescNameChars-1)
		}
		e := b[4+i*fileDescriptorSize:]
		flags := uint32(fdAttributes | fdProgressUI)
		attr := uint32(fileAttributeNormal)
		if f.IsDir {
			attr = fileAttributeDirectory
		} else {
			flags |= fdFileSize
			putU32(e[64:], uint32(uint64(f.Size)>>32))
			putU32(e[68:], uint32(f.Size))
		}
		if ft := FileTime(f.ModTime); ft != 0 {
			flags |= fdWritesTime
			putU32(e[56:], uint32(ft))
			putU32(e[60:], uint32(ft>>32))
		}
		putU32(e[0:], flags)
		putU32(e[36:], attr)
		for j, u := range name {
			e[72+j*2] = byte(u)
			e[72+j*2+1] = byte(u >> 8)
		}
	}
	return b, nil
}

func putU32(b []byte, v uint32) { binary.LittleEndian.PutUint32(b, v) }

func utf16Encode(s string) []uint16 { return utf16.Encode([]rune(s)) }
