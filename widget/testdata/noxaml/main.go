// Программа без XAML для widget/noxaml_test.go: строит виджеты кодом.
package main

import (
	"fmt"

	"github.com/oops1/headless-gui/v3/widget"
)

func main() {
	p := widget.NewPanel(widget.CurrentTheme().WindowBG)
	b := widget.NewButton("OK")
	p.AddChild(b)
	fmt.Println(len(p.Children()), b.Text)
}
