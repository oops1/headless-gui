// Package printing — печать из приложений на движке.
//
// Модель. Движок рисует всё сам, в том числе страницу для бумаги: приложение
// рисует каждую страницу в *image.RGBA тем же DrawContext, что и экран (см.
// engine.New и Engine.RenderOnce), а этот пакет отправляет готовые страницы на
// принтер. Шрифты, отрисовка и вёрстка остаются едиными для экрана и бумаги:
// не нужно второй раз вёрстывать документ средствами печатной подсистемы ОС, и
// «на экране одно, на бумаге другое» исключено по построению.
//
// Как отправляется на принтер:
//   - Windows — системный диалог выбора принтера (PrintDlgEx) и печать через
//     GDI (StartDoc/StretchDIBits);
//   - Linux — IPP-запрос Print-Job прямо к CUPS по HTTP, документом PDF; без
//     внешних утилит lp/lpr, которых в пакете без зависимостей нет;
//   - macOS — не поддерживается (см. printing_unsupported.go);
//   - везде — сохранение в PDF (SavePDF, WritePDF).
//
// Размеры. Приложению надо знать, какого размера в пикселях рисовать страницу.
// PageSetup.SheetSize отвечает на это для бумаги, ориентации и разрешения.
package printing

import (
	"errors"
	"fmt"
	"image"
	"math"
	"strings"
)

// Orientation — ориентация листа.
type Orientation int

const (
	// Portrait — книжная: ширина листа не больше высоты. Нулевое значение —
	// книжная, как по умолчанию во всех системах.
	Portrait Orientation = iota
	// Landscape — альбомная: ширина не меньше высоты.
	Landscape
)

func (o Orientation) String() string {
	if o == Landscape {
		return "landscape"
	}
	return "portrait"
}

// Paper — размер бумаги в миллиметрах.
//
// Размеры хранятся без учёта ориентации: ориентацию задаёт PageSetup. Поэтому
// у Paper нет «ширины» и «высоты», только две стороны: короткая и длинная. Так
// произвольная бумага, заданная как 297×210, не превращается в странную
// «альбомную книжную».
type Paper struct {
	// Name — человеческое имя («A4») для интерфейса и имени документа.
	Name string
	// ShortMM, LongMM — короткая и длинная стороны листа в миллиметрах.
	ShortMM, LongMM float64
	// pwg — имя размера по PWG 5101.1 для IPP-атрибута media (у стандартных
	// размеров). Без него CUPS пришлось бы угадывать бумагу по PDF.
	pwg string
}

// Стандартные размеры бумаги.
var (
	A3        = Paper{"A3", 297, 420, "iso_a3_297x420mm"}
	A4        = Paper{"A4", 210, 297, "iso_a4_210x297mm"}
	A5        = Paper{"A5", 148, 210, "iso_a5_148x210mm"}
	A6        = Paper{"A6", 105, 148, "iso_a6_105x148mm"}
	Letter    = Paper{"Letter", 215.9, 279.4, "na_letter_8.5x11in"}
	Legal     = Paper{"Legal", 215.9, 355.6, "na_legal_8.5x14in"}
	Tabloid   = Paper{"Tabloid", 279.4, 431.8, "na_ledger_11x17in"}
	Executive = Paper{"Executive", 184.15, 266.7, "na_executive_7.25x10.5in"}
)

// CustomPaper — бумага произвольного размера. Стороны можно задать в любом
// порядке: короткой станет меньшая.
func CustomPaper(name string, widthMM, heightMM float64) Paper {
	s, l := widthMM, heightMM
	if s > l {
		s, l = l, s
	}
	if name == "" {
		name = fmt.Sprintf("%g×%g мм", round2(s), round2(l))
	}
	return Paper{Name: name, ShortMM: s, LongMM: l}
}

// SizeMM возвращает ширину и высоту листа в миллиметрах в данной ориентации.
func (p Paper) SizeMM(o Orientation) (w, h float64) {
	if o == Landscape {
		return p.LongMM, p.ShortMM
	}
	return p.ShortMM, p.LongMM
}

