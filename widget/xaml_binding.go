// xaml_binding.go — живая привязка данных (Data Binding) для XAML (P0+).
//
// Поддержано:
//   - {Binding Path}            — OneWay (модель → UI) по умолчанию
//   - {Binding Path, Mode=...}  — OneWay | TwoWay | OneTime
//   - {Binding ..., StringFormat=%.2f} — Go-формат значения
//   - Авто-обновление UI при INotifyPropertyChanged у DataContext
//   - TwoWay (UI → модель) для TextBox, CheckBox, ToggleButton, RadioButton, Slider
//   - BindingScope.SetDataContext / Refresh для ручного управления
//
// Реализация связывает виджеты по имени (см. ensureName в xaml_resources.go):
// безымянным элементам с {Binding} присваивается авто-имя, билдер регистрирует
// их в registry, а BindingScope резолвит имя → виджет.
package widget

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	dgridPkg "github.com/oops1/headless-gui/v3/widget/datagrid"
)

// ValueConverter — преобразователь значений привязки (WPF IValueConverter).
type ValueConverter = dgridPkg.IValueConverter

var (
	convMu     sync.RWMutex
	converters = map[string]ValueConverter{}
)

// RegisterValueConverter регистрирует IValueConverter под ключом для
// использования в XAML: Converter={StaticResource key}. Потокобезопасно.
func RegisterValueConverter(key string, c ValueConverter) {
	if key == "" || c == nil {
		return
	}
	convMu.Lock()
	converters[key] = c
	convMu.Unlock()
}

func lookupConverter(key string) ValueConverter {
	if key == "" {
		return nil
	}
	convMu.RLock()
	defer convMu.RUnlock()
	return converters[key]
}

// applyConvert применяет прямое преобразование конвертера (модель → UI).
func applyConvert(spec bindingSpec, v interface{}) interface{} {
	if c := lookupConverter(spec.converterKey); c != nil {
		return c.Convert(v)
	}
	return v
}

// applyConvertBack применяет обратное преобразование (UI → модель).
func applyConvertBack(spec bindingSpec, v interface{}) interface{} {
	if c := lookupConverter(spec.converterKey); c != nil {
		return c.ConvertBack(v)
	}
	return v
}

// BindingMode — направление привязки (как в WPF).
type BindingMode int

const (
	BindOneWay  BindingMode = iota // модель → UI (по умолчанию)
	BindTwoWay                      // модель ↔ UI
	BindOneTime                     // однократно на момент загрузки
)

// bindingSpec — разобранное выражение {Binding ...}.
type bindingSpec struct {
	path         string
	mode         BindingMode
	stringFormat string // Go-формат (напр. "%.2f"); пусто = "%v"
	converterKey string // ключ зарегистрированного IValueConverter (или "")
	elementName  string // источник {Binding ElementName=...} (или "")
	relativeSelf bool   // {Binding ..., RelativeSource={RelativeSource Self}}
}

// pendingBinding — привязка, ожидающая резолва имени → виджета после build.
type pendingBinding struct {
	name string
	prop string
	spec bindingSpec
}

// bindingTarget — связанная цель (виджет + свойство + спецификация).
type bindingTarget struct {
	w    Widget
	prop string
	spec bindingSpec
}

// triggerCond — одно условие триггера. property!="" → значение свойства самого
// контрола (property Trigger); иначе path → значение из DataContext (DataTrigger).
type triggerCond struct {
	path     string
	property string
	value    string
}

// triggerDef — триггер: все conds должны совпасть (AND для MultiTrigger),
// тогда применяются setters. Для одиночного Trigger/DataTrigger — одно условие.
type triggerDef struct {
	conds   []triggerCond
	setters map[string]string
}

// pendingTrigger — DataTrigger, ожидающий резолва имени → виджета.
type pendingTrigger struct {
	name string
	defs []triggerDef
	base map[string]string // базовые значения свойств (когда триггеры неактивны)
}

// triggerTarget — связанный DataTrigger.
type triggerTarget struct {
	w    Widget
	defs []triggerDef
	base map[string]string
}

