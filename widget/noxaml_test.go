package widget

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Программа, которая XAML не грузит, не держит в бинаре ни парсера, ни
// строителей (ADR-0012, 8.2b): фасад импортирует widget/xaml, но компоновщик
// выбрасывает недостижимое. Контроль — программа с LoadUIFromXAML: в ней те
// же символы есть, значит проверка их видит. Глобального состояния тест не
// трогает: только собирает программы из testdata.
func TestNoXAML_ParserNotLinked(t *testing.T) {
	if testing.Short() {
		t.Skip("сборка двух программ")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("нет go в PATH")
	}
	dir := t.TempDir()
	symbols := func(name string) []string {
		t.Helper()
		out := filepath.Join(dir, name+".exe")
		cmd := exec.Command(goBin, "build", "-o", out, "./testdata/"+name)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("сборка %s: %v\n%s", name, err, b)
		}
		b, err := exec.Command(goBin, "tool", "nm", out).CombinedOutput()
		if err != nil {
			t.Fatalf("nm %s: %v\n%s", name, err, b)
		}
		return xamlCode(string(b))
	}
	if got := symbols("noxaml"); len(got) > 0 {
		n := len(got)
		if n > 10 {
			got = got[:10]
		}
		t.Errorf("программа без XAML держит %d символов кода XAML, например:\n%s", n, strings.Join(got, "\n"))
	}
	if got := symbols("withxaml"); len(got) == 0 {
		t.Error("в программе с LoadUIFromXAML не нашлось кода XAML — проверка ничего не видит")
	}
}

// xamlCode — символы кода (T) пакета widget/xaml из вывода go tool nm
// («адрес тип имя»; имя может содержать пробелы); инициализация пакета не
// считается. Декодер encoding/xml не признак: его держит и SVG.
func xamlCode(nm string) []string {
	var out []string
	for _, line := range strings.Split(nm, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || f[1] != "T" {
			continue
		}
		sym := strings.Join(f[2:], " ")
		if !strings.HasPrefix(sym, "github.com/oops1/headless-gui/v4/widget/xaml.") {
			continue
		}
		if strings.HasSuffix(sym, ".init") || strings.Contains(sym, ".init.") {
			continue
		}
		out = append(out, sym)
	}
	return out
}
