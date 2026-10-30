package base

// nativeedge.go — биты краёв окна между рамкой (widget/chrome) и бэкендом
// окна ОС (platform/*): рамка передаёт их в OnNativeResize, бэкенд по ним
// просит систему тянуть край.

// Биты краёв окна для OnNativeResize. Хост получает их вместо внутреннего
// ResizeEdge: значения устойчивы и не зависят от порядка констант ResizeEdge.
const (
	NativeEdgeTop    = 1
	NativeEdgeBottom = 2
	NativeEdgeLeft   = 4
	NativeEdgeRight  = 8
)
