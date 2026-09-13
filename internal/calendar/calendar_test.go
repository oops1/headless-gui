package calendar

import (
	"testing"
	"time"
)

// Сетка любого месяца за много лет и при обоих первых днях недели: ряды ровно
// по семь дней подряд без пропусков, первый столбец — заданный день недели,
// все числа месяца на месте и только один раз, соседние месяцы помечены.
func TestMonthGridShape(t *testing.T) {
	loc := time.UTC
	for _, first := range []time.Weekday{time.Monday, time.Sunday} {
		for year := 1999; year <= 2031; year++ {
			for month := time.January; month <= time.December; month++ {
				rows := MonthGrid(year, month, loc, first)
				if len(rows) < 4 || len(rows) > 6 {
					t.Fatalf("%d-%02d (с %v): %d недель", year, month, first, len(rows))
				}
				seen := map[int]int{}
				prev := time.Time{}
				for r, row := range rows {
					if len(row) != 7 {
						t.Fatalf("%d-%02d: в неделе %d дней %d", year, month, r, len(row))
					}
					if row[0].Date.Weekday() != first {
						t.Fatalf("%d-%02d: неделя %d начинается с %v, а не с %v",
							year, month, r, row[0].Date.Weekday(), first)
					}
					for _, d := range row {
						if !prev.IsZero() && !SameDay(d.Date, prev.AddDate(0, 0, 1)) {
							t.Fatalf("%d-%02d: после %v идёт %v", year, month, prev, d.Date)
						}
						prev = d.Date
						if d.InMonth != (d.Date.Month() == month) {
							t.Fatalf("%d-%02d: у %v InMonth=%v", year, month, d.Date, d.InMonth)
						}
						if d.InMonth {
							seen[d.Date.Day()]++
						}
					}
				}
				days := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
				for day := 1; day <= days; day++ {
					if seen[day] != 1 {
						t.Fatalf("%d-%02d: число %d встретилось %d раз", year, month, day, seen[day])
					}
				}
				// Первая неделя содержит 1-е число, последняя — последнее.
				if !rows[0][WeekdayIndex(time.Date(year, month, 1, 0, 0, 0, 0, loc), first)].InMonth {
					t.Fatalf("%d-%02d: 1-е число не в первой неделе", year, month)
				}
			}
		}
	}
}

// Известный месяц: август 2026 начинается в субботу.
func TestMonthGridAugust2026(t *testing.T) {
	mon := MonthGrid(2026, time.August, time.UTC, time.Monday)
	if got := mon[0][0].Date; !SameDay(got, time.Date(2026, time.July, 27, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("с понедельника сетка начинается %v, ждал 27 июля", got)
	}
	sun := MonthGrid(2026, time.August, time.UTC, time.Sunday)
	if got := sun[0][0].Date; !SameDay(got, time.Date(2026, time.July, 26, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("с воскресенья сетка начинается %v, ждал 26 июля", got)
	}
}

func TestDateHelpers(t *testing.T) {
	at := time.Date(2026, time.September, 13, 17, 45, 12, 5, time.UTC)
	if got := DateOnly(at); !got.Equal(time.Date(2026, time.September, 13, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("DateOnly = %v", got)
	}
	if got := FirstOfMonth(at); !got.Equal(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("FirstOfMonth = %v", got)
	}
	if !SameDay(at, DateOnly(at)) || SameDay(at, at.AddDate(0, 0, 1)) {
		t.Fatal("SameDay ошибается")
	}
}
