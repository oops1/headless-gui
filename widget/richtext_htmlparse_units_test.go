package widget

import (
	"strings"
	"testing"
)

// Единицы измерения CSS и выгрузка с защитой пробелов.

func TestRichHTML_Units(t *testing.T) {
	px := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"10px", 10, true}, {"0", 0, true}, {"12pt", 16, true}, {"1in", 96, true},
		{"2.54cm", 96, true}, {"25.4mm", 96, true}, {"1pc", 16, true}, {"2em", 32, true},
		{"1rem", 16, true}, {"50%", 0, false}, {"abc", 0, false}, {"", 0, false}, {"5furlong", 0, false},
	}
	for _, c := range px {
		got, ok := rhParseLength(c.in)
		if ok != c.ok || ok && (got-c.want > 0.001 || c.want-got > 0.001) {
			t.Errorf("rhParseLength(%q) = %v, %v", c.in, got, ok)
		}
	}
	pt := []struct {
		in     string
		parent float64
		want   float64
		ok     bool
	}{
		{"12pt", 0, 12, true}, {"16px", 0, 12, true}, {"1in", 0, 72, true}, {"2.54cm", 0, 72, true},
		{"25.4mm", 0, 72, true}, {"1pc", 0, 12, true}, {"2em", 10, 20, true}, {"2em", 0, 24, true},
		{"1.5rem", 10, 18, true}, {"200%", 10, 20, true}, {"smaller", 10, 8.3, true}, {"larger", 10, 12, true},
		{"x-small", 0, 7.5, true}, {"-3pt", 0, 0, false}, {"99999pt", 0, 0, false}, {"3furlong", 0, 0, false},
	}
	for _, c := range pt {
		got, ok := rhParseFontSize(c.in, c.parent)
		if ok != c.ok || ok && (got-c.want > 0.001 || c.want-got > 0.001) {
			t.Errorf("rhParseFontSize(%q, %v) = %v, %v; ждали %v", c.in, c.parent, got, ok, c.want)
		}
	}
	for in, want := range map[string]float64{"1": 7.5, "3": 12, "7": 36, "9": 36, "0": 7.5, "-1": 10, "+2": 18} {
		if got, ok := rhFontTagSize(in); !ok || got != want {
			t.Errorf("rhFontTagSize(%q) = %v, %v; ждали %v", in, got, ok, want)
		}
	}
	if _, ok := rhFontTagSize("x"); ok {
		t.Error("rhFontTagSize(x) принял мусор")
	}
}

func TestRichHTML_ExportPreservesSpacesAndEmptyParagraphs(t *testing.T) {
	// Выгрузка пишет &nbsp; там, где HTML схлопнул бы пробел, и <br> в пустом абзаце.
	paras := []RichParagraph{rdP(rdTxt("  a  b ", RichRun{})), rdP()}
	html := richSelectionHTML(paras, richBuildDoc(paras), 0, 9)
	if !strings.Contains(html, "&nbsp; a &nbsp;b&nbsp;") {
		t.Errorf("пробелы не защищены: %s", html)
	}
	if !strings.Contains(html, "<br></p>") {
		t.Errorf("пустой абзац без <br>: %s", html)
	}
	// Одиночный пробел между словами остаётся обычным: по нему идёт перенос.
	one := []RichParagraph{rdP(rdTxt("a b c", RichRun{}))}
	if h := richSelectionHTML(one, richBuildDoc(one), 0, 5); !strings.Contains(h, ">a b c<") {
		t.Errorf("%s", h)
	}
}
