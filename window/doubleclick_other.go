//go:build !windows

package window

import "time"

// nativeDoubleClickTime — интервал двойного щелчка, заданный системой.
//
// На X11 и Wayland общесистемного значения в самом протоколе нет: его держат
// настройки окружения (XSettings у GNOME, свой ключ у KDE), и читать их
// значило бы тянуть зависимость от конкретного рабочего стола. Возвращаем 0 —
// движок остаётся при своём значении по умолчанию.
func nativeDoubleClickTime() time.Duration { return 0 }
