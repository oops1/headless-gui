package loop

// damage.go — повреждение кадра и рендер по запросу (бывший
// engine/invalidate.go, ADR-0011, 7.3): заявленные области в физических
// пикселях, полная инвалидация, ничейное повреждение пути команд (force) и
// поколение инвалидации (invGen), по которому цикл решает, готовить ли кадр.

import (
	"image"
	"math"
)

// MaxDamageRects — сколько областей движок держит по отдельности, прежде чем
// схлопнуть их в одно объединение.
//
// Порог нужен, чтобы список не рос без предела: сотня областей сама по себе
// дороже, чем перерисовать их общий прямоугольник. Шестнадцати хватает на
// любой разумный кадр — обычно их две-три.
const MaxDamageRects = 16

// SetRenderOnDemand включает/выключает рендер по запросу.
// Безопасно вызывать в любой момент; включение сразу инвалидирует кадр.
func (l *Loop) SetRenderOnDemand(v bool) {
	l.onDemand.Store(v)
	l.Invalidate()
}

// RenderOnDemand сообщает, включён ли рендер по запросу.
func (l *Loop) RenderOnDemand() bool {
	return l.onDemand.Load()
}

// Invalidate помечает весь кадр как изменившийся: ближайший тик отрендерит его.
// Дёшево (атомарный инкремент) — можно вызывать часто.
func (l *Loop) Invalidate() {
	// Сначала поколение записи (cmdpath.go), потом повреждение: кадр,
	// заставший полное повреждение, обязан застать и новое поколение — иначе
	// он воспроизвёл бы устаревшие списки как полный кадр.
	l.host.OnInvalidateAll()
	l.damageMu.Lock()
	l.damageAll = true
	l.damageMu.Unlock()
	l.invGen.Add(1)
}

// InvalidateRect помечает изменившейся прямоугольную область (в ЛОГИЧЕСКИХ
// пикселях холста — система координат виджетов). Diff ближайшего кадра
// ограничится тайлами, пересекающими ЗАЯВЛЕННЫЕ области.
// Внутри damage хранится в физических пикселях (масштабируется здесь).
//
// Области хранятся списком, а не одним объединением. Разница видна там, где
// изменения далеко друг от друга: перетаскивание рамки окна двигает две
// узкие полосы на разных краях экрана, и их объединение — почти весь экран.
// По объединению уходили тайлы всего прямоугольника (десятки килобайт на
// кадр), по списку — только тайлы самих полос.
func (l *Loop) InvalidateRect(r image.Rectangle) {
	if r.Empty() {
		return
	}
	// Ничейное повреждение (cmdpath.go): под областью могло измениться то, о
	// чём ни один виджет не сообщил, — виджеты под ней перезаписываются.
	l.damageMu.Lock()
	l.force = addDamage(l.force, r)
	l.damageMu.Unlock()
	l.DamageRect(r)
}

// DamageRect — InvalidateRect без ничейного повреждения: для областей, под
// которыми дерево виджетов не менялось (подсказка, переходы оверлеев) и для
// уведомлений виджетов, уже помеченных устаревшими (widget/retain.go).
func (l *Loop) DamageRect(r image.Rectangle) {
	if l.NoteDamage(r) {
		l.invGen.Add(1)
	}
}

// NoteDamage добавляет область к повреждению кадра, не объявляя UI
// изменившимся (invGen): для кадра, который цикл и так готовит. false —
// область пуста.
func (l *Loop) NoteDamage(r image.Rectangle) bool {
	if r.Empty() {
		return false
	}
	// Масштаб читается lock-free (scaleBits): InvalidateRect вызывается из
	// сеттеров виджетов, в т.ч. когда движок уже держит свой e.mu.
	if k := l.host.Scale(); k != 1 {
		r = scaleRectF(r, k)
	}
	l.damageMu.Lock()
	if !l.damageAll {
		l.damage = addDamage(l.damage, r)
	}
	l.damageMu.Unlock()
	return true
}

