package widget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
)

// Оптические размеры: у значка свой файл на 16, 20 и 24, и по запрошенному
// размеру набор берёт подходящий, а не растягивает один.
func TestIconSet_OpticalSizesPickFileByRequestedSize(t *testing.T) {
	dir := t.TempDir()
	for size, col := range map[string]string{"16": "#ff0000", "20": "#00ff00", "24": "#0000ff"} {
		svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" fill="` + col + `"/></svg>`
		if err := os.WriteFile(filepath.Join(dir, "wifi_"+size+".svg"), []byte(svg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := NewIconSet(dir)
	ref := theme.IconRef{Source: "wifi_{size}.svg", Sizes: theme.IconSizes(16, 20, 24)}
	dominant := func(size int) string {
		img := s.ResolveIcon(ref, size)
		r, g, b, _ := img.At(size/2, size/2).RGBA()
		switch {
		case r > 0x8000:
			return "16"
		case g > 0x8000:
			return "20"
		case b > 0x8000:
			return "24"
		}
		return "?"
	}
	for size, want := range map[int]string{12: "16", 16: "16", 17: "20", 20: "20", 21: "24", 24: "24", 36: "24"} {
		if got := dominant(size); got != want {
			t.Errorf("запрошено %d: взят файл %s, ждали %s", size, got, want)
		}
	}
	// Без Sizes путь не трогается: один файл на любой размер.
	if got := (theme.IconRef{Source: "a_{size}.svg"}).SourceFor(16); got != "a_{size}.svg" {
		t.Errorf("без Sizes путь изменён: %q", got)
	}
}
