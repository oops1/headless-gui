package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// realFont — настоящий TTF из assets/fonts: подделка из случайных байт не
// разберётся, и проверять на ней регистрацию нечего.
func realFont(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "assets", "fonts", "LiberationSans-Regular.ttf"))
	if err != nil {
		t.Skipf("шрифта для проверки нет: %v", err)
	}
	return data
}

// registered сообщает, зарегистрирован ли шрифт под этим именем ПО-НАСТОЯЩЕМУ.
//
// Спрашивать список имён мало: fontFor подсовывает шрифт по умолчанию всякому
// незнакомому имени, и виджет с опечаткой в FontFamily рисуется молча.
// Сравнение с заведомо несуществующим именем это различает.
func registered(e *Engine, name string) bool {
	return e.canvas.fontFor(name) != e.canvas.fontFor("нет-такого-шрифта-\x00")
}

func TestRegisterFontFS_RegistersFileAndFamily(t *testing.T) {
	data := realFont(t)
	fsys := fstest.MapFS{
		"ui/fonts/LiberationSans-Regular.ttf": {Data: data},
		"ui/fonts/LiberationSans-Bold.ttf":    {Data: data},
		"ui/fonts/README.md":                  {Data: []byte("не шрифт")},
	}

	eng := New(64, 64, 30)
	if err := eng.RegisterFontFS(fsys, "ui/fonts"); err != nil {
		t.Fatalf("регистрация: %v", err)
	}

	// Regular даёт и имя файла, и имя семейства; Bold — только имя файла:
	// FontWeight переключает лишь встроенные шрифты, жирный берётся по имени.
	for _, name := range []string{"LiberationSans-Regular", "LiberationSans", "LiberationSans-Bold"} {
		if !registered(eng, name) {
			t.Errorf("шрифт %q не зарегистрирован", name)
		}
	}
	if registered(eng, "README") {
		t.Error("не-шрифт из каталога попал в список шрифтов")
	}
	if !eng.canvas.SetDefaultFont("LiberationSans") {
		t.Error("шрифт из fs.FS не годится в шрифты по умолчанию")
	}
}

// Корень fs.FS: embed.FS часто кладут шрифты прямо в корень.
func TestRegisterFontFS_RootDir(t *testing.T) {
	fsys := fstest.MapFS{"Inter-Regular.ttf": {Data: realFont(t)}}
	eng := New(64, 64, 30)
	if err := eng.RegisterFontFS(fsys, ""); err != nil {
		t.Fatalf("регистрация из корня: %v", err)
	}
	if !registered(eng, "Inter") {
		t.Error("шрифт из корня fs.FS не зарегистрирован")
	}
}

// Опечатка в //go:embed обязана быть слышной: иначе её ищут по пропавшему
// шрифту на экране.
func TestRegisterFontFS_ReportsEmptyAndMissing(t *testing.T) {
	eng := New(64, 64, 30)
	fsys := fstest.MapFS{"fonts/README.md": {Data: []byte("не шрифт")}}

	if err := eng.RegisterFontFS(fsys, "fonts"); err == nil {
		t.Error("каталог без шрифтов принят молча")
	}
	if err := eng.RegisterFontFS(fsys, "нет-такого-каталога"); err == nil {
		t.Error("несуществующий каталог принят молча")
	}
	if err := eng.RegisterFontFS(nil, "fonts"); err == nil {
		t.Error("отсутствие файловой системы принято молча")
	}
}

// Битый файл рядом с целым не отменяет регистрацию целого: каталог поставки
// не обязан быть стерильным.
func TestRegisterFontFS_SkipsBrokenFile(t *testing.T) {
	fsys := fstest.MapFS{
		"f/Broken-Regular.ttf":         {Data: []byte("это не шрифт")},
		"f/LiberationSans-Regular.ttf": {Data: realFont(t)},
	}
	eng := New(64, 64, 30)
	if err := eng.RegisterFontFS(fsys, "f"); err != nil {
		t.Fatalf("битый сосед сорвал регистрацию: %v", err)
	}
	if !registered(eng, "LiberationSans") {
		t.Error("целый шрифт не зарегистрирован")
	}
	if registered(eng, "Broken") {
		t.Error("битый файл зарегистрирован как шрифт")
	}
}

// Имя семейства, уже занятое приложением, каталог не перебивает: приложение
// зарегистрировало его позже и намеренно.
func TestRegisterFontFS_DoesNotStealTakenFamily(t *testing.T) {
	data := realFont(t)
	eng := New(64, 64, 30)
	// Своё «Roboto» приложения — это на самом деле Liberation, и подмену видно
	// по метрикам: у него другая ширина строки.
	eng.canvas.RegisterFont("Roboto", data)
	own := eng.canvas.fontFor("Roboto")

	robo, err := os.ReadFile(filepath.Join("..", "assets", "fonts", "Roboto-Regular.ttf"))
	if err != nil {
		t.Skipf("Roboto для проверки нет: %v", err)
	}
	fsys := fstest.MapFS{"f/Roboto-Regular.ttf": {Data: robo}}
	if err := eng.RegisterFontFS(fsys, "f"); err != nil {
		t.Fatalf("регистрация: %v", err)
	}

	if eng.canvas.fontFor("Roboto") != own {
		t.Error("каталог перебил имя семейства, занятое приложением")
	}
	if !registered(eng, "Roboto-Regular") {
		t.Error("под именем файла шрифт из каталога всё равно обязан быть")
	}
}

// Каталог на диске и каталог в fs.FS дают ОДИН И ТОТ ЖЕ набор имён: иначе
// поставка в embed.FS расходилась бы с разработкой из корня репозитория.
func TestRegisterFontFS_SameNamesAsDisk(t *testing.T) {
	dir := filepath.Join("..", "assets", "fonts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("каталога шрифтов нет: %v", err)
	}
	fsys := fstest.MapFS{}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, ent.Name()))
		if err != nil {
			t.Fatal(err)
		}
		fsys["fonts/"+ent.Name()] = &fstest.MapFile{Data: data}
	}

	fromDisk := New(64, 64, 30)
	fromDisk.RegisterFontDir(dir)
	fromFS := New(64, 64, 30)
	if err := fromFS.RegisterFontFS(fsys, "fonts"); err != nil {
		t.Fatalf("регистрация из fs.FS: %v", err)
	}

	want := strings.Join(fromDisk.AvailableFonts(), "\n")
	got := strings.Join(fromFS.AvailableFonts(), "\n")
	if want != got {
		t.Errorf("наборы имён разошлись:\nс диска:\n%s\nиз fs.FS:\n%s", want, got)
	}
}
