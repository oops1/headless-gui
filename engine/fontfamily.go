// fontfamily.go — выбор шрифта по семейству, весу и наклону.
//
// До этого именованный шрифт был просто именем файла: «OpenSans-Bold» и
// «OpenSans» — два несвязанных имени, а алиас семейства получал только
// Regular. Тема не могла сказать «Open Sans, SemiBold»: в FontSpec были лишь
// семейство и два флага, а флаги движок не читал вовсе.
//
// Теперь каждый зарегистрированный шрифт записывается в таблицу семейств по
// собственным данным (name-таблица и OS/2 самого файла, а не по имени файла —
// «Golos Text SemiBold» лежит в файле GolosText-SemiBold.ttf, но объявляет
// себя весом 600 семейства «Golos Text»). Запрос приходит обычным именем
// шрифта, поэтому интерфейс DrawContext не менялся: имя вида
// widget.FontFace("Open Sans", 600, false) разбирается здесь же, в fontFor.
package engine

import (
	"encoding/binary"
	"strings"
	"sync"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/oops1/headless-gui/v3/widget"
)

// faceEntry — одно начертание семейства: под каким именем оно лежит в
// реестре именованных шрифтов, какой у него вес и наклонное ли оно.
type faceEntry struct {
	name   string
	weight int
	italic bool
}

// fontFamilies — таблица семейств. Общая у канваса и его клонов (HiDPI,
// попапы): шрифты они делят, поэтому и таблица должна быть одна.
type fontFamilies struct {
	mu  sync.RWMutex
	fam map[string][]faceEntry // ключ семейства (familyKey) → начертания
	// defKey — ключ семейства шрифта по умолчанию; "" — шрифт по умолчанию
	// не из таблицы (встроенный Go Regular).
	defKey string
	// resolved — кэш разбора составных имён: запрос → имя в реестре шрифтов
	// ("" — подходящего нет). Сбрасывается любым изменением таблицы.
	resolved map[string]string
}

func newFontFamilies() *fontFamilies {
	return &fontFamilies{fam: map[string][]faceEntry{}, resolved: map[string]string{}}
}

// familyKey приводит название семейства к ключу: без регистра, пробелов,
// дефисов и подчёркиваний. «Open Sans», «OpenSans» и «open-sans» — одно
// семейство: в XAML пишут первым, в имени файла стоит второе.
func familyKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case ' ', '-', '_', '\t':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// add записывает начертание в семейство key; начертание с тем же весом и
// наклоном заменяется (позднейшая регистрация побеждает, как и у имён).
func (t *fontFamilies) add(key string, e faceEntry) {
	if key == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	list := t.fam[key]
	for i := range list {
		if list[i].weight == e.weight && list[i].italic == e.italic {
			list[i] = e
			t.resolved = map[string]string{}
			return
		}
	}
	t.fam[key] = append(list, e)
	t.resolved = map[string]string{}
}

func (t *fontFamilies) setDefault(key string) {
	t.mu.Lock()
	t.defKey = key
	t.resolved = map[string]string{}
	t.mu.Unlock()
}

// weightRank упорядочивает кандидатов по правилу CSS Fonts: при запросе
// 400–500 сначала сам вес, затем 500/400, затем более лёгкие по убыванию, затем
// более тяжёлые; при запросе легче 400 — сначала более лёгкие, потом
// тяжёлые; при запросе тяжелее 500 — сначала более тяжёлые, потом лёгкие.
// Меньшая пара — лучше.
func weightRank(want, have int) (group, dist int) {
	switch {
	case have == want:
		return 0, 0
	case want >= 400 && want <= 500:
		switch {
		case have > want && have <= 500:
			return 1, have - want
		case have < want:
			return 2, want - have
		}
		return 3, have - want
	case want < 400:
		if have < want {
			return 1, want - have
		}
		return 2, have - want
	default:
		if have > want {
			return 1, have - want
		}
		return 2, want - have
	}
}

// pick выбирает начертание семейства. Наклон важнее веса: курсивный
// Light лучше, чем прямой Bold, когда просили наклонный. Нет ни одного
// наклонного — берём прямые.
func pick(list []faceEntry, weight int, italic bool) (faceEntry, bool) {
	var best faceEntry
	found := false
	var bg, bd int
	for pass := 0; pass < 2 && !found; pass++ {
		for _, e := range list {
			if pass == 0 && e.italic != italic {
				continue
			}
			g, d := weightRank(weight, e.weight)
			if !found || g < bg || (g == bg && d < bd) {
				best, bg, bd, found = e, g, d, true
			}
		}
	}
	return best, found
}

