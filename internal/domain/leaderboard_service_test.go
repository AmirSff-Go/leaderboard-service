package domain_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestService() (*domain.LeaderboardService, *fakeLeaderboardRepo, *fakeScoreRepo) {
	lbRepo := newFakeLeaderboardRepo()
	scoreRepo := newFakeScoreRepo()
	factory := domain.NewScoreProcessorFactory()
	return domain.NewLeaderboardService(lbRepo, scoreRepo, factory), lbRepo, scoreRepo
}

// seedLeaderboard creates an all-time leaderboard (IntervalSeconds=0) in the fake repo.
// IntervalSeconds=0 means durationIndex is always 0, making tests deterministic.
func seedLeaderboard(ctx context.Context, lbRepo *fakeLeaderboardRepo, gameID uuid.UUID, name string, lbType domain.LeaderboardType) *domain.Leaderboard {
	lb := &domain.Leaderboard{
		GameID:          gameID,
		UniqueName:      name,
		Type:            lbType,
		IntervalSeconds: 0,
	}
	_ = lbRepo.Create(ctx, lb)
	return lb
}

// --- CreateLeaderboard ---

func TestLeaderboardService_CreateLeaderboard(t *testing.T) {
	ctx := context.Background()
	gameID := uuid.New()

	t.Run("creates leaderboard and assigns a non-nil ID", func(t *testing.T) {
		svc, _, _ := newTestService()
		lb, err := svc.CreateLeaderboard(ctx, gameID, "season-1", "Season 1 rankings", domain.Record, 0)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, lb.ID)
		assert.Equal(t, "season-1", lb.UniqueName)
		assert.Equal(t, domain.Record, lb.Type)
	})

	t.Run("duplicate name in same game returns ErrDuplicateLeaderboardName", func(t *testing.T) {
		svc, _, _ := newTestService()
		_, err := svc.CreateLeaderboard(ctx, gameID, "season-1", "", domain.Record, 0)
		require.NoError(t, err)
		_, err = svc.CreateLeaderboard(ctx, gameID, "season-1", "", domain.Additive, 0)
		assert.Equal(t, domain.ErrDuplicateLeaderboardName, err)
	})

	t.Run("same name in different games is allowed", func(t *testing.T) {
		svc, _, _ := newTestService()
		_, err := svc.CreateLeaderboard(ctx, gameID, "global", "", domain.Record, 0)
		require.NoError(t, err)
		_, err = svc.CreateLeaderboard(ctx, uuid.New(), "global", "", domain.Record, 0)
		assert.NoError(t, err)
	})
}

// --- SubmitScore ---

