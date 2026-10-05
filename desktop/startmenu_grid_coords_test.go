package desktop_test

import "image"

// Координаты по заданию (100 %).
//
// Тесты кликают по местам, которые называет задание: поля 32, поиск сверху 28 и
// 32 высотой, заголовок раздела 28, сетка 6×96 по центру, нижняя полоса 64.

// cell — центр ячейки закреплённого (колонка, ряд) первой страницы.
func (s *win11Scene) cell(col, row int) image.Point {
	r := s.rect()
	m := s.tm.GetMetric
	x := r.Min.X + (r.Dx()-6*96)/2 + col*96 + 48
	top := r.Min.Y + int(m("startmenu.w11.search.top")+m("startmenu.w11.search.height")) +
		int(m("startmenu.w11.section.gap")) + 4 + int(m("startmenu.w11.section.height")) + 8
	return image.Pt(x, top+row*(84+int(m("startmenu.w11.grid.gap.y")))+42)
}

// searchPt — точка в строке поиска.
func (s *win11Scene) searchPt() image.Point {
	r := s.rect()
	return image.Pt(r.Min.X+200, r.Min.Y+28+16)
}

// allBtn — кнопка «Все приложения ›».
func (s *win11Scene) allBtn() image.Point {
	r := s.rect()
	return image.Pt(r.Max.X-32-20, r.Min.Y+28+32+20+14)
}

// footerY — середина нижней полосы по вертикали.
func (s *win11Scene) footerY() int { r := s.rect(); return r.Max.Y - 1 - 32 }

func (s *win11Scene) userBtn() image.Point  { return image.Pt(s.rect().Min.X+32+10, s.footerY()) }
func (s *win11Scene) powerBtn() image.Point { return image.Pt(s.rect().Max.X-32-12, s.footerY()) }

// recAt — центр строки «Рекомендуем» (колонка, ряд); три ряда прижаты к нижней
// полосе с зазором 8.
func (s *win11Scene) recAt(col, row int) image.Point {
	r := s.rect()
	bottom := r.Max.Y - 1 - 64 - 8
	top := bottom - 3*56
	colW := (r.Dx() - 64) / 2
	return image.Pt(r.Min.X+32+col*colW+colW/3, top+row*56+28)
}

// moreBtn — кнопка «Дополнительно ›».
func (s *win11Scene) moreBtn() image.Point {
	r := s.rect()
	top := r.Max.Y - 1 - 64 - 8 - 3*56
	return image.Pt(r.Max.X-32-20, top-8-14)
}
