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

// BucketCacheGracePeriod is how long a period bucket's Redis cache entry outlives the period
// itself before it's allowed to expire. Postgres is the source of truth and can always rehydrate
// a historical bucket on demand, so this only trades a slightly colder read for bounded memory.
const BucketCacheGracePeriod = 24 * time.Hour

// BucketCacheTTL returns how long the Redis cache entry for (intervalSeconds, durationIndex)
// should live from now, or 0 to mean "never expire". All-time leaderboards (intervalSeconds <= 0)
// have exactly one bucket that's permanently current, so they're never expired: without this,
// every leaderboard would leave two permanent Redis keys behind forever (the sorted set and its
// synced marker), and a consumer product with tens of thousands of boards never reclaims that
// memory. For a periodic leaderboard, the TTL is set to the bucket's period end plus a grace
// window: while the period is still active this simply refreshes to a large value on every write,
// and once the period ends and nothing touches the bucket again, it expires on its own shortly
// after — including buckets nobody ever reads again, which a TTL set only at read time would miss.
func BucketCacheTTL(intervalSeconds, durationIndex int, now time.Time) time.Duration {
	if intervalSeconds <= 0 {
		return 0
	}
	periodEnd := time.Unix(int64(durationIndex+1)*int64(intervalSeconds), 0)
	ttl := periodEnd.Sub(now) + BucketCacheGracePeriod
	if ttl < BucketCacheGracePeriod {
		ttl = BucketCacheGracePeriod
	}
	return ttl
}
