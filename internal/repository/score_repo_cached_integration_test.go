//go:build integration

package repository_test

import (
	"context"
	"testing"
	"time"

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
		err := repo.SubmitScoreAtomic(ctx, lb.ID, user, 0, 0, func(current *domain.Score) (bool, int, error) {
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
		require.NoError(t, repo.SubmitScoreAtomic(ctx, lb.ID, user, 0, 0, func(current *domain.Score) (bool, int, error) {
			return true, i * 10, nil
		}))
	}

	// Ask for a small page — a page-scoped warm-up (the old bug) would only cache this slice.
	ranking, err := repo.GetRanking(ctx, lb.ID, 0, 0, 1, 5)
	require.NoError(t, err)
	require.Len(t, ranking, 5)
	assert.Equal(t, 200, ranking[0].Score)

	key := cache.LeaderboardKey(lb.ID, 0)
	card, err := testRedisClient.ZCard(ctx, key).Result()
	require.NoError(t, err)
	assert.EqualValues(t, 20, card, "the full leaderboard must be hydrated, not just the requested page")

	total, err := repo.CountByLeaderboard(ctx, lb.ID, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 20, total)
}

// TestCachedScoreRepo_IncrementalWriteAfterWarm verifies a write against an already-warm cache
// applies incrementally and the result stays consistent with Postgres.
func TestCachedScoreRepo_IncrementalWriteAfterWarm(t *testing.T) {
	ctx := context.Background()
	lb := seedGameAndLeaderboard(t, domain.Record, 0)
	repo := newCachedRepo(t)

	require.NoError(t, repo.SubmitScoreAtomic(ctx, lb.ID, "user1", 0, 0, func(current *domain.Score) (bool, int, error) {
		return true, 100, nil
	}))
	_, err := repo.GetRanking(ctx, lb.ID, 0, 0, 1, 10) // warms the cache
	require.NoError(t, err)

	require.NoError(t, repo.SubmitScoreAtomic(ctx, lb.ID, "user2", 0, 0, func(current *domain.Score) (bool, int, error) {
		return true, 250, nil
	}))

	key := cache.LeaderboardKey(lb.ID, 0)
	card, err := testRedisClient.ZCard(ctx, key).Result()
	require.NoError(t, err)
	assert.EqualValues(t, 2, card)

	total, err := repo.CountByLeaderboard(ctx, lb.ID, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)

	rank, err := repo.GetUserRank(ctx, lb.ID, 0, 0, 250)
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
		require.NoError(t, repo.SubmitScoreAtomic(ctx, lb.ID, user, 0, 0, func(current *domain.Score) (bool, int, error) {
			return true, i * 5, nil
		}))
	}

	key := cache.LeaderboardKey(lb.ID, 0)
	require.NoError(t, testRedisClient.ZAdd(ctx, key, redis.Z{Score: 99999, Member: "ghost-user"}).Err())

	syncedKey := cache.LeaderboardSyncedKey(lb.ID, 0)
	syncedExists, err := testRedisClient.Exists(ctx, syncedKey).Result()
	require.NoError(t, err)
	require.Zero(t, syncedExists, "precondition: poisoned state must not be marked synced")

	ranking, err := repo.GetRanking(ctx, lb.ID, 0, 0, 1, 20)
	require.NoError(t, err)
	require.Len(t, ranking, 10, "poisoned entry must be discarded, real 10 entries restored")
	for _, s := range ranking {
		assert.NotEqual(t, "ghost-user", s.UserID)
	}

	score, err := testRedisClient.ZScore(ctx, key, "ghost-user").Result()
	assert.ErrorIs(t, err, redis.Nil, "ghost-user must be gone after self-heal")
	assert.Zero(t, score)

	total, err := repo.CountByLeaderboard(ctx, lb.ID, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 10, total)
}

// TestCachedScoreRepo_TTL verifies a real Redis EXPIRE is actually set on both the sorted set and
// its synced marker — the unit tests against fakes can only prove BucketCacheTTL computes the
// right duration and that the service passes it through; only a real Redis can prove the repo
// layer turns that duration into an actual key expiry.
func TestCachedScoreRepo_TTL(t *testing.T) {
	ctx := context.Background()

	t.Run("ttl > 0 sets an expiry on warm (cold read) and on a write to an already-warm bucket", func(t *testing.T) {
		lb := seedGameAndLeaderboard(t, domain.Record, 0)
		repo := newCachedRepo(t)
		ttl := time.Hour

		require.NoError(t, repo.SubmitScoreAtomic(ctx, lb.ID, "user1", 0, ttl, func(current *domain.Score) (bool, int, error) {
			return true, 100, nil
		}))
		// First read is a cold warm — exercises warmCache's TTL branch.
		_, err := repo.GetRanking(ctx, lb.ID, 0, ttl, 1, 10)
		require.NoError(t, err)

		key := cache.LeaderboardKey(lb.ID, 0)
		syncedKey := cache.LeaderboardSyncedKey(lb.ID, 0)
		assertTTLRoughly(t, key, ttl)
		assertTTLRoughly(t, syncedKey, ttl)

		// A write against the now-warm cache goes through refreshTTL instead of warmCache — must
		// also set the expiry, not just leave whatever warmCache set.
		require.NoError(t, testRedisClient.Persist(ctx, key).Err())
		require.NoError(t, testRedisClient.Persist(ctx, syncedKey).Err())
		require.NoError(t, repo.SubmitScoreAtomic(ctx, lb.ID, "user2", 0, ttl, func(current *domain.Score) (bool, int, error) {
			return true, 200, nil
		}))
		assertTTLRoughly(t, key, ttl)
		assertTTLRoughly(t, syncedKey, ttl)
	})

	t.Run("ttl <= 0 (all-time) leaves keys without an expiry", func(t *testing.T) {
		lb := seedGameAndLeaderboard(t, domain.Record, 0) // IntervalSeconds: 0 -> BucketCacheTTL is 0
		repo := newCachedRepo(t)

		require.NoError(t, repo.SubmitScoreAtomic(ctx, lb.ID, "user1", 0, 0, func(current *domain.Score) (bool, int, error) {
			return true, 100, nil
		}))
		_, err := repo.GetRanking(ctx, lb.ID, 0, 0, 1, 10)
		require.NoError(t, err)

		key := cache.LeaderboardKey(lb.ID, 0)
		syncedKey := cache.LeaderboardSyncedKey(lb.ID, 0)
		assertNoTTL(t, key)
		assertNoTTL(t, syncedKey)
	})
}