// elemBindingTarget — привязка свойства одного виджета к свойству другого
// ({Binding ElementName=src, Path=srcProp}). Однонаправленно src → target.
type elemBindingTarget struct {
	target  Widget
	prop    string
	src     Widget
	srcProp string
	spec    bindingSpec
}

// pendingItems — ItemsControl, ожидающий резолва имени → виджета (для живого
// перестроения при изменении ObservableCollection).
type pendingItems struct {
	name       string
	template   xElement
	sourcePath string
	env        *xamlEnv
}

// itemsTarget — связанный ItemsControl (panel = развёрнутый StackPanel).
type itemsTarget struct {
	panel      Widget
	template   xElement
	sourcePath string
	env        *xamlEnv
	reg        map[string]Widget
	baseDir    string
}

// BindingScope управляет набором привязок одного загруженного XAML-дерева.
type BindingScope struct {
	mu       sync.Mutex
	ctx      interface{}
	targets  []bindingTarget
	elemTgts []elemBindingTarget
	triggers []triggerTarget
	items    []itemsTarget
	cmdTgts  []commandTarget
	reg      map[string]Widget
	baseDir  string
	updating atomic.Bool // защита от петли обратной связи во время Refresh
}

// commandTarget — привязка Button.Command к объекту-команде из DataContext.
type commandTarget struct {
	btn  *Button
	path string
}

// newBindingScope строит scope из собранных в env привязок/триггеров/ItemsControl,
// резолвя имена элементов в registry.
func newBindingScope(ctx interface{}, env *xamlEnv, reg map[string]Widget, baseDir string) *BindingScope {
	s := &BindingScope{ctx: ctx, reg: reg, baseDir: baseDir}
	if env == nil {
		return s
	}
	for _, pb := range env.bindings {
		w, ok := reg[pb.name]
		if !ok {
			continue
		}
		// Command — привязка объекта-команды (не строкового значения).
		if strings.EqualFold(pb.prop, "Command") {
			if btn, ok2 := w.(*Button); ok2 {
				s.cmdTgts = append(s.cmdTgts, commandTarget{btn: btn, path: pb.spec.path})
			}
			continue
		}
		if pb.spec.elementName != "" {
			// Привязка к свойству другого элемента (ElementName / RelativeSource Self).
			if src, ok2 := reg[pb.spec.elementName]; ok2 {
				sprop := pb.spec.path
				if sprop == "" {
					sprop = pb.prop
				}
				s.elemTgts = append(s.elemTgts, elemBindingTarget{
					target: w, prop: pb.prop, src: src, srcProp: sprop, spec: pb.spec,
				})
			}
			continue
		}
		s.targets = append(s.targets, bindingTarget{w: w, prop: pb.prop, spec: pb.spec})
	}
	for _, pt := range env.triggers {
		if w, ok := reg[pt.name]; ok {
			s.triggers = append(s.triggers, triggerTarget{w: w, defs: pt.defs, base: pt.base})
		}
	}
	for _, pi := range env.items {
		if w, ok := reg[pi.name]; ok {
			s.items = append(s.items, itemsTarget{
				panel: w, template: pi.template, sourcePath: pi.sourcePath,
				env: pi.env, reg: reg, baseDir: baseDir,
			})
		}
	}
	return s
}

// activate подключает TwoWay-обработчики, подписку на INotifyPropertyChanged и
// выполняет начальное обновление UI.
func (s *BindingScope) activate() {
	if s == nil {
		return
	}
	s.wireTwoWay()
	s.wireElementSources()
	s.wireTriggerSources()
	s.wireItems()
	s.wireCommands()
	s.subscribe()
	s.Refresh()
}

// wireCommands присваивает Button.Command объект-команду из DataContext.
func (s *BindingScope) wireCommands() {
	if s.ctx == nil {
		return
	}
	for _, ct := range s.cmdTgts {
		v, ok := dgridPkg.GetPropertyValue(s.ctx, ct.path)
		if !ok {
			continue
		}
		if cmd, ok := v.(ICommand); ok {
			ct.btn.Command = cmd
		}
	}
}

