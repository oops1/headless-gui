package pdf

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// parsed — результат разбора собранного файла «с нуля», независимо от кода
// писателя: так тест ловит ошибку в самом писателе, а не повторяет её.
type parsed struct {
	data    []byte
	offsets []int64 // по номеру объекта; [0] — свободная запись
	size    int
	root    int
	info    int
}

// parsePDF читает хвост файла по правилам читателя: startxref → таблица xref →
// trailer. Каждое нарушение формата — ошибка теста.
func parsePDF(t *testing.T, data []byte) *parsed {
	t.Helper()
	if !bytes.HasPrefix(data, []byte("%PDF-1.")) {
		t.Fatalf("нет заголовка %%PDF-1.x: %q", data[:min(8, len(data))])
	}
	if !bytes.HasSuffix(data, []byte("%%EOF\n")) {
		t.Fatalf("файл не кончается на %%%%EOF")
	}
	i := bytes.LastIndex(data, []byte("startxref\n"))
	if i < 0 {
		t.Fatal("нет startxref")
	}
	rest := string(data[i+len("startxref\n"):])
	numEnd := strings.IndexByte(rest, '\n')
	xrefPos, err := strconv.Atoi(rest[:numEnd])
	if err != nil {
		t.Fatalf("startxref: %v", err)
	}
	if !bytes.HasPrefix(data[xrefPos:], []byte("xref\n")) {
		t.Fatalf("по смещению startxref=%d нет слова xref: %q", xrefPos, data[xrefPos:xrefPos+10])
	}
	tab := data[xrefPos+len("xref\n"):]
	nl := bytes.IndexByte(tab, '\n')
	var first, count int
	if _, err := fmt.Sscanf(string(tab[:nl]), "%d %d", &first, &count); err != nil || first != 0 {
		t.Fatalf("заголовок подраздела xref %q: %v", tab[:nl], err)
	}
	tab = tab[nl+1:]
	p := &parsed{data: data, size: count, offsets: make([]int64, count)}
	for n := 0; n < count; n++ {
		if len(tab) < 20 {
			t.Fatalf("xref обрывается на записи %d", n)
		}
		e := tab[:20]
		tab = tab[20:]
		// Ровно 20 байт: 10 цифр, пробел, 5 цифр, пробел, n|f, пробел, \n.
		if !regexp.MustCompile(`^\d{10} \d{5} [nf] \n$`).Match(e) {
			t.Fatalf("запись xref %d неверной формы: %q", n, e)
		}
		off, _ := strconv.ParseInt(string(e[:10]), 10, 64)
		p.offsets[n] = off
		if (e[17] == 'f') != (n == 0) {
			t.Fatalf("запись %d: тип %c", n, e[17])
		}
	}
	if !bytes.HasPrefix(tab, []byte("trailer\n")) {
		t.Fatalf("после xref нет trailer: %q", tab[:min(20, len(tab))])
	}
	m := regexp.MustCompile(`/Size (\d+) /Root (\d+) 0 R /Info (\d+) 0 R`).FindSubmatch(tab)
	if m == nil {
		t.Fatalf("trailer не разобран: %q", tab)
	}
	if sz, _ := strconv.Atoi(string(m[1])); sz != count {
		t.Fatalf("trailer /Size=%d, а в xref %d записей", sz, count)
	}
	p.root, _ = strconv.Atoi(string(m[2]))
	p.info, _ = strconv.Atoi(string(m[3]))
	return p
}

// object возвращает текст объекта n, проверив, что смещение из xref указывает
// ровно на его начало, а не на пробел до него или на середину.
func (p *parsed) object(t *testing.T, n int) []byte {
	t.Helper()
	off := p.offsets[n]
	want := fmt.Sprintf("%d 0 obj\n", n)
	if !bytes.HasPrefix(p.data[off:], []byte(want)) {
		end := min(int(off)+20, len(p.data))
		t.Fatalf("xref[%d]=%d указывает не на %q: %q", n, off, want, p.data[off:end])
	}
	rest := p.data[off:]
	e := bytes.Index(rest, []byte("endobj\n"))
	if e < 0 {
		t.Fatalf("объект %d без endobj", n)
	}
	return rest[:e]
}

