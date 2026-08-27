package theme

import (
	"fmt"
	"image/color"
)

// Theme — разрешённый профиль: цепочка наследования уже пройдена, откаты по
// состояниям посчитаны, всё сведено в плоские таблицы.
//
// Тема неизменяема после сборки. Style/Metric/Color читают её без
// блокировок и без аллокаций — это горячий путь отрисовки.
type Theme struct {
	name  string
	chain []string // цепочка профилей от корня к листу — для диагностики

	styles map[StyleKey]*Style

	colors  map[Key]color.RGBA
	metrics map[Key]float64
	flags   map[Key]bool
	fonts   map[Key]FontSpec
	icons   map[Key]IconRef
	anims   map[Key]AnimSpec

	presenters map[string]string

	// fallback — стиль для компонента, о котором тема не знает ничего.
	// Возвращается указателем, поэтому промах не аллоцирует.
	fallback *Style
}

// Name возвращает имя темы (имя профиля-листа).
func (t *Theme) Name() string { return t.name }

// Chain возвращает цепочку наследования от корня к листу: полезно в
// сообщениях об ошибках и в тестах на структуру темы.
func (t *Theme) Chain() []string { return append([]string(nil), t.chain...) }

// Style возвращает стиль (компонент, часть, состояние).
//
// Поиск идёт по уже разрешённой таблице и не аллоцирует: набор состояний
// сводится к доминирующему (см. State.Dominant), незаявленная часть
// откатывается к компоненту целиком, неизвестный компонент — к встроенному
// стилю по умолчанию. Возвращённый указатель ОБЩИЙ — менять его нельзя.
func (t *Theme) Style(component, part string, state State) *Style {
	st := state.Dominant()
	if s, ok := t.styles[StyleKey{component, part, st}]; ok {
		return s
	}
	// Часть не объявлена — берём стиль компонента целиком.
	if part != "" {
		if s, ok := t.styles[StyleKey{component, "", st}]; ok {
			return s
		}
	}
	return t.fallback
}

// Color/Metric/Flag/Font/Icon/Anim — плоские токены. Второе значение
// сообщает, объявлен ли токен: ноль как «не задано» и ноль как «задано
// нулём» — разные вещи (нулевая длительность анимации значит «мгновенно»,
// а не «возьми умолчание»).
func (t *Theme) Color(k Key) (color.RGBA, bool) { v, ok := t.colors[k]; return v, ok }
func (t *Theme) Metric(k Key) (float64, bool)   { v, ok := t.metrics[k]; return v, ok }
func (t *Theme) Flag(k Key) (bool, bool)        { v, ok := t.flags[k]; return v, ok }
func (t *Theme) Font(k Key) (FontSpec, bool)    { v, ok := t.fonts[k]; return v, ok }
func (t *Theme) Icon(k Key) (IconRef, bool)     { v, ok := t.icons[k]; return v, ok }
func (t *Theme) Anim(k Key) (AnimSpec, bool)    { v, ok := t.anims[k]; return v, ok }

// ColorOr/MetricOr/FlagOr — то же с запасным значением, когда вызывающему
// нечего делать с фактом отсутствия.
func (t *Theme) ColorOr(k Key, def color.RGBA) color.RGBA {
	if v, ok := t.colors[k]; ok {
		return v
	}
	return def
}

func (t *Theme) MetricOr(k Key, def float64) float64 {
	if v, ok := t.metrics[k]; ok {
		return v
	}
	return def
}

func (t *Theme) FlagOr(k Key, def bool) bool {
	if v, ok := t.flags[k]; ok {
		return v
	}
	return def
}

// PresenterName возвращает имя презентера, которым тема подменяет отрисовку
// компонента ("" — рисовать презентером по умолчанию).
func (t *Theme) PresenterName(component string) string { return t.presenters[component] }

// ─── Разрешение ─────────────────────────────────────────────────────────────

