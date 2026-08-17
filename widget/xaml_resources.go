// xaml_resources.go — WPF Resources, Styles и markup-расширения (P0).
//
// Реализовано как пред-обработка дерева xElement ДО построения виджетов:
//   1. collect  — собирает ResourceDictionary / <X.Resources>: скалярные
//                 ресурсы (цвета/строки/числа) по x:Key и стили <Style>.
//   2. process  — резолвит markup в атрибутах ({StaticResource},
//                 {DynamicResource}, {Binding}, {x:Null}) и применяет стили
//                 (Setter'ы вливаются в атрибуты элемента как значения
//                 по умолчанию — явные атрибуты имеют приоритет).
//
// Такой подход не требует менять сигнатуры ~25 построителей: существующие
// билдеры читают уже подготовленные атрибуты через el.attr(...).
//
// Поддержано:
//   - <ResourceDictionary>, <Window.Resources>, <Grid.Resources> и т.п.
//   - x:Key, {StaticResource key}, {DynamicResource key} (как статический)
//   - <Style TargetType="Button" x:Key="..."> с <Setter Property Value>
//   - неявные стили (Style без ключа применяется ко всем элементам типа)
//   - BasedOn="{StaticResource baseKey}"
//   - {Binding Path} — однонаправленно от DataContext на момент загрузки
//   - {x:Null}
//
// Не входит в этот этап (следующие итерации): ControlTemplate/DataTemplate,
// триггеры, TwoWay/live-биндинг с INotifyPropertyChanged, конвертеры.
package widget

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"

	dgridPkg "github.com/oops1/headless-gui/v3/widget/datagrid"
)

// styleTrigger — разобранный триггер (<Trigger>/<DataTrigger>/<MultiTrigger>/
// <MultiDataTrigger>) внутри <Style.Triggers>.
type styleTrigger struct {
	conds   []triggerCond     // условия (AND); одно у простого триггера
	setters map[string]string // применяемые при срабатывании Property→Value
}

// xamlStyle — разобранный <Style>.
type xamlStyle struct {
	target   string            // TargetType (lowercased, без namespace)
	key      string            // x:Key (или "")
	basedOn  string            // ключ BasedOn (или "")
	setters  map[string]string // Property → Value
	triggers []styleTrigger    // <Style.Triggers> (DataTrigger)
}

// xamlEnv — собранные ресурсы и стили одного XAML-документа.
type xamlEnv struct {
	scalars        map[string]string     // x:Key → скалярное значение (цвет/строка/число)
	keyedStyles    map[string]*xamlStyle // x:Key → стиль
	implicitStyles map[string]*xamlStyle // TargetType → неявный стиль
	templates      map[string]*xElement  // x:Key → <ControlTemplate>

	bindings []pendingBinding // собранные {Binding} для живой привязки
	triggers []pendingTrigger // собранные DataTrigger для динамических сеттеров
	items    []pendingItems   // ItemsControl'ы для живого перестроения
	virtuals []pendingVirtual // VirtualizingItemsControl'ы (виртуализация)
	locs     []pendingLoc     // {Loc Key} — локализованные строки (динамические)
	nameSeq  int              // счётчик авто-имён для безымянных элементов

	// inItemTemplate=true внутри клонов DataTemplate: биндинги резолвятся
	// одноразово против элемента коллекции и НЕ регистрируются как живые.
	inItemTemplate bool
}

// preprocessXAML выполняет пред-обработку дерева: ресурсы, стили, markup,
// и СОБИРАЕТ {Binding}-выражения для последующей живой привязки.
// ctx (DataContext) используется для начального резолва {Binding}; может быть nil.
// Возвращает env (с собранными привязками/триггерами/ItemsControl'ами).
func preprocessXAML(root *xElement, ctx interface{}) *xamlEnv {
	env := &xamlEnv{
		scalars:        map[string]string{},
		keyedStyles:    map[string]*xamlStyle{},
		implicitStyles: map[string]*xamlStyle{},
		templates:      map[string]*xElement{},
	}
	env.collect(root, 0)
	env.process(root, ctx)
	return env
}

