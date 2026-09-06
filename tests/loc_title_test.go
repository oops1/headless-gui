package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// {Loc} в заголовке — запрос GG-48.
//
// Живой перевод разметки знает фиксированный список свойств, и `title` в нём не
// было: `<DockPane Title="{Loc ...}">` переводился один раз при разборе XAML и
// оставался на языке загрузки. Четыре док-панели после переключения языка
// стояли с прежними заголовками.

const locTitleXAML = `<Window x:Name="win" Title="{Loc app.title}" Width="800" Height="600">
  <DockManager x:Name="dm" Left="0" Top="0" Width="800" Height="600">
    <DockPane x:Name="files" Id="files" Title="{Loc pane.files}" Side="Left" Size="200"/>
    <DockPane x:Name="log" Id="log" Title="{Loc pane.log}" Side="Bottom" Size="120"/>
    <DockContent><TextBox Text="—"/></DockContent>
  </DockManager>
</Window>`

func TestLocTitle_FollowsLanguageSwitch(t *testing.T) {
	widget.ClearLocalizedItems()
	t.Cleanup(widget.ClearLocalizedItems)

	widget.RegisterStrings("ru", map[string]string{
		"app.title":  "Го.Гит",
		"pane.files": "Файлы",
		"pane.log":   "Журнал",
	})
	widget.RegisterStrings("en", map[string]string{
		"app.title":  "Go.Git",
		"pane.files": "Files",
		"pane.log":   "Log",
	})

	widget.SetLanguage("ru")
	root, reg, err := widget.LoadUIFromXAML([]byte(locTitleXAML))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	_ = root

	files, ok := reg["files"].(*widget.DockPane)
	if !ok {
		t.Fatalf("DockPane собрался как %T", reg["files"])
	}
	logPane := reg["log"].(*widget.DockPane)
	win, ok := reg["win"].(*widget.Window)
	if !ok {
		t.Fatalf("Window собрался как %T", reg["win"])
	}

	if files.Title != "Файлы" || logPane.Title != "Журнал" {
		t.Fatalf("заголовки при загрузке: %q, %q", files.Title, logPane.Title)
	}
	if win.Title != "Го.Гит" {
		t.Fatalf("заголовок окна при загрузке: %q", win.Title)
	}

	widget.SetLanguage("en")
	if files.Title != "Files" || logPane.Title != "Log" {
		t.Errorf("после смены языка заголовки панелей %q, %q", files.Title, logPane.Title)
	}
	if win.Title != "Go.Git" {
		t.Errorf("после смены языка заголовок окна %q", win.Title)
	}

	widget.SetLanguage("ru")
	if files.Title != "Файлы" {
		t.Errorf("возврат к русскому дал %q", files.Title)
	}
}

// Смена заголовка перерисовывает панель и перекладывает стопку: ширина корешка
// вкладки считается по подписи, и без перекладки полоса вкладок разъезжается.
func TestLocTitle_RedrawsAfterChange(t *testing.T) {
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 24, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 600, 400))

	m := widget.NewDockManager()
	m.SetBounds(image.Rect(0, 0, 600, 400))
	a := widget.NewDockPane("a", "Ф", nil)
	b := widget.NewDockPane("b", "Ж", nil)
	m.AddPane(a, widget.DockLeft)
	m.AddPane(b, widget.DockLeft)
	m.SetBounds(image.Rect(0, 0, 600, 400))
	root.AddChild(m)

	eng := engine.New(600, 400, 30)
	eng.SetRoot(root)
	eng.RenderOnce()
	before := snapshotRGBA(eng.RenderOnce())

	a.SetTitle("Очень длинное название панели")
	eng.Invalidate()
	after := snapshotRGBA(eng.RenderOnce())

	diff := 0
	for i := range before.Pix {
		if before.Pix[i] != after.Pix[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Error("смена заголовка панели ничего не изменила на экране")
	}
}

// Повторная установка того же заголовка ничего не делает — иначе смена языка
// без изменений перекладывала бы весь док.
func TestLocTitle_SameTitleIsNoOp(t *testing.T) {
	p := widget.NewDockPane("id", "Файлы", nil)
	p.SetTitle("Файлы")
	if p.Title != "Файлы" {
		t.Errorf("заголовок стал %q", p.Title)
	}
}
