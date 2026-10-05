package engine

import (
	"fmt"
	"image/color"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ApplyThemeProfile применяет к движку активную тему менеджера заново — после
// того как приложение изменило её не сменой профиля, а переопределением
// (Manager.SetAccent, Manager.SetFlag, перерегистрация профиля).
//
// Компоненты оболочки (пакет desktop) читают стили у менеджера при каждой
// отрисовке и подписаны на его смену, а виджетам со своей палитрой нужен
// обход дерева — его и делает SetTheme. Без активной темы возвращает ошибку.
//
// Профиль с флагом theme.FlagFontDefaultGlobal (Windows 10) заодно делает
// Fonts["default"].Size кеглем по умолчанию для всех виджетов, у которых свой
// не задан (widget.DefaultFontSize); профиль без флага возвращает 10 pt.
//
// Профиль с флагом theme.FlagTextSubpixel (Windows 10) включает подпиксельное
// позиционирование глифов (Engine.SetTextSubpixel), профиль без флага
// выключает то, что включила тема. Режим, выбранный приложением явным вызовом
// SetTextSubpixel, тема не трогает (Engine.UseThemeTextSubpixel возвращает
// выбор теме). Переключение сбрасывает замеры строк: ширины в режимах разные.
func (e *Engine) ApplyThemeProfile(m *theme.Manager) error {
	if m == nil {
		return fmt.Errorf("engine: ApplyThemeProfile без менеджера тем")
	}
	t := m.Active()
	if t == nil {
		return fmt.Errorf("engine: у менеджера нет активной темы")
	}
	// «Меньше движения» (theme.FlagMotionReduce) доходит и до виджетов со
	// своими анимациями — компоненты оболочки получают его через
	// Manager.GetAnimation сами.
	widget.SetReduceMotion(t.MotionReduced())
	e.SetTheme(widget.Materialize(t))
	return nil
}

// SetMotionReduce включает и выключает режим «меньше движения» на лету:
// ставит флаг theme.FlagMotionReduce менеджеру тем и применяет тему к
// движку. Потребитель вызывает это из своих настроек доступности: декоративные
// анимации (наведение, выезд панелей, появление окон, переключатели)
// становятся мгновенными, функциональные (раскрытие карточки уведомления) —
// мгновенными или короткими (метрика theme.KeyMotionReduceFunctionalMS).
// Идущие анимации доигрывают как были, новые стартуют уже без движения.
func (e *Engine) SetMotionReduce(m *theme.Manager, on bool) error {
	if m == nil {
		return fmt.Errorf("engine: SetMotionReduce без менеджера тем")
	}
	m.SetFlag(theme.FlagMotionReduce, on)
	return e.ApplyThemeProfile(m)
}

// SetAccent меняет акцент менеджера тем на лету и применяет тему к движку:
// компоненты перерисовываются без пересоздания. Эквивалент m.SetAccent(c) с
// последующим ApplyThemeProfile(m); акцент переживает смену профиля
// (SetThemeProfile).
func (e *Engine) SetAccent(m *theme.Manager, c color.RGBA) error {
	if m == nil {
		return fmt.Errorf("engine: SetAccent без менеджера тем")
	}
	m.SetAccent(c)
	return e.ApplyThemeProfile(m)
}
