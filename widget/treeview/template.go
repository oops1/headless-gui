package treeview

import (
	"image"
	"reflect"

	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

// ─── HierarchicalDataTemplate ──────────────────────────────────────────────

// HierarchicalDataTemplate определяет шаблон отображения узла
// с поддержкой иерархии дочерних элементов (WPF HierarchicalDataTemplate).
//
// XAML-пример:
//
//	<TreeView.ItemTemplate>
//	    <HierarchicalDataTemplate ItemsSource="{Binding Children}">
//	        <StackPanel Orientation="Horizontal">
//	            <Image Source="{Binding Icon}" Width="16" Height="16"/>
//	            <TextBlock Text="{Binding Name}" Margin="5,0,0,0"/>
//	        </StackPanel>
//	    </HierarchicalDataTemplate>
//	</TreeView.ItemTemplate>
type HierarchicalDataTemplate struct {
	// ItemsSourcePath — путь Binding к коллекции дочерних элементов.
	// Например: "Children", "SubItems", "Nodes"
	ItemsSourcePath string

	// HeaderPath — путь Binding к текстовому заголовку узла.
	// Например: "Name", "Title", "Text"
	HeaderPath string

	// IconPath — путь Binding к иконке узла (image.Image).
	// Например: "Icon", "Image"
	IconPath string

	// IsExpandedPath — путь Binding к состоянию раскрытия.
	// Например: "IsExpanded"
	IsExpandedPath string

	// CustomRenderer — опциональный кастомный рендерер узла.
	// Если задан, используется вместо стандартной отрисовки (иконка + текст).
	CustomRenderer NodeRenderer
}

// NodeRendererContext — контекст отрисовки узла для кастомного рендерера.
type NodeRendererContext struct {
	// Rect — прямоугольник для рисования содержимого (после стрелки и отступа).
	Rect image.Rectangle
	// Item — объект данных (DataContext узла).
	Item interface{}
	// IsSelected — узел выбран.
	IsSelected bool
	// IsHovered — курсор над узлом.
	IsHovered bool
	// Expanded — узел раскрыт.
	Expanded bool
	// DrawCtx — контекст рисования.
	DrawCtx DrawContextBridge
}

// NodeRenderer — пользовательская функция отрисовки содержимого узла.
type NodeRenderer func(ctx NodeRendererContext)

// ─── Resolve helpers ───────────────────────────────────────────────────────

// resolveHeader возвращает текст заголовка для объекта данных по шаблону.
func (t *HierarchicalDataTemplate) resolveHeader(dataContext interface{}) string {
	if t == nil || t.HeaderPath == "" || dataContext == nil {
		return ""
	}
	b := &datagrid.Binding{Path: t.HeaderPath}
	return datagrid.ResolveBinding(b, dataContext)
}

// resolveIcon возвращает иконку для объекта данных по шаблону.
func (t *HierarchicalDataTemplate) resolveIcon(dataContext interface{}) image.Image {
	if t == nil || t.IconPath == "" || dataContext == nil {
		return nil
	}
	val, ok := datagrid.GetPropertyValue(dataContext, t.IconPath)
	if !ok {
		return nil
	}
	if img, ok := val.(image.Image); ok {
		return img
	}
	return nil
}

// resolveIsExpanded возвращает состояние раскрытия из объекта данных.
func (t *HierarchicalDataTemplate) resolveIsExpanded(dataContext interface{}) bool {
	if t == nil || t.IsExpandedPath == "" || dataContext == nil {
		return false
	}
	val, ok := datagrid.GetPropertyValue(dataContext, t.IsExpandedPath)
	if !ok {
		return false
	}
	if b, ok := val.(bool); ok {
		return b
	}
	return false
}

// resolveChildren возвращает дочерние объекты как []interface{} из DataContext.
// Поддерживает:
//   - *ObservableCollection
//   - []interface{}
//   - []T (через reflect)
func (t *HierarchicalDataTemplate) resolveChildren(dataContext interface{}) []interface{} {
	if t == nil || t.ItemsSourcePath == "" || dataContext == nil {
		return nil
	}
	val, ok := datagrid.GetPropertyValue(dataContext, t.ItemsSourcePath)
	if !ok {
		return nil
	}

	// *ObservableCollection
	if oc, ok := val.(*datagrid.ObservableCollection); ok {
		return oc.Items()
	}

	// []interface{}
	if items, ok := val.([]interface{}); ok {
		return items
	}

	// []T → []interface{} через reflect
	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Slice {
		result := make([]interface{}, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			result[i] = rv.Index(i).Interface()
		}
		return result
	}

	return nil
}
