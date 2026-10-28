package engine

// motionprofile.go — профиль движения движка (задача 3.2 плана 4.0).
//
// «Меньше движения» до сих пор было настройкой процесса: флаг темы и
// widget.SetReduceMotion. Удалённому столу нужно другое: анимация меню там
// стоит канала, и решает это подключение (WinLine — по
// PERF_DISABLE_MENUANIMATIONS клиента), а не пользователь в настройках. И
// решает за свой движок: локальный экран той же машины анимируется как
// обычно.
//
// Профиль узнают по горутине движка (widget/motionprofile.go): цикл, Post,
// тики, а пока хоть у одного движка процесса профиль не MotionFull — и чужая
// горутина на время работы движка на ней: ввод (guardInput), синхронный кадр
// (RenderOnce, RenderFrameNow), очередь Post незапущенного движка. Привязка
// нужна и движку с MotionFull: обработчик движка A, переславший событие
// движку B, иначе отдал бы B профиль A. Флаг темы и SetReduceMotion действуют
// как прежде, вместе с профилем: движение уменьшено, если этого хочет любой
// из них.

import (
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// motionEngines — сколько движков процесса живут с профилем не MotionFull.
// Пока нуль, чужую горутину к движку не привязывают: ввод и кадр не платят за
// поиск горутины.
var motionEngines atomic.Int32

// MotionProfile — профиль движения движка (см. theme.MotionProfile).
type MotionProfile = theme.MotionProfile

const (
	// MotionFull — анимации как объявлены (по умолчанию).
	MotionFull = theme.MotionFull
	// MotionReduced — как флаг «меньше движения», но для этого движка.
	MotionReduced = theme.MotionReduced
	// MotionOff — без движения, кроме зациклённых индикаторов и таймеров.
	MotionOff = theme.MotionOff
	// MotionRemote — длительности прежние, промежуточных состояний не больше
	// RemoteMotionSteps на переход.
	MotionRemote = theme.MotionRemote
)

// RemoteMotionSteps — состояний перехода в профиле MotionRemote.
const RemoteMotionSteps = widget.RemoteMotionSteps

// SetMotionProfile задаёт профиль движения движка. Можно звать из любой
// горутины и на лету: идущие анимации подхватывают профиль на следующем шаге
// (MotionOff доводит их до конца, MotionRemote квантует остаток), новые
// заводятся уже с ним. Неизвестное значение — MotionFull.
func (e *Engine) SetMotionProfile(p MotionProfile) {
	if p > MotionRemote {
		p = MotionFull
	}
	e.motion.Store(uint32(p))
	e.countMotion(p != MotionFull && !e.stopped.Load())
	if e.stopped.Load() {
		e.countMotion(false) // Stop успел между проверкой и учётом
	}
}

// countMotion держит движок в motionEngines (on) или вне его. Stop выводит
// движок из счёта, профиль при этом остаётся каким был.
func (e *Engine) countMotion(on bool) {
	if e.motionCounted.CompareAndSwap(!on, on) {
		if on {
			motionEngines.Add(1)
		} else {
			motionEngines.Add(-1)
		}
	}
}

// MotionProfile возвращает профиль движения движка.
func (e *Engine) MotionProfile() MotionProfile {
	return MotionProfile(e.motion.Load())
}

// motionScope объявляет текущую горутину горутиной движка на время его работы
// на ней (ввод, синхронный кадр, очередь Post): анимации, заведённые там,
// получают профиль движка и привязываются к его циклу. Пока у всех движков
// MotionFull — ничего. nil — снимать нечего.
func (e *Engine) motionScope() func() {
	if motionEngines.Load() == 0 {
		return nil
	}
	return widget.BindAnimationLoop(e)
}
