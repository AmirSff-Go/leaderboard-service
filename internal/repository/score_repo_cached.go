package repository

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/AmirSff-Go/leaderboard-service/internal/cache"
	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type CachedScoreRepo struct {
	postgres *PostgresScoreRepo
	redis    *redis.Client
}

func NewCachedScoreRepo(postgres *PostgresScoreRepo, redis *redis.Client) *CachedScoreRepo {
	return &CachedScoreRepo{
		postgres: postgres,
		redis:    redis,
	}
}

func (r *CachedScoreRepo) Upsert(ctx context.Context, score *domain.Score) error {
	err := r.postgres.Upsert(ctx, score)
	if err != nil {
		return err
	}

	key := cache.LeaderboardKey(score.LeaderboardID, score.DurationIndex)
	_, err = r.redis.ZAdd(ctx, key, redis.Z{
		Score:  float64(score.Score),
		Member: score.UserID,
	}).Result()
	if err != nil {
		// Log the error but don't fail the request
		slog.Error("Failed to update Redis cache", "error", err)
	}
	return nil
}

// SubmitScoreAtomic delegates the atomic read-decide-write to Postgres (source of truth), then
// syncs Redis with the confirmed outcome. Redis failure is non-fatal, matching Upsert's behavior.
func (r *CachedScoreRepo) SubmitScoreAtomic(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int,
	decide func(current *domain.Score) (bool, int, error)) error {

	var saved bool
	var finalScore int

	err := r.postgres.SubmitScoreAtomic(ctx, leaderboardID, userID, durationIndex, func(current *domain.Score) (bool, int, error) {
		shouldSave, score, err := decide(current)
		saved, finalScore = shouldSave, score
		return shouldSave, score, err
	})
	if err != nil {
		return err
	}

	if saved {
		key := cache.LeaderboardKey(leaderboardID, durationIndex)
		if _, err := r.redis.ZAdd(ctx, key, redis.Z{
			Score:  float64(finalScore),
			Member: userID,
		}).Result(); err != nil {
			// Log the error but don't fail the request
			slog.Error("Failed to update Redis cache", "error", err)
		}
	}

	return nil
}

func (r *CachedScoreRepo) GetByLeaderboardAndUser(ctx context.Context, leaderboardID uuid.UUID, userID string,
	durationIndex int) (*domain.Score, error) {
	score, err := r.postgres.GetByLeaderboardAndUser(ctx, leaderboardID, userID, durationIndex)
	if err != nil {
		return nil, err
	}
	return score, nil
}

func (r *CachedScoreRepo) GetUserRank(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, score int) (int, error) {
	key := cache.LeaderboardKey(leaderboardID, durationIndex)
	count, err := r.redis.ZCount(ctx, key, fmt.Sprintf("(%d", score), "+inf").Result()
	if err != nil {
		// Log the error and fallback to Postgres
		slog.Error("Failed to get rank from Redis cache", "error", err)
		return r.postgres.GetUserRank(ctx, leaderboardID, durationIndex, score)
	}
	return int(count) + 1, nil
}

func (r *CachedScoreRepo) GetRanking(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, page,
	pageSize int) ([]*domain.Score, error) {
	key := cache.LeaderboardKey(leaderboardID, durationIndex)
	start := int64((page - 1) * pageSize)
	stop := start + int64(pageSize) - 1
	results, err := r.redis.ZRevRangeWithScores(ctx, key, start, stop).Result()
	if err != nil {
		// Log the error and fallback to Postgres
		slog.Error("Failed to get ranking from Redis cache", "error", err)
		return r.postgres.GetRanking(ctx, leaderboardID, durationIndex, page, pageSize)
	}

	if len(results) > 0 {
		scores := make([]*domain.Score, 0, len(results))
		for _, res := range results {
			scores = append(scores, &domain.Score{
				UserID: res.Member.(string),
				Score:  int(res.Score),
			})
		}
		return scores, nil
	}
	pgScores, err := r.postgres.GetRanking(ctx, leaderboardID, durationIndex, page, pageSize)
	if err != nil {
		return nil, err
	}
	// warm Redis
	pipe := r.redis.Pipeline()
	for _, s := range pgScores {
		pipe.ZAdd(ctx, key, redis.Z{Score: float64(s.Score), Member: s.UserID})
	}
	pipe.Exec(ctx)
	return pgScores, nil
}

func (r *CachedScoreRepo) CountByLeaderboard(ctx context.Context, leaderboardID uuid.UUID, durationIndex int) (int, error) {
	key := cache.LeaderboardKey(leaderboardID, durationIndex)
	count, err := r.redis.ZCard(ctx, key).Result()
	if err != nil {
		slog.Error("Failed to get count from Redis cache", "error", err)
		return r.postgres.CountByLeaderboard(ctx, leaderboardID, durationIndex)
	}
	if count == 0 {
		return r.postgres.CountByLeaderboard(ctx, leaderboardID, durationIndex)
	}
	return int(count), nil
}
