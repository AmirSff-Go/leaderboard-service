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
}
