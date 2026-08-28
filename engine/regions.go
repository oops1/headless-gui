// regions.go — накопление признака содержимого по тайлам.
//
// Растеризатор знает, чем он залил каждый участок: сплошным цветом, блитом
// изображения, глифами. Здесь это знание запоминается по тайлам и уходит
// наружу вместе с кадром — потребителю больше не нужно восстанавливать его
// вторым проходом кодека.
//
// Стоимость — одно присваивание в уже существующих циклах отрисовки.
package engine

import (
	"image"
	"image/color"

	"github.com/oops1/headless-gui/v3/output"
)

// tileMark — признак одного тайла за текущий кадр.
type tileMark struct {
	kind  output.RegionKind
	color color.RGBA
	// touched — тайл в этом кадре трогали. Нетронутый тайл не попадает в
	// выдачу вовсе: его содержимое не менялось, и говорить о нём нечего.
	touched bool
}

// initTileMarks заводит таблицу признаков под размер холста.
func (c *Canvas) initTileMarks() {
	n := c.tilesX * c.tilesY
	if n <= 0 {
		c.marks = nil
		return
	}
	if cap(c.marks) >= n {
		c.marks = c.marks[:n]
	} else {
		c.marks = make([]tileMark, n)
	}
	c.resetTileMarks(image.Rect(0, 0, c.W, c.H))
}

// resetTileMarks сбрасывает признаки тайлов, пересекающих r.
//
// В начале кадра сбрасываются тайлы внутри damage: за их содержимое отвечает
// этот кадр, а прошлые сведения к нему отношения не имеют. Тайлы вне damage
// сохраняют признак с того кадра, когда их рисовали.
func (c *Canvas) resetTileMarks(r image.Rectangle) {
	if len(c.marks) == 0 {
		return
	}
	tx0, ty0, tx1, ty1, ok := c.tileRange(r)
	if !ok {
		return
	}
	for ty := ty0; ty <= ty1; ty++ {
		row := ty * c.tilesX
		for tx := tx0; tx <= tx1; tx++ {
			c.marks[row+tx] = tileMark{}
		}
	}
}

// tileRange переводит прямоугольник ФИЗИЧЕСКИХ пикселей в диапазон тайлов.
func (c *Canvas) tileRange(r image.Rectangle) (tx0, ty0, tx1, ty1 int, ok bool) {
	r = r.Intersect(image.Rect(0, 0, c.W, c.H))
	if r.Empty() || c.tilesX == 0 {
		return 0, 0, 0, 0, false
	}
	ts := output.TileSize
	tx0, ty0 = r.Min.X/ts, r.Min.Y/ts
	tx1, ty1 = (r.Max.X-1)/ts, (r.Max.Y-1)/ts
	if tx1 >= c.tilesX {
		tx1 = c.tilesX - 1
	}
	if ty1 >= c.tilesY {
		ty1 = c.tilesY - 1
	}
	return tx0, ty0, tx1, ty1, true
}

// markSolid помечает область сплошной заливкой цвета col.
//
// Тайл, накрытый заливкой ЦЕЛИКОМ, становится сплошным: о нём теперь известно
// всё. Задетый частично — смешанным: рядом с заливкой в нём осталось что-то
// ещё, и обещать потребителю один цвет нельзя.
func (c *Canvas) markSolid(r image.Rectangle, col color.RGBA, opaque bool) {
	c.markTiles(r, output.RegionSolid, col, opaque)
}

// markImage помечает область блитом изображения. Картинка считается
// перекрывающей: даже с альфой это «здесь картинка», а не «неизвестно что».
func (c *Canvas) markImage(r image.Rectangle) {
	c.markTiles(r, output.RegionImage, color.RGBA{}, true)
}

// markText помечает область глифами.
func (c *Canvas) markText(r image.Rectangle) {
	// Глифы кладутся поверх фона и тайл целиком не закрывают.
	c.markTiles(r, output.RegionText, color.RGBA{}, false)
}

// markKind помечает область тем, чем её рисует текущий вызывающий.
//
// Маски альфы и цветные глифы — общий путь для трёх разных вещей: букв,
// сглаженных дуг скруглённых заливок и размытого силуэта тени. Раньше все
// три уходили наружу как «текст», и потребитель применял к серому размытию
// текстовый кодек. Теперь вид объявляет вызывающий, а по умолчанию —
// честное «не знаю».
func (c *Canvas) markKind(r image.Rectangle) {
	kind := c.maskKind
	if kind == output.RegionText {
		c.markText(r)
		return
	}
	c.markTiles(r, kind, color.RGBA{}, false)
}

