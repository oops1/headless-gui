package desktop

import (
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
)

// Тесты пакета не должны зависеть от порядка запуска (go test -shuffle=on).
// Общее состояние процесса — тема и размер шрифта виджетов, режим «меньше
// движения», реестр анимаций, язык, размер экрана для меню — остаётся от
// теста к тесту, и тест, прошедший в одном порядке, падает в другом.
//
// Договорённость: каждый тест первой строкой зовёт testreset.Globals(t), а
// движок создаёт через testengine.New(t, …), который гасит его по окончании.
func TestHermetic_EveryTestResetsGlobals(t *testing.T) {
	testreset.Globals(t)
	testreset.RequireInEveryTest(t, 600)
}
