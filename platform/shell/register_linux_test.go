//go:build linux && !android

package shell

import "github.com/oops1/headless-gui/v4/platform/native"

// wantShell — службы оболочки Linux: все.
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