// PWGName — имя размера по PWG 5101.1 для IPP-атрибута media: «iso_a4_210x297mm».
// Для своего размера строится самоописывающее имя «custom_имя_ШxВmm». Принтер,
// который такого размера не знает, атрибут игнорирует (CUPS отвечает «успех с
// заменой»), а не отказывает, так что лишним он не бывает.
func (p Paper) PWGName() string {
	if p.pwg != "" {
		return p.pwg
	}
	return fmt.Sprintf("custom_%s_%gx%gmm", pwgToken(p.Name), round2(p.ShortMM), round2(p.LongMM))
}

// pwgToken оставляет в имени только допустимые для keyword символы
// (латиница, цифры, '-', '.'): кириллица и пробелы в значении keyword
// недопустимы, и сервер отверг бы весь запрос, а не один атрибут.
func pwgToken(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
			b.WriteRune(r)
		case r == ' ' || r == '_':
			b.WriteByte('-')
		}
	}
	// Края без дефисов и точек: имя «-5x3» после выброса кириллицы выглядело бы
	// как параметр командной строки, а не как слово.
	s = strings.Trim(b.String(), "-.")
	if s == "" {
		return "paper"
	}
	return s
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// Margins — поля в миллиметрах.
type Margins struct{ Left, Top, Right, Bottom float64 }

// UniformMargins — одинаковые поля со всех сторон.
func UniformMargins(mm float64) Margins { return Margins{mm, mm, mm, mm} }

// Разумные границы. Нижняя граница DPI — экранный предел; верхняя вместе с
// MaxPagePixels не даёт запросить страницу, на которую не хватит памяти.
const (
	// DefaultDPI — разрешение по умолчанию: 300 точек на дюйм хватает для
	// текста, а A4 при нём — 35 МБ вместо 560 МБ при 1200.
	DefaultDPI = 300
	// MinDPI, MaxDPI — допустимый диапазон разрешения.
	MinDPI = 72
	MaxDPI = 1200
	// MaxPagePixels — наибольшая площадь страницы в пикселях (≈400 МБ RGBA).
	// Тот же порядок, что у холста движка (engine.MaxCanvasPixels).
	MaxPagePixels = 100 << 20
	// MaxPaperMM — наибольшая сторона бумаги: PDF не допускает страниц больше
	// 200 дюймов.
	MaxPaperMM = 5080
)

const mmPerInch = 25.4

// PageSetup — параметры страницы: бумага, ориентация, поля и разрешение, в
// котором приложение рисует страницы.
type PageSetup struct {
	Paper       Paper
	Orientation Orientation
	// Margins — поля. Страница рисуется на ВЕСЬ лист; поля лишь говорят
	// приложению, где рисовать содержимое (ContentRect). Так лист совпадает с
	// бумагой миллиметр в миллиметр, а принтеры с разными непечатаемыми
	// краями печатают одно и то же.
	Margins Margins
	// DPI — разрешение страницы-картинки; 0 означает DefaultDPI.
	DPI int
}

// NewPageSetup — книжный лист бумаги с полями в 20 мм слева и 10 мм с остальных
// сторон и разрешением по умолчанию. Это не рекомендация, а отправная точка: поля
// приложение меняет сразу после.
func NewPageSetup(p Paper) PageSetup {
	return PageSetup{Paper: p, Margins: Margins{Left: 20, Top: 10, Right: 10, Bottom: 10}}
}

// ErrBadSetup — параметры страницы недопустимы; подробности в тексте ошибки.
var ErrBadSetup = errors.New("printing: недопустимые параметры страницы")

// dpi возвращает разрешение с учётом умолчания.
func (s PageSetup) dpi() int {
	if s.DPI == 0 {
		return DefaultDPI
	}
	return s.DPI
}

// EffectiveDPI — разрешение, в котором страницы будут рисоваться (с учётом
// значения по умолчанию).
func (s PageSetup) EffectiveDPI() int { return s.dpi() }

