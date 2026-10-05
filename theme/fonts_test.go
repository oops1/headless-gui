package theme

import (
	"strings"
	"testing"
)

func TestFontSpec_EffectiveWeight(t *testing.T) {
	cases := []struct {
		f    FontSpec
		want int
	}{
		{FontSpec{}, 0},
		{FontSpec{Bold: true}, WeightBold},
		{FontSpec{Weight: WeightLight}, WeightLight},
		{FontSpec{Bold: true, Weight: WeightSemiBold}, WeightSemiBold},
	}
	for _, c := range cases {
		if got := c.f.EffectiveWeight(); got != c.want {
			t.Errorf("%+v: вес %d, ждали %d", c.f, got, c.want)
		}
	}
}

// Профиль Windows 10 пишет Open Sans 8,5 pt и объявляет именованные шрифты
// оболочки; остальные темы остаются на прежнем шрифте и кегле 9.
func TestWindows10Profile_Fonts(t *testing.T) {
	m := NewManager()
	if err := RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	def, ok := m.GetFont("default")
	if !ok || def.Family != "Open Sans" || def.Size != 8.5 {
		t.Errorf("default = %+v, ждали Open Sans 8.5", def)
	}
	for _, c := range []struct {
		key    Key
		weight int
	}{{"caption", 0}, {"title", WeightSemiBold}, {"clock.large", WeightLight}} {
		f, ok := m.GetFont(c.key)
		if !ok || f.Family != "Open Sans" || f.Size <= 0 || f.EffectiveWeight() != c.weight {
			t.Errorf("%s = %+v (есть=%v), ждали Open Sans веса %d", c.key, f, ok, c.weight)
		}
	}
	if !m.GetFlag(FlagTextSubpixel, false) {
		t.Error("профиль Windows 10 не просит подпиксельный текст")
	}
	// Дочерняя тёмная тема наследует шрифты.
	if err := m.SetTheme(ProfileWindows10Dark); err != nil {
		t.Fatal(err)
	}
	if f, _ := m.GetFont("default"); f.Family != "Open Sans" || f.Size != 8.5 {
		t.Errorf("тёмная Windows 10: default = %+v", f)
	}

	// Прочие темы не тронуты.
	for _, name := range []string{ProfileWindows2000, ProfileWindows11, ProfileMacOS} {
		if err := m.SetTheme(name); err != nil {
			t.Fatal(err)
		}
		f, _ := m.GetFont("default")
		if f.Family != "" || f.Size != 9 || f.Weight != 0 {
			t.Errorf("%s: default = %+v, ждали {Size: 9}", name, f)
		}
		if m.GetFlag(FlagTextSubpixel, false) {
			t.Errorf("%s просит подпиксельный текст", name)
		}
	}
}

// Вес шрифта переживает JSON-профиль и дельту стиля.
func TestJSON_FontWeight(t *testing.T) {
	src := `{"name":"w","fonts":{"title":{"family":"Open Sans","size":10,"weight":600}}}`
	res, err := LoadTheme(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	f := res.Profile.Fonts["title"]
	if f.Family != "Open Sans" || f.Size != 10 || f.Weight != 600 {
		t.Errorf("шрифт из JSON: %+v", f)
	}
}
