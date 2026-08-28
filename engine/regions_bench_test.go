// regions_bench_test.go — цена markTiles на построчной заливке.
//
// Линейный градиент (widget.drawLinearGradient) и — на дробном HiDPI —
// радиальный (через DrawImageScaled он строчной заливки не делает, но сам
// линейный градиент на scale==1 её не миновал) кладут заливку не одним
// прямоугольником, а построчно: DrawHLine/FillRect в цикле по каждой строке
// области. На область высотой 300 точек это 300 отдельных вызовов markSolid,
// каждый — полоса высотой в 1 физический пиксель. Тайл 64×64 такая полоса не
// накрывает целиком никогда, и раньше markTiles на КАЖДЫЙ такой вызов заново
// строил Rect тайла и проверял full — притом что ответ («нет, не full») был
// один и тот же 300 раз подряд. BenchmarkMarkTilesRowByRow меряет эту цену,
// BenchmarkMarkTilesOneCall — тот же результат по числу вызовов markTiles за
// один проход (не по классификации: одна сплошная заливка и 300 построчных
// разноцветных дают РАЗНЫЙ, а не одинаковый признак тайла — см.
// TestMarkTiles_StripedFillStaysMixedEvenSameColor ниже и объяснение там же).
package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/output"
)

const (
	benchRegionW = 400
	benchRegionH = 300
)

// benchRegionsCanvas — канвас под замер без масштаба: логические и
// физические координаты совпадают, арифметика теста прозрачна.
func benchRegionsCanvas(b *testing.B) *Canvas {
	b.Helper()
	eng := New(benchRegionW, benchRegionH, 30)
	return eng.canvas
}

// BenchmarkMarkTilesRowByRow — 300 вызовов markTiles высотой в 1 физический
// пиксель, цвет меняется от строки к строке (как у настоящего линейного
// градиента — соседние строки почти никогда не совпадают цветом).
func BenchmarkMarkTilesRowByRow(b *testing.B) {
	c := benchRegionsCanvas(b)
	full := image.Rect(0, 0, benchRegionW, benchRegionH)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		c.resetTileMarks(full)
		b.StartTimer()
		for y := 0; y < benchRegionH; y++ {
			col := color.RGBA{R: uint8(y), G: uint8(255 - y), B: 80, A: 255}
			c.markSolid(image.Rect(0, y, benchRegionW, y+1), col, true)
		}
	}
}

// BenchmarkMarkTilesOneCall — та же площадь ОДНИМ вызовом markTiles: цель, к
// которой должна приблизиться цена BenchmarkMarkTilesRowByRow по числу
// вызовов и обходов диапазона тайлов за кадр (не по итоговому Kind — см.
// package-комментарий выше).
func BenchmarkMarkTilesOneCall(b *testing.B) {
	c := benchRegionsCanvas(b)
	full := image.Rect(0, 0, benchRegionW, benchRegionH)
	col := color.RGBA{R: 200, G: 90, B: 40, A: 255}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		c.resetTileMarks(full)
		b.StartTimer()
		c.markSolid(full, col, true)
	}
}

// ─── Корректность ───────────────────────────────────────────────────────────
//
// canBeFull в markTiles — быстрый путь ВНУТРИ одного вызова: он не переносит
// запись между вызовами (накопитель, переживающий несколько markSolid,
// потребовал бы поля на Canvas в canvas.go, который здесь трогать нельзя, а
// заодно рисковал бы оставить canvas.marks устаревшим для кода, что читает
// его напрямую, минуя regionsFor, — так делает часть тестов в
// regions_test.go). Каждый вызов markTiles по-прежнему применяется целиком и
// сразу; canBeFull лишь решает, нужно ли на ЭТОТ вызов вообще считать Rect
// тайла и full, зная заранее, что full нигде не достижим. Тесты ниже это и
// проверяют: результат обязан совпадать с копией алгоритма ДО оптимизации
// (referenceMarkTiles) на тех же последовательностях вызовов.

