package mergeview

import (
	"slices"
	"testing"
)

// Render с длиной маркера: все четыре маркера нужной длины, подписи на месте;
// Result — та же сборка с семью знаками.
func TestRenderMarkerSize(t *testing.T) {
	cs := Merge(lines("a\nb\nc"), lines("a\nНАШЕ\nc"), lines("a\nИХ\nc"), false)

	got := Render(cs, nil, Format{Style: StyleDiff3, Labels: Labels{Ours: "main"}, MarkerSize: 9})
	want := lines("a\n<<<<<<<<< main\nНАШЕ\n||||||||| base\nb\n=========\nИХ\n>>>>>>>>> merge head\nc")
	if !slices.Equal(got, want) {
		t.Fatalf("Render с длиной 9:\n%q\nхочу:\n%q", got, want)
	}

	for _, size := range []int{0, -3, 7} {
		got := Render(cs, nil, Format{MarkerSize: size})
		if !slices.Equal(got, Result(cs, nil, StyleMerge, Labels{})) {
			t.Fatalf("длина %d не дала семь знаков: %q", size, got)
		}
	}
}
