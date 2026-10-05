package svg

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestVisualSheet — инструмент для глаз, не проверка: рисует все *.svg из
// каталога SVG_SHEET_DIR в контактный лист SVG_SHEET_OUT (PNG, иконки 128 px
// на сером фоне; порядок — по имени файла). Если задан SVG_SHEET_REF — каталог
// с эталонами <имя>.png (например, от rsvg-convert -w 128 -h 128), рядом с
// каждой иконкой рисуется эталон, а в лог идёт среднее отклонение пикселей на
// белом фоне. Без переменных окружения тест пропускается.
//
//	SVG_SHEET_DIR=/путь/к/svg SVG_SHEET_OUT=sheet.png go test ./widget/svg -run VisualSheet -v
func TestVisualSheet(t *testing.T) {
	dir, out := os.Getenv("SVG_SHEET_DIR"), os.Getenv("SVG_SHEET_OUT")
	if dir == "" || out == "" {
		t.Skip("SVG_SHEET_DIR/SVG_SHEET_OUT не заданы")
	}
	refDir := os.Getenv("SVG_SHEET_REF")
	files, _ := filepath.Glob(filepath.Join(dir, "*.svg"))
	sort.Strings(files)
	cell := 128 // SVG_SHEET_CELL меняет размер ячейки (эталоны должны быть того же размера)
	if v, err := strconv.Atoi(os.Getenv("SVG_SHEET_CELL")); err == nil && v > 0 {
		cell = v
	}
	cols, per := 6, 1
	if refDir != "" {
		cols, per = 3, 2
	}
	if cell > 128 {
		cols = 2 / per
		if cols < 1 {
			cols = 1
		}
	}
	rows := (len(files) + cols - 1) / cols
	sheet := image.NewRGBA(image.Rect(0, 0, cols*per*(cell+8)+8, rows*(cell+8)+8))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.RGBA{200, 200, 200, 255}), image.Point{}, draw.Src)
	white := image.NewUniform(color.RGBA{255, 255, 255, 255})
	for i, f := range files {
		doc, err := ParseFile(f)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		img := Render(doc, cell, cell, color.RGBA{})
		x, y := 8+(i%cols)*per*(cell+8), 8+(i/cols)*(cell+8)
		r := image.Rect(x, y, x+cell, y+cell)
		draw.Draw(sheet, r, white, image.Point{}, draw.Src)
		draw.Draw(sheet, r, img, image.Point{}, draw.Over)
		msg := ""
		if refDir != "" {
			name := strings.TrimSuffix(filepath.Base(f), ".svg")
			if fh, err := os.Open(filepath.Join(refDir, name+".png")); err == nil {
				ref, _ := png.Decode(fh)
				fh.Close()
				if ref != nil {
					r2 := image.Rect(x+cell+8, y, x+2*cell+8, y+cell)
					draw.Draw(sheet, r2, white, image.Point{}, draw.Src)
					draw.Draw(sheet, r2, ref, ref.Bounds().Min, draw.Over)
					msg = diffMsg(sheet, r, r2)
				}
			}
		}
		t.Logf("%2d %-60s shapes=%-3d %s", i, filepath.Base(f), len(doc.Shapes), msg)
	}
	fh, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	if err := png.Encode(fh, sheet); err != nil {
		t.Fatal(err)
	}
}

// diffMsg — среднее абсолютное отклонение каналов между двумя ячейками листа.
func diffMsg(sheet *image.RGBA, a, b image.Rectangle) string {
	var sum float64
	n := 0
	for y := 0; y < a.Dy(); y++ {
		for x := 0; x < a.Dx(); x++ {
			ca := sheet.RGBAAt(a.Min.X+x, a.Min.Y+y)
			cb := sheet.RGBAAt(b.Min.X+x, b.Min.Y+y)
			sum += math.Abs(float64(ca.R)-float64(cb.R)) + math.Abs(float64(ca.G)-float64(cb.G)) + math.Abs(float64(ca.B)-float64(cb.B))
			n += 3
		}
	}
	return fmt.Sprintf("diff=%.2f", sum/float64(n))
}
