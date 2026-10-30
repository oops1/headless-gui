//go:build windows

package shell

import "github.com/oops1/headless-gui/v3/platform/native"

// wantShell — службы оболочки Windows: все.
func wantShell() native.Shell {
	return native.Shell{
		OpenURL:     platformOpenURL,
		OpenFile:    platformOpenFile,
		RevealFile:  platformRevealFile,
		Languages:   systemLanguages,
		Theme:       detectSystemTheme,
		Accent:      detectSystemAccent,
		WatchTheme:  watchSystemTheme,
		WatchAccent: watchSystemAccent,
	}
}
