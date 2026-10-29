package xaml_test

// chrome_ext_test.go — внешние тесты загрузчика (xaml_test): им нужны теги
// <Window> и дока, а их строители живут в widget/chrome и widget/dock,
// которые сами импортируют xaml, — внутренние тесты пакета их импортировать
// не могут (цикл).
// Внутренности xaml внешним тестам отдают export_test.go и экспортированные
// помощники внутренних тестовых файлов пакета (SaveStrings в testutil_test.go,
// UseFakeField в fieldfake_test.go).

import (
	"github.com/oops1/headless-gui/v3/widget/chrome"
	"github.com/oops1/headless-gui/v3/widget/dock"
	"github.com/oops1/headless-gui/v3/widget/xaml"
)

func init() {
	chrome.RegisterXAML(xaml.Default())
	dock.RegisterXAML(xaml.Default())
}
