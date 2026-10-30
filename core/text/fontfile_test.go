package text

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Файл шрифта читается одним буфером точного размера: растущая читалка
// выделяла в несколько раз больше (53 МБ на 22 МБ шрифтов).
func TestReadFileBounded_AllocatesFileSizeOnce(t *testing.T) {
	const size = 6 << 20
	p := filepath.Join(t.TempDir(), "big.bin")
	if err := os.WriteFile(p, bytes.Repeat([]byte{7}, size), 0o644); err != nil {
		t.Fatal(err)
	}
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	data, err := readFileBounded(p, MaxFontFileSize)
	runtime.ReadMemStats(&m1)
	if err != nil || len(data) != size {
		t.Fatalf("прочитано %d байт, err=%v", len(data), err)
	}
	if got := m1.TotalAlloc - m0.TotalAlloc; got > size+size/4 {
		t.Errorf("выделено %d байт на файл в %d (ожидался один буфер)", got, size)
	}
	// Предел по-прежнему действует.
	if _, err := readFileBounded(p, size-1); err == nil {
		t.Error("файл больше предела прочитан")
	}
}
