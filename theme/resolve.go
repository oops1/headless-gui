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
//
// ov — переопределения приложения поверх профилей (акцент, флаги); нулевое
// значение — «ничего не переопределено».
func resolve(name string, byName map[string]*Profile, ov overrides) (*Theme, error) {
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

	// ── Переопределения приложения: флаги и акцент ───────────────────────
	for k, v := range ov.flags {
		t.flags[k] = v
	}
	t.applyAccent(ov.accent)
	t.applyColorFrom(chain, ov.accent != nil)

	// ── Стили: слияние дельт по цепочке ─────────────────────────────────
	// merged[ключ] — накопленные дельты предков и потомков в порядке цепочки.
	merged := map[StyleKey][]StyleDelta{}
	bases := map[string]string{}
	for _, p := range chain {
		for k, d := range p.Styles {
			k.State = k.State.Dominant()
			merged[k] = append(merged[k], d)
		}
		// Условные правила профиля идут после его обычных: флаг уточняет
		// то, что профиль уже сказал, а не заменяет его.
		for _, c := range p.Conditional {
			if !t.flags[c.Flag] {
				continue
			}
			k := c.Key
			k.State = k.State.Dominant()
			merged[k] = append(merged[k], c.Delta)
		}
		for comp, base := range p.StyleBase {
			bases[comp] = base
		}
	}
	applyStyleBases(merged, bases)
	applyPartFallbacks(merged)

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
		applyAll(normal, t.colors, merged[StyleKey{cp.component, cp.part, StateNormal}])
		// Часть наследует стиль компонента целиком, если он объявлен.
		if cp.part != "" {
			whole := base.Clone()
			applyAll(whole, t.colors, merged[StyleKey{cp.component, "", StateNormal}])
			applyAll(whole, t.colors, merged[StyleKey{cp.component, cp.part, StateNormal}])
			normal = whole
		}
		t.styles[StyleKey{cp.component, cp.part, StateNormal}] = normal

		for _, st := range statePriority {
			s := normal.Clone()
			if cp.part != "" {
				applyAll(s, t.colors, merged[StyleKey{cp.component, "", st}])
			}
			applyAll(s, t.colors, merged[StyleKey{cp.component, cp.part, st}])
			t.styles[StyleKey{cp.component, cp.part, st}] = s
		}
	}

	return t, nil
}

func applyAll(s *Style, colors map[Key]color.RGBA, deltas []StyleDelta) {
	for i := range deltas {
		deltas[i].applyTo(s)
		deltas[i].applyTokens(s, colors)
	}
}

// overrides — то, что приложение накладывает поверх профилей: акцент и флаги
// (Manager.SetAccent, Manager.SetFlag). Переживает смену темы: пользователь
// выбирает акцент для оболочки, а не для одного профиля.
type overrides struct {
	accent *color.RGBA
	flags  map[Key]bool
}

// applyAccent доводит токены акцента до итогового вида.
//
// С переопределением производные считаются заново от него, а значения,
// объявленные профилем, отбрасываются: наведение от старого акцента на новом
// выглядело бы чужим. Без переопределения профиль хозяин своих токенов, и
// считаются только недостающие производные. Выделение по умолчанию равно
// акценту — профилю не нужно объявлять его отдельно, чтобы оно следовало за
// сменой.
func (t *Theme) applyAccent(override *color.RGBA) {
	if override != nil {
		for k, c := range DeriveAccent(*override).tokens() {
			t.colors[k] = c
		}
		// Выделение, объявленное профилем, остаётся его решением.
		if _, own := t.colors[KeySelection]; !own {
			t.colors[KeySelection] = t.colors[KeyAccent]
		}
		return
	}
	base, ok := t.colors[KeyAccent]
	if !ok {
		return
	}
	for k, c := range DeriveAccent(base).tokens() {
		if _, declared := t.colors[k]; !declared {
			t.colors[k] = c
		}
	}
	if _, declared := t.colors[KeySelection]; !declared {
		t.colors[KeySelection] = t.colors[KeyAccent]
	}
}

// applyColorFrom раскрывает ссылки «токен следует за токеном»
// (Profile.SetColorFrom) по цепочке профилей: ссылка потомка перекрывает
// ссылку предка. Идёт после applyAccent — производные акцента уже посчитаны.
//
// С переопределённым акцентом ссылка сильнее объявленного значения; без
// него — только заполняет необъявленный токен. Токен-источник, которого в
// теме нет, ссылку отменяет.
func (t *Theme) applyColorFrom(chain []*Profile, accentOverridden bool) {
	var follow map[Key]Key
	for _, p := range chain {
		for k, from := range p.ColorFrom {
			if follow == nil {
				follow = map[Key]Key{}
			}
			follow[k] = from
		}
	}
	for k, from := range follow {
		src, ok := t.colors[from]
		if !ok {
			continue
		}
		if _, declared := t.colors[k]; declared && !accentOverridden {
			continue
		}
		t.colors[k] = src
	}
}

// applyStyleBases дописывает правила базовых компонентов перед правилами
// производных (см. Profile.SetStyleBase).
func applyStyleBases(merged map[StyleKey][]StyleDelta, bases map[string]string) {
	if len(bases) == 0 {
		return
	}
	// Снимок ключей: карта дополняется по ходу.
	keys := make([]StyleKey, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	for comp, base := range bases {
		for _, k := range keys {
			if k.Component != base {
				continue
			}
			dk := k
			dk.Component = comp
			// Правила базы идут первыми: свои правила компонента перекрывают.
			merged[dk] = append(append([]StyleDelta(nil), merged[k]...), merged[dk]...)
		}
	}
}

// partFallbacks — части, которые по умолчанию повторяют другую часть той же
// темы, пока ни один профиль цепочки не объявил их сам.
//
// Заголовок диалога — это заголовок окна: диалог и есть окно. Раньше каждый
// профиль копировал себе заголовок окна под именем диалога, и тёмные
// разновидности, наследующие светлые, раздувались лишним токеном на каждую
// такую копию. Повтор на уровне разрешения берёт заголовок окна той темы,
// что собирается, — с поправками всех профилей цепочки.
var partFallbacks = []struct{ to, from StyleKey }{
	{
		to:   StyleKey{Component: "dialog", Part: "titlebar", State: StateNormal},
		from: StyleKey{Component: "window", Part: "titlebar", State: StateFocused},
	},
}

// applyPartFallbacks дописывает необъявленным частям правила их образцов.
func applyPartFallbacks(merged map[StyleKey][]StyleDelta) {
	for _, f := range partFallbacks {
		if len(merged[f.to]) > 0 {
			continue
		}
		if src := merged[f.from]; len(src) > 0 {
			merged[f.to] = append([]StyleDelta(nil), src...)
		}
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
