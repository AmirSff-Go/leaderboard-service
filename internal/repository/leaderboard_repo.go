package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
)

type LeaderboardRepo interface {
	Create(ctx context.Context, leaderboard *domain.Leaderboard) error
	GetByGameAndName(ctx context.Context, gameID uuid.UUID, uniqueName string) (*domain.Leaderboard, error)
	ListByGame(ctx context.Context, gameID uuid.UUID) ([]*domain.Leaderboard, error)
}
