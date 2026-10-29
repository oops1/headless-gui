package xaml

// builders.go — реестр строителей тегов (ADR-0012 §4). Пакеты виджетов вне
// xaml (widget/text и следующие этапы) регистрируют свои теги функцией
// RegisterXAML(r *Registry) — явно, не в init: программа, которая XAML не
// грузит, строителей в бинаре не держит. Фасад widget регистрирует все
// пакеты при первой загрузке разметки; программа на widget/base и
// widget/xaml регистрирует те пакеты, что импортировала. Загрузчик
// спрашивает реестр Default() раньше пользовательских тегов
// (RegisterXAMLWidget).

import (
	"image"
	"strings"
	"sync"

	"github.com/oops1/headless-gui/v3/widget/base"
)

// Builder строит виджет по элементу разметки. Общее для всех тегов —
// границы, Width/Height, attached-свойства, имя — загрузчик применяет к
// результату сам. nil без ошибки — элемент пропускается.
//
//facadegen:skip
type Builder func(el *Element, l *Loader) (base.Widget, error)

// Registry — теги разметки и их строители.
//
//facadegen:skip
type Registry struct {
	mu sync.RWMutex
	m  map[string]entry
}

// entry — строитель тега; complete — строитель сам делает всё (дети, имя,
// свойства), загрузчик результат не дообрабатывает.
type entry struct {
	b        Builder
	complete bool
}

// NewRegistry — пустой реестр.
//
//facadegen:skip
func NewRegistry() *Registry { return &Registry{m: map[string]entry{}} }

var defaultRegistry = NewRegistry()

// Default — реестр, которым строят виджеты LoadUIFromXAML*.
//
//facadegen:skip
func Default() *Registry { return defaultRegistry }

// Register связывает теги (без учёта регистра) со строителем b; повторная
// регистрация тега заменяет прежнюю.
func (r *Registry) Register(b Builder, tags ...string) {
	r.register(entry{b: b}, tags)
}

// RegisterComplete связывает теги со строителем-контейнером, который сам
// строит детей (Loader.BuildChild), задаёт имя (Loader.SetName), границы и
// свойства: загрузчик его результат не дообрабатывает (корневое окно).
func (r *Registry) RegisterComplete(b Builder, tags ...string) {
	r.register(entry{b: b, complete: true}, tags)
}

func (r *Registry) register(e entry, tags []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range tags {
		r.m[strings.ToLower(t)] = e
	}
}

// Unregister убирает теги из реестра.
func (r *Registry) Unregister(tags ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range tags {
		delete(r.m, strings.ToLower(t))
	}
}

// Lookup — строитель тега (без учёта регистра).
func (r *Registry) Lookup(tag string) (Builder, bool) {
	e, ok := r.lookup(tag)
	return e.b, ok
}

func (r *Registry) lookup(tag string) (entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.m[strings.ToLower(tag)]
	return e, ok
}

// Loader — загрузка, в которой строится элемент: то, что строителю нужно
// от неё, кроме самого элемента.
//
//facadegen:skip
type Loader struct {
	baseDir string
	reg     map[string]base.Widget
	off     image.Point
	depth   int
}

// Offset — смещение родителя: координаты элемента в разметке относительны
// ему.
func (l *Loader) Offset() image.Point { return l.off }

// BuildChild строит дочерний элемент el со смещением родителя off (для
// строителей-контейнеров, RegisterComplete): тем же загрузчиком, с
// регистрацией имён и пределом вложенности.
func (l *Loader) BuildChild(el *Element, off image.Point) (base.Widget, error) {
	return buildXAMLWidgetAt(*el, l.reg, off, l.baseDir, l.depth+1)
}

// BuildPopupMenu строит меню из элемента el (<TrayMenu>, <TitleTabsMenu> окна, контекстное
// меню) со смещением off.
func (l *Loader) BuildPopupMenu(el *Element, off image.Point) (base.Widget, error) {
	return buildXAMLPopupMenu(*el, l.reg, off, l.baseDir)
}

// SetName регистрирует виджет под именем разметки (x:Name).
func (l *Loader) SetName(name string, w base.Widget) { l.reg[name] = w }

// BaseDir — каталог разметки (или синтетический каталог fs.FS из
// LoadUIFromXAMLFS), от которого считаются пути ресурсов.
func (l *Loader) BaseDir() string { return l.baseDir }

// Image декодирует картинку ресурса src (путь относительно разметки; граница
// каталога и пределы размера — как у прочих ресурсов, base.OpenResource).
func (l *Loader) Image(src string) (image.Image, error) {
	return decodeXAMLImage(l.baseDir, src)
}
