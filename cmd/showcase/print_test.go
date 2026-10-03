package main

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/cmd/internal/showcasestrings"
	"github.com/oops1/headless-gui/v3/printing"
	"github.com/oops1/headless-gui/v3/widget"
)

// Документ для «Сохранить в PDF» и «Печать…» собирается движком: две страницы
// A4 в разрешении принтера. Тест держит три вещи: страниц две и они в размер
// листа, на них что-то нарисовано (не пустой белый лист), а пакет printing
// принимает их и пишет настоящий PDF.

func sampleDoc() []widget.RichParagraph {
	return []widget.RichParagraph{
		{Runs: []widget.RichRun{{Text: "Заголовок", Font: widget.BuiltinFontBold, Size: 20}}},
		{Runs: []widget.RichRun{
			{Text: "Обычный текст, "},
			{Text: "красный", Color: color.RGBA{R: 224, G: 90, B: 90, A: 255}},
		}},
	}
}

func TestBuildPrintJob_TwoA4Pages(t *testing.T) {
	showcasestrings.Register()
	job, err := buildPrintJob(sampleDoc(), []string{"[12:00:00] первая строка", "[12:00:01] вторая строка"})
	if err != nil {
		t.Fatalf("buildPrintJob: %v", err)
	}
	if len(job.Pages) != 2 {
		t.Fatalf("страниц %d, ждали 2", len(job.Pages))
	}
	if err := job.Validate(); err != nil {
		t.Fatalf("задание не проходит проверку: %v", err)
	}

	// Лист A4: размер страницы-картинки совпадает с листом до пикселя-двух
	// (движок округляет логический размер).
	w, h := job.Setup.SheetSize()
	for i, p := range job.Pages {
		b := p.Bounds()
		if abs(b.Dx()-w) > 2 || abs(b.Dy()-h) > 2 {
			t.Errorf("страница %d: %d×%d, лист %d×%d", i+1, b.Dx(), b.Dy(), w, h)
		}
		if !hasInk(p.(interface {
			At(x, y int) color.Color
		}), b.Dx(), b.Dy()) {
			t.Errorf("страница %d осталась пустой", i+1)
		}
	}
}

func TestBuildPrintJob_SavesPDF(t *testing.T) {
	showcasestrings.Register()
	job, err := buildPrintJob(sampleDoc(), nil) // пустой журнал — тоже страница
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "showcase.pdf")
	if err := printing.SavePDF(path, job, printing.PDFOptions{Title: job.Name}); err != nil {
		t.Fatalf("SavePDF: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Errorf("файл не похож на PDF: %q", data[:min(8, len(data))])
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// hasInk — на странице есть хотя бы один заметно не белый пиксель.
func hasInk(img interface{ At(x, y int) color.Color }, w, h int) bool {
	for y := 0; y < h; y += 3 {
		for x := 0; x < w; x += 3 {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < 0xC000 || g < 0xC000 || b < 0xC000 {
				return true
			}
		}
	}
	return false
}
