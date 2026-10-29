// Команда facadegen перегенерирует фасад widget (ADR-0012):
//
//	go run ./internal/facadegen/cmd/facadegen
//
// Запускать из корня модуля после изменения экспортированных имён
// widget/base или подпакетов; тест фасада (widget/facade_test.go) падает,
// пока фасад не свеж.
package main

import (
	"fmt"
	"os"

	"github.com/oops1/headless-gui/v3/internal/facadegen"
)

func main() {
	if len(os.Args) > 1 {
		fmt.Fprintln(os.Stderr, "facadegen: аргументов нет; запуск из корня модуля: go run ./internal/facadegen/cmd/facadegen")
		os.Exit(2)
	}
	for _, f := range facadegen.Widget(".") {
		src, err := facadegen.Generate(f.Config)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(f.Out, src, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("записан", f.Out)
	}
}
