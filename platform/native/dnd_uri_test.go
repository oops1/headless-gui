package native

import (
	"strings"
	"testing"
)

// TestFileURIHostRejected проверяет, что file:// с чужим хостом отклоняется:
// путь на удалённой машине локальным не является.
func TestFileURIHostRejected(t *testing.T) {
	tests := []struct {
		uri  string
		want string // "" — URI должен быть отклонён
	}{
		{"file:///home/user/a.txt", "/home/user/a.txt"},
		{"file://localhost/home/user/a.txt", "/home/user/a.txt"},
		{"file://LocalHost/home/user/a.txt", "/home/user/a.txt"},
		{"file:/home/user/a.txt", "/home/user/a.txt"},
		{"file://evilhost/etc/passwd", ""},
		{"file://192.168.1.1/share/x", ""},
		{"file://evilhost", ""},
		{"file://attacker.example.com/%2Fetc/shadow", ""},
	}
	for _, c := range tests {
		got, ok := FileURIToPath(c.uri)
		if c.want == "" {
			if ok && got != "" {
				t.Errorf("FileURIToPath(%q) = %q, ожидался отказ", c.uri, got)
			}
			continue
		}
		if !ok || got != c.want {
			t.Errorf("FileURIToPath(%q) = %q,%v; ожидалось %q", c.uri, got, ok, c.want)
		}
	}
}

// TestParseURIListDropsRemoteHosts проверяет, что чужие хосты выпадают из
// списка, а локальные пути остаются.
func TestParseURIListDropsRemoteHosts(t *testing.T) {
	body := strings.Join([]string{
		"file:///home/user/ok1.txt",
		"file://evilhost/etc/passwd",
		"file://localhost/home/user/ok2.txt",
		"http://example.com/x",
	}, "\r\n")
	got := ParseURIList(body)
	want := []string{"/home/user/ok1.txt", "/home/user/ok2.txt"}
	if len(got) != len(want) {
		t.Fatalf("получено %v, ожидалось %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("получено %v, ожидалось %v", got, want)
		}
	}
}

// TestMaxDnDBytes фиксирует лимит на данные перетаскивания.
func TestMaxDnDBytes(t *testing.T) {
	if MaxDnDBytes != 16*1024*1024 {
		t.Fatalf("MaxDnDBytes = %d, ожидалось 16 МиБ", MaxDnDBytes)
	}
}
