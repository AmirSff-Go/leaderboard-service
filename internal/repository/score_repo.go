package repository

import (
	"context"
	"time"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/google/uuid"
)

// ttl is how long a caching implementation should keep the (leaderboardID, durationIndex) bucket
// cached, per domain.BucketCacheTTL — 0 means never expire. A non-caching implementation is free
// to ignore it.
type ScoreRepo interface {
	Upsert(ctx context.Context, score *domain.Score, ttl time.Duration) error

	GetByLeaderboardAndUser(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int) (*domain.Score, error)

	GetRanking(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, ttl time.Duration, page, pageSize int) ([]*domain.Score, error)

	CountByLeaderboard(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, ttl time.Duration) (int, error)

	GetUserRank(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, ttl time.Duration, score int) (int, error)

	// SubmitScoreAtomic runs decide with the current score for (leaderboardID, userID, durationIndex)
	// and, if it reports shouldSave, persists finalScore — all serialized against concurrent callers
	// for that same tuple. current is nil when no score exists yet.
	SubmitScoreAtomic(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int, ttl time.Duration,
		decide func(current *domain.Score) (shouldSave bool, finalScore int, err error)) error

	DeleteScore(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int) error

	// DeleteLeaderboardCache clears every cache entry belonging to a leaderboard, across all of
	// its period buckets. A non-caching implementation is a no-op.
	DeleteLeaderboardCache(ctx context.Context, leaderboardID uuid.UUID) error
}
