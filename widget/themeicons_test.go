package widget

import (
	"image"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
)

const validSVG = `<svg viewBox="0 0 10 10"><rect width="10" height="10" fill="#000"/></svg>`
const brokenSVG = `<svg><g></svg>`

// imgSize возвращает размеры image.Image как (ширина, высота).
func imgSize(img image.Image) (int, int) {
	b := img.Bounds()
	return b.Dx(), b.Dy()
}

// hasOpaquePixel — среди пикселей изображения есть хотя бы один непрозрачный
// (доказывает, что заглушка/иконка реально что-то нарисовала, а не отдала
// пустой холст).
func hasOpaquePixel(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a != 0 {
				return true
			}
		}
	}
	return false
}

func TestIconSet_RegisteredIconHasRequestedSize(t *testing.T) {
	s := NewIconSet("")
	s.Register("gear", []byte(validSVG))

	img := s.ResolveIcon(theme.IconRef{Name: "gear"}, 32)
	if img == nil {
		t.Fatal("ResolveIcon вернул nil")
	}
	if w, h := imgSize(img); w != 32 || h != 32 {
		t.Fatalf("размер = %dx%d, хочу 32x32", w, h)
	}
	if !hasOpaquePixel(img) {
		t.Fatal("иконка пустая (нет непрозрачных пикселей)")
	}
}

func TestIconSet_RepeatedRequestReturnsSamePointer(t *testing.T) {
	s := NewIconSet("")
	s.Register("gear", []byte(validSVG))

	img1 := s.ResolveIcon(theme.IconRef{Name: "gear"}, 24)
	img2 := s.ResolveIcon(theme.IconRef{Name: "gear"}, 24)

	rgba1, ok1 := img1.(*image.RGBA)
	rgba2, ok2 := img2.(*image.RGBA)
	if !ok1 || !ok2 {
		t.Fatalf("ожидал *image.RGBA, получил %T и %T", img1, img2)
	}
	if rgba1 != rgba2 {
		t.Fatal("повторный запрос того же размера дал другой объект — кэш не работает")
	}
}

func TestIconSet_DifferentSizesReturnDifferentImages(t *testing.T) {
	s := NewIconSet("")
	s.Register("gear", []byte(validSVG))

	img16 := s.ResolveIcon(theme.IconRef{Name: "gear"}, 16)
	img32 := s.ResolveIcon(theme.IconRef{Name: "gear"}, 32)

	if w, _ := imgSize(img16); w != 16 {
		t.Fatalf("размер 16: получил %d", w)
	}
	if w, _ := imgSize(img32); w != 32 {
		t.Fatalf("размер 32: получил %d", w)
	}
	if img16 == img32 {
		t.Fatal("иконки разных размеров — один и тот же объект")
	}
}

func TestIconSet_UnknownNameReturnsPlaceholder(t *testing.T) {
	s := NewIconSet("")

	img := s.ResolveIcon(theme.IconRef{Name: "no-such-icon"}, 20)
	if img == nil {
		t.Fatal("ResolveIcon вернул nil для неизвестного имени")
	}
	if w, h := imgSize(img); w != 20 || h != 20 {
		t.Fatalf("размер заглушки = %dx%d, хочу 20x20", w, h)
	}
	if !hasOpaquePixel(img) {
		t.Fatal("заглушка пустая")
	}
}

func TestIconSet_BrokenSVGReturnsPlaceholder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.svg"), []byte(brokenSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewIconSet(dir)

	img := s.ResolveIcon(theme.IconRef{Source: "broken.svg"}, 18)
	if img == nil {
		t.Fatal("ResolveIcon вернул nil для битого SVG")
	}
	if w, h := imgSize(img); w != 18 || h != 18 {
		t.Fatalf("размер заглушки = %dx%d, хочу 18x18", w, h)
	}
}

func TestIconSet_EmptyRefReturnsPlaceholder(t *testing.T) {
	s := NewIconSet("")
	img := s.ResolveIcon(theme.IconRef{}, 10)
	if img == nil {
		t.Fatal("ResolveIcon вернул nil для пустой ссылки")
	}
	if w, _ := imgSize(img); w != 10 {
		t.Fatalf("размер = %d, хочу 10", w)
	}
}