// registerFace записывает только что зарегистрированный шрифт в таблицу и
// запоминает его ключ семейства в самом FontCache.
//
// Имена шрифтов, начинающиеся с «$», — встроенные служебные ($hg_bold и
// прочие): они в семейства не входят, у них своя дорога (widget.Label).
func (c *Canvas) registerFace(name string, fc *FontCache) {
	if c.families == nil || fc == nil || strings.HasPrefix(name, "$") {
		return
	}
	meta := describeFont(fc, name)
	fc.famKey = meta.keys[0]
	for _, k := range meta.keys {
		c.families.add(k, faceEntry{name: name, weight: meta.weight, italic: meta.italic})
	}
}

// fontMeta — то, что движок знает о шрифте из его собственных таблиц.
type fontMeta struct {
	keys   []string // ключи семейства: сначала из name-таблицы, затем из имени регистрации
	weight int
	italic bool
}

// describeFont определяет семейство, вес и наклон шрифта.
//
// Источник правды — файл: вес из OS/2.usWeightClass, семейство из name ID 16
// (типографское) или 1. Имя, под которым шрифт зарегистрировали, служит
// запасным вариантом и дополнительным ключом: старые сборки (Open Sans Light
// с name ID 1 = «Open Sans Light») иначе остались бы отдельным семейством.
func describeFont(fc *FontCache, regName string) fontMeta {
	var m fontMeta
	var buf sfnt.Buffer
	name := func(id sfnt.NameID) string {
		if fc.ttf == nil {
			return ""
		}
		s, err := fc.ttf.Name(&buf, id)
		if err != nil {
			return ""
		}
		return s
	}
	family := name(sfnt.NameIDTypographicFamily)
	if family == "" {
		family = name(sfnt.NameIDFamily)
	}
	style := name(sfnt.NameIDTypographicSubfamily)
	if style == "" {
		style = name(sfnt.NameIDSubfamily)
	}

	// Хвост имени регистрации после последнего дефиса — тоже начертание:
	// «OpenSans-SemiBold», «Roboto-BoldItalic».
	stemFam, stemStyle := regName, ""
	if i := strings.LastIndexByte(regName, '-'); i > 0 {
		stemFam, stemStyle = regName[:i], regName[i+1:]
	}

	w, it, ok := readOS2(fc.ttfData)
	if !ok {
		w = 0
	}
	if w == 0 {
		w = weightFromStyle(style)
	}
	if w == 0 {
		w = weightFromStyle(stemStyle)
	}
	if w == 0 {
		w = 400
	}
	if !ok {
		it = styleItalic(style) || styleItalic(stemStyle)
	}
	m.weight, m.italic = w, it

	seen := map[string]bool{}
	for _, f := range []string{family, stemFam, regName} {
		if k := familyKey(f); k != "" && !seen[k] {
			seen[k] = true
			m.keys = append(m.keys, k)
		}
	}
	if len(m.keys) == 0 {
		m.keys = []string{"?"}
	}
	return m
}

// readOS2 достаёт из таблицы OS/2 вес (usWeightClass) и признак наклона
// (fsSelection, бит 0). ok=false — таблицы нет или шрифт не одиночный (TTC),
// и вызывающий судит по именам.
func readOS2(data []byte) (weight int, italic bool, ok bool) {
	if len(data) < 12 {
		return 0, false, false
	}
	switch binary.BigEndian.Uint32(data) {
	case 0x00010000, 0x4F54544F, 0x74727565: // TrueType, 'OTTO', 'true'
	default:
		return 0, false, false
	}
	n := int(binary.BigEndian.Uint16(data[4:]))
	for i := 0; i < n; i++ {
		rec := 12 + i*16
		if rec+16 > len(data) {
			break
		}
		if string(data[rec:rec+4]) != "OS/2" {
			continue
		}
		off := int(binary.BigEndian.Uint32(data[rec+8:]))
		ln := int(binary.BigEndian.Uint32(data[rec+12:]))
		if off < 0 || ln < 64 || off+ln > len(data) {
			return 0, false, false
		}
		w := int(binary.BigEndian.Uint16(data[off+4:]))
		sel := binary.BigEndian.Uint16(data[off+62:])
		if w < 100 || w > 1000 {
			w = 0
		}
		return w, sel&1 != 0, true
	}
	return 0, false, false
}

