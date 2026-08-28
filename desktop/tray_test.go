package desktop

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// ─── Сеть ────────────────────────────────────────────────────────────────────

func TestNetworkItem_PreferredSizeFromTheme(t *testing.T) {
	tm := testThemeManager(t)
	st := NewFakeSystemStatus()
	it := NewNetworkStatus(tm, st)
	defer it.Close()

	// Значок — квадрат KeyTrayIconSize, ширина запрашивается с отступами
	// стиля по обе стороны: вплотную значки трея не стоят.
	pad := int(tm.GetStyle(ComponentNetwork, "", 0).PadX)
	got := it.PreferredSize(image.Pt(999, 999))
	if got.X != 16+2*pad || got.Y != 16 {
		t.Errorf("PreferredSize = %v, ждали %dx16 (KeyTrayIconSize и отступы из тестовой темы)",
			got, 16+2*pad)
	}
}

func TestNetworkItem_ClickFiresOnRelease(t *testing.T) {
	tm := testThemeManager(t)
	st := NewFakeSystemStatus()
	it := NewNetworkStatus(tm, st)
	defer it.Close()
	it.SetBounds(image.Rect(0, 0, 16, 16))

	clicked := 0
	it.OnClick = func() { clicked++ }

	it.OnMouseButton(widget.MouseEvent{X: 5, Y: 5, Button: widget.MouseLeft, Pressed: true})
	if clicked != 0 {
		t.Fatal("клик сработал на нажатии, а не на отпускании")
	}
	it.OnMouseButton(widget.MouseEvent{X: 5, Y: 5, Button: widget.MouseLeft, Pressed: false})
	if clicked != 1 {
		t.Errorf("клик не сработал на отпускании внутри границ: clicked=%d", clicked)
	}
}

// ─── Звук ────────────────────────────────────────────────────────────────────

// TestVolumeItem_MutedDrawsDifferently — при Muted шкала громкости не
// рисуется, а на её месте перечёркивание: число закрашенных прямоугольников
// должно отличаться от «нормального» состояния.
func TestVolumeItem_MutedDrawsDifferently(t *testing.T) {
	tm := testThemeManager(t)
	st := NewFakeSystemStatus()
	it := NewVolumeStatus(tm, st)
	defer it.Close()
	it.SetBounds(image.Rect(0, 0, 16, 16))

	st.SetVolume(VolState{Level: 0.7, Muted: false})
	unmuted := &recCtx{}
	it.Draw(unmuted)

	st.SetVolume(VolState{Level: 0.7, Muted: true})
	muted := &recCtx{}
	it.Draw(muted)

	if len(unmuted.fills) == 0 || len(muted.fills) == 0 {
		t.Fatalf("ждали хоть какую-то заливку в обоих состояниях: unmuted=%d muted=%d",
			len(unmuted.fills), len(muted.fills))
	}
	if len(muted.fills) == len(unmuted.fills) {
		t.Errorf("Muted и обычное состояние нарисовали одинаковое число прямоугольников: %d",
			len(muted.fills))
	}
}

func TestVolumeItem_StatusChangeInvalidates(t *testing.T) {
	tm := testThemeManager(t)
	st := NewFakeSystemStatus()
	it := NewVolumeStatus(tm, st)
	defer it.Close()
	it.SetBounds(image.Rect(0, 0, 16, 16))

	var rectCalls int
	handle := widget.RegisterUINotifier(nil, func(image.Rectangle) { rectCalls++ })
	defer widget.UnregisterUINotifier(handle)

	st.SetVolume(VolState{Level: 0.3})
	if rectCalls == 0 {
		t.Error("изменение SystemStatus не вызвало перерисовку значка")
	}
}

func TestVolumeItem_CloseUnsubscribes(t *testing.T) {
	tm := testThemeManager(t)
	st := NewFakeSystemStatus()
	it := NewVolumeStatus(tm, st)
	it.SetBounds(image.Rect(0, 0, 16, 16))
	it.Close()

	var rectCalls int
	handle := widget.RegisterUINotifier(nil, func(image.Rectangle) { rectCalls++ })
	defer widget.UnregisterUINotifier(handle)

	st.SetVolume(VolState{Level: 0.9})
	if rectCalls != 0 {
		t.Errorf("после Close изменение статуса всё ещё вызывает Invalidate: %d уведомлений", rectCalls)
	}
}

// ─── Питание ─────────────────────────────────────────────────────────────────

func TestPowerItem_NoBatteryZeroWidth(t *testing.T) {
	tm := testThemeManager(t)
	st := NewFakeSystemStatus()
	st.SetPower(PowerState{NoBattery: true})
	it := NewPowerStatus(tm, st)
	defer it.Close()

	got := it.PreferredSize(image.Pt(999, 999))
	if got.X != 0 {
		t.Errorf("PreferredSize.X = %d, ждали 0 при NoBattery — значок вообще не показывается", got.X)
	}
}

func TestPowerItem_HasWidthWithBattery(t *testing.T) {
	tm := testThemeManager(t)
	st := NewFakeSystemStatus()
	st.SetPower(PowerState{Charge: 0.5, NoBattery: false})
	it := NewPowerStatus(tm, st)
	defer it.Close()

	got := it.PreferredSize(image.Pt(999, 999))
	if got.X == 0 {
		t.Error("PreferredSize.X = 0 при наличии батареи — значок должен показываться")
	}
}

func TestPowerItem_DoesNotDrawWithoutBattery(t *testing.T) {
	tm := testThemeManager(t)
	st := NewFakeSystemStatus()
	st.SetPower(PowerState{NoBattery: true})
	it := NewPowerStatus(tm, st)
	defer it.Close()
	// Даже если бы кто-то всё же выставил границы, рисовать нечего.
	it.SetBounds(image.Rect(0, 0, 16, 16))

	ctx := &recCtx{}
	it.Draw(ctx)
	if len(ctx.fills) != 0 {
		t.Errorf("NoBattery всё равно что-то нарисовал: %d заливок", len(ctx.fills))
	}
}
