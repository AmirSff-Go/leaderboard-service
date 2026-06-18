package cache

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func NewRedisClient(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	opts.PoolSize = 10    // connections per instance
	opts.MinIdleConns = 5 // keep 5 warm at all times
	opts.ConnMaxLifetime = 5 * time.Minute
	return redis.NewClient(opts), nil
}

func LeaderboardKey(leaderboardID uuid.UUID, durationIndex int) string {
	return fmt.Sprintf("lb:%s:%d", leaderboardID.String(), durationIndex)
}
