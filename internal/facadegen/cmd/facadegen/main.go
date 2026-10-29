// Команда facadegen перегенерирует фасады widget (ADR-0012) и window (ADR-0013):
//
//	go run ./internal/facadegen/cmd/facadegen
//
// Запускать из корня модуля после изменения экспортированных имён
// пакетов под фасадами; тесты фасадов (widget/facade_test.go,
// window/facade_test.go) падают, пока фасад не свеж.
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
	for _, f := range facadegen.All(".") {
		files, err := facadegen.Files(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		stale, err := facadegen.Stale(f, files)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for _, s := range stale {
			if err := os.Remove(s); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			fmt.Println("удалён", s)
		}
		for path, src := range files {
			if err := os.WriteFile(path, src, 0o644); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			fmt.Println("записан", path)
		}
	}
}
