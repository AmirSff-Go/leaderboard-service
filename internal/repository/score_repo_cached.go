package repository

import (
	"context"
	"fmt"

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
		fmt.Printf("Failed to update Redis cache: %v\n", err)
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
		fmt.Printf("Failed to get rank from Redis cache: %v\n", err)
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
		fmt.Printf("Failed to get ranking from Redis cache: %v\n", err)
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
		fmt.Printf("Failed to get count from Redis cache: %v\n", err)
		return r.postgres.CountByLeaderboard(ctx, leaderboardID, durationIndex)
	}
	if count == 0 {
		return r.postgres.CountByLeaderboard(ctx, leaderboardID, durationIndex)
	}
	return int(count), nil
}
