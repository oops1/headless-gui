//go:build linux && !android

package x11

import (
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// TestPropertyTooBig проверяет отсечку неправдоподобной длины свойства XDND.
func TestPropertyTooBig(t *testing.T) {
	cases := []struct {
		words int
		want  bool
	}{
		{0, false},
		{1, false},
		{native.MaxDnDBytes / 4, false},
		{native.MaxDnDBytes/4 + 1, true},
		{1 << 28, true}, // 0x1FFFFFFF-класс: сотни МБ
		{-1, true},
	}
	for _, c := range cases {
		if got := propertyTooBig(c.words); got != c.want {
			t.Errorf("propertyTooBig(%d) = %v, ожидалось %v", c.words, got, c.want)
		}
	}
}
