// quickstrings.go — строки быстрых настроек Windows 11.
//
// Как и остальные строки пакета, это ключи widget.Tr с русским и английским
// переводом, которые читаются при отрисовке: смена языка меняет надписи на
// открытой панели, ничего не пересоздавая. Потребитель, у которого эти слова
// уже есть в своём каталоге, связывает ключи движка со своими
// (widget.AliasStrings).
package desktop

import "github.com/oops1/headless-gui/v3/widget"

// Ключи строк быстрых настроек.
const (
	// StrQuickEdit — подсказка карандаша внизу панели.
	StrQuickEdit = "desktop.quick.edit"
	// StrQuickDone — кнопка выхода из режима правки.
	StrQuickDone = "desktop.quick.done"
	// StrQuickEditHint — подсказка вместо ползунков в режиме правки.
	StrQuickEditHint = "desktop.quick.editHint"
	// StrQuickSettings — подсказка шестерёнки («Все параметры»).
	StrQuickSettings = "desktop.quick.settings"
	// StrQuickBack — подсказка стрелки «назад» вложенной панели.
	StrQuickBack = "desktop.quick.back"
	// StrQuickMore — подсказка «›» у плитки и громкости.
	StrQuickMore = "desktop.quick.more"
	// StrQuickVolume — подпись громкости: подсказка значка и заголовок
	// вложенной панели устройств, если потребитель своего не дал.
	StrQuickVolume = "desktop.quick.volume"
	// StrQuickBrightness — подсказка значка яркости.
	StrQuickBrightness = "desktop.quick.brightness"
	// StrQuickMute и StrQuickUnmute — подсказка значка громкости: нажатие
	// выключает звук или включает его снова.
	StrQuickMute   = "desktop.quick.mute"
	StrQuickUnmute = "desktop.quick.unmute"
	// StrQuickBattery — подсказка заряда батареи; один %d — проценты.
	StrQuickBattery = "desktop.quick.battery"
	// StrQuickCharging — подсказка заряда при питании от сети; один %d.
	StrQuickCharging = "desktop.quick.charging"
)

func init() {
	widget.RegisterStrings("RU", map[string]string{
		StrQuickEdit:       "Изменить быстрые параметры",
		StrQuickDone:       "Готово",
		StrQuickEditHint:   "Перетащите плитки, чтобы изменить порядок",
		StrQuickSettings:   "Все параметры",
		StrQuickBack:       "Назад",
		StrQuickMore:       "Подробнее",
		StrQuickVolume:     "Громкость",
		StrQuickBrightness: "Яркость",
		StrQuickMute:       "Выключить звук",
		StrQuickUnmute:     "Включить звук",
		StrQuickBattery:    "Заряд батареи: %d %%",
		StrQuickCharging:   "Заряд батареи: %d %%, идёт зарядка",
	})
	widget.RegisterStrings("EN", map[string]string{
		StrQuickEdit:       "Edit quick settings",
		StrQuickDone:       "Done",
		StrQuickEditHint:   "Drag tiles to change their order",
		StrQuickSettings:   "All settings",
		StrQuickBack:       "Back",
		StrQuickMore:       "Details",
		StrQuickVolume:     "Volume",
		StrQuickBrightness: "Brightness",
		StrQuickMute:       "Mute",
		StrQuickUnmute:     "Unmute",
		StrQuickBattery:    "Battery: %d%%",
		StrQuickCharging:   "Battery: %d%%, charging",
	})
}
