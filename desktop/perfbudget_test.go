package desktop_test

import (
	"os"
	"strconv"
	"time"
)

// perfBudget — порог для тестов скорости по стенным часам.
//
// Локально порог тот, что записан в тесте. Машины CI (GitHub Actions
// выставляет CI=true) в разы медленнее и делят процессор с соседями: кадр
// «Пуска», который здесь рисуется за 8 мс, там шёл 84 мс, и тест падал без
// всякого дефекта. Такие тесты ловят патологию — кадр в разы дороже, чем
// должен быть, — а не колебания чужого железа, поэтому под CI порог
// умножается (по умолчанию на 4; HG_PERF_SLACK задаёт множитель явно).
func perfBudget(base time.Duration) time.Duration {
	if s := os.Getenv("HG_PERF_SLACK"); s != "" {
		if k, err := strconv.ParseFloat(s, 64); err == nil && k > 0 {
			return time.Duration(float64(base) * k)
		}
	}
	if os.Getenv("CI") != "" {
		return base * 4
	}
	return base
}
