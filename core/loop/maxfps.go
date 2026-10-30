package loop

import "time"

// SetMaxFPS ограничивает частоту кадров сверху: движок рисует не чаще fps
// кадров в секунду. Ноль и меньше снимают ограничение.
//
// Это потолок, а не новая целевая частота: выше fps, с которым создан движок
// (New), он не поднимет — движок, созданный на 20 кадров, при SetMaxFPS(60)
// останется на 20. Потолок нужен там, где частота кадров — решение не создателя
// движка, а владельца окна: Window.SetMaxFPS передаёт сюда своё значение.
//
// Можно вызывать в любой момент и с любой горутины; цикл кадров подхватывает
// новое значение на ближайшем тике. Шаг анимаций идёт в том же темпе.
func (l *Loop) SetMaxFPS(fps int) {
	if fps < 0 {
		fps = 0
	}
	l.maxFPS.Store(int32(fps))
}

// FrameInterval — период кадра с учётом потолка SetMaxFPS.
func (l *Loop) FrameInterval() time.Duration {
	fps := l.fps
	if c := int(l.maxFPS.Load()); c > 0 && c < fps {
		fps = c
	}
	return time.Duration(float64(time.Second) / float64(fps))
}
