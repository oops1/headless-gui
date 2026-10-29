// parse.go — XAML XML-парсер и утилиты разбора.
//
// Содержит: Element (внутреннее дерево), парсинг XML → Element,
// парсинг цветов, Margin и вспомогательные функции.
package xaml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/oops1/headless-gui/v3/widget/base"
)

// ─── Предел вложенности (SEC-7) ─────────────────────────────────────────────

// maxXAMLDepth — предельная глубина вложенности XAML-элементов.
//
// Разбор XML в Element, пред-обработка ресурсов и построение виджетов —
// рекурсивные обходы. Файл вида «<a><a><a>…» глубиной в сотни тысяч уровней
// исчерпывал стек горутины и ронял процесс целиком: переполнение стека в Go
// не восстанавливается через recover, это фатальная ошибка рантайма.
//
// 256 — с многократным запасом над любой осмысленной вёрсткой (реальные окна
// редко глубже 15–20 уровней), но заведомо безопасно по стеку: ~256 кадров
// парсера/билдера укладываются в стартовый стек горутины.
const maxXAMLDepth = 256

// ErrXAMLTooDeep — разметка вложена глубже maxXAMLDepth.
var ErrXAMLTooDeep = errors.New("xaml: превышена предельная вложенность элементов")

// ─── Внутреннее дерево XAML ─────────────────────────────────────────────────

// Element — узел XAML-дерева. Хранит имя тега, атрибуты, потомков и текст.
//
// Служебное: для пакетов виджетов (их XAML-строителей), в фасад не входит.
//
//facadegen:skip
type Element struct {
	Tag      string
	attrs    map[string]string
	Children []Element
	Text     string // текст между тегами (напр. <Item>Текст</Item>)
}

// Attr возвращает значение первого найденного атрибута из списка.
func (e *Element) Attr(names ...string) string {
	for _, n := range names {
		if v, ok := e.attrs[n]; ok {
			return v
		}
	}
	return ""
}

// Name возвращает идентификатор элемента (Name или x:Name).
func (e *Element) Name() string {
	return e.Attr("Name", "x:Name")
}

// Bounds вычисляет image.Rectangle из атрибутов позиции/размера.
// Поддерживает Left/Top/Right/Bottom и Left/Top/Width/Height.
func (e *Element) Bounds() image.Rectangle {
	left := ParseInt(e.Attr("Left", "X", "Canvas.Left"))
	top := ParseInt(e.Attr("Top", "Y", "Canvas.Top"))
	right := ParseInt(e.Attr("Right", "Canvas.Right"))
	bottom := ParseInt(e.Attr("Bottom", "Canvas.Bottom"))

	if w := ParseInt(e.Attr("Width")); w > 0 && right == 0 {
		right = left + w
	}
	if h := ParseInt(e.Attr("Height")); h > 0 && bottom == 0 {
		bottom = top + h
	}
	// Без Width/Right (Height/Bottom) размер по этой оси неизвестен — его
	// назовёт родитель или сам виджет. Прямоугольник при этом обязан
	// остаться ВЫРОЖДЕННЫМ В СВОЕЙ ТОЧКЕ: image.Rect нормализует границы, и
	// Rect(left, top, 0, 0) превращался в (0,0)-(left,top) — виджет получал
	// ширину, равную своей же координате Left, и высоту, равную Top. Подпись
	// с Left=24 жила в прямоугольнике шириной 24: текст рисовался за его
	// пределами и пропадал при любой частичной перерисовке.
	if right < left {
		right = left
	}
	if bottom < top {
		bottom = top
	}
	return image.Rect(left, top, right, bottom)
}

// ParseInt — целое из атрибута разметки (пробелы по краям отбрасываются);
// пусто или не число — 0.
//
// Служебное: для пакетов виджетов (их XAML-строителей), в фасад не входит.
//
//facadegen:skip
func ParseInt(s string) int {
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
	v := ParseInt(s)
	if v == 0 && s != "0" {
		return -1
	}
	return v
}

// ─── Парсер XML → Element ──────────────────────────────────────────────────

func parseXAML(data []byte) (*Element, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, fmt.Errorf("xaml: scan: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			var root Element
			if err := parseXAMLEl(d, start, &root, 0); err != nil {
				return nil, err
			}
			return &root, nil
		}
	}
}

// parseXAMLEl разбирает один элемент и его потомков. depth — текущая глубина
// вложенности (0 у корня); превышение maxXAMLDepth прерывает разбор ошибкой,
// не давая рекурсии исчерпать стек (SEC-7).
func parseXAMLEl(d *xml.Decoder, start xml.StartElement, el *Element, depth int) error {
	if depth > maxXAMLDepth {
		return fmt.Errorf("%w (%d): <%s>", ErrXAMLTooDeep, maxXAMLDepth, start.Name.Local)
	}
	el.Tag = start.Name.Local
	el.attrs = make(map[string]string, len(start.Attr)*2)
	for _, a := range start.Attr {
		el.attrs[a.Name.Local] = a.Value
		// WPF attached properties: namespace="DockPanel", local="Dock" → "DockPanel.Dock"
		if a.Name.Space != "" {
			el.attrs[a.Name.Space+"."+a.Name.Local] = a.Value
		}
	}
	for {
		tok, err := d.Token()
		if err != nil {
			return fmt.Errorf("xaml: <%s>: %w", el.Tag, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			var child Element
			if err := parseXAMLEl(d, t, &child, depth+1); err != nil {
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

// ParseMargin разбирает WPF Margin: "5", "5,10", "1,2,3,4".
//
// Служебное: для пакетов виджетов (их XAML-строителей), в фасад не входит.
//
//facadegen:skip
func ParseMargin(s string) base.Margin {
	s = strings.TrimSpace(s)
	if s == "" {
		return base.Margin{}
	}
	parts := strings.Split(s, ",")
	switch len(parts) {
	case 1:
		v := ParseInt(parts[0])
		return base.Margin{Left: v, Top: v, Right: v, Bottom: v}
	case 2:
		h := ParseInt(parts[0])
		v := ParseInt(parts[1])
		return base.Margin{Left: h, Top: v, Right: h, Bottom: v}
	case 4:
		return base.Margin{
			Left:   ParseInt(parts[0]),
			Top:    ParseInt(parts[1]),
			Right:  ParseInt(parts[2]),
			Bottom: ParseInt(parts[3]),
		}
	default:
		return base.Margin{}
	}
}

// ─── Цвета ─────────────────────────────────────────────────────────────────

// ParseColor разбирает строку цвета: "#RRGGBB", "#RRGGBBAA" или именованный цвет.
// Для hex-значений использует ParseRGBAHex из loader.go.
//
// Служебное: для пакетов виджетов (их XAML-строителей), в фасад не входит.
//
//facadegen:skip
func ParseColor(s string) (color.RGBA, error) {
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
		return base.ParseRGBAHex(s) // "#RRGGBB" / "#RRGGBBAA"
	}
}

// ApplyColor парсит XAML-атрибут цвета и записывает в dst.
// Если атрибут не найден или не парсится — dst не меняется.
//
// Служебное: для пакетов виджетов (их XAML-строителей), в фасад не входит.
//
//facadegen:skip
func ApplyColor(dst *color.RGBA, el Element, names ...string) {
	if s := el.Attr(names...); s != "" {
		if c, err := ParseColor(s); err == nil {
			*dst = c
		}
	}
}