// addDamage добавляет область к списку, поглощая вложенные.
//
// Вложенные отбрасываются не ради экономии памяти, а потому что тайлы всё
// равно сравнивались бы дважды: первое сравнение синхронизирует буферы, и
// второе вернуло бы «не изменилось» — работа впустую.
func addDamage(list []image.Rectangle, r image.Rectangle) []image.Rectangle {
	// Проверка «уже покрыта» вынесена в отдельный проход СПЕЦИАЛЬНО, хотя
	// одного цикла хватило бы. Компактация идёт «на месте» (out := list[:0] —
	// тот же массив), и ранний возврат из общего цикла отдавал бы наружу уже
	// сдвинутый список. Сегодня это безвредно: сдвиг и ранний возврат разом
	// требуют вложенной пары в списке, а её тут не бывает — обе ветки ниже
	// вложенность как раз и устраняют. Но правильность кода не должна
	// держаться на этом рассуждении: два прохода по списку из шестнадцати
	// прямоугольников ничего не стоят, а рассуждение сломается от первой же
	// правки условий.
	for _, cur := range list {
		if r.In(cur) {
			return list // новая область уже покрыта — список не меняется
		}
	}

	out := list[:0]
	for _, cur := range list {
		if cur.In(r) {
			continue // старая поглощается новой
		}
		out = append(out, cur)
	}
	out = append(out, r)
	if len(out) > MaxDamageRects {
		// Порог пройден: дальше дешевле работать одним прямоугольником.
		u := out[0]
		for _, cur := range out[1:] {
			u = u.Union(cur)
		}
		return append(out[:0], u)
	}
	return out
}

// scaleRectF масштабирует логический прямоугольник в физический по краям
// (та же математика, что canvas.sRect, но без доступа к канвасу).
func scaleRectF(r image.Rectangle, k float64) image.Rectangle {
	return image.Rect(
		int(math.Round(float64(r.Min.X)*k)),
		int(math.Round(float64(r.Min.Y)*k)),
		int(math.Round(float64(r.Max.X)*k)),
		int(math.Round(float64(r.Max.Y)*k)),
	)
}

// ConsumeDamage атомарно забирает накопленные повреждения.
// all==true — нужен полный diff (Invalidate или режим по умолчанию).
// WholeFramePending — следующий кадр будет полным: заявлена полная
// инвалидация или полный кадр (fullframe.go). Без сброса.
func (l *Loop) WholeFramePending() bool {
	l.damageMu.Lock()
	all := l.damageAll
	l.damageMu.Unlock()
	return all || l.host.FullFramePending()
}

func (l *Loop) ConsumeDamage() (regions []image.Rectangle, all bool) {
	l.damageMu.Lock()
	regions, all = l.damage, l.damageAll
	l.damage = nil
	l.damageAll = false
	l.damageMu.Unlock()
	return regions, all
}

// PeekDamage — объединение заявленного на сейчас, не забирая его: что
// перерисует ближайший кадр как минимум. all — весь кадр.
func (l *Loop) PeekDamage() (union image.Rectangle, all bool) {
	l.damageMu.Lock()
	defer l.damageMu.Unlock()
	return unionRects(l.damage), l.damageAll
}

// unionRects — общий прямоугольник списка (пустой для пустого списка).
func unionRects(rects []image.Rectangle) image.Rectangle {
	var u image.Rectangle
	for _, r := range rects {
		u = u.Union(r)
	}
	return u
}

// TakeForce забирает накопленное ничейное повреждение (логическое). Зовётся
// ПОСЛЕ consumeDamage: область, заявленная между двумя вызовами, перезапишет
// виджет уже сейчас, а её damage дойдёт следующим кадром — наоборот список
// остался бы устаревшим под уже забранным damage.
func (l *Loop) TakeForce(dst []image.Rectangle) []image.Rectangle {
	l.damageMu.Lock()
	dst = append(dst[:0], l.force...)
	l.force = l.force[:0]
	l.damageMu.Unlock()
	return dst
}
