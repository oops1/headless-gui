package widget

import "image"

// textbox_hbar.go — геометрия горизонтальной полосы прокрутки редактора.
//
// Смещение scrollX у редактора без переноса по словам было, и каретка его
// подводила, но задать его иначе было нечем: ни полосы, ни горизонтального
// колеса. Правый конец длинной строки оставался недоступен мышью. Сама полоса —
// трек, ползунок, перетаскивание — уже есть у сравнения файлов (hscrollbar.go);
// здесь только то, что принадлежит редактору: пределы смещения и положение
// трека. Чистыми функциями — чтобы проверять без отрисовки.

// tbHBarH — высота полосы вместе с полями. Столько же отнимается у текста снизу,
// пока полоса показана.
const tbHBarH = dvHBarH

// tbCaretRoom — запас справа от самой длинной строки: каретка в её конце
// должна быть видна, а не прижата к срезу (ensureCaretVisible оставляет те же
// четыре пикселя).
const tbCaretRoom = 4

// tbScrollXMax — наибольшее смещение вбок для содержимого шириной contentW в
// области шириной viewW. Запас под каретку входит в предел: иначе зажим
// смещения возвращал бы текст, подведённый к каретке в конце строки.
func tbScrollXMax(contentW, viewW int) int {
	if m := contentW + tbCaretRoom - viewW; m > 0 {
		return m
	}
	return 0
}

// tbClampScrollX зажимает смещение в [0, tbScrollXMax].
func tbClampScrollX(v, contentW, viewW int) int {
	if m := tbScrollXMax(contentW, viewW); v > m {
		v = m
	}
	if v < 0 {
		v = 0
	}
	return v
}

// tbNeedHBar — нужна ли полоса: только без переноса по словам и только когда
// самая длинная строка не помещается. С переносом строки никогда не шире
// области, и полоса отняла бы высоту впустую.
func tbNeedHBar(wrap bool, contentW, viewW int) bool {
	return !wrap && contentW > viewW
}

// tbHBarTrack — трек полосы под областью текста: тот же отступ слева и та же
// ширина, что у текста, над нижней рамкой.
func tbHBarTrack(b image.Rectangle, padX, viewW int) image.Rectangle {
	y := b.Max.Y - tbHBarH + 1
	x := b.Min.X + padX
	return image.Rect(x, y, x+viewW, y+tbHBarH-4)
}

// tbHBarHit — где нажатие считается нажатием на полосу: трек с запасом по
// сторонам и вся полоса по высоте. Трек в шесть пикселей тонок, попасть в него
// мышью без запаса трудно.
func tbHBarHit(b image.Rectangle, padX, viewW int) image.Rectangle {
	tr := tbHBarTrack(b, padX, viewW)
	return image.Rect(tr.Min.X-dvHBarInsetX, b.Max.Y-tbHBarH, tr.Max.X+dvHBarInsetX, b.Max.Y)
}
