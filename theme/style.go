package theme

import "image/color"

// Style — как выглядит компонент в одном состоянии. Плоская структура:
// всё, что нужно отрисовке, лежит рядом и читается без обращений к теме.
//
// Компонент получает её указателем из уже разрешённой таблицы (см.
// Theme.Style) и НЕ ИМЕЕТ ПРАВА менять — указатель общий для всех, кто
// спросил тот же стиль. Нужен изменённый — скопируйте значение.
type Style struct {
	// Цвета. Все — alpha-premultiplied (модель color.RGBA в Go):
	// R,G,B ≤ A. Прозрачный цвет (A=0) означает «не рисовать».
	Fill   color.RGBA
	Text   color.RGBA
	Border color.RGBA
	Shadow color.RGBA

	// Gradient заменяет Fill, когда задан: две и более точки вдоль оси
	// GradientAngle (в градусах, 0 — слева направо, 90 — сверху вниз).
	Gradient      []GradientStop
	GradientAngle float64

	// Геометрия — в логических пикселях.
	Corner      float64 // радиус скругления углов
	BorderWidth float64 // толщина рамки; 0 — рамки нет
	PadX, PadY  float64 // внутренние отступы содержимого

	Font FontSpec

	// Backdrop — что видно под слоем (прозрачность, размытие).
	Backdrop BackdropSpec

	// Elevation — высота над подложкой; из неё считается мягкая тень.
	// 0 — тени нет.
	Elevation float64

	// Bevel — объёмная рамка вместо плоской (Windows 2000). nil — плоская.
	Bevel *BevelSpec
}

// Clone возвращает независимую копию: срез точек градиента и BevelSpec
// копируются, а не разделяются. Нужен тому, кто хочет подправить стиль
// под себя, не задев общий.
func (s *Style) Clone() *Style {
	if s == nil {
		return nil
	}
	c := *s
	if s.Gradient != nil {
		c.Gradient = append([]GradientStop(nil), s.Gradient...)
	}
	if s.Bevel != nil {
		b := *s.Bevel
		c.Bevel = &b
	}
	return &c
}

// StyleDelta — частичное переопределение стиля в профиле темы. Указатель
// на поле означает «задано»; nil — «взять у родителя или из состояния
// ниже по приоритету».
//
// Из-за этого дочерний профиль пишется коротко: Windows11Dark объявляет
// десяток цветов и наследует у Windows11 всё остальное, включая геометрию,
// шрифты и анимации.
type StyleDelta struct {
	Fill   *color.RGBA `json:"-"`
	Text   *color.RGBA `json:"-"`
	Border *color.RGBA `json:"-"`
	Shadow *color.RGBA `json:"-"`

	Gradient      []GradientStop `json:"gradient,omitempty"`
	GradientAngle *float64       `json:"gradient_angle,omitempty"`

	Corner      *float64 `json:"corner,omitempty"`
	BorderWidth *float64 `json:"border_width,omitempty"`
	PadX        *float64 `json:"pad_x,omitempty"`
	PadY        *float64 `json:"pad_y,omitempty"`

	Font     *FontSpec     `json:"font,omitempty"`
	Backdrop *BackdropSpec `json:"backdrop,omitempty"`

	Elevation *float64   `json:"elevation,omitempty"`
	Bevel     *BevelSpec `json:"bevel,omitempty"`
}

// applyTo накладывает заданные поля дельты на стиль.
func (d *StyleDelta) applyTo(s *Style) {
	if d == nil {
		return
	}
	if d.Fill != nil {
		s.Fill = *d.Fill
	}
	if d.Text != nil {
		s.Text = *d.Text
	}
	if d.Border != nil {
		s.Border = *d.Border
	}
	if d.Shadow != nil {
		s.Shadow = *d.Shadow
	}
	if d.Gradient != nil {
		s.Gradient = append([]GradientStop(nil), d.Gradient...)
	}
	if d.GradientAngle != nil {
		s.GradientAngle = *d.GradientAngle
	}
	if d.Corner != nil {
		s.Corner = *d.Corner
	}
	if d.BorderWidth != nil {
		s.BorderWidth = *d.BorderWidth
	}
	if d.PadX != nil {
		s.PadX = *d.PadX
	}
	if d.PadY != nil {
		s.PadY = *d.PadY
	}
	if d.Font != nil {
		s.Font = *d.Font
	}
	if d.Backdrop != nil {
		s.Backdrop = *d.Backdrop
	}
	if d.Elevation != nil {
		s.Elevation = *d.Elevation
	}
	if d.Bevel != nil {
		b := *d.Bevel
		s.Bevel = &b
	}
}

// ─── Помощники для составления профилей в коде ──────────────────────────────
//
// Профиль, написанный на Go, иначе утонул бы во временных переменных: взять
// адрес у литерала нельзя, и каждое поле требовало бы отдельной строки.

// C возвращает указатель на цвет — для полей StyleDelta.
func C(c color.RGBA) *color.RGBA { return &c }

// N возвращает указатель на число — для полей StyleDelta.
func N(v float64) *float64 { return &v }

// RGB собирает непрозрачный цвет.
func RGB(r, g, b uint8) color.RGBA { return color.RGBA{R: r, G: g, B: b, A: 255} }

// RGBA собирает полупрозрачный цвет из ПРЯМОЙ (straight) записи и
// премультиплицирует его — в таком виде его ждут и движок, и Style.
//
// Ловушка, ради которой этот помощник и существует: color.RGBA в Go —
// alpha-premultiplied, и цвет вроде {0,120,215,90}, записанный «как в CSS»,
// при смешивании даёт пересвет и уползает в пурпур на светлом фоне.
func RGBA(r, g, b, a uint8) color.RGBA {
	m := func(v uint8) uint8 { return uint8(uint32(v) * uint32(a) / 255) }
	return color.RGBA{R: m(r), G: m(g), B: m(b), A: a}
}
