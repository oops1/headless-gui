// webdemo — МИНИМАЛЬНЫЙ пример webstream: UI движка в браузере.
//
//	go run ./cmd/webdemo
//	→ открыть http://localhost:8091
//
// Здесь нарочно мало виджетов — файл читается за минуту и показывает, как
// поднять стрим тремя строками. Полная витрина (те же вкладки, темы и
// локализация, что и в нативном окне) живёт в cmd/webshowcase.
//
// Приложение работает полностью headless: ни одного нативного окна,
// кадры уходят дельта-тайлами по WebSocket, ввод возвращается из браузера.
package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"net/http"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/output/webstream"
	"github.com/oops1/headless-gui/v3/widget"
)

func main() {
	eng := engine.New(900, 620, 30)

	root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 34, A: 255})
	root.SetBounds(image.Rect(0, 0, 900, 620))

	title := widget.NewLabel("headless-gui → браузер (WebSocket, дельта-тайлы)", color.RGBA{R: 220, G: 223, B: 235, A: 255})
	title.FontSize = 14
	title.SetBounds(image.Rect(24, 20, 876, 44))
	root.AddChild(title)

	hint := widget.NewLabel("Сервер не открывает ни одного окна. Кликайте, печатайте, вызывайте диалоги.", color.RGBA{R: 130, G: 135, B: 155, A: 255})
	hint.SetBounds(image.Rect(24, 48, 876, 66))
	root.AddChild(hint)

	// Счётчик кликов.
	count := 0
	counter := widget.NewLabel("Кликов: 0", color.RGBA{R: 166, G: 227, B: 161, A: 255})
	counter.SetBounds(image.Rect(200, 96, 500, 116))
	root.AddChild(counter)

	btn := widget.NewWin10AccentButton("Кликни меня")
	btn.SetBounds(image.Rect(24, 88, 180, 122))
	btn.OnClick = func() {
		count++
		counter.SetText(fmt.Sprintf("Кликов: %d", count))
	}
	root.AddChild(btn)

	// Диалоги поверх стрима.
	mb := widget.NewMessageBox(eng)
	dlgBtn := widget.NewButton("MessageBox…")
	dlgBtn.SetBounds(image.Rect(24, 132, 180, 166))
	dlgBtn.OnClick = func() {
		mb.ShowQuestion("", "Диалог отрисован сервером и доставлен тайлами. Работает?", nil)
	}
	root.AddChild(dlgBtn)

	fileBtn := widget.NewButton("Открыть файл…")
	fileBtn.SetBounds(image.Rect(190, 132, 346, 166))
	fileBtn.OnClick = func() {
		mb.ShowOpenFile(widget.FileDialogOptions{}, nil) // ФС СЕРВЕРА
	}
	root.AddChild(fileBtn)

	// Однострочное поле и многострочный редактор.
	in := widget.NewTextInput("однострочное поле…")
	in.SetBounds(image.Rect(24, 184, 430, 214))
	root.AddChild(in)

	tb := widget.NewTextBox("многострочный TextBox…")
	tb.SetBounds(image.Rect(24, 226, 620, 430))
	tb.SetText("Печатайте прямо в браузере.\n\nEnter — новая строка, колесо — скролл, выделение мышью, Ctrl+C/V, Ctrl+Z.")
	root.AddChild(tb)

	pb := widget.NewProgressBar()
	pb.SetValue(0.4)
	pb.SetBounds(image.Rect(24, 450, 430, 462))
	root.AddChild(pb)
	sl := widget.NewSliderRange(0, 100)
	sl.SetValue(40)
	sl.SetBounds(image.Rect(24, 480, 430, 504))
	sl.OnChange = func(v float64) { pb.SetValue(v / 100) }
	root.AddChild(sl)

	eng.SetRoot(root)
	eng.SetFocus(tb)
	eng.Start()

	srv := webstream.New(eng)
	go srv.Run()

	log.Println("webdemo: http://localhost:8091")
	log.Fatal(http.ListenAndServe(":8091", srv))
}
