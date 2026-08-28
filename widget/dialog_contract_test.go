package widget

import "testing"

// closeBtnIn возвращает кнопку ✕ из среза детей диалога (или nil).
func closeBtnIn(kids []Widget) *dialogCloseBtn {
	for _, c := range kids {
		if cb, ok := c.(*dialogCloseBtn); ok {
			return cb
		}
	}
	return nil
}

// TestDialog_CloseButtonVisibility_NoDraw проверяет, что видимость кнопки ✕
// верна сразу после изменения ShowCloseButton — БЕЗ единого вызова Draw.
// Раньше видимость синхронизировалась только внутри Draw
// (d.closeBtn.SetVisible(d.ShowCloseButton)); после того как движок научился
// пропускать Draw для поддеревьев вне изменившейся области (SkipSubtree),
// кнопка могла остаться в старом состоянии для hit-теста, который ходит по
// Widget.Children() — а не по факту отрисовки.
func TestDialog_CloseButtonVisibility_NoDraw(t *testing.T) {
	dlg := NewDialog("Заголовок", 200, 100)

	// Прямая запись в публичное поле — так делают dialog_busy.go/
	// dialog_progress.go (были) и может делать внешний код (XAML-загрузчик,
	// showcase, тесты приложений). Обратную совместимость с этим ломать
	// нельзя.
	dlg.ShowCloseButton = false

	cb := closeBtnIn(dlg.Children())
	if cb == nil {
		t.Fatalf("кнопка ✕ не найдена среди детей диалога")
	}
	if IsWidgetVisible(cb) {
		t.Fatalf("кнопка ✕ всё ещё видима после ShowCloseButton=false без единого вызова Draw")
	}

	// И обратно — тоже без Draw.
	dlg.ShowCloseButton = true
	cb = closeBtnIn(dlg.Children())
	if cb == nil || !IsWidgetVisible(cb) {
		t.Fatalf("кнопка ✕ не видима после ShowCloseButton=true без единого вызова Draw")
	}
}

// TestDialog_SetShowCloseButton_NoDraw проверяет предпочтительный метод
// SetShowCloseButton: он должен синхронизировать видимость сразу же, тоже
// без обращения к Draw.
func TestDialog_SetShowCloseButton_NoDraw(t *testing.T) {
	dlg := NewDialog("Заголовок", 200, 100)

	dlg.SetShowCloseButton(false)
	cb := closeBtnIn(dlg.Children())
	if cb == nil {
		t.Fatalf("кнопка ✕ не найдена среди детей диалога")
	}
	if IsWidgetVisible(cb) {
		t.Fatalf("SetShowCloseButton(false) не скрыл кнопку без Draw")
	}

	dlg.SetShowCloseButton(true)
	cb = closeBtnIn(dlg.Children())
	if cb == nil || !IsWidgetVisible(cb) {
		t.Fatalf("SetShowCloseButton(true) не показал кнопку без Draw")
	}
}

// TestDialog_CloseButtonVisibility_DefaultTrue — по умолчанию (NewDialog)
// кнопка ✕ видима и без единого Draw (сверяет, что тест выше не проходит
// случайно из-за общего false-состояния).
func TestDialog_CloseButtonVisibility_DefaultTrue(t *testing.T) {
	dlg := NewDialog("Заголовок", 200, 100)
	cb := closeBtnIn(dlg.Children())
	if cb == nil || !IsWidgetVisible(cb) {
		t.Fatalf("по умолчанию ShowCloseButton=true, кнопка должна быть видима без Draw")
	}
}