// referenceMarkTiles — копия markTiles ДО появления canBeFull: full считается
// для КАЖДОГО тайла честно, без быстрого пути. Эталон для дифференциального
// теста: если оптимизированный markTiles даёт другой результат — оптимизация
// сломала классификацию, а не только её цену.
func referenceMarkTiles(marks []tileMark, tilesX, tilesY, w, h int, r image.Rectangle, kind output.RegionKind, col color.RGBA, opaque bool) {
	if len(marks) == 0 || tilesX == 0 {
		return
	}
	r = r.Intersect(image.Rect(0, 0, w, h))
	if r.Empty() {
		return
	}
	ts := output.TileSize
	tx0, ty0 := r.Min.X/ts, r.Min.Y/ts
	tx1, ty1 := (r.Max.X-1)/ts, (r.Max.Y-1)/ts
	if tx1 >= tilesX {
		tx1 = tilesX - 1
	}
	if ty1 >= tilesY {
		ty1 = tilesY - 1
	}

	for ty := ty0; ty <= ty1; ty++ {
		row := ty * tilesX
		for tx := tx0; tx <= tx1; tx++ {
			m := &marks[row+tx]

			tile := image.Rect(tx*ts, ty*ts, min(tx*ts+ts, w), min(ty*ts+ts, h))
			full := r.Min.X <= tile.Min.X && r.Min.Y <= tile.Min.Y &&
				r.Max.X >= tile.Max.X && r.Max.Y >= tile.Max.Y

			if full && opaque {
				m.touched, m.kind, m.color = true, kind, col
				continue
			}
			if !m.touched {
				m.touched = true
				if kind == output.RegionSolid {
					m.kind = output.RegionMixed
				} else {
					m.kind, m.color = kind, col
				}
				continue
			}
			switch {
			case m.kind == kind:
				if kind == output.RegionSolid && m.color != col {
					m.kind = output.RegionMixed
				}
			case m.kind == output.RegionSolid:
				m.kind, m.color = kind, col
			case kind == output.RegionSolid:
				m.kind = output.RegionMixed
			default:
				m.kind = output.RegionMixed
			}
		}
	}
}

// regionsMarkCall — один вызов markTiles для дифференциального прогона.
type regionsMarkCall struct {
	r      image.Rectangle
	kind   output.RegionKind
	col    color.RGBA
	opaque bool
}

// runAgainstReference прогоняет calls через боевой c.markTiles и через
// referenceMarkTiles (на отдельной копии marks того же размера) и сверяет
// результат тайл в тайл. touched и kind обязаны совпасть всегда; color —
// только когда kind == RegionSolid (Color осмыслен лишь тогда, см.
// output.Region).
func runAgainstReference(t *testing.T, c *Canvas, calls []regionsMarkCall) {
	t.Helper()
	ref := make([]tileMark, len(c.marks))
	for _, cl := range calls {
		c.markTiles(cl.r, cl.kind, cl.col, cl.opaque)
		referenceMarkTiles(ref, c.tilesX, c.tilesY, c.W, c.H, cl.r, cl.kind, cl.col, cl.opaque)
	}
	for i := range ref {
		got, want := c.marks[i], ref[i]
		if got.touched != want.touched || got.kind != want.kind {
			t.Fatalf("тайл %d: получили touched=%v kind=%v, эталон touched=%v kind=%v",
				i, got.touched, got.kind, want.touched, want.kind)
		}
		if want.kind == output.RegionSolid && got.color != want.color {
			t.Errorf("тайл %d: цвет %v, эталон %v (Color осмыслен только при Solid)", i, got.color, want.color)
		}
	}
}

// TestMarkTiles_GradientRowsMatchReference — построчная заливка с меняющимся
// от строки к строке цветом (как у настоящего линейного градиента) даёт тот
// же признак по тайлам, что и не оптимизированный алгоритм. Размер холста
// НЕ кратен TileSize (200×150 при TileSize=64) специально: задевает крайние
// тайлы по обеим осям, для которых canBeFull считается отдельно.
func TestMarkTiles_GradientRowsMatchReference(t *testing.T) {
	const w, h = 200, 150
	eng := New(w, h, 30)
	c := eng.canvas
	c.resetTileMarks(image.Rect(0, 0, w, h))

	var calls []regionsMarkCall
	for y := 0; y < h; y++ {
		calls = append(calls, regionsMarkCall{
			r:      image.Rect(0, y, w, y+1),
			kind:   output.RegionSolid,
			col:    color.RGBA{R: uint8(y * 3), G: uint8(255 - y*2), B: 90, A: 255},
			opaque: true,
		})
	}
	runAgainstReference(t, c, calls)
}

