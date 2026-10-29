package native

import (
	"testing"
)

// Метка времени из стартового идентификатора (DESKTOP_STARTUP_ID).
func TestStartupIDTime(t *testing.T) {
	for _, tc := range []struct {
		id string
		t  uint32
		ok bool
	}{
		{"sh-1234-host_TIME123456", 123456, true},
		{"x_TIME1", 1, true},
		{"a_TIME99_TIME4294967295", 4294967295, true}, // берётся последняя
		{"abc_TIME77 extra", 77, true},                // хвост после числа не мешает
		{"", 0, false},
		{"no-time-here", 0, false},
		{"id_TIME", 0, false},           // нет числа
		{"id_TIMEabc", 0, false},        // не число
		{"id_TIME0", 0, false},          // 0 — CurrentTime, не метка
		{"id_TIME4294967296", 0, false}, // не помещается в 32 бита
		{"id_TIME12345678901", 0, false},
	} {
		got, ok := StartupIDTime(tc.id)
		if got != tc.t || ok != tc.ok {
			t.Errorf("StartupIDTime(%q) = %d, %v; ждал %d, %v", tc.id, got, ok, tc.t, tc.ok)
		}
	}
}
