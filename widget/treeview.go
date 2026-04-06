// Package widget — обратная совместимость для TreeView/TreeNode.
//
// Все типы делегируют в пакет widget/treeview.
// Старый API (NewTreeView, NewTreeNode, TreeNode)
// сохранён как type-alias / wrapper для плавной миграции.
package widget

import (
	"github.com/oops1/headless-gui/v3/widget/treeview"
)

// ─── TreeNode (обратная совместимость) ─────────────────────────────────────

// TreeNode — алиас на treeview.TreeViewItem для обратной совместимости.
// Поля Text, Expanded, Icon, Children доступны напрямую.
type TreeNode = treeview.TreeViewItem

// NewTreeNode создаёт узел с заданным текстом (обратная совместимость).
func NewTreeNode(text string) *TreeNode {
	return treeview.NewItem(text)
}

// ─── TreeView convenience ──────────────────────────────────────────────────

// NewTreeView создаёт TreeViewWidget (обратная совместимость).
// Возвращает *TreeViewWidget (который регистрируется в XAML как "treeview").
func NewTreeView() *TreeViewWidget {
	return NewTreeViewWidget()
}
