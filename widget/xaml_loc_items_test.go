package widget

import "testing"

// locItemsXAML — разметка со всеми видами «свёрнутых» строк: заголовки
// вкладок, пункты меню с подменю, элементы выпадающего списка и подсказка
// пустого поля. Ни одна из них не является свойством отдельного виджета.
const locItemsXAML = `<Window Title="T" Width="400" Height="300">
  <Menu Name="menu" Left="0" Top="0" Width="400" Height="28">
    <MenuItem Header="{Loc File}">
      <MenuItem Header="{Loc Open}"/>
      <MenuItem Separator="True"/>
      <MenuItem Header="{Loc Exit}"/>
    </MenuItem>
    <MenuItem Header="{Loc Help}"/>
  </Menu>
  <TabControl Name="tabs" Left="0" Top="30" Width="400" Height="200">
    <TabItem Header="{Loc Input}">
      <Label Left="0" Top="0" Width="100" Height="20" Text="{Loc Login}"/>
    </TabItem>
    <TabItem Header="{Loc Output}"/>
  </TabControl>
  <ComboBox Name="combo" Left="0" Top="240" Width="200" Height="30">
    <ComboBoxItem Content="{Loc Administrator}"/>
    <ComboBoxItem Content="{Loc Guest}"/>
  </ComboBox>
  <TextBox Name="edit" Left="0" Top="270" Width="200" Height="28" Placeholder="{Loc Enter a name}"/>
</Window>`

// TestLocalizedFoldedItems — при смене языка обновляются не только свойства
// виджетов, но и строки, свёрнутые в родителя: вкладки, меню, элементы списка
// и подсказка поля. Раньше они оставались на языке загрузки — из-за чего
// приложение выглядело переведённым лишь наполовину.
func TestLocalizedFoldedItems(t *testing.T) {
	ClearStrings()
	ClearLocalizedItems()
	t.Cleanup(func() {
		ClearStrings()
		ClearLocalizedItems()
		SetLanguage("EN")
	})

	RegisterStrings("RU", map[string]string{
		"File":          "Файл",
		"Open":          "Открыть",
		"Exit":          "Выход",
		"Help":          "Справка",
		"Input":         "Ввод",
		"Output":        "Вывод",
		"Login":         "Логин",
		"Administrator": "Администратор",
		"Guest":         "Гость",
		"Enter a name":  "Введите имя",
	})
	SetFallbackLanguage("EN")
	SetLanguage("EN")

	_, reg, scope, err := LoadUIFromXAMLBindings([]byte(locItemsXAML), nil)
	if err != nil {
		t.Fatalf("загрузка разметки: %v", err)
	}
	_ = scope

	tabs, _ := reg["tabs"].(*TabControl)
	menu, _ := reg["menu"].(*MenuBar)
	combo, _ := reg["combo"].(*Dropdown)
	edit, _ := reg["edit"].(*TextInput) // <TextBox> без AcceptsReturn — однострочное поле
	if tabs == nil || menu == nil || combo == nil || edit == nil {
		t.Fatalf("виджеты не найдены: tabs=%v menu=%v combo=%v edit=%v", tabs, menu, combo, edit)
	}

	// ── Английский (ключ = сам текст) ────────────────────────────────────────
	if got := tabs.TabHeader(0); got != "Input" {
		t.Errorf("вкладка 0 = %q, want Input", got)
	}
	if got := menu.Items()[0].Text; got != "File" {
		t.Errorf("меню 0 = %q, want File", got)
	}
	if got := menu.Items()[0].Items[0].Text; got != "Open" {
		t.Errorf("подпункт 0.0 = %q, want Open", got)
	}
	if got := combo.Items()[0]; got != "Administrator" {
		t.Errorf("элемент списка 0 = %q, want Administrator", got)
	}
	if edit.Placeholder != "Enter a name" {
		t.Errorf("подсказка = %q", edit.Placeholder)
	}

	// ── Переключаем язык ─────────────────────────────────────────────────────
	SetLanguage("RU")

	if got := tabs.TabHeader(0); got != "Ввод" {
		t.Errorf("вкладка 0 после смены языка = %q, want Ввод", got)
	}
	if got := tabs.TabHeader(1); got != "Вывод" {
		t.Errorf("вкладка 1 после смены языка = %q, want Вывод", got)
	}
	if got := menu.Items()[0].Text; got != "Файл" {
		t.Errorf("меню 0 после смены языка = %q, want Файл", got)
	}
	if got := menu.Items()[1].Text; got != "Справка" {
		t.Errorf("меню 1 после смены языка = %q, want Справка", got)
	}
	if got := menu.Items()[0].Items[0].Text; got != "Открыть" {
		t.Errorf("подпункт 0.0 после смены языка = %q, want Открыть", got)
	}
	if got := menu.Items()[0].Items[2].Text; got != "Выход" {
		t.Errorf("подпункт 0.2 после смены языка = %q, want Выход", got)
	}
	if !menu.Items()[0].Items[1].Separator {
		t.Error("разделитель в подменю потерялся")
	}
	if got := combo.Items()[0]; got != "Администратор" {
		t.Errorf("элемент списка 0 после смены языка = %q", got)
	}
	if got := combo.Items()[1]; got != "Гость" {
		t.Errorf("элемент списка 1 после смены языка = %q", got)
	}
	if edit.Placeholder != "Введите имя" {
		t.Errorf("подсказка после смены языка = %q", edit.Placeholder)
	}

	// Обычное свойство виджета продолжает работать как раньше.
	if lbl := findLabelWithText(reg, "Логин"); lbl == nil {
		t.Error("подпись внутри вкладки не перевелась")
	}
}

// findLabelWithText ищет среди зарегистрированных виджетов подпись с текстом.
func findLabelWithText(reg map[string]Widget, want string) *Label {
	for _, w := range reg {
		if l, ok := w.(*Label); ok && l.Text() == want {
			return l
		}
	}
	return nil
}