func TestLeaderboardService_SubmitScore(t *testing.T) {
	ctx := context.Background()
	gameID := uuid.New()

	t.Run("leaderboard not found returns ErrLeaderboardNotFound", func(t *testing.T) {
		svc, _, _ := newTestService()
		err := svc.SubmitScore(ctx, gameID, "missing", "user1", 100)
		assert.Equal(t, domain.ErrLeaderboardNotFound, err)
	})

	t.Run("record: first submission is saved", func(t *testing.T) {
		svc, lbRepo, scoreRepo := newTestService()
		lb := seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user1", 100))
		s, err := scoreRepo.GetByLeaderboardAndUser(ctx, lb.ID, "user1", 0)
		require.NoError(t, err)
		assert.Equal(t, 100, s.Score)
	})

	t.Run("record: higher score replaces existing", func(t *testing.T) {
		svc, lbRepo, scoreRepo := newTestService()
		lb := seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user1", 100))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user1", 200))
		s, _ := scoreRepo.GetByLeaderboardAndUser(ctx, lb.ID, "user1", 0)
		assert.Equal(t, 200, s.Score)
	})

	t.Run("record: lower score is ignored", func(t *testing.T) {
		svc, lbRepo, scoreRepo := newTestService()
		lb := seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user1", 100))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user1", 50))
		s, _ := scoreRepo.GetByLeaderboardAndUser(ctx, lb.ID, "user1", 0)
		assert.Equal(t, 100, s.Score)
	})

	t.Run("additive: scores accumulate across submissions", func(t *testing.T) {
		svc, lbRepo, scoreRepo := newTestService()
		lb := seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Additive)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user1", 100))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user1", 50))
		s, _ := scoreRepo.GetByLeaderboardAndUser(ctx, lb.ID, "user1", 0)
		assert.Equal(t, 150, s.Score)
	})

	t.Run("onetime: second submission is ignored regardless of score", func(t *testing.T) {
		svc, lbRepo, scoreRepo := newTestService()
		lb := seedLeaderboard(ctx, lbRepo, gameID, "test", domain.OneTime)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user1", 100))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user1", 999))
		s, _ := scoreRepo.GetByLeaderboardAndUser(ctx, lb.ID, "user1", 0)
		assert.Equal(t, 100, s.Score)
	})

	t.Run("multiple users submit to same leaderboard independently", func(t *testing.T) {
		svc, lbRepo, scoreRepo := newTestService()
		lb := seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "alice", 100))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "bob", 200))
		alice, _ := scoreRepo.GetByLeaderboardAndUser(ctx, lb.ID, "alice", 0)
		bob, _ := scoreRepo.GetByLeaderboardAndUser(ctx, lb.ID, "bob", 0)
		assert.Equal(t, 100, alice.Score)
		assert.Equal(t, 200, bob.Score)
	})

	// Regression coverage for the read-decide-write race: concurrent submissions for the same
	// user/leaderboard/period used to be able to read the same stale "current" score and clobber
	// or drop each other's write. SubmitScoreAtomic must serialize these so no update is lost.
	t.Run("concurrent additive submissions from the same user do not lose updates", func(t *testing.T) {
		svc, lbRepo, scoreRepo := newTestService()
		lb := seedLeaderboard(ctx, lbRepo, gameID, "concurrent-additive", domain.Additive)

		const submissions = 200
		var wg sync.WaitGroup
		wg.Add(submissions)
		for i := 0; i < submissions; i++ {
			go func() {
				defer wg.Done()
				assert.NoError(t, svc.SubmitScore(ctx, gameID, "concurrent-additive", "user1", 1))
			}()
		}
		wg.Wait()

		s, err := scoreRepo.GetByLeaderboardAndUser(ctx, lb.ID, "user1", 0)
		require.NoError(t, err)
		assert.Equal(t, submissions, s.Score, "every +1 submission must be reflected in the total")
	})

	t.Run("concurrent record submissions from the same user converge to the max score", func(t *testing.T) {
		svc, lbRepo, scoreRepo := newTestService()
		lb := seedLeaderboard(ctx, lbRepo, gameID, "concurrent-record", domain.Record)

		scores := make([]int, 200)
		for i := range scores {
			scores[i] = i + 1
		}
		var wg sync.WaitGroup
		wg.Add(len(scores))
		for _, sc := range scores {
			sc := sc
			go func() {
				defer wg.Done()
				assert.NoError(t, svc.SubmitScore(ctx, gameID, "concurrent-record", "user1", sc))
			}()
		}
		wg.Wait()

		s, err := scoreRepo.GetByLeaderboardAndUser(ctx, lb.ID, "user1", 0)
		require.NoError(t, err)
		assert.Equal(t, len(scores), s.Score, "the highest submitted score must win regardless of write order")
	})
}

// --- Cache TTL wiring ---
// SubmitScore and GetRankings must pass BucketCacheTTL's result through to the repository, not a
// hardcoded 0 — otherwise the cache TTL fix is dead code no matter how correct BucketCacheTTL
// itself is. fakeScoreRepo.lastTTL records whatever ttl the service actually passed in.

func TestLeaderboardService_PassesBucketCacheTTLToRepository(t *testing.T) {
	ctx := context.Background()
	gameID := uuid.New()

	t.Run("all-time leaderboard passes ttl=0 (never expire)", func(t *testing.T) {
		svc, lbRepo, scoreRepo := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "all-time", domain.Record) // IntervalSeconds: 0

		require.NoError(t, svc.SubmitScore(ctx, gameID, "all-time", "user1", 100))
		assert.Zero(t, scoreRepo.lastTTL)

		_, _, _, err := svc.GetRankings(ctx, gameID, "all-time", 1, 20, "", 0)
		require.NoError(t, err)
		assert.Zero(t, scoreRepo.lastTTL)
	})

	t.Run("periodic leaderboard passes a positive ttl", func(t *testing.T) {
		svc, lbRepo, scoreRepo := newTestService()
		lb := &domain.Leaderboard{GameID: gameID, UniqueName: "daily", Type: domain.Record, IntervalSeconds: 86400}
		require.NoError(t, lbRepo.Create(ctx, lb))

		require.NoError(t, svc.SubmitScore(ctx, gameID, "daily", "user1", 100))
		assert.Positive(t, scoreRepo.lastTTL)
		assert.GreaterOrEqual(t, scoreRepo.lastTTL, domain.BucketCacheGracePeriod)

		_, _, _, err := svc.GetRankings(ctx, gameID, "daily", 1, 20, "", 0)
		require.NoError(t, err)
		assert.Positive(t, scoreRepo.lastTTL)
	})
}

