package widget

// clicks.go — номер нажатия в серии (двойной, тройной щелчок).
//
// Номер считает движок и кладёт в MouseEvent.Clicks: интервал там системный,
// и считается он один раз на всё дерево. Но событие может прийти и не от
// движка — прямым вызовом OnMouseButton из теста или из приложения со своей
// доставкой, — и тогда Clicks равен нулю. На этот случай виджет считает сам,
// как считал до появления поля.

import "time"

const (
	// seriesInterval — порог серии в запасном счётчике. 400 мс — то значение,
	// с которым виджеты жили до появления MouseEvent.Clicks; менять его здесь
	// незачем, системный интервал приходит готовым номером.
	seriesInterval = 400

	// seriesSlack — допустимый сдвиг курсора между нажатиями серии (точки).
	seriesSlack = 4
)

// clickSeries — запасной счётчик нажатий на стороне виджета.
type clickSeries struct {
	atMs int64
	x, y int
	n    int
}

// clicksOf — номер нажатия в серии: из события, если его заполнил движок,
// иначе посчитанный на месте. Зовётся только для нажатия (e.Pressed).
func clicksOf(e MouseEvent, c *clickSeries) int {
	if e.Clicks > 0 {
		return e.Clicks
	}
	nowMs := time.Now().UnixMilli()
	near := absInt(e.X-c.x) <= seriesSlack && absInt(e.Y-c.y) <= seriesSlack
	if c.n > 0 && near && nowMs-c.atMs <= seriesInterval {
		c.n++
	} else {
		c.n = 1
	}
	c.atMs, c.x, c.y = nowMs, e.X, e.Y
	return c.n
}
