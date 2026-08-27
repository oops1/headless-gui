// Package theme — темы оформления как данные: профили токенов, наследование
// между ними и менеджер, который выдаёт разрешённые стили.
//
// Пакет намеренно ничего не знает о виджетах и не импортирует widget:
// зависимость идёт в одну сторону (widget → theme), иначе получился бы цикл.
// Поэтому все типы здесь — примитивы (color.RGBA, float64, строки).
//
// Как это устроено:
//
//	Profile  — тема как данные: токены и правила по компонентам, плюс имя
//	           родителя, у которого берётся всё незаявленное.
//	Theme    — разрешённый профиль: цепочка наследования пройдена, откаты
//	           по состояниям посчитаны, поиск стиля — одно обращение к карте.
//	Manager  — реестр профилей, активная тема, подписка на смену.
//
// Разрешение выполняется один раз при SetTheme. Отрисовка только читает
// готовые таблицы и не аллоцирует.
package theme

import (
	"fmt"
	"image"
	"sync"
)

// Observer получает уведомление о смене активной темы. Наблюдатели нужны
// тем, кто живёт ВНЕ дерева виджетов (оболочка, отдельные окна, портал
// оформления для чужих приложений) — дерево движок обходит сам.
type Observer interface {
	ThemeChanged(t *Theme)
}

// ObserverFunc позволяет подписаться функцией.
type ObserverFunc func(t *Theme)

// ThemeChanged реализует Observer.
func (f ObserverFunc) ThemeChanged(t *Theme) { f(t) }

// IconResolver превращает ссылку на иконку в изображение нужного размера.
// Реализация живёт вне этого пакета (растеризация SVG — дело движка);
// менеджер только адресует.
type IconResolver interface {
	ResolveIcon(ref IconRef, size int) image.Image
}

// Manager — реестр тем и держатель активной.
//
// Все методы безопасны для вызова из нескольких горутин. Горячий путь —
// GetStyle/GetMetric — берёт активную тему под RLock и дальше читает уже
// неизменяемую структуру.
type Manager struct {
	mu       sync.RWMutex
	profiles map[string]*Profile
	resolved map[string]*Theme // кэш разрешённых тем
	active   *Theme

	icons IconResolver

	obsMu   sync.Mutex
	nextID  uint64
	observs map[uint64]Observer
}

// NewManager создаёт пустой менеджер.
func NewManager() *Manager {
	return &Manager{
		profiles: map[string]*Profile{},
		resolved: map[string]*Theme{},
		observs:  map[uint64]Observer{},
	}
}

// RegisterTheme добавляет профиль в реестр. Профиль с тем же именем
// заменяется, а разрешённые темы, зависящие от него, сбрасываются —
// правка родителя обязана дойти до потомков.
//
// Родитель может быть зарегистрирован позже: связь проверяется при
// разрешении, а не здесь, — иначе порядок регистрации диктовал бы порядок
// объявления тем.
func (m *Manager) RegisterTheme(p *Profile) error {
	if p == nil {
		return fmt.Errorf("theme: RegisterTheme(nil)")
	}
	if p.Name == "" {
		return fmt.Errorf("theme: у профиля пустое имя")
	}
	if p.Name == p.Parent {
		return fmt.Errorf("theme: профиль %q объявлен своим же родителем", p.Name)
	}

	m.mu.Lock()
	m.profiles[p.Name] = p
	// Кэш разрешённых тем больше не достоверен: профиль мог быть чьим-то
	// предком. Кэш дешёвый — пересобрать проще, чем отследить зависимости.
	m.resolved = map[string]*Theme{}
	activeName := ""
	if m.active != nil {
		activeName = m.active.name
	}
	m.mu.Unlock()

	// Активную тему пересобираем сразу: она уже у кого-то на руках.
	if activeName != "" {
		if err := m.SetTheme(activeName); err != nil {
			return fmt.Errorf("theme: профиль %q зарегистрирован, но активная тема %q больше не собирается: %w",
				p.Name, activeName, err)
		}
	}
	return nil
}

// GetTheme возвращает разрешённую тему по имени профиля, собирая её при
// первом обращении.
func (m *Manager) GetTheme(name string) (*Theme, bool) {
	m.mu.RLock()
	if t, ok := m.resolved[name]; ok {
		m.mu.RUnlock()
		return t, true
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.resolved[name]; ok { // мог собрать сосед, пока ждали
		return t, true
	}
	t, err := resolve(name, m.profiles)
	if err != nil {
		return nil, false
	}
	m.resolved[name] = t
	return t, true
}

// SetTheme делает тему активной и уведомляет наблюдателей.
func (m *Manager) SetTheme(name string) error {
	m.mu.Lock()
	t, ok := m.resolved[name]
	if !ok {
		var err error
		t, err = resolve(name, m.profiles)
		if err != nil {
			m.mu.Unlock()
			return err
		}
		m.resolved[name] = t
	}
	m.active = t
	m.mu.Unlock()

	m.notify(t)
	return nil
}

// Active возвращает активную тему (nil, если SetTheme ещё не звали).
func (m *Manager) Active() *Theme {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// ThemeNames возвращает имена зарегистрированных профилей.
func (m *Manager) ThemeNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.profiles))
	for n := range m.profiles {
		names = append(names, n)
	}
	return names
}

