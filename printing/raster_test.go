package printing

import (
	"errors"
	"image"
	"image/color"
	"reflect"
	"testing"
)

func TestFitRect(t *testing.T) {
	cases := []struct {
		name           string
		sw, sh, pw, ph int
		want           image.Rectangle
	}{
		{"те же пропорции", 2480, 3508, 4960, 7016, image.Rect(0, 0, 4960, 7016)},
		{"тот же размер", 100, 200, 100, 200, image.Rect(0, 0, 100, 200)},
		// A4 на лист Letter 2550×3300: страница уже листа по ширине → упор в высоту.
		{"A4 на Letter", 2480, 3508, 2550, 3300, image.Rect(108, 0, 2441, 3300)},
		// Letter на A4: страница шире по пропорции → упор в ширину.
		{"Letter на A4", 2550, 3300, 2480, 3508, image.Rect(0, 149, 2480, 3358)},
		{"пустая страница", 0, 10, 100, 100, image.Rectangle{}},
		{"пустой лист", 10, 10, 0, 100, image.Rectangle{}},
	}
	for _, c := range cases {
		got := fitRect(c.sw, c.sh, c.pw, c.ph)
		if got != c.want {
			t.Errorf("%s: %v, ждали %v", c.name, got, c.want)
		}
	}
	// Всегда внутри листа и не пустая, даже на экстремальных пропорциях.
	for _, sz := range [][4]int{{1, 10000, 100, 100}, {10000, 1, 100, 100}, {3, 7, 1, 1}} {
		r := fitRect(sz[0], sz[1], sz[2], sz[3])
		if r.Empty() || !r.In(image.Rect(0, 0, sz[2], sz[3])) {
			t.Errorf("%v → %v", sz, r)
		}
	}
}

func TestNeedRotate(t *testing.T) {
	cases := []struct {
		sw, sh, pw, ph int
		want           bool
	}{
		{100, 50, 50, 100, true},   // альбомная страница, книжный лист
		{50, 100, 100, 50, true},   // наоборот
		{100, 50, 200, 100, false}, // обе альбомные
		{50, 100, 50, 100, false},
		{100, 100, 50, 100, false}, // квадратная страница не поворачивается
		{100, 50, 100, 100, false}, // квадратный лист — тоже
	}
	for _, c := range cases {
		if got := needRotate(c.sw, c.sh, c.pw, c.ph); got != c.want {
			t.Errorf("%v: %v", c, got)
		}
	}
}

func TestRotate90(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	// Метка: пиксель (x,y) получает R = 10*y + x.
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			src.SetRGBA(x, y, color.RGBA{uint8(10*y + x), 0, 0, 255})
		}
	}
	dst := rotate90(src)
	if dst.Bounds() != image.Rect(0, 0, 2, 3) {
		t.Fatalf("размер %v", dst.Bounds())
	}
	// По часовой: (x,y) → (h-1-y, x) при h=2.
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			if got := dst.RGBAAt(1-y, x).R; got != uint8(10*y+x) {
				t.Errorf("(%d,%d)→(%d,%d): %d", x, y, 1-y, x, got)
			}
		}
	}
	// Угол: левый верхний исходный окажется в правом верхнем.
	if dst.RGBAAt(1, 0).R != 0 {
		t.Error("левый верхний угол не в правом верхнем")
	}
	// Подкартинка с ненулевым Min.
	big := image.NewRGBA(image.Rect(0, 0, 10, 10))
	big.SetRGBA(5, 7, color.RGBA{99, 0, 0, 255})
	sub := big.SubImage(image.Rect(5, 7, 8, 9))
	if r := rotate90(sub); r.RGBAAt(1, 0).R != 99 {
		t.Errorf("подкартинка: %v", r.RGBAAt(1, 0))
	}
}

