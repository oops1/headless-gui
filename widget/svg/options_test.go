package svg

import (
	"image/color"
	"testing"
)

// Документ с атрибутами нового режима (join/cap/dash/opacity группы) в режиме
// по умолчанию рисуется ровно так же, как без них: побитно.
func TestOptions_DefaultIgnoresNewAttributesBitwise(t *testing.T) {
	const plain = `<svg viewBox="0 0 24 24"><g><path d="M3 20 L12 4 L21 20" fill="none" stroke="#ff0000" stroke-width="3"/>` +
		`<circle cx="12" cy="14" r="5" fill="#3366ff"/></g></svg>`
	const fancy = `<svg viewBox="0 0 24 24"><g opacity="1" stroke-linejoin="round" stroke-linecap="square"><path d="M3 20 L12 4 L21 20" fill="none" stroke="#ff0000" stroke-width="3" stroke-miterlimit="2" stroke-dasharray="2 1" stroke-dashoffset="1"/>` +
		`<circle cx="12" cy="14" r="5" fill="#3366ff"/></g></svg>`
	a, _ := Parse([]byte(plain))
	b, _ := Parse([]byte(fancy))
	// Круг с пунктиром стартует с другой точки только если пунктир на нём самом.
	for _, tint := range []bool{false, true} {
		ha := rasterHash(a, 48, 48, color.RGBA{10, 200, 30, 255}, tint)
		hb := rasterHash(b, 48, 48, color.RGBA{10, 200, 30, 255}, tint)
		if ha != hb {
			t.Errorf("tint=%v: атрибуты нового режима изменили растр по умолчанию: %s != %s", tint, ha, hb)
		}
	}
}

func TestOptions_DefaultIsZeroAndSetDefaultApplies(t *testing.T) {
	if DefaultOptions() != (Options{}) {
		t.Fatalf("по умолчанию опции должны быть нулевыми: %+v", DefaultOptions())
	}
	doc, _ := Parse([]byte(`<svg viewBox="0 0 48 48"><path d="M12 24 H36" stroke="#000" stroke-width="8" stroke-linecap="square"/></svg>`))
	before := doc.Rasterize(48, 48, color.RGBA{A: 255}, false)
	SetDefaultOptions(Options{StrokeJoins: true})
	defer SetDefaultOptions(Options{})
	after := doc.Rasterize(48, 48, color.RGBA{A: 255}, false)
	if a(before, 10, 24) != 0 || a(after, 10, 24) != 255 {
		t.Errorf("общие опции: до=%d после=%d", a(before, 10, 24), a(after, 10, 24))
	}
	if DefaultOptions() != (Options{StrokeJoins: true}) {
		t.Error("DefaultOptions не вернул установленное")
	}
}

func TestOptions_DocumentOverridesDefaultAndCacheKeysOptions(t *testing.T) {
	doc, _ := Parse([]byte(`<svg viewBox="0 0 48 48"><path d="M12 24 H36" stroke="#000" stroke-width="8" stroke-linecap="square"/></svg>`))
	cur := color.RGBA{A: 255}
	legacy := doc.RasterizeCached(48, 48, cur, false)
	doc.SetOptions(Options{StrokeJoins: true})
	precise := doc.RasterizeCached(48, 48, cur, false)
	if legacy == precise || a(legacy, 10, 24) != 0 || a(precise, 10, 24) != 255 {
		t.Fatal("кэш отдал картинку другого режима")
	}
	if doc.Options() != (Options{StrokeJoins: true}) {
		t.Error("Document.Options")
	}
	if again := doc.RasterizeCached(48, 48, cur, false); again != precise {
		t.Error("тот же режим должен браться из кэша")
	}
	// явный режим на один вызов не зависит от документного
	if img := doc.RasterizeCachedWith(48, 48, cur, false, Options{}); a(img, 10, 24) != 0 {
		t.Error("RasterizeCachedWith(Options{}) должен дать прежнюю обводку")
	}
	doc.UseDefaultOptions()
	if got := doc.RasterizeCached(48, 48, cur, false); a(got, 10, 24) != 0 {
		t.Error("UseDefaultOptions: вернулся прежний режим")
	}
}

func TestOptions_RenderWith(t *testing.T) {
	doc, _ := Parse([]byte(`<svg viewBox="0 0 48 48"><path d="M12 24 H36" stroke="#000" stroke-width="8" stroke-linecap="square"/></svg>`))
	if RenderWith(nil, 8, 8, color.RGBA{}, PreciseOptions) != nil || RenderWith(doc, 0, 8, color.RGBA{}, PreciseOptions) != nil {
		t.Error("nil/нулевой размер → nil")
	}
	if a(RenderWith(doc, 48, 48, color.RGBA{}, PreciseOptions), 10, 24) != 255 {
		t.Error("RenderWith(PreciseOptions): квадратный колпачок не нарисован")
	}
	if a(Render(doc, 48, 48, color.RGBA{}), 10, 24) != 0 {
		t.Error("Render без опций остался прежним")
	}
}
