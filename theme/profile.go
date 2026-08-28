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
		len(p.Styles) + len(p.Presenters)
}
