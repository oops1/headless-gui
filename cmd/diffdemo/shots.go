package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// shots — проверочный сценарий без окна: кадры в PNG и проверки поведения
// (ввод, буфер обмена, каретка, отмена до исходного текста). Всё — через
// события движка, как это делал бы пользователь.
func shots(eng *engine.Engine, dv *widget.DiffView, hide *widget.CheckBox, themeDD, langDD *widget.Dropdown, dir string) error {
	widget.UseMemoryClipboard() // системный буфер пользователя не трогаем
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	save := func(name string) error {
		widget.StepAnimations(time.Now().Add(time.Second))
		img := eng.RenderOnce()
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		defer f.Close()
		fmt.Println("кадр:", f.Name())
		return png.Encode(f, img)
	}
	k := eng.Scale()
	phys := func(v float64) int { return int(v * k) }
	click := func(x, y float64, btn widget.MouseButton) {
		eng.SendMouseMove(phys(x), phys(y))
		eng.SendMouseButton(phys(x), phys(y), btn, true)
		eng.SendMouseButton(phys(x), phys(y), btn, false)
	}
	key := func(code widget.KeyCode, mod widget.KeyMod) {
		eng.SendKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: true})
		eng.SendKeyEvent(widget.KeyEvent{Code: code, Mod: mod})
	}
	typeText := func(s string) {
		for _, r := range s {
			eng.SendKeyEvent(widget.KeyEvent{Rune: r, Pressed: true})
		}
	}
	check := func(ok bool, format string, a ...any) error {
		if !ok {
			return fmt.Errorf("проверка: "+format, a...)
		}
		return nil
	}
	origL, origR := dv.Text(widget.DiffLeft), dv.Text(widget.DiffRight)

	if err := save("1-start.png"); err != nil {
		return err
	}

	// Навигация по изменениям с клавиатуры.
	key(widget.KeyF7, 0)
	key(widget.KeyF7, 0)
	if err := check(dv.CurrentChange() == 1, "F7 дважды: текущее изменение %d", dv.CurrentChange()); err != nil {
		return err
	}

	// Правка: конец строки 8 справа, ввод, выделение, копирование, вставка.
	dv.SetCaret(widget.DiffRight, 7, 0)
	key(widget.KeyEnd, 0)
	typeText(" // локализуемая подпись")
	if err := check(strings.Contains(dv.Text(widget.DiffRight), "LocString // локализуемая подпись"), "ввод не попал в строку 8"); err != nil {
		return err
	}
	key(widget.KeyEnter, 0)
	typeText("Tooltip  LocString")
	key(widget.KeyHome, widget.ModShift)
	key(widget.KeyC, widget.ModCtrl)
	if err := check(widget.ClipboardGetText() == "Tooltip  LocString", "копирование: %q", widget.ClipboardGetText()); err != nil {
		return err
	}
	key(widget.KeyEnd, 0)
	key(widget.KeyEnter, 0)
	key(widget.KeyV, widget.ModCtrl)
	key(widget.KeyLeft, widget.ModShift|widget.ModCtrl)
	if err := save("2-edit.png"); err != nil {
		return err
	}
	line, col := dv.Caret(widget.DiffRight)
	if err := check(line == 9 && col == 10, "каретка после вставки: %d:%d", line+1, col+1); err != nil {
		return err
	}

	hide.SetChecked(true)
	dv.SetHideUnchanged(true)
	if err := save("3-hide.png"); err != nil {
		return err
	}
	hide.SetChecked(false)
	dv.SetHideUnchanged(false)

	themeDD.SetSelected(max(0, slices.Index(widget.ThemeNames(), "Win11 Dark")))
	eng.SetTheme(widget.ThemeByName("Win11 Dark"))
	langDD.SetSelected(1)
	widget.SetLanguage("EN")
	eng.RenderOnce() // надписи переводятся в потоке движка (Post)
	if err := save("4-dark-en.png"); err != nil {
		return err
	}

	themeDD.SetSelected(max(0, slices.Index(widget.ThemeNames(), "Win2000")))
	eng.SetTheme(widget.ThemeByName("Win2000"))
	widget.SetLanguage("RU")
	langDD.SetSelected(0)
	eng.RenderOnce()
	if err := save("5-classic.png"); err != nil {
		return err
	}

	themeDD.SetSelected(max(0, slices.Index(widget.ThemeNames(), "Win11 Light")))
	eng.SetTheme(widget.Win11LightTheme())
	click(260, 330, widget.MouseRight)
	if err := save("6-menu.png"); err != nil {
		return err
	}
	key(widget.KeyEscape, 0)

	for dv.CanUndo() {
		dv.Undo()
	}
	if err := check(dv.Text(widget.DiffLeft) == origL && dv.Text(widget.DiffRight) == origR, "отмена не вернула исходный текст"); err != nil {
		return err
	}
	if err := check(!dv.IsModified(widget.DiffRight), "после полной отмены сторона помечена изменённой"); err != nil {
		return err
	}
	dv.Redo()
	return check(dv.IsModified(widget.DiffRight), "повтор не вернул правку")
}