// wireItems подписывается на изменения ObservableCollection для живого
// перестроения ItemsControl.
func (s *BindingScope) wireItems() {
	if s.ctx == nil {
		return
	}
	for _, it := range s.items {
		val, ok := dgridPkg.GetPropertyValue(s.ctx, it.sourcePath)
		if !ok {
			continue
		}
		if oc, ok := val.(*dgridPkg.ObservableCollection); ok {
			t := it
			oc.AddCollectionChanged(func(ev dgridPkg.CollectionChangedEvent) {
				s.rebuildItems(t)
			})
		}
	}
}

// rebuildItems пересобирает дочерние виджеты ItemsControl по текущей коллекции.
// Вызывается из обработчика CollectionChanged (на горутине источника).
func (s *BindingScope) rebuildItems(it itemsTarget) {
	ctx := s.DataContext()
	if ctx == nil {
		return
	}
	val, ok := dgridPkg.GetPropertyValue(ctx, it.sourcePath)
	if !ok {
		return
	}
	items := collectionItems(val)
	if cc, ok := it.panel.(interface{ ClearChildren() }); ok {
		cc.ClearChildren()
	}
	for _, item := range items {
		c := cloneXElement(&it.template)
		re := &xamlEnv{
			scalars:        it.env.scalars,
			keyedStyles:    it.env.keyedStyles,
			implicitStyles: it.env.implicitStyles,
			inItemTemplate: true,
		}
		re.process(&c, item)
		w, err := buildXAMLWidget(c, it.reg, image.Point{}, it.baseDir)
		if err == nil && w != nil {
			it.panel.AddChild(w)
		}
	}
	it.panel.SetBounds(it.panel.Bounds()) // перелейаут
}

// wireTriggerSources подключает перевычисление property-триггеров при изменении
// свойства контрола (IsChecked/Value/SelectedIndex) — чтобы триггеры были живыми.
func (s *BindingScope) wireTriggerSources() {
	for _, tt := range s.triggers {
		hasProp := false
		for _, d := range tt.defs {
			for _, c := range d.conds {
				if c.property != "" {
					hasProp = true
				}
			}
		}
		if hasProp {
			hookSourceChange(tt.w, s.Refresh)
		}
	}
}

// DataContext возвращает текущий источник данных.
func (s *BindingScope) DataContext() interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ctx
}

// SetDataContext меняет источник данных, переподписывается и обновляет UI.
func (s *BindingScope) SetDataContext(ctx interface{}) {
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()
	s.subscribe()
	s.Refresh()
}

// Refresh перечитывает значения из DataContext и применяет к виджетам (OneWay/TwoWay).
func (s *BindingScope) Refresh() {
	if s == nil {
		return
	}
	s.mu.Lock()
	ctx := s.ctx
	targets := make([]bindingTarget, len(s.targets))
	copy(targets, s.targets)
	elemTgts := make([]elemBindingTarget, len(s.elemTgts))
	copy(elemTgts, s.elemTgts)
	triggers := make([]triggerTarget, len(s.triggers))
	copy(triggers, s.triggers)
	s.mu.Unlock()

	s.updating.Store(true)
	defer s.updating.Store(false)

	// Привязки к свойствам других элементов (ElementName) — не требуют ctx.
	for _, et := range elemTgts {
		if v, ok := getWidgetProperty(et.src, et.srcProp); ok {
			v = applyConvert(et.spec, v)
			setWidgetProperty(et.target, et.prop, formatBindingValue(v, et.spec))
		}
	}

	// Привязки к DataContext (требуют ctx).
	if ctx != nil {
		for _, t := range targets {
			if t.spec.mode == BindOneTime {
				continue // OneTime применяется только при загрузке (через атрибуты)
			}
			v, ok := dgridPkg.GetPropertyValue(ctx, t.spec.path)
			if !ok {
				continue
			}
			v = applyConvert(t.spec, v)
			setWidgetProperty(t.w, t.prop, formatBindingValue(v, t.spec))
		}
	}
	// Триггеры: базовые значения + сеттеры активных триггеров.
	for _, tt := range triggers {
		applied := make(map[string]string, len(tt.base))
		for p, v := range tt.base {
			applied[p] = v
		}
		for _, d := range tt.defs {
			active := len(d.conds) > 0
			for _, c := range d.conds {
				var cv interface{}
				var ok bool
				if c.property != "" {
					cv, ok = getWidgetProperty(tt.w, c.property) // property condition
				} else if ctx != nil {
					cv, ok = dgridPkg.GetPropertyValue(ctx, c.path) // data condition
				}
				if !ok || !valueEquals(cv, c.value) {
					active = false
					break
				}
			}
			if active {
				for p, v := range d.setters {
					applied[p] = v
				}
			}
		}
		for p, v := range applied {
			if strings.TrimSpace(v) != "" {
				setWidgetProperty(tt.w, p, v)
			}
		}
	}
}

