//go:build integration

package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/AmirSff-Go/leaderboard-service/internal/repository"
)

func TestPostgresScoreRepo_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	lb := seedGameAndLeaderboard(t, domain.Record, 0)
	repo := repository.NewPostgresScoreRepo(testDB)

	_, err := repo.GetByLeaderboardAndUser(ctx, lb.ID, "user1", 0)
	assert.ErrorIs(t, err, domain.ErrScoreNotFound)

	require.NoError(t, repo.Upsert(ctx, &domain.Score{
		LeaderboardID: lb.ID,
		UserID:        "user1",
		Score:         100,
		DurationIndex: 0,
	}, 0))

	score, err := repo.GetByLeaderboardAndUser(ctx, lb.ID, "user1", 0)
	require.NoError(t, err)
	assert.Equal(t, 100, score.Score)

	// Upsert again — ON CONFLICT DO UPDATE should replace the score, not duplicate the row.
	require.NoError(t, repo.Upsert(ctx, &domain.Score{
		LeaderboardID: lb.ID,
		UserID:        "user1",
		Score:         200,
		DurationIndex: 0,
	}, 0))
	score, err = repo.GetByLeaderboardAndUser(ctx, lb.ID, "user1", 0)
	require.NoError(t, err)
	assert.Equal(t, 200, score.Score)

	count, err := repo.CountByLeaderboard(ctx, lb.ID, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "upsert on conflict must not create a duplicate row")
}

func TestPostgresScoreRepo_GetRankingAndListAll(t *testing.T) {
	ctx := context.Background()
	lb := seedGameAndLeaderboard(t, domain.Record, 0)
	repo := repository.NewPostgresScoreRepo(testDB)

	scores := map[string]int{"alice": 300, "bob": 100, "carol": 200}
	for user, score := range scores {
		require.NoError(t, repo.Upsert(ctx, &domain.Score{
			LeaderboardID: lb.ID,
			UserID:        user,
			Score:         score,
			DurationIndex: 0,
		}, 0))
	}

	ranking, err := repo.GetRanking(ctx, lb.ID, 0, 0, 1, 10)
	require.NoError(t, err)
	require.Len(t, ranking, 3)
	assert.Equal(t, "alice", ranking[0].UserID)
	assert.Equal(t, "carol", ranking[1].UserID)
	assert.Equal(t, "bob", ranking[2].UserID)

	page1, err := repo.GetRanking(ctx, lb.ID, 0, 0, 1, 2)
	require.NoError(t, err)
	assert.Len(t, page1, 2)
	page2, err := repo.GetRanking(ctx, lb.ID, 0, 0, 2, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 1)

	rank, err := repo.GetUserRank(ctx, lb.ID, 0, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, 3, rank, "bob (100) is behind alice (300) and carol (200)")

	all, err := repo.ListAllByLeaderboard(ctx, lb.ID, 0)
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, 300, all[0].Score)
	assert.Equal(t, 100, all[2].Score)
}

// TestPostgresScoreRepo_SubmitScoreAtomic_ConcurrentAdditive proves the pg_advisory_xact_lock fix
// against a real Postgres instance: 100 concurrent +1 submissions for the same user/leaderboard/
// period must sum to exactly 100, with no lost updates. This is the scenario unit tests against
// in-memory fakes can't actually verify — it needs real concurrent transactions/connections.
func TestPostgresScoreRepo_SubmitScoreAtomic_ConcurrentAdditive(t *testing.T) {
	ctx := context.Background()
	lb := seedGameAndLeaderboard(t, domain.Additive, 0)
	repo := repository.NewPostgresScoreRepo(testDB)

	const submissions = 100
	var wg sync.WaitGroup
	wg.Add(submissions)
	for i := 0; i < submissions; i++ {
		go func() {
			defer wg.Done()
			err := repo.SubmitScoreAtomic(ctx, lb.ID, "user1", 0, 0, func(current *domain.Score) (bool, int, error) {
				base := 0
				if current != nil {
					base = current.Score
				}
				return true, base + 1, nil
			})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	score, err := repo.GetByLeaderboardAndUser(ctx, lb.ID, "user1", 0)
	require.NoError(t, err)
	assert.Equal(t, submissions, score.Score, "every +1 submission must be reflected — no lost updates")
}

// TestPostgresScoreRepo_SubmitScoreAtomic_ConcurrentRecord proves concurrent Record-type
// submissions converge to the true maximum against real Postgres, regardless of transaction
// commit order — the original bug allowed a lower score to overwrite a higher one under races.
func TestPostgresScoreRepo_SubmitScoreAtomic_ConcurrentRecord(t *testing.T) {
	ctx := context.Background()
	lb := seedGameAndLeaderboard(t, domain.Record, 0)
	repo := repository.NewPostgresScoreRepo(testDB)

	const submissions = 100
	var wg sync.WaitGroup
	wg.Add(submissions)
	for i := 1; i <= submissions; i++ {
		newScore := i
		go func() {
			defer wg.Done()
			err := repo.SubmitScoreAtomic(ctx, lb.ID, "user1", 0, 0, func(current *domain.Score) (bool, int, error) {
				if current == nil || newScore > current.Score {
					return true, newScore, nil
				}
				return false, 0, nil
			})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	score, err := repo.GetByLeaderboardAndUser(ctx, lb.ID, "user1", 0)
	require.NoError(t, err)
	assert.Equal(t, submissions, score.Score, "the highest submitted score must win regardless of commit order")
}

func TestPostgresScoreRepo_SubmitScoreAtomic_NoOpDoesNotPersist(t *testing.T) {
	ctx := context.Background()
	lb := seedGameAndLeaderboard(t, domain.Record, 0)
	repo := repository.NewPostgresScoreRepo(testDB)

	err := repo.SubmitScoreAtomic(ctx, lb.ID, "user1", 0, 0, func(current *domain.Score) (bool, int, error) {
		return false, 0, nil
	})
	require.NoError(t, err)

	_, err = repo.GetByLeaderboardAndUser(ctx, lb.ID, "user1", 0)
	assert.True(t, errors.Is(err, domain.ErrScoreNotFound), "a shouldSave=false decision must not create a row")
}
