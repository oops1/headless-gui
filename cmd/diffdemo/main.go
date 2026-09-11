// diffdemo — сравнение и правка двух файлов на контроле DiffView.
//
//	diffdemo                  — демо-пример
//	diffdemo A B              — сравнить файлы A и B
//	diffdemo -shot каталог    — отрисовать проверочные кадры без окна и выйти
//
// Приложение — тонкая обвязка: панель инструментов, строка состояния и
// реакция на события контрола. Сравнение, правка, слежение за файлами,
// подсветка и перенос блоков — внутри widget.DiffView.
package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/window"
)

//go:embed demo_left.txt
var demoLeft string

//go:embed demo_right.txt
var demoRight string

var languages = []struct{ code, name string }{{"RU", "Русский"}, {"EN", "English"}}

func main() {
	shotDir := flag.String("shot", "", "отрисовать кадры без окна в PNG и выйти")
	shotScale := flag.Float64("scale", 1, "масштаб HiDPI для -shot")
	flag.Parse()
	if n := flag.NArg(); n != 0 && n != 2 {
		fmt.Fprintln(os.Stderr, "использование: diffdemo [-shot каталог] [левый-файл правый-файл]")
		os.Exit(2)
	}
	widget.SetLanguage("RU")

	const W, H = 1440, 900
	eng := engine.New(W, H, 30)
	// Кадры -shot сравнивают между машинами: системные шрифты там не нужны.
	mono, bold := "", ""
	if *shotDir == "" {
		mono, bold = registerSystemFonts(eng)
	}
	mb := widget.NewMessageBox(eng)
	tr, trf := widget.Tr, widget.Trf

	dv := widget.NewDiffView(mono, bold)
	if flag.NArg() == 2 {
		for i, p := range []string{flag.Arg(0), flag.Arg(1)} {
			if err := dv.LoadFile(widget.DiffSide(i), p); err != nil {
				log.Fatal(err)
			}
		}
		dv.SetSyntaxHighlight(isCode(flag.Arg(0)) || isCode(flag.Arg(1)))
	} else {
		dv.SetText(widget.DiffLeft, "main", "config.go", demoLeft)
		dv.SetText(widget.DiffRight, "feature/localization", "config.go", demoRight)
	}

	// ─── Панель инструментов ────────────────────────────────────────────────
	bar := widget.NewStackPanel(widget.OrientationHorizontal)
	bar.Padding, bar.Spacing = 8, 8
	bar.SetBackgroundRole(widget.BackgroundPanel)
	bar.SetDock(widget.DockTop)
	bar.SetBounds(image.Rect(0, 0, W, 50))

	var texts []func() // перевод надписей при смене языка
	addBtn := func(key, tipKey string, w int, fn func()) *widget.Button {
		b := widget.NewButton("")
		b.SetBounds(image.Rect(0, 0, w, 32))
		b.OnClick = fn
		bar.AddChild(b)
		texts = append(texts, func() {
			if key != "" {
				b.SetText(tr(key))
			}
			if tipKey != "" {
				b.SetToolTip(tr(tipKey))
			}
		})
		return b
	}
	addCheck := func(key string, w int, fn func(bool)) *widget.CheckBox {
		c := widget.NewCheckBox("")
		c.SetBounds(image.Rect(0, 0, w, 32))
		c.OnChange = fn
		bar.AddChild(c)
		texts = append(texts, func() { c.Text = tr(key); c.Invalidate() })
		return c
	}

	status := widget.NewDockPanel()
	status.SetBackgroundRole(widget.BackgroundPanel)
	status.SetDock(widget.DockBottom)
	status.SetBounds(image.Rect(0, 0, W, 28))
	posLabel := widget.NewLabel("", widget.CurrentTheme().LabelText)
	posLabel.PaddingX, posLabel.PaddingY = 12, 5
	posLabel.SetDock(widget.DockLeft)
	posLabel.SetBounds(image.Rect(0, 0, 280, 28))
	msgLabel := widget.NewLabel("", widget.CurrentTheme().LabelText)
	msgLabel.Muted = true
	msgLabel.PaddingY = 5
	status.AddChild(posLabel)
	status.AddChild(msgLabel)
	msg := ""
	setMsg := func(s string) {
		msg = s
		if s == "" {
			s = tr("app.hint")
		}
		msgLabel.SetText(s)
	}
	sideName := func(s widget.DiffSide) string {
		if s == widget.DiffRight {
			return tr("app.side.right")
		}
		return tr("app.side.left")
	}
	showPos := func() {
		side := dv.ActiveSide()
		line, col := dv.Caret(side)
		posLabel.SetText(trf("app.status.pos", sideName(side), line+1, col+1))
	}

	pick := func(side widget.DiffSide) {
		key := "app.pick.left"
		if side == widget.DiffRight {
			key = "app.pick.right"
		}
		mb.ShowOpenFile(widget.FileDialogOptions{Title: tr(key)}, func(path string, ok bool) {
			if ok {
				dv.LoadFile(side, path)
			}
		})
	}
	var saveSide func(side widget.DiffSide, then func())
	saveSide = func(side widget.DiffSide, then func()) {
		err := dv.Save(side)
		if errors.Is(err, widget.ErrDiffNoPath) {
			key := "app.saveAs.left"
			if side == widget.DiffRight {
				key = "app.saveAs.right"
			}
			mb.ShowSaveFile(widget.FileDialogOptions{Title: tr(key), InitialName: "config.go"}, func(path string, ok bool) {
				if ok && dv.SaveAs(side, path) == nil && then != nil {
					then()
				}
			})
			return
		}
		if err == nil && then != nil {
			then()
		}
	}
	saveAll := func() {
		var todo []widget.DiffSide
		for _, s := range []widget.DiffSide{widget.DiffLeft, widget.DiffRight} {
			if dv.IsModified(s) {
				todo = append(todo, s)
			}
		}
		switch len(todo) {
		case 0:
			setMsg(tr("app.nothing"))
		case 1:
			saveSide(todo[0], nil)
		default:
			saveSide(todo[0], func() { saveSide(todo[1], nil) })
		}
	}

	addBtn("app.open.left", "", 104, func() { pick(widget.DiffLeft) })
	addBtn("app.open.right", "", 108, func() { pick(widget.DiffRight) })
	addBtn("app.save", "app.save.tip", 110, saveAll)
	// Надписи, а не стрелки «↩ ↪»: этих знаков нет во встроенных шрифтах, и
	// на машине без подходящего системного шрифта кнопки остались бы пустыми.
	undoBtn := addBtn("app.undo", "app.undo.tip", 96, dv.Undo)
	redoBtn := addBtn("app.redo", "app.redo.tip", 96, dv.Redo)
	addBtn("", "app.prev.tip", 40, dv.PrevChange).SetText("▲")
	addBtn("", "app.next.tip", 40, dv.NextChange).SetText("▼")
	hideCheck := addCheck("app.hide", 180, dv.SetHideUnchanged)
	addCheck("app.ignoreWS", 170, dv.SetIgnoreWhitespace)

	count := widget.NewLabel("", widget.CurrentTheme().LabelText)
	count.PaddingY = 7
	count.SetBounds(image.Rect(0, 0, 120, 32))
	bar.AddChild(count)
	showCount := func() { count.SetText(pluralChanges(dv.ChangeCount())) }
	texts = append(texts, showCount, showPos, func() { setMsg(msg) })

	themes := widget.ThemeNames()
	themeDD := widget.NewDropdown(themes...)
	themeDD.SetBounds(image.Rect(0, 0, 150, 32))
	themeDD.SetSelected(max(0, slices.Index(themes, "Win11 Light")))
	themeDD.OnChange = func(_ int, name string) {
		if t := widget.ThemeByName(name); t != nil {
			eng.SetTheme(t)
		}
	}
	bar.AddChild(themeDD)

	var langNames []string
	for _, l := range languages {
		langNames = append(langNames, l.name)
	}
	langDD := widget.NewDropdown(langNames...)
	langDD.SetBounds(image.Rect(0, 0, 110, 32))
	langDD.OnChange = func(i int, _ string) { widget.SetLanguage(languages[i].code) }
	bar.AddChild(langDD)

	applyTexts := func() {
		for _, f := range texts {
			f()
		}
	}
	widget.AddLanguageListener(func(string) { eng.Post(applyTexts) })

	// ─── События контрола ───────────────────────────────────────────────────
	updHistory := func() {
		undoBtn.SetEnabled(dv.CanUndo())
		redoBtn.SetEnabled(dv.CanRedo())
	}
	dv.OnDiffChanged = func(int) { showCount() }
	dv.OnTextChanged = func(widget.DiffSide) { updHistory() }
	dv.OnCaretMoved = func(widget.DiffSide, int, int) { showPos() }
	dv.OnActiveSideChanged = func(widget.DiffSide) { showPos() }
	dv.OnSaveRequest = func(side widget.DiffSide) { saveSide(side, nil) }
	dv.OnFileSaved = func(_ widget.DiffSide, path string) { setMsg(trf("app.saved", filepath.Base(path))) }
	dv.OnFileLoaded = func(widget.DiffSide, string) {
		updHistory()
		setMsg("")
		dv.SetSyntaxHighlight(isCode(dv.FilePath(widget.DiffLeft)) || isCode(dv.FilePath(widget.DiffRight)))
	}
	dv.OnError = func(_ widget.DiffSide, err error) { mb.ShowError(tr("app.err"), err.Error()) }
	// Приходит в потоке движка — контрол сам отправляет его через Post, так
	// что диалог можно показывать прямо отсюда.
	dv.OnFileChangedOnDisk = func(side widget.DiffSide, path string, deleted bool) {
		name := filepath.Base(path)
		switch {
		case deleted:
			setMsg(trf("app.disk.deleted", name))
		case !dv.IsModified(side):
			if dv.Reload(side) == nil {
				setMsg(trf("app.reloaded", name))
			}
		default:
			mb.ShowYesNo(tr("app.disk.title"), trf("app.disk.msg", name), func(r widget.MessageBoxResult) {
				if r == widget.MBResultYes && dv.Reload(side) == nil {
					setMsg(trf("app.reloaded", name))
				}
			})
		}
	}
	dv.SetWatchFiles(true)
	defer dv.Close()

	root := widget.NewDockPanel()
	root.SetBackgroundRole(widget.BackgroundWindow)
	root.AddChild(bar)
	root.AddChild(status)
	root.AddChild(dv)

	eng.SetRoot(root)
	eng.SetTheme(widget.Win11LightTheme())
	eng.SetFocus(dv)
	applyTexts()
	updHistory()

	if *shotDir != "" {
		eng.SetScale(*shotScale)
		if err := shots(eng, dv, hideCheck, themeDD, langDD, *shotDir); err != nil {
			log.Fatal(err)
		}
		return
	}
	eng.Start()
	defer eng.Stop()

	win := window.New(eng, tr("app.title"))
	win.SetResizable(true)
	win.SetMaxFPS(60)
	if err := win.Run(); err != nil {
		log.Fatal(err)
	}
}

