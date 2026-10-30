//go:build (!windows && !linux && !darwin) || android

package shell

import "github.com/oops1/headless-gui/v3/platform/native"

// wantShell — службы оболочки прочих ОС: только языки.
func wantShell() native.Shell {
	return native.Shell{Languages: systemLanguages}
}