// ─── Защита рекурсивных обходов (SEC-7) ─────────────────────────────────────
//
// collect / process / cloneXElement обходят дерево рекурсивно. Парсер уже
// держит глубину ≤ maxXAMLDepth, но эти же функции работают и с деревьями,
// собранными в обход parseXAML (клоны DataTemplate из xaml_binding.go), —
// поэтому предел проверяется и здесь. Превышение не ошибка, а обрезка:
// пред-обработка не критична для корректности, а падать процессом нельзя.

// logDeepOnce сообщает о срабатывании предела один раз на процесс: обход
// пойдёт по множеству узлов, и без этого журнал утонет в повторах.
var deepLogOnce sync.Once

func logTooDeep(what string) {
	deepLogOnce.Do(func() {
		log.Printf("xaml: %s: превышена предельная вложенность %d — поддерево пропущено", what, maxXAMLDepth)
	})
}

// ensureName возвращает имя элемента, генерируя его при отсутствии (и записывая
// в атрибуты, чтобы билдер зарегистрировал виджет в registry для привязки).
func (env *xamlEnv) ensureName(el *xElement) string {
	if n := el.name(); n != "" {
		return n
	}
	env.nameSeq++
	n := fmt.Sprintf("__bind%d", env.nameSeq)
	el.attrs["Name"] = n
	return n
}

// ─── Сбор ресурсов ──────────────────────────────────────────────────────────

func isResourceContainer(tag string) bool {
	return strings.HasSuffix(tag, ".resources") || tag == "resourcedictionary"
}

// isDataGridColumnTag сообщает, что тег — определение колонки DataGrid
// (их Binding разбирает сам DataGrid, пред-пасс не должен их трогать).
func isDataGridColumnTag(tag string) bool {
	return strings.HasPrefix(tag, "datagrid") && strings.HasSuffix(tag, "column")
}

func (env *xamlEnv) collect(el *xElement, depth int) {
	if depth > maxXAMLDepth {
		logTooDeep("collect")
		return
	}
	if isResourceContainer(strings.ToLower(el.Tag)) {
		for i := range el.Children {
			env.addResource(&el.Children[i], depth+1)
		}
	}
	for i := range el.Children {
		env.collect(&el.Children[i], depth+1)
	}
}

// addResource регистрирует один элемент-ресурс (скаляр или стиль).
func (env *xamlEnv) addResource(el *xElement, depth int) {
	if depth > maxXAMLDepth {
		logTooDeep("addResource")
		return
	}
	tag := strings.ToLower(el.Tag)
	switch {
	case tag == "resourcedictionary":
		// Вложенный/слитый словарь.
		for i := range el.Children {
			env.addResource(&el.Children[i], depth+1)
		}
		return
	case tag == "style":
		env.addStyle(el)
		return
	case tag == "controltemplate":
		if key := el.attr("x:Key", "Key"); key != "" {
			clone := cloneXElement(el)
			env.templates[key] = &clone
		}
		return
	}
	key := el.attr("x:Key", "Key")
	if key == "" {
		return
	}
	if v := scalarResourceValue(el); v != "" {
		env.scalars[key] = v
	}
}

// scalarResourceValue извлекает строковое значение скалярного ресурса:
// <SolidColorBrush Color="#FF0000"/>, <Color>#FF0000</Color>,
// <sys:String>Hi</sys:String>, <sys:Double>20</sys:Double>.
func scalarResourceValue(el *xElement) string {
	if c := el.attr("Color"); c != "" {
		return c
	}
	if v := strings.TrimSpace(el.Text); v != "" {
		return v
	}
	if v := el.attr("Value"); v != "" {
		return v
	}
	return ""
}

// addStyle разбирает <Style> и регистрирует его.
func (env *xamlEnv) addStyle(el *xElement) {
	s := &xamlStyle{
		target:  strings.ToLower(stripTypeName(el.attr("TargetType"))),
		key:     el.attr("x:Key", "Key"),
		basedOn: extractKey(el.attr("BasedOn")),
		setters: map[string]string{},
	}
	collectSetters(el, s)
	collectStyleTriggers(el, s)
	if s.key != "" {
		env.keyedStyles[s.key] = s
	}
	if s.target != "" && s.key == "" {
		env.implicitStyles[s.target] = s
	}
}

