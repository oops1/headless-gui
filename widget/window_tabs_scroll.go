package widget

// window_tabs_scroll.go — куда прокрутить полосу вкладок, чтобы активная
// была видна целиком.
//
// Отдельной чистой функцией: это арифметика, и проверять её удобнее прямо,
// а не через отрисовку окна.

// tabScrollFor возвращает сдвиг полосы, при котором вкладка idx помещается
// в видимую ширину avail целиком.
//
// Вкладка левее видимого — прижимаем её к левому краю, правее — к правому;
// видимую не трогаем, иначе полоса дёргалась бы на каждом кадре.
func tabScrollFor(widths []int, idx, avail, scroll int) int {
	if idx < 0 || idx >= len(widths) || avail <= 0 {
		return scroll
	}
	start := 0
	for i := 0; i < idx; i++ {
		start += widths[i] + titleTabGap
	}
	end := start + widths[idx]

	if start < scroll {
		return start
	}
	if end > scroll+avail {
		return end - avail
	}
	return scroll
}
