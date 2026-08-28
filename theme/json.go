package theme

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"strconv"
	"strings"
	"time"
)

// jsonProfile — профиль в том виде, в каком он лежит в файле.
//
// Формат отличается от Profile по двум причинам. Цвета в файле пишутся
// строкой «#RRGGBB» или «#RRGGBBAA» с ПРЯМОЙ альфой — так их пишут люди и
// так их выдают редакторы; премультиплицирование делает загрузчик. А ключ
// стиля — не структура, а строка «component.part:state», потому что ключом
// объекта JSON структура быть не может.
type jsonProfile struct {
	Name    string             `json:"name"`
	Parent  string             `json:"parent,omitempty"`
	Colors  map[string]string  `json:"colors,omitempty"`
	Metrics map[string]float64 `json:"metrics,omitempty"`
	Flags   map[string]bool    `json:"flags,omitempty"`
	Fonts   map[string]struct {
		Family string  `json:"family,omitempty"`
		Size   float64 `json:"size,omitempty"`
		Bold   bool    `json:"bold,omitempty"`
		Italic bool    `json:"italic,omitempty"`
	} `json:"fonts,omitempty"`
	Icons map[string]struct {
		Name   string `json:"name,omitempty"`
		Source string `json:"source,omitempty"`
	} `json:"icons,omitempty"`
	Anims map[string]struct {
		DurationMS int    `json:"duration_ms,omitempty"`
		Curve      string `json:"curve,omitempty"`
	} `json:"anims,omitempty"`
	Styles     map[string]jsonStyle `json:"styles,omitempty"`
	Presenters map[string]string    `json:"presenters,omitempty"`
}

type jsonStyle struct {
	Fill   string `json:"fill,omitempty"`
	Text   string `json:"text,omitempty"`
	Border string `json:"border,omitempty"`
	Shadow string `json:"shadow,omitempty"`

	Corner      *float64 `json:"corner,omitempty"`
	BorderWidth *float64 `json:"border_width,omitempty"`
	PadX        *float64 `json:"pad_x,omitempty"`
	PadY        *float64 `json:"pad_y,omitempty"`
	Elevation   *float64 `json:"elevation,omitempty"`

	Gradient []struct {
		Pos   float64 `json:"pos"`
		Color string  `json:"color"`
	} `json:"gradient,omitempty"`
	GradientAngle *float64 `json:"gradient_angle,omitempty"`
	// Вид градиента и параметры радиального: "linear" (по умолчанию) либо
	// "radial". Центр и радиус — ДОЛЯМИ области, как в Go-профилях: одна и
	// та же подсветка ложится под значок любого размера.
	GradientKind    string   `json:"gradient_kind,omitempty"`
	GradientCenterX *float64 `json:"gradient_center_x,omitempty"`
	GradientCenterY *float64 `json:"gradient_center_y,omitempty"`
	GradientRadius  *float64 `json:"gradient_radius,omitempty"`

	Font *struct {
		Family string  `json:"family,omitempty"`
		Size   float64 `json:"size,omitempty"`
		Bold   bool    `json:"bold,omitempty"`
		Italic bool    `json:"italic,omitempty"`
	} `json:"font,omitempty"`

	Backdrop *struct {
		Mode   string  `json:"mode,omitempty"` // none | alpha | blur
		Radius float64 `json:"radius,omitempty"`
		Tint   string  `json:"tint,omitempty"`
		// Блик по верхней кромке: именно он делает размытую подложку
		// стеклом, а не плоской заливкой.
		Highlight string `json:"highlight,omitempty"`
	} `json:"backdrop,omitempty"`

	Bevel *struct {
		Light  string  `json:"light,omitempty"`
		Shadow string  `json:"shadow,omitempty"`
		Dark   string  `json:"dark,omitempty"`
		Width  float64 `json:"width,omitempty"`
		Sunken bool    `json:"sunken,omitempty"`
	} `json:"bevel,omitempty"`
}

// LoadResult — что получилось при загрузке профиля.
//
// Warnings собирает всё, что загрузчик не понял, но пережил: неизвестное
// состояние в ключе стиля, нечитаемый цвет. Профиль с предупреждениями
// пригоден к работе — это осознанное решение: тема, где одна строка цвета
// написана с опечаткой, должна показать остальные девяносто девять, а не
// отказать целиком.
type LoadResult struct {
	Profile  *Profile
	Warnings []string
}