// collectStyleTriggers разбирает <Style.Triggers> → DataTrigger.
func collectStyleTriggers(styleEl *xElement, s *xamlStyle) {
	for i := range styleEl.Children {
		c := &styleEl.Children[i]
		if !strings.HasSuffix(strings.ToLower(c.Tag), ".triggers") {
			continue
		}
		for j := range c.Children {
			t := &c.Children[j]
			tr := styleTrigger{setters: map[string]string{}}
			switch strings.ToLower(t.Tag) {
			case "datatrigger":
				tr.conds = []triggerCond{{path: parseBindingPath(t.attr("Binding")), value: t.attr("Value")}}
			case "trigger":
				tr.conds = []triggerCond{{property: stripPropName(t.attr("Property")), value: t.attr("Value")}}
			case "multitrigger", "multidatatrigger":
				tr.conds = collectTriggerConditions(t)
			default:
				continue // EventTrigger пока не поддержан (нужны анимации)
			}
			collectSettersInto(t, tr.setters)
			if len(tr.conds) > 0 && len(tr.setters) > 0 {
				s.triggers = append(s.triggers, tr)
			}
		}
	}
}

// collectTriggerConditions собирает <Condition> из MultiTrigger.Conditions /
// MultiDataTrigger.Conditions.
func collectTriggerConditions(el *xElement) []triggerCond {
	var out []triggerCond
	for i := range el.Children {
		c := &el.Children[i]
		if !strings.HasSuffix(strings.ToLower(c.Tag), ".conditions") {
			continue
		}
		for j := range c.Children {
			cond := &c.Children[j]
			if !strings.EqualFold(cond.Tag, "Condition") {
				continue
			}
			tc := triggerCond{value: cond.attr("Value")}
			if bnd := cond.attr("Binding"); bnd != "" {
				tc.path = parseBindingPath(bnd)
			} else if p := cond.attr("Property"); p != "" {
				tc.property = stripPropName(p)
			}
			if tc.path != "" || tc.property != "" {
				out = append(out, tc)
			}
		}
	}
	return out
}

// collectSettersInto собирает <Setter Property Value> в переданную карту.
func collectSettersInto(el *xElement, dst map[string]string) {
	for i := range el.Children {
		c := &el.Children[i]
		if !strings.EqualFold(c.Tag, "Setter") {
			continue
		}
		prop := c.attr("Property")
		if prop == "" {
			continue
		}
		dst[stripPropName(prop)] = c.attr("Value")
	}
}

// collectSetters собирает <Setter Property Value> (в т.ч. внутри <Style.Setters>).
func collectSetters(el *xElement, s *xamlStyle) {
	for i := range el.Children {
		child := &el.Children[i]
		ctag := strings.ToLower(child.Tag)
		switch {
		case ctag == "setter":
			prop := child.attr("Property")
			if prop == "" {
				continue
			}
			val := child.attr("Value")
			if val == "" {
				// <Setter.Value> как вложенный элемент — берём Color/Text.
				for j := range child.Children {
					if strings.EqualFold(child.Children[j].Tag, "Setter.Value") {
						val = scalarResourceValue(&child.Children[j])
					}
				}
			}
			s.setters[stripPropName(prop)] = val
		case strings.HasSuffix(ctag, ".setters"):
			collectSetters(child, s)
		}
	}
}

// ─── Применение ─────────────────────────────────────────────────────────────

// process — точка входа обработки поддерева (глубина 0). Сигнатуру используют
// живые перестроения из xaml_binding.go, поэтому она сохранена как есть.
func (env *xamlEnv) process(el *xElement, ctx interface{}) {
	env.processAt(el, ctx, 0)
}

