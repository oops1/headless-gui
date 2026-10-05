package theme

import "image/color"

// StyleKey адресует стиль: компонент, часть внутри него и состояние.
//
// Part — уточнение («item» у меню, «titlebar» у окна, «thumb» у полосы
// прокрутки). Пустая часть описывает компонент целиком и служит запасным
// вариантом для любой его части, стиль которой профиль не объявил.
type StyleKey struct {
	Component string
	Part      string
	State     State
}

// Profile — тема как данные: набор токенов плюс правила по компонентам.
// Профиль ничего не разрешает и ничего не рисует — он только описывает.
//
// Parent — имя профиля, у которого берётся всё незаявленное. Благодаря
// этому тёмная разновидность темы состоит из десятка цветов, а не из
// копии всей палитры.
type Profile struct {
	Name   string `json:"name"`
	Parent string `json:"parent,omitempty"`

	Colors  map[Key]color.RGBA `json:"-"`
	Metrics map[Key]float64    `json:"metrics,omitempty"`
	Flags   map[Key]bool       `json:"flags,omitempty"`
	Fonts   map[Key]FontSpec   `json:"fonts,omitempty"`
	Icons   map[Key]IconRef    `json:"icons,omitempty"`
	Anims   map[Key]AnimSpec   `json:"anims,omitempty"`

	Styles map[StyleKey]StyleDelta `json:"-"`

	// Conditional — правила стиля, которые действуют, только пока в
	// разрешённой теме поднят флаг (см. SetStyleWhen). Так «светлая панель
	// задач» остаётся данными профиля, а не веткой в компоненте: флаг
	// включает тот же набор стилей, и менеджер умеет переключать его на лету.
	Conditional []ConditionalStyle `json:"-"`

	// StyleBase — «компонент → компонент, у которого берутся стили»: запись
	// {"notificationcenter": "notifications"} значит, что стиль
	// notificationcenter начинается со всех правил notifications, а свои
	// правила ложатся поверх. Позволяет завести новое имя компонента, не
	// меняя вид существующих тем (см. SetStyleBase).
	StyleBase map[string]string `json:"-"`

	// ColorFrom — «цветовой токен → токен, за которым он следует при смене
	// акцента» (см. SetColorFrom).
	ColorFrom map[Key]Key `json:"-"`

	// Presenters — имена презентеров, которыми профиль подменяет отрисовку
	// компонента целиком: macOS рисует область приложений не полосой
	// кнопок, а Dock, и одной палитрой это не выражается. Значение —
	// имя, зарегистрированное через RegisterPresenter.
	Presenters map[string]string `json:"presenters,omitempty"`
}

// NewProfile создаёт пустой профиль с готовыми картами.
func NewProfile(name string) *Profile {
	return &Profile{
		Name:       name,
		Colors:     map[Key]color.RGBA{},
		Metrics:    map[Key]float64{},
		Flags:      map[Key]bool{},
		Fonts:      map[Key]FontSpec{},
		Icons:      map[Key]IconRef{},
		Anims:      map[Key]AnimSpec{},
		Styles:     map[StyleKey]StyleDelta{},
		Presenters: map[string]string{},
	}
}

// SetStyle объявляет правило для (компонент, часть, состояние).
// Пустая часть — правило для компонента целиком.
func (p *Profile) SetStyle(component, part string, state State, d StyleDelta) *Profile {
	if p.Styles == nil {
		p.Styles = map[StyleKey]StyleDelta{}
	}
	p.Styles[StyleKey{Component: component, Part: part, State: state.Dominant()}] = d
	return p
}

// ConditionalStyle — правило стиля, зависящее от флага темы.
type ConditionalStyle struct {
	// Flag — имя флага; правило действует, пока флаг истинен.
	Flag  Key
	Key   StyleKey
	Delta StyleDelta
}

// SetStyleWhen объявляет правило, действующее только при поднятом флаге.
//
// Правило накладывается ПОВЕРХ обычных правил того же профиля (и всех
// предков), поэтому достаточно перечислить то, что флаг меняет: светлая
// панель задач переписывает подкраску и цвет текста, а геометрию, шрифты и
// состояния берёт у обычных стилей. Флаг ставит профиль (SetFlag) либо
// приложение на лету (Manager.SetFlag).
func (p *Profile) SetStyleWhen(flag Key, component, part string, state State, d StyleDelta) *Profile {
	p.Conditional = append(p.Conditional, ConditionalStyle{
		Flag:  flag,
		Key:   StyleKey{Component: component, Part: part, State: state.Dominant()},
		Delta: d,
	})
	return p
}

// SetStyleBase делает стили компонента component продолжением стилей
// компонента base: всё, что объявлено для base (в этом профиле и у
// предков), действует и для component, а собственные правила component
// перекрывают унаследованные.
//
// Нужно, чтобы вводить новые имена компонентов без визуальных последствий:
// пока профиль ничего не объявил для нового имени, оно выглядит как старое.
func (p *Profile) SetStyleBase(component, base string) *Profile {
	if p.StyleBase == nil {
		p.StyleBase = map[string]string{}
	}
	p.StyleBase[component] = base
	return p
}

// SetColorFrom заставляет цветовой токен k следовать за токеном from, когда
// приложение меняет акцент (Manager.SetAccent): второй цвет градиента
// заголовка Windows 2000 — светлый оттенок акцента, а не зашитый голубой.
//
// Без SetAccent действует значение, объявленное SetColor (классический
// голубой градиент остаётся побитно прежним); если токен профиль не
// объявлял вовсе, он берётся у from и без SetAccent. После SetAccent ссылка
// сильнее объявленного значения — по той же причине, по какой производные
// акцента считаются заново: цвет от прежнего акцента на новом выглядел бы
// чужим. ResetAccent возвращает значение профиля.
func (p *Profile) SetColorFrom(k, from Key) *Profile {
	if p.ColorFrom == nil {
		p.ColorFrom = map[Key]Key{}
	}
	p.ColorFrom[k] = from
	return p
}

// SetColor, SetMetric, SetFlag — плоские токены. Возвращают профиль,
// чтобы объявление темы читалось цепочкой.
func (p *Profile) SetColor(k Key, c color.RGBA) *Profile {
	if p.Colors == nil {
		p.Colors = map[Key]color.RGBA{}
	}
	p.Colors[k] = c
	return p
}

func (p *Profile) SetMetric(k Key, v float64) *Profile {
	if p.Metrics == nil {
		p.Metrics = map[Key]float64{}
	}
	p.Metrics[k] = v
	return p
}

func (p *Profile) SetFlag(k Key, v bool) *Profile {
	if p.Flags == nil {
		p.Flags = map[Key]bool{}
	}
	p.Flags[k] = v
	return p
}

// TokenCount — сколько собственных токенов объявляет профиль. Метрика
// «дочерний профиль должен быть коротким»: тёмная разновидность темы,
// переписывающая половину палитры, — признак того, что общее не вынесено
// в родителя.
func (p *Profile) TokenCount() int {
	return len(p.Colors) + len(p.Metrics) + len(p.Flags) +
		len(p.Fonts) + len(p.Icons) + len(p.Anims) +
		len(p.Styles) + len(p.Presenters) +
		len(p.Conditional) + len(p.StyleBase) + len(p.ColorFrom)
}
