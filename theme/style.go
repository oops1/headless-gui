package theme

import (
	"image/color"
	"math"
)

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
	// GradientKind — линейный (по умолчанию) или радиальный.
	GradientKind GradientKind
	// GradientCenter — центр радиального градиента в долях области
	// (0.5, 0.5 — середина). Нулевое значение означает середину: так стиль,
	// который просто попросил радиальный градиент, получает ожидаемое.
	GradientCenterX, GradientCenterY float64
	// GradientRadius — радиус в долях половины большей стороны (0 — до края).
	GradientRadius float64

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

	// Токены тени (Windows 11): ShadowBlur — радиус размытия, ShadowOffsetX/Y —
	// смещение, ShadowOpacity — множитель прозрачности цвета Shadow. Общие для
	// меню, всплывающих панелей и окон; значение читается через ResolveShadow.
	//
	// ShadowBlur == 0 (все прежние профили) — токены не заданы, тень считается
	// из Elevation, как раньше. ShadowOpacity == 0 — «не задан», то есть 1.
	ShadowBlur                   float64
	ShadowOffsetX, ShadowOffsetY float64
	ShadowOpacity                float64

	// Bevel — объёмная рамка вместо плоской (Windows 2000). nil — плоская.
	Bevel *BevelSpec
}

// ShadowSpec — разрешённая тень одного слоя: радиус размытия и смещение в
// логических пикселях, цвет уже с учётом прозрачности (alpha-premultiplied).
// Отдаётся Style.ResolveShadow и рисуется widget.DrawShadowSpec.
type ShadowSpec struct {
	Blur             float64
	OffsetX, OffsetY float64
	Color            color.RGBA
}

// IsZero — тени нет.
func (s ShadowSpec) IsZero() bool { return s.Blur <= 0 || s.Color.A == 0 }

// Extent — на сколько пикселей тень выходит за границы слоя: по радиусу
// размытия (в две стороны — тень строится двухпроходным размытием) и смещению.
// Нужен тому, кто резервирует место под тень (область движения панели,
// DrawMargin окна).
func (s ShadowSpec) Extent() int {
	if s.IsZero() {
		return 0
	}
	off := math.Max(math.Max(math.Abs(s.OffsetX), math.Abs(s.OffsetY)), 0)
	return int(math.Ceil(s.Blur*2+off)) + 1
}

// ExplicitShadow возвращает тень по ТОКЕНАМ стиля. false — токены не заданы
// (ShadowBlur == 0): компонент остаётся на прежней тени от Elevation.
//
// Цвет — Shadow стиля; у профиля, объявившего токены без цвета, тень чёрная.
// ShadowOpacity умножает альфу цвета (премультиплицированные каналы — вместе с
// ней).
func (s *Style) ExplicitShadow() (ShadowSpec, bool) {
	if s == nil || s.ShadowBlur <= 0 {
		return ShadowSpec{}, false
	}
	col := s.Shadow
	if col.A == 0 {
		col = color.RGBA{A: 255}
	}
	if op := s.ShadowOpacity; op > 0 && op < 1 {
		scale := func(v uint8) uint8 { return uint8(float64(v)*op + 0.5) }
		col = color.RGBA{R: scale(col.R), G: scale(col.G), B: scale(col.B), A: scale(col.A)}
	}
	return ShadowSpec{Blur: s.ShadowBlur, OffsetX: s.ShadowOffsetX, OffsetY: s.ShadowOffsetY, Color: col}, true
}