func (env *xamlEnv) processAt(el *xElement, ctx interface{}, depth int) {
	if depth > maxXAMLDepth {
		logTooDeep("process")
		return
	}
	tag := strings.ToLower(el.Tag)
	// Внутрь ресурсов/стилей/InputBindings/DataGrid-колонок не лезем —
	// они обрабатываются отдельно (DataGrid сам разбирает Binding колонок).
	if isResourceContainer(tag) || tag == "style" || tag == "setter" ||
		tag == "resourcedictionary" || strings.HasSuffix(tag, ".inputbindings") ||
		strings.HasSuffix(tag, ".columns") || isDataGridColumnTag(tag) {
		return
	}

	// ControlTemplate: ContentControl с шаблоном превращается в дерево шаблона
	// (с ContentPresenter и TemplateBinding) до дальнейшей обработки.
	if tag == "contentcontrol" || tag == "headeredcontentcontrol" {
		if env.expandControlTemplate(el) {
			tag = strings.ToLower(el.Tag)
		}
	}

	// Поднимаем property-element синтаксис (<X.Background><SolidColorBrush/>) в
	// атрибуты, чтобы существующие билдеры читали их как обычные атрибуты.
	env.liftPropertyElements(el)

	// ItemsControl: разворачиваем ItemTemplate по элементам ItemsSource ДО
	// резолва атрибутов (чтобы не испортить ItemsS="{Binding}" биндинг-логикой).
	if tag == "itemscontrol" {
		expanded := env.expandItemsControl(el, ctx)
		env.resolveAttrs(el, ctx)
		env.applyStyleTo(el, ctx)
		if expanded {
			return // клоны уже обработаны с DataContext каждого элемента
		}
		for i := range el.Children {
			env.processAt(&el.Children[i], ctx, depth+1)
		}
		return
	}

	// VirtualizingItemsControl: НЕ материализуем элементы в пре-пассе — шаблон и
	// источник сохраняются, виджеты строятся лениво по видимому окну в рантайме.
	if tag == "virtualizingitemscontrol" {
		env.registerVirtual(el, ctx)
		env.resolveAttrs(el, ctx)
		env.applyStyleTo(el, ctx)
		return
	}

	env.resolveAttrs(el, ctx)
	env.applyStyleTo(el, ctx)
	for i := range el.Children {
		env.processAt(&el.Children[i], ctx, depth+1)
	}
}

// registerVirtual сохраняет шаблон и путь источника VirtualizingItemsControl для
// последующей ленивой материализации; удаляет ItemTemplate/ItemsSource из узла.
func (env *xamlEnv) registerVirtual(el *xElement, ctx interface{}) {
	tmpl := findDataTemplateRoot(el)
	src := strings.TrimSpace(el.attr("ItemsSource"))
	name := env.ensureName(el)

	// Убираем .ItemTemplate из детей — рантайм строит строки сам.
	var keep []xElement
	for _, c := range el.Children {
		if strings.HasSuffix(strings.ToLower(c.Tag), ".itemtemplate") {
			continue
		}
		keep = append(keep, c)
	}
	el.Children = keep
	delete(el.attrs, "ItemsSource")

	if tmpl == nil || !strings.HasPrefix(src, "{Binding") {
		return // нет шаблона/привязки — останется пустой виртуализованный список
	}
	path := parseBindingPath(src)
	env.virtuals = append(env.virtuals, pendingVirtual{
		name:       name,
		template:   cloneXElement(tmpl),
		sourcePath: path,
		env:        env,
	})
}

// liftableProps — скалярные свойства, которые можно поднять из property-element
// синтаксиса в атрибут элемента.
var liftableProps = map[string]bool{
	"background": true, "foreground": true, "borderbrush": true,
	"fill": true, "stroke": true, "cornerradius": true,
	"borderthickness": true, "tooltip": true, "padding": true,
}

