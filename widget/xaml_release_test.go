package widget

import (
	"image"
	"testing"
)

// GG-75: загруженное дерево можно отписать от смены языка.
//
// LoadUIFromXAML выбрасывал BindingScope, подписанный на смену языка, а
// свёрнутые строки (меню, вкладки, колонки) копились в общем списке. После N
// загрузок одна смена языка переводила и перекладывала N мёртвых деревьев.

func locListenerCount() int {
	langMu.RLock()
	defer langMu.RUnlock()
	return len(languageListeners)
}

func locItemCount() int {
	locItemsMu.Lock()
	defer locItemsMu.Unlock()
	return len(locItems)
}

func xamlScopeCount() int {
	xamlScopesMu.Lock()
	defer xamlScopesMu.Unlock()
	return len(xamlScopes)
}

func setupLocRelease(t *testing.T) {
	t.Helper()
	ClearStrings()
	ClearLocalizedItems()
	t.Cleanup(func() {
		ClearStrings()
		ClearLocalizedItems()
		SetLanguage("EN")
	})
	RegisterStrings("RU", map[string]string{"File": "Файл", "Open": "Открыть", "Login": "Логин"})
	SetFallbackLanguage("EN")
	SetLanguage("EN")
}

// ReleaseXAML снимает и слушатель языка, и свёрнутые строки дерева.
func TestReleaseXAML_UnsubscribesTree(t *testing.T) {
	setupLocRelease(t)
	listeners, items, scopes := locListenerCount(), locItemCount(), xamlScopeCount()

	for i := 0; i < 5; i++ {
		root, _, err := LoadUIFromXAML([]byte(locItemsXAML))
		if err != nil {
			t.Fatalf("загрузка: %v", err)
		}
		if locListenerCount() <= listeners || locItemCount() <= items {
			t.Fatal("загруженное дерево не подписано — проверять нечего")
		}
		ReleaseXAML(root)
		ReleaseXAML(root) // повторно — безопасно
	}

	if got := locListenerCount(); got != listeners {
		t.Errorf("слушателей языка %d, до загрузок было %d", got, listeners)
	}
	if got := locItemCount(); got != items {
		t.Errorf("свёрнутых строк %d, до загрузок было %d", got, items)
	}
	if got := xamlScopeCount(); got != scopes {
		t.Errorf("записей для ReleaseXAML %d, было %d", got, scopes)
	}
}

// Dispose одного дерева снимает только его строки: второе дерево продолжает
// переводиться, освобождённое — нет.
func TestBindingScopeDispose_OnlyOwnItems(t *testing.T) {
	setupLocRelease(t)

	_, reg1, scope1, err := LoadUIFromXAMLBindings([]byte(locItemsXAML), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, reg2, scope2, err := LoadUIFromXAMLBindings([]byte(locItemsXAML), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer scope2.Dispose()
	menu1 := reg1["menu"].(*MenuBar)
	menu2 := reg2["menu"].(*MenuBar)

	scope1.Dispose()
	SetLanguage("RU")

	if got := menu2.Items()[0].Text; got != "Файл" {
		t.Errorf("живое дерево не перевелось: %q", got)
	}
	if got := menu2.Items()[0].Items[0].Text; got != "Открыть" {
		t.Errorf("подпункт живого дерева не перевёлся: %q", got)
	}
	if got := menu1.Items()[0].Text; got != "File" {
		t.Errorf("освобождённое дерево всё ещё переводится: %q", got)
	}
}

// Дерево, которое ничего не держит, для ReleaseXAML не запоминается: запись
// сама была бы утечкой.
func TestReleaseXAML_NothingToReleaseNotTracked(t *testing.T) {
	setupLocRelease(t)
	scopes := xamlScopeCount()

	root, _, err := LoadUIFromXAML([]byte(`<Window Title="T" Width="200" Height="100">
  <Label Left="0" Top="0" Width="100" Height="20" Text="просто текст"/>
</Window>`))
	if err != nil {
		t.Fatal(err)
	}
	if got := xamlScopeCount(); got != scopes {
		t.Fatalf("дерево без подписок записано для ReleaseXAML: %d записей, было %d", got, scopes)
	}
	ReleaseXAML(root) // no-op
}

// GG-74: при смене языка дерево из разметки переразмечается само, и флажок без
// Width получает ширину под новую подпись.
func TestLanguageChange_RelayoutsAutoWidth(t *testing.T) {
	setupLocRelease(t)
	RegisterStrings("RU", map[string]string{"Regexp": "регулярное выражение"})

	root, reg, err := LoadUIFromXAML([]byte(`<Window Title="T" Width="600" Height="100">
  <DockPanel Name="dp" Left="0" Top="0" Width="600" Height="30">
    <CheckBox Name="cb" DockPanel.Dock="Right" Content="{Loc Regexp}"/>
    <Label Text="Период:"/>
  </DockPanel>
</Window>`))
	if err != nil {
		t.Fatal(err)
	}
	defer ReleaseXAML(root)
	root.SetBounds(image.Rect(0, 0, 600, 100))
	cb := reg["cb"].(*CheckBox)
	before := cb.Bounds().Dx()

	SetLanguage("RU")

	want := max(80, checkBoxContentWidth(cb))
	if got := cb.Bounds().Dx(); got != want || got <= before {
		t.Fatalf("ширина флажка после смены языка %d (до — %d), ждал %d", got, before, want)
	}
}
