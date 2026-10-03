package window

import (
	"reflect"
	"testing"
)

func TestFileURI_EncodesLikeGLib(t *testing.T) {
	cases := map[string]string{
		"/home/u/a.txt":        "file:///home/u/a.txt",
		"/home/u/my file.txt":  "file:///home/u/my%20file.txt",
		"/home/u/Отчёт.txt":    "file:///home/u/%D0%9E%D1%82%D1%87%D1%91%D1%82.txt",
		"/tmp/a#b?c%d":         "file:///tmp/a%23b%3Fc%25d",
		"/tmp/x(1)+y=z,@w:v!$": "file:///tmp/x(1)+y=z,@w:v!$",
	}
	for in, want := range cases {
		if got := fileURI(in); got != want {
			t.Errorf("fileURI(%q) = %q, ждали %q", in, got, want)
		}
	}
}

func TestClipboardURIList_RoundTrip(t *testing.T) {
	paths := []string{"/home/u/Отчёт за год.odt", "/tmp/a#b", "/srv/c"}
	body := buildURIList(paths)
	if body != "file:///home/u/%D0%9E%D1%82%D1%87%D1%91%D1%82%20%D0%B7%D0%B0%20%D0%B3%D0%BE%D0%B4.odt\r\nfile:///tmp/a%23b\r\nfile:///srv/c\r\n" {
		t.Errorf("uri-list: %q", body)
	}
	got, ok := parseClipboardURIList(body)
	if !ok || !reflect.DeepEqual(got, paths) {
		t.Errorf("разбор: %v %v", got, ok)
	}
}

func TestClipboardURIList_RejectsForeign(t *testing.T) {
	for _, body := range []string{
		"file:///tmp/a\r\nhttps://example.com/b\r\n", // ссылка в списке
		"file://otherhost/tmp/a\r\n",                 // чужой хост
		"file:tmp/relative\r\n",                      // не абсолютный путь
		"# только комментарий\r\n",
		"",
	} {
		if p, ok := parseClipboardURIList(body); ok {
			t.Errorf("%q принят как файлы: %v", body, p)
		}
	}
	// localhost и комментарии — допустимы.
	got, ok := parseClipboardURIList("# from Nautilus\r\nfile://localhost/tmp/a\r\n")
	if !ok || !reflect.DeepEqual(got, []string{"/tmp/a"}) {
		t.Errorf("localhost: %v %v", got, ok)
	}
}

func TestGnomeCopiedFiles_RoundTrip(t *testing.T) {
	paths := []string{"/home/u/a b", "/home/u/c"}
	for _, cut := range []bool{false, true} {
		body := buildGnomeCopiedFiles(paths, cut)
		head := "copy"
		if cut {
			head = "cut"
		}
		if want := head + "\nfile:///home/u/a%20b\nfile:///home/u/c"; body != want {
			t.Errorf("cut=%v: %q", cut, body)
		}
		got, gotCut, ok := parseGnomeCopiedFiles(body)
		if !ok || gotCut != cut || !reflect.DeepEqual(got, paths) {
			t.Errorf("cut=%v разбор: %v %v %v", cut, got, gotCut, ok)
		}
	}
	for _, body := range []string{
		"move\nfile:///tmp/a",                  // неизвестное действие
		"copy\nfile:///tmp/a\nhttps://x.org/b", // ссылка в списке
		"copy",                                 // нет файлов
		"file:///tmp/a",                        // нет строки действия
	} {
		if p, _, ok := parseGnomeCopiedFiles(body); ok {
			t.Errorf("%q принят: %v", body, p)
		}
	}
	// Завершающий перевод строки и CRLF от чужих программ.
	if p, cut, ok := parseGnomeCopiedFiles("cut\r\nfile:///tmp/a\r\n"); !ok || !cut || len(p) != 1 {
		t.Errorf("CRLF: %v %v %v", p, cut, ok)
	}
}