// TestMarkTiles_MixedCallSequenceMatchesReference — прогон, где вперемешку
// встречаются: построчная заливка ОДНИМ цветом (проверяет, что "тот же цвет
// второй раз" не путается с "накрыл целиком"), разрыв (несмежный
// прямоугольник — блит картинки в стороне) и одна большая заливка, которая
// ДОЛЖНА пройти через полный (не ускоренный) путь, потому что она способна
// накрыть тайл целиком. Смешение кейсов проверяет ту же вещь, что для
// настоящего накопителя формулировалась бы как «не склеивает несмежное или
// разного вида»: здесь вместо склейки — прямое применение, и дифференциальный
// тест ловит любое расхождение с исходным алгоритмом на любом из кейсов.
func TestMarkTiles_MixedCallSequenceMatchesReference(t *testing.T) {
	const w, h = 200, 150
	eng := New(w, h, 30)
	c := eng.canvas
	c.resetTileMarks(image.Rect(0, 0, w, h))

	var calls []regionsMarkCall

	// Строки одного цвета — тайл не обязан стать Solid только оттого, что
	// цвет не менялся: первое частичное касание уже решило — Mixed.
	for y := 40; y < 70; y++ {
		calls = append(calls, regionsMarkCall{
			r: image.Rect(0, y, w, y+1), kind: output.RegionSolid,
			col: color.RGBA{R: 10, G: 20, B: 30, A: 255}, opaque: true,
		})
	}
	// Разрыв и смена вида — блит картинки в стороне от заливки.
	calls = append(calls, regionsMarkCall{
		r: image.Rect(20, 20, 60, 60), kind: output.RegionImage, opaque: true,
	})
	// Большая заливка — единственный случай в этом наборе, где full ДОЛЖЕН
	// сработать (canBeFull=true): проверяет, что быстрый путь не выключил
	// медленный там, где он необходим.
	calls = append(calls, regionsMarkCall{
		r: image.Rect(0, 0, w, h), kind: output.RegionSolid,
		col: color.RGBA{R: 5, G: 5, B: 5, A: 255}, opaque: true,
	})

	runAgainstReference(t, c, calls)
}

// TestMarkTiles_NonSolidStripsMatchOneShot — для НЕ-заливки (картинка, текст)
// построчная и одноразовая пометка одной и той же области ДЕЙСТВИТЕЛЬНО дают
// одинаковый Kind: у Solid частичное касание — это «не знаю, что рядом» (см.
// markTiles), а у картинки/текста — нет, там частичное касание прямо
// объявляет вид (глиф или блит и так не претендуют закрыть тайл целиком,
// правило «слабый признак» — только у заливки). Одна полоса построчных
// вызовов kind=Image без разрывов должна дать тот же Kind на каждом тайле,
// что и один вызов на всю область.
func TestMarkTiles_NonSolidStripsMatchOneShot(t *testing.T) {
	const w, h = 192, 192

	rowByRow := New(w, h, 30).canvas
	rowByRow.resetTileMarks(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		rowByRow.markTiles(image.Rect(0, y, w, y+1), output.RegionImage, color.RGBA{}, true)
	}

	oneShot := New(w, h, 30).canvas
	oneShot.resetTileMarks(image.Rect(0, 0, w, h))
	oneShot.markTiles(image.Rect(0, 0, w, h), output.RegionImage, color.RGBA{}, true)

	if len(rowByRow.marks) != len(oneShot.marks) {
		t.Fatalf("разное число тайлов: %d и %d", len(rowByRow.marks), len(oneShot.marks))
	}
	for i := range oneShot.marks {
		got, want := rowByRow.marks[i], oneShot.marks[i]
		if got.touched != want.touched || got.kind != want.kind {
			t.Errorf("тайл %d: построчно touched=%v kind=%v, одним вызовом touched=%v kind=%v",
				i, got.touched, got.kind, want.touched, want.kind)
		}
	}
}