// liftPropertyElements превращает <X.Prop><SolidColorBrush Color=.../></X.Prop>
// (и <X.Prop>значение</X.Prop>) в атрибут Prop="значение" на самом X.
// Структурные property-element'ы (RowDefinitions, Resources, Triggers, …) не
// затрагиваются — поднимаются только скалярные свойства из whitelist.
func (env *xamlEnv) liftPropertyElements(el *xElement) {
	elTag := strings.ToLower(el.Tag)
	for i := range el.Children {
		c := &el.Children[i]
		dot := strings.IndexByte(c.Tag, '.')
		if dot < 0 {
			continue
		}
		owner := strings.ToLower(c.Tag[:dot])
		prop := c.Tag[dot+1:]
		if owner != elTag || !liftableProps[strings.ToLower(prop)] {
			continue
		}
		if _, exists := el.attrs[prop]; exists {
			continue // явный атрибут имеет приоритет
		}
		if v := scalarFromPropertyElement(c); v != "" {
			el.attrs[prop] = v
			continue
		}
		// Градиентная кисть для фона/заливки.
		lp := strings.ToLower(prop)
		if lp == "background" || lp == "fill" {
			for j := range c.Children {
				if g := encodeGradient(&c.Children[j]); g != "" {
					el.attrs["__gradient"] = g
					break
				}
			}
		}
	}
}

// scalarFromPropertyElement извлекает скалярное значение из property-element:
// вложенный <SolidColorBrush Color=.../>, <Color>, текст или Value.
func scalarFromPropertyElement(c *xElement) string {
	for j := range c.Children {
		if v := scalarResourceValue(&c.Children[j]); v != "" {
			return v
		}
	}
	if t := strings.TrimSpace(c.Text); t != "" {
		return t
	}
	return ""
}

// resolveAttrs резолвит markup-расширения и регистрирует {Binding} в атрибутах.
func (env *xamlEnv) resolveAttrs(el *xElement, ctx interface{}) {
	// Итерируем по снимку ключей: ensureName может добавить ключ "Name".
	keys := make([]string, 0, len(el.attrs))
	for k := range el.attrs {
		keys = append(keys, k)
	}
	for _, k := range keys {
		v := el.attrs[k]
		tv := strings.TrimSpace(v)
		if strings.HasPrefix(tv, "{Binding") {
			if env.inItemTemplate {
				// Внутри DataTemplate — одноразовый резолв (с StringFormat).
				el.attrs[k] = resolveBindingFull(tv, ctx)
				continue
			}
			spec := parseBindingSpec(tv)
			name := env.ensureName(el)
			if spec.relativeSelf && spec.elementName == "" {
				spec.elementName = name // RelativeSource Self → источник = сам элемент
			}
			env.bindings = append(env.bindings, pendingBinding{name: name, prop: k, spec: spec})
			el.attrs[k] = resolveBindingFull(tv, ctx) // начальное значение с форматом
			continue
		}
		if tv == "{Loc}" || strings.HasPrefix(tv, "{Loc ") || strings.HasPrefix(tv, "{Loc}") {
			key := parseLocKey(tv)
			if env.inItemTemplate {
				el.attrs[k] = Tr(key) // внутри DataTemplate — одноразовый перевод
				continue
			}
			if isFoldedItemTag(el.Tag) {
				// Вкладки, пункты меню и элементы списков виджетами не
				// становятся — их сворачивает в себя родитель. Разметку
				// оставляем как есть: перевод и подписку на смену языка
				// заведёт сборщик родителя (см. xaml_loc_items.go).
				continue
			}
			name := env.ensureName(el)
			env.locs = append(env.locs, pendingLoc{name: name, prop: k, key: key})
			el.attrs[k] = Tr(key) // начальное значение для первого рендера
			continue
		}
		if nv, ok := env.resolveMarkup(v, ctx); ok {
			el.attrs[k] = nv
		}
	}
}

// ─── ItemsControl + DataTemplate ────────────────────────────────────────────

