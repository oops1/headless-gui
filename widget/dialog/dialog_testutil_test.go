package dialog

// Помощники тестов пакета (копии помощников тестов ядра).

import "github.com/oops1/headless-gui/v3/widget/base"

// accNodeNamed ищет узел дерева по имени (обход в глубину).
func accNodeNamed(n *base.AccessNode, name string) *base.AccessNode {
	if n == nil {
		return nil
	}
	if n.Name == name {
		return n
	}
	for _, c := range n.Children {
		if f := accNodeNamed(c, name); f != nil {
			return f
		}
	}
	return nil
}

// accessHasState — есть ли состояние в списке.
func accessHasState(states []string, want string) bool {
	for _, s := range states {
		if s == want {
			return true
		}
	}
	return false
}
