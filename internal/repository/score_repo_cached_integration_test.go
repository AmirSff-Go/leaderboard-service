//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AmirSff-Go/leaderboard-service/internal/cache"
	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/AmirSff-Go/leaderboard-service/internal/repository"
)

// newCachedRepo returns a CachedScoreRepo wired to the shared test Postgres/Redis, with Redis
// flushed first so each test starts from a cold cache.
func newCachedRepo(t *testing.T) *repository.CachedScoreRepo {
	t.Helper()
	flushRedis(t)
	postgres := repository.NewPostgresScoreRepo(testDB)
	return repository.NewCachedScoreRepo(postgres, testRedisClient)
}

// TestCachedScoreRepo_ColdWriteDoesNotPoisonCache reproduces the original bug scenario directly:
// a write to a leaderboard whose cache was never warmed must NOT create a partial sorted set that
// a later read would mistake for the complete leaderboard.
func TestCachedScoreRepo_ColdWriteDoesNotPoisonCache(t *testing.T) {
	ctx := context.Background()
	lb := seedGameAndLeaderboard(t, domain.Record, 0)
	repo := newCachedRepo(t)

	for i := 1; i <= 20; i++ {
		user := "user" + string(rune('a'+i-1))
		err := repo.SubmitScoreAtomic(ctx, lb.ID, user, 0, func(current *domain.Score) (bool, int, error) {
			return true, i * 10, nil
		})
		require.NoError(t, err)
	}

	key := cache.LeaderboardKey(lb.ID, 0)
	exists, err := testRedisClient.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "writes to a never-warmed leaderboard must not create a partial sorted set")

	syncedKey := cache.LeaderboardSyncedKey(lb.ID, 0)
	syncedExists, err := testRedisClient.Exists(ctx, syncedKey).Result()
	require.NoError(t, err)
	assert.Zero(t, syncedExists)
}

// TestCachedScoreRepo_FirstReadFullyWarmsCache verifies the read path fully hydrates the sorted
// set from Postgres (not just the requested page) and marks it synced.
func TestCachedScoreRepo_FirstReadFullyWarmsCache(t *testing.T) {
	ctx := context.Background()
	lb := seedGameAndLeaderboard(t, domain.Record, 0)
	repo := newCachedRepo(t)

	for i := 1; i <= 20; i++ {
		user := "user" + string(rune('a'+i-1))
		require.NoError(t, repo.SubmitScoreAtomic(ctx, lb.ID, user, 0, func(current *domain.Score) (bool, int, error) {
			return true, i * 10, nil
		}))
	}

	// Ask for a small page — a page-scoped warm-up (the old bug) would only cache this slice.
	ranking, err := repo.GetRanking(ctx, lb.ID, 0, 1, 5)
	require.NoError(t, err)
	require.Len(t, ranking, 5)
	assert.Equal(t, 200, ranking[0].Score)

	key := cache.LeaderboardKey(lb.ID, 0)
	card, err := testRedisClient.ZCard(ctx, key).Result()
	require.NoError(t, err)
	assert.EqualValues(t, 20, card, "the full leaderboard must be hydrated, not just the requested page")

	total, err := repo.CountByLeaderboard(ctx, lb.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, 20, total)
}

// TestCachedScoreRepo_IncrementalWriteAfterWarm verifies a write against an already-warm cache
// applies incrementally and the result stays consistent with Postgres.
func TestCachedScoreRepo_IncrementalWriteAfterWarm(t *testing.T) {
	ctx := context.Background()
	lb := seedGameAndLeaderboard(t, domain.Record, 0)
	repo := newCachedRepo(t)

	require.NoError(t, repo.SubmitScoreAtomic(ctx, lb.ID, "user1", 0, func(current *domain.Score) (bool, int, error) {
		return true, 100, nil
	}))
	_, err := repo.GetRanking(ctx, lb.ID, 0, 1, 10) // warms the cache
	require.NoError(t, err)

	require.NoError(t, repo.SubmitScoreAtomic(ctx, lb.ID, "user2", 0, func(current *domain.Score) (bool, int, error) {
		return true, 250, nil
	}))

	key := cache.LeaderboardKey(lb.ID, 0)
	card, err := testRedisClient.ZCard(ctx, key).Result()
	require.NoError(t, err)
	assert.EqualValues(t, 2, card)

	total, err := repo.CountByLeaderboard(ctx, lb.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)

	rank, err := repo.GetUserRank(ctx, lb.ID, 0, 250)
	require.NoError(t, err)
	assert.Equal(t, 1, rank)
}

// TestCachedScoreRepo_SelfHealsPoisonedCache simulates leftover state from before the fix — a
// partial sorted set with no synced marker — and verifies the next read discards it and rebuilds
// the correct, complete view from Postgres rather than trusting the stale data.
func TestCachedScoreRepo_SelfHealsPoisonedCache(t *testing.T) {
	ctx := context.Background()
	lb := seedGameAndLeaderboard(t, domain.Additive, 0)
	repo := newCachedRepo(t)

	for i := 1; i <= 10; i++ {
		user := "user" + string(rune('a'+i-1))
		require.NoError(t, repo.SubmitScoreAtomic(ctx, lb.ID, user, 0, func(current *domain.Score) (bool, int, error) {
			return true, i * 5, nil
		}))
	}

	key := cache.LeaderboardKey(lb.ID, 0)
	require.NoError(t, testRedisClient.ZAdd(ctx, key, redis.Z{Score: 99999, Member: "ghost-user"}).Err())

	syncedKey := cache.LeaderboardSyncedKey(lb.ID, 0)
	syncedExists, err := testRedisClient.Exists(ctx, syncedKey).Result()
	require.NoError(t, err)
	require.Zero(t, syncedExists, "precondition: poisoned state must not be marked synced")

	ranking, err := repo.GetRanking(ctx, lb.ID, 0, 1, 20)
	require.NoError(t, err)
	require.Len(t, ranking, 10, "poisoned entry must be discarded, real 10 entries restored")
	for _, s := range ranking {
		assert.NotEqual(t, "ghost-user", s.UserID)
	}

	score, err := testRedisClient.ZScore(ctx, key, "ghost-user").Result()
	assert.ErrorIs(t, err, redis.Nil, "ghost-user must be gone after self-heal")
	assert.Zero(t, score)

	total, err := repo.CountByLeaderboard(ctx, lb.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, 10, total)
}