// --- GetRankings ---

func TestLeaderboardService_GetRankings(t *testing.T) {
	ctx := context.Background()
	gameID := uuid.New()

	t.Run("leaderboard not found returns ErrLeaderboardNotFound", func(t *testing.T) {
		svc, _, _ := newTestService()
		_, _, _, err := svc.GetRankings(ctx, gameID, "missing", 1, 20, "", 0)
		assert.Equal(t, domain.ErrLeaderboardNotFound, err)
	})

	t.Run("empty leaderboard returns empty rankings and zero total", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		rankings, total, userEntry, err := svc.GetRankings(ctx, gameID, "test", 1, 20, "", 0)
		require.NoError(t, err)
		assert.Empty(t, rankings)
		assert.Equal(t, 0, total)
		assert.Nil(t, userEntry)
	})

	t.Run("rankings returned in descending score order", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user1", 100))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user2", 300))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user3", 200))

		rankings, total, _, err := svc.GetRankings(ctx, gameID, "test", 1, 20, "", 0)
		require.NoError(t, err)
		assert.Equal(t, 3, total)
		require.Len(t, rankings, 3)
		assert.Equal(t, 300, rankings[0].Score)
		assert.Equal(t, 200, rankings[1].Score)
		assert.Equal(t, 100, rankings[2].Score)
	})

	t.Run("rank field reflects position in the list", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user1", 100))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user2", 300))

		rankings, _, _, err := svc.GetRankings(ctx, gameID, "test", 1, 20, "", 0)
		require.NoError(t, err)
		assert.Equal(t, 1, rankings[0].Rank)
		assert.Equal(t, 2, rankings[1].Rank)
	})

	t.Run("user entry included when user has a score", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "alice", 100))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "bob", 300))

		_, _, userEntry, err := svc.GetRankings(ctx, gameID, "test", 1, 20, "alice", 0)
		require.NoError(t, err)
		require.NotNil(t, userEntry)
		assert.Equal(t, "alice", userEntry.UserID)
		assert.Equal(t, 100, userEntry.Score)
		assert.Equal(t, 2, userEntry.Rank) // bob has higher score
	})

	t.Run("user entry is nil when user has no score", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "alice", 100))

		_, _, userEntry, err := svc.GetRankings(ctx, gameID, "test", 1, 20, "ghost", 0)
		require.NoError(t, err)
		assert.Nil(t, userEntry)
	})

	t.Run("no user_id provided results in nil user entry", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "alice", 100))

		_, _, userEntry, err := svc.GetRankings(ctx, gameID, "test", 1, 20, "", 0)
		require.NoError(t, err)
		assert.Nil(t, userEntry)
	})

	t.Run("pagination returns the correct page slice", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		// Submit scores 10, 20, 30, 40, 50 → sorted desc: 50, 40, 30, 20, 10
		for i := 1; i <= 5; i++ {
			require.NoError(t, svc.SubmitScore(ctx, gameID, "test", fmt.Sprintf("user%d", i), i*10))
		}

		// page 2, page_size 2 → ranks 3 and 4 (scores 30 and 20)
		rankings, total, _, err := svc.GetRankings(ctx, gameID, "test", 2, 2, "", 0)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		require.Len(t, rankings, 2)
		assert.Equal(t, 3, rankings[0].Rank)
		assert.Equal(t, 4, rankings[1].Rank)
		assert.Equal(t, 30, rankings[0].Score)
		assert.Equal(t, 20, rankings[1].Score)
	})

	t.Run("tied scores share the same rank, matching GetUserRank's semantic", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		// alice and bob tie for first; carol is strictly behind.
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "alice", 100))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "bob", 100))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "carol", 50))

		rankings, _, _, err := svc.GetRankings(ctx, gameID, "test", 1, 20, "", 0)
		require.NoError(t, err)
		require.Len(t, rankings, 3)
		assert.Equal(t, 1, rankings[0].Rank, "first tied entry is rank 1")
		assert.Equal(t, 1, rankings[1].Rank, "second tied entry shares rank 1, not rank 2")
		assert.Equal(t, 3, rankings[2].Rank, "next distinct entry skips to rank 3, not rank 2")

		// A user's rank via the single-entry lookup must agree with their rank in the list.
		_, _, aliceEntry, err := svc.GetRankings(ctx, gameID, "test", 1, 20, "alice", 0)
		require.NoError(t, err)
		assert.Equal(t, 1, aliceEntry.Rank)
	})

	t.Run("consecutive ties reuse the previous rank instead of re-querying", func(t *testing.T) {
		svc, lbRepo, scoreRepo := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "a", 100))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "b", 100))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "c", 100))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "d", 50))

		scoreRepo.getUserRankCalls = 0
		rankings, _, _, err := svc.GetRankings(ctx, gameID, "test", 1, 20, "", 0)
		require.NoError(t, err)
		require.Len(t, rankings, 4)
		// 2 distinct scores (100 and 50) -> 2 GetUserRank calls, not 4.
		assert.Equal(t, 2, scoreRepo.getUserRankCalls)
	})

	t.Run("a tie group larger than page_size gets a consistent rank across every page", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		// five-way tie for first place, all sharing rank 1.
		for _, u := range []string{"p1", "p2", "p3", "p4", "p5"} {
			require.NoError(t, svc.SubmitScore(ctx, gameID, "test", u, 100))
		}

		for page := 1; page <= 5; page++ {
			rankings, total, _, err := svc.GetRankings(ctx, gameID, "test", page, 1, "", 0)
			require.NoError(t, err)
			assert.Equal(t, 5, total)
			require.Len(t, rankings, 1)
			assert.Equal(t, 1, rankings[0].Rank, "page %d: every member of the tie group is rank 1", page)
		}
	})

	t.Run("ties spanning a page boundary still get the correct global rank", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "first", 200))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "tied-a", 100))
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "tied-b", 100))

		// page_size 1: page 1 -> "first" (rank 1). page 2 -> first of the tied pair (rank 2),
		// whichever tied user that happens to be — its rank must be 2, not a position-derived 3.
		rankings, _, _, err := svc.GetRankings(ctx, gameID, "test", 2, 1, "", 0)
		require.NoError(t, err)
		require.Len(t, rankings, 1)
		assert.Equal(t, 100, rankings[0].Score)
		assert.Equal(t, 2, rankings[0].Rank)
	})

	t.Run("page beyond last returns empty slice", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user1", 100))

		rankings, _, _, err := svc.GetRankings(ctx, gameID, "test", 99, 20, "", 0)
		require.NoError(t, err)
		assert.Empty(t, rankings)
	})
}

