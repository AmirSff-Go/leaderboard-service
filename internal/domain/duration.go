package domain

import "time"

func ComputeDurationIndex(t time.Time, intervalSeconds int) int {
	if intervalSeconds <= 0 {
		return 0
	}
	return int(t.UTC().Unix()) / intervalSeconds
}

func CurrentDurationIndex(lb *Leaderboard) int {
	return ComputeDurationIndex(time.Now(), lb.IntervalSeconds)
}
