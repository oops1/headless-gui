package desktop

import (
	"fmt"
	"image"
	"image/color"
	"sync"
	"time"
)

// Тестовые реализации системных интерфейсов.
//
// Это не заглушки «до появления настоящего» — они остаются навсегда. Без
// них пакет нельзя ни показать в демонстрации, ни покрыть golden-тестами:
// панель задач, которой нечего показывать, не панель. Настоящие данные
// приходят от потребителя, а эти отвечают за то, чтобы компонент можно было
// проверить в отрыве от любой системы.
//
// Все они безопасны для вызова из нескольких горутин и рассылают
// уведомления подписчикам при изменениях — как и полагается настоящим.

// ─── Окна ───────────────────────────────────────────────────────────────────

// FakeWindowModel — список окон, которым распоряжается тест.
type FakeWindowModel struct {
	mu      sync.Mutex
	windows []WindowInfo
	subs    map[int]func()
	nextSub int

	// Closed/Activated/Minimized — журнал вызовов: тест проверяет, что клик
	// по кнопке панели дошёл до модели, а не только перерисовал кнопку.
	Closed    []WindowID
	Activated []WindowID
	Minimized []WindowID
}

// NewFakeWindowModel создаёт модель с заданными окнами.
func NewFakeWindowModel(windows ...WindowInfo) *FakeWindowModel {
	return &FakeWindowModel{windows: windows, subs: map[int]func(){}}
}

// Windows возвращает копию списка.
func (m *FakeWindowModel) Windows() []WindowInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]WindowInfo(nil), m.windows...)
}

// SetWindows заменяет список и уведомляет подписчиков.
func (m *FakeWindowModel) SetWindows(windows []WindowInfo) {
	m.mu.Lock()
	m.windows = append([]WindowInfo(nil), windows...)
	m.mu.Unlock()
	m.notify()
}

// Activate помечает окно активным (и снимает признак у остальных).
func (m *FakeWindowModel) Activate(id WindowID) {
	m.mu.Lock()
	m.Activated = append(m.Activated, id)
	for i := range m.windows {
		m.windows[i].Active = m.windows[i].ID == id
		if m.windows[i].ID == id {
			m.windows[i].Minimized = false
		}
	}
	m.mu.Unlock()
	m.notify()
}

// Minimize сворачивает окно.
func (m *FakeWindowModel) Minimize(id WindowID) {
	m.mu.Lock()
	m.Minimized = append(m.Minimized, id)
	for i := range m.windows {
		if m.windows[i].ID == id {
			m.windows[i].Minimized = true
			m.windows[i].Active = false
		}
	}
	m.mu.Unlock()
	m.notify()
}

// Close убирает окно из списка.
func (m *FakeWindowModel) Close(id WindowID) {
	m.mu.Lock()
	m.Closed = append(m.Closed, id)
	out := m.windows[:0]
	for _, w := range m.windows {
		if w.ID != id {
			out = append(out, w)
		}
	}
	m.windows = out
	m.mu.Unlock()
	m.notify()
}

// Subscribe подписывает на изменения списка.
func (m *FakeWindowModel) Subscribe(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	m.mu.Lock()
	m.nextSub++
	id := m.nextSub
	m.subs[id] = fn
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		delete(m.subs, id)
		m.mu.Unlock()
	}
}

func (m *FakeWindowModel) notify() {
	m.mu.Lock()
	list := make([]func(), 0, len(m.subs))
	for _, fn := range m.subs {
		list = append(list, fn)
	}
	m.mu.Unlock()
	for _, fn := range list {
		fn()
	}
}

// ─── Каталог приложений ─────────────────────────────────────────────────────

// StaticAppCatalog — неизменный список приложений с закреплением в памяти.
type StaticAppCatalog struct {
	mu     sync.Mutex
	apps   []AppInfo
	pinned []AppID

	// Launched — журнал запусков для тестов.
	Launched []AppID
	// LaunchErr — если задана, Launch возвращает эту ошибку: тестам нужен и
	// путь, на котором запуск не удался.
	LaunchErr error
}

