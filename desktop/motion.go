// motion.go — движение в компонентах рабочего стола: плавные переходы цвета
// при наведении и нажатии и плавное изменение числа (ширина боковой панели).
//
// Токены анимации давно объявлены во всех профилях темы, но читал их один
// автоскрыт. Здесь они подключаются к остальным элементам. Правила общие:
//
//   - длительность и кривую задаёт ТЕМА (theme.AnimSpec); компонент не знает
//     ни числа миллисекунд, ни имени темы;
//   - нулевая длительность (или токен не объявлен) — мгновенно, как в Windows
//     2000: компонент ничего не проверяет, просто получает нуль;
//   - часы принадлежат движку: переходы идут через widget.AnimateOwned, и
//     кадры просыпаются только пока что-то движется;
//   - перерисовывается ровно область ячейки, у которой меняется цвет, а не
//     элемент целиком и тем более не кадр.
package desktop

import (
	"fmt"
	"image"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Имена анимаций темы, которыми пользуются компоненты рабочего стола.
const (
	// AnimMenuOpen — появление и исчезновение всплывающей панели (Flyout).
	AnimMenuOpen theme.Key = "menu.open"
	// AnimWindowOpen — появление окна.
	AnimWindowOpen theme.Key = "window.open"
	// AnimHover — переход цвета при наведении и нажатии.
	AnimHover theme.Key = "hover"
	// AnimTaskbarItem — переход цвета у кнопок панели задач. Если тема его не
	// объявила, берётся AnimHover.
	AnimTaskbarItem theme.Key = "taskbar.item"
)

// animation читает длительность и кривую анимации key из темы. Нулевая
// длительность — «мгновенно»; кривая по умолчанию — линейная (nil).
func animation(tm *theme.Manager, key theme.Key) (time.Duration, widget.Easing) {
	if tm == nil {
		return 0, nil
	}
	a := tm.GetAnimation(key)
	if a.Duration <= 0 {
		return 0, nil
	}
	return a.Duration, widget.EasingByName(a.Curve)
}

// itemAnimation — анимация кнопки панели задач: свой токен, а если темы его не
// заявили — общий токен наведения.
func itemAnimation(tm *theme.Manager) (time.Duration, widget.Easing) {
	if d, c := animation(tm, AnimTaskbarItem); d > 0 {
		return d, c
	}
	return animation(tm, AnimHover)
}

// ─── Переход цвета между состояниями ────────────────────────────────────────

// maxMotionCells — сколько ячеек запоминается, прежде чем покоящиеся забудутся.
const maxMotionCells = 256

// motion — плавные переходы между стилями состояний для набора ячеек одного
// элемента (кнопки окон, значки трея, строки меню, числа календаря).
//
// Нулевое значение готово к работе; встраивается в элемент полем.
//
// Устроено «по запросу»: элемент в Draw спрашивает стиль для текущего
// состояния у Style, а тот замечает, что состояние ячейки сменилось, и пускает
// переход от того, что на экране сейчас, к новому стилю. Обработчикам мыши
// ничего подключать не нужно — они и так лишь меняют флаги и инвалидируют.
//
// Первое появление ячейки перехода не запускает: кнопка, которая только что
// возникла активной, активной и рисуется.
type motion struct {
	mu    sync.Mutex
	cells map[any]*motionCell
}

type motionCell struct {
	state theme.State // доминирующее состояние, к которому идёт переход
	to    *theme.Style
	from  theme.Style // стиль на момент начала перехода
	k     float64     // прогресс 0..1 (после кривой)
	live  bool
	rect  image.Rectangle // область ячейки — её и заявляем на каждом шаге
	tag   string
}

// tray — Style для значка трея: стиль берётся у темы по имени компонента.
func (m *motion) tray(tm *theme.Manager, component string, r image.Rectangle, st theme.State) *theme.Style {
	return m.Style(tm, 0, r, st, func(st theme.State) *theme.Style {
		return trayStyle(tm, component, st)
	})
}

// Style возвращает стиль ячейки id (любой сравнимый ключ: номер окна, имя приложения) в состоянии st: сам get(st), когда покой, и
// смесь прежнего и нового, пока идёт переход. r — область ячейки (абсолютные
// логические координаты): её перерисовывают шаги перехода.
//
// Длительность — токен AnimHover.
func (m *motion) Style(tm *theme.Manager, id any, r image.Rectangle, st theme.State, get func(theme.State) *theme.Style) *theme.Style {
	return m.style(tm, false, id, r, st, get)
}

// ItemStyle — как Style, но для кнопок панели задач: токен AnimTaskbarItem
// (а при его отсутствии AnimHover).
func (m *motion) ItemStyle(tm *theme.Manager, id any, r image.Rectangle, st theme.State, get func(theme.State) *theme.Style) *theme.Style {
	return m.style(tm, true, id, r, st, get)
}

func (m *motion) style(tm *theme.Manager, item bool, id any, r image.Rectangle, st theme.State, get func(theme.State) *theme.Style) *theme.Style {
	to := get(st)
	if tm == nil {
		return to
	}
	dom := st.Dominant()

	m.mu.Lock()
	if m.cells == nil {
		m.cells = make(map[any]*motionCell)
	}
	c := m.cells[id]
	if c == nil {
		if len(m.cells) >= maxMotionCells {
			for k, v := range m.cells {
				if !v.live {
					delete(m.cells, k)
				}
			}
		}
		m.cells[id] = &motionCell{state: dom, to: to, k: 1, rect: r, tag: "motion:" + fmt.Sprint(id)}
		m.mu.Unlock()
		return to
	}
	c.rect = r

	var start func()
	if c.state != dom {
		var dur time.Duration
		var curve widget.Easing
		if item {
			dur, curve = itemAnimation(tm)
		} else {
			dur, curve = animation(tm, AnimHover)
		}
		if dur <= 0 {
			c.state, c.to, c.live, c.k = dom, to, false, 1
			m.mu.Unlock()
			return to
		}
		// Начало — то, что на экране сейчас: оборванный переход не прыгает.
		c.from = *c.current()
		c.state, c.to, c.k, c.live = dom, to, 0, true
		cell, tag := c, c.tag
		start = func() {
			// Владелец и тег: переход той же ячейки заменяет прежний, а
			// соседние ячейки одного элемента идут независимо.
			widget.AnimateOwned(m, tag, dur, curve, func(t float64) {
				m.mu.Lock()
				cell.k = t
				if t >= 1 {
					cell.k, cell.live = 1, false
				}
				rect := cell.rect
				m.mu.Unlock()
				widget.InvalidateRect(rect)
			})
		}
	} else {
		c.to = to // тема могла смениться — покой рисуется по свежему стилю
	}
	cur := c.current()
	m.mu.Unlock()

	if start != nil {
		start()
	}
	return cur
}

// current — стиль на текущий момент перехода.
func (c *motionCell) current() *theme.Style {
	if !c.live || c.k >= 1 {
		return c.to
	}
	k := c.k
	if k < 0 {
		k = 0
	}
	return lerpStyle(&c.from, c.to, k)
}

// lerpStyle смешивает цвета двух стилей: заливку, текст, рамку, тень и
// подкраску стекла. Всё остальное (геометрия, шрифт, скругление) берётся у
// целевого — оно не переходит плавно, а меняется сразу.
//
// Градиент и объёмная рамка плавно не смешиваются: если они есть, стиль
// меняется скачком. Профили с ними (Windows 2000) анимаций не задают вовсе.
func lerpStyle(from, to *theme.Style, k float64) *theme.Style {
	if k >= 1 {
		return to
	}
	out := *to
	if len(from.Gradient) > 0 || len(to.Gradient) > 0 || from.Bevel != nil || to.Bevel != nil {
		return to
	}
	out.Fill = widget.LerpColor(from.Fill, to.Fill, k)
	out.Text = widget.LerpColor(from.Text, to.Text, k)
	out.Border = widget.LerpColor(from.Border, to.Border, k)
	out.Shadow = widget.LerpColor(from.Shadow, to.Shadow, k)
	if from.Backdrop.Mode == to.Backdrop.Mode {
		out.Backdrop.Tint = widget.LerpColor(from.Backdrop.Tint, to.Backdrop.Tint, k)
	}
	return &out
}

// ─── Плавное число ──────────────────────────────────────────────────────────

// Tween — число, которое плавно идёт к цели по анимации темы: ширина боковой
// панели меню «Пуск» между 48 и 256, высота раскрывающегося списка, что угодно
// ещё, что должно не прыгать, а ехать.
//
// Это готовый механизм анимации размера: компонент хранит Tween, в раскладке и
// отрисовке читает Value, а переключает цель через To. Нулевая длительность
// анимации в теме — число меняется сразу.
//
// onChange вызывается на каждом шаге (после обновления значения) — туда
// ставится заявка на перерисовку ТОЛЬКО затронутой области.
type Tween struct {
	mu       sync.Mutex
	tm       *theme.Manager
	key      theme.Key
	cur, tgt float64
	live     bool
	anim     *widget.Animation
	onChange func()
}

// NewTween создаёт число со значением initial, меняющееся по анимации key темы
// tm (обычно AnimMenuOpen). onChange может быть nil.
func NewTween(tm *theme.Manager, key theme.Key, initial float64, onChange func()) *Tween {
	return &Tween{tm: tm, key: key, cur: initial, tgt: initial, onChange: onChange}
}

// Value возвращает текущее значение — промежуточное, пока идёт переход.
func (t *Tween) Value() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cur
}

