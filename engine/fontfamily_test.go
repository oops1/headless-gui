package engine

import (
	"image"
	"image/color"
	"math"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	hgfonts "github.com/oops1/headless-gui/v3/assets/fonts"
	"github.com/oops1/headless-gui/v3/widget"
)

// openSansEngine — движок со вшитым Open Sans (Light, Regular, SemiBold,
// Bold, Italic, BoldItalic).
func openSansEngine(t *testing.T) *Engine {
	t.Helper()
	eng := New(64, 64, 30)
	if err := hgfonts.Register(eng); err != nil {
		t.Fatalf("вшитые шрифты: %v", err)
	}
	return eng
}

// Вшитые шрифты видны из чужого модуля без assets/fonts в рабочем каталоге
// (тест идёт из каталога engine/, где такого каталога нет).
func TestEmbeddedFonts_RegisterWithoutWorkingDirectory(t *testing.T) {
	eng := openSansEngine(t)
	for _, name := range []string{"OpenSans", "OpenSans-Regular", "OpenSans-Light", "OpenSans-SemiBold", "OpenSans-Bold", "OpenSans-Italic", "OpenSans-BoldItalic"} {
		if !registered(eng, name) {
			t.Errorf("шрифт %q не зарегистрирован", name)
		}
	}
	if !eng.SetDefaultFont("OpenSans") {
		t.Error("вшитый Open Sans не годится в шрифты по умолчанию")
	}
}

// Семейство + вес + наклон выбирают нужный файл. Раньше «Open Sans» с весом
// 600 не значило ничего: семейство получал только Regular.
func TestFontFamily_PicksWeightAndItalic(t *testing.T) {
	eng := openSansEngine(t)
	c := eng.canvas
	face := func(family string, weight int, italic bool) *FontCache {
		return c.fontFor(widget.FontFace(family, weight, italic))
	}
	byName := func(n string) *FontCache { return c.namedFonts[n] }

	cases := []struct {
		weight int
		italic bool
		want   string
	}{
		{300, false, "OpenSans-Light"},
		{400, false, "OpenSans-Regular"},
		{600, false, "OpenSans-SemiBold"},
		{700, false, "OpenSans-Bold"},
		{400, true, "OpenSans-Italic"},
		{700, true, "OpenSans-BoldItalic"},
		// Промежуточные веса — по правилу CSS: 500 берёт 400, 200 — Light,
		// 800 — Bold, 600 курсивом — ближайший тяжелее, то есть BoldItalic.
		{500, false, "OpenSans-Regular"},
		{200, false, "OpenSans-Light"},
		{800, false, "OpenSans-Bold"},
		{600, true, "OpenSans-BoldItalic"},
	}
	for _, tc := range cases {
		// Название с пробелом и имя файла — одно семейство.
		for _, fam := range []string{"Open Sans", "OpenSans", "open sans"} {
			got := face(fam, tc.weight, tc.italic)
			want := byName(tc.want)
			if tc.want == "OpenSans-Regular" {
				// Regular зарегистрирован дважды (файл и алиас семейства) —
				// кэши разные, данные одни.
				if got != want && got != byName("OpenSans") {
					t.Errorf("%s %d italic=%v: не Regular", fam, tc.weight, tc.italic)
				}
				continue
			}
			if got != want {
				t.Errorf("%s %d italic=%v: выбран не %s", fam, tc.weight, tc.italic, tc.want)
			}
		}
	}

	// Разные веса — действительно разные шрифты.
	if face("Open Sans", 300, false) == face("Open Sans", 600, false) {
		t.Error("Light и SemiBold — один шрифт")
	}
	// Название семейства без веса — обычное начертание, не шрифт по умолчанию.
	if c.fontFor("Open Sans") == c.fontCache {
		t.Error(`"Open Sans" не найден по названию семейства`)
	}
	// Имя файла работает как раньше.
	if c.fontFor("OpenSans-Bold") != byName("OpenSans-Bold") {
		t.Error("имя файла перестало находить шрифт")
	}
}

