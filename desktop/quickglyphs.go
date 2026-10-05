// quickglyphs.go — значки быстрых настроек Windows 11, нарисованные векторно.
//
// Шеврон, карандаш, шестерёнка, громкость, яркость: у движка нет набора
// иконок, который гарантированно лежит у каждого потребителя, а растровый
// значок 20 px на экране 200 % мылится. SVG растеризуется в физическом размере
// (widget.DrawSVG) и кэшируется самим документом, цвет — currentColor стиля.
package desktop

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"sync"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/svg"
)

// qsGlyph — имя значка панели.
type qsGlyph string

const (
	qsGlyphChevron qsGlyph = "chevron"  // › на плитке и у громкости
	qsGlyphBack    qsGlyph = "back"     // ‹ вложенной панели
	qsGlyphPencil  qsGlyph = "pencil"   // «Изменить»
	qsGlyphGear    qsGlyph = "gear"     // «Параметры»
	qsGlyphSun     qsGlyph = "sun"      // яркость
	qsGlyphVolMute qsGlyph = "vol.mute" // звук выключен
	qsGlyphVol0    qsGlyph = "vol.0"    // тихо
	qsGlyphVol1    qsGlyph = "vol.1"
	qsGlyphVol2    qsGlyph = "vol.2"
	qsGlyphGrip    qsGlyph = "grip" // ручка перетаскивания в режиме правки
	qsGlyphBatPlug qsGlyph = "plug" // молния: батарея на зарядке
)

// qsSVG оборачивает тело в svg с общими для линейных значков параметрами.
func qsSVG(body string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" ` +
		`stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`
}

// qsGearPath строит контур шестерёнки: восемь зубцов вокруг центра.
func qsGearPath() string {
	const teeth, rOut, rIn = 8, 10.0, 7.6
	var b strings.Builder
	for i := 0; i < teeth; i++ {
		a := float64(i) * 2 * math.Pi / teeth
		step := 2 * math.Pi / teeth
		pts := [4][2]float64{
			{a - step*0.34, rIn}, {a - step*0.2, rOut}, {a + step*0.2, rOut}, {a + step*0.34, rIn},
		}
		for j, p := range pts {
			x := 12 + p[1]*math.Sin(p[0])
			y := 12 - p[1]*math.Cos(p[0])
			cmd := "L"
			if i == 0 && j == 0 {
				cmd = "M"
			}
			fmt.Fprintf(&b, "%s%.2f %.2f ", cmd, x, y)
		}
	}
	b.WriteString("Z")
	return b.String()
}

// qsGlyphSources — тела значков. Громкость — динамик и до двух дуг.
var qsGlyphSources = map[qsGlyph]string{
	qsGlyphChevron: `<path d="M9.5 5.5 16 12l-6.5 6.5"/>`,
	qsGlyphBack:    `<path d="M14.5 5.5 8 12l6.5 6.5"/>`,
	qsGlyphPencil:  `<path d="M4.5 19.5 5.4 15 16.2 4.2a2.1 2.1 0 0 1 3 3L8.4 18.1z"/><path d="M14.4 6 17.4 9"/>`,
	qsGlyphGear:    `<path d="` + qsGearPath() + `"/><circle cx="12" cy="12" r="3.2"/>`,
	qsGlyphSun: `<circle cx="12" cy="12" r="4"/>` +
		`<path d="M12 2.8v2.4M12 18.8v2.4M2.8 12h2.4M18.8 12h2.4M5.5 5.5l1.7 1.7M16.8 16.8l1.7 1.7M5.5 18.5l1.7-1.7M16.8 7.2l1.7-1.7"/>`,
	qsGlyphVol0:    `<path d="M4 9.5h3.2L12 5.5v13l-4.8-4H4z"/>`,
	qsGlyphVol1:    `<path d="M4 9.5h3.2L12 5.5v13l-4.8-4H4z"/><path d="M15.4 9.2a4 4 0 0 1 0 5.6"/>`,
	qsGlyphVol2:    `<path d="M4 9.5h3.2L12 5.5v13l-4.8-4H4z"/><path d="M15.4 9.2a4 4 0 0 1 0 5.6"/><path d="M18 6.6a8 8 0 0 1 0 10.8"/>`,
	qsGlyphVolMute: `<path d="M4 9.5h3.2L12 5.5v13l-4.8-4H4z"/><path d="M15.5 9.5l5 5M20.5 9.5l-5 5"/>`,
	qsGlyphGrip: `<g fill="currentColor" stroke="none"><circle cx="9" cy="6.5" r="1.5"/><circle cx="15" cy="6.5" r="1.5"/>` +
		`<circle cx="9" cy="12" r="1.5"/><circle cx="15" cy="12" r="1.5"/>` +
		`<circle cx="9" cy="17.5" r="1.5"/><circle cx="15" cy="17.5" r="1.5"/></g>`,
	qsGlyphBatPlug: `<path d="M13 3 6.5 13.2h5L10.5 21 17.5 10.6h-5z"/>`,
}

var (
	qsGlyphMu   sync.Mutex
	qsGlyphDocs = map[qsGlyph]*svg.Document{}
)

// qsGlyphDoc возвращает разобранный документ значка (разбирается один раз).
func qsGlyphDoc(g qsGlyph) *svg.Document {
	qsGlyphMu.Lock()
	defer qsGlyphMu.Unlock()
	if d, ok := qsGlyphDocs[g]; ok {
		return d
	}
	var doc *svg.Document
	if body, ok := qsGlyphSources[g]; ok {
		doc, _ = svg.Parse([]byte(qsSVG(body)))
	}
	qsGlyphDocs[g] = doc
	return doc
}

// drawQSGlyph рисует значок g в квадрате стороной side по центру r цветом col.
func drawQSGlyph(ctx widget.DrawContext, g qsGlyph, r image.Rectangle, side int, col color.RGBA) {
	if r.Empty() || side <= 0 || col.A == 0 {
		return
	}
	doc := qsGlyphDoc(g)
	if doc == nil {
		return
	}
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	box := image.Rect(cx-side/2, cy-side/2, cx-side/2+side, cy-side/2+side)
	widget.DrawSVG(ctx, doc, box, col, false)
}

// qsVolumeGlyph выбирает значок громкости по уровню.
func qsVolumeGlyph(level float64, muted bool) qsGlyph {
	switch {
	case muted:
		return qsGlyphVolMute
	case level <= 0:
		return qsGlyphVol0
	case level < 0.34:
		return qsGlyphVol1
	default:
		return qsGlyphVol2
	}
}
