//go:build goexperiment.simd && amd64 && go1.27

package pixsimd

import (
	"strings"
	"testing"

	"simd/archsimd"
)

// Выбор набора при запуске: процессор без AVX2 и рубильник оставляют
// скалярный путь и называют причину, исправный процессор получает векторный.
func TestSelectVector(t *testing.T) {
	prevActive, prevFallback := active, fallback
	defer func() { active, fallback = prevActive, prevFallback }()

	for _, tc := range []struct {
		name            string
		hasAVX2, off    bool
		impl, fallbackS string
	}{
		{"без AVX2", false, false, "generic", "без AVX2"},
		{"без AVX2 и рубильник", false, true, "generic", "без AVX2"},
		{"рубильник", true, true, "generic", "HEADLESS_GUI_NOSIMD"},
		{"AVX2", true, false, "avx2", ""},
	} {
		// Векторный набор на процессоре без AVX2 упал бы при самопроверке:
		// последний случай — только там, где AVX2 действительно есть.
		if tc.hasAVX2 && !tc.off && !archsimd.X86.AVX2() {
			continue
		}
		active, fallback = generic, ""
		selectVector(tc.hasAVX2, tc.off)
		if active.name != tc.impl {
			t.Errorf("%s: выбран %s, ожидался %s", tc.name, active.name, tc.impl)
		}
		if tc.fallbackS == "" && fallback != "" || !strings.Contains(fallback, tc.fallbackS) {
			t.Errorf("%s: причина %q, ожидалось упоминание %q", tc.name, fallback, tc.fallbackS)
		}
	}
}
