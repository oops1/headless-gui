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
// (навигация, решения, правка итога, отмена). Всё — через события движка, как
// это делал бы пользователь.
func shots(eng *engine.Engine, mv *widget.MergeView, showBase *widget.CheckBox, themeDD *widget.Dropdown, dir string) error {
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

	total := mv.ConflictCount()
	if err := check(total > 0, "в демо-файлах нет конфликтов"); err != nil {
		return err
	}
	if err := check(strings.Contains(mv.Result(), "<<<<<<<"), "нерешённый конфликт без маркеров"); err != nil {
		return err
	}
	if err := save("1-start.png"); err != nil {
		return err
	}

	// Навигация с клавиатуры и решение «взять наше».
	key(widget.KeyF7, 0)
	if err := check(mv.CurrentConflict() == 0, "F7: текущий конфликт %d", mv.CurrentConflict()); err != nil {
		return err
	}
	key(widget.Key1, widget.ModAlt)
	if err := check(mv.Unresolved() == total-1, "Alt+1: нерешённых %d из %d", mv.Unresolved(), total); err != nil {
		return err
	}
	if err := save("2-take-ours.png"); err != nil {
		return err
	}

	// Следующий конфликт — их сторона.
	key(widget.KeyF7, 0)
	key(widget.Key3, widget.ModAlt)
	if err := check(mv.Unresolved() == total-2 || total < 2, "Alt+3: нерешённых %d", mv.Unresolved()); err != nil {
		return err
	}

	// Правка итога руками: решения соседних блоков она не трогает.
	before := mv.Unresolved()
	mv.SetCaret(0, len("package ui"))
	typeText(" // слито")
	if err := check(strings.Contains(mv.Result(), "// слито"), "правка не попала в итог"); err != nil {
		return err
	}
	if err := check(mv.Unresolved() == before, "правка руками изменила число нерешённых"); err != nil {
		return err
	}
	if err := save("3-edited.png"); err != nil {
		return err
	}

	// Без базы и в тёмной теме.
	showBase.SetChecked(false)
	mv.SetShowBase(false)
	themeDD.SetSelected(max(0, slices.Index(widget.ThemeNames(), "Win11 Dark")))
	eng.SetTheme(widget.ThemeByName("Win11 Dark"))
	widget.SetLanguage("EN")
	eng.RenderOnce() // надписи переводятся в потоке движка (Post)
	if err := save("4-dark-no-base.png"); err != nil {
		return err
	}
	widget.SetLanguage("RU")
	showBase.SetChecked(true)
	mv.SetShowBase(true)
	eng.SetTheme(widget.Win11LightTheme())

	// Отмена до исходного состояния возвращает и правку, и решения.
	for mv.CanUndo() {
		mv.Undo()
	}
	if err := check(mv.Unresolved() == total, "после полной отмены нерешённых %d из %d", mv.Unresolved(), total); err != nil {
		return err
	}
	if err := check(!strings.Contains(mv.Result(), "// слито"), "отмена не убрала правку руками"); err != nil {
		return err
	}
	return save("5-undone.png")
}
