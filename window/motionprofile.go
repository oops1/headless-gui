package window

import "github.com/oops1/headless-gui/v3/engine"

// inheritMotionProfile даёт вторичному движку (диалог, плавающая панель)
// профиль движения движка родительского окна. Профиль, сменённый у родителя
// позже, сам до уже открытых окон не доходит.
func inheritMotionProfile(eng *engine.Engine, parent *Window) {
	if parent == nil {
		return
	}
	if mp, ok := parent.eng.(interface{ MotionProfile() engine.MotionProfile }); ok {
		eng.SetMotionProfile(mp.MotionProfile())
	}
}
