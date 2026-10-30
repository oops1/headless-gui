package base

// owner_services_review_test.go — случаи из ревью 5.2 (службы владельца) на
// поддельном владельце: меню строки меню через хозяина, кэши раскладки поля
// ввода и строки меню при смене владельца.

import (
	"image"
	"image/color"
	"testing"
)

// zzOwner — поддельный владелец с измерителем «k px на руну».
type zzOwner struct {
	set *MeasureSet
}

func (*zzOwner) TreeOwner() {}

func (o *zzOwner) OwnerMeasurers() *MeasureSet { return o.set }

func newZZOwner(k int) *zzOwner {
	text := func(s string, _ float64) int { return k * len([]rune(s)) }
	return &zzOwner{set: NewMeasureSet(Measurers{
		Text:     text,
		TextFont: func(s string, pt float64, _ string) int { return text(s, pt) },
	})}
}

func zzRoot(o Owner) *Panel {
	root := NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 800, 600))
	BindOwner(root, o)
	return root
}

// Меню строки меню владельца не имеет: ширина считается глобальным
// измерителем, а строка меню и отрисовка — владельцем.
func TestOwnerServicesReview_MenuBarPopupMeasuresWithOwner(t *testing.T) {
	useTestMeasurer(t)
	o := newZZOwner(40)
	root := zzRoot(o)
	mb := NewMenuBar()
	mb.SetBounds(image.Rect(0, 0, 800, 24))
	mb.AddMenu("Файл", MenuItem{Text: "Открыть документ"})
	root.AddChild(mb)

	if OwnerOf(mb) != Owner(o) {
		t.Fatal("строка меню не в дереве владельца")
	}
	// Меню — не ребёнок строки: владельца находит через хозяина (SetHost).
	if got := OwnerOf(mb.popup.ownerWidget()); got != Owner(o) {
		t.Errorf("владелец меню строки меню через хозяина = %v, ждали владельца строки", got)
	}
	mb.popup.SetItems([]MenuItem{{Text: "Открыть документ"}})
	mb.popup.mu.RLock()
	w, _ := mb.popup.calcSize()
	mb.popup.mu.RUnlock()
	ownerW := 40 * len([]rune("Открыть документ"))
	if w < ownerW {
		t.Errorf("ширина меню %d меньше подписи, измеренной владельцем (%d): подпись вылезет за меню", w, ownerW)
	}
}

// Контрольная: раскладка строки меню и вкладок действительно идёт владельцем.
func TestOwnerServicesReview_MenuBarRectsUseOwner(t *testing.T) {
	useTestMeasurer(t)
	o := newZZOwner(40)
	root := zzRoot(o)
	mb := NewMenuBar()
	mb.SetBounds(image.Rect(0, 0, 800, 24))
	mb.AddMenu("Файл")
	root.AddChild(mb)
	mb.recalcRects()
	mb.mu.RLock()
	r := mb.itemRects[0]
	mb.mu.RUnlock()
	if r.Dx() < 40*4 {
		t.Errorf("пункт строки меню %d — не владельцем", r.Dx())
	}
}
