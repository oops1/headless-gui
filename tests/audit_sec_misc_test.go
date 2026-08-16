package tests

// Регрессионные тесты мелких находок аудита безопасности (SEC-11/12/14):
// отписка от коллекции, пароль вне undo-истории, тема под замком кадра.

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	dg "github.com/oops1/headless-gui/v3/widget/datagrid"
)

// TestObservableCollectionUnsubscribe — подписка снимается по дескриптору,
// а не «последняя добавленная»: чужой обработчик остаётся на месте.
func TestObservableCollectionUnsubscribe(t *testing.T) {
	oc := dg.NewObservableCollection()
	var a, b int
	ida := oc.AddCollectionChanged(func(dg.CollectionChangedEvent) { a++ })
	idb := oc.AddCollectionChanged(func(dg.CollectionChangedEvent) { b++ })
	if oc.HandlerCount() != 2 {
		t.Fatalf("подписчиков %d, want 2", oc.HandlerCount())
	}
	oc.Add(1)
	if a != 1 || b != 1 {
		t.Fatalf("после Add: a=%d b=%d", a, b)
	}

	oc.RemoveCollectionChanged(ida) // снимаем ПЕРВОГО — второй должен жить
	oc.Add(2)
	if a != 1 || b != 2 {
		t.Errorf("после отписки a: a=%d b=%d (ожидалось 1/2)", a, b)
	}
	oc.RemoveCollectionChanged(ida) // повторно — no-op
	oc.RemoveCollectionChanged(-1)  // мусорный id — no-op
	oc.RemoveCollectionChanged(idb)
	oc.Add(3)
	if b != 2 || oc.HandlerCount() != 0 {
		t.Errorf("после отписки b: b=%d подписчиков=%d", b, oc.HandlerCount())
	}
	if oc.AddCollectionChanged(nil) != -1 {
		t.Error("nil-обработчик должен возвращать -1")
	}
}

// TestPropertyNotifierRemoveByFunc — RemovePropertyChanged снимает именно
// переданный обработчик, а не последний добавленный.
func TestPropertyNotifierRemoveByFunc(t *testing.T) {
	var pn dg.PropertyNotifier
	var first, second int
	h1 := func(any, string) { first++ }
	h2 := func(any, string) { second++ }
	pn.AddPropertyChanged(h1)
	pn.AddPropertyChanged(h2)

	pn.RemovePropertyChanged(h1) // раньше снял бы h2 (последний)
	pn.NotifyPropertyChanged(nil, "X")
	if first != 0 || second != 1 {
		t.Errorf("first=%d second=%d — снят не тот обработчик", first, second)
	}
}

// TestPasswordNotInUndoHistory — в режиме пароля Ctrl+Z после очистки не
// возвращает пароль, а прежние руны затираются.
func TestPasswordNotInUndoHistory(t *testing.T) {
	eng := engine.New(300, 100, 30)
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 300, 100))
	ti := widget.NewPasswordInput("")
	ti.SetBounds(image.Rect(10, 10, 200, 40))
	root.AddChild(ti)
	eng.SetRoot(root)
	eng.SetFocus(ti)

	for _, r := range "secret" {
		eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyCode(r), Rune: r, Pressed: true})
	}
	if ti.GetText() != "secret" {
		t.Fatalf("набор не сработал: %q", ti.GetText())
	}
	ti.SetText("")

	// Ctrl+Z — история пуста, пароль не возвращается.
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyZ, Mod: widget.ModCtrl, Pressed: true})
	if got := ti.GetText(); got != "" {
		t.Errorf("Ctrl+Z восстановил пароль: %q", got)
	}
}

// TestSetThemeUnderFrameLock — смена темы во время активного рендера не
// вызывает гонок (запускать под -race): палитра меняется под frameMu.
func TestSetThemeUnderFrameLock(t *testing.T) {
	eng := engine.New(200, 120, 60)
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 200, 120))
	for i := 0; i < 20; i++ {
		b := widget.NewButton("Кнопка")
		b.SetBounds(image.Rect(10, 5*i, 150, 5*i+4))
		root.AddChild(b)
	}
	eng.SetRoot(root)
	eng.SetRenderOnDemand(false) // кадры идут постоянно — рендер активен
	eng.Start()
	defer eng.Stop()

	names := widget.ThemeNames()
	for i := 0; i < 30; i++ {
		if th := widget.ThemeByName(names[i%len(names)]); th != nil {
			eng.SetTheme(th)
		}
	}
}
