package tests

// Регрессионные тесты мелких находок аудита безопасности (SEC-11/12/14/17/18):
// отписка от коллекции, пароль вне undo-истории, тема под замком кадра,
// невалидные шрифты, геометрия PopupMenu под замком.

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
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

// ─── SEC-18: геометрия PopupMenu и каскад под geoMu ─────────────────────────

// TestPopupMenuShowWhileDrawing — Show/openChild из «событийной» горутины
// параллельно DrawOverlay/OverlayBounds из «рендер»-горутины: под -race без
// сообщений о гонке (раньше popupX/child писались и читались без замка).
func TestPopupMenuShowWhileDrawing(t *testing.T) {
	widget.SetScreenBounds(800, 600)
	m := widget.NewPopupMenu()
	m.SetItems([]widget.MenuItem{
		{Text: "Один"},
		{Text: "Каскад", SubItems: []widget.MenuItem{{Text: "А"}, {Text: "Б"}}},
		{Text: "Три"},
	})
	m.SetBounds(image.Rect(0, 0, 10, 10))

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { // «рендер»
		defer close(done)
		ctx := &recCtx{}
		for {
			select {
			case <-stop:
				return
			default:
			}
			m.DrawOverlay(ctx)
			_ = m.OverlayBounds()
			_ = m.Bounds()
		}
	}()
	// «события»: показать, навести на пункт с каскадом (открывает child),
	// увести, закрыть — много раз.
	for i := 0; i < 300; i++ {
		m.Show(20+i%50, 30)
		m.OnMouseMove(40, 30+2+30+15) // второй пункт → openChild
		m.OnMouseMove(40, 30+2+15)    // первый пункт → closeChild
		m.Close()
	}
	close(stop)
	<-done
}

// ─── SEC-17: невалидный шрифт не подменяется молча ──────────────────────────

// TestInvalidFontNotSilentlySubstituted — битые TTF-данные в RegisterFont/
// RegisterFallbackFont отклоняются, а не регистрируются как «ещё один
// Go Regular»; RegisterFallbackFontFile возвращает ошибку.
func TestInvalidFontNotSilentlySubstituted(t *testing.T) {
	eng := engine.New(64, 64, 10)
	defer eng.Stop()
	garbage := []byte("this is definitely not a font file")

	if eng.RegisterFallbackFont(garbage) {
		t.Fatal("RegisterFallbackFont(мусор) вернул true")
	}
	before := len(eng.AvailableFonts())
	eng.RegisterFont("Broken", garbage)
	if got := len(eng.AvailableFonts()); got != before {
		t.Fatalf("битый шрифт зарегистрирован: было %d, стало %d", before, got)
	}
	for _, n := range eng.AvailableFonts() {
		if n == "Broken" {
			t.Fatal("шрифт Broken попал в реестр")
		}
	}

	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.ttf")
	if err := os.WriteFile(bad, garbage, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := eng.RegisterFallbackFontFile(bad); err == nil {
		t.Fatal("RegisterFallbackFontFile(битый файл) вернул nil — ошибка «невалидный шрифт» недостижима")
	}
	if err := eng.RegisterFallbackFontFile(filepath.Join(dir, "missing.ttf")); err == nil {
		t.Fatal("RegisterFallbackFontFile(нет файла) вернул nil")
	}
}

// ─── SEC-11: подписки на CollectionView снимаются ───────────────────────────

// TestCollectionViewRemoveViewChanged — AddViewChangedHandle/RemoveViewChanged:
// снятие по дескриптору, чужой слушатель остаётся.
func TestCollectionViewRemoveViewChanged(t *testing.T) {
	oc := dg.NewObservableCollectionFrom([]interface{}{1, 2, 3})
	cv := widget.NewCollectionView(oc)
	var a, b int
	ida := cv.AddViewChangedHandle(func() { a++ })
	idb := cv.AddViewChangedHandle(func() { b++ })
	if cv.AddViewChangedHandle(nil) != -1 {
		t.Error("nil-слушатель должен давать -1")
	}
	cv.Refresh()
	if a != 1 || b != 1 {
		t.Fatalf("после Refresh a=%d b=%d", a, b)
	}
	cv.RemoveViewChanged(ida)
	cv.Refresh()
	if a != 1 || b != 2 {
		t.Errorf("после отписки a: a=%d b=%d (ожидалось 1/2)", a, b)
	}
	cv.RemoveViewChanged(ida) // повторно — no-op
	cv.RemoveViewChanged(idb)
	if cv.ViewHandlerCount() != 0 {
		t.Errorf("слушателей %d, want 0", cv.ViewHandlerCount())
	}
	// Dispose представления снимает его подписку на источник.
	if oc.HandlerCount() != 1 {
		t.Fatalf("подписок на источник %d, want 1", oc.HandlerCount())
	}
	cv.Dispose()
	if oc.HandlerCount() != 0 {
		t.Errorf("после Dispose подписок на источник %d, want 0", oc.HandlerCount())
	}
}

// TestBindingScopeDisposeUnsubscribesCollectionView — ItemsControl поверх
// CollectionView: Dispose снимает слушателя представления.
func TestBindingScopeDisposeUnsubscribesCollectionView(t *testing.T) {
	oc := dg.NewObservableCollectionFrom([]interface{}{
		&itemRow{Name: "Аня"}, &itemRow{Name: "Боб"},
	})
	cv := widget.NewCollectionView(oc)
	vm := &cvVM{People: cv} // cvVM из collectionview_items_test.go
	const xaml = `<Canvas xmlns="clr">
		<ItemsControl Name="lst" ItemsSource="{Binding People}">
			<ItemsControl.ItemTemplate>
				<DataTemplate>
					<TextBlock Text="{Binding Name}"/>
				</DataTemplate>
			</ItemsControl.ItemTemplate>
		</ItemsControl>
	</Canvas>`
	_, _, scope, err := widget.LoadUIFromXAMLBindings([]byte(xaml), vm)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cv.ViewHandlerCount() == 0 {
		t.Fatal("ItemsControl не подписался на представление — тест бессмыслен")
	}
	scope.Dispose()
	if got := cv.ViewHandlerCount(); got != 0 {
		t.Errorf("после Dispose слушателей представления %d, want 0", got)
	}
}

// TestVICRebindCollectionView — повторный BindCollectionView снимает
// прежнюю подписку (иначе N перепривязок = N живых замыканий и N SetItems).
func TestVICRebindCollectionView(t *testing.T) {
	cv1 := widget.NewCollectionView(dg.NewObservableCollectionFrom([]interface{}{1}))
	cv2 := widget.NewCollectionView(dg.NewObservableCollectionFrom([]interface{}{2}))
	vic := widget.NewVirtualizingItemsControl()
	vic.BindCollectionView(cv1)
	vic.BindCollectionView(cv1) // повторно — та же CV
	vic.BindCollectionView(cv2)
	if cv1.ViewHandlerCount() != 0 {
		t.Errorf("после перепривязки у cv1 осталось %d слушателей", cv1.ViewHandlerCount())
	}
	if cv2.ViewHandlerCount() != 1 {
		t.Errorf("у cv2 слушателей %d, want 1", cv2.ViewHandlerCount())
	}
	vic.UnbindCollectionView()
	if cv2.ViewHandlerCount() != 0 {
		t.Errorf("после Unbind у cv2 осталось %d слушателей", cv2.ViewHandlerCount())
	}
}
