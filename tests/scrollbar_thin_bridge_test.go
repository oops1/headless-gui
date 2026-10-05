package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Поля полосы прокрутки ThemeStyle переживают мост «тема → профиль → тема».
func TestBridge_ScrollbarFieldsRoundTrip(t *testing.T) {
	src := widget.DarkTheme()
	src.Style.ScrollbarThin = true
	src.Style.ScrollbarWidth = 14
	src.Style.ScrollbarThinWidth = 3
	src.Style.ScrollbarThinHoverWidth = 8

	got := roundTrip(t, src).Style
	if !got.ScrollbarThin || got.ScrollbarWidth != 14 || got.ScrollbarThinWidth != 3 || got.ScrollbarThinHoverWidth != 8 {
		t.Errorf("поля полосы потеряны: %+v", got)
	}
}

// Пресеты полос не объявляют: профиль, собранный из них, без токенов полосы,
// то есть ScrollView под ними остаётся прежним (фиксированная полоса 10 px).
func TestBridge_PresetsDeclareNoScrollbarTokens(t *testing.T) {
	for _, name := range widget.ThemeNames() {
		p := widget.ProfileFromTheme(widget.ThemeByName(name))
		for _, k := range []theme.Key{"scrollbar.thin", "scrollbar.width", "scrollbar.thin.width", "scrollbar.thin.hover.width"} {
			if _, ok := p.Metrics[k]; ok {
				t.Errorf("%s: объявлена метрика %s", name, k)
			}
			if _, ok := p.Flags[k]; ok {
				t.Errorf("%s: объявлен флаг %s", name, k)
			}
		}
	}
}
