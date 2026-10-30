package engine

// damagemerge.go — пересекающиеся области кадра сливаются до диффа.
//
// Дифф идёт по каждой заявленной области отдельно (diffAndSyncInRects), и
// тайл, попавший в две области, сравнивался дважды: второй раз — с уже
// синхронизированным front, впустую, но за полную цену memcmp. Окно,
// переезжающее вместе с детьми, заявляет одно и то же место по разу на
// каждого ребёнка (Base.SetBounds — объединение старых и новых границ), да
// ещё тень за его границами: на перетаскивании окна программы в сцене
// WinLine это было до восьми областей поверх одного места, и сравнение
// кадра стоило в разы больше, чем само рисование.

import "image"

// mergeOverlaps возвращает области, в которых пересекающиеся заменены
// объемлющими; непересекающиеся остаются как есть, порядок не важен.
// Исходный срез не меняется.
func mergeOverlaps(rs []image.Rectangle) []image.Rectangle {
	if len(rs) < 2 {
		return rs
	}
	out := make([]image.Rectangle, 0, len(rs))
	for _, r := range rs {
		if r.Empty() {
			continue
		}
		for merged := true; merged; {
			merged = false
			for i, q := range out {
				if q.Overlaps(r) {
					r = r.Union(q)
					out = append(out[:i], out[i+1:]...)
					merged = true
					break
				}
			}
		}
		out = append(out, r)
	}
	return out
}