// UnloadTheme убирает профиль из реестра.
//
// Отказывает, если тема активна или является родителем другого профиля:
// в обоих случаях удаление оставило бы висящую ссылку — активную тему
// стало бы нечем пересобрать, а потомка нечем разрешить.
func (m *Manager) UnloadTheme(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.profiles[name]; !ok {
		return fmt.Errorf("theme: профиль %q не зарегистрирован", name)
	}
	if m.active != nil && m.active.name == name {
		return fmt.Errorf("theme: профиль %q активен — сначала переключитесь на другую тему", name)
	}
	for _, p := range m.profiles {
		if p.Parent == name {
			return fmt.Errorf("theme: профиль %q — родитель %q", name, p.Name)
		}
	}

	delete(m.profiles, name)
	delete(m.resolved, name)
	return nil
}

// SetIconResolver задаёт разрешатель иконок (растеризацию SVG выполняет
// движок, а не этот пакет).
func (m *Manager) SetIconResolver(r IconResolver) {
	m.mu.Lock()
	m.icons = r
	m.mu.Unlock()
}

// ─── Горячий путь: чтение активной темы ─────────────────────────────────────

// GetStyle возвращает стиль (компонент, часть, состояние) из активной темы.
// Никогда не возвращает nil: без активной темы отдаёт встроенный стиль по
// умолчанию, чтобы отсутствие темы не роняло отрисовку.
func (m *Manager) GetStyle(component, part string, state State) *Style {
	m.mu.RLock()
	t := m.active
	m.mu.RUnlock()
	if t == nil {
		return &emptyStyle
	}
	return t.Style(component, part, state)
}

// GetMetric возвращает метрику активной темы (0, если не задана).
func (m *Manager) GetMetric(k Key) float64 {
	m.mu.RLock()
	t := m.active
	m.mu.RUnlock()
	if t == nil {
		return 0
	}
	return t.MetricOr(k, 0)
}

// GetAnimation возвращает анимацию активной темы. Незаданная анимация —
// нулевая длительность, то есть «мгновенно»: так классические темы
// отключают движение, не сообщая об этом компонентам.
func (m *Manager) GetAnimation(k Key) AnimSpec {
	m.mu.RLock()
	t := m.active
	m.mu.RUnlock()
	if t == nil {
		return AnimSpec{}
	}
	a, _ := t.Anim(k)
	return a
}

// GetIcon возвращает иконку активной темы нужного размера. nil — иконки
// нет либо разрешатель не задан; вызывающий обязан пережить nil (в
// компонентах для этого есть заглушечный глиф).
func (m *Manager) GetIcon(name string, size int) image.Image {
	m.mu.RLock()
	t, r := m.active, m.icons
	m.mu.RUnlock()
	if t == nil || r == nil {
		return nil
	}
	ref, ok := t.Icon(Key(name))
	if !ok {
		return nil
	}
	return r.ResolveIcon(ref, size)
}

// emptyStyle — стиль, отдаваемый до установки темы. Общий на процесс и
// неизменяемый по соглашению (как и любой стиль из таблицы).
var emptyStyle = Style{Text: RGB(0, 0, 0)}

// ─── Подписка ───────────────────────────────────────────────────────────────

// Subscribe подписывает наблюдателя на смену темы и возвращает функцию
// отписки. Забытая отписка удерживает наблюдателя в памяти — отписывайтесь
// там же, где освобождаете остальное состояние.
func (m *Manager) Subscribe(o Observer) (unsubscribe func()) {
	if o == nil {
		return func() {}
	}
	m.obsMu.Lock()
	m.nextID++
	id := m.nextID
	m.observs[id] = o
	m.obsMu.Unlock()

	return func() {
		m.obsMu.Lock()
		delete(m.observs, id)
		m.obsMu.Unlock()
	}
}

// ObserverCount — сколько наблюдателей подписано (для тестов на утечку).
func (m *Manager) ObserverCount() int {
	m.obsMu.Lock()
	defer m.obsMu.Unlock()
	return len(m.observs)
}

// notify рассылает уведомление вне мьютекса реестра: наблюдатель вправе
// в ответ спросить у менеджера стиль, и держать при этом его же замок —
// верный способ получить взаимную блокировку.
func (m *Manager) notify(t *Theme) {
	m.obsMu.Lock()
	list := make([]Observer, 0, len(m.observs))
	for _, o := range m.observs {
		list = append(list, o)
	}
	m.obsMu.Unlock()

	for _, o := range list {
		o.ThemeChanged(t)
	}
}
