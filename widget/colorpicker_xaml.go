package widget

import (
	"image/color"
	"strconv"
	"strings"
)

// colorpicker_xaml.go — теги <ColorPicker> и <Swatch> в разметке.
//
//	<ColorPicker x:Name="accent" Value="#0078D7" FontSize="10"
//	             Palette="#000000,#FFFFFF,#0078D7"
//	             ValueChangedCommand="{Binding Accent}"/>
//	<Swatch Color="#C42B1C" Width="18" Height="18" CornerRadius="3"/>
//
// Цвет — тем же синтаксисом, что и остальные цвета разметки (parseXAMLColor):
// «#RRGGBB», «#AARRGGBB» и имена. Своя палитра перечисляется через запятую:
// цвета редактора темы у каждого приложения свои, а набор по умолчанию —
// общий.

func buildXAMLColorPicker(el xElement) Widget {
	p := NewColorPicker()
	if v := el.attr("Value", "Color", "SelectedColor"); v != "" {
		if c, err := parseXAMLColor(v); err == nil {
			p.SetValue(c)
		}
	}
	if v := el.attr("Palette", "Colors"); v != "" {
		if cols := parseXAMLColorList(v); len(cols) > 0 {
			p.SetPalette(cols)
		}
	}
	if fs := el.attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			p.FontSize = v
		}
	}
	return p
}

func buildXAMLSwatch(el xElement) Widget {
	s := NewSwatch(color.RGBA{A: 255})
	if v := el.attr("Color", "Value", "Fill", "Background"); v != "" {
		if c, err := parseXAMLColor(v); err == nil {
			s.Color = c
		}
	}
	if v := el.attr("BorderBrush", "Border"); v != "" {
		if c, err := parseXAMLColor(v); err == nil {
			s.Border = c
		}
	}
	if v := el.attr("CornerRadius"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			s.CornerRadius = n
		}
	}
	return s
}

// parseXAMLColorList разбирает список цветов через запятую. Нераспознанные
// пропускаются молча — как и всюду в разметке: одна опечатка не должна
// оставлять контрол вовсе без палитры.
func parseXAMLColorList(s string) []color.RGBA {
	parts := strings.Split(s, ",")
	out := make([]color.RGBA, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if c, err := parseXAMLColor(part); err == nil {
			out = append(out, c)
		}
	}
	return out
}
