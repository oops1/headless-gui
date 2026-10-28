// Package testreset — изоляция тестов от глобального состояния процесса.
//
// Пакеты widget, theme и engine держат часть состояния на весь процесс:
// активную тему и размер шрифта, режим «меньше движения», размер экрана для
// меню, реестр анимаций, язык интерфейса, приёмники уведомлений. Тест, который
// это состояние поменял и не вернул, ломает тесты, идущие после него, и
// ломает только при определённом порядке: go test -shuffle=on падал на
// разных тестах в зависимости от seed.
//
// Globals(t) вызывается первой строкой теста: он приводит всё общее состояние
// к исходному (как у свежего процесса) и ставит t.Cleanup, который делает то
// же после теста. Тест, которому нужно другое состояние, задаёт его сам —
// после Globals, а не полагается на то, что оставил предыдущий.
package testreset

import (
	"image"
	"testing"

	hgtext "github.com/oops1/headless-gui/v3/core/text"
	"github.com/oops1/headless-gui/v3/internal/testhook"
	"github.com/oops1/headless-gui/v3/widget"
)

// Globals сбрасывает общее состояние процесса к исходному сейчас и после теста.
func Globals(t testing.TB) {
	t.Helper()
	Reset()
	t.Cleanup(Reset)
}

// Reset приводит общее состояние пакета widget к тому, с каким стартует
// процесс. Для TestMain и помощников; тестам достаточно Globals.
func Reset() {
	// Анимации: незавершённые (часы, переходы) живут в общем реестре и будят
	// чужие проверки AnimationsActive.
	widget.StopAllAnimations()
	widget.SetReduceMotion(false)

	// Политика сбоев: сломанные виджеты и стек рисующихся общие на процесс;
	// паника, которую тест проверял с FailurePropagate, оставляет стек.
	widget.ClearAllBroken()
	widget.ResetDrawTrace()

	// Размер шрифта — до темы: ApplyGlobalTheme снимает только тот размер,
	// который поставила тема, а явный SetDefaultFontSize ей не принадлежит.
	widget.SetDefaultFontSize(0)
	widget.ApplyGlobalTheme(widget.DarkTheme())

	// Экран и рабочая область, на которых клампятся меню.
	widget.SetScreenBounds(0, 0)
	widget.SetPopupWorkArea(image.Rectangle{})
	widget.SetPopupsHosted(false)

	// Внешние слоты уведомлений (тесты подменяют их без движка).
	widget.SetUIChangeNotifier(nil)
	widget.SetUIRectChangeNotifier(nil)

	// Язык интерфейса и раскладка.
	if testhook.ResetLanguage != nil {
		testhook.ResetLanguage()
	}
	widget.SetFallbackLanguage("EN")
	widget.SetLocale("EN")

	// Общий кэш растров глифов: бюджет и записи (индекс шрифтов остаётся —
	// шрифт неизменяем, а записи на результат не влияют, только на промахи).
	hgtext.SetGlyphCacheBudget(hgtext.DefaultGlyphCacheBudget)
	hgtext.ResetGlyphCache()
}
