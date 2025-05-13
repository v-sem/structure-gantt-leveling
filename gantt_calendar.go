package main

import (
	"time"
)

type TimeRange struct {
	StartTimeId  int
	FinishTimeId int
}

type DaySchedule struct {
	TimeRanges []TimeRange
	Duration   time.Duration
}

type Calendar struct {
	ID         int
	Name       string
	WeekDays   []DaySchedule
	CustomDays map[int]DaySchedule
}

func (c *Calendar) GetWorkingDurationForDate(dateId int) time.Duration {
	if day, ok := c.CustomDays[dateId]; ok {
		return day.Duration
	}

	t, err := parseDateId(dateId)
	if err != nil {
		return 0
	}

	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 6
	} else {
		weekday = weekday - 1
	}

	if weekday >= 0 && weekday < len(c.WeekDays) {
		return c.WeekDays[weekday].Duration
	}

	return 0
}

func (c *Calendar) GetWorkingDurationBetween(startDateId, finishDateId int) time.Duration {
	var total time.Duration

	startDate, _ := parseDateId(startDateId)
	endDate, _ := parseDateId(finishDateId)

	for d := startDate; d.Before(endDate); d = d.AddDate(0, 0, 1) {
		dateId := dateIdFromTime(d)
		total += c.GetWorkingDurationForDate(dateId)
	}

	return total
}