// NewStaticAppCatalog создаёт каталог.
func NewStaticAppCatalog(apps ...AppInfo) *StaticAppCatalog {
	return &StaticAppCatalog{apps: apps}
}

// Apps возвращает копию списка приложений.
func (c *StaticAppCatalog) Apps() []AppInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]AppInfo(nil), c.apps...)
}

// Pinned возвращает копию списка закреплённых.
func (c *StaticAppCatalog) Pinned() []AppID {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]AppID(nil), c.pinned...)
}

// Pin закрепляет приложение (повторное закрепление ничего не меняет).
func (c *StaticAppCatalog) Pin(id AppID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.pinned {
		if p == id {
			return
		}
	}
	c.pinned = append(c.pinned, id)
}

// Unpin снимает закрепление.
func (c *StaticAppCatalog) Unpin(id AppID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.pinned[:0]
	for _, p := range c.pinned {
		if p != id {
			out = append(out, p)
		}
	}
	c.pinned = out
}

// Launch записывает запуск в журнал и возвращает LaunchErr.
func (c *StaticAppCatalog) Launch(id AppID) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Launched = append(c.Launched, id)
	if c.LaunchErr != nil {
		return c.LaunchErr
	}
	for _, a := range c.apps {
		if a.ID == id {
			return nil
		}
	}
	return fmt.Errorf("desktop: приложение %q не найдено в каталоге", id)
}

// ─── Показатели системы ─────────────────────────────────────────────────────

// FakeSystemStatus — сеть, звук и питание, которыми распоряжается тест.
type FakeSystemStatus struct {
	mu      sync.Mutex
	net     NetState
	vol     VolState
	power   PowerState
	subs    map[int]func()
	nextSub int
}

// NewFakeSystemStatus создаёт показатели с разумными значениями: Wi-Fi с
// хорошим сигналом, звук на две трети, батарея наполовину от сети.
func NewFakeSystemStatus() *FakeSystemStatus {
	return &FakeSystemStatus{
		net:   NetState{Kind: NetWiFi, Quality: 0.8, Name: "Сеть"},
		vol:   VolState{Level: 0.65},
		power: PowerState{Charge: 0.5, OnAC: true},
		subs:  map[int]func(){},
	}
}

// Network, Volume, Power возвращают текущие значения.
func (s *FakeSystemStatus) Network() NetState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.net
}

func (s *FakeSystemStatus) Volume() VolState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vol
}

func (s *FakeSystemStatus) Power() PowerState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.power
}

// SetNetwork, SetVolume, SetPower меняют значения и уведомляют подписчиков.
func (s *FakeSystemStatus) SetNetwork(n NetState) {
	s.mu.Lock()
	s.net = n
	s.mu.Unlock()
	s.notify()
}

func (s *FakeSystemStatus) SetVolume(v VolState) {
	s.mu.Lock()
	s.vol = v
	s.mu.Unlock()
	s.notify()
}

func (s *FakeSystemStatus) SetPower(p PowerState) {
	s.mu.Lock()
	s.power = p
	s.mu.Unlock()
	s.notify()
}

// Subscribe подписывает на изменения показателей.
func (s *FakeSystemStatus) Subscribe(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	s.mu.Lock()
	s.nextSub++
	id := s.nextSub
	s.subs[id] = fn
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.subs, id)
		s.mu.Unlock()
	}
}

func (s *FakeSystemStatus) notify() {
	s.mu.Lock()
	list := make([]func(), 0, len(s.subs))
	for _, fn := range s.subs {
		list = append(list, fn)
	}
	s.mu.Unlock()
	for _, fn := range list {
		fn()
	}
}

// ─── Уведомления ────────────────────────────────────────────────────────────

// FakeNotifications — центр уведомлений в памяти.
type FakeNotifications struct {
	mu      sync.Mutex
	list    []Notification
	subs    map[int]func()
	nextSub int
	nextID  NotificationID
}