// TestBandSpansTile — полосы покрывают лист без зазоров и наложений при любом
// масштабе, а исходные строки — без пропусков.
func TestBandSpansTile(t *testing.T) {
	for _, c := range []struct{ srcH, band, dstY, dstH int }{
		{3508, 512, 0, 7016},   // увеличение вдвое
		{3508, 512, -30, 3508}, // 1:1 со смещением
		{3508, 512, 12, 1000},  // сильное уменьшение
		{3508, 512, 0, 3507},   // чуть меньше
		{10, 3, 5, 7},
		{1, 512, 0, 1},
		{7, 2, 0, 3},
	} {
		spans := bandSpans(c.srcH, c.band, c.dstY, c.dstH)
		if len(spans) == 0 {
			t.Errorf("%+v: нет полос", c)
			continue
		}
		if spans[0].DstY0 != c.dstY || spans[len(spans)-1].DstY1 != c.dstY+c.dstH {
			t.Errorf("%+v: покрыто %d…%d, ждали %d…%d", c, spans[0].DstY0, spans[len(spans)-1].DstY1, c.dstY, c.dstY+c.dstH)
		}
		prev := c.dstY
		srcNext := 0
		for i, s := range spans {
			if s.DstY0 != prev {
				t.Errorf("%+v: полоса %d начинается с %d, ждали %d (зазор/наложение)", c, i, s.DstY0, prev)
			}
			if s.DstY1 <= s.DstY0 {
				t.Errorf("%+v: пустая полоса %d", c, i)
			}
			if s.SrcY1-s.SrcY0 > c.band {
				t.Errorf("%+v: полоса %d выше %d строк", c, i, c.band)
			}
			// Пропущенные (нулевой высоты) полосы допустимы только при сильном
			// уменьшении; строки источника идут подряд с учётом пропусков.
			if s.SrcY0 < srcNext {
				t.Errorf("%+v: источник идёт назад", c)
			}
			srcNext = s.SrcY1
			prev = s.DstY1
		}
	}
	if bandSpans(0, 10, 0, 10) != nil || bandSpans(10, 0, 0, 10) != nil || bandSpans(10, 10, 0, 0) != nil {
		t.Error("вырожденные входы дали полосы")
	}
}

func TestFillBGRX(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.SetRGBA(0, 0, color.RGBA{10, 20, 30, 255})
	img.SetRGBA(1, 0, color.RGBA{0, 0, 0, 0}) // прозрачный → белый
	img.SetRGBA(2, 0, color.RGBA{100, 0, 0, 200})
	img.SetRGBA(0, 1, color.RGBA{1, 2, 3, 255})
	dst := make([]byte, 4*3*2)
	fillBGRX(dst, img, 0, 2)
	want := []byte{
		30, 20, 10, 0, 255, 255, 255, 0, 55, 55, 155, 0, // строка 0: B,G,R,0
		3, 2, 1, 0, 255, 255, 255, 0, 255, 255, 255, 0, // строка 1 (пустые пиксели — прозрачные → белые)
	}
	if !reflect.DeepEqual(dst, want) {
		t.Errorf("RGBA:\n got %v\nwant %v", dst, want)
	}
	// Только вторая строка — со сдвигом в начало буфера.
	one := make([]byte, 12)
	fillBGRX(one, img, 1, 2)
	if !reflect.DeepEqual(one, want[12:]) {
		t.Errorf("одна строка: %v", one)
	}
	// Общий путь (не *image.RGBA) даёт то же для непрозрачных пикселей.
	g := image.NewGray(image.Rect(0, 0, 2, 1))
	g.SetGray(0, 0, color.Gray{50})
	g.SetGray(1, 0, color.Gray{200})
	gd := make([]byte, 8)
	fillBGRX(gd, g, 0, 1)
	if !reflect.DeepEqual(gd, []byte{50, 50, 50, 0, 200, 200, 200, 0}) {
		t.Errorf("Gray: %v", gd)
	}
}

func TestPageOrder(t *testing.T) {
	if got := pageOrder(3, 2, true); !reflect.DeepEqual(got, []int{0, 1, 2, 0, 1, 2}) {
		t.Errorf("комплектами: %v", got)
	}
	if got := pageOrder(3, 2, false); !reflect.DeepEqual(got, []int{0, 0, 1, 1, 2, 2}) {
		t.Errorf("постранично: %v", got)
	}
	if got := pageOrder(2, 0, true); !reflect.DeepEqual(got, []int{0, 1}) {
		t.Errorf("нулевые копии: %v", got)
	}
}

// fakeDevice — запоминающее устройство: ничего не печатает.
type fakeDevice struct {
	caps    deviceCaps
	events  []string
	bands   []fakeBand
	failAt  string // событие, на котором вернуть ошибку: "StartDoc", "StartPage", "Band", "EndPage", "EndDoc"
	aborted bool
	doc     string
}

type fakeBand struct {
	Dst     image.Rectangle
	W, Rows int
	Bits    []byte
	Page    int
}

