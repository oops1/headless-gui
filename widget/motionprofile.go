package widget

// motionprofile.go — профиль движения движка (engine.SetMotionProfile,
// задача 3.2 плана 4.0; запрос WinLine: клиент удалённого стола с выключенной
// анимацией меню).
//
// Профиль — свойство движка, а не процесса: реестр анимаций общий, но каждую
// шагает цикл, к которому она привязана (BindAnimationLoop), и ключ цикла —
// сам движок. Поэтому профиль узнаётся по ключу: у шага — по ключу цикла, у
// MotionDur и токенов темы (theme.AnimReduced) — по ключу горутины, с
// которой заводят анимацию. Отдельного реестра профилей нет, и забывать
// остановленный движок некому.
//
// Что делает профиль:
//   - MotionReduced — MotionDur и токены темы отдают длительности как при
//     флаге «меньше движения»;
//   - MotionOff — то же, функциональные тоже мгновенны, и шаг доводит до
//     конца любую незацикленную анимацию с тиком, как бы её ни завели;
//   - MotionRemote — длительности прежние, шаг квантует прогресс на
//     RemoteMotionSteps и не зовёт тик повторно на том же шаге.

import "github.com/oops1/headless-gui/v3/theme"

// RemoteMotionSteps — сколько промежуточных состояний у перехода в профиле
// MotionRemote, считая конечное: 0, 1/3, 2/3, 1.
const RemoteMotionSteps = 3

// motionProfiler — ключ цикла, у которого есть профиль движения (движок).
type motionProfiler interface {
	MotionProfile() theme.MotionProfile
}

func init() { theme.SetMotionProfileProbe(CurrentMotionProfile) }

// CurrentMotionProfile возвращает профиль движения движка, на горутине
// которого идёт вызов: его цикл, Post, тики, обработчики ввода. Вне движка —
// theme.MotionFull.
func CurrentMotionProfile() theme.MotionProfile {
	return loopMotionProfile(currentAnimLoop())
}

// loopMotionProfile — профиль цикла key; цикл без профиля — MotionFull.
func loopMotionProfile(key any) theme.MotionProfile {
	if p, ok := key.(motionProfiler); ok {
		return p.MotionProfile()
	}
	return theme.MotionFull
}

// noInertia — прокрутка без маховика: профиль движения движка MotionOff
// (движения нет) или MotionRemote (каждый кадр маховика уходит по сети, а
// жест колеса и так приходит с удалённого клиента дельтами).
func noInertia() bool {
	switch CurrentMotionProfile() {
	case theme.MotionOff, theme.MotionRemote:
		return true
	}
	return false
}
