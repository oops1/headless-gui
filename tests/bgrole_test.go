package tests

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Роль фона у StackPanel и DockPanel — пункт 4 замечаний difftool.
//
// Контейнеры раскладки тему не принимали: панель инструментов с непрозрачным
// фоном оставалась белой в тёмной теме. Но и «непрозрачный — значит из темы»
// не годится: свой фон панели (Background="#2D2D30") смена темы стёрла бы, а
// внутри диалога — сразу при показе. Поэтому теме следует только панель с
// назначенной ролью.

var custom = color.RGBA{R: 0x2D, G: 0x2D, B: 0x30, A: 255}

// Свой фон тема не трогает — прежнее поведение.
func TestBackgroundRole_CustomSurvivesTheme(t *testing.T) {
	sp := widget.NewStackPanel(widget.OrientationHorizontal)
	sp.Background = custom
	sp.ApplyTheme(widget.ThemeByName("Win11 Light"))
	if sp.Background != custom {
		t.Errorf("свой фон стёрт темой: %v", sp.Background)
	}

	dp := widget.NewDockPanel()
	dp.Background = custom
	dp.ApplyTheme(widget.ThemeByName("Win11 Light"))
	if dp.Background != custom {
		t.Errorf("свой фон DockPanel стёрт темой: %v", dp.Background)
	}
}

// Панель с ролью следует теме в обе стороны.
func TestBackgroundRole_FollowsTheme(t *testing.T) {
	light, dark := widget.ThemeByName("Win11 Light"), widget.ThemeByName("Win11 Dark")

	sp := widget.NewStackPanel(widget.OrientationHorizontal)
	sp.SetBackgroundRole(widget.BackgroundPanel)
	sp.ApplyTheme(dark)
	if sp.Background != dark.PanelBG {
		t.Errorf("в тёмной теме фон %v, ожидался PanelBG %v", sp.Background, dark.PanelBG)
	}
	sp.ApplyTheme(light)
	if sp.Background != light.PanelBG {
		t.Errorf("в светлой теме фон %v, ожидался PanelBG %v", sp.Background, light.PanelBG)
	}

	dp := widget.NewDockPanel()
	dp.SetBackgroundRole(widget.BackgroundWindow)
	dp.ApplyTheme(dark)
	if dp.Background != dark.WindowBG {
		t.Errorf("DockPanel: фон %v, ожидался WindowBG %v", dp.Background, dark.WindowBG)
	}
}

// Роль, назначенная после применения темы, действует сразу, а не ждёт смены.
func TestBackgroundRole_AppliesImmediately(t *testing.T) {
	sp := widget.NewStackPanel(widget.OrientationHorizontal)
	sp.SetBackgroundRole(widget.BackgroundPanel)
	if want := widget.CurrentTheme().PanelBG; sp.Background != want {
		t.Errorf("фон %v, в текущей теме PanelBG %v", sp.Background, want)
	}
}

// В разметке роль задаётся как ссылка на поле темы.
func TestBackgroundRole_FromXAML(t *testing.T) {
	xaml := `<Window Width="400" Height="200"><Canvas>
	  <StackPanel x:Name="bar" Background="{Theme PanelBG}"/>
	  <DockPanel x:Name="body" Background="{Theme WindowBG}"/>
	  <StackPanel x:Name="own" Background="#2D2D30"/>
	</Canvas></Window>`
	_, reg, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	if r := reg["bar"].(*widget.StackPanel).BackgroundRole; r != widget.BackgroundPanel {
		t.Errorf("{Theme PanelBG} дал роль %v", r)
	}
	if r := reg["body"].(*widget.DockPanel).BackgroundRole; r != widget.BackgroundWindow {
		t.Errorf("{Theme WindowBG} дал роль %v", r)
	}
	own := reg["own"].(*widget.StackPanel)
	if own.BackgroundRole != widget.BackgroundCustom || own.Background != custom {
		t.Errorf("свой цвет в разметке: роль %v, фон %v", own.BackgroundRole, own.Background)
	}
}
