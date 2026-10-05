package svg

import (
	"sort"
	"strings"
)

// cssRule — одно правило <style>: простой селектор и его объявления.
type cssRule struct {
	tag   string // "" — любой тег
	id    string
	class []string
	spec  int // специфичность: id=100, класс=10, тег=1
	order int // порядок в таблице стилей — при равной специфичности побеждает поздний
	decl  map[string]string
}

// styleSheet — таблица стилей документа. Поддержан небольшой, но ходовый
// у значков подмножество CSS: селекторы тега, .класса, #id, их сочетания без
// пробелов (rect.st0, g#a.b), «*» и списки через запятую. Комбинаторы
// (потомок, дочерний, +, ~), атрибутные селекторы и псевдоклассы не
// поддержаны — такое правило пропускается целиком, как и @-правила.
type styleSheet struct {
	rules []cssRule
}

// parseStyleSheet разбирает текст всех <style> документа.
func parseStyleSheet(css string) *styleSheet {
	ss := &styleSheet{}
	css = stripCSSComments(css)
	order := 0
	for len(css) > 0 {
		css = strings.TrimSpace(css)
		if css == "" {
			break
		}
		if css[0] == '@' {
			// @media/@font-face/@keyframes: пропускаем до конца блока (или до ';').
			css = skipAtRule(css)
			continue
		}
		open := strings.IndexByte(css, '{')
		if open < 0 {
			break
		}
		sel := css[:open]
		body := css[open+1:]
		end := strings.IndexByte(body, '}')
		if end < 0 {
			end = len(body)
			css = ""
		} else {
			css = body[end+1:]
		}
		decl := parseDeclarations(body[:end])
		if len(decl) == 0 {
			continue
		}
		for _, one := range strings.Split(sel, ",") {
			r, ok := parseSimpleSelector(strings.TrimSpace(one))
			if !ok {
				continue
			}
			r.order = order
			order++
			r.decl = decl
			ss.rules = append(ss.rules, r)
		}
	}
	// Стабильная сортировка по (специфичность, порядок): применяя правила
	// подряд, получаем верный приоритет.
	sort.SliceStable(ss.rules, func(i, j int) bool {
		a, b := ss.rules[i], ss.rules[j]
		if a.spec != b.spec {
			return a.spec < b.spec
		}
		return a.order < b.order
	})
	return ss
}

func stripCSSComments(s string) string {
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i+2:], "*/")
		if j < 0 {
			return s[:i]
		}
		s = s[:i] + " " + s[i+2+j+2:]
	}
}

// skipAtRule пропускает @-правило: либо до ';', либо весь вложенный блок {…}.
func skipAtRule(s string) string {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ';':
			if depth == 0 {
				return s[i+1:]
			}
		case '{':
			depth++
		case '}':
			depth--
			if depth <= 0 {
				return s[i+1:]
			}
		}
	}
	return ""
}

// parseDeclarations разбирает «a:b; c:d». !important отбрасывается (все
// объявления из таблицы стилей и так сильнее атрибутов представления).
func parseDeclarations(s string) map[string]string {
	var m map[string]string
	for _, d := range strings.Split(s, ";") {
		kv := strings.SplitN(d, ":", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(kv[0]))
		v := strings.TrimSpace(kv[1])
		if i := strings.Index(v, "!important"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		if k == "" || v == "" {
			continue
		}
		if m == nil {
			m = map[string]string{}
		}
		m[k] = v
	}
	return m
}

// parseSimpleSelector разбирает «tag.class#id» без комбинаторов.
func parseSimpleSelector(s string) (cssRule, bool) {
	var r cssRule
	if s == "" {
		return r, false
	}
	for i := 0; i < len(s); {
		switch s[i] {
		case '.', '#':
			kind := s[i]
			j := i + 1
			for j < len(s) && isIdentByte(s[j]) {
				j++
			}
			name := s[i+1 : j]
			if name == "" {
				return r, false
			}
			if kind == '.' {
				r.class = append(r.class, name)
				r.spec += 10
			} else {
				if r.id != "" && r.id != name {
					return r, false
				}
				r.id = name
				r.spec += 100
			}
			i = j
		case '*':
			i++
		default:
			if !isIdentByte(s[i]) || i != 0 {
				return r, false // комбинатор, [attr], :pseudo, пробел
			}
			j := i
			for j < len(s) && isIdentByte(s[j]) {
				j++
			}
			r.tag = s[i:j]
			r.spec++
			i = j
		}
	}
	return r, true
}

func isIdentByte(c byte) bool {
	return c == '-' || c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

// match сообщает, подходит ли правило элементу.
func (r *cssRule) match(n *xnode) bool {
	if r.tag != "" && r.tag != n.XMLName.Local {
		return false
	}
	if r.id != "" {
		if v, _ := n.attr("id"); v != r.id {
			return false
		}
	}
	if len(r.class) > 0 {
		cl, _ := n.attr("class")
		have := strings.Fields(cl)
	next:
		for _, want := range r.class {
			for _, h := range have {
				if h == want {
					continue next
				}
			}
			return false
		}
	}
	return true
}

// declarationsFor собирает объявления таблицы стилей для элемента (слабее
// style="" самого элемента, сильнее атрибутов представления). nil — ничего.
func (ss *styleSheet) declarationsFor(n *xnode) map[string]string {
	if ss == nil {
		return nil
	}
	var out map[string]string
	for i := range ss.rules {
		r := &ss.rules[i]
		if !r.match(n) {
			continue
		}
		if out == nil {
			out = make(map[string]string, len(r.decl))
		}
		for k, v := range r.decl {
			out[k] = v
		}
	}
	return out
}
