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
	opts.ConnMaxIdleTime = 2 * time.Minute // matches the Postgres pool's idle timeout
	return redis.NewClient(opts), nil
}

func LeaderboardKey(leaderboardID uuid.UUID, durationIndex int) string {
	return fmt.Sprintf("lb:%s:%d", leaderboardID.String(), durationIndex)
}

// LeaderboardSyncedKey marks whether LeaderboardKey holds a *complete* mirror of Postgres for
// that (leaderboard, duration_index) bucket. A sorted set with zero members is indistinguishable
// from one that was never warmed, so completeness can't be inferred from the sorted set alone —
// this is a separate marker set only after a full hydration from Postgres.
func LeaderboardSyncedKey(leaderboardID uuid.UUID, durationIndex int) string {
	return fmt.Sprintf("lb:%s:%d:synced", leaderboardID.String(), durationIndex)
}

// LeaderboardKeyPattern matches every key belonging to a leaderboard across all of its period
// buckets — both LeaderboardKey and LeaderboardSyncedKey forms, since ":synced" is a suffix on
// top of the same "lb:{id}:{n}" prefix. For SCAN MATCH, not direct GET/SET.
func LeaderboardKeyPattern(leaderboardID uuid.UUID) string {
	return fmt.Sprintf("lb:%s:*", leaderboardID.String())
}