// Пустое семейство — «шрифт по умолчанию нужного веса».
func TestFontFamily_DefaultFamilyWeights(t *testing.T) {
	eng := New(64, 64, 30)
	c := eng.canvas

	// Встроенный Go Regular жирного начертания в семействе не имеет:
	// берётся встроенный Go Bold — как у widget.Label{Bold: true}.
	if c.fontFor(widget.FontFace("", 700, false)) != c.namedFonts[widget.BuiltinFontBold] {
		t.Error("bold без семейства при шрифте Go Regular — не встроенный жирный")
	}
	if c.fontFor(widget.FontFace("", 0, true)) != c.namedFonts[widget.BuiltinFontItalic] {
		t.Error("курсив без семейства при шрифте Go Regular — не встроенный курсивный")
	}
	// Незнакомое семейство ведёт себя так же: нет шрифта — нет и спора.
	if c.fontFor(widget.FontFace("Нет Такого", 700, false)) != c.namedFonts[widget.BuiltinFontBold] {
		t.Error("bold незнакомого семейства — не встроенный жирный")
	}
	// Лёгкий вес без семейства — просто шрифт по умолчанию.
	if c.fontFor(widget.FontFace("", 300, false)) != c.fontCache {
		t.Error("Light без семейства — не шрифт по умолчанию")
	}

	// Когда по умолчанию стоит Open Sans, жирный — Open Sans Bold.
	if err := hgfonts.Register(eng); err != nil {
		t.Fatal(err)
	}
	if !eng.SetDefaultFont("OpenSans") {
		t.Fatal("SetDefaultFont")
	}
	if c.fontFor(widget.FontFace("", 700, false)) != c.namedFonts["OpenSans-Bold"] {
		t.Error("bold при шрифте по умолчанию Open Sans — не OpenSans-Bold")
	}
	if c.fontFor(widget.FontFace("", 600, false)) != c.namedFonts["OpenSans-SemiBold"] {
		t.Error("SemiBold при шрифте по умолчанию Open Sans — не OpenSans-SemiBold")
	}
	if c.fontFor(widget.FontFace("", 700, true)) != c.namedFonts["OpenSans-BoldItalic"] {
		t.Error("bold italic при шрифте по умолчанию Open Sans — не OpenSans-BoldItalic")
	}
}

// Составное имя не должно ломать обычные: опечатка в имени по-прежнему даёт
// шрифт по умолчанию, а не панику или пустоту.
func TestFontFamily_UnknownNameFallsBackToDefault(t *testing.T) {
	eng := openSansEngine(t)
	c := eng.canvas
	if c.fontFor("нет-такого") != c.fontCache {
		t.Error("незнакомое имя — не шрифт по умолчанию")
	}
	if c.fontFor("$hg_face:мусор") != c.fontCache {
		t.Error("битое составное имя — не шрифт по умолчанию")
	}
	if c.fontFor("") != c.fontCache {
		t.Error("пустое имя — не шрифт по умолчанию")
	}
}

// Свойства шрифта читаются из самого файла, а не из его имени.
func TestDescribeFont_ReadsFileMetadata(t *testing.T) {
	cases := []struct {
		file   string
		key    string
		weight int
		italic bool
	}{
		{"OpenSans-Light", "opensans", 300, false},
		{"OpenSans-SemiBold", "opensans", 600, false},
		{"OpenSans-Bold", "opensans", 700, false},
		{"OpenSans-BoldItalic", "opensans", 700, true},
		{"GolosText-SemiBold", "golostext", 600, false},
		{"Roboto-Italic", "roboto", 400, true},
	}
	for _, tc := range cases {
		data, err := readFontFile("../assets/fonts/" + tc.file + ".ttf")
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		fc := newFontCacheFromData(data, DefaultDPI)
		m := describeFont(fc, tc.file)
		if m.keys[0] != tc.key || m.weight != tc.weight || m.italic != tc.italic {
			t.Errorf("%s: ключ %q вес %d наклон %v; ждали %q %d %v",
				tc.file, m.keys[0], m.weight, m.italic, tc.key, tc.weight, tc.italic)
		}
	}
}

