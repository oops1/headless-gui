package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Шрифты из assets/fonts движок ищет относительно рабочего каталога процесса,
// а тест идёт из каталога пакета — отсюда "..".
const assetFontsDir = "../assets/fonts"

// Файлы fallback названы в коде поимённо: удалить или переименовать их значит
// молча оставить движок без покрытия для псевдографики и ✓ ✗ ⚠ на машине, где
// системных шрифтов нет. Тест держит это имя.
func TestAssetFallbackFontsPresent(t *testing.T) {
	for _, p := range assetFallbackFontPaths() {
		full := filepath.Join("..", p)
		data, err := os.ReadFile(full)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		if newFontCacheFromData(data, DefaultDPI) == nil {
			t.Errorf("%s: не разбирается как шрифт", p)
		}
	}
}

// Всякий TTF/OTF из папки движок регистрирует при старте, молча пропуская
// нечитаемые. Битый файл там оборачивается пропавшим шрифтом без единого
// сообщения, поэтому разбор проверяется здесь.
func TestAssetFontsParse(t *testing.T) {
	entries, err := os.ReadDir(assetFontsDir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ent := range entries {
		ext := strings.ToLower(filepath.Ext(ent.Name()))
		if ent.IsDir() || (ext != ".ttf" && ext != ".otf") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(assetFontsDir, ent.Name()))
		if err != nil {
			t.Errorf("%s: %v", ent.Name(), err)
			continue
		}
		if newFontCacheFromData(data, DefaultDPI) == nil {
			t.Errorf("%s: не разбирается как шрифт", ent.Name())
		}
		n++
	}
	if n == 0 {
		t.Fatal("в assets/fonts не нашлось ни одного шрифта")
	}
	t.Logf("разобрано шрифтов: %d", n)
}

// Каждое семейство обязано ехать со своей лицензией: и OFL, и Bitstream Vera
// требуют, чтобы текст разрешения путешествовал вместе со шрифтом. Файл
// лицензии, потерянный при чистке папки, — это нарушение, которое иначе
// обнаружится не у нас.
func TestAssetFontLicensesPresent(t *testing.T) {
	// префикс имени файла шрифта → файл лицензии рядом с ним
	licenses := map[string]string{
		"Roboto":     "Roboto-OFL.txt",
		"OpenSans":   "OpenSans-OFL.txt",
		"Inter":      "Inter-OFL.txt",
		"Liberation": "Liberation-OFL.txt",
		"DejaVu":     "DejaVu-LICENSE.txt",
		"Go-":        "Go-LICENSE.txt",
	}
	entries, err := os.ReadDir(assetFontsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		ext := strings.ToLower(filepath.Ext(ent.Name()))
		if ent.IsDir() || (ext != ".ttf" && ext != ".otf") {
			continue
		}
		found := ""
		for prefix, lic := range licenses {
			if strings.HasPrefix(ent.Name(), prefix) {
				found = lic
				break
			}
		}
		if found == "" {
			t.Errorf("%s: семейство не значится в таблице лицензий теста", ent.Name())
			continue
		}
		if _, err := os.Stat(filepath.Join(assetFontsDir, found)); err != nil {
			t.Errorf("%s: нет файла лицензии %s", ent.Name(), found)
		}
	}
}
