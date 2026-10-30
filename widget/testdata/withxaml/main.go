// Контрольная программа для widget/noxaml_test.go: грузит XAML.
package main

import (
	"fmt"

	"github.com/oops1/headless-gui/v4/widget"
)

func main() {
	root, _, err := widget.LoadUIFromXAML([]byte(`<StackPanel><Button Content="OK"/></StackPanel>`))
	fmt.Println(root != nil, err)
}
