package theme

import (
	"image/color"
	"time"
)

// Key — имя плоского токена: "taskbar.height", "button.corner",
// "accent". Точки — только соглашение о читаемости, разбором имени пакет
// не занимается.
type Key string

// Веса шрифта по шкале CSS/OpenType для FontSpec.Weight. Движок подбирает
// ближайшее начертание семейства из тех, что зарегистрированы.
const (
	WeightThin     = 100
	WeightLight    = 300
	WeightRegular  = 400
	WeightMedium   = 500
	WeightSemiBold = 600
	WeightBold     = 700
	WeightBlack    = 900
)

// FlagTextSubpixel — флаг профиля: тема рассчитана на подпиксельное
// позиционирование глифов (engine.Engine.SetTextSubpixel). Профиль только
// просит; включает режим тот, кто владеет движком. Без флага шаг глифов
// округляется до целого пикселя, как у всех прежних тем.
const FlagTextSubpixel Key = "text.subpixel"

// FlagFontDefaultGlobal — флаг профиля: Fonts["default"].Size становится
// размером шрифта по умолчанию для ВСЕХ виджетов, у которых свой кегль не задан
// (заголовки окон widget.Window, диалоги, вкладки, пункты меню), а не только
// для компонентов оболочки, читающих шрифт из своих стилей.
//
// Без флага (все прежние темы) виджеты остаются на widget.DefaultFontSizePt =
// 10 pt: профиль Windows 11 объявляет Fonts["default"] = 9 pt, и применение
// профиля не должно было менять раскладку каждого окна. Флаг можно включить на
// лету: Manager.SetFlag(FlagFontDefaultGlobal, true) с последующим
// Engine.ApplyThemeProfile. Применяет его widget.ApplyGlobalTheme.
const FlagFontDefaultGlobal Key = "font.default.global"

// FontSpec — шрифт как данные темы. Имя семейства соответствует шрифту,
// зарегистрированному в движке (engine.RegisterFont, RegisterFontFS): либо
// имени шрифта («OpenSans-Bold»), либо названию семейства («Open Sans») — во
// втором случае начертание выбирается по Weight и Italic. Пустое семейство —
// шрифт по умолчанию.
type FontSpec struct {
	Family string  `json:"family,omitempty"`
	Size   float64 `json:"size,omitempty"` // в пунктах, может быть дробным (8.5); 0 — размер по умолчанию
	Bold   bool    `json:"bold,omitempty"`
	Italic bool    `json:"italic,omitempty"`
	// Weight — вес 100..900 (WeightLight, WeightSemiBold…); 0 — не задан:
	// тогда вес даёт Bold (700) либо обычное начертание. Заданный Weight
	// важнее Bold.
	Weight int `json:"weight,omitempty"`
}

// IsZero — спецификация ничего не задаёт (её можно наследовать целиком).
func (f FontSpec) IsZero() bool { return f == FontSpec{} }

// EffectiveWeight — вес, который просит спецификация: Weight, а если он не
// задан, 700 при Bold и 0 («обычный») иначе.
func (f FontSpec) EffectiveWeight() int {
	switch {
	case f.Weight > 0:
		return f.Weight
	case f.Bold:
		return WeightBold
	}
	return 0
}

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
// Режим BackdropBlur размывает композицию под слоем: движок умеет это
// (Canvas.BlurBehind), и профиль просто объявляет радиус и подкраску. Если же
// контекст рисования размывать не умеет (виджет-контекст без BackdropDrawer),
// слой рисуется как BackdropAlpha с цветом Tint — либо, когда задан Fallback,
// сплошным цветом Fallback: полупрозрачная плёнка без размытия читается
// грязным пятном, и честнее нарисовать близкий непрозрачный цвет.
type BackdropSpec struct {
	Mode   BackdropMode `json:"mode,omitempty"`
	Radius float64      `json:"radius,omitempty"` // радиус размытия в логических пикселях
	Tint   color.RGBA   `json:"-"`                // подмешиваемый цвет (alpha-premultiplied)

	// Noise — амплитуда зернистого шума поверх подкраски (acrylic Windows 10),
	// доля полной шкалы яркости: 0 — шума нет, 0.02 — ±2 % (заметно только
	// вблизи, но без него размытая подложка выглядит гладким пластиком).
	// Шум рисуется, только если контекст его умеет (widget.NoiseDrawer).
	Noise float64 `json:"noise,omitempty"`

	// Fallback — непрозрачный цвет «вместо стекла», когда размытие
	// недоступно. Прозрачный (нулевой) — запасного цвета нет, слой
	// ограничивается подкраской Tint.
	Fallback color.RGBA `json:"-"`

	// Highlight — цвет внутреннего блика по ВЕРХНЕЙ кромке слоя.
	//
	// Без него размытая подложка читается плоской заливкой: ощущение стекла
	// даёт именно светлая кромка сверху — свет, поймавшийся на скошенном
	// крае. В Figma это внутренняя тень со смещением вверх; здесь — линия
	// толщиной в точку внутри контура.
	Highlight color.RGBA `json:"-"`
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
