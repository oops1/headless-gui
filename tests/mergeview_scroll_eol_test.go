package tests

import (
	"fmt"
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// GG-62: полоса-обзор MergeView отзывается на мышь, прокрутку можно задать
// снаружи. GG-60: итог после SetChunks кончается переводом строки, вид записи
// и длина маркеров задаются.

// mergeLong — файл в 120 строк с тремя далеко разнесёнными конфликтами: верхние
// панели заметно длиннее окна.
func mergeLong() (base, ours, theirs string) {
	var b, o, t strings.Builder
	for i := 0; i < 120; i++ {
		line := fmt.Sprintf("line %d", i)
		b.WriteString(line + "\n")
		switch i {
		case 10, 60, 110:
			o.WriteString(line + " ours\n")
			t.WriteString(line + " theirs\n")
		default:
			o.WriteString(line + "\n")
			t.WriteString(line + "\n")
		}
	}
	return b.String(), o.String(), t.String()
}

// Полоса-обзор стоит у правого края: контрол шириной 1000 держит её в полосе
// x = 974…986 (поле 14, ширина 12).
const mvRulerX = 980

func mergeLongView(t *testing.T) *widget.MergeView {
	t.Helper()
	mv := widget.NewMergeView("", "")
	mv.SetBounds(image.Rect(0, 0, 1000, 600))
	mv.SetTexts(mergeLong())
	if mv.ConflictCount() != 3 {
		t.Fatalf("подготовка: конфликтов %d, хочу 3", mv.ConflictCount())
	}
	return mv
}

func mvPress(mv *widget.MergeView, x, y int) {
	mv.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
}

func mvRelease(mv *widget.MergeView, x, y int) {
	mv.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
}

// Щелчок по полосе ниже ползунка переносит туда видимую область, и мышь
// контрол забирает себе — иначе протяжку получил бы кто-то другой.
func TestMergeView_RulerClickScrolls(t *testing.T) {
	mv := mergeLongView(t)
	e := widget.MouseEvent{X: mvRulerX, Y: 500, Button: widget.MouseLeft, Pressed: true}
	if !mv.WantsCapture(e) {
		t.Fatal("контрол не берёт мышь над полосой-обзором")
	}
	mvPress(mv, mvRulerX, 500)
	mvRelease(mv, mvRulerX, 500)
	if mv.Scroll() <= 0 {
		t.Fatalf("щелчок по полосе не прокрутил панели: scroll=%v", mv.Scroll())
	}
}

// Ползунок тащится без скачка: нажатие на нём не двигает панели, протяжка
// двигает, а возврат мыши на место возвращает и прокрутку.
func TestMergeView_RulerThumbDrag(t *testing.T) {
	mv := mergeLongView(t)
	mv.SetScroll(0)

	// Ползунок у верхнего края полосы (она начинается под шапками, ниже y≈58).
	mvPress(mv, mvRulerX, 70)
	if got := mv.Scroll(); got != 0 {
		t.Fatalf("нажатие на ползунке прокрутило панели на %v — схватить без скачка нельзя", got)
	}
	mv.OnMouseMove(mvRulerX, 170)
	moved := mv.Scroll()
	if moved < 300 {
		t.Fatalf("протяжка ползунка на 100 точек прокрутила только на %v", moved)
	}
	mv.OnMouseMove(mvRulerX, 70)
	if got := mv.Scroll(); got > 1 {
		t.Fatalf("ползунок вернули на место, а прокрутка %v", got)
	}
	mvRelease(mv, mvRulerX, 70)

	// После отпускания движение мыши полосу больше не тащит.
	mv.OnMouseMove(mvRulerX, 400)
	if got := mv.Scroll(); got > 1 {
		t.Fatalf("после отпускания панели едут за мышью: scroll=%v", got)
	}
}

// Щелчок по отметке конфликта на полосе — переход к этому конфликту.
func TestMergeView_RulerMarkGoesToConflict(t *testing.T) {
	mv := mergeLongView(t)
	found := false
	for y := 60; y < 580 && !found; y++ {
		mv.SetScroll(0) // ползунок наверху и не закрывает отметку второго
		mvPress(mv, mvRulerX, y)
		mvRelease(mv, mvRulerX, y)
		found = mv.CurrentConflict() == 1
	}
	if !found {
		t.Fatal("ни один щелчок по полосе не перешёл ко второму конфликту")
	}
	if mv.Scroll() <= 0 {
		t.Fatal("переход к конфликту не прокрутил панели к нему")
	}
}

// Прокрутка снаружи: SetScroll ограничивается краями, ScrollToLine ставит строку
// в видимую область.
func TestMergeView_ScrollAPI(t *testing.T) {
	mv := mergeLongView(t)
	mv.SetScroll(-50)
	if got := mv.Scroll(); got != 0 {
		t.Fatalf("SetScroll(-50) = %v, хочу 0", got)
	}
	mv.SetScroll(1e9)
	top := mv.Scroll()
	if top <= 0 || top >= 1e9 {
		t.Fatalf("SetScroll за нижний край не ограничен: %v", top)
	}

	mv.SetScroll(0)
	mv.ScrollToLine(widget.MergeTheirs, 100)
	if got := mv.Scroll(); got <= 0 {
		t.Fatalf("ScrollToLine(их, 100) не прокрутил верхние панели: %v", got)
	}
	mv.ScrollToLine(widget.MergeResult, 100)
	if got := mv.ResultScroll(); got <= 0 {
		t.Fatalf("ScrollToLine(итог, 100) не прокрутил итог: %v", got)
	}
}

// GG-60: итог готовыми блоками кончается переводом строки; вид записи задаётся
// и переживает SetChunks; пустое слияние — пустой файл.
func TestMergeView_ResultEOL(t *testing.T) {
	mv := widget.NewMergeView("", "")
	mv.SetChunks([]widget.MergeChunk{{Merged: []string{"one"}}})
	if got := mv.Result(); got != "one\n" {
		t.Fatalf("итог блоков %q, хочу \"one\\n\"", got)
	}

	mv.SetResultEOL("\r\n", true, false)
	if got := mv.Result(); got != "\xEF\xBB\xBFone" {
		t.Fatalf("BOM без перевода в конце: %q", got)
	}
	mv.SetChunks([]widget.MergeChunk{{Merged: []string{"a", "b"}}})
	if got := mv.Result(); got != "\xEF\xBB\xBFa\r\nb" {
		t.Fatalf("настройка записи не пережила SetChunks: %q", got)
	}
	if eol, bom, fin := mv.ResultEOL(); eol != "\r\n" || !bom || fin {
		t.Fatalf("ResultEOL = %q %v %v", eol, bom, fin)
	}

	mv.SetResultEOL("", false, true)
	mv.SetChunks(nil)
	if got := mv.Result(); got != "" {
		t.Fatalf("пустое слияние дало %q вместо пустого файла", got)
	}

	// SetTexts берёт вид у нашей стороны.
	mv.SetTexts("a\r\n", "a\r\n", "a\r\n")
	if got := mv.Result(); got != "a\r\n" {
		t.Fatalf("SetTexts: итог %q, хочу \"a\\r\\n\"", got)
	}
}

// GG-60: длина маркеров как conflict-marker-size у git.
func TestMergeView_MarkerSize(t *testing.T) {
	mv := widget.NewMergeView("", "")
	mv.SetTexts(mvBase, mvOurs, mvTheirs)
	if got := mv.MarkerSize(); got != 7 {
		t.Fatalf("длина маркера по умолчанию %d, хочу 7", got)
	}

	mv.SetMarkerSize(10)
	lines := mv.ResultLines()
	for _, want := range []string{"<<<<<<<<<< HEAD", "==========", ">>>>>>>>>> merge head"} {
		if !slices.Contains(lines, want) {
			t.Fatalf("нет строки %q в итоге:\n%s", want, strings.Join(lines, "\n"))
		}
	}

	mv.SetMarkerSize(0)
	if !slices.Contains(mv.ResultLines(), "<<<<<<< HEAD") {
		t.Fatalf("SetMarkerSize(0) не вернул семь знаков:\n%s", mv.Result())
	}
}

// Смена стиля, длины маркеров и подписей сторон переписывает только маркеры
// нерешённых конфликтов: правка руками остаётся. Раньше итог пересобирался
// целиком и правка пропадала — в демо это случалось при каждой смене языка.
func TestMergeView_FormatKeepsManualEdits(t *testing.T) {
	mv := widget.NewMergeView("", "")
	mv.SetTexts(mvBase, mvOurs, mvTheirs)
	mv.SetCaret(0, len("package ui"))
	mv.InsertText(" // руками")

	mv.SetStyle(widget.MergeStyleDiff3)
	mv.SetMarkerSize(9)
	mv.SetSides(
		widget.MergeSideInfo{Title: "main"},
		widget.MergeSideInfo{Title: "base"},
		widget.MergeSideInfo{Title: "feature"},
	)

	res := mv.Result()
	if !strings.Contains(res, "// руками") {
		t.Fatalf("правка руками стёрта сменой вида маркеров:\n%s", res)
	}
	lines := mv.ResultLines()
	for _, want := range []string{"<<<<<<<<< main", "||||||||| base", "=========", ">>>>>>>>> feature"} {
		if !slices.Contains(lines, want) {
			t.Fatalf("нет строки %q:\n%s", want, res)
		}
	}
	if mv.Unresolved() != 1 {
		t.Fatalf("нерешённых %d, хочу 1", mv.Unresolved())
	}
}
