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
package engine

import (

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
func (e *Engine) SetPacing(p Pacing) {
	e.pacing.Store(uint32(p))
	e.Invalidate()
}

// Pacing сообщает текущий режим темпа.
func (e *Engine) Pacing() Pacing { return Pacing(e.pacing.Load()) }

// SetFrameSink задаёт сток кадров (nil — снять).
func (e *Engine) SetFrameSink(s FrameSink) {
	e.sinkMu.Lock()
	e.sink = s
	e.sinkMu.Unlock()
}

// getFrameSink возвращает текущий сток.
func (e *Engine) getFrameSink() FrameSink {
	e.sinkMu.RLock()
	defer e.sinkMu.RUnlock()
	return e.sink
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
func (e *Engine) RequestFrame() {
	e.frameWanted.Store(true)
	if !e.pacingIsExternal() {
		// Тикерный темп: кадры готовит тикер, и он их пропускает, пока UI не
		// менялся. Без инвалидации запрос просто исчезал бы — приложение,
		// написанное под внешний темп, при переключении режима переставало
		// получать кадры вовсе.
		e.Invalidate()
		return
	}
	select {
	case e.wake <- struct{}{}:
	default: // сигнал уже в очереди — второго не нужно
	}
}

// takeFrameRequest забирает запрос кадра, если он был.
func (e *Engine) takeFrameRequest() bool {
	return e.frameWanted.Swap(false)
}

// deliver отдаёт кадр стоку и в канал.
//
// Сток первым: он синхронный и не теряет кадров, а канал — очередь с
// ограниченной глубиной, из которой при переполнении кадр выбрасывается.
func (e *Engine) deliver(frame output.Frame) {
	if s := e.getFrameSink(); s != nil {
		s.Present(frame)
	}
	select {
	case e.frames <- frame:
	default:
		// Потребитель не успевает — кадр теряется. Так было всегда; кому
		// потеря недопустима, тот берёт сток.
	}
}

// pacingIsExternal — короткая проверка режима из горутины рендера.
func (e *Engine) pacingIsExternal() bool {
	return Pacing(e.pacing.Load()) == PacingExternal
}

