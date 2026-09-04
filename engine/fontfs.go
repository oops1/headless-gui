// fontfs.go — шрифты из fs.FS: программа везёт их внутри себя.
//
// Папка assets/fonts ищется ОТНОСИТЕЛЬНО РАБОЧЕГО КАТАЛОГА процесса. Пока
// программа запускается из корня репозитория, этого не видно; установленная
// куда-нибудь в Program Files или запущенная службой — не находит там ничего и
// молча остаётся на встроенном Go Regular. Молча: движок пропускает
// нечитаемые файлы шрифтов без сообщения, и отсутствие всей папки для него
// такой же обычный случай.
//
// Разметку эта беда уже не касается — она умеет ехать в embed.FS
// (LoadUIFromXAMLFS). Здесь то же самое для шрифтов: каталог внутри fs.FS
// регистрируется так же, как каталог на диске, и вся поставка укладывается в
// один исполняемый файл.
package engine

import (
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
)

// RegisterFontFS регистрирует все шрифты (TTF/OTF) из каталога dir внутри
// fsys. Пустой dir означает корень fsys.
//
// Имена те же, что и у каталога на диске: файл Roboto-Regular.ttf даёт шрифты
// «Roboto-Regular» и «Roboto», файл Roboto-Bold.ttf — только «Roboto-Bold».
//
//	//go:embed assets/fonts
//	var fontsFS embed.FS
//
//	eng.RegisterFontFS(fontsFS, "assets/fonts")
//	eng.SetDefaultFont("Roboto")
//
// В отличие от RegisterFontDir возвращает ошибку. Каталог на диске может
// законно отсутствовать — это обычная поставка без своих шрифтов. Каталог
// внутри fs.FS отсутствовать не может: его имя написано в самой программе, и
// ошибка здесь означает опечатку в //go:embed, которую иначе пришлось бы
// искать по пропавшему шрифту на экране.
//
// Отдельные нечитаемые файлы пропускаются, как и у RegisterFontDir. Ошибка
// возвращается, только если не зарегистрировалось НИ ОДНОГО шрифта.
//
// Fallback-шрифты (символы ✓ ✗ ⚠, псевдографика) сюда не входят: они не
// именованные и добавляются через RegisterFallbackFont — данные для него
// читаются из того же fs.FS.
//
// Вызывать до Start().
func (e *Engine) RegisterFontFS(fsys fs.FS, dir string) error {
	if fsys == nil {
		return fmt.Errorf("RegisterFontFS: файловая система не задана")
	}
	if dir == "" {
		dir = "."
	}
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("RegisterFontFS %q: %w", dir, err)
	}

	e.frameMu.Lock()
	defer e.frameMu.Unlock()

	n := 0
	var firstErr error
	keep := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		ext := strings.ToLower(path.Ext(name))
		if ext != ".ttf" && ext != ".otf" {
			continue
		}
		data, err := readFontFS(fsys, path.Join(dir, name))
		if err != nil {
			keep(err)
			continue
		}
		if !e.registerFontBlob(strings.TrimSuffix(name, path.Ext(name)), data) {
			keep(fmt.Errorf("%s: не разбирается как шрифт", name))
			continue
		}
		n++
	}
	if n == 0 {
		if firstErr != nil {
			return fmt.Errorf("RegisterFontFS %q: ни одного шрифта не зарегистрировано: %w", dir, firstErr)
		}
		return fmt.Errorf("RegisterFontFS %q: нет файлов .ttf или .otf", dir)
	}
	return nil
}

// registerFontBlob регистрирует шрифт под именем файла и, если это Regular, —
// ещё и под именем семейства. Сообщает, разобрались ли данные как шрифт.
//
// Псевдоним семейства не перебивает уже занятое имя: каталог мог принести
// своё «Roboto» после того, как приложение зарегистрировало собственное.
func (e *Engine) registerFontBlob(stem string, data []byte) bool {
	e.canvas.RegisterFont(stem, data)
	if !e.canvas.hasFont(stem) {
		return false
	}
	if fam := fontFamilyAlias(stem); fam != "" && fam != stem && !e.canvas.hasFont(fam) {
		e.canvas.RegisterFont(fam, data)
	}
	return true
}

// readFontFS читает файл шрифта из fs.FS с той же границей размера, что и с
// диска: «шрифт» на десятки гигабайт не должен превращаться в OOM при старте.
func readFontFS(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > maxFontFileSize {
		return nil, fmt.Errorf("%s: файл шрифта слишком большой (%d байт > %d)", name, st.Size(), maxFontFileSize)
	}
	// Читаем на байт больше границы: усечённый шрифт бесполезен, и отличить
	// «ровно по границе» от «больше» иначе нечем.
	data, err := io.ReadAll(io.LimitReader(f, maxFontFileSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxFontFileSize {
		return nil, fmt.Errorf("%s: файл шрифта слишком большой (> %d байт)", name, maxFontFileSize)
	}
	return data, nil
}
