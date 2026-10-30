package native

// sortmonitors.go — устойчивый порядок списка мониторов.

import "sort"

// SortMonitors упорядочивает мониторы: основной первым, остальные слева
// направо, затем сверху вниз. Порядок, в котором их перечисляет система, ничем
// не гарантирован, а приложению нужен предсказуемый («второй монитор»).
//
//facadegen:skip
func SortMonitors(list []Monitor) []Monitor {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.Primary != b.Primary {
			return a.Primary
		}
		if a.Bounds.Min.X != b.Bounds.Min.X {
			return a.Bounds.Min.X < b.Bounds.Min.X
		}
		return a.Bounds.Min.Y < b.Bounds.Min.Y
	})
	return list
}
