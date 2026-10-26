package engine

// workpool.go — постоянный пул воркеров для параллельных частей кадра.
//
// Дифф тайлов и большой непрозрачный блит делились на полосы, и на каждую
// полосу каждого кадра заводилась горутина: на 20 процессорах это 20
// созданий и 20 завершений на кадр, плюс замыкание и срез результатов. Пул
// заводит воркеры один раз и раздаёт им полосы через канал.
//
// Пул один на процесс, а не на движок: движков в процессе бывает много (окно
// на монитор, hosted-диалоги, сессии WinLine), а процессоров столько же,
// сколько было. Он стартует лениво, при первой параллельной работе, и живёт
// до конца процесса: остановка движка его не касается, следующий движок
// находит его готовым.
//
// Правила, на которых держится отсутствие взаимной блокировки:
//
//   - часть 0 всегда выполняет вызывающая горутина, поэтому работа идёт, даже
//     когда все воркеры заняты чужими кадрами;
//   - задание пула не ставит в пул новых заданий и ничего не ждёт: дифф и
//     блит полосы — чистая работа над памятью холста.
//
// Паника в части не должна ни уронить процесс из воркера, где её никто не
// поймает, ни оставить другие части писать в холст после того, как
// вызывающий ушёл: она перехватывается, вызывающий дожидается всех частей и
// поднимает панику у себя — там её ловит политика сбоев (failure.go).

import (
	"image"
	"runtime"
	"sync"

	"github.com/oops1/headless-gui/v3/output"
)

// poolTask — работа, разбитая на части; runPart(i) выполняет часть i.
// Реализации — поля холста, переиспользуемые от кадра к кадру: передача
// указателя на них в канал ничего не аллоцирует.
type poolTask interface {
	runPart(i int)
}

// poolBatch — одна раздача частей: ожидание и паника, если была.
type poolBatch struct {
	wg       sync.WaitGroup
	mu       sync.Mutex
	panicked any
}

func (b *poolBatch) setPanic(r any) {
	b.mu.Lock()
	if b.panicked == nil {
		b.panicked = r
	}
	b.mu.Unlock()
}

func (b *poolBatch) takePanic() any {
	b.mu.Lock()
	r := b.panicked
	b.panicked = nil
	b.mu.Unlock()
	return r
}

type poolJob struct {
	t     poolTask
	part  int
	batch *poolBatch
}

var (
	workPoolOnce sync.Once
	workPoolJobs chan poolJob
)

// startWorkPool запускает воркеры, если они ещё не запущены. По одному на
// процессор на момент запуска: часть 0 делает вызывающий, и запас нужен,
// когда параллельно рисуют несколько движков.
func startWorkPool() {
	workPoolOnce.Do(func() {
		n := runtime.GOMAXPROCS(0)
		workPoolJobs = make(chan poolJob, 2*n)
		for i := 0; i < n; i++ {
			go workPoolWorker()
		}
	})
}

func workPoolWorker() {
	for j := range workPoolJobs {
		runPoolJob(j)
	}
}

// runPoolJob — одна часть в воркере. Отдельной функцией, чтобы defer был
// открытым (в теле цикла он аллоцировал бы запись на каждое задание).
func runPoolJob(j poolJob) {
	defer j.batch.wg.Done()
	runPoolPart(j.t, j.part, j.batch)
}

// runPoolPart выполняет часть, перехватывая панику в batch.
func runPoolPart(t poolTask, part int, b *poolBatch) {
	defer func() {
		if r := recover(); r != nil {
			b.setPanic(r)
		}
	}()
	t.runPart(part)
}

// poolRun выполняет части 0..parts-1: часть 0 — в вызывающей горутине,
// остальные — в пуле. Возвращается, когда выполнены все. Паника в любой
// части поднимается здесь, после завершения всех частей.
//
// b принадлежит вызывающему и не должен использоваться двумя poolRun сразу;
// у холста он свой на каждый вид работы.
func poolRun(t poolTask, b *poolBatch, parts int) {
	if parts > 1 {
		startWorkPool()
		b.wg.Add(parts - 1)
		for i := 1; i < parts; i++ {
			workPoolJobs <- poolJob{t: t, part: i, batch: b}
		}
	}
	runPoolPart(t, 0, b)
	b.wg.Wait()
	if r := b.takePanic(); r != nil {
		panic(r)
	}
}

// ─── Задания холста ──────────────────────────────────────────────────────────

// diffJob — дифф тайлов по полосам рядов [ty0..ty1]: часть i — ряды
// [ty0+i*chunk .. ty0+(i+1)*chunk-1]. Полосы не пересекаются по памяти —
// сравнение, извлечение и синхронизация front каждая делает в своих рядах.
type diffJob struct {
	c                  *Canvas
	tx0, tx1, ty0, ty1 int
	chunk              int
	results            [][]output.DirtyTile
	batch              poolBatch
}

func (j *diffJob) runPart(i int) {
	r0 := j.ty0 + i*j.chunk
	r1 := min(r0+j.chunk-1, j.ty1)
	if r0 > r1 {
		j.results[i] = nil
		return
	}
	j.results[i] = j.c.diffTileRows(j.tx0, r0, j.tx1, r1)
}

// blitJob — непрозрачный блит по горизонтальным полосам: часть i — строки
// [i*step .. (i+1)*step) прямоугольника r. Полосы не пересекаются по строкам
// back.
type blitJob struct {
	c      *Canvas
	r      image.Rectangle
	src    image.Image
	offset image.Point
	step   int
	batch  poolBatch
}

func (j *blitJob) runPart(i int) {
	h := j.r.Dy()
	y := i * j.step
	if y >= h {
		return
	}
	y1 := min(y+j.step, h)
	band := image.Rect(j.r.Min.X, j.r.Min.Y+y, j.r.Max.X, j.r.Min.Y+y1)
	j.c.blitSrcBand(band, j.src, j.offset.Add(image.Pt(0, y)))
}
