// notifyview_slide.go — плавное раскрытие и сворачивание в центре уведомлений
// Windows 10: текст карточки (две строки и до восьми), группа приложения и
// сетка быстрых действий.
//
// Раскладка центра считается заново на каждое событие и кадр, поэтому
// анимации не вмешиваются в неё: вид хранит для каждого элемента лишь долю
// раскрытости k (0 — свёрнут, 1 — раскрыт), а раскладка, пока k в пути, берёт
// высоту между свёрнутым и раскрытым видом. Начинается движение «по запросу», как
// у motion.Style: раскладка замечает, что цель (раскрыто или нет) сменилась, и
// просит запустить анимацию — выключатели состояния (мышь, клавиши, методы
// SetGroupCollapsed и SetQuickExpanded) ничего не подключают.
//
// Длительность и кривая — токен темы notification.expand (AnimNotificationExpand);
// не объявлен — menu.open; нуль — мгновенно. Элемент, впервые попавший в
// раскладку (центр только что открыт, пришло новое уведомление), не движется:
// он рисуется сразу в своём состоянии.
package desktop

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// AnimNotificationExpand — анимация раскрытия и сворачивания элементов центра
// уведомлений Windows 10: текста карточки, группы и сетки быстрых действий.
// Профиль, не назвавший токен, получает AnimMenuOpen.
const AnimNotificationExpand theme.Key = "notification.expand"

// slideKind — что раскрывается.
type slideKind uint8

const (
	slideCard slideKind = iota + 1
	slideGroup
	slideQuick
)

// ncSlideKey называет раскрывающийся элемент. Привязка к уведомлению и
// приложению, а не к положению: раскладка меняется, элемент остаётся тем же.
type ncSlideKey struct {
	kind slideKind
	note NotificationID
	app  AppID
}

// ncSlide — состояние раскрытия одного элемента.
type ncSlide struct {
	k, from float64
	open    bool // цель: раскрыт ли элемент, когда движение кончится
	live    bool
	anim    *widget.Animation
}

func slideTarget(open bool) float64 {
	if open {
		return 1
	}
	return 0
}

// expandAnimation читает анимацию раскрытия из темы: свой токен, а если профиль
// его не объявил, — открытие панели.
func expandAnimation(tm *theme.Manager) (time.Duration, widget.Easing) {
	if tm == nil {
		return 0, nil
	}
	if t := tm.Active(); t != nil {
		if _, ok := t.Anim(AnimNotificationExpand); ok {
			return animation(tm, AnimNotificationExpand)
		}
	}
	return animation(tm, AnimMenuOpen)
}

// slideStep возвращает долю раскрытости элемента и идёт ли его движение. open —
// цель сейчас. Вызывается раскладкой под замком вида.
//
// Сменилась цель — движение стартует от доли, до которой элемент добрался:
// оборванное раскрытие не прыгает. Сам запуск анимации откладывается до снятия
// замка (flushSlides): тики берут тот же замок.
func (v *richView) slideStep(key ncSlideKey, open bool) (float64, bool) {
	if v.mode == ncModeToast {
		return slideTarget(open), false
	}
	s := v.slides[key]
	if s == nil {
		if v.slides == nil {
			v.slides = map[ncSlideKey]*ncSlide{}
		}
		s = &ncSlide{k: slideTarget(open), open: open}
		v.slides[key] = s
		return s.k, false
	}
	if s.open != open {
		s.open = open
		dur, curve := expandAnimation(v.tm)
		if dur <= 0 {
			if s.anim != nil {
				s.anim.Stop()
				s.anim = nil
			}
			s.k, s.live = slideTarget(open), false
			return s.k, false
		}
		s.from, s.live = s.k, true
		v.starts = append(v.starts, func() { v.slideStart(key, s, dur, curve) })
	}
	return s.k, s.live
}

// slideSettle ставит элемент в конечное состояние без движения: раскладка
// выяснила, что менять ему нечего (свёрнутый и раскрытый вид одной высоты).
// Вызывается под замком.
func (v *richView) slideSettle(key ncSlideKey, open bool) {
	if s := v.slides[key]; s != nil {
		if s.anim != nil {
			s.anim.Stop()
			s.anim = nil
		}
		s.k, s.live, s.open = slideTarget(open), false, open
	}
}

