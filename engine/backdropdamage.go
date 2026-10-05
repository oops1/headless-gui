package engine

// backdropdamage.go — частичная перерисовка и размытая подложка.
//
// Частичный кадр восстанавливает фон только в повреждённой области, а
// размытие слоя (BlurBehind) берёт пиксели и вокруг неё: за краем повреждения
// лежит прошлый кадр — уже размытый и подкрашенный. Получалось размытие
// поверх размытия, и место, где навели мышь или поставили фокус, светлело
// пятном на панели задач Windows 11 и в акриле Windows 10. Вдобавок
// уменьшение перед размытием идёт блоками от начала снимаемой области, и
// обрезанная повреждением область давала другую сетку блоков — результат
// расходился с полным кадром даже на свежем фоне.
//
// Поэтому канвас помнит, где в прошлом кадре было размытие, и частичный кадр,
// задевший такую область, расширяется на неё целиком: внутри слоя всё
// перерисовывается так же, как в полном кадре, и пиксели совпадают. Запас
// радиуса за краем слоя перерисовывать не нужно — там в прошлом кадре лежит
// то же, что и сейчас (размытие его не трогает), а лишняя перерисовка будила
// бы соседей: тик часов на панели задач перерисовывал бы обои над ней.

import "image"

// noteBackdrop запоминает область размытия текущего кадра (физическая).
// Вызывается из BlurBehind.
func (c *Canvas) noteBackdrop(r image.Rectangle) {
	r = r.Intersect(c.back.Bounds())
	if !r.Empty() {
		c.backdropsNext = append(c.backdropsNext, r)
	}
}

// expandForBackdrops расширяет области повреждения частичного кадра на
// области размытия прошлого кадра, которые они задевают. Расширение может
// задеть следующую область — повторяется, пока набор не устоится.
func (c *Canvas) expandForBackdrops(damage []image.Rectangle) []image.Rectangle {
	if len(c.backdrops) == 0 || len(damage) == 0 {
		return damage
	}
	used := make([]bool, len(c.backdrops))
	for changed := true; changed; {
		changed = false
		for i, b := range c.backdrops {
			if used[i] {
				continue
			}
			for _, d := range damage {
				if d.Overlaps(b) {
					used[i] = true
					damage = append(damage, b)
					changed = true
					break
				}
			}
		}
	}
	return damage
}

// finishBackdrops закрывает кадр: области размытия, нарисованные в нём,
// становятся «прошлыми». В частичном кадре слои вне повреждения не
// рисовались, но их размытие на экране осталось — их прежние области
// сохраняются.
func (c *Canvas) finishBackdrops(partial bool, damage []image.Rectangle) {
	next := c.backdropsNext
	if partial {
		for _, b := range c.backdrops {
			touched := false
			for _, d := range damage {
				if d.Overlaps(b) {
					touched = true
					break
				}
			}
			if !touched {
				next = append(next, b)
			}
		}
	}
	c.backdrops, c.backdropsNext = next, c.backdrops[:0]
}
