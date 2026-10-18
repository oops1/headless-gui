package engine

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
)

// TestEngineNew_ColdReport — замер ПЕРВОГО engine.New процесса (холодный кэш
// файлов шрифтов): время, выделенная память и живая куча после сборки мусора.
// Печатает числа при HG_REPORT_NEW=1; без переменной пропускается (число
// зависит от машины, утверждать нечего).
func TestEngineNew_ColdReport(t *testing.T) {
	testreset.Globals(t)
	if os.Getenv("HG_REPORT_NEW") == "" {
		t.Skip("замер включается переменной HG_REPORT_NEW=1")
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	t0 := time.Now()
	eng := New(1280, 800, 30)
	d := time.Since(t0)
	runtime.ReadMemStats(&after)
	runtime.GC()
	var live runtime.MemStats
	runtime.ReadMemStats(&live)
	t.Logf("первый New: %v, выделено %.1f МБ (%d аллокаций), живая куча +%.1f МБ",
		d, float64(after.TotalAlloc-before.TotalAlloc)/(1<<20), after.Mallocs-before.Mallocs,
		(float64(live.HeapAlloc)-float64(before.HeapAlloc))/(1<<20))
	runtime.KeepAlive(eng)
}
