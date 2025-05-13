package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func parseInt(s string) (int, error) {
	var i int
	_, err := fmt.Sscan(s, &i)
	return i, err
}

func parseTimeId(id int) time.Duration {
	seconds := id % 100
	minutes := (id / 100) % 100
	hours := id / 10000
	return time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
}

func parseDateId(dateId int) (time.Time, error) {
	dateStr := strconv.Itoa(dateId)
	return time.Parse("20060102", dateStr)
}

func parseGanttDuration(input string) (time.Duration, error) {
	var total time.Duration
	var num int
	var unit string

	parts := strings.Fields(input)
	for _, part := range parts {
		_, err := fmt.Sscanf(part, "%d%s", &num, &unit)
		if err != nil {
			return 0, fmt.Errorf("ошибка сканирования %s", part)
		}
		switch unit {
		case "w":
			total += time.Duration(num) * 5 * 8 * time.Hour // 1 рабочая неделя = 5 рабочих дней
		case "d":
			total += time.Duration(num) * 8 * time.Hour // 1 рабочий день = 8 часов
		case "h":
			total += time.Duration(num) * time.Hour
		case "m":
			total += time.Duration(num) * time.Minute
		default:
			return 0, fmt.Errorf("неизвестная единица измерения: %s", unit)
		}
	}
	return total, nil
}

func dateIdFromTime(t time.Time) int {
	return t.Year()*10000 + int(t.Month())*100 + t.Day()
}
