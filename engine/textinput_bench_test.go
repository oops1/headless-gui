package engine

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// textinput_bench_test.go — стоимость кадра поля ввода (PERF-11).
// Живёт в пакете engine, потому что нужен НАСТОЯЩИЙ канвас с растеризацией
// текста: замер с виджет-заглушкой ничего не сказал бы про измерение рун.

// benchTextInput готовит канвас и поле ввода с текстом в фокусе.
func benchTextInput(b *testing.B) (*Canvas, *widget.TextInput) {
	b.Helper()
	eng := New(640, 480, 20)
	ti := widget.NewTextInput("подсказка")
	ti.SetBounds(image.Rect(20, 40, 320, 68))
	ti.SetText("Некоторый текст в поле ввода 0123456789")
	ti.SetFocused(true)
	return eng.canvas, ti
}

// BenchmarkTextInputDrawCached — установившийся режим: текст не менялся,
// раскладка берётся из кэша (поведение ПОСЛЕ PERF-11).
func BenchmarkTextInputDrawCached(b *testing.B) {
	c, ti := benchTextInput(b)
	ti.Draw(c) // прогрев кэша
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ti.Draw(c)
	}
}

// BenchmarkTextInputDrawUncached — тот же кадр, но кэш раскладки сбрасывается
// перед каждым Draw (ревизией метрик): позиции всех рун меряются заново.
// Это поведение ДО PERF-11, когда полный layout считался на каждом кадре.
func BenchmarkTextInputDrawUncached(b *testing.B) {
	c, ti := benchTextInput(b)
	ti.Draw(c)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		widget.BumpTextMetricsRev()
		ti.Draw(c)
	}
}

// BenchmarkTextInputDrawPasswordCached — режим пароля: прежний Draw собирал
// строку маски (и открытый текст!) на каждом кадре.
func BenchmarkTextInputDrawPasswordCached(b *testing.B) {
	c, ti := benchTextInput(b)
	ti.SetPasswordMode(true)
	ti.Draw(c)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ti.Draw(c)
	}
}