// expandItemsControl разворачивает <ItemsControl ItemsSource="{Binding ...}">
// с <ItemsControl.ItemTemplate><DataTemplate>: для каждого элемента коллекции
// клонирует шаблон, обрабатывает его с этим элементом как DataContext и делает
// дочерним. Сам ItemsControl превращается в вертикальный StackPanel.
//
// Привязка одноразовая (на момент загрузки). Возвращает true, если развёрнут.
func (env *xamlEnv) expandItemsControl(el *xElement, ctx interface{}) bool {
	tmpl := findDataTemplateRoot(el)
	src := strings.TrimSpace(el.attr("ItemsSource"))
	if tmpl == nil || !strings.HasPrefix(src, "{Binding") || ctx == nil {
		return false
	}
	path := parseBindingPath(src)
	val, ok := dgridPkg.GetPropertyValue(ctx, path)
	if !ok {
		return false
	}
	items := collectionItems(val)

	// Сохраняем СЫРОЙ шаблон (до обработки) для живого перестроения.
	rawTmpl := cloneXElement(tmpl)

	var kids []xElement
	prev := env.inItemTemplate
	env.inItemTemplate = true
	for _, it := range items {
		c := cloneXElement(tmpl)
		env.process(&c, it) // одноразовый резолв против элемента коллекции
		kids = append(kids, c)
	}
	env.inItemTemplate = prev

	// Превращаем в StackPanel.
	el.Tag = "StackPanel"
	delete(el.attrs, "ItemsSource")
	if _, has := el.attrs["Orientation"]; !has {
		el.attrs["Orientation"] = "Vertical"
	}
	el.Children = kids

	// Регистрируем для живого обновления при изменении ObservableCollection.
	name := env.ensureName(el)
	env.items = append(env.items, pendingItems{
		name:       name,
		template:   rawTmpl,
		sourcePath: path,
		env:        env,
	})
	return true
}

// findDataTemplateRoot ищет корневой элемент шаблона в ItemsControl.ItemTemplate.
func findDataTemplateRoot(el *xElement) *xElement {
	for i := range el.Children {
		c := &el.Children[i]
		if !strings.HasSuffix(strings.ToLower(c.Tag), ".itemtemplate") {
			continue
		}
		for j := range c.Children {
			dt := &c.Children[j]
			if !strings.EqualFold(dt.Tag, "DataTemplate") {
				continue
			}
			for k := range dt.Children {
				if !strings.Contains(dt.Children[k].Tag, ".") {
					return &dt.Children[k]
				}
			}
		}
	}
	return nil
}

