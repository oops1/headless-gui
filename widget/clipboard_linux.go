//go:build linux

package widget

// clipboard_linux.go — буфер обмена Linux через xclip/xsel.
//
// Без CGO: используем внешние утилиты xclip или xsel.
// Fallback: in-memory буфер если утилиты недоступны.

import (
	"bytes"
	"os/exec"
	"strings"
)

type linuxClipboard struct {
	tool string // "xclip", "xsel" или "" (fallback).
	mem  string // fallback in-memory.
}

func init() {
	cb := &linuxClipboard{}
	// Определяем доступный инструмент.
	if path, err := exec.LookPath("xclip"); err == nil && path != "" {
		cb.tool = "xclip"
	} else if path, err := exec.LookPath("xsel"); err == nil && path != "" {
		cb.tool = "xsel"
	}
	defaultClipboard = cb
}

func (c *linuxClipboard) GetText() string {
	switch c.tool {
	case "xclip":
		out, err := exec.Command("xclip", "-selection", "clipboard", "-o").Output()
		if err != nil {
			return c.mem
		}
		return strings.TrimRight(string(out), "\n")
	case "xsel":
		out, err := exec.Command("xsel", "--clipboard", "--output").Output()
		if err != nil {
			return c.mem
		}
		return strings.TrimRight(string(out), "\n")
	default:
		return c.mem
	}
}

func (c *linuxClipboard) SetText(s string) {
	c.mem = s
	switch c.tool {
	case "xclip":
		cmd := exec.Command("xclip", "-selection", "clipboard", "-i")
		cmd.Stdin = bytes.NewReader([]byte(s))
		cmd.Run()
	case "xsel":
		cmd := exec.Command("xsel", "--clipboard", "--input")
		cmd.Stdin = bytes.NewReader([]byte(s))
		cmd.Run()
	}
}