func pluralChanges(n int) string {
	if n == 0 {
		return "  " + widget.Tr("app.changes.0")
	}
	key := "app.changes.5"
	switch {
	case n%100 >= 11 && n%100 <= 14:
	case n%10 == 1:
		key = "app.changes.1"
	case n%10 >= 2 && n%10 <= 4:
		key = "app.changes.2"
	}
	return "  " + widget.Trf(key, n)
}

// registerSystemFonts берёт Consolas и Segoe UI, если они есть. Без них
// контрол рисует встроенными Go-шрифтами — ничего регистрировать не нужно.
func registerSystemFonts(eng *engine.Engine) (mono, bold string) {
	dir := os.Getenv("WINDIR")
	if dir == "" {
		return "", ""
	}
	fonts := filepath.Join(dir, "Fonts")
	if eng.RegisterFontFile("Consolas", filepath.Join(fonts, "consola.ttf")) == nil {
		mono = "Consolas"
	}
	if eng.RegisterFontFile("SegoeUI", filepath.Join(fonts, "segoeui.ttf")) == nil {
		eng.SetDefaultFont("SegoeUI")
	}
	if eng.RegisterFontFile("SegoeUIBold", filepath.Join(fonts, "segoeuib.ttf")) == nil {
		bold = "SegoeUIBold"
	}
	return mono, bold
}
