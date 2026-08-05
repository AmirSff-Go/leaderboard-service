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

	warm, err := r.isWarm(ctx, score.LeaderboardID, score.DurationIndex)
	if err != nil {
		slog.Error("Failed to check Redis cache warm state", "error", err)
		return nil
	}
	if !warm {
		// See SubmitScoreAtomic: writing into a cold set would recreate the partial-view bug.
		return nil
	}

	key := cache.LeaderboardKey(score.LeaderboardID, score.DurationIndex)
	if _, err := r.redis.ZAdd(ctx, key, redis.Z{
		Score:  float64(score.Score),
		Member: score.UserID,
	}).Result(); err != nil {
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
		warm, err := r.isWarm(ctx, leaderboardID, durationIndex)
		if err != nil {
			slog.Error("Failed to check Redis cache warm state", "error", err)
		} else if warm {
			// Only safe to apply an incremental update when the set already mirrors Postgres in
			// full — ZAdd-ing into a cold/never-warmed set is exactly how the cache used to end up
			// holding a partial view that later reads trusted as complete. If it's not warm yet,
			// leave it alone; the next read triggers a full warmCache that already includes this
			// write (it landed in Postgres above).
			key := cache.LeaderboardKey(leaderboardID, durationIndex)
			if _, err := r.redis.ZAdd(ctx, key, redis.Z{
				Score:  float64(finalScore),
				Member: userID,
			}).Result(); err != nil {
				slog.Error("Failed to update Redis cache", "error", err)
			}
		}
	}

	return nil
}

// isWarm reports whether the Redis sorted set for (leaderboardID, durationIndex) is a complete
// mirror of Postgres. A missing marker means either it was never warmed or Redis was flushed/
// restarted — in both cases the sorted set (even if non-empty from a prior partial write) can't
// be trusted.
func (r *CachedScoreRepo) isWarm(ctx context.Context, leaderboardID uuid.UUID, durationIndex int) (bool, error) {
	n, err := r.redis.Exists(ctx, cache.LeaderboardSyncedKey(leaderboardID, durationIndex)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// warmCache fully hydrates the sorted set for (leaderboardID, durationIndex) from Postgres and
// marks it synced. It replaces whatever the key currently holds, so a previously-poisoned partial
// set is corrected rather than merged with.
func (r *CachedScoreRepo) warmCache(ctx context.Context, leaderboardID uuid.UUID, durationIndex int) ([]*domain.Score, error) {
	all, err := r.postgres.ListAllByLeaderboard(ctx, leaderboardID, durationIndex)
	if err != nil {
		return nil, err
	}

	key := cache.LeaderboardKey(leaderboardID, durationIndex)
	syncedKey := cache.LeaderboardSyncedKey(leaderboardID, durationIndex)

	// TxPipelined (MULTI/EXEC) rather than a plain Pipeline: the whole replace-and-mark-synced
	// sequence must run as one indivisible unit. A plain pipeline only batches the network
	// round trip — Redis can still interleave another client's commands between them, which
	// could otherwise leave the synced marker set while a concurrent warm's Del/ZAdd is
	// mid-flight against the same key.
	_, err = r.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Del(ctx, key)
		for _, s := range all {
			pipe.ZAdd(ctx, key, redis.Z{Score: float64(s.Score), Member: s.UserID})
		}
		pipe.Set(ctx, syncedKey, "1", 0)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return all, nil
}

func paginateScores(scores []*domain.Score, page, pageSize int) []*domain.Score {
	start := (page - 1) * pageSize
	if start < 0 || start >= len(scores) {
		return []*domain.Score{}
	}
	end := start + pageSize
	if end > len(scores) {
		end = len(scores)
	}
	return scores[start:end]
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
	warm, err := r.isWarm(ctx, leaderboardID, durationIndex)
	if err != nil {
		slog.Error("Failed to check Redis cache warm state", "error", err)
		return r.postgres.GetUserRank(ctx, leaderboardID, durationIndex, score)
	}
	if !warm {
		// Rank must reflect the whole leaderboard; a cold cache can't answer this correctly, and
		// GetRanking (called first in the GetRankings flow) will normally have already warmed it.
		return r.postgres.GetUserRank(ctx, leaderboardID, durationIndex, score)
	}

	key := cache.LeaderboardKey(leaderboardID, durationIndex)
	count, err := r.redis.ZCount(ctx, key, fmt.Sprintf("(%d", score), "+inf").Result()
	if err != nil {
		slog.Error("Failed to get rank from Redis cache", "error", err)
		return r.postgres.GetUserRank(ctx, leaderboardID, durationIndex, score)
	}
	return int(count) + 1, nil
}

func (r *CachedScoreRepo) GetRanking(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, page,
	pageSize int) ([]*domain.Score, error) {
	warm, err := r.isWarm(ctx, leaderboardID, durationIndex)
	if err != nil {
		slog.Error("Failed to check Redis cache warm state", "error", err)
		return r.postgres.GetRanking(ctx, leaderboardID, durationIndex, page, pageSize)
	}

	if !warm {
		all, err := r.warmCache(ctx, leaderboardID, durationIndex)
		if err != nil {
			slog.Error("Failed to warm Redis cache, falling back to Postgres", "error", err)
			return r.postgres.GetRanking(ctx, leaderboardID, durationIndex, page, pageSize)
		}
		return paginateScores(all, page, pageSize), nil
	}

	key := cache.LeaderboardKey(leaderboardID, durationIndex)
	start := int64((page - 1) * pageSize)
	stop := start + int64(pageSize) - 1
	results, err := r.redis.ZRevRangeWithScores(ctx, key, start, stop).Result()
	if err != nil {
		slog.Error("Failed to get ranking from Redis cache", "error", err)
		return r.postgres.GetRanking(ctx, leaderboardID, durationIndex, page, pageSize)
	}

	scores := make([]*domain.Score, 0, len(results))
	for _, res := range results {
		scores = append(scores, &domain.Score{
			UserID: res.Member.(string),
			Score:  int(res.Score),
		})
	}
	return scores, nil
}

func (r *CachedScoreRepo) CountByLeaderboard(ctx context.Context, leaderboardID uuid.UUID, durationIndex int) (int, error) {
	warm, err := r.isWarm(ctx, leaderboardID, durationIndex)
	if err != nil {
		slog.Error("Failed to check Redis cache warm state", "error", err)
		return r.postgres.CountByLeaderboard(ctx, leaderboardID, durationIndex)
	}

	if !warm {
		all, err := r.warmCache(ctx, leaderboardID, durationIndex)
		if err != nil {
			slog.Error("Failed to warm Redis cache, falling back to Postgres", "error", err)
			return r.postgres.CountByLeaderboard(ctx, leaderboardID, durationIndex)
		}
		return len(all), nil
	}

	key := cache.LeaderboardKey(leaderboardID, durationIndex)
	count, err := r.redis.ZCard(ctx, key).Result()
	if err != nil {
		slog.Error("Failed to get count from Redis cache", "error", err)
		return r.postgres.CountByLeaderboard(ctx, leaderboardID, durationIndex)
	}
	return int(count), nil
}
