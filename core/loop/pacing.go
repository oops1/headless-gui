// pacing.go — кто задаёт темп кадров и куда они уходят.
//
// Темп задавал внутренний тикер: кадр готовился по расписанию, а переполнение
// канала кадров приводило к молчаливой потере кадра. Двум потребителям этого
// мало.
//
// Локальному выводу нужен темп по вертикальной синхронизации: кадр готовится
// тогда, когда сток готов его принять, — иначе работа делается впустую.
//
// Оболочке удалённого стола нужно другое: она меняет дерево виджетов из своей
// горутины и гонится с горутиной рендера. Внешний темп закрывает и это —
// потребитель зовёт RequestFrame там же, где мутирует сцену, и обе работы
// оказываются на одной горутине по построению.

package loop

import (
	"time"

	"github.com/oops1/headless-gui/v3/output"
)

// Pacing — кто решает, когда готовится кадр.
type Pacing uint8

const (
	// PacingTicker — внутренний тикер FPS, как было всегда.
	PacingTicker Pacing = iota
	// PacingExternal — кадры готовятся только по RequestFrame.
	PacingExternal
)

// FrameSink — сток готовых кадров.
//
// Альтернатива каналу Frames(), а не замена: канал продолжает работать. Сток
// получает кадр СИНХРОННО, в горутине рендера, и потому не может его
// потерять — в отличие от канала, который при переполнении молча выбрасывал
// кадр.
type FrameSink interface {
	// Present вызывается по готовности кадра. Тайлы, признаки содержимого и
	// перемещения — то же, что лежит в output.Frame.
	Present(frame output.Frame)
}

// SetPacing выбирает, кто задаёт темп.
//
// При PacingExternal внутренний тикер кадры не запускает — но продолжает
// продвигать анимации: иначе анимация, начатая приложением, замерла бы до
// следующего RequestFrame и дёргалась бы рывками.
func (l *Loop) SetPacing(p Pacing) {
	l.pacing.Store(uint32(p))
	l.Invalidate()
}

// Pacing сообщает текущий режим темпа.
func (l *Loop) Pacing() Pacing { return Pacing(l.pacing.Load()) }

// SetFrameSink задаёт сток кадров (nil — снять).
func (l *Loop) SetFrameSink(s FrameSink) {
	l.sinkMu.Lock()
	l.sink = s
	l.sinkMu.Unlock()
}

// getFrameSink возвращает текущий сток.
func (l *Loop) getFrameSink() FrameSink {
	l.sinkMu.RLock()
	defer l.sinkMu.RUnlock()
	return l.sink
}

// RequestFrame просит подготовить кадр: сток готов его принять.
//
// Имеет смысл при PacingExternal; при PacingTicker просто помечает кадр
// нужным, как обычная инвалидация, — чтобы переключение режима не требовало
// переписывать вызовы.
//
// Вызов не блокирует: он лишь поднимает флаг, а кадр готовит горутина
// рендера. Несколько вызовов подряд дают один кадр — сток всё равно не
// успеет принять больше.
func (l *Loop) RequestFrame() {
	l.frameWanted.Store(true)
	if !l.pacingIsExternal() {
		// Тикерный темп: кадры готовит тикер, и он их пропускает, пока UI не
		// менялся. Без инвалидации запрос просто исчезал бы — приложение,
		// написанное под внешний темп, при переключении режима переставало
		// получать кадры вовсе.
		l.Invalidate()
		return
	}
	select {
	case l.wake <- struct{}{}:
	default: // сигнал уже в очереди — второго не нужно
	}
}

// takeFrameRequest забирает запрос кадра, если он был.
func (l *Loop) takeFrameRequest() bool {
	return l.frameWanted.Swap(false)
}

// Deliver отдаёт кадр стоку и в канал.
//
// Сток первым: он синхронный и не теряет кадров, а канал — очередь с
// ограниченной глубиной, из которой при переполнении кадр выбрасывается.
// Выброс запоминается: следующий кадр, для которого в канале найдётся место,
// уйдёт полным (fullframe.go) — разности после потерянного кадра потребитель
// наложил бы на картинку, которой у него нет.
func (l *Loop) Deliver(frame output.Frame) {
	start := time.Now()
	defer func() { l.cnt.deliverNs.Add(sinceNanos(start)) }()
	if s := l.getFrameSink(); s != nil {
		// Что сток сделает с Data, неизвестно: буферы кадра закреплены и в
		// пул не вернутся (SetFrameRecycling).
		frame.Pin()
		// Граница потребителя (failure.go): упавший сток не роняет цикл.
		l.host.GuardCall("FrameSink", func() { s.Present(frame) })
	}
	select {
	case l.frames <- frame:
	default:
		// Потребитель не успевает — кадр теряется, но не молча.
		l.lostFrame.Store(true)
		l.cnt.lost.Add(1)
		frame.Release() // никто его не увидит — буфер сразу в пул
	}
}

// pacingIsExternal — короткая проверка режима из горутины рендера.
func (l *Loop) pacingIsExternal() bool {
	return Pacing(l.pacing.Load()) == PacingExternal
}
