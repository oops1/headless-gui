//go:build linux

package widget

// clipboard_linux.go — запасной буфер обмена Linux через xclip/xsel.
//
// Основной путь — нативный: под Wayland это wl_data_device
// (window/wayland_clipboard_linux.go), и окно подменяет провайдер сразу,
// как только соединение поднялось. Эти утилиты остаются для X11-сессий без
// нативного владельца и как запас там, где окна ещё нет.
//
// Текст НЕ подрезается: скопированная строка с завершающим переводом строки
// должна вставиться ровно такой же. Прежний TrimRight съедал его и портил
// вставку кода и списков.

import (
	"bytes"
	"os/exec"
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
		return string(out)
	case "xsel":
		out, err := exec.Command("xsel", "--clipboard", "--output").Output()
		if err != nil {
			return c.mem
		}
		return string(out)
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