// TestCachedScoreRepo_DeleteScore verifies deletion removes the Postgres row (source of truth for
// ErrScoreNotFound) and, when the cache is warm, the member from the cached sorted set too.
func TestCachedScoreRepo_DeleteScore(t *testing.T) {
	ctx := context.Background()

	t.Run("removes from Postgres and a warm cache", func(t *testing.T) {
		lb := seedGameAndLeaderboard(t, domain.Record, 0)
		repo := newCachedRepo(t)
		require.NoError(t, repo.SubmitScoreAtomic(ctx, lb.ID, "alice", 0, 0, func(current *domain.Score) (bool, int, error) {
			return true, 100, nil
		}))
		_, err := repo.GetRanking(ctx, lb.ID, 0, 0, 1, 10) // warms the cache
		require.NoError(t, err)

		require.NoError(t, repo.DeleteScore(ctx, lb.ID, "alice", 0))

		_, err = repo.GetByLeaderboardAndUser(ctx, lb.ID, "alice", 0)
		assert.ErrorIs(t, err, domain.ErrScoreNotFound, "Postgres row must be gone")

		key := cache.LeaderboardKey(lb.ID, 0)
		score, err := testRedisClient.ZScore(ctx, key, "alice").Result()
		assert.ErrorIs(t, err, redis.Nil, "cached member must be gone too")
		assert.Zero(t, score)
	})

	t.Run("deleting an absent score returns ErrScoreNotFound and touches nothing", func(t *testing.T) {
		lb := seedGameAndLeaderboard(t, domain.Record, 0)
		repo := newCachedRepo(t)
		err := repo.DeleteScore(ctx, lb.ID, "ghost", 0)
		assert.ErrorIs(t, err, domain.ErrScoreNotFound)
	})
}

// TestCachedScoreRepo_DeleteLeaderboardCache verifies SCAN+DEL clears every period bucket's keys
// for a leaderboard, not just the one currently warm — a leaderboard accumulates one key pair per
// period over its lifetime, and deletion needs to reclaim all of them, not just the latest.
func TestCachedScoreRepo_DeleteLeaderboardCache(t *testing.T) {
	ctx := context.Background()
	lb := seedGameAndLeaderboard(t, domain.Record, 3600) // periodic, so distinct duration indexes are meaningful
	repo := newCachedRepo(t)

	for _, durationIndex := range []int{0, 1, 2} {
		require.NoError(t, repo.SubmitScoreAtomic(ctx, lb.ID, "alice", durationIndex, time.Hour, func(current *domain.Score) (bool, int, error) {
			return true, 100, nil
		}))
		_, err := repo.GetRanking(ctx, lb.ID, durationIndex, time.Hour, 1, 10) // warm each bucket
		require.NoError(t, err)
	}
	for _, durationIndex := range []int{0, 1, 2} {
		exists, err := testRedisClient.Exists(ctx, cache.LeaderboardKey(lb.ID, durationIndex)).Result()
		require.NoError(t, err)
		require.EqualValues(t, 1, exists, "precondition: bucket %d must be warm before deletion", durationIndex)
	}

	require.NoError(t, repo.DeleteLeaderboardCache(ctx, lb.ID))

	for _, durationIndex := range []int{0, 1, 2} {
		key := cache.LeaderboardKey(lb.ID, durationIndex)
		syncedKey := cache.LeaderboardSyncedKey(lb.ID, durationIndex)
		existsKey, err := testRedisClient.Exists(ctx, key).Result()
		require.NoError(t, err)
		assert.Zero(t, existsKey, "bucket %d sorted set must be gone", durationIndex)
		existsSynced, err := testRedisClient.Exists(ctx, syncedKey).Result()
		require.NoError(t, err)
		assert.Zero(t, existsSynced, "bucket %d synced marker must be gone", durationIndex)
	}
}

func assertTTLRoughly(t *testing.T, key string, want time.Duration) {
	t.Helper()
	ttl, err := testRedisClient.TTL(context.Background(), key).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, time.Duration(0), "key %q must have an expiry set", key)
	assert.InDelta(t, want.Seconds(), ttl.Seconds(), 10, "key %q TTL should be close to the requested duration", key)
}

func assertNoTTL(t *testing.T, key string) {
	t.Helper()
	ttl, err := testRedisClient.TTL(context.Background(), key).Result()
	require.NoError(t, err)
	assert.Equal(t, time.Duration(-1), ttl, "key %q must not have an expiry", key)
}
