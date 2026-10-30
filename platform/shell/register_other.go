//go:build (!windows && !linux && !darwin) || android

package shell

import (
	"github.com/oops1/headless-gui/v4/platform/native"
)

func init() {
	// Открывать ссылки и читать тему здесь нечем: только языки из
	// переменных среды.
	native.RegisterShell(native.Shell{Languages: systemLanguages})
}