// valueEquals сравнивает значение модели со строкой триггера (без учёта регистра).
func valueEquals(cv interface{}, s string) bool {
	return strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", cv)), strings.TrimSpace(s))
}

// subscribe подписывается на INotifyPropertyChanged DataContext (если поддерживает).
func (s *BindingScope) subscribe() {
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	if n, ok := ctx.(interface {
		AddPropertyChanged(dgridPkg.PropertyChangedHandler)
	}); ok {
		n.AddPropertyChanged(func(sender interface{}, prop string) {
			s.Refresh()
		})
	}
}

// writeBack записывает значение из UI в модель (TwoWay), применяя ConvertBack.
// Игнорируется во время Refresh, чтобы программное обновление не зациклилось.
func (s *BindingScope) writeBack(spec bindingSpec, value interface{}) {
	if s.updating.Load() {
		return
	}
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	if ctx == nil {
		return
	}
	dgridPkg.SetPropertyValue(ctx, spec.path, applyConvertBack(spec, value))
}

// wireTwoWay подключает обратную запись UI → модель для редактируемых виджетов.
func (s *BindingScope) wireTwoWay() {
	for _, t := range s.targets {
		if t.spec.mode != BindTwoWay {
			continue
		}
		spec := t.spec
		switch strings.ToLower(t.prop) {
		case "text":
			if ti, ok := t.w.(*TextInput); ok {
				ti.OnChange = func(v string) { s.writeBack(spec, v) }
			}
		case "ischecked", "checked":
			switch cw := t.w.(type) {
			case *CheckBox:
				cw.OnChange = func(b bool) { s.writeBack(spec, b) }
			case *ToggleSwitch:
				cw.OnChange = func(b bool) { s.writeBack(spec, b) }
			case *RadioButton:
				cw.OnChange = func(b bool) { s.writeBack(spec, b) }
			}
		case "value":
			if sl, ok := t.w.(*Slider); ok {
				sl.OnChange = func(v float64) { s.writeBack(spec, v) }
			}
		case "selectedindex":
			if dd, ok := t.w.(*Dropdown); ok {
				dd.OnChange = func(idx int, _ string) { s.writeBack(spec, idx) }
			}
		}
	}
}

// wireElementSources подключает обновление при изменении виджета-источника
// для привязок ElementName (src → target).
func (s *BindingScope) wireElementSources() {
	for _, et := range s.elemTgts {
		hookSourceChange(et.src, s.Refresh)
	}
}

// hookSourceChange навешивает cb на изменение значения виджета, сохраняя
// уже установленный OnChange (цепочка вызовов).
func hookSourceChange(w Widget, cb func()) {
	switch t := w.(type) {
	case *TextInput:
		prev := t.OnChange
		t.OnChange = func(v string) {
			if prev != nil {
				prev(v)
			}
			cb()
		}
	case *Slider:
		prev := t.OnChange
		t.OnChange = func(v float64) {
			if prev != nil {
				prev(v)
			}
			cb()
		}
	case *CheckBox:
		prev := t.OnChange
		t.OnChange = func(b bool) {
			if prev != nil {
				prev(b)
			}
			cb()
		}
	case *ToggleSwitch:
		prev := t.OnChange
		t.OnChange = func(b bool) {
			if prev != nil {
				prev(b)
			}
			cb()
		}
	case *RadioButton:
		prev := t.OnChange
		t.OnChange = func(b bool) {
			if prev != nil {
				prev(b)
			}
			cb()
		}
	case *Dropdown:
		prev := t.OnChange
		t.OnChange = func(i int, txt string) {
			if prev != nil {
				prev(i, txt)
			}
			cb()
		}
	}
}

