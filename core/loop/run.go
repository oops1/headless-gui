package loop

import (
	"time"

	"github.com/oops1/headless-gui/v3/internal/goid"
)

func (l *Loop) run() {
	defer close(l.done)
	l.loopGID.Store(goid.Current())
	defer l.loopGID.Store(0)
	// Анимации, заведённые на горутине цикла, шагает только этот движок: у
	// вторичного движка диалога свои обработчики и своя горутина (GG-68).
	defer l.host.BindLoop()()

	interval := l.FrameInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// lastGen — поколение инвалидации, отрендеренное последним кадром
	// (on-demand). Сентинел гарантирует рендер первого кадра.
	lastGen := ^uint64(0)
	// lastFrameAt — когда рисовался последний кадр: кадр по пробуждению не
	// должен идти чаще темпа кадров.
	var lastFrameAt time.Time

	for {
		select {
		case <-l.wake:
			// Отложенные вызовы разбираем ПЕРВЫМИ: они меняют дерево, и их
			// изменения должны попасть в тот же кадр, а не в следующий.
			l.RunPosted()
			if l.pacingIsExternal() {
				// Внешний темп: кадр — только по запросу стока. Анимации
				// продвигаются в тикере, здесь — только кадр.
				if !l.takeFrameRequest() {
					continue
				}
				frame := l.host.RenderFrame()
				lastFrameAt = time.Now()
				if len(frame.Tiles) == 0 && len(frame.Moves) == 0 {
					continue
				}
				l.Deliver(frame)
				continue
			}
			// Тикерный темп: через очередь приходит ввод окна, и его результат
			// должен появиться сразу, а не на следующем тике. Но не чаще темпа
			// кадров — мышь шлёт сотни событий в секунду, и кадр на каждое был
			// бы работой впустую; не нарисованное здесь нарисует тикер.
			if !l.onDemand.Load() || time.Since(lastFrameAt) < interval {
				continue
			}
			gen := l.invGen.Load()
			if gen == lastGen && !l.host.FullFramePending() {
				continue
			}
			lastGen = gen
			frame := l.host.RenderFrame()
			lastFrameAt = time.Now()
			if len(frame.Tiles) == 0 && len(frame.Moves) == 0 {
				continue
			}
			l.Deliver(frame)

		case <-ticker.C:
			// Потолок частоты (SetMaxFPS) мог смениться с прошлого тика.
			if iv := l.FrameInterval(); iv != interval {
				interval = iv
				ticker.Reset(iv)
			}
			// Отложенные вызовы — раньше всего: они меняют дерево, а решение
			// о пропуске кадра принимается по заявленному damage, и damage от
			// них обязан успеть.
			l.RunPosted()
			// Продвигаем анимации ДО решения о пропуске кадра: тики зовут
			// сеттеры виджетов, те самоинвалидируются (авто-damage), поэтому
			// damage от тиков попадёт в invGen ниже и кадр перерисуется
			// частично. Вызывается в любом режиме — анимации живут и при
			// SetRenderOnDemand(false).
			l.host.GuardCall("Animation", func() { l.host.StepAnimations(time.Now()) })
			// Внешний темп: кадры готовит RequestFrame. Анимации всё равно
			// продвигаем — иначе начатая приложением анимация замерла бы до
			// следующего запроса и шла бы рывками.
			if l.pacingIsExternal() {
				continue
			}
			if l.onDemand.Load() {
				gen := l.invGen.Load()
				// Полный кадр после потери или смены размера рисуется и без
				// изменений: иначе статичный интерфейс остался бы у
				// потребителя неверным (fullframe.go).
				if gen == lastGen && !l.host.AnimationNeeded(interval) && !l.host.FullFramePending() {
					l.cnt.skipped.Add(1)
					continue // UI не менялся — пропускаем кадр целиком
				}
				lastGen = gen // снимаем ДО рендера: инвалидация во время кадра не потеряется
			}
			frame := l.host.RenderFrame()
			lastFrameAt = time.Now()
			// Кадр из ОДНИХ переносов — полноценный кадр: окно переехало,
			// пикселей не изменилось, и потребителю есть что выполнить.
			// Проверка только по тайлам отбрасывала его молча.
			if len(frame.Tiles) == 0 && len(frame.Moves) == 0 {
				continue
			}
			// Один путь выдачи на оба темпа: сток и канал получают кадр
			// в одном месте, иначе сток молча оставался бы пустым при
			// тикерном темпе — ровно это и показал тест.
			l.Deliver(frame)
		case <-l.quit:
			return
		}
	}
}
