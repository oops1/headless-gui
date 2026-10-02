package window

// waylandpopupwire.go — чистая часть xdg_popup: как попросить компоновщик
// поставить всплывающее окно в нужное место и как прочитать его ответ.
//
// Файл без платформенного суффикса намеренно — по тем же причинам, что и
// waylandwire.go: сокет компоновщика есть только под Linux, а раскладку байтов
// нужно проверять на любой машине, в общем прогоне тестов.
//
// Зачем всё это. На Wayland у клиента НЕТ экранных координат: окно не знает,
// где оно стоит, и поставить второе окно «вот в эту точку экрана» нельзя.
// Поэтому меню и выпадающие списки, которым не хватает места в окне, раньше
// обрезались его краем — в узком окне Блокнота (эталон 490 px) «Файл» уезжал
// за границу. Для них в протоколе есть xdg_popup: окно-потомок, чьё место
// задаётся ОТНОСИТЕЛЬНО родителя через xdg_positioner, а решение, куда оно в
// итоге влезет, принимает компоновщик — он один знает размер экрана.

import "encoding/binary"

const (
	// xdg_wm_base
	xdgWmBaseCreatePositioner = 1

	// xdg_surface
	xdgSurfaceGetPopup          = 2
	xdgSurfaceSetWindowGeometry = 3

	// xdg_positioner
	xdgPositionerDestroy                 = 0
	xdgPositionerSetSize                 = 1
	xdgPositionerSetAnchorRect           = 2
	xdgPositionerSetAnchor               = 3
	xdgPositionerSetGravity              = 4
	xdgPositionerSetConstraintAdjustment = 5

	// xdg_positioner.anchor / gravity (значения общие для обоих перечислений)
	xdgAnchorTopLeft      = 5
	xdgGravityBottomRight = 8

	// xdg_positioner.constraint_adjustment — что компоновщику разрешено
	// сделать, если попап не влезает на экран.
	xdgConstraintSlideX = 1
	xdgConstraintSlideY = 2
	xdgConstraintFlipY  = 8

	// xdg_popup
	xdgPopupDestroy     = 0
	xdgPopupGrab        = 1
	xdgPopupEvConfigure = 0
	xdgPopupEvPopupDone = 1
)

// wlPopupConstraint — что разрешено компоновщику, когда попап не помещается.
//
// Сдвиг по обеим осям и переворот по вертикали: меню у нижнего края экрана
// должно раскрываться ВВЕРХ, как это делают все меню, а не упираться в край и
// терять нижние пункты. Переворот по горизонтали не разрешаем: меню,
// прыгнувшее влево от курсора, человек читает как промах мыши.
const wlPopupConstraint = xdgConstraintSlideX | xdgConstraintSlideY | xdgConstraintFlipY

// wlPositionerRequests — запросы позиционеру, после которых попап встаёт
// левым верхним углом ровно в точку (x, y) окна-родителя.
//
// Приём стандартный: якорная область — точка 1×1 в нужном месте, якорь — её
// левый верхний угол, тяготение — вправо-вниз. Тогда попап растёт от точки
// в ту же сторону, в какую его рисует движок.
//
// Координаты и размер — в координатах поверхности родителя (у движка они
// физические, как и всё, что уходит в бэкенд).
func wlPositionerRequests(posID uint32, x, y, w, h int) [][]byte {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return [][]byte{
		newWlMsg(posID, xdgPositionerSetSize).putInt(int32(w)).putInt(int32(h)).bytes(),
		newWlMsg(posID, xdgPositionerSetAnchorRect).
			putInt(int32(x)).putInt(int32(y)).putInt(1).putInt(1).bytes(),
		newWlMsg(posID, xdgPositionerSetAnchor).putUint(xdgAnchorTopLeft).bytes(),
		newWlMsg(posID, xdgPositionerSetGravity).putUint(xdgGravityBottomRight).bytes(),
		newWlMsg(posID, xdgPositionerSetConstraintAdjustment).putUint(wlPopupConstraint).bytes(),
	}
}

// wlParsePopupConfigure читает xdg_popup.configure: место и размер, которые
// компоновщик в итоге дал попапу, относительно окна-родителя.
//
// Читать его обязательно, а не считать, что попап встал куда просили:
// компоновщик вправе сдвинуть или перевернуть окно, чтобы оно влезло на
// экран. Клики приходят в координатах поверхности попапа, и чтобы перевести
// их обратно в координаты окна, нужно именно это — фактическое место.
func wlParsePopupConfigure(b []byte) (x, y, w, h int, ok bool) {
	if len(b) < 16 {
		return 0, 0, 0, 0, false
	}
	x = int(int32(binary.LittleEndian.Uint32(b[0:4])))
	y = int(int32(binary.LittleEndian.Uint32(b[4:8])))
	w = int(int32(binary.LittleEndian.Uint32(b[8:12])))
	h = int(int32(binary.LittleEndian.Uint32(b[12:16])))
	return x, y, w, h, true
}
