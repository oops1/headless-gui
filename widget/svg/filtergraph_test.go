package svg

import (
	"bytes"
	"encoding/xml"
	"image"
	"image/color"
	"testing"
)

// ── терпимый разбор ──────────────────────────────────────────────────────────

func TestParse_Lenient(t *testing.T) {
	cases := map[string]string{
		"html-сущность в тексте": `<svg viewBox="0 0 8 8"><title>a&nbsp;b &copy;</title><rect width="8" height="8"/></svg>`,
		"голый амперсанд":        `<svg viewBox="0 0 8 8"><desc>a & b</desc><rect width="8" height="8"/></svg>`,
		"сущность в атрибуте":    `<svg viewBox="0 0 8 8"><rect width="8" height="8" data-x="a&nbsp;b"/></svg>`,
		"сущность из DOCTYPE": `<!DOCTYPE svg [<!ENTITY ns_svg "http://www.w3.org/2000/svg">]>
<svg xmlns="&ns_svg;" viewBox="0 0 8 8"><rect width="8" height="8"/></svg>`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			doc, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("терпимый разбор не должен падать: %v", err)
			}
			if len(doc.Shapes) != 1 {
				t.Errorf("shapes=%d, want 1", len(doc.Shapes))
			}
		})
	}
}

// Корректный файл разбирается так же, как строгим декодером; настоящая
// ошибка (незакрытый элемент) по-прежнему ошибка.
func TestParse_LenientKeepsStrictResultAndErrors(t *testing.T) {
	const good = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><g fill="#f00"><rect width="4" height="8"/><path d="M4 0h4v8H4z" fill="#00f"/></g></svg>`
	doc, err := Parse([]byte(good))
	if err != nil || len(doc.Shapes) != 2 {
		t.Fatalf("shapes=%d err=%v", len(doc.Shapes), err)
	}
	if _, err := Parse([]byte(`<svg viewBox="0 0 8 8"><g><rect width="8" height="8"/></svg>`)); err == nil {
		t.Error("незакрытый <g> должен остаться ошибкой")
	}
	if _, err := Parse([]byte(`not xml at all`)); err == nil {
		t.Error("не XML должен остаться ошибкой")
	}
}

// Объявления сущностей одного документа не просачиваются в общую таблицу
// HTML-сущностей и в другие документы.
func TestParse_DoctypeEntitiesDoNotLeak(t *testing.T) {
	_, err := Parse([]byte(`<!DOCTYPE svg [<!ENTITY zzleak "http://www.w3.org/2000/svg">]><svg xmlns="&zzleak;" viewBox="0 0 8 8"><rect width="8" height="8"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := xml.HTMLEntity["zzleak"]; ok {
		t.Error("сущность документа попала в общую таблицу xml.HTMLEntity")
	}
	doc, err := Parse([]byte(`<svg xmlns="&zzleak;" viewBox="0 0 8 8"><rect width="8" height="8"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Shapes) != 0 {
		t.Error("чужое пространство имён не должно рисоваться")
	}
}

// ── фильтр с неизвестным примитивом ──────────────────────────────────────────

// feTurbulence граф не умеет: фильтр пропускается целиком, а не остаётся
// «только размытием» (раньше тень из feOffset+feComposite превращалась в
// размытую копию самого элемента).
func TestFilter_UnknownPrimitiveSkipsWholeFilter(t *testing.T) {
	const body = `<rect x="14" y="14" width="12" height="12" fill="#000"`
	plain := render(t, `<svg viewBox="0 0 40 40">`+body+`/></svg>`, 40, 40)
	unk := render(t, `<svg viewBox="0 0 40 40"><defs><filter id="f"><feGaussianBlur stdDeviation="3"/><feTurbulence baseFrequency="0.1"/></filter></defs>`+body+` filter="url(#f)"/></svg>`, 40, 40)
	if !bytes.Equal(plain.Pix, unk.Pix) {
		t.Error("фильтр с неизвестным примитивом должен пропускаться целиком")
	}
	// ...и для группы тоже
	grp := render(t, `<svg viewBox="0 0 40 40"><defs><filter id="f"><feGaussianBlur stdDeviation="3"/><feMorphology radius="1"/></filter></defs><g filter="url(#f)">`+body+`/></g></svg>`, 40, 40)
	if !bytes.Equal(plain.Pix, grp.Pix) {
		t.Error("то же для <g filter>")
	}
}

// ── тень из графа ────────────────────────────────────────────────────────────

const shadowFilter = `<defs><filter id="s" x="-50%" y="-50%" width="200%" height="200%">
<feGaussianBlur in="SourceAlpha" stdDeviation="1"/>
<feOffset dx="6" dy="6" result="o"/>
<feFlood flood-color="#ff0000" flood-opacity="0.5"/>
<feComposite in2="o" operator="in"/>
<feMerge><feMergeNode/><feMergeNode in="SourceGraphic"/></feMerge>
</filter></defs>`

func TestFilter_OffsetCompositeMergeShadow(t *testing.T) {
	img := render(t, `<svg viewBox="0 0 40 40">`+shadowFilter+`<rect x="8" y="8" width="14" height="14" fill="#0000ff" filter="url(#s)"/></svg>`, 40, 40)
	// сам прямоугольник — синий, не размытый
	if c := img.RGBAAt(12, 12); !near(c, 0, 0, 255, 255, 3) {
		t.Errorf("элемент поверх тени: %v", c)
	}
	// справа-снизу от него — красноватая полупрозрачная тень
	c := img.RGBAAt(25, 25)
	if c.A < 90 || c.A > 140 || c.R < 90 || c.B != 0 || c.G != 0 {
		t.Errorf("тень (красная, ~50%%): %v", c)
	}
	// слева-сверху ничего нет: тень сдвинута, а не размыта копией элемента
	if c := img.RGBAAt(5, 5); c.A != 0 {
		t.Errorf("слева-сверху пусто: %v", c)
	}
	if c := img.RGBAAt(7, 15); c.A != 0 {
		t.Errorf("слева от элемента пусто (раньше там была размытая кайма): %v", c)
	}
}

func TestFilter_ShadowOnGroupAndScaledTransform(t *testing.T) {
	// transform масштабирует сдвиг: dx=3 при scale(2) — 6 единиц viewBox
	img := render(t, `<svg viewBox="0 0 40 40">`+`<defs><filter id="s"><feOffset in="SourceAlpha" dx="3" dy="3"/><feMerge><feMergeNode/><feMergeNode in="SourceGraphic"/></feMerge></filter></defs>`+
		`<g transform="scale(2)" filter="url(#s)"><rect x="4" y="4" width="7" height="7" fill="#00ff00"/></g></svg>`, 40, 40)
	if c := img.RGBAAt(10, 10); !near(c, 0, 255, 0, 255, 3) {
		t.Errorf("элемент: %v", c)
	}
	// тень чёрная (SourceAlpha), смещённая на 6 px: выступает справа от x=22
	if c := img.RGBAAt(26, 26); !near(c, 0, 0, 0, 255, 3) {
		t.Errorf("тень со сдвигом 6: %v", c)
	}
	if c := img.RGBAAt(30, 30); c.A != 0 {
		t.Errorf("за тенью пусто: %v", c)
	}
}

func TestFilter_DropShadow(t *testing.T) {
	img := render(t, `<svg viewBox="0 0 40 40"><defs><filter id="d"><feDropShadow dx="5" dy="5" stdDeviation="0" flood-color="#000" flood-opacity="0.5"/></filter></defs>`+
		`<rect x="6" y="6" width="12" height="12" fill="#ffffff" filter="url(#d)"/></svg>`, 40, 40)
	if c := img.RGBAAt(10, 10); !near(c, 255, 255, 255, 255, 2) {
		t.Errorf("элемент: %v", c)
	}
	if c := img.RGBAAt(21, 21); c.A < 120 || c.A > 135 {
		t.Errorf("тень 50%%: %v", c)
	}
}

// Именованные result и вход BackgroundImageFix из экспорта Figma.
func TestFilter_FigmaStyleChain(t *testing.T) {
	img := render(t, `<svg viewBox="0 0 40 40"><defs><filter id="f" x="0" y="0" width="40" height="40" filterUnits="userSpaceOnUse">
<feFlood flood-opacity="0" result="BackgroundImageFix"/>
<feColorMatrix in="SourceAlpha" type="matrix" values="0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 127 0" result="hardAlpha"/>
<feOffset dy="4"/>
<feGaussianBlur stdDeviation="0"/>
<feColorMatrix type="matrix" values="0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0.25 0"/>
<feBlend mode="normal" in2="BackgroundImageFix" result="effect1_dropShadow"/>
<feBlend mode="normal" in="SourceGraphic" in2="effect1_dropShadow" result="shape"/>
</filter></defs><rect x="8" y="8" width="16" height="16" fill="#ff0000" filter="url(#f)"/></svg>`, 40, 40)
	if c := img.RGBAAt(16, 16); !near(c, 255, 0, 0, 255, 2) {
		t.Errorf("элемент: %v", c)
	}
	if c := img.RGBAAt(16, 26); c.A < 55 || c.A > 72 || c.R != 0 {
		t.Errorf("тень 25%% под элементом: %v", c)
	}
	if c := img.RGBAAt(16, 5); c.A != 0 {
		t.Errorf("над элементом пусто: %v", c)
	}
}

func TestFilter_UnresolvedInputSkipsFilter(t *testing.T) {
	const body = `<rect x="14" y="14" width="12" height="12" fill="#000"`
	plain := render(t, `<svg viewBox="0 0 40 40">`+body+`/></svg>`, 40, 40)
	for name, f := range map[string]string{
		"BackgroundImage":   `<feOffset in="BackgroundImage" dx="3"/>`,
		"нет такого result": `<feOffset in="nope" dx="3"/>`,
		"objectBoundingBox": `<feOffset dx="0.2"/>`,
	} {
		attr := ""
		if name == "objectBoundingBox" {
			attr = ` primitiveUnits="objectBoundingBox"`
		}
		img := render(t, `<svg viewBox="0 0 40 40"><defs><filter id="f"`+attr+`>`+f+`</filter></defs>`+body+` filter="url(#f)"/></svg>`, 40, 40)
		if !bytes.Equal(plain.Pix, img.Pix) {
			t.Errorf("%s: фильтр должен пропускаться", name)
		}
	}
}

func TestFilter_ArithmeticAndBlend(t *testing.T) {
	a := floodRGBA(image.Rect(0, 0, 2, 1), color.RGBA{255, 0, 0, 255})
	b := floodRGBA(image.Rect(0, 0, 2, 1), color.RGBA{0, 0, 255, 255})
	got := compositeRGBA(a, b, "arithmetic", [4]float64{0, 0.5, 0.5, 0})
	if p := got.Pix[:4]; p[0] != 128 || p[2] != 128 || p[3] != 255 {
		t.Errorf("arithmetic: %v", p)
	}
	if p := compositeRGBA(a, b, "in", [4]float64{}).Pix[:4]; p[0] != 255 || p[3] != 255 {
		t.Errorf("in: %v", p)
	}
	if p := compositeRGBA(a, b, "out", [4]float64{}).Pix[:4]; p[3] != 0 {
		t.Errorf("out: %v", p)
	}
	if p := blendRGBA(a, b, "multiply").Pix[:4]; p[0] != 0 || p[2] != 0 || p[3] != 255 {
		t.Errorf("multiply красного на синий: %v", p)
	}
	if p := blendRGBA(a, b, "screen").Pix[:4]; p[0] != 255 || p[2] != 255 {
		t.Errorf("screen: %v", p)
	}
}

func TestFilter_ShiftFractional(t *testing.T) {
	src := floodRGBA(image.Rect(0, 0, 4, 1), color.RGBA{0, 0, 0, 255})
	for i := 4; i < len(src.Pix); i++ {
		src.Pix[i] = 0 // чёрный только первый пиксель
	}
	out := shiftRGBA(src, 1.5, 0)
	if out.Pix[3] != 0 || out.Pix[7] != 128 || out.Pix[11] != 128 || out.Pix[15] != 0 {
		t.Errorf("дробный сдвиг: %v", out.Pix)
	}
}
