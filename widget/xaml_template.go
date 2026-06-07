// xaml_template.go — ControlTemplate / ContentPresenter / TemplateBinding (P0).
//
// Реализовано как пред-обработка дерева: ContentControl с шаблоном (Template=
// "{StaticResource ...}" или <ContentControl.Template><ControlTemplate>)
// превращается в дерево визуала шаблона. ContentPresenter заменяется
// содержимым контрола, а {TemplateBinding Prop} / {Binding RelativeSource=
// {RelativeSource TemplatedParent}, Path=Prop} — значениями свойств контрола.
package widget

import "strings"

// expandControlTemplate раскрывает ControlTemplate для ContentControl.
// Возвращает true, если шаблон найден и применён (el заменён деревом шаблона).
func (env *xamlEnv) expandControlTemplate(el *xElement) bool {
	tmpl := env.findTemplate(el)
	if tmpl == nil {
		return false
	}
	root := firstElementChild(tmpl)
	if root == nil {
		return false
	}

	// Свойства templated parent (для TemplateBinding) и содержимое.
	parent := el.attrs
	contentText := el.attr("Content")
	var content []xElement
	for i := range el.Children {
		c := el.Children[i]
		if strings.Contains(c.Tag, ".") {
			continue // property element (Template, Content и т.п.)
		}
		content = append(content, c)
	}

	clone := cloneXElement(root)
	applyTemplateNode(&clone, parent, content, contentText)

	// Переносим layout-атрибуты контрола на корень шаблона (если не заданы).
	for _, k := range []string{
		"Name", "x:Name", "Width", "Height", "Left", "Top", "Right", "Bottom",
		"Canvas.Left", "Canvas.Top", "Canvas.Right", "Canvas.Bottom",
		"Grid.Row", "Grid.Column", "Grid.RowSpan", "Grid.ColumnSpan",
		"Margin", "HorizontalAlignment", "VerticalAlignment", "DockPanel.Dock",
	} {
		if v, ok := el.attrs[k]; ok {
			if _, has := clone.attrs[k]; !has {
				clone.attrs[k] = v
			}
		}
	}

	el.Tag = clone.Tag
	el.attrs = clone.attrs
	el.Children = clone.Children
	el.Text = clone.Text
	return true
}

// findTemplate ищет ControlTemplate: Template="{StaticResource key}" или
// <X.Template><ControlTemplate>.
func (env *xamlEnv) findTemplate(el *xElement) *xElement {
	if t := el.attr("Template"); t != "" {
		if key := extractKey(t); key != "" {
			if tmpl, ok := env.templates[key]; ok {
				return tmpl
			}
		}
	}
	for i := range el.Children {
		c := &el.Children[i]
		if !strings.HasSuffix(strings.ToLower(c.Tag), ".template") {
			continue
		}
		for j := range c.Children {
			if strings.EqualFold(c.Children[j].Tag, "ControlTemplate") {
				return &c.Children[j]
			}
		}
	}
	return nil
}

// firstElementChild возвращает первый дочерний элемент-виджет (не property).
func firstElementChild(el *xElement) *xElement {
	for i := range el.Children {
		if !strings.Contains(el.Children[i].Tag, ".") {
			return &el.Children[i]
		}
	}
	return nil
}

// applyTemplateNode рекурсивно резолвит TemplateBinding в атрибутах и заменяет
// ContentPresenter содержимым контрола.
func applyTemplateNode(node *xElement, parent map[string]string, content []xElement, contentText string) {
	for k, v := range node.attrs {
		if nv, ok := resolveTemplateBinding(v, parent); ok {
			node.attrs[k] = nv
		}
	}
	var kids []xElement
	for i := range node.Children {
		ch := node.Children[i]
		if strings.EqualFold(ch.Tag, "ContentPresenter") {
			if len(content) > 0 {
				kids = append(kids, content...)
			} else if contentText != "" {
				kids = append(kids, xElement{Tag: "TextBlock", attrs: map[string]string{"Text": contentText}})
			}
			continue
		}
		applyTemplateNode(&ch, parent, content, contentText)
		kids = append(kids, ch)
	}
	node.Children = kids
}

// resolveTemplateBinding раскрывает {TemplateBinding Prop} и
// {Binding RelativeSource={RelativeSource TemplatedParent}, Path=Prop}.
func resolveTemplateBinding(val string, parent map[string]string) (string, bool) {
	v := strings.TrimSpace(val)
	if len(v) < 2 || v[0] != '{' || !strings.HasSuffix(v, "}") {
		return val, false
	}
	inner := strings.TrimSpace(v[1 : len(v)-1])
	switch {
	case strings.HasPrefix(inner, "TemplateBinding"):
		prop := strings.TrimSpace(strings.TrimPrefix(inner, "TemplateBinding"))
		return parent[prop], true
	case strings.HasPrefix(inner, "Binding") && strings.Contains(inner, "TemplatedParent"):
		prop := bindingParam(inner, "Path=")
		if prop == "" {
			// первый токен после Binding
			rest := strings.TrimSpace(strings.TrimPrefix(inner, "Binding"))
			if i := strings.IndexByte(rest, ','); i >= 0 {
				rest = rest[:i]
			}
			prop = strings.TrimSpace(rest)
		}
		return parent[prop], true
	}
	return val, false
}
