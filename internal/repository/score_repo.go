package repository

import (
	"context"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/google/uuid"
)

type ScoreRepo interface {
	// Save a score for a user in a leaderboard
	Upsert(ctx context.Context, score *domain.Score) error

	// Get the current score for a user in a leaderboard
	GetByLeaderboardAndUser(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int) (*domain.Score, error)

	// Get ranking (pagination)
	GetRanking(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, page, pageSize int) ([]*domain.Score, error)

	// Count total scores in a leaderboard period
	CountByLeaderboard(ctx context.Context, leaderboardID uuid.UUID, durationIndex int) (int, error)
}
