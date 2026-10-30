package desktop

// Значок звука без звукового устройства: подсказка «нет устройства», а не
// «0%» (замечание WinLine после v3.32.0, п.6).

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/theme"
)

func TestVolumeItem_NoDeviceToolTipAndGlyph(t *testing.T) {
	testreset.Globals(t)
	tm := testThemeManager(t)
	st := NewFakeSystemStatus()
	it := NewVolumeStatus(tm, st)
	defer it.Close()
	it.SetBounds(image.Rect(0, 0, 16, 16))

	// Устройства нет: подсказка не «0%», значок перечёркнут, как при Muted.
	st.SetVolume(VolState{NoDevice: true})
	if got, want := it.GetToolTip(), "Звук: нет устройства"; got != want {
		t.Errorf("GetToolTip() без устройства = %q, ждали %q", got, want)
	}
	noDev, muted, level := &recPixel{}, &recPixel{}, &recPixel{}
	it.Draw(noDev)
	st.SetVolume(VolState{Level: 0.5, Muted: true})
	it.Draw(muted)
	st.SetVolume(VolState{Level: 0})
	it.Draw(level)
	if len(noDev.pixels) == 0 {
		t.Error("без устройства значок не перечёркнут")
	}
	if len(noDev.pixels) != len(muted.pixels) || len(noDev.fills) != len(muted.fills) {
		t.Errorf("без устройства значок должен совпасть с выключенным: %d/%d пикселей, %d/%d заливок",
			len(noDev.pixels), len(muted.pixels), len(noDev.fills), len(muted.fills))
	}
	// Тишина на живом устройстве по-прежнему «0%», шкала без перечёркивания.
	if got, want := volumeTooltip(VolState{Level: 0}), "Звук: 0%"; got != want {
		t.Errorf("уровень 0 при устройстве: %q, ждали %q", got, want)
	}
	if len(level.pixels) != 0 {
		t.Error("тишина на живом устройстве перечёркнута")
	}

	// Выключен и без устройства одновременно — говорит «нет устройства».
	st.SetVolume(VolState{Muted: true, NoDevice: true})
	if got, want := it.GetToolTip(), "Звук: нет устройства"; got != want {
		t.Errorf("Muted без устройства: %q, ждали %q", got, want)
	}

	// Устройство вернулось.
	st.SetVolume(VolState{Level: 0.4})
	if got, want := it.GetToolTip(), "Звук: 40%"; got != want {
		t.Errorf("после возвращения устройства %q, ждали %q", got, want)
	}
}

// Значок без источника показаний вовсе — тоже «нет устройства».
func TestVolumeItem_NilStatusIsNoDevice(t *testing.T) {
	testreset.Globals(t)
	it := NewVolumeStatus(testThemeManager(t), nil)
	defer it.Close()
	if got, want := it.GetToolTip(), "Звук: нет устройства"; got != want {
		t.Errorf("GetToolTip() без источника = %q, ждали %q", got, want)
	}
}

func TestVolumeItem_NoDeviceToolTipEnglish(t *testing.T) {
	testreset.Globals(t)
	useLanguage(t, "EN")
	if got, want := volumeTooltip(VolState{NoDevice: true}), "Sound: no device"; got != want {
		t.Errorf("EN: %q, ждали %q", got, want)
	}
	if got, want := volumeTooltip(VolState{Muted: true}), "Sound: muted"; got != want {
		t.Errorf("EN muted: %q, ждали %q", got, want)
	}
}

// Иконка из набора темы: своя «tray.volume.nodevice», а без неё — «выключено».
func TestVolumeIconKeys_NoDevice(t *testing.T) {
	testreset.Globals(t)
	got := volumeIconKeys(VolState{NoDevice: true})
	want := []theme.Key{"tray.volume.nodevice", "tray.volume.muted"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("ключи иконки без устройства %v, ждали %v", got, want)
	}
}

// Плитка звука в быстрых настройках — не «включённая» без устройства.
func TestQuickSettings_VolumeTileInactiveWithoutDevice(t *testing.T) {
	testreset.Globals(t)
	st := NewFakeSystemStatus()
	q := NewQuickSettings(testThemeManager(t), st)
	defer q.Close()
	if !q.volumeActive() {
		t.Fatal("плитка звука с устройством должна быть включённой")
	}
	st.SetVolume(VolState{NoDevice: true})
	if q.volumeActive() {
		t.Error("плитка звука без устройства осталась включённой")
	}
}
