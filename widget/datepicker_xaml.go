package widget

import (
	"strconv"
	"strings"
	"time"
)

// datepicker_xaml.go — тег <DatePicker> в разметке.
//
//	<DatePicker x:Name="since" SelectedDate="2026-09-13"
//	            DisplayDateStart="2020-01-01" DisplayDateEnd="2026-12-31"
//	            DateFormat="dd.MM.yyyy" FirstDayOfWeek="Monday"
//	            Placeholder="с какого числа" FontSize="10"
//	            SelectedDateChangedCommand="{Binding Since}"/>
//
// Даты в разметке — ISO 8601 (2026-09-13) или, как в WPF, инвариантной
// культурой M/d/yyyy: разметка не должна менять смысл от языка интерфейса.
// DateFormat принимает шаблон в духе .NET (dd, d, MM, M, yyyy, yy) или готовую
// раскладку time.Format.

func buildXAMLDatePicker(el xElement) Widget {
	p := NewDatePicker()
	if v := el.attr("DateFormat", "SelectedDateFormat"); v != "" {
		p.SetFormat(dotnetDateLayout(v))
	}
	if v := el.attr("FirstDayOfWeek"); v != "" {
		if d, ok := parseXAMLWeekday(v); ok {
			p.SetFirstDayOfWeek(d)
		}
	}
	start, _ := parseXAMLDate(el.attr("DisplayDateStart"))
	end, _ := parseXAMLDate(el.attr("DisplayDateEnd"))
	if !start.IsZero() || !end.IsZero() {
		p.SetDisplayDateRange(start, end)
	}
	if d, ok := parseXAMLDate(el.attr("SelectedDate")); ok {
		p.SetSelectedDate(d)
	}
	if v := el.attr("Placeholder", "Watermark"); v != "" {
		p.Placeholder = v
	}
	if fs := el.attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			p.FontSize = v
		}
	}
	return p
}

// parseXAMLDate — дата разметки: ISO 8601 или M/d/yyyy.
func parseXAMLDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02", "1/2/2006"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseXAMLWeekday(s string) (time.Weekday, bool) {
	for d := time.Sunday; d <= time.Saturday; d++ {
		if strings.EqualFold(s, d.String()) {
			return d, true
		}
	}
	return 0, false
}

// dotnetDateLayout переводит шаблон даты в духе .NET в раскладку time.Format.
// Строка, уже содержащая «2006», считается готовой раскладкой.
func dotnetDateLayout(s string) string {
	if strings.Contains(s, "2006") {
		return s
	}
	return strings.NewReplacer(
		"yyyy", "2006", "yy", "06",
		"MM", "01", "M", "1",
		"dd", "02", "d", "2",
	).Replace(s)
}
