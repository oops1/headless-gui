package theme

import (
	"image/color"
	"time"
)

// Key — имя плоского токена: "taskbar.height", "button.corner",
// "accent". Точки — только соглашение о читаемости, разбором имени пакет
// не занимается.
type Key string

// FontSpec — шрифт как данные темы. Имя семейства соответствует шрифту,
// зарегистрированному в движке (engine.RegisterFont); пустое — шрифт по
// умолчанию.
type FontSpec struct {
	Family string  `json:"family,omitempty"`
	Size   float64 `json:"size,omitempty"` // в пунктах; 0 — размер по умолчанию
	Bold   bool    `json:"bold,omitempty"`
	Italic bool    `json:"italic,omitempty"`
}

// IsZero — спецификация ничего не задаёт (её можно наследовать целиком).
func (f FontSpec) IsZero() bool { return f == FontSpec{} }

// IconRef — ссылка на иконку в наборе темы. Источником может быть файл SVG
// или заранее зарегистрированное имя; разрешает ссылку IconSet, а не тема.
type IconRef struct {
	Name   string `json:"name,omitempty"`   // имя в наборе: "start", "volume.muted"
	Source string `json:"source,omitempty"` // путь к SVG относительно профиля
}

// IsZero — ссылка пуста.
func (r IconRef) IsZero() bool { return r == IconRef{} }

// AnimSpec — анимация как данные темы: сколько длится и по какой кривой
// идёт. Имя кривой соответствует easing-кривым движка ("linear",
// "out-cubic", "out-back", …); пустое — линейная.
//
// Тема задаёт длительность нулём там, где анимации не место: классика
// Windows 2000 не анимирует ничего, и компоненту не нужно об этом знать —
// он просто получит нулевую длительность и переключится мгновенно.
type AnimSpec struct {
	Duration time.Duration `json:"-"`
	Curve    string        `json:"curve,omitempty"`

	// DurationMS — длительность для JSON (миллисекунды); при загрузке
	// профиля переносится в Duration.
	DurationMS int `json:"duration_ms,omitempty"`
}

// IsZero — анимация не задана (значит, мгновенно).
func (a AnimSpec) IsZero() bool { return a.Duration == 0 && a.Curve == "" }

// BackdropMode — что находится под полупрозрачным слоем.
type BackdropMode int

const (
	// BackdropNone — слой непрозрачен, под ним ничего не видно.
	BackdropNone BackdropMode = iota
	// BackdropAlpha — слой смешивается с уже нарисованным (то, что сегодня
	// умеет Panel.UseAlpha).
	BackdropAlpha
	// BackdropBlur — под слоем размывается композированная область
	// (acrylic/mica Windows 11, материалы macOS).
	BackdropBlur
)

// BackdropSpec — описание подложки слоя.
//
// Режим BackdropBlur появляется в модели раньше, чем движок научится
// размывать: до тех пор он рисуется как BackdropAlpha с цветом Tint. Профиль
// темы пишется сразу правильно и начнёт размывать сам, когда рендерер это
// поддержит, — заменой реализации, а не правкой профилей.
type BackdropSpec struct {
	Mode   BackdropMode `json:"mode,omitempty"`
	Radius float64      `json:"radius,omitempty"` // радиус размытия в логических пикселях
	Tint   color.RGBA   `json:"-"`                // подмешиваемый цвет (alpha-premultiplied)
}

// IsZero — подложка не задана.
func (b BackdropSpec) IsZero() bool {
	return b.Mode == BackdropNone && b.Radius == 0 && b.Tint == color.RGBA{}
}

// BevelSpec — объёмная рамка Windows 2000: две светлые грани сверху-слева и
// две тёмные снизу-справа. Вынесена в тему, чтобы «классический» вид был
// данными профиля, а не веткой `if Classic3D` внутри каждого виджета.
type BevelSpec struct {
	Light  color.RGBA `json:"-"` // внешняя светлая грань
	Shadow color.RGBA `json:"-"` // внешняя тёмная грань
	Dark   color.RGBA `json:"-"` // внутренняя, самая тёмная
	Width  float64    `json:"width,omitempty"`
	// Sunken — рамка утоплена (поле ввода, дорожка прогресса), а не выпукла
	// (кнопка в покое).
	Sunken bool `json:"sunken,omitempty"`
}

// GradientKind — как разложены цвета градиента.
type GradientKind uint8

const (
	// GradientLinear — вдоль оси GradientAngle (значение по умолчанию, чтобы
	// стиль без указания вида вёл себя как раньше).
	GradientLinear GradientKind = iota
	// GradientRadial — кругом от центра к краю: подсветка под значком дока,
	// ореол под курсором. Осью такое не выразить.
	GradientRadial
)

// String — имя вида для JSON и диагностики.
func (k GradientKind) String() string {
	if k == GradientRadial {
		return "radial"
	}
	return "linear"
}

// ParseGradientKind разбирает имя вида ("linear", "radial").
func ParseGradientKind(s string) (GradientKind, bool) {
	switch s {
	case "", "linear":
		return GradientLinear, true
	case "radial":
		return GradientRadial, true
	}
	return GradientLinear, false
}

// GradientStop — точка линейного градиента: положение вдоль оси [0,1] и цвет.
// Ось задаётся углом в Style.GradientAngle.
type GradientStop struct {
	Pos   float64    `json:"pos"`
	Color color.RGBA `json:"-"`
}
