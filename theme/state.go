package theme

// State — состояние компонента, в котором его просят нарисовать. Битовая
// маска, а не строка: состояния сочетаются (наведённая кнопка в фокусе —
// StateHover|StateFocused), а сравнение и объединение стоят одну операцию.
type State uint16

const (
	// StateHover — курсор над компонентом.
	StateHover State = 1 << iota
	// StateActive — компонент выбран или его окно активно (вкладка, пункт
	// меню, кнопка панели задач у окна на переднем плане).
	StateActive
	// StatePressed — кнопка нажата и ещё не отпущена.
	StatePressed
	// StateDisabled — компонент отключён и ввод не принимает.
	StateDisabled
	// StateFocused — компонент держит клавиатурный фокус.
	StateFocused
)

// StateNormal — покой: ни одного установленного бита.
const StateNormal State = 0

// statePriority — порядок разрешения состояний, от старшего к младшему.
//
// Порядок не произвольный: отключённый компонент выглядит отключённым, даже
// если курсор над ним; нажатие важнее наведения, потому что нажать, не
// наведя, нельзя; фокус — самое слабое, это рамка поверх любого другого
// вида. Тем самым набор битов всегда сводится к одному состоянию, для
// которого тема и объявляет стиль.
var statePriority = [...]State{
	StateDisabled,
	StatePressed,
	StateActive,
	StateHover,
	StateFocused,
}

// Dominant сводит набор состояний к одному — старшему по приоритету
// (Disabled > Pressed > Active > Hover > Focused). Пустой набор остаётся
// StateNormal.
//
// Благодаря этому таблица разрешённых стилей хранит по шесть записей на
// компонент, а не по тридцать две (все сочетания битов), и поиск стиля не
// требует перебора.
func (s State) Dominant() State {
	for _, candidate := range statePriority {
		if s&candidate != 0 {
			return candidate
		}
	}
	return StateNormal
}

// Has сообщает, установлен ли бит (или все биты набора).
func (s State) Has(flag State) bool { return s&flag == flag }

// String возвращает имя состояния для сообщений и JSON-профилей.
// Для набора битов — имя доминирующего.
func (s State) String() string {
	switch s.Dominant() {
	case StateDisabled:
		return "disabled"
	case StatePressed:
		return "pressed"
	case StateActive:
		return "active"
	case StateHover:
		return "hover"
	case StateFocused:
		return "focused"
	default:
		return "normal"
	}
}

// ParseState разбирает имя состояния (как в JSON-профиле). Пустая строка и
// "normal" дают StateNormal; неизвестное имя — ошибку.
func ParseState(name string) (State, error) {
	switch name {
	case "", "normal":
		return StateNormal, nil
	case "hover":
		return StateHover, nil
	case "active":
		return StateActive, nil
	case "pressed":
		return StatePressed, nil
	case "disabled":
		return StateDisabled, nil
	case "focused":
		return StateFocused, nil
	}
	return StateNormal, &UnknownStateError{Name: name}
}

// UnknownStateError — имя состояния, которого нет в модели.
type UnknownStateError struct{ Name string }

func (e *UnknownStateError) Error() string {
	return "theme: неизвестное состояние " + e.Name
}