// stream достаёт поток объекта строго по /Length из словаря.
func (p *parsed) stream(t *testing.T, n int) (dict string, body []byte) {
	t.Helper()
	obj := p.object(t, n)
	s := bytes.Index(obj, []byte("stream\n"))
	if s < 0 {
		t.Fatalf("в объекте %d нет потока", n)
	}
	dict = string(obj[:s])
	m := regexp.MustCompile(`/Length (\d+)`).FindStringSubmatch(dict)
	if m == nil {
		t.Fatalf("в объекте %d нет /Length", n)
	}
	l, _ := strconv.Atoi(m[1])
	body = obj[s+len("stream\n"):]
	if len(body) < l+len("\nendstream\n") {
		t.Fatalf("объект %d: /Length=%d больше тела %d", n, l, len(body))
	}
	if !bytes.HasPrefix(body[l:], []byte("\nendstream\n")) {
		t.Fatalf("объект %d: после %d байт потока нет endstream: %q", n, l, body[l:min(l+15, len(body))])
	}
	return dict, body[:l]
}

func testImage(w, h int, base color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := base
			c.R += uint8(x * 7)
			c.G += uint8(y * 5)
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestWriteStructure(t *testing.T) {
	pages := []Page{
		PageFromImage(testImage(30, 40, color.RGBA{10, 20, 30, 255}), 72),
		PageFromImage(testImage(60, 20, color.RGBA{200, 100, 50, 255}), 144),
	}
	var buf bytes.Buffer
	opt := Options{Title: "Отчёт (1)", Author: "A\\B", Creator: "тест",
		Created: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)}
	if err := Write(&buf, pages, opt); err != nil {
		t.Fatal(err)
	}
	p := parsePDF(t, buf.Bytes())

	// 1 каталог + 1 дерево + 3·2 страницы + Info + свободная нулевая запись.
	if p.size != 2+6+1+1 {
		t.Fatalf("записей в xref: %d", p.size)
	}
	// Каждое смещение указывает ровно на «N 0 obj».
	for n := 1; n < p.size; n++ {
		p.object(t, n)
	}

	cat := string(p.object(t, p.root))
	if !strings.Contains(cat, "/Type /Catalog") || !strings.Contains(cat, "/Pages 2 0 R") {
		t.Errorf("каталог: %q", cat)
	}
	tree := string(p.object(t, 2))
	if !strings.Contains(tree, "/Count 2") || !strings.Contains(tree, "/Kids [3 0 R 6 0 R]") {
		t.Errorf("дерево страниц: %q", tree)
	}

	// Размеры в пунктах: 30×40 пикс при 72 dpi → 30×40; 60×20 при 144 → 30×10.
	if pg := string(p.object(t, 3)); !strings.Contains(pg, "/MediaBox [0 0 30 40]") {
		t.Errorf("страница 1: %q", pg)
	}
	if pg := string(p.object(t, 6)); !strings.Contains(pg, "/MediaBox [0 0 30 10]") {
		t.Errorf("страница 2: %q", pg)
	}

	info := string(p.object(t, p.info))
	for _, want := range []string{
		"/Title <FEFF", "/Author (A\\\\B)", "/Creator <FEFF",
		"/CreationDate (D:20260304050607Z)", "/Producer (headless-gui)",
	} {
		if !strings.Contains(info, want) {
			t.Errorf("Info без %q: %q", want, info)
		}
	}
	// Заголовок «Отчёт (1)» — UTF-16BE: О=041E, т=0442, скобки как есть.
	if !strings.Contains(info, "041E") || !strings.Contains(info, "0028") {
		t.Errorf("заголовок не в UTF-16BE: %q", info)
	}
}

