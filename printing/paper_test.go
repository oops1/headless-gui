package printing

import (
	"errors"
	"image"
	"math"
	"strings"
	"testing"
)

func TestPagePixels(t *testing.T) {
	cases := []struct {
		name   string
		paper  Paper
		o      Orientation
		dpi    int
		wh, ht int
	}{
		{"A4 300 книжная", A4, Portrait, 300, 2480, 3508},
		{"A4 300 альбомная", A4, Landscape, 300, 3508, 2480},
		{"A4 72", A4, Portrait, 72, 595, 842},
		{"A4 150", A4, Portrait, 150, 1240, 1754},
		{"A4 600", A4, Portrait, 600, 4961, 7016},
		{"Letter 300", Letter, Portrait, 300, 2550, 3300},
		{"Legal 300", Legal, Portrait, 300, 2550, 4200},
		{"A3 300", A3, Portrait, 300, 3508, 4961},
		{"A5 300", A5, Portrait, 300, 1748, 2480},
	}
	for _, c := range cases {
		w, h := PagePixels(c.paper, c.o, c.dpi)
		if w != c.wh || h != c.ht {
			t.Errorf("%s: %d×%d, ждали %d×%d", c.name, w, h, c.wh, c.ht)
		}
	}
	if w, h := PagePixels(A4, Portrait, 0); w != 0 || h != 0 {
		t.Errorf("dpi=0 дал %d×%d", w, h)
	}
	if w, h := PagePixels(Paper{}, Portrait, 300); w != 0 || h != 0 {
		t.Errorf("пустая бумага дала %d×%d", w, h)
	}
}

// TestCustomPaperOrientation — своя бумага, заданная «вверх ногами», всё равно
// подчиняется ориентации: короткая сторона — ширина в книжной.
func TestCustomPaperOrientation(t *testing.T) {
	p := CustomPaper("", 200, 100)
	if p.ShortMM != 100 || p.LongMM != 200 {
		t.Fatalf("стороны %v×%v", p.ShortMM, p.LongMM)
	}
	if w, h := p.SizeMM(Portrait); w != 100 || h != 200 {
		t.Errorf("книжная: %v×%v", w, h)
	}
	if w, h := p.SizeMM(Landscape); w != 200 || h != 100 {
		t.Errorf("альбомная: %v×%v", w, h)
	}
	if p.Name == "" {
		t.Error("имя не придумано")
	}
}

func TestPWGName(t *testing.T) {
	if got := A4.PWGName(); got != "iso_a4_210x297mm" {
		t.Errorf("A4: %q", got)
	}
	if got := Letter.PWGName(); got != "na_letter_8.5x11in" {
		t.Errorf("Letter: %q", got)
	}
	if got := CustomPaper("Этикетка 5x3", 50, 30).PWGName(); got != "custom_5x3_30x50mm" {
		t.Errorf("своя с кириллицей: %q", got)
	}
	if got := CustomPaper("Только кириллица", 100, 150).PWGName(); !strings.HasPrefix(got, "custom_") ||
		strings.ContainsAny(got, " Ёё") || strings.Contains(got, "кирил") {
		t.Errorf("недопустимые символы в keyword: %q", got)
	}
	if got := CustomPaper("Ярлык", 100, 150).PWGName(); got != "custom_paper_100x150mm" {
		t.Errorf("полностью нелатинское имя: %q", got)
	}
}

func TestPageSetupDefaultsAndRects(t *testing.T) {
	s := NewPageSetup(A4)
	if s.EffectiveDPI() != DefaultDPI {
		t.Errorf("dpi по умолчанию %d", s.EffectiveDPI())
	}
	w, h := s.SheetSize()
	if w != 2480 || h != 3508 {
		t.Fatalf("лист %d×%d", w, h)
	}
	if b := s.Bounds(); b != image.Rect(0, 0, 2480, 3508) {
		t.Errorf("Bounds %v", b)
	}
	// Поля 20/10/10/10 мм при 300 dpi: 236/118/118/118 пикселей.
	if r := s.ContentRect(); r != image.Rect(236, 118, 2480-118, 3508-118) {
		t.Errorf("ContentRect %v", r)
	}
	if err := s.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
	pw, ph := s.PagePoints()
	if math.Abs(pw-595.276) > 0.01 || math.Abs(ph-841.89) > 0.01 {
		t.Errorf("пункты %v×%v", pw, ph)
	}
	// Альбомная меняет и лист, и пункты.
	s.Orientation = Landscape
	if w, h := s.SheetSize(); w != 3508 || h != 2480 {
		t.Errorf("альбомный лист %d×%d", w, h)
	}
}

func TestPageSetupValidate(t *testing.T) {
	bad := map[string]PageSetup{
		"нулевая бумага":      {},
		"огромная бумага":     {Paper: CustomPaper("x", 10, MaxPaperMM+1)},
		"NaN бумага":          {Paper: Paper{ShortMM: math.NaN(), LongMM: 100}},
		"dpi мал":             {Paper: A4, DPI: 10},
		"dpi велик":           {Paper: A4, DPI: 5000},
		"гигантская страница": {Paper: A3, DPI: 1200},
		"отрицательное поле":  {Paper: A4, Margins: Margins{Left: -1}},
		"бесконечное поле":    {Paper: A4, Margins: Margins{Top: math.Inf(1)}},
		"NaN поле":            {Paper: A4, Margins: Margins{Right: math.NaN()}},
		"поля съели страницу": {Paper: A4, Margins: UniformMargins(110)},
		"странная ориентация": {Paper: A4, Orientation: 7},
	}
	for name, s := range bad {
		err := s.Validate()
		if err == nil {
			t.Errorf("%s: ошибки нет", name)
		} else if !errors.Is(err, ErrBadSetup) {
			t.Errorf("%s: %v не ErrBadSetup", name, err)
		}
	}
	// Граница: A4 при 600 dpi (35 Мпикс) допустимо.
	if err := (PageSetup{Paper: A4, DPI: 600}).Validate(); err != nil {
		t.Errorf("A4@600: %v", err)
	}
}
