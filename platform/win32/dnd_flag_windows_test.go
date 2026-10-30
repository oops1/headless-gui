//go:build windows

package win32

import "github.com/oops1/headless-gui/v3/platform/native"

// Перетаскивание включает возможность platform/dnd (флаг в реестре), а
// тестовый бинарь бэкенда её не подключает — включаем флаг сами, как фасад
// window. dndOff снимает его (для проверок «без перетаскивания»).
var dndOff = native.RegisterDragDrop()