// ResolveShadow возвращает тень стиля: по токенам, а если их нет — из
// Elevation и Shadow ровно так, как рисовалось всегда (размытие = высота,
// смещение вниз = высота/2). false — тени нет. Единая точка для всех, кто
// рисует тень по стилю: PaintStyle, окна, диалоги, меню.
func (s *Style) ResolveShadow() (ShadowSpec, bool) {
	if s == nil {
		return ShadowSpec{}, false
	}
	if sp, ok := s.ExplicitShadow(); ok {
		return sp, !sp.IsZero()
	}
	if s.Elevation > 0 && s.Shadow.A > 0 {
		return ShadowSpec{Blur: s.Elevation, OffsetY: s.Elevation / 2, Color: s.Shadow}, true
	}
	return ShadowSpec{}, false
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

	// FillFrom, TextFrom, BorderFrom — цвет по ссылке на плоский токен темы
	// ("accent", "accent.hover", "surface"…) вместо готового значения.
	// Ссылка раскрывается при разрешении темы, по ГОТОВЫМ токенам цепочки, —
	// поэтому смена токена (Manager.SetAccent) меняет и стили, которые на
	// него ссылаются, а потомок профиля может подменить сам токен, не трогая
	// стилей. Если задана и ссылка, и готовое значение ОДНОЙ дельты, побеждает
	// ссылка; дельта потомка, как всегда, перекрывает дельту предка. Токен,
	// которого в теме нет, ссылку отменяет: остаётся значение из предыдущих
	// дельт.
	FillFrom, TextFrom, BorderFrom Key `json:"-"`

	// ShadowFrom — цвет тени по ссылке на токен (то же, что BorderFrom, для
	// Shadow): тёмная разновидность темы меняет токен, а не переписывает стили.
	ShadowFrom Key `json:"-"`
	// BackdropFrom — основа материала подложки по ссылке на цветовой токен
	// ("surface", "surface.alt"): из него берутся непрозрачный Fallback и цвет
	// подкраски Tint (альфа подкраски остаётся из объявленного Backdrop.Tint).
	// Так Mica светлой и тёмной тем отличается одним токеном, а не копией стилей.
	BackdropFrom Key `json:"-"`

	Gradient      []GradientStop `json:"gradient,omitempty"`
	GradientAngle *float64       `json:"gradient_angle,omitempty"`
	GradientKind  *GradientKind  `json:"gradient_kind,omitempty"`

	GradientCenterX *float64 `json:"gradient_center_x,omitempty"`
	GradientCenterY *float64 `json:"gradient_center_y,omitempty"`
	GradientRadius  *float64 `json:"gradient_radius,omitempty"`

	Corner      *float64 `json:"corner,omitempty"`
	BorderWidth *float64 `json:"border_width,omitempty"`
	PadX        *float64 `json:"pad_x,omitempty"`
	PadY        *float64 `json:"pad_y,omitempty"`

	Font     *FontSpec     `json:"font,omitempty"`
	Backdrop *BackdropSpec `json:"backdrop,omitempty"`

	Elevation *float64   `json:"elevation,omitempty"`
	Bevel     *BevelSpec `json:"bevel,omitempty"`

	// Токены тени (см. Style.ShadowBlur).
	ShadowBlur    *float64 `json:"shadow_blur,omitempty"`
	ShadowOffsetX *float64 `json:"shadow_offset_x,omitempty"`
	ShadowOffsetY *float64 `json:"shadow_offset_y,omitempty"`
	ShadowOpacity *float64 `json:"shadow_opacity,omitempty"`
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
	if d.GradientKind != nil {
		s.GradientKind = *d.GradientKind
	}
	if d.GradientCenterX != nil {
		s.GradientCenterX = *d.GradientCenterX
	}
	if d.GradientCenterY != nil {
		s.GradientCenterY = *d.GradientCenterY
	}
	if d.GradientRadius != nil {
		s.GradientRadius = *d.GradientRadius
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
	if d.ShadowBlur != nil {
		s.ShadowBlur = *d.ShadowBlur
	}
	if d.ShadowOffsetX != nil {
		s.ShadowOffsetX = *d.ShadowOffsetX
	}
	if d.ShadowOffsetY != nil {
		s.ShadowOffsetY = *d.ShadowOffsetY
	}
	if d.ShadowOpacity != nil {
		s.ShadowOpacity = *d.ShadowOpacity
	}
	if d.Bevel != nil {
		b := *d.Bevel
		s.Bevel = &b
	}
}

// applyTokens раскрывает ссылки на цветовые токены поверх уже наложенных
// значений дельты.
func (d *StyleDelta) applyTokens(s *Style, colors map[Key]color.RGBA) {
	if d == nil {
		return
	}
	if d.FillFrom != "" {
		if c, ok := colors[d.FillFrom]; ok {
			s.Fill = c
		}
	}
	if d.TextFrom != "" {
		if c, ok := colors[d.TextFrom]; ok {
			s.Text = c
		}
	}
	if d.BorderFrom != "" {
		if c, ok := colors[d.BorderFrom]; ok {
			s.Border = c
		}
	}
	if d.ShadowFrom != "" {
		if c, ok := colors[d.ShadowFrom]; ok {
			s.Shadow = c
		}
	}
	if d.BackdropFrom != "" {
		if c, ok := colors[d.BackdropFrom]; ok {
			base := RGB(c.R, c.G, c.B)
			s.Backdrop.Fallback = base
			if a := s.Backdrop.Tint.A; a > 0 {
				s.Backdrop.Tint = RGBA(base.R, base.G, base.B, a)
			}
		}
	}
}

// ─── Помощники для составления профилей в коде ──────────────────────────────
//
// Профиль, написанный на Go, иначе утонул бы во временных переменных: взять
// адрес у литерала нельзя, и каждое поле требовало бы отдельной строки.

// C возвращает указатель на цвет — для полей StyleDelta.
func C(c color.RGBA) *color.RGBA { return &c }

// GK возвращает указатель на вид градиента — для StyleDelta.
func GK(k GradientKind) *GradientKind { return &k }

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