// --- ListLeaderboards / UpdateLeaderboard / DeleteLeaderboard ---

func TestLeaderboardService_ListLeaderboards(t *testing.T) {
	ctx := context.Background()
	gameID := uuid.New()

	t.Run("returns only the requesting game's leaderboards", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "mine", domain.Record)
		seedLeaderboard(ctx, lbRepo, uuid.New(), "someone-elses", domain.Record)

		lbs, err := svc.ListLeaderboards(ctx, gameID)
		require.NoError(t, err)
		require.Len(t, lbs, 1)
		assert.Equal(t, "mine", lbs[0].UniqueName)
	})

	t.Run("a game with no leaderboards gets an empty slice, not an error", func(t *testing.T) {
		svc, _, _ := newTestService()
		lbs, err := svc.ListLeaderboards(ctx, gameID)
		require.NoError(t, err)
		assert.Empty(t, lbs)
	})
}

func TestLeaderboardService_UpdateLeaderboard(t *testing.T) {
	ctx := context.Background()
	gameID := uuid.New()

	t.Run("renames and updates description together", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "old-name", domain.Record)

		lb, err := svc.UpdateLeaderboard(ctx, gameID, "old-name", "new-name", "new description")
		require.NoError(t, err)
		assert.Equal(t, "new-name", lb.UniqueName)
		assert.Equal(t, "new description", lb.Description)

		_, _, _, err = svc.GetRankings(ctx, gameID, "old-name", 1, 20, "", 0)
		assert.Equal(t, domain.ErrLeaderboardNotFound, err, "old name must no longer resolve")
		_, _, _, err = svc.GetRankings(ctx, gameID, "new-name", 1, 20, "", 0)
		require.NoError(t, err)
	})

	t.Run("omitted fields keep their current value", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		lb := seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		lb.Description = "original"
		require.NoError(t, lbRepo.Update(ctx, lb))

		updated, err := svc.UpdateLeaderboard(ctx, gameID, "test", "", "")
		require.NoError(t, err)
		assert.Equal(t, "test", updated.UniqueName)
		assert.Equal(t, "original", updated.Description)
	})

	t.Run("leaderboard not found returns ErrLeaderboardNotFound", func(t *testing.T) {
		svc, _, _ := newTestService()
		_, err := svc.UpdateLeaderboard(ctx, gameID, "missing", "x", "")
		assert.Equal(t, domain.ErrLeaderboardNotFound, err)
	})

	t.Run("renaming to a name already used by this game returns ErrDuplicateLeaderboardName", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "taken", domain.Record)
		seedLeaderboard(ctx, lbRepo, gameID, "movable", domain.Record)

		_, err := svc.UpdateLeaderboard(ctx, gameID, "movable", "taken", "")
		assert.Equal(t, domain.ErrDuplicateLeaderboardName, err)
	})

	t.Run("type and interval_seconds are not exposed for editing", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		lb := seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record) // IntervalSeconds: 0
		updated, err := svc.UpdateLeaderboard(ctx, gameID, "test", "renamed", "")
		require.NoError(t, err)
		assert.Equal(t, domain.Record, updated.Type)
		assert.Equal(t, lb.IntervalSeconds, updated.IntervalSeconds)
	})
}

