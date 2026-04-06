// Package widget — TreeView: иерархический список с раскрытием/свёртыванием узлов.
//
// Аналог WPF TreeView / TreeViewItem. Отображает дерево элементов
// с отступами по уровню вложенности и стрелками ▸/▾ для раскрытия.
//
// XAML:
//
//	<TreeView Background="#252526" Foreground="#CCCCCC" Name="tree">
//	    <TreeViewItem Header="Root" IsExpanded="True">
//	        <TreeViewItem Header="Child 1"/>
//	        <TreeViewItem Header="Child 2">
//	            <TreeViewItem Header="Grandchild"/>
//	        </TreeViewItem>
//	    </TreeViewItem>
//	</TreeView>
package widget

import (
	"image"
	"image/color"
	"sync/atomic"
)

// ─── TreeNode ───────────────────────────────────────────────────────────────

// TreeNode — узел дерева.
type TreeNode struct {
	Text     string
	Children []*TreeNode
	Expanded bool
	Icon     image.Image // опциональная иконка (отображается перед текстом)
}

// NewTreeNode создаёт узел с заданным текстом.
func NewTreeNode(text string) *TreeNode {
	return &TreeNode{Text: text}
}

// AddChild добавляет дочерний узел.
func (n *TreeNode) AddChild(child *TreeNode) {
	n.Children = append(n.Children, child)
}

// HasChildren возвращает true, если у узла есть потомки.
func (n *TreeNode) HasChildren() bool {
	return len(n.Children) > 0
}

// ─── flatNode ───────────────────────────────────────────────────────────────

// flatNode — видимый узел с глубиной вложенности (для плоского рендеринга).
type flatNode struct {
	node  *TreeNode
	depth int
}

// ─── TreeView ───────────────────────────────────────────────────────────────

// TreeView — виджет для отображения иерархического списка.
type TreeView struct {
	Base

	// Roots — корневые узлы дерева.
	Roots []*TreeNode

	// Background — цвет фона.
	Background color.RGBA
	// Foreground — цвет текста.
	Foreground color.RGBA
	// SelectColor — цвет фона выделенного элемента.
	SelectColor color.RGBA
	// HoverColor — цвет фона при наведении.
	HoverColor color.RGBA
	// ArrowColor — цвет стрелок ▸/▾.
	ArrowColor color.RGBA

	// ItemHeight — высота одной строки (px). 0 → 22.
	ItemHeight int
	// IndentSize — отступ на один уровень вложенности (px). 0 → 18.
	IndentSize int

	// OnSelect вызывается при выборе узла кликом.
	OnSelect func(node *TreeNode)

	// внутреннее состояние
	selectedIdx atomic.Int32 // индекс в flat-списке (-1 = нет)
	hoverIdx    atomic.Int32 // индекс при hover (-1 = нет)
	scrollOff   int          // смещение прокрутки (в строках)
}

// NewTreeView создаёт пустой TreeView с цветами из текущей темы.
func NewTreeView() *TreeView {
	tv := &TreeView{
		Background:  win10.WindowBG,
		Foreground:  win10.LabelText,
		SelectColor: win10.ListItemSelect,
		HoverColor:  win10.ListItemHover,
		ArrowColor:  win10.Disabled,
		ItemHeight:  22,
		IndentSize:  18,
	}
	tv.selectedIdx.Store(-1)
	tv.hoverIdx.Store(-1)
	return tv
}

// AddRoot добавляет корневой узел.
func (tv *TreeView) AddRoot(n *TreeNode) {
	tv.Roots = append(tv.Roots, n)
}

// SelectedNode возвращает текущий выделенный узел или nil.
func (tv *TreeView) SelectedNode() *TreeNode {
	idx := int(tv.selectedIdx.Load())
	flat := tv.visibleNodes()
	if idx >= 0 && idx < len(flat) {
		return flat[idx].node
	}
	return nil
}

// ─── Плоский список видимых узлов ───────────────────────────────────────────

// visibleNodes возвращает плоский список видимых (раскрытых) узлов с глубиной.
func (tv *TreeView) visibleNodes() []flatNode {
	var result []flatNode
	for _, root := range tv.Roots {
		collectVisible(root, 0, &result)
	}
	return result
}

func collectVisible(n *TreeNode, depth int, out *[]flatNode) {
	*out = append(*out, flatNode{node: n, depth: depth})
	if n.Expanded {
		for _, child := range n.Children {
			collectVisible(child, depth+1, out)
		}
	}
}

// ─── Draw ───────────────────────────────────────────────────────────────────

