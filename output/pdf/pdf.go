// Package pdf собирает PDF-файл из страниц-картинок. Без зависимостей: заголовок,
// объекты, таблица xref и trailer пишутся вручную.
//
// Зачем это движку. Он рисует всё сам, в том числе страницу для печати: тем же
// DrawContext, что и экран, в *image.RGBA. Готовую картинку страницы надо куда-то
// отдать. На Linux единственный общий язык с принтером — это CUPS, а он ждёт
// документ в понятном ему формате: PDF. Отдельный полезный выход — «Сохранить
// как PDF» без всякого принтера, и он нужен на любой платформе.
//
// Каждая страница — одна картинка на всю страницу. Текста в PDF нет: он
// растеризован движком, поэтому шрифты, начертание и вёрстка на бумаге
// совпадают с экраном буква в букву, а файл не зависит от шрифтов у читателя.
// Цена — текст в файле не выделяется и не ищется; это осознанный выбор.
//
// Картинка сжимается без потерь (Flate, по умолчанию) или с потерями (JPEG,
// если задано Options.JPEGQuality). Прозрачность смешивается с белым: бумага
// белая, а у PDF-картинки без маски альфа-канала нет, и прозрачный пиксель
// иначе оказался бы чёрным.
package pdf

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// Ограничения на размер страницы в пунктах PDF (1 пт = 1/72 дюйма). Верхний —
// из спецификации Acrobat (200 дюймов): страницы больше многие просмотрщики
// отказываются открывать. Нижний защищает от нулевых и отрицательных размеров,
// после которых файл получился бы корректным по структуре и пустым на экране.
const (
	MinPageSizePt = 3
	MaxPageSizePt = 14400
)

// Options — параметры документа.
type Options struct {
	// Title, Author, Creator попадают в словарь Info. Любые символы Юникода:
	// не-ASCII кодируется в UTF-16BE, как требует PDF, иначе русское название
	// в свойствах файла превратилось бы в кракозябры.
	Title   string
	Author  string
	Creator string

	// Created — время создания. Нулевое — в файл не пишется. Нулевое значение
	// по умолчанию нужно ради воспроизводимости: один и тот же ввод даёт те же
	// байты, и файл можно сравнивать в тестах.
	Created time.Time

	// JPEGQuality: 0 — страницы сжимаются без потерь (Flate), 1..100 — JPEG с
	// этим качеством. JPEG на порядок меньше для фотографий, но размывает
	// мелкий текст; для интерфейса приложения без потерь обычно лучше.
	JPEGQuality int
}

// Page — одна страница: картинка и размер страницы в пунктах.
type Page struct {
	Image image.Image
	// WidthPt, HeightPt — размер страницы в пунктах PDF (1/72 дюйма). Картинка
	// растягивается на всю страницу. Размер не берётся из пикселей молча:
	// пиксели без DPI ничего не значат, а печать с чужим масштабом — это
	// «A4 превратился в открытку».
	WidthPt, HeightPt float64
}

// PageFromImage строит страницу по картинке, нарисованной в заданном
// разрешении: размер в пунктах = пиксели × 72 / dpi.
func PageFromImage(img image.Image, dpi float64) Page {
	if img == nil || dpi <= 0 {
		return Page{Image: img}
	}
	b := img.Bounds()
	return Page{
		Image:    img,
		WidthPt:  float64(b.Dx()) * 72 / dpi,
		HeightPt: float64(b.Dy()) * 72 / dpi,
	}
}

// Writer пишет PDF по одной странице. Нужен ради памяти: приложение рисует
// страницу, добавляет её и тут же отпускает картинку — все страницы разом в
// памяти держать не требуется (A4 при 300 dpi — это 35 МБ на страницу).
type Writer struct {
	cw    countWriter
	opt   Options
	pages int
	// offsets[n] — смещение объекта номер n от начала файла. Нумерация
	// детерминирована заранее: 1 — каталог, 2 — дерево страниц, дальше по три
	// объекта на страницу (страница, содержимое, картинка) и в конце Info.
	// Поэтому дерево страниц можно записать при закрытии, когда число страниц
	// уже известно, а xref всё равно считается по смещениям, а не по порядку.
	offsets map[int]int64
	closed  bool
	err     error
}