// flushSlides запускает анимации, о которых попросила раскладка.
func (v *richView) flushSlides() {
	v.mu.Lock()
	q := v.starts
	v.starts = nil
	v.mu.Unlock()
	for _, f := range q {
		f()
	}
}

// slideStart пускает анимацию элемента. Новая анимация того же элемента
// заменяет прежнюю (widget.AnimateOwned по владельцу и тегу).
func (v *richView) slideStart(key ncSlideKey, s *ncSlide, dur time.Duration, curve widget.Easing) {
	v.mu.Lock()
	live := s.live && v.slides[key] == s
	v.mu.Unlock()
	if !live {
		return
	}
	a := widget.AnimateOwned(s, "slide", dur, curve, func(t float64) {
		// Кривая с отскоком (out-back) вышла бы за 0..1 и вывернула бы высоту
		// наизнанку: доля раскрытости остаётся в границах.
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		v.mu.Lock()
		if v.slides[key] != s || !s.live {
			v.mu.Unlock()
			return
		}
		to := slideTarget(s.open)
		s.k = s.from + (to-s.from)*t
		if t >= 1 {
			s.k, s.live, s.anim = to, false, nil
		}
		v.mu.Unlock()
		v.slideTick(key)
	})
	v.mu.Lock()
	if v.slides[key] == s && s.live {
		s.anim = a
	}
	v.mu.Unlock()
}

// slideTick заявляет перерисовку шага движения: от верха двигающегося элемента
// до низа списка (всё, что ниже, съезжает вместе с ним). Выше и вокруг ничего не
// меняется. Если высота списка сократилась так, что он сам подтянулся к верху
// (прокрутка упёрлась в край), сдвинулось всё — тогда перерисовывается весь
// список. Сетка быстрых действий меняет и список, и подвал: от верха списка до
// низа панели.
func (v *richView) slideTick(key ncSlideKey) {
	v.mu.Lock()
	old := v.lastLayout
	v.mu.Unlock()
	if old.panel.Empty() || old.viewport.Empty() {
		return
	}
	top := old.slideTop(key)
	// Раскладка заново: она же подтягивает прокрутку к новой высоте.
	nl := v.layout(old.panel)
	switch {
	case nl.scroll != old.scroll:
		v.invalidate(nl.viewport)
	case key.kind == slideQuick:
		v.invalidate(image.Rect(nl.panel.Min.X, nl.viewport.Min.Y, nl.panel.Max.X, nl.panel.Max.Y))
	default:
		v.invalidate(image.Rect(nl.viewport.Min.X, top, nl.viewport.Max.X, nl.viewport.Max.Y).Intersect(nl.viewport))
	}
}

// slideTop — верх двигающегося элемента в раскладке (для списка — не выше
// его окна).
func (l *richLayout) slideTop(key ncSlideKey) int {
	top := l.viewport.Min.Y
	for _, g := range l.groups {
		if key.kind == slideGroup && g.app == key.app {
			top = g.rect.Min.Y
		}
		for _, c := range g.cards {
			if key.kind == slideCard && c.n.ID == key.note {
				top = c.rect.Min.Y
			}
		}
	}
	if top < l.viewport.Min.Y {
		top = l.viewport.Min.Y
	}
	return top
}

// forgetSlides забывает движения исчезнувших уведомлений и групп. nil, nil —
// все (панель закрылась). Вызывается под замком.
func (v *richView) forgetSlides(ids map[NotificationID]bool, apps map[AppID]bool) {
	for k, s := range v.slides {
		keep := false
		switch k.kind {
		case slideCard:
			keep = ids != nil && ids[k.note]
		case slideGroup:
			keep = apps != nil && apps[k.app]
		case slideQuick:
			keep = ids != nil // набор плиток живёт, пока открыта панель
		}
		if !keep {
			if s.anim != nil {
				s.anim.Stop()
			}
			delete(v.slides, k)
		}
	}
	if ids == nil && apps == nil {
		v.starts = nil
	}
}
