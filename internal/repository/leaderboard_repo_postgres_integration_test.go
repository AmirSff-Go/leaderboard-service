//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/AmirSff-Go/leaderboard-service/internal/repository"
)

func TestPostgresLeaderboardRepo_Update(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewPostgresLeaderboardRepo(testDB)

	t.Run("renames and redescribes", func(t *testing.T) {
		lb := seedGameAndLeaderboard(t, domain.Record, 0)
		lb.UniqueName = "renamed-" + lb.UniqueName
		lb.Description = "updated description"

		require.NoError(t, repo.Update(ctx, lb))

		fetched, err := repo.GetByGameAndName(ctx, lb.GameID, lb.UniqueName)
		require.NoError(t, err)
		assert.Equal(t, lb.UniqueName, fetched.UniqueName)
		assert.Equal(t, "updated description", fetched.Description)
	})

	t.Run("renaming to a name already taken in the same game returns ErrDuplicateLeaderboardName", func(t *testing.T) {
		gameID := seedGameAndLeaderboard(t, domain.Record, 0).GameID
		taken := &domain.Leaderboard{GameID: gameID, UniqueName: "taken-name", Type: domain.Record}
		require.NoError(t, repo.Create(ctx, taken))
		movable := &domain.Leaderboard{GameID: gameID, UniqueName: "movable-name", Type: domain.Record}
		require.NoError(t, repo.Create(ctx, movable))

		movable.UniqueName = "taken-name"
		err := repo.Update(ctx, movable)
		assert.Equal(t, domain.ErrDuplicateLeaderboardName, err)
	})

	t.Run("updating a nonexistent leaderboard returns ErrLeaderboardNotFound", func(t *testing.T) {
		ghost := &domain.Leaderboard{ID: uuid.New(), UniqueName: "ghost", Description: "x"}
		err := repo.Update(ctx, ghost)
		assert.Equal(t, domain.ErrLeaderboardNotFound, err)
	})
}

func TestPostgresLeaderboardRepo_Delete(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewPostgresLeaderboardRepo(testDB)
	scoreRepo := repository.NewPostgresScoreRepo(testDB)

	t.Run("deletes the leaderboard and cascades its scores", func(t *testing.T) {
		lb := seedGameAndLeaderboard(t, domain.Record, 0)
		require.NoError(t, scoreRepo.Upsert(ctx, &domain.Score{
			LeaderboardID: lb.ID, UserID: "alice", Score: 100, DurationIndex: 0,
		}, 0))

		require.NoError(t, repo.Delete(ctx, lb.ID))

		_, err := repo.GetByGameAndName(ctx, lb.GameID, lb.UniqueName)
		assert.Equal(t, domain.ErrLeaderboardNotFound, err)

		count, err := scoreRepo.CountByLeaderboard(ctx, lb.ID, 0, 0)
		require.NoError(t, err)
		assert.Zero(t, count, "scores must cascade-delete with their leaderboard")
	})

	t.Run("deleting a nonexistent leaderboard returns ErrLeaderboardNotFound", func(t *testing.T) {
		err := repo.Delete(ctx, uuid.New())
		assert.Equal(t, domain.ErrLeaderboardNotFound, err)
	})
}
