package desktop

import (
	"image"
	"sync"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Презентеры — ответ на «одна логика, разный вид».
//
// Токенами выражается палитра и геометрия, но не форма. Док macOS — не
// перекрашенная полоса кнопок: элементы стоят по центру на плавающей
// подложке, значок под курсором увеличивается, подписи нет вовсе. Переписать
// это цветами нельзя, а заводить второй компонент — значит раздвоить
// поведение: активацию, сворачивание, подписку на список окон пришлось бы
// поддерживать дважды.
//
// Поэтому тема приносит с собой презентер — чужую отрисовку и раскладку для
// известного ей компонента. Компонент остаётся один, его тесты на поведение
// проходят для обеих тем без изменений; меняется только тот, кто рисует.
//
// Компонент НЕ ИМЕЕТ ПРАВА знать имя активной темы. Он спрашивает: «есть ли
// для меня презентер», и если есть — отдаёт ему отрисовку.

// Presenter — чужая отрисовка компонента, которую приносит тема.
type Presenter interface {
	// Measure возвращает желаемый размер компонента при доступном месте.
	Measure(c Component, avail image.Point) image.Point
	// Layout возвращает прямоугольники ячеек в границах bounds — по одному
	// на ячейку, в том же порядке, что Cells().
	//
	// Компонент берёт их себе: по ним он считает попадание мыши. Иначе клик
	// шёл бы по собственной раскладке компонента, а картинка была бы чужой:
	// в доке со значками разного размера это расхождение сразу заметно.
	// nil означает «раскладывай сам».
	Layout(c Component, bounds image.Rectangle) []image.Rectangle
	// Draw рисует компонент целиком.
	Draw(ctx widget.DrawContext, c Component)
}

// Component — то, что презентер должен знать о компоненте, чтобы его
// нарисовать: границы, тема и содержимое.
//
// Содержимое отдаётся списком ячеек, а не типом компонента: презентер дока
// одинаково нарисует и кнопки окон, и закреплённые приложения, если те
// умеют рассказать о себе одинаково.
type Component interface {
	Bounds() image.Rectangle
	Theme() *theme.Manager
	// Cells возвращает содержимое компонента для отрисовки.
	Cells() []Cell
	// HoverIndex возвращает индекс ячейки под курсором (-1 — нет).
	HoverIndex() int
}

// Cell — одна ячейка содержимого: значок, подпись и состояние.
type Cell struct {
	Title  string
	Icon   image.Image
	Active bool
	Muted  bool // свёрнутое окно, отключённое приложение
}

var (
	presentersMu sync.RWMutex
	presenters   = map[string]Presenter{}
)

// RegisterPresenter регистрирует презентер под именем, которым его называет
// профиль темы (Profile.Presenters). Повторная регистрация заменяет.
//
// Реестр глобальный на процесс — как и реестр анимаций: презентер не
// хранит состояния, это чистая отрисовка.
func RegisterPresenter(name string, p Presenter) {
	if name == "" || p == nil {
		return
	}
	presentersMu.Lock()
	presenters[name] = p
	presentersMu.Unlock()
}

// PresenterFor возвращает презентер, который активная тема назначила
// компоненту (nil — рисовать самому).
func PresenterFor(tm *theme.Manager, component string) Presenter {
	if tm == nil {
		return nil
	}
	t := tm.Active()
	if t == nil {
		return nil
	}
	name := t.PresenterName(component)
	if name == "" {
		return nil
	}
	presentersMu.RLock()
	p := presenters[name]
	presentersMu.RUnlock()
	return p
}

// PresenterNames возвращает имена зарегистрированных презентеров
// (для диагностики и тестов).
func PresenterNames() []string {
	presentersMu.RLock()
	defer presentersMu.RUnlock()
	out := make([]string, 0, len(presenters))
	for n := range presenters {
		out = append(out, n)
	}
	return out
}
