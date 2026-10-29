package widget

// xaml_register.go — регистрация XAML-тегов пакетов виджетов (ADR-0012 §4).
// Обёртки LoadUIFromXAML* (xaml_gen.go) зовут registerXAML перед загрузкой:
// реестр xaml о пакетах виджетов не знает, а регистрация не в init —
// программа, которая XAML не грузит, строителей в бинаре не держит.

import (
	"sync"

	"github.com/oops1/headless-gui/v3/widget/richtext"
	"github.com/oops1/headless-gui/v3/widget/text"
	"github.com/oops1/headless-gui/v3/widget/xaml"
)

var xamlOnce sync.Once

// registerXAML регистрирует теги всех пакетов виджетов в xaml.Default() —
// один раз, при первой загрузке разметки.
func registerXAML() {
	xamlOnce.Do(func() {
		text.RegisterXAML(xaml.Default())
		richtext.RegisterXAML(xaml.Default())
	})
}
