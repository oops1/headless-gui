package engine

// workpool.go — обёртки пула воркеров процесса (internal/workpool) для заданий
// движка; задания холста — в canvas_jobs.go.

import (
	"github.com/oops1/headless-gui/v4/internal/workpool"
)

// poolBatch — одна раздача частей (workpool.Batch).
type poolBatch = workpool.Batch

// poolRun выполняет части 0..parts-1 в пуле (workpool.Run).
func poolRun(t workpool.Task, b *poolBatch, parts int) { workpool.Run(t, b, parts) }

// startWorkPool запускает воркеры пула заранее (workpool.Start).
func startWorkPool() { workpool.Start() }
