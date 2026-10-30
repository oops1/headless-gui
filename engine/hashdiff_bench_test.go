package engine

// hashdiff_bench_test.go — хэш-дифф тайлов (ADR-0006, приёмка 3): пары
// «hash/full» в одном запуске. Кадр без изменений вне on-demand, Invalidate
// корня с правкой одного лейбла, рабочий стол без изменений, большая сцена.

import (
	"fmt"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

func benchHashPair(b *testing.B, run func(b *testing.B, hash bool)) {
	b.Run("hash", func(b *testing.B) { run(b, true) })
	b.Run("full", func(b *testing.B) { run(b, false) })
}

// BenchmarkHash_NoChange — кадр без изменений вне on-demand: запись дерева,
// хэши тайлов — и ни растра, ни диффа.
func BenchmarkHash_NoChange(b *testing.B) {
	benchHashPair(b, func(b *testing.B, hash bool) {
		eng, _ := benchTextScene(1280, 800)
		defer eng.Stop()
		eng.SetHashDiff(hash)
		eng.renderFrame()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eng.renderFrame()
		}
	})
}

// BenchmarkHash_RootInvalidateLabel — Invalidate корня (все 180 лейблов
// перезаписываются) при правке одного: рисуются только его тайлы.
func BenchmarkHash_RootInvalidateLabel(b *testing.B) {
	benchHashPair(b, func(b *testing.B, hash bool) {
		eng, root := benchTextScene(1280, 800)
		defer eng.Stop()
		eng.SetHashDiff(hash)
		eng.SetRenderOnDemand(true)
		var ls []*widget.Label
		labelsOf(root, &ls)
		eng.renderFrame()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ls[77].SetText(fmt.Sprintf("значение %d", i&1))
			invalidateW(root)
			eng.renderFrame()
		}
	})
}

// BenchmarkHash_DesktopNoChange — рабочий стол (с размытием панели) без
// изменений вне on-demand.
func BenchmarkHash_DesktopNoChange(b *testing.B) {
	benchHashPair(b, func(b *testing.B, hash bool) {
		sc := buildPipelineDesktop()
		defer sc.closeAll()
		defer sc.eng.Stop()
		eng := sc.eng
		eng.SetHashDiff(hash)
		eng.SetRenderOnDemand(false)
		eng.renderFrame()
		eng.renderFrame()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eng.renderFrame()
		}
	})
}

// BenchmarkHash_BigNoChange — сцена на 21 тыс. команд без изменений.
func BenchmarkHash_BigNoChange(b *testing.B) {
	benchHashPair(b, func(b *testing.B, hash bool) {
		eng, _ := benchBigScene()
		defer eng.Stop()
		eng.SetHashDiff(hash)
		eng.renderFrame()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eng.renderFrame()
		}
	})
}