// countWriter считает записанные байты. Без него смещения xref пришлось бы
// считать «в уме» вдоль каждого Fprintf, и первая же ошибка на один байт
// делает файл нечитаемым: просмотрщик идёт по смещению и не находит объект.
type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// NewWriter начинает документ и пишет заголовок.
func NewWriter(w io.Writer, opt Options) (*Writer, error) {
	if w == nil {
		return nil, errors.New("pdf: нет приёмника для записи")
	}
	if opt.JPEGQuality < 0 || opt.JPEGQuality > 100 {
		return nil, fmt.Errorf("pdf: JPEGQuality %d вне 0..100", opt.JPEGQuality)
	}
	pw := &Writer{cw: countWriter{w: w}, opt: opt, offsets: map[int]int64{}}
	// Вторая строка из четырёх байт ≥ 0x80 — рекомендация спецификации (7.5.2):
	// по ней программы передачи и почтовые шлюзы понимают, что файл двоичный, и
	// не портят его, «исправляя» переводы строк.
	pw.write("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	if pw.err != nil {
		return nil, pw.err
	}
	return pw, nil
}

func (w *Writer) write(s string) {
	if w.err == nil {
		_, w.err = io.WriteString(&w.cw, s)
	}
}

func (w *Writer) writeBytes(b []byte) {
	if w.err == nil {
		_, w.err = w.cw.Write(b)
	}
}

// begin отмечает смещение объекта и открывает его.
func (w *Writer) begin(num int) {
	w.offsets[num] = w.cw.n
	w.write(strconv.Itoa(num) + " 0 obj\n")
}

func (w *Writer) end() { w.write("endobj\n") }

// Номера объектов страницы i (с нуля).
func pageObj(i int) int    { return 3 + 3*i }
func contentObj(i int) int { return 4 + 3*i }
func imageObj(i int) int   { return 5 + 3*i }

// AddPage дописывает страницу.
func (w *Writer) AddPage(p Page) error {
	if w.err != nil {
		return w.err
	}
	if w.closed {
		return errors.New("pdf: документ уже закрыт")
	}
	if p.Image == nil {
		return errors.New("pdf: у страницы нет картинки")
	}
	b := p.Image.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw <= 0 || ih <= 0 {
		return errors.New("pdf: у картинки страницы пустой размер")
	}
	if !(p.WidthPt >= MinPageSizePt && p.WidthPt <= MaxPageSizePt &&
		p.HeightPt >= MinPageSizePt && p.HeightPt <= MaxPageSizePt) {
		return fmt.Errorf("pdf: размер страницы %.1f×%.1f пт вне %d…%d",
			p.WidthPt, p.HeightPt, MinPageSizePt, MaxPageSizePt)
	}

	// Сначала сжимаем, потом пишем: длина потока должна стоять в словаре ДО
	// самого потока, а получить её можно только после сжатия. Прямая длина в
	// заголовке надёжнее косвенной (/Length N 0 R): так читают все.
	var (
		data   []byte
		filter string
		parms  string
		err    error
	)
	if w.opt.JPEGQuality > 0 {
		data, err = encodeJPEG(p.Image, w.opt.JPEGQuality)
		filter = "/DCTDecode"
	} else {
		data, err = encodeFlate(p.Image)
		filter = "/FlateDecode"
		// Predictor 15 + «Up» на каждой строке: интерфейсные страницы — это
		// плоские заливки, и разность с верхней строкой превращает их в нули,
		// которые сжимаются в разы лучше сырых байтов.
		parms = fmt.Sprintf(" /DecodeParms << /Predictor 15 /Colors 3 /BitsPerComponent 8 /Columns %d >>", iw)
	}
	if err != nil {
		return fmt.Errorf("pdf: сжатие страницы: %w", err)
	}

	i := w.pages
	wpt, hpt := fnum(p.WidthPt), fnum(p.HeightPt)

	w.begin(pageObj(i))
	w.write(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] "+
		"/Resources << /XObject << /Im0 %d 0 R >> /ProcSet [/PDF /ImageC] >> /Contents %d 0 R >>\n",
		wpt, hpt, imageObj(i), contentObj(i)))
	w.end()

	// Картинка растягивается матрицей на всю страницу: единичный квадрат XObject
	// масштабируется до размера страницы в пунктах.
	content := fmt.Sprintf("q %s 0 0 %s 0 0 cm /Im0 Do Q\n", wpt, hpt)
	w.begin(contentObj(i))
	w.write(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream\n", len(content), content))
	w.end()

	w.begin(imageObj(i))
	w.write(fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d "+
		"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter %s%s /Length %d >>\nstream\n",
		iw, ih, filter, parms, len(data)))
	w.writeBytes(data)
	w.write("\nendstream\n")
	w.end()

	if w.err != nil {
		return w.err
	}
	w.pages++
	return nil
}

