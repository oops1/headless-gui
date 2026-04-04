// xaml_parse.go — XAML XML-парсер и утилиты разбора.
//
// Содержит: xElement (внутреннее дерево), парсинг XML → xElement,
// парсинг цветов, Margin и вспомогательные функции.
package widget

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
)

// ─── Внутреннее дерево XAML ─────────────────────────────────────────────────

// xElement — узел XAML-дерева. Хранит имя тега, атрибуты, потомков и текст.
type xElement struct {
	Tag      string
	attrs    map[string]string
	Children []xElement
	Text     string // текст между тегами (напр. <Item>Текст</Item>)
}

// attr возвращает значение первого найденного атрибута из списка.
func (e *xElement) attr(names ...string) string {
	for _, n := range names {
		if v, ok := e.attrs[n]; ok {
			return v
		}
	}
	return ""
}

// name возвращает идентификатор элемента (Name или x:Name).
func (e *xElement) name() string {
	return e.attr("Name", "x:Name")
}

// bounds вычисляет image.Rectangle из атрибутов позиции/размера.
// Поддерживает Left/Top/Right/Bottom и Left/Top/Width/Height.
func (e *xElement) bounds() image.Rectangle {
	left := xatoi(e.attr("Left", "X", "Canvas.Left"))
	top := xatoi(e.attr("Top", "Y", "Canvas.Top"))
	right := xatoi(e.attr("Right", "Canvas.Right"))
	bottom := xatoi(e.attr("Bottom", "Canvas.Bottom"))

	if w := xatoi(e.attr("Width")); w > 0 && right == 0 {
		right = left + w
	}
	if h := xatoi(e.attr("Height")); h > 0 && bottom == 0 {
		bottom = top + h
	}
	return image.Rect(left, top, right, bottom)
}

func xatoi(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, _ := strconv.Atoi(s)
	return v
}

// xatoiOrNeg1 парсит строку как int; при пустой строке или ошибке возвращает -1.
func xatoiOrNeg1(s string) int {
	if s == "" {
		return -1
	}
	v := xatoi(s)
	if v == 0 && s != "0" {
		return -1
	}
	return v
}

// ─── Парсер XML → xElement ──────────────────────────────────────────────────

func parseXAML(data []byte) (*xElement, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, fmt.Errorf("xaml: scan: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			var root xElement
			if err := parseXAMLEl(d, start, &root); err != nil {
				return nil, err
			}
			return &root, nil
		}
	}
}

func parseXAMLEl(d *xml.Decoder, start xml.StartElement, el *xElement) error {
	el.Tag = start.Name.Local
	el.attrs = make(map[string]string, len(start.Attr))
	for _, a := range start.Attr {
		el.attrs[a.Name.Local] = a.Value
	}
	for {
		tok, err := d.Token()
		if err != nil {
			return fmt.Errorf("xaml: <%s>: %w", el.Tag, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			var child xElement
			if err := parseXAMLEl(d, t, &child); err != nil {
				return err
			}
			el.Children = append(el.Children, child)
		case xml.EndElement:
			return nil
		case xml.CharData:
			if s := strings.TrimSpace(string(t)); s != "" {
				el.Text = s
			}
		}
	}
}

// ─── Margin ────────────────────────────────────────────────────────────────

// parseMargin разбирает WPF Margin: "5", "5,10", "1,2,3,4".
func parseMargin(s string) Margin {
	s = strings.TrimSpace(s)
	if s == "" {
		return Margin{}
	}
	parts := strings.Split(s, ",")
	switch len(parts) {
	case 1:
		v := xatoi(parts[0])
		return Margin{Left: v, Top: v, Right: v, Bottom: v}
	case 2:
		h := xatoi(parts[0])
		v := xatoi(parts[1])
		return Margin{Left: h, Top: v, Right: h, Bottom: v}
	case 4:
		return Margin{
			Left:   xatoi(parts[0]),
			Top:    xatoi(parts[1]),
			Right:  xatoi(parts[2]),
			Bottom: xatoi(parts[3]),
		}
	default:
		return Margin{}
	}
}

// ─── Цвета ─────────────────────────────────────────────────────────────────

// parseXAMLColor разбирает строку цвета: "#RRGGBB", "#RRGGBBAA" или именованный цвет.
// Для hex-значений использует parseColor из loader.go.
func parseXAMLColor(s string) (color.RGBA, error) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "transparent":
		return color.RGBA{}, nil
	case "white":
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}, nil
	case "black":
		return color.RGBA{A: 255}, nil
	case "red":
		return color.RGBA{R: 255, A: 255}, nil
	case "green":
		return color.RGBA{G: 128, A: 255}, nil
	case "blue":
		return color.RGBA{B: 255, A: 255}, nil
	case "gray", "grey":
		return color.RGBA{R: 128, G: 128, B: 128, A: 255}, nil
	case "lightgray", "lightgrey":
		return color.RGBA{R: 211, G: 211, B: 211, A: 255}, nil
	case "darkgray", "darkgrey":
		return color.RGBA{R: 43, G: 43, B: 43, A: 220}, nil
	case "silver":
		return color.RGBA{R: 192, G: 192, B: 192, A: 255}, nil
	case "dodgerblue", "accent":
		return color.RGBA{R: 0, G: 120, B: 215, A: 255}, nil
	case "yellow":
		return color.RGBA{R: 255, G: 255, A: 255}, nil
	case "orange":
		return color.RGBA{R: 255, G: 165, A: 255}, nil
	case "cyan", "aqua":
		return color.RGBA{G: 255, B: 255, A: 255}, nil
	case "magenta", "fuchsia":
		return color.RGBA{R: 255, B: 255, A: 255}, nil
	case "navy":
		return color.RGBA{B: 128, A: 255}, nil
	case "teal":
		return color.RGBA{G: 128, B: 128, A: 255}, nil
	case "maroon":
		return color.RGBA{R: 128, A: 255}, nil
	case "olive":
		return color.RGBA{R: 128, G: 128, A: 255}, nil
	case "purple":
		return color.RGBA{R: 128, B: 128, A: 255}, nil
	case "lime":
		return color.RGBA{G: 255, A: 255}, nil
	case "cornflowerblue":
		return color.RGBA{R: 100, G: 149, B: 237, A: 255}, nil
	case "wheat":
		return color.RGBA{R: 245, G: 222, B: 179, A: 255}, nil
	default:
		return parseColor(s) // "#RRGGBB" / "#RRGGBBAA"
	}
}

// applyColor парсит XAML-атрибут цвета и записывает в dst.
// Если атрибут не найден или не парсится — dst не меняется.
func applyColor(dst *color.RGBA, el xElement, names ...string) {
	if s := el.attr(names...); s != "" {
		if c, err := parseXAMLColor(s); err == nil {
			*dst = c
		}
	}
}