var errFake = errors.New("сбой устройства")

func (d *fakeDevice) Caps() deviceCaps { return d.caps }
func (d *fakeDevice) ev(name string) error {
	d.events = append(d.events, name)
	if d.failAt == name {
		return errFake
	}
	return nil
}
func (d *fakeDevice) StartDoc(name string) error { d.doc = name; return d.ev("StartDoc") }
func (d *fakeDevice) StartPage() error           { return d.ev("StartPage") }
func (d *fakeDevice) Band(dst image.Rectangle, w, rows int, bits []byte) error {
	d.bands = append(d.bands, fakeBand{dst, w, rows, append([]byte(nil), bits...), d.pages()})
	return d.ev("Band")
}
func (d *fakeDevice) EndPage() error { return d.ev("EndPage") }
func (d *fakeDevice) EndDoc() error  { return d.ev("EndDoc") }
func (d *fakeDevice) Abort()         { d.aborted = true }
func (d *fakeDevice) pages() int {
	n := 0
	for _, e := range d.events {
		if e == "StartPage" {
			n++
		}
	}
	return n - 1
}

// solid — страница одного цвета (R = номер страницы), чтобы по байтам полосы
// узнать, какая страница напечатана.
func solid(w, h int, r uint8) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+3] = r, 255
	}
	return img
}

func pageTags(d *fakeDevice) []int {
	var out []int
	last := -1
	for _, b := range d.bands {
		if b.Page != last {
			out = append(out, int(b.Bits[2])) // R из порядка B,G,R,0
			last = b.Page
		}
	}
	return out
}

func TestPrintRasterOrderAndCopies(t *testing.T) {
	pages := []image.Image{solid(10, 20, 1), solid(10, 20, 2), solid(10, 20, 3)}
	run := func(copies int, collate bool) *fakeDevice {
		d := &fakeDevice{caps: deviceCaps{PhysW: 10, PhysH: 20}}
		if err := printRaster(d, "док", pages, copies, collate); err != nil {
			t.Fatal(err)
		}
		return d
	}
	d := run(2, true)
	if !reflect.DeepEqual(pageTags(d), []int{1, 2, 3, 1, 2, 3}) {
		t.Errorf("комплектами: %v", pageTags(d))
	}
	if d.doc != "док" {
		t.Errorf("имя документа %q", d.doc)
	}
	d = run(2, false)
	if !reflect.DeepEqual(pageTags(d), []int{1, 1, 2, 2, 3, 3}) {
		t.Errorf("постранично: %v", pageTags(d))
	}
	// Порядок вызовов: Doc ... (Page Band EndPage)* ... EndDoc.
	d = run(1, false)
	want := []string{"StartDoc"}
	for i := 0; i < 3; i++ {
		want = append(want, "StartPage", "Band", "EndPage")
	}
	want = append(want, "EndDoc")
	if !reflect.DeepEqual(d.events, want) {
		t.Errorf("события: %v", d.events)
	}
	if d.aborted {
		t.Error("успешная печать вызвала Abort")
	}
}

// TestPrintRasterPlacement — страница ложится на лист с учётом непечатаемого
// края: начало координат устройства смещено на PHYSICALOFFSET.
func TestPrintRasterPlacement(t *testing.T) {
	// Лист 100×200, непечатаемый край 5 слева и 7 сверху; страница 50×100 — те же пропорции.
	d := &fakeDevice{caps: deviceCaps{PhysW: 100, PhysH: 200, OffX: 5, OffY: 7}}
	if err := printRaster(d, "p", []image.Image{solid(50, 100, 9)}, 1, false); err != nil {
		t.Fatal(err)
	}
	if len(d.bands) != 1 {
		t.Fatalf("полос %d", len(d.bands))
	}
	b := d.bands[0]
	if b.Dst != image.Rect(-5, -7, 95, 193) || b.W != 50 || b.Rows != 100 {
		t.Errorf("полоса: %+v", b)
	}

	// Другая пропорция: A4-подобная страница на более «квадратный» лист —
	// вписывается по центру без искажения.
	d = &fakeDevice{caps: deviceCaps{PhysW: 120, PhysH: 120}}
	if err := printRaster(d, "p", []image.Image{solid(40, 80, 9)}, 1, false); err != nil {
		t.Fatal(err)
	}
	if got := d.bands[0].Dst; got != image.Rect(30, 0, 90, 120) {
		t.Errorf("вписывание: %v", got)
	}
}

