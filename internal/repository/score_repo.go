package repository

import (
	"context"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/google/uuid"
)

type ScoreRepo interface {
	Upsert(ctx context.Context, score *domain.Score) error

	GetByLeaderboardAndUser(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int) (*domain.Score, error)

	GetRanking(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, page, pageSize int) ([]*domain.Score, error)

	CountByLeaderboard(ctx context.Context, leaderboardID uuid.UUID, durationIndex int) (int, error)

	GetUserRank(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, score int) (int, error)

	// SubmitScoreAtomic runs decide with the current score for (leaderboardID, userID, durationIndex)
	// and, if it reports shouldSave, persists finalScore — all serialized against concurrent callers
	// for that same tuple. current is nil when no score exists yet.
	SubmitScoreAtomic(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int,
		decide func(current *domain.Score) (shouldSave bool, finalScore int, err error)) error
}