// LoadTheme читает профиль из JSON.
//
// Возвращает профиль даже при частично неверных данных; всё непонятое
// перечислено в LoadResult.Warnings. Ошибка возвращается только тогда,
// когда читать нечего: сломанный JSON или профиль без имени.
func LoadTheme(r io.Reader) (*LoadResult, error) {
	var jp jsonProfile
	dec := json.NewDecoder(r)
	if err := dec.Decode(&jp); err != nil {
		return nil, fmt.Errorf("theme: разбор JSON: %w", err)
	}
	if jp.Name == "" {
		return nil, fmt.Errorf("theme: в профиле не указано имя")
	}

	res := &LoadResult{Profile: NewProfile(jp.Name)}
	p := res.Profile
	p.Parent = jp.Parent

	warn := func(format string, args ...any) {
		res.Warnings = append(res.Warnings, fmt.Sprintf(format, args...))
	}

	for k, v := range jp.Colors {
		c, err := ParseColor(v)
		if err != nil {
			warn("цвет %q: %v", k, err)
			continue
		}
		p.Colors[Key(k)] = c
	}
	for k, v := range jp.Metrics {
		p.Metrics[Key(k)] = v
	}
	for k, v := range jp.Flags {
		p.Flags[Key(k)] = v
	}
	for k, v := range jp.Fonts {
		p.Fonts[Key(k)] = FontSpec{Family: v.Family, Size: v.Size, Bold: v.Bold, Italic: v.Italic}
	}
	for k, v := range jp.Icons {
		p.Icons[Key(k)] = IconRef{Name: v.Name, Source: v.Source}
	}
	for k, v := range jp.Anims {
		p.Anims[Key(k)] = AnimSpec{
			Duration:   time.Duration(v.DurationMS) * time.Millisecond,
			DurationMS: v.DurationMS,
			Curve:      v.Curve,
		}
	}
	for k, v := range jp.Presenters {
		p.Presenters[k] = v
	}

	for rawKey, js := range jp.Styles {
		key, err := ParseStyleKey(rawKey)
		if err != nil {
			warn("стиль %q: %v", rawKey, err)
			continue
		}
		d, ws := js.toDelta()
		for _, w := range ws {
			warn("стиль %q: %s", rawKey, w)
		}
		p.Styles[key] = d
	}

	return res, nil
}

// toDelta переводит запись стиля из JSON в дельту, собирая предупреждения
// по нечитаемым цветам.
func (js jsonStyle) toDelta() (StyleDelta, []string) {
	var d StyleDelta
	var warns []string

	colorField := func(raw, name string, dst **color.RGBA) {
		if raw == "" {
			return
		}
		c, err := ParseColor(raw)
		if err != nil {
			warns = append(warns, fmt.Sprintf("%s: %v", name, err))
			return
		}
		*dst = &c
	}
	colorField(js.Fill, "fill", &d.Fill)
	colorField(js.Text, "text", &d.Text)
	colorField(js.Border, "border", &d.Border)
	colorField(js.Shadow, "shadow", &d.Shadow)

	d.Corner, d.BorderWidth, d.PadX, d.PadY = js.Corner, js.BorderWidth, js.PadX, js.PadY
	d.Elevation = js.Elevation
	d.GradientAngle = js.GradientAngle

	if js.GradientKind != "" {
		if k, ok := ParseGradientKind(js.GradientKind); ok {
			d.GradientKind = &k
		} else {
			warns = append(warns, fmt.Sprintf("gradient_kind: неизвестный вид %q", js.GradientKind))
		}
	}
	d.GradientCenterX, d.GradientCenterY = js.GradientCenterX, js.GradientCenterY
	d.GradientRadius = js.GradientRadius

	for i, stop := range js.Gradient {
		c, err := ParseColor(stop.Color)
		if err != nil {
			warns = append(warns, fmt.Sprintf("градиент, точка %d: %v", i, err))
			continue
		}
		d.Gradient = append(d.Gradient, GradientStop{Pos: stop.Pos, Color: c})
	}

	if js.Font != nil {
		d.Font = &FontSpec{Family: js.Font.Family, Size: js.Font.Size, Bold: js.Font.Bold, Italic: js.Font.Italic}
	}

	if js.Backdrop != nil {
		b := BackdropSpec{Radius: js.Backdrop.Radius}
		switch js.Backdrop.Mode {
		case "", "none":
			b.Mode = BackdropNone
		case "alpha":
			b.Mode = BackdropAlpha
		case "blur":
			b.Mode = BackdropBlur
		default:
			warns = append(warns, fmt.Sprintf("подложка: неизвестный режим %q", js.Backdrop.Mode))
		}
		if js.Backdrop.Tint != "" {
			if c, err := ParseColor(js.Backdrop.Tint); err == nil {
				b.Tint = c
			} else {
				warns = append(warns, fmt.Sprintf("подложка, tint: %v", err))
			}
		}
		if js.Backdrop.Highlight != "" {
			if c, err := ParseColor(js.Backdrop.Highlight); err == nil {
				b.Highlight = c
			} else {
				warns = append(warns, fmt.Sprintf("подложка, highlight: %v", err))
			}
		}
		d.Backdrop = &b
	}

	if js.Bevel != nil {
		bv := BevelSpec{Width: js.Bevel.Width, Sunken: js.Bevel.Sunken}
		for _, f := range []struct {
			raw, name string
			dst       *color.RGBA
		}{
			{js.Bevel.Light, "light", &bv.Light},
			{js.Bevel.Shadow, "shadow", &bv.Shadow},
			{js.Bevel.Dark, "dark", &bv.Dark},
		} {
			if f.raw == "" {
				continue
			}
			c, err := ParseColor(f.raw)
			if err != nil {
				warns = append(warns, fmt.Sprintf("фаска, %s: %v", f.name, err))
				continue
			}
			*f.dst = c
		}
		d.Bevel = &bv
	}

	return d, warns
}