// Дробный кегль темы (8,5 pt) доходит до растеризатора: размер лица в
// пикселях — 8,5 · 96 / 72 = 11,33, а не 8, 9 или 11.
func TestFractionalSize_ReachesRasterizer(t *testing.T) {
	eng := openSansEngine(t)
	fc := eng.canvas.fontFor("OpenSans")
	// Подпиксельный режим: ширина линейна по кеглю и ничем не округлена.
	fc.SetSubpixel(true)
	const s = "Hamburgefonstiv Открыть меню Пуск"
	w8, w85, w9 := fc.MeasureExact(s, 8), fc.MeasureExact(s, 8.5), fc.MeasureExact(s, 9)
	if !(w8 < w85 && w85 < w9) {
		t.Fatalf("ширина не растёт с кеглем: 8 → %.2f, 8.5 → %.2f, 9 → %.2f", w8, w85, w9)
	}
	// Линейность: 8,5 лежит посередине между 8 и 9 (с запасом на кернинг).
	if mid := (w8 + w9) / 2; math.Abs(w85-mid) > 0.01*mid {
		t.Errorf("8.5 pt = %.2f px, ждали около %.2f", w85, mid)
	}
	// Высота лица: ppem = 8,5 · 96 / 72.
	m := fc.Face(8.5).Metrics()
	want := 11.3333 * (2189.0 + 600.0) / 2048.0 // ascent+descent Open Sans в долях em
	got := float64(m.Ascent+m.Descent) / 64
	if math.Abs(got-want) > 0.05 {
		t.Errorf("высота лица 8.5 pt = %.2f px, ждали %.2f", got, want)
	}
	// И через канвас (путь, которым идёт тема): MeasureTextFont для разных
	// дробных кеглей даёт разные ширины длинной строки.
	long := ""
	for i := 0; i < 20; i++ {
		long += s + " "
	}
	c := eng.canvas
	a := c.MeasureTextFont(long, 8.4, "Open Sans")
	b := c.MeasureTextFont(long, 8.6, "Open Sans")
	if a >= b {
		t.Errorf("8.4 pt → %d px, 8.6 pt → %d px: дробный кегль не доходит до измерения", a, b)
	}
}

// ─── Подпиксельное позиционирование ─────────────────────────────────────────

// MeasureExact — ширина строки без округления, пиксели (только для тестов).
func (fc *FontCache) MeasureExact(text string, sizePt float64) float64 {
	face := fc.Face(sizePt)
	var w fixed.Int26_6
	prev := rune(-1)
	for _, r := range text {
		if prev >= 0 {
			w += face.Kern(prev, r)
		}
		a, _ := face.GlyphAdvance(r)
		w += a
		prev = r
	}
	return float64(w) / 64
}

// По умолчанию подпикселя нет: шаг глифа округлён, и все прежние тексты
// рисуются теми же пикселями.
func TestSubpixel_OffByDefault(t *testing.T) {
	eng := openSansEngine(t)
	if eng.TextSubpixel() {
		t.Fatal("подпиксельный текст включён по умолчанию")
	}
	fc := eng.canvas.fontFor("OpenSans")
	adv, _ := fc.Face(8.5).GlyphAdvance('i')
	if adv&63 != 0 {
		t.Errorf("шаг глифа %v дробный при выключенном подпикселе", adv)
	}
}

