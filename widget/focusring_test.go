package widget

import (
	"image"
	"image/color"
	"testing"
)

// ─── Общая рамка фокуса ──────────────────────────────────────────────────────

// borderRec записывает контуры, нарисованные DrawBorder и DrawRoundBorder.
type borderRec struct {
	DrawContext
	rects []recBorder
}

type recBorder struct {
	r      image.Rectangle
	corner int
	col    color.RGBA
}

func (c *borderRec) FillRect(x, y, w, h int, col color.RGBA)         {}
func (c *borderRec) FillRoundRect(x, y, w, h, r int, col color.RGBA) {}
func (c *borderRec) DrawHLine(x, y, w int, col color.RGBA)           {}

func (c *borderRec) DrawBorder(x, y, w, h int, col color.RGBA) {
	c.rects = append(c.rects, recBorder{r: image.Rect(x, y, x+w, y+h), col: col})
}

func (c *borderRec) DrawRoundBorder(x, y, w, h, r int, col color.RGBA) {
	c.rects = append(c.rects, recBorder{r: image.Rect(x, y, x+w, y+h), corner: r, col: col})
}

// Рамка двухцветная: два наружных контура цвета текста, идущие внутрь, и
// внутренний контрастный; радиус уменьшается на каждом шаге.
func TestDrawFocusRing_TwoToneAndInset(t *testing.T) {
	rec := &borderRec{}
	r := image.Rect(10, 10, 110, 50)
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	DrawFocusRing(rec, r, 4, white)
	if len(rec.rects) != 3 {
		t.Fatalf("контуров %d, ожидалось 3 (два наружных и внутренний)", len(rec.rects))
	}
	for i, want := range []recBorder{
		{r: r, corner: 4, col: white},
		{r: r.Inset(1), corner: 3, col: white},
		{r: r.Inset(2), corner: 2, col: color.RGBA{A: 255}},
	} {
		if rec.rects[i] != want {
			t.Errorf("контур %d: %+v, ожидалось %+v", i, rec.rects[i], want)
		}
	}

	// На светлом тексте внутренний чёрный, на тёмном — белый: видно на любом фоне.
	rec = &borderRec{}
	DrawFocusRing(rec, r, 0, color.RGBA{A: 255})
	if got := rec.rects[len(rec.rects)-1].col; got != white {
		t.Errorf("внутренний контур при тёмном наружном: %+v", got)
	}
	if rec.rects[0].corner != 0 {
		t.Error("прямоугольная рамка получила скругление")
	}

	// Пустой прямоугольник и прозрачный цвет безопасны.
	rec = &borderRec{}
	DrawFocusRing(rec, image.Rectangle{}, 0, white)
	if len(rec.rects) != 0 {
		t.Error("рамка нарисована в пустом прямоугольнике")
	}
	DrawFocusRing(rec, r, 0, color.RGBA{})
	if rec.rects[0].col != white {
		t.Errorf("прозрачный цвет не заменён белым: %+v", rec.rects[0].col)
	}
}

// Тема задаёт толщину и цвет рамки токенами.
func TestDrawFocusRing_ThemeTokens(t *testing.T) {
	old := CurrentTheme()
	t.Cleanup(func() { ApplyGlobalTheme(old) })
	th := Win11LightTheme()
	th.Style.FocusRingWidth = 3
	th.Style.FocusRing = color.RGBA{R: 200, G: 30, B: 30, A: 255}
	ApplyGlobalTheme(th)

	rec := &borderRec{}
	DrawFocusRing(rec, image.Rect(0, 0, 60, 30), 0, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	if len(rec.rects) != 4 {
		t.Fatalf("контуров %d, ожидалось 4 (три наружных и внутренний)", len(rec.rects))
	}
	for i := 0; i < 3; i++ {
		if rec.rects[i].col != th.Style.FocusRing {
			t.Errorf("контур %d цвета %+v: токен темы не применён", i, rec.rects[i].col)
		}
	}
}

// Кнопка диалога с FocusRing рисует общую рамку в фокусе и не рисует без него;
// обычная кнопка — как прежде, рамкой акцента.
func TestButton_FocusRingOnlyInDialogs(t *testing.T) {
	old := CurrentTheme()
	t.Cleanup(func() { ApplyGlobalTheme(old) })
	ApplyGlobalTheme(Win11LightTheme()) // не классика: у неё фокус — пунктир
	ring := trBtn("dlg.ok", true)
	if !ring.FocusRing {
		t.Fatal("кнопка диалога без FocusRing")
	}
	plain := NewButton("Обычная")
	if plain.FocusRing {
		t.Fatal("обычная кнопка не должна менять вид фокуса")
	}
	b := image.Rect(0, 0, 100, 30)
	for _, tc := range []struct {
		name     string
		btn      *Button
		focused  bool
		wantRing bool
	}{
		{"диалог, в фокусе", ring, true, true},
		{"диалог, без фокуса", ring, false, false},
		{"обычная, в фокусе", plain, true, false},
	} {
		tc.btn.SetFocused(tc.focused)
		rec := &borderRec{}
		tc.btn.drawChrome(rec, b, tc.btn.Background, tc.btn.BorderColor, false)
		hasRing := false
		for _, r := range rec.rects {
			if r.r == b.Inset(1) || r.r == b.Inset(2) {
				hasRing = true
			}
		}
		if hasRing != tc.wantRing {
			t.Errorf("%s: общая рамка нарисована=%v, ожидалось %v (%d контуров)",
				tc.name, hasRing, tc.wantRing, len(rec.rects))
		}
	}
}

// Вид рамки, объявленный в ThemeStyle, переживает путь «плоская тема → профиль
// → плоская тема», а тема без объявлений остаётся прежней.
func TestFocusRing_RoundTripThroughProfile(t *testing.T) {
	src := Win10DarkTheme()
	src.Style.FocusRingWidth = 3
	src.Style.FocusRing = color.RGBA{R: 10, G: 20, B: 30, A: 255}
	got := Materialize(themeResolved(t, ProfileFromTheme(src))).Style
	if got.FocusRingWidth != 3 || got.FocusRing != src.Style.FocusRing {
		t.Errorf("рамка фокуса потеряна в мосте: толщина %d, цвет %+v", got.FocusRingWidth, got.FocusRing)
	}
	for _, name := range ThemeNames() {
		st := Materialize(themeResolved(t, ProfileFromTheme(ThemeByName(name)))).Style
		if st.FocusRingWidth != 0 || st.FocusRing.A != 0 {
			t.Errorf("%s: пресет без объявлений получил рамку фокуса %d/%+v", name, st.FocusRingWidth, st.FocusRing)
		}
	}
}

// Все кнопки диалогов движка рисуют фокус общей рамкой: MessageBox, диалог
// подтверждения, диалог ввода.
func TestDialogButtons_UseSharedFocusRing(t *testing.T) {
	sh := &fakeShower{}
	mb := NewMessageBox(sh)
	for name, dlg := range map[string]*Dialog{
		"MessageBox":    mb.ShowYesNoCancel("Вопрос", "Сохранить?", nil),
		"Подтверждение": NewConfirmDialog("Подтверждение", "Удалить?", nil),
	} {
		n := 0
		for _, c := range dlg.Children() {
			if b, ok := c.(*Button); ok {
				n++
				if !b.FocusRing {
					t.Errorf("%s: кнопка %q без общей рамки фокуса", name, b.Text)
				}
			}
		}
		if n == 0 {
			t.Errorf("%s: кнопок в диалоге не найдено", name)
		}
	}
}