// weightFromStyle переводит название начертания в вес; 0 — не узнали.
func weightFromStyle(style string) int {
	s := familyKey(style)
	s = strings.NewReplacer("italic", "", "oblique", "").Replace(s)
	switch s {
	case "":
		return 0
	case "thin", "hairline":
		return 100
	case "extralight", "ultralight":
		return 200
	case "light":
		return 300
	case "regular", "normal", "book", "roman":
		return 400
	case "medium":
		return 500
	case "semibold", "demibold":
		return 600
	case "bold":
		return 700
	case "extrabold", "ultrabold", "heavy":
		return 800
	case "black":
		return 900
	}
	return 0
}

func styleItalic(style string) bool {
	s := strings.ToLower(style)
	return strings.Contains(s, "italic") || strings.Contains(s, "oblique")
}

// faceFor разбирает имя шрифта, которого нет в реестре буквально: либо
// составное (widget.FontFace), либо просто название семейства («Open Sans» при
// файле OpenSans-Regular.ttf). nil — подходящего нет, вызывающий берёт шрифт
// по умолчанию.
func (c *Canvas) faceFor(req string) *FontCache {
	t := c.families
	if t == nil {
		return nil
	}
	family, weight, italic, composite := widget.ParseFontFace(req)
	if !composite {
		family, weight, italic = req, 400, false
	}

	t.mu.RLock()
	name, hit := t.resolved[req]
	t.mu.RUnlock()
	if !hit {
		name = c.resolveFace(family, weight, italic)
		t.mu.Lock()
		t.resolved[req] = name
		t.mu.Unlock()
	}
	if name == "" {
		return nil
	}
	return c.namedFonts[name]
}

// resolveFace подбирает имя шрифта в реестре под (семейство, вес, наклон).
//
// Пустое или неизвестное семейство означает «шрифт по умолчанию»: так
// Bold:true в стиле без Family даёт жирное начертание того шрифта, которым
// тема пишет остальное. Если у семейства по умолчанию жирного нет (встроенный
// Go Regular), берётся встроенный Go Bold — как и у widget.Label.
func (c *Canvas) resolveFace(family string, weight int, italic bool) string {
	t := c.families
	if weight <= 0 {
		weight = 400
	}
	// Копии: add правит срезы на месте, а здесь с ними работают без замка.
	t.mu.RLock()
	list := append([]faceEntry(nil), t.fam[familyKey(family)]...)
	def := append([]faceEntry(nil), t.fam[t.defKey]...)
	t.mu.RUnlock()

	if len(list) > 0 {
		if e, ok := pick(list, weight, italic); ok {
			return e.name
		}
	}

	// Семейство не задано или неизвестно — шрифт по умолчанию.
	bold := weight >= 600
	if e, ok := pick(def, weight, italic); ok && (!bold || e.weight >= 600) && (!italic || e.italic) {
		return e.name
	}
	fb := ""
	switch {
	case bold && italic:
		fb = widget.BuiltinFontBoldItalic
	case bold:
		fb = widget.BuiltinFontBold
	case italic:
		fb = widget.BuiltinFontItalic
	}
	if _, ok := c.namedFonts[fb]; ok {
		return fb
	}
	return ""
}

// penGlyph выбирает маску глифа для пера в позиции pen и возвращает её вместе
// с целым пикселем, от которого отсчитывается offX. Без подпикселя перо
// округляется, как и раньше.
func penGlyph(fc *FontCache, subpixel bool, sizePt float64, r rune, pen fixed.Int26_6) (cachedGlyph, int) {
	if !subpixel {
		return fc.Glyph(sizePt, r), pen.Round()
	}
	// Четверти пикселя с округлением к ближайшей; >> на отрицательных —
	// арифметический, то есть floor, и &3 даёт верную долю и слева от нуля.
	q := (pen + 8) >> 4
	return fc.GlyphAt(sizePt, r, int(q&3)), int(q >> 2)
}

// setTextSubpixel включает дробное позиционирование глифов у всех шрифтов
// канваса: основного, именованных и запасных. Шрифты, зарегистрированные
// позже, наследуют режим от основного (RegisterFont, AddFallbackFont).
func (c *Canvas) setTextSubpixel(on bool) {
	c.fontCache.SetSubpixel(on)
	for _, fc := range c.namedFonts {
		fc.SetSubpixel(on)
	}
	for _, fc := range c.fallbacks {
		fc.SetSubpixel(on)
	}
	c.shaper.dropLayouts()
	c.fontRev++
}