func TestIconSet_NameTakesPriorityOverSource(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "icon.svg"), []byte(validSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewIconSet(dir)
	registered := image.NewRGBA(image.Rect(0, 0, 5, 5))
	s.RegisterImage("icon", registered)

	img := s.ResolveIcon(theme.IconRef{Name: "icon", Source: "icon.svg"}, 5)
	if img != image.Image(registered) {
		t.Fatal("при заданных Name и Source приоритет должен быть у Name")
	}
}

func TestIconSet_FromFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "icon.svg"), []byte(validSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewIconSet(dir)

	img := s.ResolveIcon(theme.IconRef{Source: "icon.svg"}, 22)
	if img == nil {
		t.Fatal("ResolveIcon вернул nil для существующего файла")
	}
	if w, h := imgSize(img); w != 22 || h != 22 {
		t.Fatalf("размер = %dx%d, хочу 22x22", w, h)
	}
	if !hasOpaquePixel(img) {
		t.Fatal("иконка из файла пустая")
	}
}

func TestIconSet_PathOutsideBaseDirRejected(t *testing.T) {
	root := t.TempDir()
	secretDir := filepath.Join(root, "secret")
	baseDir := filepath.Join(root, "theme")
	if err := os.MkdirAll(secretDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(secretDir, "secret.svg")
	if err := os.WriteFile(secretPath, []byte(validSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewIconSet(baseDir)

	img := s.ResolveIcon(theme.IconRef{Source: "../secret/secret.svg"}, 16)
	if img == nil {
		t.Fatal("ResolveIcon вернул nil вместо заглушки")
	}
	if w, _ := imgSize(img); w != 16 {
		t.Fatalf("размер заглушки = %d, хочу 16", w)
	}

	// Заглушка того же размера — доказательство, что вернулась именно она,
	// а не содержимое файла снаружи baseDir.
	ph := s.ResolveIcon(theme.IconRef{}, 16)
	rgba1, ok1 := img.(*image.RGBA)
	rgba2, ok2 := ph.(*image.RGBA)
	if !ok1 || !ok2 || rgba1 != rgba2 {
		t.Fatal("путь вне baseDir не свёлся к заглушке")
	}

	// Абсолютный путь тоже отклоняется.
	imgAbs := s.ResolveIcon(theme.IconRef{Source: secretPath}, 16)
	rgbaAbs, okAbs := imgAbs.(*image.RGBA)
	if !okAbs || rgbaAbs != rgba2 {
		t.Fatal("абсолютный путь не отклонён")
	}
}

func TestBuiltinIcons_AllNamesNonEmpty(t *testing.T) {
	names := []string{
		"start", "volume", "volume.muted",
		"network.wifi", "network.ethernet", "battery",
	}
	s := BuiltinIcons()
	for _, name := range names {
		img := s.ResolveIcon(theme.IconRef{Name: name}, 24)
		if img == nil {
			t.Fatalf("%s: ResolveIcon вернул nil", name)
		}
		if w, h := imgSize(img); w != 24 || h != 24 {
			t.Fatalf("%s: размер = %dx%d, хочу 24x24", name, w, h)
		}
		if !hasOpaquePixel(img) {
			t.Fatalf("%s: иконка пустая (нет непрозрачных пикселей)", name)
		}
	}
}

func TestIconSet_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "icon.svg"), []byte(validSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewIconSet(dir)
	s.Register("gear", []byte(validSVG))
	builtin := BuiltinIcons()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			size := 12 + i%5
			switch i % 4 {
			case 0:
				_ = s.ResolveIcon(theme.IconRef{Name: "gear"}, size)
			case 1:
				_ = s.ResolveIcon(theme.IconRef{Source: "icon.svg"}, size)
			case 2:
				_ = s.ResolveIcon(theme.IconRef{Name: "unknown"}, size)
			case 3:
				_ = builtin.ResolveIcon(theme.IconRef{Name: "start"}, size)
			}
		}(i)
	}
	wg.Wait()
}