func TestLeaderboardService_DeleteLeaderboard(t *testing.T) {
	ctx := context.Background()
	gameID := uuid.New()

	t.Run("deletes the leaderboard and its cache", func(t *testing.T) {
		svc, lbRepo, scoreRepo := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "alice", 100))

		require.NoError(t, svc.DeleteLeaderboard(ctx, gameID, "test"))

		_, _, _, err := svc.GetRankings(ctx, gameID, "test", 1, 20, "", 0)
		assert.Equal(t, domain.ErrLeaderboardNotFound, err)
		assert.Equal(t, 1, scoreRepo.deleteLeaderboardCacheCalls, "cache invalidation must run for the deleted leaderboard")
	})

	t.Run("leaderboard not found returns ErrLeaderboardNotFound", func(t *testing.T) {
		svc, _, _ := newTestService()
		err := svc.DeleteLeaderboard(ctx, gameID, "missing")
		assert.Equal(t, domain.ErrLeaderboardNotFound, err)
	})

	t.Run("recreating under the same name after delete starts with no scores", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "alice", 100))
		require.NoError(t, svc.DeleteLeaderboard(ctx, gameID, "test"))

		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		_, total, _, err := svc.GetRankings(ctx, gameID, "test", 1, 20, "", 0)
		require.NoError(t, err)
		assert.Equal(t, 0, total)
	})
}

// --- SetScore / DeleteScore ---

func TestLeaderboardService_SetScore(t *testing.T) {
	ctx := context.Background()
	gameID := uuid.New()

	t.Run("overwrites the score regardless of leaderboard type semantics", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "alice", 100))

		// record type would reject a lower score via SubmitScore; SetScore must not apply that.
		require.NoError(t, svc.SetScore(ctx, gameID, "test", "alice", 42, -1))

		_, _, userEntry, err := svc.GetRankings(ctx, gameID, "test", 1, 20, "alice", 0)
		require.NoError(t, err)
		assert.Equal(t, 42, userEntry.Score)
	})

	t.Run("leaderboard not found returns ErrLeaderboardNotFound", func(t *testing.T) {
		svc, _, _ := newTestService()
		err := svc.SetScore(ctx, gameID, "missing", "alice", 1, -1)
		assert.Equal(t, domain.ErrLeaderboardNotFound, err)
	})
}

func TestLeaderboardService_DeleteScore(t *testing.T) {
	ctx := context.Background()
	gameID := uuid.New()

	t.Run("removes the user's score", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "alice", 100))

		require.NoError(t, svc.DeleteScore(ctx, gameID, "test", "alice", -1))

		_, _, userEntry, err := svc.GetRankings(ctx, gameID, "test", 1, 20, "alice", 0)
		require.NoError(t, err)
		assert.Nil(t, userEntry)
	})

	t.Run("deleting an absent score returns ErrScoreNotFound", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		err := svc.DeleteScore(ctx, gameID, "test", "ghost", -1)
		assert.Equal(t, domain.ErrScoreNotFound, err)
	})

	t.Run("leaderboard not found returns ErrLeaderboardNotFound", func(t *testing.T) {
		svc, _, _ := newTestService()
		err := svc.DeleteScore(ctx, gameID, "missing", "alice", -1)
		assert.Equal(t, domain.ErrLeaderboardNotFound, err)
	})
}
