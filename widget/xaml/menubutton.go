package xaml

// menubutton.go — теги <MenuButton> и <SplitButton>: кнопка, меню и
// доступность частей.

import (
	"strings"

	"github.com/oops1/headless-gui/v4/widget/base"
)

// buildXAMLMenuButton строит кнопку с меню из <MenuButton> или <SplitButton>:
//
//	<SplitButton Name="pull" Content="Pull" Icon="pull.png" IconPosition="Top">
//	  <MenuItem Header="{Loc Fetch}"/>
//	  <MenuItem Header="Pull (rebase)"/>
//	</SplitButton>
//
// Атрибуты — как у Button; Split="True" делает разделённой и <MenuButton>,
// IsActionEnabled/IsMenuEnabled="False" выключают части.
func buildXAMLMenuButton(el Element, tag, baseDir string) base.Widget {
	btn, _ := buildXAMLButton(el, baseDir).(*base.Button)
	if btn == nil {
		return nil
	}
	mb := &base.MenuButton{Button: btn}
	mb.Split = tag == "splitbutton" || strings.EqualFold(el.Attr("Split"), "true")
	// Новая кнопка включена целиком; сеттеры зовутся только для выключения
	// (они инвалидируют кнопку, а без владельца это уходит всем движкам).
	if strings.EqualFold(el.Attr("IsActionEnabled"), "false") {
		mb.SetActionEnabled(false)
	}
	if strings.EqualFold(el.Attr("IsMenuEnabled"), "false") {
		mb.SetMenuEnabled(false)
	}
	items, keys := parseMenuItems(el, baseDir)
	mb.Items = items
	registerLocItemList(keys, func(i int, s string) {
		if i < len(mb.Items) {
			mb.Items[i].Text = s
		}
	})
	return mb
}
