package engine

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// mnemonicHost — фокусный виджет, который, как приложение, передаёт Alt+буква
// строке меню и записывает дошедшие до него клавиши.
type mnemonicHost struct {
	widget.Base
	bar  *widget.MenuBar
	keys []widget.KeyCode
}

func (h *mnemonicHost) Draw(widget.DrawContext) {}
func (h *mnemonicHost) SetFocused(bool)         {}
func (h *mnemonicHost) IsFocused() bool         { return false }
func (h *mnemonicHost) OnKeyEvent(e widget.KeyEvent) {
	if !e.Pressed {
		return
	}
	if e.Mod&widget.ModAlt != 0 && h.bar.ActivateMnemonic(e) {
		return
	}
	h.keys = append(h.keys, e.Code)
}

// После Alt+буква клавиши идут строке меню, а закрывшись, меню возвращает
// фокус прежнему виджету. Раньше ActivateMnemonic открывал меню, но фокус
// оставался у прежнего: ↓ и буква пункта уходили ему, а меню висело
// открытым, пока по нему не щёлкнут мышью.
func TestActivateMnemonic_TakesAndReturnsFocus(t *testing.T) {
	e := New(400, 300, 1)
	defer e.Stop()

	var picked string
	bar := widget.NewMenuBar()
	bar.UseMnemonics = true
	bar.AddMenu("_File", widget.MenuItem{Text: "_New"}, widget.MenuItem{Text: "_Open"})
	bar.OnSelect = func(_, _ int, text string) { picked = text }
	bar.SetBounds(image.Rect(0, 0, 400, 24))

	host := &mnemonicHost{bar: bar}
	host.SetBounds(image.Rect(0, 40, 400, 300))

	root := widget.NewCanvas()
	root.SetBounds(image.Rect(0, 0, 400, 300))
	root.AddChild(bar)
	root.AddChild(host)
	e.SetRoot(root)
	e.SetFocus(host)

	e.SendKeyEvent(widget.KeyEvent{Code: widget.KeyF, Mod: widget.ModAlt, Pressed: true})
	if got := e.focus.get(); got != bar {
		t.Fatalf("после Alt+F фокус у %T, а не у строки меню", got)
	}
	e.SendKeyEvent(widget.KeyEvent{Code: widget.KeyO, Pressed: true})
	if picked != "_Open" && picked != "Open" {
		t.Errorf("буква пункта не выбрала его: %q", picked)
	}
	if got := e.focus.get(); got != host {
		t.Fatalf("меню закрылось, а фокус у %T, а не у прежнего виджета", got)
	}

	// Escape тоже возвращает фокус, и набор дальше идёт прежнему виджету.
	e.SendKeyEvent(widget.KeyEvent{Code: widget.KeyF, Mod: widget.ModAlt, Pressed: true})
	e.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	e.SendKeyEvent(widget.KeyEvent{Code: widget.KeyX, Pressed: true})
	if got := e.focus.get(); got != host {
		t.Fatalf("после Escape фокус у %T", got)
	}
	if n := len(host.keys); n != 1 || host.keys[0] != widget.KeyX {
		t.Errorf("прежнему виджету дошло %v, ждали только X", host.keys)
	}
}
