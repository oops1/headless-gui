// itemstyle.go — оформление отдельного узла: цвет текста и начертание.
//
// Дерево рисовало текст любого узла одинаково: цветом темы и обычным
// начертанием. У узла есть иконка, тег и признак «включён», а способа выделить
// строку не было — активный репозиторий приходилось помечать подкраской
// иконки, потому что больше нечем.
package treeview

import "image/color"

// itemStyle возвращает цвет и начертание текста узла.
//
// Порядок такой: сперва спрашивается колбэк ItemStyle — он знает о состоянии
// приложения то, чего не знает узел, — и только потом смотрятся поля самого
// узла. Ни то, ни другое не заданo — цвет темы и обычное начертание, как было.
func (tv *TreeView) itemStyle(item *TreeViewItem) (color.RGBA, bool) {
	fg, bold := tv.Theme.Foreground, false
	if item == nil {
		return fg, bold
	}
	if item.Foreground.A > 0 {
		fg = item.Foreground
	}
	if item.Bold {
		bold = true
	}
	if tv.ItemStyle != nil {
		if c, b, ok := tv.ItemStyle(item); ok {
			if c.A > 0 {
				fg = c
			}
			bold = b
		}
	}
	return fg, bold
}