// ParseStyleKey разбирает ключ стиля вида «component», «component.part»,
// «component:state» или «component.part:state».
func ParseStyleKey(s string) (StyleKey, error) {
	var k StyleKey
	body := s
	if i := strings.IndexByte(s, ':'); i >= 0 {
		body = s[:i]
		st, err := ParseState(s[i+1:])
		if err != nil {
			return k, err
		}
		k.State = st
	}
	if body == "" {
		return k, fmt.Errorf("не указан компонент")
	}
	if i := strings.IndexByte(body, '.'); i >= 0 {
		k.Component, k.Part = body[:i], body[i+1:]
	} else {
		k.Component = body
	}
	return k, nil
}

// String возвращает ключ в том же виде, в каком его читает ParseStyleKey.
func (k StyleKey) String() string {
	s := k.Component
	if k.Part != "" {
		s += "." + k.Part
	}
	if k.State != StateNormal {
		s += ":" + k.State.String()
	}
	return s
}

// ParseColor разбирает цвет «#RGB», «#RRGGBB» или «#RRGGBBAA».
//
// Альфа в записи — ПРЯМАЯ (как в CSS), результат — alpha-premultiplied,
// как того требует color.RGBA в Go и вся отрисовка движка. Автор темы
// пишет «#0078D75A», не задумываясь о премультиплицировании, и не получает
// пересвеченный пурпур на светлом фоне.
func ParseColor(s string) (color.RGBA, error) {
	raw := strings.TrimSpace(s)
	raw = strings.TrimPrefix(raw, "#")
	var r, g, b, a uint64
	var err error

	hex := func(part string) uint64 {
		if err != nil {
			return 0
		}
		var v uint64
		v, err = strconv.ParseUint(part, 16, 16)
		return v
	}

	switch len(raw) {
	case 3: // #RGB — каждый разряд удваивается
		r, g, b = hex(raw[0:1]), hex(raw[1:2]), hex(raw[2:3])
		r, g, b = r*17, g*17, b*17
		a = 255
	case 6:
		r, g, b = hex(raw[0:2]), hex(raw[2:4]), hex(raw[4:6])
		a = 255
	case 8:
		r, g, b, a = hex(raw[0:2]), hex(raw[2:4]), hex(raw[4:6]), hex(raw[6:8])
	default:
		return color.RGBA{}, fmt.Errorf("непонятная запись цвета %q (ждали #RGB, #RRGGBB или #RRGGBBAA)", s)
	}
	if err != nil {
		return color.RGBA{}, fmt.Errorf("непонятная запись цвета %q: %w", s, err)
	}
	return RGBA(uint8(r), uint8(g), uint8(b), uint8(a)), nil
}

// FormatColor записывает цвет так, как его читает ParseColor: обратный
// перевод из premultiplied в прямую альфу.
func FormatColor(c color.RGBA) string {
	if c.A == 0 {
		return "#00000000"
	}
	un := func(v uint8) uint8 {
		x := uint32(v) * 255 / uint32(c.A)
		if x > 255 {
			x = 255
		}
		return uint8(x)
	}
	if c.A == 255 {
		return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02X%02X%02X%02X", un(c.R), un(c.G), un(c.B), c.A)
}
