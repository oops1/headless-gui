package xaml

// stretch.go — тег <Stretch> (распорка с весом).

import "github.com/oops1/headless-gui/v3/widget/base"

// buildXAMLStretch строит распорку из <Stretch/> или <ToolBarStretch/>.
func buildXAMLStretch(el xElement) base.Widget {
	s := base.NewStretch()
	if w := xatoi(el.attr("Weight")); w > 0 {
		s.Weight = w
	}
	return s
}
