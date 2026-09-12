// mergedemo — разрешение конфликта слияния на контроле MergeView.
//
//	mergedemo                      — демо-пример (три вкомпилированных файла)
//	mergedemo BASE OURS THEIRS     — слить три файла
//	mergedemo -shot каталог        — отрисовать проверочные кадры без окна
//
// Приложение — тонкая обвязка: панель инструментов, строка состояния и реакция
// на события контрола. Слияние, решения, правка итога и маркеры конфликта —
// внутри widget.MergeView.
package main

import (
	_ "embed"
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

//go:embed demo_base.txt
var demoBase string

//go:embed demo_ours.txt
var demoOurs string

//go:embed demo_theirs.txt
var demoTheirs string

var languages = []struct{ code, name string }{{"RU", "Русский"}, {"EN", "English"}}

func main() {
	shotDir := flag.String("shot", "", "отрисовать кадры без окна в PNG и выйти")
	out := flag.String("o", "", "записать итог в файл при Ctrl+S")
	flag.Parse()
	if n := flag.NArg(); n != 0 && n != 3 {
		fmt.Fprintln(os.Stderr, "использование: mergedemo [-shot каталог] [-o итог] [база наше их]")
		os.Exit(2)
	}
	widget.SetLanguage("RU")

	const W, H = 1280, 820
	eng := engine.New(W, H, 30)
	mono, bold := "", ""
	if *shotDir == "" {
		mono, bold = registerSystemFonts(eng)
	}
	mb := widget.NewMessageBox(eng)
	tr, trf := widget.Tr, widget.Trf

	mv := widget.NewMergeView(mono, bold)
	names := [3]string{"base", "ours", "theirs"}
	if flag.NArg() == 3 {
		texts := make([]string, 3)
		for i, p := range []string{flag.Arg(0), flag.Arg(1), flag.Arg(2)} {
			data, err := os.ReadFile(p)
			if err != nil {
				log.Fatal(err)
			}
			texts[i], names[i] = string(data), filepath.Base(p)
		}
		mv.SetTexts(texts[0], texts[1], texts[2])
	} else {
		mv.SetTexts(demoBase, demoOurs, demoTheirs)
		names = [3]string{"button.go", "button.go", "button.go"}
	}
	setSides := func() {
		mv.SetSides(
			widget.MergeSideInfo{Title: tr("app.side.ours"), Note: names[1]},
			widget.MergeSideInfo{Title: tr("app.side.base"), Note: names[0]},
			widget.MergeSideInfo{Title: tr("app.side.theirs"), Note: names[2]},
		)
	}
	setSides()

	// ─── Панель инструментов ────────────────────────────────────────────────
	bar := widget.NewStackPanel(widget.OrientationHorizontal)
	bar.Padding, bar.Spacing = 8, 8
	bar.SetBackgroundRole(widget.BackgroundPanel)
	bar.SetDock(widget.DockTop)
	bar.SetBounds(image.Rect(0, 0, W, 50))

	var texts []func()
	addBtn := func(key string, w int, fn func()) *widget.Button {
		b := widget.NewButton("")
		b.SetBounds(image.Rect(0, 0, w, 32))
		b.OnClick = fn
		bar.AddChild(b)
		if key != "" {
			// Кнопка без ключа держит свою подпись (стрелки навигации): перевод
			// пустого ключа затёр бы её.
			texts = append(texts, func() { b.SetText(tr(key)) })
		}
		return b
	}

	status := widget.NewDockPanel()
	status.SetBackgroundRole(widget.BackgroundPanel)
	status.SetDock(widget.DockBottom)
	status.SetBounds(image.Rect(0, 0, W, 28))
	left := widget.NewLabel("", widget.CurrentTheme().LabelText)
	left.PaddingX, left.PaddingY = 12, 5
	left.SetDock(widget.DockLeft)
	left.SetBounds(image.Rect(0, 0, 360, 28))
	hint := widget.NewLabel("", widget.CurrentTheme().LabelText)
	hint.Muted = true
	hint.PaddingY = 5
	status.AddChild(left)
	status.AddChild(hint)

	showState := func() {
		line, col := mv.Caret()
		left.SetText(trf("app.status", mv.Unresolved(), mv.ConflictCount(), line+1, col+1))
	}
	saveResult := func() {
		if *out == "" {
			mb.ShowInfo(tr("app.result.title"), trf("app.result.hint", mv.Unresolved()))
			return
		}
		if err := os.WriteFile(*out, []byte(mv.Result()), 0o644); err != nil {
			mb.ShowError(tr("app.err"), err.Error())
			return
		}
		hint.SetText(trf("app.saved", filepath.Base(*out)))
	}

	addBtn("app.ours", 120, func() { mv.ResolveCurrent(widget.MergeTakeOurs) })
	addBtn("app.theirs", 120, func() { mv.ResolveCurrent(widget.MergeTakeTheirs) })
	addBtn("app.both", 120, func() { mv.ResolveCurrent(widget.MergeTakeOursThenTheirs) })
	addBtn("app.undo", 110, mv.Undo)
	addBtn("", 40, mv.PrevConflict).SetText("▲")
	addBtn("", 40, mv.NextConflict).SetText("▼")
	addBtn("app.save", 130, saveResult)

	showBase := widget.NewCheckBox("")
	showBase.SetBounds(image.Rect(0, 0, 170, 32))
	showBase.SetChecked(true)
	showBase.OnChange = mv.SetShowBase
	bar.AddChild(showBase)
	texts = append(texts, func() { showBase.Text = tr("app.showBase"); showBase.Invalidate() })

	styles := []string{"merge", "diff3"}
	styleDD := widget.NewDropdown(styles...)
	styleDD.SetBounds(image.Rect(0, 0, 110, 32))
	styleDD.OnChange = func(i int, _ string) {
		if i == 1 {
			mv.SetStyle(widget.MergeStyleDiff3)
		} else {
			mv.SetStyle(widget.MergeStyleMerge)
		}
	}
	bar.AddChild(styleDD)

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
		// Заголовки панелей — тоже надписи приложения: их подписи в маркерах
		// конфликта меняются вместе с языком.
		setSides()
		showState()
		hint.SetText(tr("app.hint"))
	}
	widget.AddLanguageListener(func(string) { eng.Post(applyTexts) })

	// ─── События контрола ───────────────────────────────────────────────────
	mv.OnResolvedChanged = func(int) { showState() }
	mv.OnCaretMoved = func(int, int) { showState() }
	mv.OnResultEdited = func() { showState() }
	mv.OnSaveRequest = saveResult

	root := widget.NewDockPanel()
	root.SetBackgroundRole(widget.BackgroundWindow)
	root.AddChild(bar)
	root.AddChild(status)
	root.AddChild(mv)

	eng.SetRoot(root)
	eng.SetTheme(widget.Win11LightTheme())
	eng.SetFocus(mv)
	applyTexts()

	if *shotDir != "" {
		if err := shots(eng, mv, showBase, themeDD, *shotDir); err != nil {
			log.Fatal(err)
		}
		return
	}
	eng.Start()
	defer eng.Stop()

	win := window.New(eng, tr("app.title"))
	win.SetResizable(true)
	win.SetMaxFPS(60)
	// Несохранённый итог с нерешёнными конфликтами не теряется молча.
	win.SetOnCloseRequest(func() bool {
		if mv.Unresolved() == 0 {
			return true
		}
		mb.ShowYesNo(tr("app.close.title"), trf("app.close.msg", mv.Unresolved()), func(r widget.MessageBoxResult) {
			if r == widget.MBResultYes {
				win.Close()
			}
		})
		return false
	})
	if err := win.Run(); err != nil {
		log.Fatal(err)
	}
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