// Target возвращает цель, к которой идёт значение.
func (t *Tween) Target() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.tgt
}

// Animating сообщает, идёт ли переход.
func (t *Tween) Animating() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.live
}

// To задаёт новую цель. Прежний переход прерывается: новый стартует от
// текущего значения, без скачка.
func (t *Tween) To(v float64) {
	dur, curve := animation(t.tm, t.key)

	t.mu.Lock()
	if v == t.tgt && (t.live || t.cur == v) {
		t.mu.Unlock()
		return
	}
	from := t.cur
	t.tgt = v
	if dur <= 0 {
		t.cur, t.live = v, false
		cb, prev := t.onChange, t.anim
		t.anim = nil
		t.mu.Unlock()
		if prev != nil {
			prev.Stop()
		}
		if cb != nil {
			cb()
		}
		return
	}
	t.live = true
	t.mu.Unlock()

	a := widget.AnimateOwned(t, "tween", dur, curve, func(k float64) {
		t.mu.Lock()
		t.cur = widget.LerpF(from, v, k)
		if k >= 1 {
			t.cur, t.live = v, false
		}
		cb := t.onChange
		t.mu.Unlock()
		if cb != nil {
			cb()
		}
	})
	t.mu.Lock()
	t.anim = a
	t.mu.Unlock()
}

// Set ставит значение сразу, без перехода, и останавливает идущий.
func (t *Tween) Set(v float64) {
	t.mu.Lock()
	changed := t.cur != v
	t.cur, t.tgt, t.live = v, v, false
	cb, prev := t.onChange, t.anim
	t.anim = nil
	t.mu.Unlock()
	if prev != nil {
		prev.Stop()
	}
	if changed && cb != nil {
		cb()
	}
}
