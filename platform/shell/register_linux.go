//go:build linux && !android

package shell

import (
	"github.com/oops1/headless-gui/v3/platform/native"
)

func init() {
	native.RegisterShell(native.Shell{
		OpenURL:     platformOpenURL,
		OpenFile:    platformOpenFile,
		RevealFile:  platformRevealFile,
		Languages:   systemLanguages,
		Theme:       detectSystemTheme,
		Accent:      detectSystemAccent,
		WatchTheme:  watchSystemTheme,
		WatchAccent: watchSystemAccent,
	})
}
