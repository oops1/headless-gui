package xaml

// stretch.go — тег <Stretch> (распорка с весом).

import "github.com/oops1/headless-gui/v4/widget/base"

// buildXAMLStretch строит распорку из <Stretch/> или <ToolBarStretch/>.
func buildXAMLStretch(el Element) base.Widget {
	s := base.NewStretch()
	if w := ParseInt(el.Attr("Weight")); w > 0 {
		s.Weight = w
	}
	return s
}
