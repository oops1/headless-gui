package widget

// loader.go — загрузчик UI из JSON-конфигурации (с 8.3 — в фасаде, ADR-0012:
// он строит виджеты всех пакетов, поэтому живёт выше них).
//
// Формат JSON:
//
//	{
//	  "type": "panel",        // panel | label | button | textinput | dropdown | progressbar
//	  "id": "myWidget",      // опциональный идентификатор для поиска в map
//	  "bounds": [x1,y1,x2,y2], // прямоугольник в абсолютных координатах
//	  "style": "win10",      // для panel: win10 | transparent | solid
//	                          // для button: accent | normal
//	  "text": "Hello",       // для label, button
//	  "placeholder": "...",  // для textinput
//	  "textColor": "#RRGGBB", // hex-цвет текста
//	  "bgColor":   "#RRGGBBAA", // hex-цвет фона (с альфой)
//	  "items": ["A","B"],    // для dropdown
//	  "focused": true,        // для textinput
//	  "children": [...]       // вложенные виджеты
//	}

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"strconv"
	"strings"

	"github.com/oops1/headless-gui/v3/widget/base"
	"github.com/oops1/headless-gui/v3/widget/text"
)

// UINode описывает один виджет в JSON-конфигурации.
type UINode struct {
	Type        string            `json:"type"`
	ID          string            `json:"id,omitempty"`
	Bounds      [4]int            `json:"bounds"`             // [x1, y1, x2, y2]
	Style       string            `json:"style,omitempty"`
	Text        string            `json:"text,omitempty"`
	Placeholder string            `json:"placeholder,omitempty"`
	TextColor   string            `json:"textColor,omitempty"` // "#RRGGBB" или "#RRGGBBAA"
	BgColor     string            `json:"bgColor,omitempty"`
	Items       []string          `json:"items,omitempty"`
	Focused     bool              `json:"focused,omitempty"`
	Selected    int               `json:"selected,omitempty"`
	Children    []UINode          `json:"children,omitempty"`
	Props       map[string]string `json:"props,omitempty"`
}

// LoadUIFromFile читает JSON-файл и строит дерево виджетов.
// Возвращает корневой виджет и map[id]Widget для именованных виджетов.
func LoadUIFromFile(path string) (base.Widget, map[string]base.Widget, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("loader: read %q: %w", path, err)
	}
	return LoadUIFromJSON(data)
}

// LoadUIFromJSON разбирает JSON и строит дерево виджетов.
func LoadUIFromJSON(data []byte) (base.Widget, map[string]base.Widget, error) {
	var root UINode
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("loader: parse JSON: %w", err)
	}
	registry := make(map[string]base.Widget)
	w, err := buildWidget(root, registry)
	if err != nil {
		return nil, nil, err
	}
	return w, registry, nil
}

// buildWidget рекурсивно строит виджет из UINode.
func buildWidget(n UINode, reg map[string]base.Widget) (base.Widget, error) {
	var w base.Widget
	var err error

	switch strings.ToLower(n.Type) {
	case "panel":
		w = buildPanel(n)
	case "label":
		w = buildLabel(n)
	case "button":
		w = buildButton(n)
	case "textinput", "input":
		w = buildTextInput(n)
	case "dropdown":
		w = buildDropdown(n)
	case "progressbar", "progress":
		w = buildProgressBar(n)
	case "checkbox":
		w = buildCheckBoxJSON(n)
	case "radiobutton", "radio":
		w = buildRadioButtonJSON(n)
	case "slider":
		w = buildSliderJSON(n)
	case "toggleswitch", "toggle":
		w = buildToggleSwitchJSON(n)
	case "scrollview":
		w = buildScrollViewJSON(n)
	case "listview":
		w = buildListViewJSON(n)
	case "tabcontrol", "tabs":
		w = buildTabControlJSON(n)
	default:
		return nil, fmt.Errorf("loader: unknown widget type %q", n.Type)
	}

	// Bounds: [x1, y1, x2, y2]
	w.SetBounds(image.Rect(n.Bounds[0], n.Bounds[1], n.Bounds[2], n.Bounds[3]))

	// Регистрация по ID
	if n.ID != "" {
		reg[n.ID] = w
	}

	// Дочерние виджеты
	for _, childNode := range n.Children {
		child, err2 := buildWidget(childNode, reg)
		if err2 != nil {
			return nil, err2
		}
		w.AddChild(child)
	}

	return w, err
}

// ─── Построители виджетов ────────────────────────────────────────────────────

func buildPanel(n UINode) base.Widget {
	switch strings.ToLower(n.Style) {
	case "win10":
		p := base.NewWin10Panel()
		if n.BgColor != "" {
			if c, err := base.ParseRGBAHex(n.BgColor); err == nil {
				p.Background = c
			}
		}
		return p
	case "transparent":
		p := base.NewPanel(color.RGBA{}) // A=0: FillRect пропустит рисование
		p.UseAlpha = true           // Over-режим — фон не затирается
		return p
	default:
		bg := color.RGBA{R: 43, G: 43, B: 43, A: 220}
		if n.BgColor != "" {
			if c, err := base.ParseRGBAHex(n.BgColor); err == nil {
				bg = c
			}
		}
		return base.NewPanel(bg)
	}
}