// TestMarkTiles_StripedFillStaysMixedEvenSameColor — заливка НЕ становится
// Solid только оттого, что её удалось бы представить как одну сплошную
// область: 300 построчных вызовов ОДНИМ и тем же цветом по-прежнему дают
// Mixed, а не Solid, хотя один вызов той же площади и цвета дал бы Solid.
// Так было и до оптимизации (см. правило «частичная заливка — не знаю, что
// рядом» в markTiles) — фиксируем это явно, чтобы будущая правка случайно не
// начала СКЛЕИВАТЬ такие строки в один вызов: это была бы уже не оптимизация
// цены, а другой результат классификации.
func TestMarkTiles_StripedFillStaysMixedEvenSameColor(t *testing.T) {
	const w, h = 128, 128
	col := color.RGBA{R: 90, G: 90, B: 200, A: 255}

	striped := New(w, h, 30).canvas
	striped.resetTileMarks(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		striped.markTiles(image.Rect(0, y, w, y+1), output.RegionSolid, col, true)
	}

	solid := New(w, h, 30).canvas
	solid.resetTileMarks(image.Rect(0, 0, w, h))
	solid.markTiles(image.Rect(0, 0, w, h), output.RegionSolid, col, true)

	for i := range solid.marks {
		if striped.marks[i].kind != output.RegionMixed {
			t.Errorf("тайл %d: построчная заливка одним цветом дала %v, ждали Mixed",
				i, striped.marks[i].kind)
		}
		if solid.marks[i].kind != output.RegionSolid {
			t.Errorf("тайл %d: одна заливка на всю область дала %v, ждали Solid",
				i, solid.marks[i].kind)
		}
	}
}

// Обрезанный крайний тайл: полоса, равная его высоте, накрывает его целиком.
//
// Ускорение markTiles стоит на том, что полоса тоньше тайла не накроет ни
// один тайл диапазона — и проверять нечего. Но крайний ряд тайлов обрезан по
// краю холста, если высота холста не кратна 64: там тайл может быть в одну
// точку, и та самая «тонкая» полоса накрывает его ПОЛНОСТЬЮ. Без поправки
// на обрезку сплошная заливка нижней строки экрана уехала бы к потребителю
// как mixed — то есть картинкой вместо цвета, самый дорогой вид тайла.
func TestMarkTiles_ThinStripFillsATruncatedEdgeTile(t *testing.T) {
	const ts = output.TileSize
	// Высота с остатком в одну точку: последний ряд тайлов — высотой 1.
	eng := New(2*ts, 2*ts+1, 30)
	c := eng.canvas

	col := color.RGBA{R: 10, G: 200, B: 90, A: 255}
	strip := image.Rect(0, 2*ts, 2*ts, 2*ts+1)
	c.markTiles(strip, output.RegionSolid, col, true)

	lastRow := (c.tilesY - 1) * c.tilesX
	for tx := 0; tx < c.tilesX; tx++ {
		m := c.marks[lastRow+tx]
		if m.kind != output.RegionSolid {
			t.Errorf("тайл %d нижнего ряда объявлен %v, а полоса накрыла его целиком",
				tx, m.kind)
		}
		if m.color != col {
			t.Errorf("тайл %d нижнего ряда: цвет %v вместо %v", tx, m.color, col)
		}
	}

	// А та же полоса выше по холсту тайл не накрывает — там он полной высоты.
	c2 := New(2*ts, 2*ts+1, 30).canvas
	c2.markTiles(image.Rect(0, 0, 2*ts, 1), output.RegionSolid, col, true)
	if got := c2.marks[0].kind; got == output.RegionSolid {
		t.Errorf("полоса в одну точку объявила тайл 64x64 сплошным (%v)", got)
	}
}
