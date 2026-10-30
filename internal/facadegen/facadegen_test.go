package facadegen

import (
	"go/parser"
	"go/token"
	"testing"
)

// Условие сборки файла: //go:build, суффиксы ОС и архитектуры; суффикс,
// уже стоящий в выражении конъюнктом, второй раз не пишется.
func TestConstraintOf(t *testing.T) {
	for _, tc := range []struct{ name, build, want string }{
		{"a.go", "", ""},
		{"a_windows.go", "", "windows"},
		{"a_windows.go", "windows", "windows"},
		{"a_linux.go", "linux && !android", "linux && !android"},
		{"a_linux.go", "linux || freebsd", "linux && (linux || freebsd)"},
		{"a_amd64.go", "", "amd64"},
		{"a_windows_amd64.go", "", "windows && amd64"},
		{"a_other.go", "!windows", "!windows"},
		{"a_test_helper.go", "", ""},
		{"a_aix.go", "", "aix"},
	} {
		src := "package p\n"
		if tc.build != "" {
			src = "//go:build " + tc.build + "\n\n" + src
		}
		f, err := parser.ParseFile(token.NewFileSet(), tc.name, src, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		if got := constraintOf(srcFile{tc.name, f}); got != tc.want {
			t.Errorf("%s (%q): %q, ждали %q", tc.name, tc.build, got, tc.want)
		}
	}
}
