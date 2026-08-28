// theme_bridge.go — мост между плоской темой виджетов и профилями токенов
// из пакета theme.
//
// Старая тема (widget.Theme, семь десятков цветовых полей) остаётся: на неё
// опираются все конструкторы виджетов, ApplyTheme, шесть пресетов и код
// потребителей. Новая модель адресует цвет тройкой (компонент, часть,
// состояние). Мост переводит одно в другое в обе стороны:
//
//	ProfileFromTheme — плоская тема → профиль токенов;
//	Materialize      — разрешённая тема → плоская тема.
//
// Обе стороны ходят по ОДНОЙ таблице привязок (theme_bindings.go). Это не
// экономия строк, а гарантия: разойтись прямому и обратному переводу
// попросту негде, и тест round-trip по всем шести пресетам это стережёт.
package widget

import (
	"image/color"

	"github.com/oops1/headless-gui/v3/theme"
)

// styleRole — какое поле стиля занимает цвет.
type styleRole int

const (
	roleFill styleRole = iota
	roleText
	roleBorder
	roleShadow
)

// legacyBinding — одна привязка поля плоской темы к адресу в модели токенов.
//
// Адреса два вида. Цвет, который принадлежит компоненту в определённом
// состоянии (BtnHoverBG — заливка кнопки при наведении), живёт в Styles.
// Цвет сквозной, который читают полтора десятка не связанных виджетов
// (Accent), живёт плоским токеном: разложить его по компонентам значило бы
// продублировать одно значение пятнадцать раз и потерять смысл «акцент у
// темы один».
type legacyBinding struct {
	// name — имя поля в widget.Theme; служит для диагностики и для теста
	// полноты покрытия.
	name string
	// field возвращает адрес поля в плоской теме.
	field func(*Theme) *color.RGBA

	// Заполнено ровно одно из двух: либо style+role, либо key.
	style theme.StyleKey
	role  styleRole
	key   theme.Key
}

// isFlat — привязка адресует плоский токен, а не стиль компонента.
func (b legacyBinding) isFlat() bool { return b.style.Component == "" }

// ─── Метрики и флаги ────────────────────────────────────────────────────────

// Ключи токенов, в которые переезжает ThemeStyle. Радиусы — метрики: один
// «радиус контролов» на тему нагляднее, чем одно и то же число, повторённое
// в стиле каждого компонента.
const (
	keyControlCorner theme.Key = "control.corner"
	keyWindowCorner  theme.Key = "window.corner"

	// Флаги вида темы. По плану им место в презентерах — «классический»
	// вид должен быть отдельной отрисовкой, а не веткой if внутри каждого
	// виджета. Презентеры появятся позже; до тех пор флаг переносится как
	// флаг, чтобы мост уже был обратимым.
	keyClassic3D   theme.Key = "style.classic3d"
	keyMacTitleBar theme.Key = "style.mac.titlebar"

	// Фаска — общий вид темы, а не свойство компонента: одну и ту же
	// тройку цветов рисуют кнопка, флажок, вкладки, меню и окно.
	keyBevelLight  theme.Key = "bevel.light"
	keyBevelShadow theme.Key = "bevel.shadow"
	keyBevelDark   theme.Key = "bevel.dark"

	// Имя пресета, из которого собран профиль: без него обратный перевод
	// терял бы ThemeStyle.Name, а по нему виджеты узнают вид темы.
	keyStyleName theme.Key = "style.name"
)

// ─── Плоская тема → профиль ─────────────────────────────────────────────────

