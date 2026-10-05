// startmenu_tiles_groups.go — куда ляжет плитка, которую несут мышью: место в
// существующей группе или новая группа.
//
// Как в Windows 10, плитку можно отпустить не только на другую плитку, но и в
// пустое место:
//
//   - под последней группой — там появляется новая группа;
//   - между двумя группами (в зазоре и в верхней половине заголовка нижней) —
//     новая группа встаёт между ними;
//   - над первой группой (верхняя половина её заголовка и поле выше) — новая
//     группа становится первой.
//
// Нижняя половина заголовка и сам ряд плиток по-прежнему означают «в эту
// группу». У новой группы нет названия (Title пуст): в предпросмотре над ней
// подсказка desktop.start.newGroup, а как её назвать, решает потребитель по
// OnTilesChanged. Группа, из которой унесли последнюю плитку, при отпускании
// удаляется; пока плитку несут, она остаётся пустой с заголовком, чтобы
// раскладка под курсором не прыгала.
package desktop

import (
	"fmt"
	"image"

	"github.com/oops1/headless-gui/v3/widget"
)

// withoutTile возвращает группы без плитки id и сами её данные.
func withoutTile(groups []TileGroup, id TileID) ([]TileGroup, Tile, bool) {
	out := make([]TileGroup, len(groups))
	var found Tile
	ok := false
	for i, g := range groups {
		out[i] = g
		out[i].Tiles = nil
		for _, t := range g.Tiles {
			if t.ID == id {
				found, ok = t, true
				continue
			}
			out[i].Tiles = append(out[i].Tiles, t)
		}
	}
	return out, found, ok
}

// newGroupID подбирает идентификатор новой группы, не занятый другими.
func newGroupID(groups []TileGroup) string {
	used := make(map[string]bool, len(groups))
	for _, g := range groups {
		used[g.ID] = true
	}
	for n := len(groups) + 1; ; n++ {
		if id := fmt.Sprintf("group-%d", n); !used[id] {
			return id
		}
	}
}

// dragApply применяет перенос d к группам. Возвращает получившиеся группы и
// индекс новой группы среди них (-1 — новой группы нет). final — перенос
// завершён: опустевшая группа, из которой унесли плитку, удаляется; в
// предпросмотре (final = false) она остаётся.
func dragApply(groups []TileGroup, d tileDrag, final bool) ([]TileGroup, int) {
	src := -1
	for i, g := range groups {
		for _, t := range g.Tiles {
			if t.ID == d.id {
				src = i
			}
		}
	}
	rest, tile, ok := withoutTile(groups, d.id)
	if !ok || len(rest) == 0 {
		return groups, -1
	}
	emptied := len(rest[src].Tiles) == 0
	toNew, gi, idx := d.toNew, d.toGroup, d.toIndex
	if toNew && emptied && (gi == src || gi == src+1) {
		// Единственную плитку группы «в новую группу» по соседству со старой не
		// переносим: старая группа и есть такая группа, она остаётся на месте.
		toNew, gi, idx = false, src, 0
	}

	newIdx := -1
	if toNew {
		if gi < 0 {
			gi = 0
		}
		if gi > len(rest) {
			gi = len(rest)
		}
		ng := TileGroup{ID: newGroupID(rest), Tiles: []Tile{tile}}
		rest = append(rest, TileGroup{})
		copy(rest[gi+1:], rest[gi:])
		rest[gi] = ng
		newIdx = gi
	} else {
		if gi < 0 {
			gi = 0
		}
		if gi >= len(rest) {
			gi = len(rest) - 1
		}
		tiles := rest[gi].Tiles
		if idx < 0 {
			idx = 0
		}
		if idx > len(tiles) {
			idx = len(tiles)
		}
		next := make([]Tile, 0, len(tiles)+1)
		next = append(next, tiles[:idx]...)
		next = append(next, tile)
		next = append(next, tiles[idx:]...)
		rest[gi].Tiles = next
	}

	if final && emptied {
		si := src
		if newIdx >= 0 && newIdx <= src {
			si++ // новая группа встала перед исходной и сдвинула её
		}
		if si < len(rest) && len(rest[si].Tiles) == 0 {
			rest = append(rest[:si], rest[si+1:]...)
			if newIdx > si {
				newIdx--
			}
		}
	}
	return rest, newIdx
}