// TestFlateImageRoundTrip — картинка из потока разворачивается обратно в те же
// пиксели: проверяет и zlib-обёртку, и предсказатель «Up», и /Length.
func TestFlateImageRoundTrip(t *testing.T) {
	src := testImage(37, 23, color.RGBA{1, 2, 3, 255})
	var buf bytes.Buffer
	if err := Write(&buf, []Page{PageFromImage(src, 96)}, Options{}); err != nil {
		t.Fatal(err)
	}
	p := parsePDF(t, buf.Bytes())
	dict, body := p.stream(t, 5)
	for _, want := range []string{"/Width 37", "/Height 23", "/FlateDecode", "/Predictor 15", "/Columns 37", "/DeviceRGB"} {
		if !strings.Contains(dict, want) {
			t.Errorf("словарь картинки без %q: %s", want, dict)
		}
	}
	zr, err := zlib.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("поток — не zlib: %v", err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	stride := 1 + 3*37
	if len(raw) != stride*23 {
		t.Fatalf("распаковано %d байт, ждали %d", len(raw), stride*23)
	}
	prev := make([]byte, 3*37)
	for y := 0; y < 23; y++ {
		line := raw[y*stride : (y+1)*stride]
		if line[0] != 2 {
			t.Fatalf("строка %d: фильтр %d, ждали 2 (Up)", y, line[0])
		}
		for i := 0; i < 3*37; i++ {
			prev[i] += line[1+i] // восстановление: Up
		}
		for x := 0; x < 37; x++ {
			c := src.RGBAAt(x, y)
			if prev[3*x] != c.R || prev[3*x+1] != c.G || prev[3*x+2] != c.B {
				t.Fatalf("пиксель (%d,%d): %v, ждали %v", x, y, prev[3*x:3*x+3], c)
			}
		}
	}
}

// TestTransparencyFlattenedToWhite — прозрачный пиксель на бумаге белый, а не
// чёрный; полупрозрачный смешивается с белым.
func TestTransparencyFlattenedToWhite(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 1))
	img.SetRGBA(0, 0, color.RGBA{0, 0, 0, 0})     // полностью прозрачный
	img.SetRGBA(1, 0, color.RGBA{0, 0, 0, 255})   // чёрный непрозрачный
	img.SetRGBA(2, 0, color.RGBA{100, 0, 0, 200}) // красный 200/255, предумноженный
	row := make([]byte, 9)
	fillRowRGB(img, 0, 0, 3, row)
	want := []byte{255, 255, 255, 0, 0, 0, 155, 55, 55}
	if !bytes.Equal(row, want) {
		t.Errorf("RGBA: %v, ждали %v", row, want)
	}

	// Та же картинка в непредумноженном виде и в общем (медленном) пути.
	n := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	n.SetNRGBA(0, 0, color.NRGBA{9, 9, 9, 0})
	n.SetNRGBA(1, 0, color.NRGBA{0, 0, 0, 255})
	n.SetNRGBA(2, 0, color.NRGBA{128, 0, 0, 128})
	fillRowRGB(n, 0, 0, 3, row)
	if row[0] != 255 || row[3] != 0 || row[6] < 190 || row[6] > 192 || row[7] < 126 || row[7] > 128 {
		t.Errorf("NRGBA: %v", row)
	}
	g := image.NewGray(image.Rect(0, 0, 2, 1))
	g.SetGray(1, 0, color.Gray{77})
	row = make([]byte, 6)
	fillRowRGB(g, 0, 0, 2, row)
	if !bytes.Equal(row, []byte{0, 0, 0, 77, 77, 77}) {
		t.Errorf("Gray: %v", row)
	}
}

// TestSubImageOffset — картинка с ненулевым Min (SubImage) читается с нужного
// места, а не с её собственного нуля.
func TestSubImageOffset(t *testing.T) {
	src := testImage(20, 20, color.RGBA{0, 0, 0, 255})
	sub := src.SubImage(image.Rect(5, 7, 15, 12)).(*image.RGBA)
	var buf bytes.Buffer
	if err := Write(&buf, []Page{{Image: sub, WidthPt: 100, HeightPt: 50}}, Options{}); err != nil {
		t.Fatal(err)
	}
	p := parsePDF(t, buf.Bytes())
	dict, body := p.stream(t, 5)
	if !strings.Contains(dict, "/Width 10 /Height 5") {
		t.Fatalf("размеры: %s", dict)
	}
	zr, _ := zlib.NewReader(bytes.NewReader(body))
	raw, _ := io.ReadAll(zr)
	// Первая строка (Up от нулей) — это сам первый ряд подкартинки.
	c := src.RGBAAt(5, 7)
	if raw[1] != c.R || raw[2] != c.G || raw[3] != c.B {
		t.Errorf("первый пиксель %v, ждали %v", raw[1:4], c)
	}
}

func TestJPEGPage(t *testing.T) {
	src := testImage(64, 48, color.RGBA{50, 60, 70, 255})
	var buf bytes.Buffer
	if err := Write(&buf, []Page{PageFromImage(src, 96)}, Options{JPEGQuality: 90}); err != nil {
		t.Fatal(err)
	}
	p := parsePDF(t, buf.Bytes())
	dict, body := p.stream(t, 5)
	if !strings.Contains(dict, "/DCTDecode") || strings.Contains(dict, "/DecodeParms") {
		t.Errorf("словарь: %s", dict)
	}
	img, err := jpeg.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("поток — не JPEG: %v", err)
	}
	if img.Bounds().Dx() != 64 || img.Bounds().Dy() != 48 {
		t.Errorf("размер JPEG %v", img.Bounds())
	}
}