// Validate проверяет параметры страницы.
func (s PageSetup) Validate() error {
	if !(s.Paper.ShortMM > 0 && s.Paper.LongMM > 0) ||
		s.Paper.ShortMM > MaxPaperMM || s.Paper.LongMM > MaxPaperMM {
		return fmt.Errorf("%w: размер бумаги %g×%g мм", ErrBadSetup, s.Paper.ShortMM, s.Paper.LongMM)
	}
	if s.Orientation != Portrait && s.Orientation != Landscape {
		return fmt.Errorf("%w: ориентация %d", ErrBadSetup, int(s.Orientation))
	}
	if d := s.dpi(); d < MinDPI || d > MaxDPI {
		return fmt.Errorf("%w: разрешение %d dpi вне %d…%d", ErrBadSetup, d, MinDPI, MaxDPI)
	}
	w, h := s.SheetSize()
	if int64(w)*int64(h) > MaxPagePixels {
		return fmt.Errorf("%w: страница %d×%d пикселей слишком велика (предел %d пикселей); уменьшите разрешение",
			ErrBadSetup, w, h, MaxPagePixels)
	}
	// Форма «!(0 ≤ x ≤ предел)» отсекает и отрицательные поля, и NaN, и
	// бесконечность: round от них даёт непредсказуемое целое.
	for _, v := range []float64{s.Margins.Left, s.Margins.Top, s.Margins.Right, s.Margins.Bottom} {
		if !(v >= 0 && v <= MaxPaperMM) {
			return fmt.Errorf("%w: поле %g мм", ErrBadSetup, v)
		}
	}
	if s.ContentRect().Empty() {
		return fmt.Errorf("%w: поля не оставляют места для содержимого", ErrBadSetup)
	}
	return nil
}

// PagePixels — размер листа бумаги в пикселях при заданном разрешении: сколько
// пикселей рисовать приложению, чтобы получить страницу ровно по размеру бумаги.
// Для недопустимых данных возвращает 0, 0.
func PagePixels(p Paper, o Orientation, dpi int) (w, h int) {
	if dpi <= 0 || !(p.ShortMM > 0 && p.LongMM > 0) {
		return 0, 0
	}
	wmm, hmm := p.SizeMM(o)
	return mmToPx(wmm, dpi), mmToPx(hmm, dpi)
}

func mmToPx(mm float64, dpi int) int {
	return int(math.Round(mm * float64(dpi) / mmPerInch))
}

// SheetSize — размер листа в пикселях при разрешении страницы.
func (s PageSetup) SheetSize() (w, h int) { return PagePixels(s.Paper, s.Orientation, s.dpi()) }

// Bounds — прямоугольник страницы в пикселях: (0,0)–(SheetSize). Удобен для
// image.NewRGBA.
func (s PageSetup) Bounds() image.Rectangle {
	w, h := s.SheetSize()
	return image.Rect(0, 0, w, h)
}

// ContentRect — область внутри полей в пикселях страницы: именно здесь
// приложению следует рисовать содержимое. Пустой прямоугольник означает, что
// поля больше листа.
func (s PageSetup) ContentRect() image.Rectangle {
	w, h := s.SheetSize()
	d := s.dpi()
	// Границы считаются отдельными числами, а не через image.Rect: тот молча
	// меняет местами перевёрнутые углы, и «поля шире листа» превратились бы в
	// корректный, но ложный прямоугольник.
	x0, y0 := mmToPx(s.Margins.Left, d), mmToPx(s.Margins.Top, d)
	x1, y1 := w-mmToPx(s.Margins.Right, d), h-mmToPx(s.Margins.Bottom, d)
	if x0 < 0 || y0 < 0 || x1 > w || y1 > h || x1 <= x0 || y1 <= y0 {
		return image.Rectangle{}
	}
	return image.Rect(x0, y0, x1, y1)
}

// PagePoints — размер листа в пунктах (1/72 дюйма) — единица PDF.
func (s PageSetup) PagePoints() (w, h float64) {
	wmm, hmm := s.Paper.SizeMM(s.Orientation)
	return wmm * 72 / mmPerInch, hmm * 72 / mmPerInch
}
