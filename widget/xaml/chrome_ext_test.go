package xaml_test

// chrome_ext_test.go — внешние тесты загрузчика (xaml_test): им нужен тег
// <Window>, а его строитель живёт в widget/chrome, который сам импортирует
// xaml, — внутренние тесты пакета его импортировать не могут (цикл).
// Внутренности xaml внешним тестам отдают export_test.go и помощники
// внутренних тестов (SaveStrings, UseFakeField…).

import (
	"github.com/oops1/headless-gui/v3/widget/chrome"
	"github.com/oops1/headless-gui/v3/widget/xaml"
)

func init() {
	chrome.RegisterXAML(xaml.Default())
}
