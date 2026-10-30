package base

import (
	"testing"
	"testing/fstest"

	"github.com/oops1/headless-gui/v4/theme"
	svgPkg "github.com/oops1/headless-gui/v4/widget/svg"
)

func iconFSOf(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for name, col := range files {
		m[name] = &fstest.MapFile{Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" fill="` + col + `"/></svg>`)}
	}
	return m
}

// Набор из fs.FS (go:embed): оптические размеры выбирают файл внутри FS так же,
// как набор с диска, а подкаталог dir — корень путей Source.
func TestIconSetFS_OpticalSizesFromFS(t *testing.T) {
	fsys := iconFSOf(map[string]string{
		"icons/wifi_16.svg": "#ff0000", "icons/wifi_20.svg": "#00ff00", "icons/wifi_24.svg": "#0000ff",
	})
	s := NewIconSetFS(fsys, "icons")
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
}

// Корень FS без dir, обратная косая в пути (разметку пишут на Windows) и
// повторный запрос из кэша.
func TestIconSetFS_RootAndBackslash(t *testing.T) {
	s := NewIconSetFS(iconFSOf(map[string]string{"a/b.svg": "#ff0000"}), "")
	for _, src := range []string{"a/b.svg", `a\b.svg`, "./a/b.svg"} {
		img := s.ResolveIcon(theme.IconRef{Source: src}, 16)
		if r, _, _, _ := img.At(8, 8).RGBA(); r < 0x8000 {
			t.Errorf("Source %q: иконка не из файла (заглушка?)", src)
		}
	}
	a := s.ResolveIcon(theme.IconRef{Source: "a/b.svg"}, 16)
	if b := s.ResolveIcon(theme.IconRef{Source: "a/b.svg"}, 16); a != b {
		t.Error("повторный запрос не из кэша")
	}
}

// Граница та же, что у набора с диска: выход из dir и абсолютный путь дают
// заглушку, а не файл из соседнего каталога FS.
func TestIconSetFS_RejectsEscape(t *testing.T) {
	fsys := iconFSOf(map[string]string{"icons/ok.svg": "#ff0000", "secret.svg": "#ff0000"})
	s := NewIconSetFS(fsys, "icons")
	ph := s.placeholder(16)
	for _, src := range []string{"../secret.svg", "/secret.svg", "../icons/ok.svg", "missing.svg"} {
		if got := s.ResolveIcon(theme.IconRef{Source: src}, 16); got != ph {
			t.Errorf("Source %q: ждали заглушку", src)
		}
	}
	if got := s.ResolveIcon(theme.IconRef{Source: "ok.svg"}, 16); got == ph {
		t.Error("ok.svg внутри dir не найден")
	}
}

func TestParseFS(t *testing.T) {
	fsys := iconFSOf(map[string]string{"x.svg": "#00f"})
	if _, err := svgPkg.ParseFS(fsys, "x.svg"); err != nil {
		t.Fatal(err)
	}
	if _, err := svgPkg.ParseFS(fsys, "nope.svg"); err == nil {
		t.Error("несуществующий файл принят")
	}
}
