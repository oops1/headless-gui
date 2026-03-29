// griddemo — демо Grid-раскладки в нативном окне.
//
// Загружает grid_demo.xaml и показывает в окне Ebiten.
//
// Запуск (из директории GuiEngine/window):
//
//	go run ../cmd/griddemo/main.go
//
// Или:
//
//	cd GuiEngine/window
//	go run ../cmd/griddemo
package main

import (
	"log"

	"github.com/oops1/headless-gui/engine"
	"github.com/oops1/headless-gui/widget"
	"github.com/oops1/headless-gui/window"
)

func main() {
	const (
		screenW = 1024
		screenH = 768
	)

	// ─── Движок ─────────────────────────────────────────────────────────────
	eng := engine.New(screenW, screenH, 30)

	// ─── UI из XAML ─────────────────────────────────────────────────────────
	root, named, err := widget.LoadUIFromXAMLFile("../assets/ui/grid_demo.xaml")
	if err != nil {
		log.Fatalf("ошибка загрузки grid_demo.xaml: %v", err)
	}

	// Обработчики кнопок
	if btn, ok := named["btnOK"].(*widget.Button); ok {
		btn.OnClick = func() {
			log.Println("[Grid Demo] OK нажата")
		}
	}
	if btn, ok := named["btnCancel"].(*widget.Button); ok {
		btn.OnClick = func() {
			log.Println("[Grid Demo] Cancel нажата")
		}
	}

	// Фокус на поле ввода
	if input, ok := named["mainInput"].(*widget.TextInput); ok {
		eng.SetFocus(input)
	}

	eng.SetRoot(root)
	eng.Start()
	defer eng.Stop()

	// ─── Нативное окно ──────────────────────────────────────────────────────
	win := window.New(eng, "Grid Layout Demo")
	win.SetMaxFPS(60)

	if err := win.Run(); err != nil {
		log.Fatal(err)
	}
}
