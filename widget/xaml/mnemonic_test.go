package xaml

import (
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
)

func TestMenuBar_UseMnemonicsFromXAML(t *testing.T) {
	const src = `<Canvas Width="400" Height="200">
  <Menu Name="main" Left="0" Top="0" Width="400" Height="28" UseMnemonics="True">
    <MenuItem Header="_Файл">
      <MenuItem Text="_Новый"/>
    </MenuItem>
  </Menu>
</Canvas>`
	root, reg, err := LoadUIFromXAML([]byte(src))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	defer ReleaseXAML(root)

	mb, ok := reg["main"].(*base.MenuBar)
	if !ok {
		t.Fatalf("main — %T", reg["main"])
	}
	if !mb.UseMnemonics {
		t.Error("атрибут UseMnemonics не прочитан")
	}
}