// TestPrintRasterBandsCoverPage — большая страница уходит несколькими полосами,
// вместе покрывающими её целиком, а число строк в полосах даёт высоту страницы.
func TestPrintRasterBandsCoverPage(t *testing.T) {
	const w, h = 2000, 3000 // полоса: 8 МБ / (4·2000) = 1048 → не более 512 строк
	d := &fakeDevice{caps: deviceCaps{PhysW: 4000, PhysH: 6000}}
	if err := printRaster(d, "p", []image.Image{solid(w, h, 9)}, 1, false); err != nil {
		t.Fatal(err)
	}
	if len(d.bands) < 5 {
		t.Fatalf("полос %d", len(d.bands))
	}
	rows, prev := 0, 0
	for _, b := range d.bands {
		rows += b.Rows
		if b.Rows > 512 || b.W != w || len(b.Bits) != 4*w*b.Rows {
			t.Errorf("полоса %+v", b.Dst)
		}
		if b.Dst.Min.Y != prev {
			t.Errorf("зазор: полоса с %d, ждали %d", b.Dst.Min.Y, prev)
		}
		prev = b.Dst.Max.Y
	}
	if rows != h || prev != 6000 {
		t.Errorf("строк %d из %d, низ %d из 6000", rows, h, prev)
	}
}

// TestPrintRasterRotatesLandscape — альбомная страница на книжный лист
// поворачивается, а не сжимается в узкую полосу.
func TestPrintRasterRotatesLandscape(t *testing.T) {
	d := &fakeDevice{caps: deviceCaps{PhysW: 100, PhysH: 200}}
	if err := printRaster(d, "p", []image.Image{solid(200, 100, 9)}, 2, false); err != nil {
		t.Fatal(err)
	}
	if len(d.bands) != 2 {
		t.Fatalf("полос %d", len(d.bands))
	}
	for _, b := range d.bands {
		if b.W != 100 || b.Rows != 200 || b.Dst != image.Rect(0, 0, 100, 200) {
			t.Errorf("после поворота: %+v", b)
		}
	}
}

func TestPrintRasterAbortsOnError(t *testing.T) {
	for _, at := range []string{"StartPage", "Band", "EndPage"} {
		d := &fakeDevice{caps: deviceCaps{PhysW: 10, PhysH: 20}, failAt: at}
		err := printRaster(d, "p", []image.Image{solid(10, 20, 1), solid(10, 20, 2)}, 1, false)
		if !errors.Is(err, errFake) {
			t.Errorf("%s: err = %v", at, err)
		}
		if !d.aborted {
			t.Errorf("%s: оборванный документ не прерван", at)
		}
		for _, e := range d.events {
			if e == "EndDoc" {
				t.Errorf("%s: после сбоя вызван EndDoc", at)
			}
		}
	}
	// Сбой StartDoc: документ не начат — прерывать нечего.
	d := &fakeDevice{caps: deviceCaps{PhysW: 10, PhysH: 20}, failAt: "StartDoc"}
	if err := printRaster(d, "p", []image.Image{solid(10, 20, 1)}, 1, false); !errors.Is(err, errFake) || d.aborted {
		t.Errorf("StartDoc: err=%v aborted=%v", err, d.aborted)
	}
	// Сбой EndDoc — ошибка, но Abort уже бесполезен.
	d = &fakeDevice{caps: deviceCaps{PhysW: 10, PhysH: 20}, failAt: "EndDoc"}
	if err := printRaster(d, "p", []image.Image{solid(10, 20, 1)}, 1, false); !errors.Is(err, errFake) || d.aborted {
		t.Errorf("EndDoc: err=%v aborted=%v", err, d.aborted)
	}
}

func TestPrintRasterBadInput(t *testing.T) {
	d := &fakeDevice{caps: deviceCaps{PhysW: 10, PhysH: 20}}
	if err := printRaster(d, "p", nil, 1, false); !errors.Is(err, ErrNoPages) {
		t.Errorf("без страниц: %v", err)
	}
	d = &fakeDevice{}
	if err := printRaster(d, "p", []image.Image{solid(1, 1, 1)}, 1, false); err == nil {
		t.Error("принтер без размера листа принят")
	}
	if len(d.events) != 0 {
		t.Error("документ начат на устройстве без размеров")
	}
}
