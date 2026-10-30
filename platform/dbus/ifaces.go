package dbus

// Стандартные интерфейсы D-Bus: их реализует любой объект, экспортированный
// на шину (свойства и самоописание).
const (
	IfaceProperties     = "org.freedesktop.DBus.Properties"
	IfaceIntrospectable = "org.freedesktop.DBus.Introspectable"
)
