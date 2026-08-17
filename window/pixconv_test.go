package window

import (
	"bytes"
	"testing"
)

// TestSwapRBRow — эталон: побайтовая перестановка R↔B; проверяем разные
// длины (в т.ч. некратные 4 — хвост не трогается) и dst короче src.
func TestSwapRBRow(t *testing.T) {
	for _, n := range []int{0, 4, 8, 12, 13, 15, 64, 4096} {
		src := make([]byte, n)
		for i := range src {
			src[i] = byte(i*7 + 3)
		}
		want := make([]byte, n)
		for i := 0; i+4 <= n; i += 4 {
			want[i+0] = src[i+2]
			want[i+1] = src[i+1]
			want[i+2] = src[i+0]
			want[i+3] = src[i+3]
		}
		got := make([]byte, n)
		swapRBRow(got, src)
		if !bytes.Equal(got, want) {
			t.Fatalf("n=%d: got %v want %v", n, got, want)
		}
	}
	// dst короче src — конвертируется только общая часть.
	src := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	dst := make([]byte, 4)
	swapRBRow(dst, src)
	if !bytes.Equal(dst, []byte{3, 2, 1, 4}) {
		t.Fatalf("короткий dst: %v", dst)
	}
}
