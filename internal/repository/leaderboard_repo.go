package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
)

var (
	ErrLeaderboardNotFound      = errors.New("leaderboard not found")
	ErrDuplicateLeaderboardName = errors.New("leaderboard name already exists for this game")
)

type LeaderboardRepo interface {
	Create(ctx context.Context, leaderboard *domain.Leaderboard) error
	GetByGameAndName(ctx context.Context, gameID uuid.UUID, uniqueName string) (*domain.Leaderboard, error)
	ListByGame(ctx context.Context, gameID uuid.UUID) ([]*domain.Leaderboard, error)
}