// withMaskKind выполняет f, объявляя, чем рисуют маски внутри неё.
//
// Возврат к прежнему значению, а не к «не знаю»: вызовы вкладываются —
// текстовый прогон внутри фигуры остаётся текстом только внутри себя.
func (c *Canvas) withMaskKind(kind output.RegionKind, f func()) {
	prev := c.maskKind
	c.maskKind = kind
	f()
	c.maskKind = prev
}

// markTiles — накопление признака: новый примитив поверх того, что уже было.
//
// Правила выведены из физики отрисовки, а не из порядка вызовов:
//
//   - полная НЕПРОЗРАЧНАЯ заливка стирает всё, что было под ней, и делает
//     тайл сплошным — сколько бы всего там ни рисовали раньше;
//   - заливка — слабый признак: это фон. Текст или картинка поверх фона дают
//     текст или картинку, а не «смешано»: потребителю важно именно то, что
//     сверху, а что под текстом лежал фон, он и так знает;
//   - текст поверх картинки (и наоборот) — честное «не знаю»;
//   - частичная заливка на чистом тайле — тоже «не знаю»: рядом осталось
//     то, что лежало раньше.
func (c *Canvas) markTiles(r image.Rectangle, kind output.RegionKind, col color.RGBA, opaque bool) {
	if len(c.marks) == 0 {
		return
	}
	r = r.Intersect(image.Rect(0, 0, c.W, c.H))
	if r.Empty() {
		return
	}
	tx0, ty0, tx1, ty1, ok := c.tileRange(r)
	if !ok {
		return
	}
	ts := output.TileSize

	for ty := ty0; ty <= ty1; ty++ {
		row := ty * c.tilesX
		for tx := tx0; tx <= tx1; tx++ {
			m := &c.marks[row+tx]

			tile := image.Rect(tx*ts, ty*ts, min(tx*ts+ts, c.W), min(ty*ts+ts, c.H))
			full := r.Min.X <= tile.Min.X && r.Min.Y <= tile.Min.Y &&
				r.Max.X >= tile.Max.X && r.Max.Y >= tile.Max.Y

			// Скруглённый клип режет углы: примитив накрывает прямоугольник
			// тайла, но пиксели за дугой остаются прежними. Обещать
			// потребителю сплошной цвет тут нельзя — он зальёт квадратом то,
			// что на деле скруглено.
			if full && c.round.active && c.round.clipsTile(tile) {
				full = false
			}

			// Перекрытие: примитив накрыл тайл целиком и ничего не пропускает.
			if full && opaque {
				m.touched, m.kind, m.color = true, kind, col
				continue
			}
			if !m.touched {
				m.touched = true
				if kind == output.RegionSolid {
					// Заливка, не накрывшая тайл целиком: часть площади —
					// прежнее содержимое.
					m.kind = output.RegionMixed
				} else {
					m.kind, m.color = kind, col
				}
				continue
			}

			switch {
			case m.kind == kind:
				// Тот же вид второй раз: для заливки важен цвет, для
				// остального вид не меняется.
				if kind == output.RegionSolid && m.color != col {
					m.kind = output.RegionMixed
				}
			case m.kind == output.RegionSolid:
				// Фон был, поверх легло содержимое — оно и есть признак.
				m.kind, m.color = kind, col
			case kind == output.RegionSolid:
				// Заливка легла поверх картинки или текста, не закрыв тайл
				// целиком: часть площади — содержимое, часть — заливка.
				// Обещать потребителю что-то одно нельзя.
				m.kind = output.RegionMixed
			default:
				m.kind = output.RegionMixed
			}
		}
	}
}

// regionsFor собирает признаки тайлов, попавших в кадр.
//
// Порядок совпадает с порядком тайлов: потребителю удобно идти по обоим
// спискам разом.
func (c *Canvas) regionsFor(tiles []output.DirtyTile) []output.Region {
	if len(c.marks) == 0 || len(tiles) == 0 {
		return nil
	}
	ts := output.TileSize
	out := make([]output.Region, 0, len(tiles))
	for _, t := range tiles {
		tx, ty := t.X/ts, t.Y/ts
		if tx >= c.tilesX || ty >= c.tilesY {
			continue
		}
		m := c.marks[ty*c.tilesX+tx]
		if !m.touched {
			// Тайл изменился, но растеризатор его не помечал — значит
			// рисовало что-то, о чём мы признака не ведём. Честное «не знаю».
			out = append(out, output.Region{
				Rect: image.Rect(t.X, t.Y, t.X+t.W, t.Y+t.H),
				Kind: output.RegionMixed,
			})
			continue
		}
		out = append(out, output.Region{
			Rect:  image.Rect(t.X, t.Y, t.X+t.W, t.Y+t.H),
			Kind:  m.kind,
			Color: m.color,
		})
	}
	return out
}