// resolveOrder возвращает цепочку профилей от корня к листу.
// Ошибка — если родитель не зарегистрирован или цепочка зациклена.
func resolveOrder(name string, byName map[string]*Profile) ([]*Profile, error) {
	var chain []*Profile
	seen := map[string]bool{}
	for cur := name; cur != ""; {
		if seen[cur] {
			return nil, fmt.Errorf("theme: цикл наследования на профиле %q", cur)
		}
		seen[cur] = true
		p, ok := byName[cur]
		if !ok {
			return nil, fmt.Errorf("theme: профиль %q не зарегистрирован", cur)
		}
		chain = append(chain, p)
		cur = p.Parent
	}
	// Собрали от листа к корню — разворачиваем.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// resolve собирает тему из профиля и его предков.
//
// Разрешение выполняется ОДИН РАЗ, при смене темы: дальше отрисовка только
// читает готовые таблицы. Здесь же раскрываются откаты по состояниям —
// каждому объявленному (компонент, часть) заполняются все шесть состояний,
// чтобы поиск во время отрисовки был одним обращением к карте.
func resolve(name string, byName map[string]*Profile) (*Theme, error) {
	chain, err := resolveOrder(name, byName)
	if err != nil {
		return nil, err
	}

	t := &Theme{
		name:       name,
		styles:     map[StyleKey]*Style{},
		colors:     map[Key]color.RGBA{},
		metrics:    map[Key]float64{},
		flags:      map[Key]bool{},
		fonts:      map[Key]FontSpec{},
		icons:      map[Key]IconRef{},
		anims:      map[Key]AnimSpec{},
		presenters: map[string]string{},
	}
	for _, p := range chain {
		t.chain = append(t.chain, p.Name)
	}

	// ── Плоские токены: потомок переписывает предка ─────────────────────
	for _, p := range chain {
		for k, v := range p.Colors {
			t.colors[k] = v
		}
		for k, v := range p.Metrics {
			t.metrics[k] = v
		}
		for k, v := range p.Flags {
			t.flags[k] = v
		}
		for k, v := range p.Fonts {
			t.fonts[k] = v
		}
		for k, v := range p.Icons {
			t.icons[k] = v
		}
		for k, v := range p.Anims {
			t.anims[k] = v
		}
		for comp, presenter := range p.Presenters {
			t.presenters[comp] = presenter
		}
	}

	// ── Стили: слияние дельт по цепочке ─────────────────────────────────
	// merged[ключ] — накопленные дельты предков и потомков в порядке цепочки.
	merged := map[StyleKey][]StyleDelta{}
	for _, p := range chain {
		for k, d := range p.Styles {
			k.State = k.State.Dominant()
			merged[k] = append(merged[k], d)
		}
	}

	// Пары (компонент, часть), о которых тема вообще что-то знает.
	type compPart struct{ component, part string }
	parts := map[compPart]bool{}
	for k := range merged {
		parts[compPart{k.Component, k.Part}] = true
	}

	base := defaultStyle(t)
	t.fallback = base

	// Для каждой пары заполняем ВСЕ состояния: сначала покой (от него
	// наследуются остальные), затем прочие — каждое поверх покоя.
	for cp := range parts {
		normal := base.Clone()
		applyAll(normal, merged[StyleKey{cp.component, cp.part, StateNormal}])
		// Часть наследует стиль компонента целиком, если он объявлен.
		if cp.part != "" {
			whole := base.Clone()
			applyAll(whole, merged[StyleKey{cp.component, "", StateNormal}])
			applyAll(whole, merged[StyleKey{cp.component, cp.part, StateNormal}])
			normal = whole
		}
		t.styles[StyleKey{cp.component, cp.part, StateNormal}] = normal

		for _, st := range statePriority {
			s := normal.Clone()
			if cp.part != "" {
				applyAll(s, merged[StyleKey{cp.component, "", st}])
			}
			applyAll(s, merged[StyleKey{cp.component, cp.part, st}])
			t.styles[StyleKey{cp.component, cp.part, st}] = s
		}
	}

	return t, nil
}

func applyAll(s *Style, deltas []StyleDelta) {
	for i := range deltas {
		deltas[i].applyTo(s)
	}
}

// defaultStyle — встроенные значения, с которых начинается любой стиль.
// Берутся из плоских токенов темы, если та их объявила: так профиль задаёт
// «общий вид» одной строкой, не перечисляя каждый компонент.
func defaultStyle(t *Theme) *Style {
	s := &Style{
		Fill:   t.ColorOr("surface", color.RGBA{}),
		Text:   t.ColorOr("text", RGB(0, 0, 0)),
		Border: t.ColorOr("border", color.RGBA{}),
		Corner: t.MetricOr("control.corner", 0),
		PadX:   t.MetricOr("control.pad.x", 0),
		PadY:   t.MetricOr("control.pad.y", 0),
	}
	if f, ok := t.fonts["default"]; ok {
		s.Font = f
	}
	return s
}
