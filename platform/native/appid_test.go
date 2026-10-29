package native

import (
	"testing"
)

func TestDefaultAppID(t *testing.T) {
	cases := map[string]string{
		"/usr/bin/winline-calc":         "winline-calc",
		"./winline-explorer":            "winline-explorer",
		"winline-calc":                  "winline-calc",
		"C:\\Program Files\\app\\x.exe": "x",
		"":                              "headless-gui",
		"/":                             "headless-gui",
	}
	for argv0, want := range cases {
		if got := DefaultAppID(argv0); got != want {
			t.Errorf("DefaultAppID(%q) = %q, ждал %q", argv0, got, want)
		}
	}
}