// getWidgetProperty читает значение свойства виджета (для ElementName-привязок).
func getWidgetProperty(w Widget, prop string) (interface{}, bool) {
	switch strings.ToLower(prop) {
	case "text", "content":
		switch t := w.(type) {
		case *Label:
			return t.Text(), true
		case *TextInput:
			return t.GetText(), true
		case *Button:
			return t.Text, true
		}
	case "value":
		switch t := w.(type) {
		case *Slider:
			return t.Value(), true
		case *ProgressBar:
			return t.Value(), true
		}
	case "ischecked", "checked":
		switch t := w.(type) {
		case *CheckBox:
			return t.IsChecked(), true
		case *ToggleSwitch:
			return t.IsOn(), true
		case *RadioButton:
			return t.IsSelected(), true
		}
	case "selectedindex":
		switch t := w.(type) {
		case *Dropdown:
			return t.Selected(), true
		case *ListView:
			return t.Selected(), true
		}
	case "isenabled":
		if e, ok := w.(interface{ IsEnabled() bool }); ok {
			return e.IsEnabled(), true
		}
	}
	return nil, false
}

// ─── Применение значения к свойству виджета ─────────────────────────────────

// setWidgetProperty устанавливает свойство виджета по WPF-имени из строкового
// значения. Используется живой привязкой (и пригодно для будущих триггеров).
func setWidgetProperty(w Widget, prop, val string) {
	switch strings.ToLower(prop) {
	case "text", "content":
		setWidgetText(w, val)
	case "ischecked", "checked":
		b := parseXAMLBool(val)
		switch t := w.(type) {
		case *CheckBox:
			t.SetChecked(b)
		case *ToggleSwitch:
			t.SetOn(b)
		case *RadioButton:
			t.SetSelected(b)
		}
	case "value":
		if f, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil {
			switch t := w.(type) {
			case *Slider:
				t.SetValue(f)
			case *ProgressBar:
				t.SetValue(f)
			}
		}
	case "isenabled":
		if e, ok := w.(interface{ SetEnabled(bool) }); ok {
			e.SetEnabled(parseXAMLBool(val))
		}
	case "visibility":
		if v, ok := w.(interface{ SetVisible(bool) }); ok {
			lv := strings.ToLower(strings.TrimSpace(val))
			v.SetVisible(!(lv == "collapsed" || lv == "hidden"))
		}
	case "tooltip":
		if ts, ok := w.(interface{ SetToolTip(string) }); ok {
			ts.SetToolTip(val)
		}
	case "foreground":
		if c, err := parseXAMLColor(val); err == nil {
			setWidgetForeground(w, c)
		}
	case "background":
		if c, err := parseXAMLColor(val); err == nil {
			setWidgetBackground(w, c)
		}
	case "selectedindex":
		if i, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			switch t := w.(type) {
			case *Dropdown:
				t.SetSelected(i)
			case *ListView:
				t.SetSelected(i)
			}
		}
	}
}

func setWidgetText(w Widget, s string) {
	switch t := w.(type) {
	case *Label:
		t.SetText(s)
	case *Button:
		t.Text = s
	case *TextInput:
		t.SetText(s)
	case *CheckBox:
		t.Text = s
	case *RadioButton:
		t.Text = s
	case *ToggleSwitch:
		t.Text = s
	}
}

func setWidgetForeground(w Widget, c color.RGBA) {
	switch t := w.(type) {
	case *Label:
		t.TextColor = c
	case *Button:
		t.TextColor = c
	case *TextInput:
		t.TextColor = c
	case *CheckBox:
		t.TextColor = c
	case *RadioButton:
		t.TextColor = c
	case *ToggleSwitch:
		t.TextColor = c
	}
}

