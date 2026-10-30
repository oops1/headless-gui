package desktop_test

import (
	"testing"

	"github.com/oops1/headless-gui/v4/desktop"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/theme"
)

// Одна модель «Не беспокоить» на центр, тост и кнопку трея: кнопка следует за
// переключениями модели сама, без пересылки потребителем.
func TestNotificationButton_FollowsDoNotDisturbModel(t *testing.T) {
	testreset.Globals(t)
	tm, _ := edgeManager(t, theme.ProfileWindows11)
	b := desktop.NewNotificationButton(tm, desktop.NewFakeNotifications())
	dnd := desktop.NewDoNotDisturb(true)
	b.BindDoNotDisturb(dnd)
	if !b.DoNotDisturb() {
		t.Fatal("кнопка не взяла начальное состояние модели")
	}
	dnd.SetEnabled(false)
	if b.DoNotDisturb() {
		t.Error("кнопка не заметила выключения режима")
	}
	b.Close()
	dnd.SetEnabled(true)
	if b.DoNotDisturb() {
		t.Error("после Close кнопка всё ещё подписана на модель")
	}
}
