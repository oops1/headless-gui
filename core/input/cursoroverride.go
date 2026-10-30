// cursoroverride.go — принудительная форма курсора поверх курсора виджетов.
//
// Курсор до сих пор определялся только тем, что лежит под мышью, и менялся
// только после её движения. Приложению этого мало: просмотрщик изображений
// прячет курсор после паузы без движения мыши (мыши-то как раз никто не
// двигает), а на время перетаскивания показывает «сжатую руку» над любым
// виджетом, в том числе над тем, что курсор задаёт сам.
//
// SetCursorOverride задаёт такую форму поверх всего дерева; окно узнаёт о ней
// через SetOnCursorOverrideChanged и применяет сразу, не дожидаясь движения.
//
// SetCursorImage — то же, но форма не из набора widget.Cursor, а картинка
// (указатель удалённого рабочего стола: до 384×384, с альфой).

package input

import (
	"image"
	"sync"

	"github.com/oops1/headless-gui/v4/widget"
)

// cursorOverride — состояние принудительного курсора. Нулевое значение готово
// к работе: override не задан. Замок собственный: вызвать SetCursorOverride
// могут с любой горутины (таймер бездействия), а CursorAt читает его на
// горутине движка.
type cursorOverride struct {
	mu  sync.Mutex
	c   widget.Cursor
	set bool
	// img — курсор-картинка (SetCursorImage). Задан — set тоже true, а c —
	// стрелка: так тот, кто про картинки не знает (CursorOverride), видит
	// «override есть» и не путает форму с прежней.
	img      *widget.CursorImage
	onChange func()
}

// SetCursorOverride задаёт форму курсора, которая перекрывает курсор любого
// виджета под мышью, пока не будет снята ClearCursorOverride.
//
// Форма применяется к окну немедленно — без ожидания движения мыши: так можно
// спрятать курсор (widget.CursorNone) по таймеру бездействия. Безопасен для
// вызова с любой горутины. Повторный вызов с той же формой окно не беспокоит.
//
// Форма действует и над вынесенными попапами (меню и выпадающие списки в
// отдельных окнах ОС): окно движка (window.Run) переносит её на каждый попап,
// в том числе открытый уже после вызова. Снятая — возвращает попапам стрелку.
//
// Заменяет и курсор-картинку (SetCursorImage): действует последний вызов.
func (d *Dispatcher) SetCursorOverride(c widget.Cursor) {
	o := &d.cursorOvr
	o.mu.Lock()
	changed := !o.set || o.c != c || o.img != nil
	o.c, o.set, o.img = c, true, nil
	fn := o.onChange
	o.mu.Unlock()
	if changed && fn != nil {
		fn()
	}
}

// SetCursorImage показывает указатель из картинки: поверх курсора любого
// виджета и над окнами-попапами, как SetCursorOverride. Картинка — NRGBA (прямая
// альфа) до 384×384 (RDP; допустимо до widget.MaxCursorImageSide), hotspotX,
// hotspotY — горячая точка в её пикселях.
//
// Картинка копируется: приложение вправе менять свой буфер сразу после вызова.
// Повторная установка той же картинки (тот же хеш содержимого и горячей точки)
// окно не беспокоит и не создаёт системный курсор заново — берётся из кэша окна
// (до 16 последних картинок), так что слать указатель на каждое движение мыши
// не накладно. Безопасен для вызова с любой горутины.
//
// Сброс — ClearCursorOverride или SetCursorImage(nil, 0, 0). Пустая или
// недопустимая картинка (сторона больше widget.MaxCursorImageSide) действует
// как nil: снимает курсор-картинку, но не принудительную форму SetCursorOverride.
// Ограничение не зависит от платформы: курсор WinLine RDP — до 32×32, но
// картинки крупнее допустимы, если система их показывает.
func (d *Dispatcher) SetCursorImage(img *image.NRGBA, hotspotX, hotspotY int) {
	// Ключ считается по буферу приложения без копии: повторная установка той же
	// картинки (указатель шлют на каждое обновление) не выделяет память.
	key, valid := widget.CursorImageKey(img, hotspotX, hotspotY)
	o := &d.cursorOvr
	o.mu.Lock()
	var changed bool
	switch {
	case !valid:
		if o.img != nil { // снять картинку: как ClearCursorOverride
			o.c, o.set, o.img = widget.CursorArrow, false, nil
			changed = true
		}
	case o.img != nil && o.img.Key == key:
		// та же картинка — ничего не меняется
	default:
		if ci := widget.NewCursorImage(img, hotspotX, hotspotY); ci != nil {
			o.c, o.set, o.img = widget.CursorArrow, true, ci
			changed = true
		}
	}
	fn := o.onChange
	o.mu.Unlock()
	if changed && fn != nil {
		fn()
	}
}

// ClearCursorOverride снимает принудительный курсор (форму и картинку): форму
// снова определяет виджет под мышью. Окно возвращает её сразу, по последнему
// положению мыши.
func (d *Dispatcher) ClearCursorOverride() {
	o := &d.cursorOvr
	o.mu.Lock()
	changed := o.set
	o.c, o.set, o.img = widget.CursorArrow, false, nil
	fn := o.onChange
	o.mu.Unlock()
	if changed && fn != nil {
		fn()
	}
}

// CursorOverride возвращает принудительную форму курсора и признак, что она
// задана. Курсор-картинка (SetCursorImage) виден здесь как «задан, стрелка»;
// сама картинка — в CursorImageOverride.
func (d *Dispatcher) CursorOverride() (widget.Cursor, bool) {
	o := &d.cursorOvr
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.c, o.set
}

// CursorImageOverride возвращает курсор-картинку, заданную SetCursorImage.
// Значение неизменяемое, его можно хранить и читать с любой горутины.
func (d *Dispatcher) CursorImageOverride() (*widget.CursorImage, bool) {
	o := &d.cursorOvr
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.img, o.img != nil
}

// SetOnCursorOverrideChanged подключает уведомление о том, что принудительный
// курсор (форма или картинка) задан, сменён или снят. Его ставит окно
// (window.Run): по уведомлению оно заново спрашивает CursorAt и отдаёт форму
// ОС. fn зовётся на горутине того, кто менял override, вне замков движка; nil
// снимает уведомление.
func (d *Dispatcher) SetOnCursorOverrideChanged(fn func()) {
	o := &d.cursorOvr
	o.mu.Lock()
	o.onChange = fn
	o.mu.Unlock()
}
