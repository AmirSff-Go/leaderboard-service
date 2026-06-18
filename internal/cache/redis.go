package cache

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func NewRedisClient(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return redis.NewClient(opts), nil
}

func LeaderboardKey(leaderboardID uuid.UUID, durationIndex int) string {
	return fmt.Sprintf("lb:%s:%d", leaderboardID.String(), durationIndex)
}
