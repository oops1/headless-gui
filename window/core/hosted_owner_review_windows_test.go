//go:build windows

package core

// hosted_owner_review_windows_test.go — случай из ревью 5.4: живое окно Win32
// в тесте не оставляет опрос раскладки, менявший глобальную локаль соседей.

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/widget"
)

// Пробный тест ревью 5.4: живое окно Win32 из szWin32Window оставляет
// localePoll (окно не закрыто через tearDown) — после теста опрос раскладки
// продолжает менять глобальную локаль и будить все движки процесса.
func TestHostedReview_LeakedLocalePollAfterLiveWindow(t *testing.T) {
	t.Run("live", func(t *testing.T) { szWin32Window(t, "1", 400, 300) })
	widget.SetLocale("zz-ZZ")
	set := widget.Locale()
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if widget.Locale() != set {
			t.Errorf("после теста с живым окном локаль сменил чужой опрос: %q", widget.Locale())
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Log("локаль не менялась (раскладка ОС совпала или опрос остановлен)")
}

// Тот же утёкший опрос раскладки ломает тесты 5.4: после живого окна
// testreset.Globals сбрасывает локаль, опрос через ≤300 мс возвращает
// раскладку ОС — широковещание будит главный движок посреди «тишины».
func TestHostedReview_LiveWindowThenHostedTests(t *testing.T) {
	t.Run("live", func(t *testing.T) { szWin32Window(t, "1", 400, 300) })
	for i := 0; i < 5; i++ {
		t.Run("dialog", TestHostedDialog_BelongsToSecondaryEngine)
		t.Run("dock", TestHostedDockPane_FloatsAndReturns)
	}
}
