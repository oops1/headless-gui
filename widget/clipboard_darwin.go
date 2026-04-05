//go:build darwin

package widget

// clipboard_darwin.go — буфер обмена macOS через pbcopy/pbpaste.

import (
	"bytes"
	"os/exec"
	"strings"
)

type darwinClipboard struct{}

func init() {
	defaultClipboard = &darwinClipboard{}
}

func (c *darwinClipboard) GetText() string {
	out, err := exec.Command("pbpaste").Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

func (c *darwinClipboard) SetText(s string) {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = bytes.NewReader([]byte(s))
	cmd.Run()
}