func TestSubpixel_FractionalAdvanceAndWidth(t *testing.T) {
	eng := openSansEngine(t)
	str := "iiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiii" // 40 узких букв
	hinted := eng.canvas.MeasureTextFont(str, 8.5, "OpenSans")

	eng.SetTextSubpixel(true)
	if !eng.TextSubpixel() {
		t.Fatal("SetTextSubpixel(true) не включил режим")
	}
	sub := eng.canvas.MeasureTextFont(str, 8.5, "OpenSans")

	// Эталон: шаг 'i' без округления из самого шрифта.
	data, _ := readFontFile("../assets/fonts/OpenSans-Regular.ttf")
	f, err := opentype.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 8.5, DPI: 96, Hinting: font.HintingNone})
	if err != nil {
		t.Fatal(err)
	}
	adv, _ := ref.GlyphAdvance('i')
	want := float64(adv) / 64 * 40
	if math.Abs(float64(sub)-want) > 1 {
		t.Errorf("подпиксель: ширина %d, эталон %.2f", sub, want)
	}
	// Целочисленный шаг уходит от настоящего на пиксели — в этом и дефект.
	if math.Abs(float64(hinted)-want) < 3 {
		t.Errorf("округлённая ширина %d не отличается от точной %.2f: тест не ловит дефект", hinted, want)
	}

	// Выключили — вернулось прежнее.
	eng.SetTextSubpixel(false)
	if back := eng.canvas.MeasureTextFont(str, 8.5, "OpenSans"); back != hinted {
		t.Errorf("после выключения ширина %d, было %d", back, hinted)
	}
}

// Шрифт, зарегистрированный после включения режима, наследует его.
func TestSubpixel_AppliesToLaterFonts(t *testing.T) {
	eng := New(64, 64, 30)
	eng.SetTextSubpixel(true)
	if err := hgfonts.Register(eng); err != nil {
		t.Fatal(err)
	}
	if !eng.canvas.fontFor("OpenSans-Bold").Subpixel() {
		t.Error("шрифт, добавленный после включения, остался с целым шагом")
	}
}

// Маска глифа зависит от доли пикселя, на которую сдвинуто перо.
func TestSubpixel_PhasesGiveDifferentMasks(t *testing.T) {
	eng := openSansEngine(t)
	eng.SetTextSubpixel(true)
	fc := eng.canvas.fontFor("OpenSans")
	g0 := fc.GlyphAt(8.5, 'l', 0)
	g2 := fc.GlyphAt(8.5, 'l', 2)
	if g0.mask == nil || g2.mask == nil {
		t.Fatal("нет маски")
	}
	if g0.mask.Rect == g2.mask.Rect {
		same := true
		for i := range g0.mask.Pix {
			if g0.mask.Pix[i] != g2.mask.Pix[i] {
				same = false
				break
			}
		}
		if same {
			t.Error("маски для разных долей пикселя совпали")
		}
	}
	// Фаза 0 — та же маска, что у обычного Glyph.
	if gl := fc.Glyph(8.5, 'l'); gl.offX != g0.offX || gl.mask.Rect != g0.mask.Rect {
		t.Error("Glyph и GlyphAt(…, 0) разошлись")
	}
}

func TestPenGlyph_PhaseSelection(t *testing.T) {
	eng := openSansEngine(t)
	fc := eng.canvas.fontFor("OpenSans")
	// Без подпикселя перо округляется, как и раньше.
	if _, x := penGlyph(fc, false, 8.5, 'l', fixed.Int26_6(10*64+40)); x != 11 {
		t.Errorf("целочисленный режим: x = %d, ждали 11", x)
	}
	// С подпикселем 10 px + 40/64 → четверть 10,625 → ближайшая 10,5 (фаза 2).
	if _, x := penGlyph(fc, true, 8.5, 'l', fixed.Int26_6(10*64+40)); x != 10 {
		t.Errorf("подпиксель: x = %d, ждали 10", x)
	}
	// Близко к следующему пикселю — переходим на него с фазой 0.
	if _, x := penGlyph(fc, true, 8.5, 'l', fixed.Int26_6(10*64+60)); x != 11 {
		t.Errorf("подпиксель, 10+60/64: x = %d, ждали 11", x)
	}
	// Отрицательные координаты (текст левее холста): floor, а не усечение к нулю.
	if _, x := penGlyph(fc, true, 8.5, 'l', fixed.Int26_6(-64*3+8)); x != -3 {
		t.Errorf("подпиксель, -3+8/64: x = %d, ждали -3", x)
	}
}

