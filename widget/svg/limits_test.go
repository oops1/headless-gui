package svg

import (
	"strings"
	"testing"
)

// Глубокая вложенность отклоняется, а не роняет стек.
func TestParse_DepthLimit(t *testing.T) {
	const n = 100000
	var b strings.Builder
	b.WriteString(`<svg viewBox="0 0 24 24">`)
	for i := 0; i < n; i++ {
		b.WriteString("<g>")
	}
	b.WriteString(`<rect width="4" height="4" fill="#fff"/>`)
	for i := 0; i < n; i++ {
		b.WriteString("</g>")
	}
	b.WriteString(`</svg>`)

	doc, err := Parse([]byte(b.String()))
	if err == nil {
		t.Fatalf("глубина %d принята, ожидалась ошибка (shapes=%d)", n, len(doc.Shapes))
	}
	if !strings.Contains(err.Error(), "вложенность") {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// Вложенность на грани лимита разбирается штатно.
func TestParse_DepthUnderLimit(t *testing.T) {
	depth := MaxDepth - 2
	var b strings.Builder
	b.WriteString(`<svg viewBox="0 0 24 24">`)
	for i := 0; i < depth; i++ {
		b.WriteString("<g>")
	}
	b.WriteString(`<rect width="4" height="4" fill="#ffffff"/>`)
	for i := 0; i < depth; i++ {
		b.WriteString("</g>")
	}
	b.WriteString(`</svg>`)

	doc, err := Parse([]byte(b.String()))
	if err != nil {
		t.Fatalf("глубина %d: %v", depth, err)
	}
	if len(doc.Shapes) != 1 {
		t.Fatalf("shapes=%d, want 1", len(doc.Shapes))
	}
}

// Данные больше MaxFileBytes отклоняются без разбора.
func TestParse_SizeLimit(t *testing.T) {
	data := make([]byte, MaxFileBytes+1)
	for i := range data {
		data[i] = ' '
	}
	copy(data, []byte(`<svg viewBox="0 0 24 24"></svg>`))
	if _, err := Parse(data); err == nil {
		t.Fatal("данные больше лимита приняты")
	}
}

// Вложенные группы наследуют состояние как прежде.
func TestParse_NestedGroupsInherit(t *testing.T) {
	const src = `<svg viewBox="0 0 24 24"><g fill="#ff0000"><g opacity="0.5">` +
		`<rect x="2" y="2" width="8" height="8"/></g></g></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Shapes) != 1 {
		t.Fatalf("shapes=%d", len(doc.Shapes))
	}
	sh := doc.Shapes[0]
	if sh.Fill.R != 255 || sh.Fill.G != 0 {
		t.Errorf("fill=%v", sh.Fill)
	}
	if sh.FillOpacity > 0.6 || sh.FillOpacity < 0.4 {
		t.Errorf("opacity=%v, want ~0.5", sh.FillOpacity)
	}
}

// Некорректный XML по-прежнему возвращает ошибку.
func TestParse_BadXML(t *testing.T) {
	if _, err := Parse([]byte(`<svg><g></svg>`)); err == nil {
		t.Fatal("незакрытый тег принят")
	}
	if _, err := Parse([]byte(`просто текст`)); err == nil {
		t.Fatal("документ без корня принят")
	}
}
