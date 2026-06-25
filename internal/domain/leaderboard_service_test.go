package domain_test

import (
	"context"
	"fmt"
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

	t.Run("page beyond last returns empty slice", func(t *testing.T) {
		svc, lbRepo, _ := newTestService()
		seedLeaderboard(ctx, lbRepo, gameID, "test", domain.Record)
		require.NoError(t, svc.SubmitScore(ctx, gameID, "test", "user1", 100))

		rankings, _, _, err := svc.GetRankings(ctx, gameID, "test", 99, 20, "", 0)
		require.NoError(t, err)
		assert.Empty(t, rankings)
	})
}