// Строка из одинаковых узких букв с подпикселем идёт ровно: расстояние между
// соседними штрихами равно настоящему шагу (2,8 px), а не скачет между 2 и 3.
func TestSubpixel_DrawnStrokesAreEvenlySpaced(t *testing.T) {
	eng := openSansEngine(t)
	eng.SetTextSubpixel(true)
	c := eng.canvas
	const n = 16
	str := ""
	for i := 0; i < n; i++ {
		str += "l"
	}
	c.DrawTextFont(str, 2, 2, 8.5, "OpenSans", color.RGBA{0, 0, 0, 255})
	img := c.back
	colInk := make([]float64, img.Rect.Dx())
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			colInk[x] += float64(img.RGBAAt(x, y).A) / 255
		}
	}
	// Соседние 'l' на дробном шаге могут слиться в одну полосу — поэтому
	// проверяем протяжённость всей строки: первая и последняя колонки с краской.
	first, last := -1, -1
	for x, v := range colInk {
		if v >= 0.05 {
			if first < 0 {
				first = x
			}
			last = x
		}
	}
	if first < 0 {
		t.Fatal("строка не нарисована")
	}
	data, _ := readFontFile("../assets/fonts/OpenSans-Regular.ttf")
	f, _ := opentype.Parse(data)
	ref, _ := opentype.NewFace(f, &opentype.FaceOptions{Size: 8.5, DPI: 96, Hinting: font.HintingNone})
	adv, _ := ref.GlyphAdvance('l')
	wantSpan := float64(adv) / 64 * float64(n-1)
	span := float64(last - first)
	// Округлённый шаг (3 px вместо 2,86) накопил бы за 15 букв больше трёх
	// пикселей расхождения; подпиксель держит его в пределах долей пикселя.
	if math.Abs(span-wantSpan) > 1.5 {
		t.Errorf("протяжённость строки %.1f px, ждали около %.1f", span, wantSpan)
	}
}

// inkProfile — суммы альфы по колонкам холста, без нулевых краёв.
func inkProfile(img *image.RGBA) []int {
	cols := make([]int, img.Rect.Dx())
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			cols[x] += int(img.RGBAAt(x, y).A)
		}
	}
	lo, hi := 0, len(cols)
	for lo < hi && cols[lo] == 0 {
		lo++
	}
	for hi > lo && cols[hi-1] == 0 {
		hi--
	}
	return cols[lo:hi]
}

// Второй штрих строки "ll" встаёт на дробную позицию (шаг 'l' ≈ 2,86 px).
// Без подпикселя он рисуется той же маской, что и первый, только сдвинутой на
// целое число пикселей; с подпикселем маска снимается для своей четверти
// пикселя и рисунок штриха другой.
func TestSubpixel_SecondGlyphIsRenderedAtItsPhase(t *testing.T) {
	render := func(on bool, text string) *image.RGBA {
		eng := openSansEngine(t)
		eng.SetTextSubpixel(on)
		eng.canvas.DrawTextFont(text, 2, 2, 8.5, "OpenSans", color.RGBA{0, 0, 0, 255})
		return eng.canvas.back
	}
	second := func(on bool) []int {
		one, two := render(on, "l"), render(on, "ll")
		diff := image.NewRGBA(two.Rect)
		for i := 3; i < len(two.Pix); i += 4 {
			d := int(two.Pix[i]) - int(one.Pix[i])
			if d < 0 {
				d = 0
			}
			diff.Pix[i] = uint8(d)
		}
		return inkProfile(diff)
	}
	equal := func(a, b []int) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	for _, on := range []bool{false, true} {
		first := inkProfile(render(on, "l"))
		same := equal(first, second(on))
		if !on && !same {
			t.Errorf("целочисленный режим: второй штрих нарисован иначе, чем первый: %v и %v", first, second(on))
		}
		if on && same {
			t.Errorf("подпиксельный режим: второй штрих нарисован той же маской, что и первый: %v", first)
		}
	}
}
