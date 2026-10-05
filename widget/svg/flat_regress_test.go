package svg

import (
	"crypto/sha256"
	"encoding/hex"
	"image/color"
	"testing"
)

// flatCases — документы без градиентов/clip/mask/use: их растр не должен
// меняться от появления этих возможностей. Хеши сняты ДО их добавления.
var flatCases = map[string]string{
	"nonzero": iconSrc(12, false),
	"evenodd": iconSrc(12, true),
	"shapes": `<svg viewBox="0 0 24 24"><g fill="#3366ff" opacity="0.8"><rect x="2" y="2" width="10" height="8" rx="2"/>` +
		`<circle cx="16" cy="14" r="5" fill-opacity="0.5"/></g>` +
		`<path d="M3 20 L12 14 L21 20" fill="none" stroke="#ff0000" stroke-width="2"/>` +
		`<polygon points="1,1 5,1 3,4" style="fill:currentColor"/></svg>`,
}

func rasterHash(doc *Document, w, h int, cur color.RGBA, tint bool) string {
	img := doc.Rasterize(w, h, cur, tint)
	sum := sha256.Sum256(img.Pix)
	return hex.EncodeToString(sum[:8])
}

var flatWant = map[string][2]string{
	"shapes":  {"8079e8f1a817de7a", "40cf343b200a1188"},
	"nonzero": {"affaa2a00c131cf4", "2730ebecd027f405"},
	"evenodd": {"baaeec1dbd0948ab", "747a597464e35625"},
}

func TestFlatRasterUnchanged(t *testing.T) {
	for name, src := range flatCases {
		doc, err := Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		for i, tint := range []bool{false, true} {
			got := rasterHash(doc, 48, 48, color.RGBA{10, 200, 30, 255}, tint)
			if got != flatWant[name][i] {
				t.Errorf("%s tint=%v: хеш растра %s, ожидался %s (плоские SVG изменились)",
					name, tint, got, flatWant[name][i])
			}
		}
	}
}