// ProfileFromTheme собирает профиль токенов из плоской темы.
//
// Нужен, чтобы шесть готовых пресетов стали профилями без переписывания
// палитры вручную: тема, собранная сегодня в коде, немедленно доступна
// новой модели. Имя профиля берётся из ThemeStyle.Name, если оно задано.
func ProfileFromTheme(t *Theme) *theme.Profile {
	if t == nil {
		return theme.NewProfile("")
	}
	name := t.Style.Name
	if name == "" {
		name = "Безымянная"
	}
	p := theme.NewProfile(name)

	// Цвета: каждый по своему адресу из таблицы.
	styles := map[theme.StyleKey]theme.StyleDelta{}
	for _, b := range legacyBindings {
		c := *b.field(t)
		if b.isFlat() {
			p.Colors[b.key] = c
			continue
		}
		d := styles[b.style]
		switch b.role {
		case roleFill:
			d.Fill = theme.C(c)
		case roleText:
			d.Text = theme.C(c)
		case roleBorder:
			d.Border = theme.C(c)
		case roleShadow:
			d.Shadow = theme.C(c)
		}
		styles[b.style] = d
	}
	for k, d := range styles {
		p.Styles[k] = d
	}

	// Форма и вид.
	p.SetMetric(keyControlCorner, float64(t.Style.ControlCorner))
	p.SetMetric(keyWindowCorner, float64(t.Style.WindowCorner))
	p.SetFlag(keyClassic3D, t.Style.Classic3D)
	p.SetFlag(keyMacTitleBar, t.Style.MacTitleBar)
	p.SetColor(keyBevelLight, t.Style.BevelLight)
	p.SetColor(keyBevelShadow, t.Style.BevelShadow)
	p.SetColor(keyBevelDark, t.Style.BevelDark)
	p.SetColor(keyStyleName, encodeName(t.Style.Name))

	// Радиус окна принадлежит окну — объявляем и стилем, чтобы профиль
	// читался осмысленно тем, кто пишет тему руками.
	p.SetStyle("window", "", theme.StateNormal, mergeCorner(p.Styles[theme.StyleKey{Component: "window"}],
		float64(t.Style.WindowCorner)))

	return p
}

// mergeCorner дописывает радиус в уже собранную дельту стиля.
func mergeCorner(d theme.StyleDelta, corner float64) theme.StyleDelta {
	d.Corner = theme.N(corner)
	return d
}

// ─── Профиль → плоская тема ─────────────────────────────────────────────────

// Materialize собирает плоскую тему из разрешённой темы токенов.
//
// Это и есть обещание совместимости: код, который знает только
// widget.Theme, получает её из профиля и не замечает, что палитра теперь
// живёт в другой модели.
func Materialize(rt *theme.Theme) *Theme {
	t := &Theme{}
	if rt == nil {
		return t
	}

	for _, b := range legacyBindings {
		dst := b.field(t)
		if b.isFlat() {
			if c, ok := rt.Color(b.key); ok {
				*dst = c
			}
			continue
		}
		s := rt.Style(b.style.Component, b.style.Part, b.style.State)
		switch b.role {
		case roleFill:
			*dst = s.Fill
		case roleText:
			*dst = s.Text
		case roleBorder:
			*dst = s.Border
		case roleShadow:
			*dst = s.Shadow
		}
	}

	t.Style = ThemeStyle{
		Name:          decodeName(rt),
		ControlCorner: int(rt.MetricOr(keyControlCorner, 0)),
		WindowCorner:  int(rt.MetricOr(keyWindowCorner, 0)),
		Classic3D:     rt.FlagOr(keyClassic3D, false),
		MacTitleBar:   rt.FlagOr(keyMacTitleBar, false),
		BevelLight:    rt.ColorOr(keyBevelLight, color.RGBA{}),
		BevelShadow:   rt.ColorOr(keyBevelShadow, color.RGBA{}),
		BevelDark:     rt.ColorOr(keyBevelDark, color.RGBA{}),
	}
	return t
}

// ─── Имя вида темы ──────────────────────────────────────────────────────────
//
// Имя — строка, а плоских строковых токенов в модели нет: цвета, числа и
// флаги покрывают всё, что нужно отрисовке, и заводить ради имени отдельную
// карту значило бы усложнять модель ради моста. Поэтому имя профиля и есть
// имя вида: Materialize берёт его у темы. Отдельный цветовой токен хранит
// признак «имя пустое» — иначе безымянный пресет после перевода туда-обратно
// получал бы имя профиля, которого у него не было.

func encodeName(name string) color.RGBA {
	if name == "" {
		return color.RGBA{} // прозрачный — имени не было
	}
	return color.RGBA{A: 255}
}

func decodeName(rt *theme.Theme) string {
	if c, ok := rt.Color(keyStyleName); ok && c.A == 0 {
		return ""
	}
	return rt.Name()
}