// collectionItems извлекает элементы из CollectionView, ObservableCollection или среза.
func collectionItems(v interface{}) []interface{} {
	if cv, ok := v.(*CollectionView); ok {
		return cv.Items()
	}
	if oc, ok := v.(*dgridPkg.ObservableCollection); ok {
		return oc.Items()
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Slice {
		out := make([]interface{}, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = rv.Index(i).Interface()
		}
		return out
	}
	return nil
}

// cloneXElement делает глубокую копию узла XAML-дерева.
func cloneXElement(el *xElement) xElement {
	return cloneXElementAt(el, 0)
}

// cloneXElementAt — рекурсивное тело cloneXElement с ограничением глубины
// (SEC-7): поддерево глубже maxXAMLDepth не копируется.
func cloneXElementAt(el *xElement, depth int) xElement {
	c := xElement{Tag: el.Tag, Text: el.Text, attrs: make(map[string]string, len(el.attrs))}
	for k, v := range el.attrs {
		c.attrs[k] = v
	}
	if depth > maxXAMLDepth {
		logTooDeep("cloneXElement")
		return c
	}
	for i := range el.Children {
		c.Children = append(c.Children, cloneXElementAt(&el.Children[i], depth+1))
	}
	return c
}

// applyStyleTo находит применимый стиль и вливает его Setter'ы в атрибуты
// элемента (только те, что не заданы явно).
func (env *xamlEnv) applyStyleTo(el *xElement, ctx interface{}) {
	tag := strings.ToLower(el.Tag)
	var st *xamlStyle
	if sref := el.attr("Style"); sref != "" {
		if key := extractKey(sref); key != "" {
			st = env.keyedStyles[key]
		}
	}
	if st == nil {
		st = env.implicitStyles[tag]
	}
	if st == nil {
		return
	}
	for prop, val := range env.mergedSetters(st, map[*xamlStyle]bool{}) {
		if _, exists := el.attrs[prop]; exists {
			continue // явный атрибут имеет приоритет над Setter
		}
		if nv, ok := env.resolveMarkup(val, ctx); ok {
			val = nv
		}
		el.attrs[prop] = val
	}

	// DataTrigger'ы стиля → динамические сеттеры (вычисляются в BindingScope).
	// Внутри DataTemplate — пропускаем (контекст элемента одноразовый).
	if len(st.triggers) > 0 && !env.inItemTemplate {
		name := env.ensureName(el)
		var defs []triggerDef
		base := map[string]string{}
		for _, tr := range st.triggers {
			d := triggerDef{conds: tr.conds, setters: map[string]string{}}
			for p, v := range tr.setters {
				if nv, ok := env.resolveMarkup(v, ctx); ok {
					v = nv
				}
				d.setters[p] = v
				if _, seen := base[p]; !seen {
					base[p] = el.attrs[p] // базовое значение (после слияния стиля)
				}
			}
			defs = append(defs, d)
		}
		env.triggers = append(env.triggers, pendingTrigger{name: name, defs: defs, base: base})
	}
}

// mergedSetters возвращает Setter'ы стиля с учётом цепочки BasedOn
// (база применяется первой, текущий стиль переопределяет).
func (env *xamlEnv) mergedSetters(st *xamlStyle, seen map[*xamlStyle]bool) map[string]string {
	out := map[string]string{}
	if st == nil || seen[st] {
		return out
	}
	seen[st] = true
	if st.basedOn != "" {
		for k, v := range env.mergedSetters(env.keyedStyles[st.basedOn], seen) {
			out[k] = v
		}
	}
	for k, v := range st.setters {
		out[k] = v
	}
	return out
}

// ─── Markup-расширения ──────────────────────────────────────────────────────

// resolveMarkup разворачивает значение-атрибут, если это markup-расширение.
// Возвращает (новое значение, true), если значение было раскрыто.
func (env *xamlEnv) resolveMarkup(val string, ctx interface{}) (string, bool) {
	v := strings.TrimSpace(val)
	if len(v) < 2 || v[0] != '{' || !strings.HasSuffix(v, "}") {
		return val, false
	}
	inner := strings.TrimSpace(v[1 : len(v)-1])
	switch {
	case strings.HasPrefix(inner, "x:Null"):
		return "", true
	case strings.HasPrefix(inner, "StaticResource"), strings.HasPrefix(inner, "DynamicResource"):
		k := extractKey(v)
		if sv, ok := env.scalars[k]; ok {
			return sv, true
		}
		// Не скаляр (возможно ключ стиля) — оставляем как есть для applyStyleTo.
		return val, false
	case strings.HasPrefix(inner, "Binding"):
		// Однонаправленный биндинг на момент загрузки.
		return resolveBindingValue(v, ctx), true
	}
	return val, false
}

// resolveBindingValue читает значение из DataContext по пути {Binding Path}.
func resolveBindingValue(expr string, ctx interface{}) string {
	if ctx == nil {
		return ""
	}
	path := parseBindingPath(expr)
	if path == "" {
		return ""
	}
	if v, ok := dgridPkg.GetPropertyValue(ctx, path); ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// ─── Утилиты ────────────────────────────────────────────────────────────────

// extractKey извлекает ключ из "{StaticResource key}" / "{DynamicResource key}"
// или возвращает строку как есть (если это уже голый ключ).
func extractKey(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		for _, p := range []string{"StaticResource", "DynamicResource"} {
			if strings.HasPrefix(inner, p) {
				return strings.TrimSpace(strings.TrimPrefix(inner, p))
			}
		}
		return ""
	}
	return s
}

// stripTypeName нормализует TargetType: "{x:Type Button}" / "local:Foo" → "Foo".
func stripTypeName(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		s = strings.TrimSpace(s[1 : len(s)-1])
		s = strings.TrimSpace(strings.TrimPrefix(s, "x:Type"))
	}
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(s)
}

// stripPropName убирает префикс класса у Setter.Property ("Button.Background" →
// "Background"; "TextBlock.FontSize" → "FontSize").
func stripPropName(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}
