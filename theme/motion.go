package theme

import (
	"sync"
	"time"
)

// «Меньше движения» (FlagMotionReduce).
//
// Единая точка решения — Manager.GetAnimation: через неё читают токены
// анимации все потребители оболочки (Flyout, Tween, кнопки панели задач,
// центр уведомлений, «Пуск», автоскрытие панели). Включённый флаг меняет
// длительность, которую они получают, и больше ничего им знать не нужно:
// нулевая длительность и так значит «мгновенно».
//
// Анимации делятся на два рода. Декоративная только украшает (наведение,
// выезд панели, появление окна) — при флаге её длительность нулевая.
// Функциональная несёт смысл (раскрытие карточки уведомления: пользователь
// должен понять, что именно развернулось) — при флаге она мгновенна или
// укорочена до метрики KeyMotionReduceFunctionalMS.

// AnimKind — род анимации для режима «меньше движения».
type AnimKind uint8

const (
	// AnimDecorative — украшение: при «меньше движения» отключается совсем.
	// Род по умолчанию для любого токена, о котором ничего не объявлено.
	AnimDecorative AnimKind = iota
	// AnimFunctional — анимация, объясняющая изменение (раскрытие,
	// сворачивание): при «меньше движения» мгновенная или укороченная.
	AnimFunctional
)

// KeyMotionReduceFunctionalMS — метрика профиля: до скольких миллисекунд
// укорачиваются ФУНКЦИОНАЛЬНЫЕ анимации при «меньше движения». Нет метрики
// (или 0) — мгновенно.
const KeyMotionReduceFunctionalMS Key = "motion.reduce.functional_ms"

var (
	animKindMu sync.RWMutex
	animKinds  = map[Key]AnimKind{
		// Раскрытие и сворачивание карточки центра уведомлений.
		"notification.expand": AnimFunctional,
	}
)

// RegisterAnimationKind объявляет род анимации с токеном k. Потребитель,
// добавивший свою анимацию (индикатор кнопки окна, раскрытие группы), объявляет
// её род один раз при старте; незарегистрированный токен считается
// декоративным.
func RegisterAnimationKind(k Key, kind AnimKind) {
	animKindMu.Lock()
	animKinds[k] = kind
	animKindMu.Unlock()
}

// AnimationKindOf возвращает род анимации с токеном k.
func AnimationKindOf(k Key) AnimKind {
	animKindMu.RLock()
	kind := animKinds[k]
	animKindMu.RUnlock()
	return kind
}

// MotionReduced сообщает, включён ли режим «меньше движения» в теме.
func (t *Theme) MotionReduced() bool {
	return t != nil && t.FlagOr(FlagMotionReduce, false)
}

// AnimReduced применяет режим «меньше движения» к анимации k: декоративная
// получает нулевую длительность, функциональная — не больше метрики
// KeyMotionReduceFunctionalMS (по умолчанию нуль). Кривая сохраняется. Без
// флага возвращает a как есть.
func (t *Theme) AnimReduced(k Key, a AnimSpec) AnimSpec {
	if !t.MotionReduced() || a.Duration <= 0 {
		return a
	}
	limit := time.Duration(0)
	if AnimationKindOf(k) == AnimFunctional {
		if ms := t.MetricOr(KeyMotionReduceFunctionalMS, 0); ms > 0 {
			limit = time.Duration(ms * float64(time.Millisecond))
		}
	}
	if a.Duration > limit {
		a.Duration = limit
		a.DurationMS = int(limit / time.Millisecond)
	}
	return a
}

// MotionReduced сообщает, включён ли режим «меньше движения» у активной темы.
func (m *Manager) MotionReduced() bool {
	m.mu.RLock()
	t := m.active
	m.mu.RUnlock()
	return t.MotionReduced()
}

// GetAnimationRaw возвращает анимацию активной темы как она объявлена, без
// режима «меньше движения». Нужна тому, кто сам решает, что делать с флагом;
// обычному компоненту — GetAnimation.
func (m *Manager) GetAnimationRaw(k Key) AnimSpec {
	m.mu.RLock()
	t := m.active
	m.mu.RUnlock()
	if t == nil {
		return AnimSpec{}
	}
	a, _ := t.Anim(k)
	return a
}
