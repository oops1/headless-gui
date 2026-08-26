// Тесты диалога ожидания (ShowBusy): вёрстка по центру, светящаяся полоса,
// управление из фоновой горутины.
package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// TestBusyDialog_ShowsAndCloses — диалог открывается модально, полоса
// стартует в неопределённом режиме, SetProgress переводит её на значение,
// Close закрывает (повторный вызов безопасен).
func TestBusyDialog_ShowsAndCloses(t *testing.T) {
	eng := engine.New(900, 600, 30)
	eng.SetRoot(widget.NewPanel(widget.DarkTheme().WindowBG))
	mb := widget.NewMessageBox(eng)

	bd := mb.ShowBusy("Обработка данных", "Пожалуйста, подождите…",
		"Не закрывайте это окно", nil)
	if bd == nil {
		t.Fatal("ShowBusy вернул nil")
	}
	if !bd.IsIndeterminate() {
		t.Error("полоса должна стартовать в неопределённом режиме")
	}

	bd.SetProgress(0.4)
	if bd.IsIndeterminate() {
		t.Error("SetProgress не перевёл полосу в определённый режим")
	}
	if got := bd.Progress(); got < 0.39 || got > 0.41 {
		t.Errorf("Progress() = %v, ждали 0.4", got)
	}

	// Строки меняются из любой горутины и не паникуют без процента/деталей.
	bd.SetTitle("Готово")
	bd.SetSubtitle("Почти всё")
	bd.SetHint("")
	bd.SetDetail("это поле у диалога ожидания отсутствует")

	bd.Close()
	bd.Close() // идемпотентно
}

// TestBusyDialog_CancelClosesAndNotifies — с onCancel диалог получает ✕ и
// Escape, а колбэк вызывается ровно один раз.
func TestBusyDialog_CancelClosesAndNotifies(t *testing.T) {
	eng := engine.New(900, 600, 30)
	eng.SetRoot(widget.NewPanel(widget.DarkTheme().WindowBG))
	mb := widget.NewMessageBox(eng)

	calls := 0
	bd := mb.ShowBusy("Работа", "", "", func() { calls++ })
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	if calls != 1 {
		t.Errorf("onCancel вызван %d раз, ждали 1", calls)
	}
	bd.Close()
	if calls != 1 {
		t.Errorf("после Close onCancel вызван ещё раз: %d", calls)
	}
}