func TestPageGeometry(t *testing.T) {
	// A4 при 300 dpi: 2480×3508 пикс → 595.2×841.9 пт.
	pg := PageFromImage(image.NewRGBA(image.Rect(0, 0, 2480, 3508)), 300)
	if fnum(pg.WidthPt) != "595.2" || fnum(pg.HeightPt) != "841.92" {
		t.Errorf("A4@300: %s×%s", fnum(pg.WidthPt), fnum(pg.HeightPt))
	}
	for v, want := range map[float64]string{1: "1", 0.5: "0.5", 595.276: "595.276", 1e6: "1000000", 12.0004: "12"} {
		if got := fnum(v); got != want {
			t.Errorf("fnum(%v) = %q, ждали %q", v, got, want)
		}
	}
}

func TestErrors(t *testing.T) {
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	cases := map[string][]Page{
		"нет картинки":      {{WidthPt: 100, HeightPt: 100}},
		"пустая картинка":   {{Image: image.NewRGBA(image.Rect(0, 0, 0, 0)), WidthPt: 100, HeightPt: 100}},
		"нулевой размер":    {{Image: img}},
		"слишком большая":   {{Image: img, WidthPt: 100, HeightPt: MaxPageSizePt + 1}},
		"NaN в размере":     {{Image: img, WidthPt: nan(), HeightPt: 100}},
		"отрицательный":     {{Image: img, WidthPt: -5, HeightPt: 100}},
		"нет страниц вовсе": nil,
	}
	for name, pages := range cases {
		buf.Reset()
		if err := Write(&buf, pages, Options{}); err == nil {
			t.Errorf("%s: ошибки нет", name)
		}
	}
	if _, err := NewWriter(&buf, Options{JPEGQuality: 101}); err == nil {
		t.Error("JPEGQuality=101 принят")
	}
	if _, err := NewWriter(nil, Options{}); err == nil {
		t.Error("nil-приёмник принят")
	}
}

func nan() float64 { var z float64; return z / z }

// failAfter — приёмник, который отказывает после n байт.
type failAfter struct{ n int }

func (f *failAfter) Write(p []byte) (int, error) {
	if len(p) > f.n {
		n := f.n
		f.n = 0
		return n, errors.New("диск полон")
	}
	f.n -= len(p)
	return len(p), nil
}

// TestWriteErrorSticky — ошибка записи не теряется и повторно не «лечится».
func TestWriteErrorSticky(t *testing.T) {
	img := PageFromImage(testImage(50, 50, color.RGBA{1, 1, 1, 255}), 72)
	err := Write(&failAfter{n: 200}, []Page{img, img}, Options{})
	if err == nil || !strings.Contains(err.Error(), "диск полон") {
		t.Fatalf("err = %v", err)
	}
}

// TestIncrementalMatchesWhole — постраничная запись даёт те же байты, что
// запись целиком.
func TestIncrementalMatchesWhole(t *testing.T) {
	a := PageFromImage(testImage(10, 10, color.RGBA{9, 9, 9, 255}), 72)
	b := PageFromImage(testImage(12, 8, color.RGBA{99, 9, 9, 255}), 72)
	var whole, inc bytes.Buffer
	if err := Write(&whole, []Page{a, b}, Options{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	w, _ := NewWriter(&inc, Options{Title: "x"})
	for _, pg := range []Page{a, b} {
		if err := w.AddPage(pg); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(whole.Bytes(), inc.Bytes()) {
		t.Error("постраничная запись отличается от целой")
	}
	if err := w.AddPage(a); err == nil {
		t.Error("страница в закрытый документ принята")
	}
	// Воспроизводимость: те же входные данные — те же байты.
	var again bytes.Buffer
	_ = Write(&again, []Page{a, b}, Options{Title: "x"})
	if !bytes.Equal(whole.Bytes(), again.Bytes()) {
		t.Error("вывод невоспроизводим")
	}
}

func TestManyPagesXref(t *testing.T) {
	img := PageFromImage(testImage(8, 8, color.RGBA{5, 5, 5, 255}), 72)
	pages := make([]Page, 25)
	for i := range pages {
		pages[i] = img
	}
	var buf bytes.Buffer
	if err := Write(&buf, pages, Options{}); err != nil {
		t.Fatal(err)
	}
	p := parsePDF(t, buf.Bytes())
	if p.size != 79 { // свободная запись + каталог + дерево + 75 + Info
		t.Fatalf("size=%d", p.size)
	}
	for n := 1; n < p.size; n++ {
		p.object(t, n)
	}
	if tree := string(p.object(t, 2)); !strings.Contains(tree, "/Count 25") {
		t.Errorf("дерево: %.60s", tree)
	}
}
