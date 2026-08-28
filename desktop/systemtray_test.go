package desktop

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// trayTrio — контейнер с тремя значками состояния на тестовой теме.
func trayTrio(t *testing.T) (*SystemTray, *FakeSystemStatus) {
	t.Helper()
	tm := testThemeManager(t)
	st := NewFakeSystemStatus()
	tray := NewSystemTray(tm)
	tray.AddItem(NewNetworkStatus(tm, st))
	tray.AddItem(NewVolumeStatus(tm, st))
	tray.AddItem(NewPowerStatus(tm, st))
	return tray, st
}

// Места хватает — прячется ничего и шеврона нет.
func TestSystemTray_AllFitNoChevron(t *testing.T) {
	tray, _ := trayTrio(t)
	defer tray.Close()

	tray.SetBounds(image.Rect(0, 0, 400, 24))
	if got := len(tray.Hidden()); got != 0 {
		t.Errorf("спрятано %d значков там, где места вдоволь", got)
	}
	for i, it := range tray.Items() {
		if it.Bounds().Empty() {
			t.Errorf("значок %d не получил места", i)
		}
	}
}

// Места мало — лишние прячутся, и появляется шеврон.
func TestSystemTray_OverflowHidesExtras(t *testing.T) {
	tray, _ := trayTrio(t)
	defer tray.Close()

	// Ширины хватает от силы на один значок с кнопкой раскрытия.
	tray.SetBounds(image.Rect(0, 0, 30, 24))
	hidden := tray.Hidden()
	if len(hidden) == 0 {
		t.Fatal("в тесную панель влезли все значки — переполнение не сработало")
	}
	for _, it := range hidden {
		if !it.Bounds().Empty() {
			t.Error("спрятанный значок сохранил границы — он поймает мышь вслепую")
		}
	}

	// Щелчок по шеврону раскрывает область.
	if tray.Overflow().IsOpen() {
		t.Fatal("область скрытых значков открыта до щелчка")
	}
	tray.Overflow().Screen = image.Rect(0, 0, 200, 200)
	press := widget.MouseEvent{X: 28, Y: 12, Button: widget.MouseLeft, Pressed: true}
	if !tray.OnMouseButton(press) {
		t.Fatal("щелчок по шеврону не поглощён")
	}
	if !tray.Overflow().IsOpen() {
		t.Error("щелчок по шеврону не раскрыл область")
	}
	if tray.Overflow().OverlayBounds().Empty() {
		t.Error("раскрытая область вышла нулевой")
	}

	// Повторный щелчок — сворачивает.
	tray.OnMouseButton(press)
	if tray.Overflow().IsOpen() {
		t.Error("повторный щелчок не свернул область")
	}
}

// Щелчок мимо шеврона контейнер не трогает: он должен дойти до значка.
func TestSystemTray_ClickOutsideChevronPassesThrough(t *testing.T) {
	tray, _ := trayTrio(t)
	defer tray.Close()
	tray.SetBounds(image.Rect(0, 0, 30, 24))

	if tray.OnMouseButton(widget.MouseEvent{X: 2, Y: 12, Button: widget.MouseLeft, Pressed: true}) {
		t.Error("контейнер поглотил щелчок по значку, а не по шеврону")
	}
}

// Ширина запрашивается по значкам, а не берётся с потолка.
func TestSystemTray_PreferredSizeFollowsItems(t *testing.T) {
	tray, _ := trayTrio(t)
	defer tray.Close()

	avail := image.Pt(400, 24)
	want := 0
	for i, it := range tray.Items() {
		if i > 0 {
			want += int(testThemeManager(t).GetMetric(KeyTrayGap))
		}
		want += it.PreferredSize(avail).X
	}
	if got := tray.PreferredSize(avail).X; got != want {
		t.Errorf("PreferredSize = %d, ждали %d (сумма значков и зазоров)", got, want)
	}
}
