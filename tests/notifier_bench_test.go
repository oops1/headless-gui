package tests

// notifier_bench_test.go — стоимость широковещания уведомлений об изменении UI
// по 1/2/3 приёмникам (движкам). Проверяет, что переход с «последний
// выигрывает» на broadcast-реестр не даёт заметной просадки на реальном числе
// живых движков (единицы).

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// benchRectReceiver — дешёвый no-op приёмник точечной инвалидации: имитирует
// engine.InvalidateRect без реальной работы, чтобы мерить именно накладные
// расходы обхода реестра и диспетчеризации.
func benchRectReceiver(image.Rectangle) {}

func benchmarkBroadcast(b *testing.B, receivers int) {
	handles := make([]uint64, 0, receivers)
	for i := 0; i < receivers; i++ {
		handles = append(handles, widget.RegisterUINotifier(nil, benchRectReceiver))
	}
	defer func() {
		for _, h := range handles {
			widget.UnregisterUINotifier(h)
		}
	}()

	// Виджет с непустыми bounds: Invalidate() → notifyRectChanged(bounds) →
	// широковещание по всем зарегистрированным приёмникам.
	lbl := widget.NewLabel("bench", color.RGBA{R: 200, G: 200, B: 200, A: 255})
	lbl.SetBounds(image.Rect(0, 0, 120, 24))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lbl.Invalidate()
	}
}

func BenchmarkBroadcastNotify_1(b *testing.B) { benchmarkBroadcast(b, 1) }
func BenchmarkBroadcastNotify_2(b *testing.B) { benchmarkBroadcast(b, 2) }
func BenchmarkBroadcastNotify_3(b *testing.B) { benchmarkBroadcast(b, 3) }