// Close завершает документ: дерево страниц, каталог, Info, xref, trailer.
// Нижележащий io.Writer не закрывается — им владеет вызывающий. Без страниц
// документ не создаётся: PDF с нулём страниц многие просмотрщики считают
// повреждённым.
func (w *Writer) Close() error {
	if w.err != nil {
		return w.err
	}
	if w.closed {
		return nil
	}
	w.closed = true
	if w.pages == 0 {
		w.err = errors.New("pdf: в документе нет ни одной страницы")
		return w.err
	}

	w.begin(1)
	w.write("<< /Type /Catalog /Pages 2 0 R >>\n")
	w.end()

	var kids strings.Builder
	for i := 0; i < w.pages; i++ {
		if i > 0 {
			kids.WriteByte(' ')
		}
		kids.WriteString(strconv.Itoa(pageObj(i)) + " 0 R")
	}
	w.begin(2)
	w.write(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>\n", kids.String(), w.pages))
	w.end()

	info := 3 + 3*w.pages
	w.begin(info)
	w.write("<< /Producer " + pdfText("headless-gui"))
	if w.opt.Title != "" {
		w.write(" /Title " + pdfText(w.opt.Title))
	}
	if w.opt.Author != "" {
		w.write(" /Author " + pdfText(w.opt.Author))
	}
	if w.opt.Creator != "" {
		w.write(" /Creator " + pdfText(w.opt.Creator))
	}
	if !w.opt.Created.IsZero() {
		w.write(" /CreationDate (" + w.opt.Created.UTC().Format("D:20060102150405") + "Z)")
	}
	w.write(" >>\n")
	w.end()

	// xref. Каждая запись — РОВНО 20 байт ("0000000015 00000 n \n"): читатель
	// находит запись объекта N арифметикой, смещение = начало_таблицы + 20·N, а
	// не поиском. Лишний или недостающий байт в одной записи сдвигает все
	// следующие, и просмотрщик «чинит» файл перебором или отказывается его
	// открывать. Поэтому пробел перед переводом строки обязателен.
	size := info + 1
	xrefPos := w.cw.n
	w.write(fmt.Sprintf("xref\n0 %d\n", size))
	w.write("0000000000 65535 f \n")
	for n := 1; n < size; n++ {
		off, ok := w.offsets[n]
		if !ok {
			w.err = fmt.Errorf("pdf: внутренняя ошибка: объект %d не записан", n)
			return w.err
		}
		w.write(fmt.Sprintf("%010d 00000 n \n", off))
	}
	w.write(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		size, info, xrefPos))
	return w.err
}

// Write собирает документ целиком из готовых страниц.
func Write(w io.Writer, pages []Page, opt Options) error {
	pw, err := NewWriter(w, opt)
	if err != nil {
		return err
	}
	for i, p := range pages {
		if err := pw.AddPage(p); err != nil {
			return fmt.Errorf("страница %d: %w", i+1, err)
		}
	}
	return pw.Close()
}

