package main

// print.go — документ для «Сохранить в PDF» и «Печать…» на вкладке «Платформа».
//
// Страницы рисует сам движок: отдельный engine.Engine с масштабом dpi/96
// отдаёт каждую страницу картинкой в разрешении листа, а пакет printing
// складывает их в PDF или отправляет на принтер. Так бумага повторяет экран:
// тот же шрифт, та же вёрстка, тот же RichText.

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/printing"
	"github.com/oops1/headless-gui/v3/widget"
)

// Цвета бумаги: белый лист, чёрный текст — не зависят от темы приложения.
var (
	paperWhite = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	paperInk   = color.RGBA{R: 20, G: 20, B: 20, A: 255}
	paperGray  = color.RGBA{R: 130, G: 130, B: 130, A: 255}
)

// printDPI — разрешение страниц. 150 точек на дюйм достаточно для чтения и
// держит страницу A4 в полутора миллионах пикселей (300 dpi — это шесть).
const printDPI = 150

// maxLogLines — сколько последних строк журнала попадает на вторую страницу.
const maxLogLines = 45

// buildPrintJob собирает задание из двух страниц A4: текст из редактора
// RichText и журнал событий. Вызывается на горутине интерфейса — виджеты
// страниц живут в отдельном движке, но тема и шрифты общие на процесс.
func buildPrintJob(doc []widget.RichParagraph, logLines []string) (printing.Job, error) {
	setup := printing.NewPageSetup(printing.A4)
	setup.Margins = printing.UniformMargins(18) // мм
	setup.DPI = printDPI
	if err := setup.Validate(); err != nil {
		return printing.Job{}, err
	}

	// Лист в физических пикселях → логические точки интерфейса: тот же
	// масштаб k отдаётся движку, поэтому A4 получается около 794×1123.
	k := float64(printDPI) / 96
	sheetW, sheetH := setup.SheetSize()
	logical := func(v int) int { return int(float64(v)/k + 0.5) }
	cr := setup.ContentRect()
	content := image.Rect(logical(cr.Min.X), logical(cr.Min.Y), logical(cr.Max.X), logical(cr.Max.Y))
	lay := pageLayout{sheet: image.Rect(0, 0, logical(sheetW), logical(sheetH)), content: content}

	eng := engine.New(logical(sheetW), logical(sheetH), 1)
	eng.SetScale(k)

	pages := []func(lay pageLayout, n, total int) widget.Widget{
		func(lay pageLayout, n, total int) widget.Widget { return pageDocument(lay, n, total, doc) },
		func(lay pageLayout, n, total int) widget.Widget { return pageLog(lay, n, total, logLines) },
	}
	job := printing.Job{Name: widget.Tr("GuiEngine showcase"), Setup: setup}
	for i, build := range pages {
		eng.SetRoot(build(lay, i+1, len(pages)))
		img := eng.RenderOnce()
		if img == nil {
			return printing.Job{}, fmt.Errorf("страница %d не отрисована", i+1)
		}
		job.Pages = append(job.Pages, img)
	}
	return job, nil
}

// pageCanvas — белый лист с заголовком, линейкой и номером страницы.
func pageCanvas(lay pageLayout, title string, n, total int) (*widget.Canvas, image.Rectangle) {
	root := widget.NewCanvas()
	root.Background = paperWhite
	root.UseAlpha = false
	root.SetBounds(lay.sheet)
	content := lay.content

	head := widget.NewLabel(title, paperInk)
	head.Bold = true
	head.FontSize = 20
	head.SetBounds(image.Rect(content.Min.X, content.Min.Y, content.Max.X, content.Min.Y+34))
	root.AddChild(head)

	rule := widget.NewLabel("", paperInk)
	rule.HasBG = true
	rule.Background = paperGray
	rule.SetBounds(image.Rect(content.Min.X, content.Min.Y+38, content.Max.X, content.Min.Y+39))
	root.AddChild(rule)

	foot := widget.NewLabel(widget.Trf("Page %d of %d · %s", n, total, time.Now().Format("02.01.2006 15:04")), paperGray)
	foot.FontSize = 9
	foot.SetBounds(image.Rect(content.Min.X, content.Max.Y-18, content.Max.X, content.Max.Y))
	root.AddChild(foot)

	body := image.Rect(content.Min.X, content.Min.Y+50, content.Max.X, content.Max.Y-28)
	return root, body
}

// pageDocument — страница 1: заголовок и форматированный текст из редактора.
func pageDocument(lay pageLayout, n, total int, doc []widget.RichParagraph) widget.Widget {
	root, body := pageCanvas(lay, widget.Tr("Document from the editor"), n, total)

	rt := widget.NewRichText()
	rt.TextColor = paperInk
	rt.Background = paperWhite
	rt.FontSize = 12
	rt.SetParagraphs(doc)
	rt.SetBounds(body)
	root.AddChild(rt)
	return root
}

// pageLog — страница 2: последние строки журнала событий моноширинным шрифтом.
func pageLog(lay pageLayout, n, total int, lines []string) widget.Widget {
	root, body := pageCanvas(lay, widget.Tr("Event log"), n, total)

	if len(lines) > maxLogLines {
		lines = lines[len(lines)-maxLogLines:]
	}
	var paras []widget.RichParagraph
	for _, l := range lines {
		paras = append(paras, widget.RichParagraph{Runs: []widget.RichRun{
			{Text: l, Font: widget.BuiltinFontMono, Size: 9},
		}})
	}
	if len(paras) == 0 {
		paras = []widget.RichParagraph{{Runs: []widget.RichRun{{Text: widget.Tr("The log is empty")}}}}
	}
	rt := widget.NewRichText()
	rt.TextColor = paperInk
	rt.Background = paperWhite
	rt.SetParagraphs(paras)
	rt.SetBounds(body)
	root.AddChild(rt)
	return root
}

// pageLayout — размеры листа и области содержимого в логических точках.
type pageLayout struct {
	sheet, content image.Rectangle
}
