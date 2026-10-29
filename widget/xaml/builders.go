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
	m  map[string]Builder
}

// NewRegistry — пустой реестр.
//
//facadegen:skip
func NewRegistry() *Registry { return &Registry{m: map[string]Builder{}} }

var defaultRegistry = NewRegistry()

// Default — реестр, которым строят виджеты LoadUIFromXAML*.
//
//facadegen:skip
func Default() *Registry { return defaultRegistry }

// Register связывает теги (без учёта регистра) со строителем b; повторная
// регистрация тега заменяет прежнюю.
func (r *Registry) Register(b Builder, tags ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range tags {
		r.m[strings.ToLower(t)] = b
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
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.m[strings.ToLower(tag)]
	return b, ok
}

// Loader — загрузка, в которой строится элемент: то, что строителю нужно
// от неё, кроме самого элемента.
//
//facadegen:skip
type Loader struct {
	baseDir string
}

// BaseDir — каталог разметки (или синтетический каталог fs.FS из
// LoadUIFromXAMLFS), от которого считаются пути ресурсов.
func (l *Loader) BaseDir() string { return l.baseDir }

// Image декодирует картинку ресурса src (путь относительно разметки; граница
// каталога и пределы размера — как у прочих ресурсов, base.OpenResource).
func (l *Loader) Image(src string) (image.Image, error) {
	return decodeXAMLImage(l.baseDir, src)
}
