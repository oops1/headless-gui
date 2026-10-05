package svg

import (
	"image/color"
	"testing"
)

// Странные значения в градиентах, фильтрах, масках и ссылках не должны ронять
// разбор и растеризацию.
func TestRobust_WeirdValues(t *testing.T) {
	cases := map[string]string{
		"nan-градиент":      `<svg viewBox="0 0 8 8"><defs><linearGradient id="g" x1="NaN" x2="Inf" gradientTransform="matrix(Inf 0 0 NaN 0 0)"><stop offset="0" stop-color="#f00"/><stop offset="1" stop-color="#00f"/></linearGradient></defs><rect width="8" height="8" fill="url(#g)"/></svg>`,
		"r=0":               `<svg viewBox="0 0 8 8"><defs><radialGradient id="g" r="0"><stop offset="0" stop-color="#f00"/><stop offset="1" stop-color="#00f"/></radialGradient></defs><rect width="8" height="8" fill="url(#g)"/></svg>`,
		"вырожд.матр.":      `<svg viewBox="0 0 8 8"><defs><linearGradient id="g" gradientTransform="scale(0)"><stop offset="0" stop-color="#f00"/><stop offset="1" stop-color="#00f"/></linearGradient></defs><rect width="8" height="8" fill="url(#g)"/></svg>`,
		"огромное размытие": `<svg viewBox="0 0 8 8"><defs><filter id="f"><feGaussianBlur stdDeviation="1e30 NaN"/></filter></defs><rect width="8" height="8" filter="url(#f)"/></svg>`,
		"матрица 19":        `<svg viewBox="0 0 8 8"><defs><filter id="f"><feColorMatrix values="1 2 3"/></filter></defs><rect width="8" height="8" filter="url(#f)"/></svg>`,
		"clip-вырожд.":      `<svg viewBox="0 0 8 8"><defs><clipPath id="c" transform="scale(0)"><rect width="8" height="8"/></clipPath></defs><rect width="8" height="8" clip-path="url(#c)"/></svg>`,
		"clip на не-clip":   `<svg viewBox="0 0 8 8"><rect id="r" width="2" height="2"/><rect width="8" height="8" clip-path="url(#r)" mask="url(#r)" filter="url(#r)" fill="url(#r)"/></svg>`,
		"image без href":    `<svg viewBox="0 0 8 8"><image width="8" height="8"/><use/><use href="#нет"/></svg>`,
		"пустой style":      `<svg viewBox="0 0 8 8"><style></style><style>{{{ }}} a{ .b</style><rect width="8" height="8"/></svg>`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			doc, err := Parse([]byte(src))
			if err != nil {
				t.Fatal(err)
			}
			doc.Rasterize(16, 16, color.RGBA{A: 255}, false)
			doc.Rasterize(16, 16, color.RGBA{255, 0, 0, 255}, true)
		})
	}
}

func TestNamedColors_Extended(t *testing.T) {
	for name, want := range map[string]color.RGBA{
		"darkgray":       {169, 169, 169, 255},
		"cornflowerblue": {100, 149, 237, 255},
		"rebeccapurple":  {102, 51, 153, 255},
		"RED":            {255, 0, 0, 255},
	} {
		if got, ok := ParseColor(name); !ok || got != want {
			t.Errorf("%s → %v %v, want %v", name, got, ok, want)
		}
	}
	if len(namedColors) < 147 {
		t.Errorf("в таблице %d цветов, ожидалось не меньше 147", len(namedColors))
	}
}
