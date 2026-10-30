//go:build darwin

package shell

import "github.com/oops1/headless-gui/v4/platform/native"

// wantShell — службы оболочки macOS: открытие и языки, темы пока нет
// (register_darwin.go).
func wantShell() native.Shell {
	return native.Shell{
		OpenURL:    platformOpenURL,
		OpenFile:   platformOpenFile,
		RevealFile: platformRevealFile,
		Languages:  systemLanguages,
	}
}
