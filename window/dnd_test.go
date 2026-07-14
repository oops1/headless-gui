package window

import (
	"reflect"
	"testing"
)

// TestParseURIList проверяет разбор тела text/uri-list: file://-URI,
// декодирование %XX, отбрасывание hostname, CRLF/LF, комментарии, кириллица,
// пробелы и несколько файлов. Прочие схемы игнорируются.
func TestParseURIList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "single file LF",
			in:   "file:///home/user/a.txt\n",
			want: []string{"/home/user/a.txt"},
		},
		{
			name: "CRLF terminators",
			in:   "file:///a.txt\r\nfile:///b.txt\r\n",
			want: []string{"/a.txt", "/b.txt"},
		},
		{
			name: "percent-encoded space",
			in:   "file:///home/user/my%20file.txt\n",
			want: []string{"/home/user/my file.txt"},
		},
		{
			name: "cyrillic percent-encoded (UTF-8)",
			// «файл.txt» в UTF-8, percent-encoded.
			in:   "file:///home/%D1%84%D0%B0%D0%B9%D0%BB.txt\n",
			want: []string{"/home/файл.txt"},
		},
		{
			name: "hostname stripped",
			in:   "file://localhost/etc/hosts\n",
			want: []string{"/etc/hosts"},
		},
		{
			name: "comments and blank lines skipped",
			in:   "# comment\r\n\r\nfile:///a\r\n#another\r\n",
			want: []string{"/a"},
		},
		{
			name: "non-file scheme ignored",
			in:   "http://example.com/x\r\nfile:///local\r\n",
			want: []string{"/local"},
		},
		{
			name: "multiple files with encoded chars",
			in:   "file:///a%2Fb.txt\nfile:///c%23d.txt\n",
			want: []string{"/a/b.txt", "/c#d.txt"},
		},
		{
			name: "empty body",
			in:   "",
			want: nil,
		},
		{
			name: "no scheme host (file:/path)",
			in:   "file:/abs/path\n",
			want: []string{"/abs/path"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseURIList(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseURIList(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

// TestPercentDecode — точечные проверки декодера процентов.
func TestPercentDecode(t *testing.T) {
	cases := map[string]string{
		"nochange":     "nochange",
		"a%20b":        "a b",
		"%2F":          "/",
		"bad%2":        "bad%2",  // обрезанная последовательность — как есть
		"%GG":          "%GG",    // невалидные hex-цифры — как есть
		"%D0%AF":       "Я",       // кириллица (UTF-8)
	}
	for in, want := range cases {
		if got := percentDecode(in); got != want {
			t.Fatalf("percentDecode(%q) = %q, want %q", in, got, want)
		}
	}
}