// NewFakeNotifications создаёт пустой центр уведомлений.
func NewFakeNotifications() *FakeNotifications {
	return &FakeNotifications{subs: map[int]func(){}}
}

// List возвращает копию списка.
func (n *FakeNotifications) List() []Notification {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]Notification(nil), n.list...)
}

// Add добавляет уведомление и возвращает его идентификатор.
func (n *FakeNotifications) Add(note Notification) NotificationID {
	n.mu.Lock()
	n.nextID++
	note.ID = n.nextID
	if note.Time.IsZero() {
		note.Time = time.Now()
	}
	n.list = append(n.list, note)
	n.mu.Unlock()
	n.notify()
	return note.ID
}

// Dismiss убирает уведомление.
func (n *FakeNotifications) Dismiss(id NotificationID) {
	n.mu.Lock()
	out := n.list[:0]
	for _, note := range n.list {
		if note.ID != id {
			out = append(out, note)
		}
	}
	n.list = out
	n.mu.Unlock()
	n.notify()
}

// Subscribe подписывает на изменения списка.
func (n *FakeNotifications) Subscribe(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	n.mu.Lock()
	n.nextSub++
	id := n.nextSub
	n.subs[id] = fn
	n.mu.Unlock()
	return func() {
		n.mu.Lock()
		delete(n.subs, id)
		n.mu.Unlock()
	}
}

func (n *FakeNotifications) notify() {
	n.mu.Lock()
	list := make([]func(), 0, len(n.subs))
	for _, fn := range n.subs {
		list = append(list, fn)
	}
	n.mu.Unlock()
	for _, fn := range list {
		fn()
	}
}

// ─── Часы ───────────────────────────────────────────────────────────────────

// FakeClock — часы с заданным временем. Нужны golden-тестам: панель с
// настоящими часами пришлось бы переснимать каждую минуту.
type FakeClock struct {
	mu sync.Mutex
	t  time.Time
}

// NewFakeClock создаёт часы, показывающие t.
func NewFakeClock(t time.Time) *FakeClock { return &FakeClock{t: t} }

// Now возвращает заданное время.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Set переводит часы.
func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

// Advance двигает часы вперёд.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// ─── Миниатюры окон ──────────────────────────────────────────────────────────

// FakeWindowPreviews — модель окон, умеющая отдавать миниатюры.
//
// Отдельный тип, а не метод у FakeWindowModel: предпросмотр даётся модели по
// необязательному интерфейсу, и должна остаться модель БЕЗ него — иначе
// проверить поведение движка с такой моделью было бы нечем.
type FakeWindowPreviews struct {
	*FakeWindowModel
}

// NewFakeWindowPreviews создаёт модель с миниатюрами.
func NewFakeWindowPreviews(windows ...WindowInfo) *FakeWindowPreviews {
	return &FakeWindowPreviews{FakeWindowModel: NewFakeWindowModel(windows...)}
}

// Preview рисует узнаваемую заглушку: полосы своего для каждого окна цвета.
// Настоящая оболочка отдаёт здесь снимок участка холста.
func (f *FakeWindowPreviews) Preview(id WindowID, max image.Point) image.Image {
	if max.X <= 0 || max.Y <= 0 {
		return nil
	}
	img := image.NewRGBA(image.Rect(0, 0, max.X, max.Y))
	base := color.RGBA{
		R: uint8(40 + 60*(id%3)),
		G: uint8(70 + 50*((id+1)%3)),
		B: uint8(110 + 40*((id+2)%3)),
		A: 255,
	}
	for y := 0; y < max.Y; y++ {
		row := base
		if (y/8)%2 == 0 {
			row = fadeRGBA(base, 0.8)
		}
		for x := 0; x < max.X; x++ {
			img.SetRGBA(x, y, row)
		}
	}
	return img
}

// fadeRGBA затемняет цвет — заглушке довольно двух оттенков.
func fadeRGBA(c color.RGBA, k float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(c.R) * k),
		G: uint8(float64(c.G) * k),
		B: uint8(float64(c.B) * k),
		A: c.A,
	}
}
