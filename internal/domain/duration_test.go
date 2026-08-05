package domain_test

import (
	"testing"
	"time"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestComputeDurationIndex(t *testing.T) {
	tests := []struct {
		name     string
		unix     int64
		interval int
		want     int
	}{
		{
			name:     "zero interval returns 0 (all-time)",
			unix:     1_000_000,
			interval: 0,
			want:     0,
		},
		{
			name:     "negative interval returns 0",
			unix:     1_000_000,
			interval: -100,
			want:     0,
		},
		{
			name:     "known division: 3700 / 3600 = 1",
			unix:     3700,
			interval: 3600,
			want:     1,
		},
		{
			name:     "exact boundary starts a new bucket",
			unix:     7200,
			interval: 3600,
			want:     2,
		},
		{
			name:     "one second before boundary stays in current bucket",
			unix:     7199,
			interval: 3600,
			want:     1,
		},
		{
			name:     "weekly interval: exactly one week from epoch",
			unix:     604_800,
			interval: 604_800,
			want:     1,
		},
		{
			name:     "interval of 1 returns unix timestamp itself",
			unix:     42,
			interval: 1,
			want:     42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.ComputeDurationIndex(time.Unix(tt.unix, 0).UTC(), tt.interval)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestComputeDurationIndex_SamePeriodBucketsTogether(t *testing.T) {
	interval := 3600
	// All three are within the second hour bucket (3600–7199)
	timestamps := []int64{3600, 4000, 7199}
	first := domain.ComputeDurationIndex(time.Unix(timestamps[0], 0).UTC(), interval)
	for _, unix := range timestamps[1:] {
		got := domain.ComputeDurationIndex(time.Unix(unix, 0).UTC(), interval)
		assert.Equal(t, first, got, "unix=%d should be in the same bucket as unix=%d", unix, timestamps[0])
	}
}

func TestComputeDurationIndex_CrossBoundaryGetsDifferentBucket(t *testing.T) {
	interval := 3600
	before := domain.ComputeDurationIndex(time.Unix(7199, 0).UTC(), interval)
	after := domain.ComputeDurationIndex(time.Unix(7200, 0).UTC(), interval)
	assert.NotEqual(t, before, after)
	assert.Equal(t, before+1, after)
}

func TestBucketCacheTTL(t *testing.T) {
	t.Run("all-time (interval <= 0) never expires", func(t *testing.T) {
		now := time.Unix(1_000_000, 0).UTC()
		assert.Zero(t, domain.BucketCacheTTL(0, 0, now))
		assert.Zero(t, domain.BucketCacheTTL(-100, 0, now))
	})

	t.Run("mid-period bucket gets a TTL covering the remaining period plus grace", func(t *testing.T) {
		interval := 3600
		now := time.Unix(3700, 0).UTC() // 100s into bucket 1 (3600-7199)
		ttl := domain.BucketCacheTTL(interval, 1, now)
		// bucket ends at 7200 -> 3500s remaining, plus the grace period.
		want := 3500*time.Second + domain.BucketCacheGracePeriod
		assert.Equal(t, want, ttl)
	})

	t.Run("a bucket whose period already ended clamps to exactly the grace period", func(t *testing.T) {
		interval := 3600
		now := time.Unix(100_000, 0).UTC() // long after bucket 1 (3600-7199) ended
		ttl := domain.BucketCacheTTL(interval, 1, now)
		assert.Equal(t, domain.BucketCacheGracePeriod, ttl)
	})

	t.Run("right at the period boundary is treated as just ended (clamped to grace)", func(t *testing.T) {
		interval := 3600
		now := time.Unix(7200, 0).UTC() // exactly when bucket 1 ends / bucket 2 starts
		ttl := domain.BucketCacheTTL(interval, 1, now)
		assert.Equal(t, domain.BucketCacheGracePeriod, ttl)
	})
}
