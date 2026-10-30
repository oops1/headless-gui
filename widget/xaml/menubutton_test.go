package xaml_test

import (
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
	"github.com/oops1/headless-gui/v4/widget/xaml"
)

// Разметка: <SplitButton> и <MenuButton> с пунктами.
func TestMenuButton_XAML(t *testing.T) {
	_, reg, err := xaml.LoadUIFromXAML([]byte(`<Window Title="T" Width="600" Height="100">
  <ToolBar Name="tb" Left="0" Top="0" Width="600" Height="40">
    <SplitButton Name="pull" Content="Pull">
      <MenuItem Header="Fetch"/>
      <MenuItem Header="Pull (rebase)"/>
    </SplitButton>
    <MenuButton Name="flow" Content="Git-Flow" IsMenuEnabled="False">
      <MenuItem Header="Start Feature…"/>
    </MenuButton>
  </ToolBar>
</Window>`))
	if err != nil {
		t.Fatal(err)
	}
	pull, ok := reg["pull"].(*base.MenuButton)
	if !ok {
		t.Fatalf("pull — %T, ждал *MenuButton", reg["pull"])
	}
	flow, ok := reg["flow"].(*base.MenuButton)
	if !ok {
		t.Fatalf("flow — %T, ждал *MenuButton", reg["flow"])
	}
	if !pull.Split || len(pull.Items) != 2 || pull.Items[1].Text != "Pull (rebase)" {
		t.Fatalf("pull: Split=%v Items=%+v", pull.Split, pull.Items)
	}
	if flow.Split || flow.MenuEnabled() || len(flow.Items) != 1 {
		t.Fatalf("flow: Split=%v MenuEnabled=%v Items=%+v", flow.Split, flow.MenuEnabled(), flow.Items)
	}
	if pull.CornerRadius != 4 {
		t.Fatalf("скругление кнопки панели %d, ждал 4", pull.CornerRadius)
	}
}