// fnum печатает число для PDF: без экспоненты и без хвостовых нулей. В PDF
// нет научной записи, а "1e+06" читатель разберёт как мусор.
func fnum(v float64) string {
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// pdfText кодирует строку для словаря Info. ASCII без управляющих символов —
// обычная строка в скобках (с экранированием); всё остальное — UTF-16BE с
// меткой порядка байт FEFF в шестнадцатеричной записи: так PDF хранит текст
// вне PDFDocEncoding, и именно так русское название читается правильно.
func pdfText(s string) string {
	plain := true
	for _, r := range s {
		if r < 0x20 || r > 0x7e {
			plain = false
			break
		}
	}
	if plain {
		var b strings.Builder
		b.WriteByte('(')
		for _, r := range s {
			if r == '(' || r == ')' || r == '\\' {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
		b.WriteByte(')')
		return b.String()
	}
	var b strings.Builder
	b.WriteString("<FEFF")
	for _, u := range utf16.Encode([]rune(s)) {
		fmt.Fprintf(&b, "%04X", u)
	}
	b.WriteByte('>')
	return b.String()
}

// ─── Сжатие картинки ─────────────────────────────────────────────────────────

// encodeFlate кодирует картинку как RGB 8 бит со сжатием zlib и PNG-предсказателем
// «Up». Здесь именно zlib, а не «голый» deflate из compress/flate: фильтр
// FlateDecode в PDF — это формат RFC 1950 с двухбайтным заголовком и
// контрольной суммой. Без обёртки читатель видит «Unknown compression method».
func encodeFlate(img image.Image) ([]byte, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	var out bytes.Buffer
	zw, err := zlib.NewWriterLevel(&out, zlib.DefaultCompression)
	if err != nil {
		return nil, err
	}
	// Строка обрабатывается без копии всей картинки: на 300 dpi A4 это сэкономило
	// бы лишние 26 МБ на каждой странице.
	cur := make([]byte, 3*w)
	prev := make([]byte, 3*w)
	line := make([]byte, 1+3*w)
	line[0] = 2 // фильтр PNG «Up»
	for y := 0; y < h; y++ {
		fillRowRGB(img, b.Min.X, b.Min.Y+y, w, cur)
		for i := range cur {
			line[1+i] = cur[i] - prev[i]
		}
		if _, err := zw.Write(line); err != nil {
			return nil, err
		}
		cur, prev = prev, cur
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// fillRowRGB записывает в dst строку из w пикселей (x0, y), смешанных с белым.
func fillRowRGB(img image.Image, x0, y, w int, dst []byte) {
	switch m := img.(type) {
	case *image.RGBA:
		// Предумноженный альфой RGBA: c + (255-a) — это то же, что смешение с
		// белым. Для непрозрачного пикселя (a=255) выражение равно c.
		o := m.PixOffset(x0, y)
		for x := 0; x < w; x++ {
			p := m.Pix[o+4*x : o+4*x+4 : o+4*x+4]
			na := 255 - int(p[3])
			dst[3*x] = clamp8(int(p[0]) + na)
			dst[3*x+1] = clamp8(int(p[1]) + na)
			dst[3*x+2] = clamp8(int(p[2]) + na)
		}
	case *image.NRGBA:
		o := m.PixOffset(x0, y)
		for x := 0; x < w; x++ {
			p := m.Pix[o+4*x : o+4*x+4 : o+4*x+4]
			a := int(p[3])
			// Непредумноженная: c·a/255 + (255-a).
			dst[3*x] = clamp8((int(p[0])*a+127)/255 + 255 - a)
			dst[3*x+1] = clamp8((int(p[1])*a+127)/255 + 255 - a)
			dst[3*x+2] = clamp8((int(p[2])*a+127)/255 + 255 - a)
		}
	default:
		// Любая другая картинка — медленно, через интерфейс. RGBA() даёт 16 бит
		// с предумножением.
		for x := 0; x < w; x++ {
			r, g, bl, a := img.At(x0+x, y).RGBA()
			na := 0xffff - a
			dst[3*x] = clamp8(int((r + na) >> 8))
			dst[3*x+1] = clamp8(int((g + na) >> 8))
			dst[3*x+2] = clamp8(int((bl + na) >> 8))
		}
	}
}

func clamp8(v int) byte {
	if v > 255 {
		return 255
	}
	if v < 0 {
		return 0
	}
	return byte(v)
}

// encodeJPEG кодирует картинку в JPEG. Перед этим она смешивается с белым:
// кодек JPEG альфу отбрасывает, и прозрачное место стало бы чёрным.
func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	b := img.Bounds()
	flat := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(flat, flat.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), img, b.Min, draw.Over)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, flat, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