func setWidgetBackground(w Widget, c color.RGBA) {
	switch t := w.(type) {
	case *Button:
		t.Background = c
	case *Panel:
		t.Background = c
		t.UseAlpha = c.A < 255
	case *TextInput:
		t.Background = c
	case *Grid:
		t.Background = c
		t.UseAlpha = c.A < 255
	case *Canvas:
		t.Background = c
		t.UseAlpha = c.A < 255
	}
}

// ─── Разбор {Binding ...} ───────────────────────────────────────────────────

// parseBindingSpec разбирает выражение {Binding ...} → bindingSpec.
func parseBindingSpec(expr string) bindingSpec {
	sp := bindingSpec{mode: BindOneWay}
	inner := strings.TrimSpace(expr)
	inner = strings.TrimPrefix(inner, "{")
	inner = strings.TrimSuffix(inner, "}")

	// Path: явный "Path=" имеет приоритет; иначе первый токен (если не "name=").
	if p := bindingParam(inner, "Path="); p != "" {
		sp.path = p
	} else if fp := parseBindingPath(expr); !strings.Contains(fp, "=") {
		sp.path = fp
	}
	if m := bindingParam(inner, "Mode="); m != "" {
		switch strings.ToLower(m) {
		case "twoway":
			sp.mode = BindTwoWay
		case "onetime":
			sp.mode = BindOneTime
		default:
			sp.mode = BindOneWay
		}
	}
	if en := bindingParam(inner, "ElementName="); en != "" {
		sp.elementName = en
	}
	if cv := bindingParam(inner, "Converter="); cv != "" {
		sp.converterKey = extractKey(cv) // {StaticResource key} или голый key
	}
	if rs := bindingParam(inner, "RelativeSource="); rs != "" {
		// {RelativeSource Self} → привязка к собственному свойству элемента.
		if strings.Contains(rs, "Self") {
			sp.relativeSelf = true
		}
	}
	if i := strings.Index(inner, "StringFormat="); i >= 0 {
		sf := strings.TrimSpace(inner[i+len("StringFormat="):])
		sf = strings.TrimSuffix(sf, "}")
		sp.stringFormat = strings.Trim(sf, " '\"")
	}
	return sp
}

// bindingParam извлекает значение параметра key= из тела binding (до запятой).
// Учитывает значения в фигурных скобках ({StaticResource ...}).
func bindingParam(inner, key string) string {
	i := strings.Index(inner, key)
	if i < 0 {
		return ""
	}
	rest := strings.TrimSpace(inner[i+len(key):])
	if strings.HasPrefix(rest, "{") {
		if j := strings.IndexByte(rest, '}'); j >= 0 {
			return strings.TrimSpace(rest[:j+1])
		}
	}
	if j := strings.IndexByte(rest, ','); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}

// dgridGetProperty — обёртка над datagrid.GetPropertyValue для других файлов пакета.
func dgridGetProperty(obj interface{}, path string) (interface{}, bool) {
	return dgridPkg.GetPropertyValue(obj, path)
}

// resolveBindingFull разрешает {Binding ...} против ctx одноразово,
// применяя StringFormat (для значений внутри DataTemplate и начальных значений).
func resolveBindingFull(expr string, ctx interface{}) string {
	if ctx == nil {
		return ""
	}
	spec := parseBindingSpec(expr)
	if spec.path == "" {
		return ""
	}
	v, ok := dgridPkg.GetPropertyValue(ctx, spec.path)
	if !ok {
		return ""
	}
	return formatBindingValue(applyConvert(spec, v), spec)
}

// formatBindingValue форматирует значение модели для UI (StringFormat или %v).
func formatBindingValue(v interface{}, spec bindingSpec) string {
	if spec.stringFormat != "" && strings.Contains(spec.stringFormat, "%") {
		return fmt.Sprintf(spec.stringFormat, v)
	}
	return fmt.Sprintf("%v", v)
}

// parseXAMLBool — true/1/yes/on → true.
func parseXAMLBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on", "checked":
		return true
	}
	return false
}
