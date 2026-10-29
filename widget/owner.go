package widget

// owner.go — владелец дерева: адресная инвалидация (ADR-0008, задача 5.1).
//
// Уведомления виджета об изменении доходят до движка, которому принадлежит
// его дерево, и не доходят до других движков процесса. Движок привязывает
// себя к корню (BindOwner из SetRoot, ShowModal) и регистрируется в реестре
// как приёмник-владелец (RegisterUINotifierFor); виджет находит владельца
// подъёмом по цепочке родителей до верха. Рассылка идёт через тот же реестр
// (invalidate.go): приёмники-движки, кроме владельца, пропускаются, а
// наблюдатели без владельца (RegisterUINotifier без Owner, внешний слот
// SetUIChangeNotifier) получают всё, как прежде. Дерево без владельца
// уведомляет всех.

import (
	"image"
	"sync/atomic"
)

// Owner — владелец дерева виджетов: движок. Значение сравнимо (==): по нему
// приёмник реестра узнаёт себя. Метод-маркер отличает владельца от любого
// другого значения.
type Owner interface {
	// TreeOwner — маркер: тип объявляет себя владельцем деревьев.
	TreeOwner()
}

// ownerRef — держатель интерфейса под атомарный указатель.
type ownerRef struct{ o Owner }

// BindOwner привязывает владельца o к виджету w (nil — снять). Зовёт движок:
// SetRoot и ShowModal привязывают, CloseModal и Stop снимают. Виджет без Base
// привязки не хранит.
func BindOwner(w Widget, o Owner) {
	bp, ok := w.(baseProvider)
	if !ok {
		return
	}
	b := bp.baseRef()
	var prev Owner
	if ref := b.owner.Load(); ref != nil {
		prev = ref.o
	}
	if o == nil {
		b.owner.Store(nil)
	} else {
		b.owner.Store(&ownerRef{o: o})
	}
	if prev != o {
		// Раскладки, посчитанные измерителем прежнего владельца (перенос
		// лейбла, строки поля ввода), ключены ревизией метрик: дерево сменило
		// движок — пересчитать (owner_services.go).
		BumpTextMetricsRev()
	}
}

// OwnerOf — владелец дерева виджета w: привязка верхнего виджета его цепочки
// родителей; nil — дерево без владельца.
func OwnerOf(w Widget) Owner {
	if bp, ok := w.(baseProvider); ok {
		return bp.baseRef().ownerOf()
	}
	return nil
}

// ownerOf поднимается по родителям до верха и берёт привязку верхнего.
// Родитель по AddChild сильнее привязки: панель, вернувшаяся из своего окна в
// главное дерево, числится за главным движком. Привязка сильнее родителя по
// записи (recParent): корень, отданный другому движку, мог когда-то
// рисоваться ребёнком — подсказка последней записи не отменяет SetRoot.
func (b *Base) ownerOf() Owner {
	c := b
	for i := 0; i < maxScreenRectDepth; i++ {
		if p := c.parent.Load(); p != nil {
			c = p
			continue
		}
		if ref := c.owner.Load(); ref != nil {
			return ref.o
		}
		p := c.recParent.Load()
		if p == nil {
			return nil
		}
		c = p
	}
	return nil
}

// notifyOwnedRect — область r (экранная) виджета, уже помеченного
// устаревшим: владельцу дерева и наблюдателям.
func (b *Base) notifyOwnedRect(r image.Rectangle) { notifyRectTo(r, true, b.ownerOf()) }

// notifyRect — ничейная область r (экранная): контур перетаскивания окна.
func (b *Base) notifyRect(r image.Rectangle) { notifyRectTo(r, false, b.ownerOf()) }

// notifyChanged — виджет мог изменить что угодно (оверлей вне границ).
func (b *Base) notifyChanged() { notifyUIChangedTo(b.ownerOf()) }

// notifyAnimation — заведена анимация с владельцем owner: будит движок его
// дерева и наблюдателей; без виджета-владельца — всех.
func notifyAnimation(owner any) {
	if bp, ok := owner.(baseProvider); ok {
		notifyAnimationAddedTo(bp.baseRef().ownerOf())
		return
	}
	notifyAnimationAdded()
}

// skipFor — пропустить ли приёмник rcv при рассылке дереву владельца to:
// чужой движок — да; владелец, наблюдатель без владельца и рассылка без
// адресата — нет.
func (rcv *uiReceiver) skipFor(to Owner) bool {
	return to != nil && rcv.owner != nil && rcv.owner != to
}

var _ = atomic.Pointer[ownerRef]{}
