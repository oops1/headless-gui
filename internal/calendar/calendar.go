// Package calendar — сетка месяца без вида: какие дни стоят в какой неделе.
//
// Живёт отдельно, потому что календарей в движке два — выпадающий календарь
// рабочего стола (desktop) и выбор даты (widget.DatePicker), — а desktop
// зависит от widget, и общий расчёт положить можно только ниже обоих. Два
// разных расчёта одной сетки однажды разошлись бы на неделю.
package calendar

import "time"

// Day — ячейка сетки: дата и признак, что она из показанного месяца. Дни
// соседних месяцев тоже попадают в сетку — иначе первая и последняя неделя
// обрывались бы неполными рядами вместо ровных семи столбцов.
type Day struct {
	Date    time.Time
	InMonth bool
}

// WeekdayIndex — номер дня недели t в неделе, начинающейся с first: 0 — первый
// столбец сетки.
func WeekdayIndex(t time.Time, first time.Weekday) int {
	return (int(t.Weekday()) - int(first) + 7) % 7
}

// MonthGrid строит полные недели месяца month года year: от first-дня недели,
// содержащей 1-е число, до последнего дня недели, содержащей последнее число.
//
// Первый день недели — параметр, а не константа: в русской и европейской
// культуре неделя начинается с понедельника, в американской — с воскресенья,
// и календарь, начинающий не с того дня, сдвигает каждое число на столбец.
func MonthGrid(year int, month time.Month, loc *time.Location, first time.Weekday) [][]Day {
	if loc == nil {
		loc = time.Local
	}
	start1 := time.Date(year, month, 1, 0, 0, 0, 0, loc)
	start := start1.AddDate(0, 0, -WeekdayIndex(start1, first))

	last := start1.AddDate(0, 1, -1)
	end := last.AddDate(0, 0, 6-WeekdayIndex(last, first))

	var rows [][]Day
	for d := start; !d.After(end); {
		row := make([]Day, 7)
		for col := range row {
			row[col] = Day{Date: d, InMonth: d.Month() == month}
			d = d.AddDate(0, 0, 1)
		}
		rows = append(rows, row)
	}
	return rows
}

// SameDay сравнивает даты без времени суток.
func SameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// FirstOfMonth — полночь первого числа месяца, в котором лежит t.
func FirstOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// DateOnly — полночь того же дня: выбор даты не должен тащить за собой время
// суток, иначе сравнение «с такого-то по такое-то» зависело бы от часа выбора.
func DateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
