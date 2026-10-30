package engine

// stats.go — счётчики работы движка.
//
// Числа кадра до сих пор жили только в бенчмарках, а в рабочей программе
// заказчику приходилось ставить замеры у себя и угадывать, что из времени
// кадра пришлось на движок. Здесь — то, что движок знает о себе сам: сколько
// кадров нарисовано и пропущено, сколько тайлов отдано, на что ушло время.
//
// Счётчики атомарные и только растут: снимок читается без замков из любой
// горутины, а разность двух снимков даёт числа за промежуток.
//
// Время меряется time.Now. На Windows эти часы идут шагами порядка половины
// миллисекунды, и отдельный короткий кадр может записаться нулём, а соседний —
// целым шагом. Сумма за сотни кадров от этого верна в среднем: делите её на
// число кадров, а не смотрите на один. Цена в горячем
// пути — три вызова time.Now и несколько атомарных сложений на кадр; кадр
// стоит сотни микросекунд, так что это меньше процента даже на самом дешёвом.

import (
	"sync/atomic"
	"time"
)

// Stats — снимок счётчиков движка (Engine.Stats). Все поля, кроме PostQueue,
// накоплены с создания движка и только растут: числа за промежуток — разность
// двух снимков.
type Stats struct {
	// Frames — сколько раз движок готовил кадр (цикл, RenderOnce,
	// RenderFrameNow), включая кадры, в которых ничего не изменилось.
	Frames uint64
	// FramesSkipped — сколько тиков цикла кадров прошло без кадра: UI не
	// менялся, анимаций нет. Пробуждения по Post (ввод окна), после которых
	// рисовать нечего, сюда не входят: они случаются на каждое событие и
	// смысла отношения «кадры к тикам» не меняют.
	FramesSkipped uint64
	// FramesLost — сколько кадров выброшено из переполненного канала Frames:
	// потребитель не успевал. Сток (SetFrameSink) кадров не теряет.
	FramesLost uint64
	// Tiles — сколько изменившихся тайлов отдано в кадрах.
	Tiles uint64
	// Moves — сколько переносов (MoveRegion) отдано в кадрах.
	Moves uint64
	// DrawNanos — время отрисовки: фон, дерево, оверлеи, модалки, попапы,
	// подсказка, применение переносов.
	DrawNanos uint64
	// DiffNanos — время сравнения с прошлым кадром и описания тайлов. При
	// включённом SaveFrames сюда входит и снимок кадра для записи.
	DiffNanos uint64
	// DeliverNanos — время отдачи кадров стоку и в канал. У синхронного стока
	// сюда входит его собственная работа.
	DeliverNanos uint64
	// Posted — сколько функций поставлено в очередь Post.
	Posted uint64
	// PostedRun — сколько из них выполнено.
	PostedRun uint64
	// PostQueue — сколько функций ждёт выполнения сейчас (Posted − PostedRun).
	PostQueue uint64
	// Panics — сколько паник поймано политикой сбоев (failure.go).
	Panics uint64
	// Recorded — сколько команд рисования записало дерево виджетов (путь
	// команд, cmdpath.go): Draw звался только у тех, чьи списки устарели.
	Recorded uint64
	// Retained — сколько команд верхнего уровня удержанных списков легло в
	// кадр ссылкой вместо Draw (списки детей под ссылкой не пересчитываются;
	// чистые дети удержанного контейнера сюда не входят — их не посещают).
	// Нулевой Retained при ненулевом Recorded — удержание не работает.
	Retained uint64
	// TiledFrames — сколько кадров растрилось по тайлам в пуле (tileframe.go);
	// остальные кадры пути команд воспроизводились в холст движка напрямую.
	TiledFrames uint64
	// BlurHits, BlurMisses — попадания и промахи кэша размытой подложки:
	// промах рисует слой ореолом и заполняет кэш.
	BlurHits, BlurMisses uint64
	// TilesSkipped — тайлы, которые хэш-дифф (hashdiff.go) не рисовал и не
	// сравнивал: их команды не менялись с кадра, когда их рисовали.
	TilesSkipped uint64
	// BufferBytes — байт в собственных буферах кадра холста после последнего
	// кадра: front и окно back (ADR-0007). Не накопительный: текущее
	// значение. Окна воркеров, кэш размытия и буферы потребителя сюда не
	// входят.
	BufferBytes uint64
	// PopupBytes — байт памяти хостовых попапов (SetPopupSink) после
	// последнего кадра: общий холст-заготовка арены и картинки открытых и
	// исчезающих попапов (ADR-0009). Не накопительный; ноль, пока попапов нет.
	PopupBytes uint64
}

// engineStats — счётчики, из которых собирается Stats. Пишет их горутина
// кадра (и Post — из любой горутины), читает Stats — откуда угодно.
type engineStats struct {
	// Счётчики цикла (Post, пропуски тиков, потери, доставка) — у e.lp
	// (loop.Counters); skipped здесь — кадры, которые renderFrame начал и не
	// стал рисовать.
	frames, skipped      atomic.Uint64
	tiles, moves         atomic.Uint64
	drawNs, diffNs       atomic.Uint64
	panics               atomic.Uint64
	recorded, retained   atomic.Uint64
	tiledFrames          atomic.Uint64
	blurHits, blurMisses atomic.Uint64
	tilesSkipped         atomic.Uint64
	bufferBytes          atomic.Uint64
}

// Stats возвращает снимок счётчиков движка. Безопасен из любой горутины и не
// берёт замков; поля читаются по одному, так что снимок, снятый посреди кадра,
// может сочетать числа до и после него — для разности за секунду это неважно.
func (e *Engine) Stats() Stats {
	s := &e.stats
	lc := e.lp.Counters()
	posted, run := lc.Posted, lc.PostedRun
	var queue uint64
	if posted > run {
		queue = posted - run
	}
	return Stats{
		Frames:        s.frames.Load(),
		FramesSkipped: s.skipped.Load() + lc.Skipped,
		FramesLost:    lc.Lost,
		Tiles:         s.tiles.Load(),
		Moves:         s.moves.Load(),
		DrawNanos:     s.drawNs.Load(),
		DiffNanos:     s.diffNs.Load(),
		DeliverNanos:  lc.DeliverNanos,
		Posted:        posted,
		PostedRun:     run,
		PostQueue:     queue,
		Panics:        s.panics.Load(),
		Recorded:      s.recorded.Load(),
		Retained:      s.retained.Load(),
		TiledFrames:   s.tiledFrames.Load(),
		BlurHits:      s.blurHits.Load(),
		BlurMisses:    s.blurMisses.Load(),
		TilesSkipped:  s.tilesSkipped.Load(),
		BufferBytes:   s.bufferBytes.Load(),
		PopupBytes:    e.ov.PopupBytes(),
	}
}

// sinceNanos — сколько наносекунд прошло с t; отрицательное (часы перевели)
// считается нулём.
func sinceNanos(t time.Time) uint64 {
	d := time.Since(t)
	if d < 0 {
		return 0
	}
	return uint64(d)
}
