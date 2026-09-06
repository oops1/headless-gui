package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// {Loc} в заголовках колонок таблицы — запрос GG-16.
//
// Колонка — не виджет: сборщик складывает её в таблицу, и свойства, которое
// можно переустановить при смене языка, у неё снаружи нет. Поэтому {Loc …} в
// Header разворачивался один раз при загрузке и оставался на языке загрузки
// навсегда, а приложение переустанавливало заголовки из кода.

const locGridXAML = `<Window Width="400" Height="300">
  <Canvas>
    <DataGrid x:Name="grid" Left="0" Top="0" Width="400" Height="200">
      <DataGrid.Columns>
        <DataGridTextColumn Header="{Loc col.name}" Binding="{Binding Name}" Width="200"/>
        <DataGridCheckBoxColumn Header="{Loc col.active}" Binding="{Binding Active}" Width="100"/>
        <DataGridTextColumn Header="Без перевода" Binding="{Binding Other}" Width="100"/>
      </DataGrid.Columns>
    </DataGrid>
  </Canvas>
</Window>`

func headersOf(t *testing.T, w *widget.DataGridWidget) []string {
	t.Helper()
	var out []string
	for _, c := range w.Grid.Columns() {
		out = append(out, c.Header())
	}
	return out
}

func TestLocColumns_FollowLanguageSwitch(t *testing.T) {
	widget.ClearLocalizedItems()
	t.Cleanup(widget.ClearLocalizedItems)

	widget.RegisterStrings("ru", map[string]string{
		"col.name":   "Имя",
		"col.active": "Активен",
	})
	widget.RegisterStrings("en", map[string]string{
		"col.name":   "Name",
		"col.active": "Active",
	})

	widget.SetLanguage("ru")
	_, reg, err := widget.LoadUIFromXAML([]byte(locGridXAML))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	grid, ok := reg["grid"].(*widget.DataGridWidget)
	if !ok {
		t.Fatalf("DataGrid собрался как %T", reg["grid"])
	}

	if got := headersOf(t, grid); got[0] != "Имя" || got[1] != "Активен" {
		t.Fatalf("заголовки при загрузке: %v", got)
	}

	widget.SetLanguage("en")
	got := headersOf(t, grid)
	if got[0] != "Name" || got[1] != "Active" {
		t.Errorf("после смены языка заголовки %v, ждали Name/Active", got)
	}
	// Обычный заголовок не трогается сменой языка.
	if got[2] != "Без перевода" {
		t.Errorf("нелокализуемый заголовок стал %q", got[2])
	}

	widget.SetLanguage("ru")
	if got := headersOf(t, grid); got[0] != "Имя" {
		t.Errorf("возврат к русскому дал %v", got)
	}
}

// Ключ без перевода показывается как есть — так же, как везде в {Loc}.
func TestLocColumns_UnknownKeyShowsItself(t *testing.T) {
	widget.ClearLocalizedItems()
	t.Cleanup(widget.ClearLocalizedItems)
	widget.SetLanguage("ru")

	xaml := `<Window Width="300" Height="200"><Canvas>
	  <DataGrid x:Name="g" Left="0" Top="0" Width="300" Height="150">
	    <DataGrid.Columns>
	      <DataGridTextColumn Header="{Loc нет.такого.ключа}" Width="100"/>
	    </DataGrid.Columns>
	  </DataGrid>
	</Canvas></Window>`

	_, reg, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	g := reg["g"].(*widget.DataGridWidget)
	if got := headersOf(t, g)[0]; got != "нет.такого.ключа" {
		t.Errorf("незнакомый ключ показан как %q", got)
	}
}
