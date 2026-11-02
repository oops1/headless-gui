package dialog_test

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v4/engine"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// childOfType — ребёнок диалога по имени типа: строка пути и поле ввода пути
// снаружи пакета не видны.
func childOfType(t *testing.T, fd *widget.FileDialog, typ string) widget.Widget {
	t.Helper()
	for _, c := range fd.Dialog().Children() {
		if fmt.Sprintf("%T", c) == typ {
			return c
		}
	}
	t.Fatalf("в диалоге нет %s", typ)
	return nil
}

// Ввод пути на настоящем движке: щелчок по пустому месту строки пути отдаёт
// фокус полю; набранный путь и Enter переходят, а диалог остаётся; Escape во
// время ввода отменяет ввод, не закрывая диалог; Enter без ввода сохраняет.
//
// Путь заведомо длиннее полосы, а имя текущей папки — длиннее её самой:
// головные сегменты уходят под «…», имя обрезается, а справа всё равно
// остаётся место для щелчка (crumbEditGap).
func TestFileDialog_PathEditOnEngine(t *testing.T) {
	testreset.Globals(t)
	dir := filepath.Join(t.TempDir(), "a-rather-long-folder-name-one", "a-rather-long-folder-name-two",
		strings.Repeat("very-long-folder-name-", 5))
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	eng := engine.New(1000, 700, 20)
	eng.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 30, A: 255})
	root.SetBounds(image.Rect(0, 0, 1000, 700))
	eng.SetRoot(root)
	eng.RenderOnce() // сегменты строки пути считаются при отрисовке

	var called, ok bool
	var path string
	fd := widget.NewMessageBox(eng).ShowSaveFile(widget.FileDialogOptions{StartDir: dir, InitialName: "a.txt"},
		func(p string, k bool) { called, path, ok = true, p, k })
	frame := eng.RenderOnce()

	crumb := childOfType(t, fd, "*dialog.crumbBar")
	in := childOfType(t, fd, "*dialog.pathInput").(interface {
		IsFocused() bool
		GetText() string
	})
	b := crumb.Bounds()

	// Справа от сегментов — только фон полосы: длинное имя обрезано, а не
	// нарисовано поверх места для щелчка и дальше за край.
	if frame.Bounds().Dx() != 1000 {
		t.Fatalf("кадр %v: тест считает в масштабе 1", frame.Bounds())
	}
	gap := image.Rect(b.Max.X-30, b.Min.Y+6, b.Max.X-6, b.Max.Y-6)
	bg := frame.RGBAAt(gap.Min.X, gap.Min.Y)
	for py := gap.Min.Y; py < gap.Max.Y; py++ {
		for px := gap.Min.X; px < gap.Max.X; px++ {
			if c := frame.RGBAAt(px, py); c != bg {
				t.Fatalf("в пустом месте строки пути нарисовано: (%d,%d) %v, фон %v", px, py, c, bg)
			}
		}
	}

	x, y := b.Max.X-20, b.Min.Y+b.Dy()/2
	eng.SendMouseButton(x, y, widget.MouseLeft, true)
	eng.SendMouseButton(x, y, widget.MouseLeft, false)
	if !in.IsFocused() || in.GetText() != dir {
		t.Fatalf("после щелчка: фокус у поля %v, текст %q", in.IsFocused(), in.GetText())
	}

	// Текущая папка выделена целиком: набранный путь её заменяет.
	for _, r := range sub {
		eng.SendKeyEvent(widget.KeyEvent{Rune: r, Pressed: true})
	}
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if called || fd.CurrentDir() != sub || in.IsFocused() {
		t.Fatalf("Enter: результат %v, каталог %q, фокус у поля %v", called, fd.CurrentDir(), in.IsFocused())
	}

	eng.SendMouseButton(x, y, widget.MouseLeft, true)
	eng.SendMouseButton(x, y, widget.MouseLeft, false)
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	if called || in.IsFocused() || !crumb.(interface{ IsVisible() bool }).IsVisible() {
		t.Fatalf("Escape во время ввода: результат %v, фокус у поля %v", called, in.IsFocused())
	}

	// Без ввода Enter сохраняет в выбранную папку.
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if want := filepath.Join(sub, "a.txt"); !called || !ok || path != want {
		t.Fatalf("сохранение: вызван %v, ok %v, путь %q, ожидался %q", called, ok, path, want)
	}
}
