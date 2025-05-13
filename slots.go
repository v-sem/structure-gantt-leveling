package main

import (
	"errors"
	"time"
)

type Slots []time.Duration

func NewSlots(slots int, delay time.Duration) Slots {
	s := make(Slots, slots)
	for i := range s {
		s[i] = delay
	}
	return s
}

func (s Slots) GetLevelingDelayAndAdd(d time.Duration) (time.Duration, error) {
	slot, err := s.FindSlot()
	if err != nil {
		return 0, err
	}
	delay := s[slot]
	s[slot] += d
	return delay, nil
}

func (s Slots) FindSlot() (int, error) {
	if len(s) == 0 {
		return 0, errors.New("no slots")
	}
	i, d := 0, s[0]
	for n, v := range s {
		if d > v {
			i, d = n, v
		}
	}
	return i, nil
}

func (s Slots) SetDelay(slot int, d time.Duration) {
	s[slot] = d
}