func (tv *TreeView) Draw(ctx DrawContext) {
	b := tv.Bounds()
	if b.Empty() {
		return
	}
	ih := tv.itemH()
	indent := tv.indentW()

	// Фон
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), tv.Background)

	// Clip к bounds TreeView
	ctx.SetClip(b)
	defer ctx.ClearClip()

	flat := tv.visibleNodes()
	selIdx := int(tv.selectedIdx.Load())
	hovIdx := int(tv.hoverIdx.Load())

	maxVisible := b.Dy() / ih
	startIdx := tv.scrollOff
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := startIdx + maxVisible + 1
	if endIdx > len(flat) {
		endIdx = len(flat)
	}

	for i := startIdx; i < endIdx; i++ {
		fn := flat[i]
		y := b.Min.Y + (i-startIdx)*ih

		// Фон: выделение или hover
		if i == selIdx {
			ctx.FillRect(b.Min.X, y, b.Dx(), ih, tv.SelectColor)
		} else if i == hovIdx {
			ctx.FillRect(b.Min.X, y, b.Dx(), ih, tv.HoverColor)
		}

		x := b.Min.X + 6 + fn.depth*indent

		// Стрелка ▸/▾
		if fn.node.HasChildren() {
			arrowY := y + ih/2
			if fn.node.Expanded {
				// ▾ — вниз
				tv.drawArrowDown(ctx, x, arrowY)
			} else {
				// ▸ — вправо
				tv.drawArrowRight(ctx, x, arrowY)
			}
		}

		// Иконка (если есть)
		textX := x + 14
		if fn.node.Icon != nil {
			iconSz := 16
			iconY := y + (ih-iconSz)/2
			ctx.DrawImageScaled(fn.node.Icon, textX, iconY, iconSz, iconSz)
			textX += iconSz + 4
		}

		// Текст
		textY := y + (ih-13)/2
		ctx.DrawText(fn.node.Text, textX, textY, tv.Foreground)
	}

	tv.drawDisabledOverlay(ctx)
}

// drawArrowRight рисует треугольник ▸ (свёрнуто).
func (tv *TreeView) drawArrowRight(ctx DrawContext, x, cy int) {
	col := tv.ArrowColor
	for dy := -3; dy <= 3; dy++ {
		w := 4 - abs(dy)
		for dx := 0; dx < w; dx++ {
			ctx.SetPixel(x+dx, cy+dy, col)
		}
	}
}

// drawArrowDown рисует треугольник ▾ (раскрыто).
func (tv *TreeView) drawArrowDown(ctx DrawContext, x, cy int) {
	col := tv.ArrowColor
	for dx := -3; dx <= 3; dx++ {
		h := 4 - abs(dx)
		for dy := 0; dy < h; dy++ {
			ctx.SetPixel(x+3+dx, cy-2+dy, col)
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ─── Mouse events ───────────────────────────────────────────────────────────

// OnMouseMove обновляет hover-индекс.
func (tv *TreeView) OnMouseMove(x, y int) {
	if !tv.IsEnabled() {
		return
	}
	b := tv.Bounds()
	if !image.Pt(x, y).In(b) {
		tv.hoverIdx.Store(-1)
		return
	}

	ih := tv.itemH()
	row := (y - b.Min.Y) / ih
	idx := tv.scrollOff + row

	flat := tv.visibleNodes()
	if idx >= 0 && idx < len(flat) {
		tv.hoverIdx.Store(int32(idx))
	} else {
		tv.hoverIdx.Store(-1)
	}
}

// OnMouseButton обрабатывает клик: раскрытие/свёртывание или выделение.
func (tv *TreeView) OnMouseButton(e MouseEvent) bool {
	if !tv.IsEnabled() {
		return false
	}
	if e.Button != MouseLeft || !e.Pressed {
		return false
	}
	b := tv.Bounds()
	if !image.Pt(e.X, e.Y).In(b) {
		return false
	}

	ih := tv.itemH()
	row := (e.Y - b.Min.Y) / ih
	idx := tv.scrollOff + row

	flat := tv.visibleNodes()
	if idx < 0 || idx >= len(flat) {
		return true
	}

	fn := flat[idx]
	tv.selectedIdx.Store(int32(idx))

	// Если у узла есть дети — переключаем раскрытие
	if fn.node.HasChildren() {
		fn.node.Expanded = !fn.node.Expanded
	}

	if tv.OnSelect != nil {
		tv.OnSelect(fn.node)
	}
	return true
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func (tv *TreeView) itemH() int {
	if tv.ItemHeight > 0 {
		return tv.ItemHeight
	}
	return 22
}

func (tv *TreeView) indentW() int {
	if tv.IndentSize > 0 {
		return tv.IndentSize
	}
	return 18
}

// ApplyTheme обновляет цвета TreeView.
func (tv *TreeView) ApplyTheme(t *Theme) {
	tv.Background = t.WindowBG
	tv.Foreground = t.TreeText
	tv.ArrowColor = t.TreeArrow
	tv.SelectColor = t.ListItemSelect
	tv.HoverColor = t.ListItemHover
}