// dragPreview возвращает группы с плиткой перетаскивания, вставленной на
// целевое место: по ним рисуется раскладка во время переноса.
func dragPreview(groups []TileGroup, d tileDrag) []TileGroup {
	out, _ := dragApply(groups, d, false)
	return out
}

// dragExtra — запас высоты под последней группой, пока плитку несут: на нём
// плитку отпускают, чтобы создать новую группу, даже если содержимое целиком
// заполняет область. Без переноса — 0.
func (m *StartMenu) dragExtra() int {
	m.v.mu.Lock()
	active := m.v.drag.active
	m.v.mu.Unlock()
	return m.dragExtraFor(active)
}

// dragExtraLocked — то же для вызова под замком меню.
func (m *StartMenu) dragExtraLocked() int { return m.dragExtraFor(m.v.drag.active) }

func (m *StartMenu) dragExtraFor(active bool) int {
	if !active {
		return 0
	}
	return m.metricInt(KeyTileGroupHeader) + m.metricInt(KeyTileGroupHeaderGap) + m.metricInt(KeyTileUnit)/2
}

// dropTarget определяет, куда ляжет перетаскиваемая плитка при отпускании в
// точке pt: либо новая группа (newGroup, group — её место среди групп), либо
// группа по вертикали и место в ней по ближайшей плитке.
func (m *StartMenu) dropTarget(g startGeo, id TileID, pt image.Point) (group, index int, newGroup bool) {
	if g.cols == 0 {
		return 0, 0, false
	}
	v := m.v
	v.mu.Lock()
	groups := v.groups
	scroll := v.tileScroll
	v.mu.Unlock()
	rest, _, ok := withoutTile(groups, id)
	if !ok || len(rest) == 0 {
		return 0, 0, false
	}
	l := computeTileLayout(rest, m.tileKeyFor(g))
	scroll = clampScroll(scroll, l.height+m.dragExtra(), tilesViewHeight(g))
	origin := image.Pt(g.tinner.Min.X, g.tinner.Min.Y-scroll)

	// Зоны новой группы считаются в координатах раскладки.
	y := pt.Y - origin.Y
	half := m.metricInt(KeyTileGroupHeader) / 2
	n := len(l.groups)
	switch {
	case y < l.groups[0].header.Min.Y+half:
		return 0, 0, true
	case y > l.groups[n-1].bottom:
		return n, 0, true
	}
	for i := 1; i < n; i++ {
		if y > l.groups[i-1].bottom && y < l.groups[i].header.Min.Y+half {
			return i, 0, true
		}
	}

	group = 0
	for i, gg := range l.groups {
		if y >= gg.header.Min.Y {
			group = i
		}
	}
	gg := l.groups[group]
	if gg.count == 0 {
		return group, 0, false
	}
	tiles := l.tiles[gg.first : gg.first+gg.count]
	best, bestD := 0, -1
	for i, t := range tiles {
		r := t.rect.Add(origin)
		c := centerOf(r)
		d := absInt(pt.X-c.X) + absInt(pt.Y-c.Y)
		if pt.In(r) {
			d = -1
		}
		if bestD < 0 || d < bestD || d == -1 {
			best, bestD = i, d
		}
		if d == -1 {
			break
		}
	}
	index = best
	if c := centerOf(tiles[best].rect.Add(origin)); pt.X > c.X || pt.Y > tiles[best].rect.Add(origin).Max.Y {
		index = best + 1
	}
	return group, index, false
}

// dropDrag завершает перетаскивание: плитка встаёт на целевое место, потребитель
// получает новый порядок (с новой группой, если плитку отпустили в пустое место).
func (m *StartMenu) dropDrag(d tileDrag) {
	v := m.v
	v.mu.Lock()
	next, _ := dragApply(v.groups, d, true)
	changed := !sameOrder(v.groups, next)
	if changed {
		v.groups = cloneGroups(next)
		v.tilesRev++
	}
	cb := m.OnTilesChanged
	out := cloneGroups(v.groups)
	v.mu.Unlock()
	widget.InvalidateRect(m.areaRect(areaTiles))
	widget.InvalidateRect(m.contentRect())
	if changed && cb != nil {
		cb(out)
	}
}
