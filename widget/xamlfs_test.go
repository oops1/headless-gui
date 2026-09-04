package widget

// Тесты LoadUIFromXAMLFS (GG-7): ресурсы разметки (Image Source и т.п.)
// читаются из fs.FS так же, как раньше только из каталога на диске.
//
// Проверяем на testing/fstest.MapFS — стандартная библиотека, без
// дополнительных зависимостей.

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// tinyPNGBytes — минимальный валидный PNG (8×8, непрозрачный), пригодный и
// для файла на диске, и для fstest.MapFile.Data.
func tinyPNGBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, image.NewUniform(image.White).At(0, 0))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// xamlfsImageMarkup — <Image Source="…"> с именем "pic", по которому
// достаём виджет из reg. src подставляется как есть — тесты передают и
// прямой слэш, и обратный, и путь с выходом за пределы.
func xamlfsImageMarkup(src string) string {
	return `<Canvas Width="50" Height="50">` +
		`<Image x:Name="pic" Left="0" Top="0" Width="16" Height="16" Source="` + src + `"/>` +
		`</Canvas>`
}

// TestLoadUIFromXAMLFS_ImageFromMapFS — <Image Source="icons/a.png"/>
// грузится из fs.FS (embed.FS в проде, MapFS в тесте): картинка попадает в
// виджет, а не остаётся nil.
//
// Без LoadUIFromXAMLFS этот тест не компилируется вовсе — функции не
// существовало.
func TestLoadUIFromXAMLFS_ImageFromMapFS(t *testing.T) {
	fsys := fstest.MapFS{
		"icons/a.png": {Data: tinyPNGBytes(t)},
	}

	w, reg, err := LoadUIFromXAMLFS([]byte(xamlfsImageMarkup("icons/a.png")), fsys)
	if err != nil {
		t.Fatalf("LoadUIFromXAMLFS: %v", err)
	}
	if w == nil {
		t.Fatal("корневой виджет nil")
	}
	iw, ok := reg["pic"].(*ImageWidget)
	if !ok {
		t.Fatalf("pic не *ImageWidget: %T", reg["pic"])
	}
	if iw.Image() == nil {
		t.Fatal("Image Source=\"icons/a.png\" не загружен из fs.FS")
	}
}

// TestLoadUIFromXAMLFS_DiskPathStillWorks — тот же XAML без fs.FS, с baseDir
// на временном каталоге диска, даёт тот же результат: старый путь
// (LoadUIFromXAMLWithBase) не сломан появлением LoadUIFromXAMLFS.
func TestLoadUIFromXAMLFS_DiskPathStillWorks(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "icons"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "icons", "a.png"), tinyPNGBytes(t), 0o644); err != nil {
		t.Fatal(err)
	}

	_, reg, err := LoadUIFromXAMLWithBase([]byte(xamlfsImageMarkup("icons/a.png")), dir)
	if err != nil {
		t.Fatalf("LoadUIFromXAMLWithBase: %v", err)
	}
	iw, ok := reg["pic"].(*ImageWidget)
	if !ok {
		t.Fatalf("pic не *ImageWidget: %T", reg["pic"])
	}
	if iw.Image() == nil {
		t.Fatal("Image Source=\"icons/a.png\" не загружен с диска — старый путь сломан")
	}
}

// TestLoadUIFromXAMLFS_BackslashPath — разметку часто пишут на Windows, где
// путь может содержать "\": "icons\a.png" обязан найтись в fs.FS так же, как
// "icons/a.png" — резолв приводит разделитель к виду, который принимает
// fs.FS (см. resolveXAMLResourceFS в xaml.go).
func TestLoadUIFromXAMLFS_BackslashPath(t *testing.T) {
	fsys := fstest.MapFS{
		"icons/a.png": {Data: tinyPNGBytes(t)},
	}

	_, reg, err := LoadUIFromXAMLFS([]byte(xamlfsImageMarkup(`icons\a.png`)), fsys)
	if err != nil {
		t.Fatalf("LoadUIFromXAMLFS: %v", err)
	}
	iw, ok := reg["pic"].(*ImageWidget)
	if !ok {
		t.Fatalf("pic не *ImageWidget: %T", reg["pic"])
	}
	if iw.Image() == nil {
		t.Fatal(`Image Source="icons\a.png" (обратный слэш) не найден в fs.FS`)
	}
}

// TestLoadUIFromXAMLFS_PathEscapeRejected — "../secret.png" не читается:
// граница fs.FS дублирует ту же проверку выхода за пределы каталога, что
// давно есть для baseDir на диске (SEC-8, ErrResourceOutsideBase). Загрузка
// не должна падать ошибкой и тем более паниковать — просто картинки не будет.
func TestLoadUIFromXAMLFS_PathEscapeRejected(t *testing.T) {
	fsys := fstest.MapFS{
		"secret.png":     {Data: tinyPNGBytes(t)},
		"ui/icons/dummy": {Data: []byte("x")}, // просто чтобы fsys не был плоско-пустым
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("LoadUIFromXAMLFS запаниковал на \"../secret.png\": %v", r)
		}
	}()

	_, reg, err := LoadUIFromXAMLFS([]byte(xamlfsImageMarkup("../secret.png")), fsys)
	if err != nil {
		t.Fatalf("LoadUIFromXAMLFS: %v", err)
	}
	iw, ok := reg["pic"].(*ImageWidget)
	if !ok {
		t.Fatalf("pic не *ImageWidget: %T", reg["pic"])
	}
	if iw.Image() != nil {
		t.Fatal("Image Source=\"../secret.png\" прочитан за пределами fs.FS — граница не работает")
	}
}

// TestLoadUIFromXAMLFS_MissingFileNoImage — отсутствующий в fs.FS файл не
// приводит к панике или ошибке загрузки: виджет просто остаётся без
// картинки — ровно как ведёт себя сегодняшний код при чтении с диска
// (resolveXAMLResource+os.Open тоже не паникует, а логирует и пропускает).
func TestLoadUIFromXAMLFS_MissingFileNoImage(t *testing.T) {
	fsys := fstest.MapFS{} // пусто — "icons/a.png" отсутствует

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("LoadUIFromXAMLFS запаниковал на отсутствующем файле: %v", r)
		}
	}()

	_, reg, err := LoadUIFromXAMLFS([]byte(xamlfsImageMarkup("icons/a.png")), fsys)
	if err != nil {
		t.Fatalf("LoadUIFromXAMLFS: %v", err)
	}
	iw, ok := reg["pic"].(*ImageWidget)
	if !ok {
		t.Fatalf("pic не *ImageWidget: %T", reg["pic"])
	}
	if iw.Image() != nil {
		t.Fatal("Image Source указывает на несуществующий файл, но картинка загружена")
	}
}
