package text

import (
	"testing"
)

// Свойства шрифта читаются из самого файла, а не из его имени.
func TestDescribeFont_ReadsFileMetadata(t *testing.T) {
	cases := []struct {
		file   string
		key    string
		weight int
		italic bool
	}{
		{"OpenSans-Light", "opensans", 300, false},
		{"OpenSans-SemiBold", "opensans", 600, false},
		{"OpenSans-Bold", "opensans", 700, false},
		{"OpenSans-BoldItalic", "opensans", 700, true},
		{"GolosText-SemiBold", "golostext", 600, false},
		{"Roboto-Italic", "roboto", 400, true},
	}
	for _, tc := range cases {
		data, err := ReadFontFile("../../assets/fonts/" + tc.file + ".ttf")
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		fc := NewFontCacheFromData(data, DefaultDPI)
		m := describeFont(fc, tc.file)
		if m.keys[0] != tc.key || m.weight != tc.weight || m.italic != tc.italic {
			t.Errorf("%s: ключ %q вес %d наклон %v; ждали %q %d %v",
				tc.file, m.keys[0], m.weight, m.italic, tc.key, tc.weight, tc.italic)
		}
	}
}