func buildLabel(n UINode) base.Widget {
	var lbl *base.Label
	if n.TextColor != "" {
		if c, err := base.ParseRGBAHex(n.TextColor); err == nil {
			lbl = base.NewLabel(n.Text, c)
		}
	}
	if lbl == nil {
		lbl = base.NewWin10Label(n.Text)
	}
	if n.BgColor != "" {
		if c, err := base.ParseRGBAHex(n.BgColor); err == nil {
			lbl.HasBG = true
			lbl.Background = c
		}
	}
	return lbl
}

func buildButton(n UINode) base.Widget {
	text := n.Text
	if text == "" {
		text = n.ID
	}
	switch strings.ToLower(n.Style) {
	case "accent":
		return base.NewWin10AccentButton(text)
	default:
		return base.NewButton(text)
	}
}

func buildTextInput(n UINode) base.Widget {
	ph := n.Placeholder
	if ph == "" {
		ph = n.Text
	}
	ti := text.NewTextInput(ph)
	if n.Text != "" && n.Placeholder == "" {
		ti.SetText(n.Text)
	}
	if n.Focused {
		ti.SetFocused(true)
	}
	if n.TextColor != "" {
		if c, err := base.ParseRGBAHex(n.TextColor); err == nil {
			ti.TextColor = c
		}
	}
	if n.BgColor != "" {
		if c, err := base.ParseRGBAHex(n.BgColor); err == nil {
			ti.Background = c
		}
	}
	return ti
}

func buildDropdown(n UINode) base.Widget {
	dd := base.NewDropdown(n.Items...)
	if n.Selected >= 0 && n.Selected < len(n.Items) {
		dd.SetSelected(n.Selected)
	}
	return dd
}

func buildProgressBar(n UINode) base.Widget {
	pb := base.NewProgressBar()
	if n.BgColor != "" {
		if c, err := base.ParseRGBAHex(n.BgColor); err == nil {
			pb.FillColor = c
		}
	}
	return pb
}

// ─── Новые виджеты (JSON) ────────────────────────────────────────────────────

func buildCheckBoxJSON(n UINode) base.Widget {
	cb := base.NewCheckBox(n.Text)
	if n.Props["checked"] == "true" {
		cb.SetChecked(true)
	}
	if n.TextColor != "" {
		if c, err := base.ParseRGBAHex(n.TextColor); err == nil {
			cb.TextColor = c
		}
	}
	return cb
}

func buildRadioButtonJSON(n UINode) base.Widget {
	group := n.Props["group"]
	rb := base.NewRadioButton(n.Text, group)
	if n.Props["selected"] == "true" || n.Props["checked"] == "true" {
		rb.SetSelected(true)
	}
	return rb
}

func buildSliderJSON(n UINode) base.Widget {
	s := base.NewSlider()
	if min, ok := n.Props["min"]; ok {
		if v, err := strconv.ParseFloat(min, 64); err == nil {
			s.Min = v
		}
	}
	if max, ok := n.Props["max"]; ok {
		if v, err := strconv.ParseFloat(max, 64); err == nil {
			s.Max = v
		}
	}
	if val, ok := n.Props["value"]; ok {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			s.SetValue(v)
		}
	}
	return s
}

func buildToggleSwitchJSON(n UINode) base.Widget {
	ts := base.NewToggleSwitch(n.Text)
	if n.Props["on"] == "true" || n.Props["checked"] == "true" {
		ts.SetOn(true)
	}
	return ts
}

func buildScrollViewJSON(n UINode) base.Widget {
	sv := base.NewScrollView()
	if ch, ok := n.Props["contentHeight"]; ok {
		if v, err := strconv.Atoi(ch); err == nil {
			sv.ContentHeight = v
		}
	}
	if n.BgColor != "" {
		if c, err := base.ParseRGBAHex(n.BgColor); err == nil {
			sv.Background = c
		}
	}
	return sv
}

func buildListViewJSON(n UINode) base.Widget {
	lv := base.NewListView(n.Items...)
	if n.Selected >= 0 && n.Selected < len(n.Items) {
		lv.SetSelectedQuiet(n.Selected) // начальное выделение — не выбор пользователя
	}
	if ih, ok := n.Props["itemHeight"]; ok {
		if v, err := strconv.Atoi(ih); err == nil && v > 0 {
			lv.ItemHeight = v
		}
	}
	return lv
}

func buildTabControlJSON(n UINode) base.Widget {
	tc := base.NewTabControl()
	if sel := n.Selected; sel >= 0 {
		tc.SetActive(sel)
	}
	return tc
}

// ─── Парсинг цвета ────────────────────────────────────────────────────────────

