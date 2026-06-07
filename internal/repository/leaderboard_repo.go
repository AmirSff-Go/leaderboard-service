package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/lib/pq"

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

type PostgresLeaderboardRepo struct {
	db *sql.DB
}

func NewPostgresLeaderboardRepo(db *sql.DB) *PostgresLeaderboardRepo {
	return &PostgresLeaderboardRepo{db: db}
}

func (r *PostgresLeaderboardRepo) Create(ctx context.Context, leaderboard *domain.Leaderboard) error {
	if leaderboard.ID == uuid.Nil {
		leaderboard.ID = uuid.New()
	}
	query := `
		INSERT INTO leaderboards (id, game_id, unique_name, description, type, interval_seconds, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING created_at, updated_at
	`
	err := r.db.QueryRowContext(ctx, query,
		leaderboard.ID,
		leaderboard.GameID,
		leaderboard.UniqueName,
		leaderboard.Description,
		leaderboard.Type,
		leaderboard.IntervalSeconds,
	).Scan(&leaderboard.CreatedAt, &leaderboard.UpdatedAt)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return ErrDuplicateLeaderboardName
		}
		return err
	}
	return nil
}

func (r *PostgresLeaderboardRepo) GetByGameAndName(ctx context.Context, gameID uuid.UUID, uniqueName string) (*domain.Leaderboard, error) {
	query := `
		SELECT id, game_id, unique_name, description, type, interval_seconds, created_at, updated_at
		FROM leaderboards
		WHERE game_id = $1 AND unique_name = $2
	`
	var leaderboard domain.Leaderboard
	err := r.db.QueryRowContext(ctx, query, gameID, uniqueName).Scan(
		&leaderboard.ID,
		&leaderboard.GameID,
		&leaderboard.UniqueName,
		&leaderboard.Description,
		&leaderboard.Type,
		&leaderboard.IntervalSeconds,
		&leaderboard.CreatedAt,
		&leaderboard.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrLeaderboardNotFound
		}
		return nil, err
	}
	return &leaderboard, nil
}

func (r *PostgresLeaderboardRepo) ListByGame(ctx context.Context, gameID uuid.UUID) ([]*domain.Leaderboard, error) {
	query := `
		SELECT id, game_id, unique_name, description, type, interval_seconds, created_at, updated_at
		FROM leaderboards
		WHERE game_id = $1
	`
	rows, err := r.db.QueryContext(ctx, query, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	leaderboards := make([]*domain.Leaderboard, 0)
	for rows.Next() {
		var leaderboard domain.Leaderboard
		if err := rows.Scan(
			&leaderboard.ID,
			&leaderboard.GameID,
			&leaderboard.UniqueName,
			&leaderboard.Description,
			&leaderboard.Type,
			&leaderboard.IntervalSeconds,
			&leaderboard.CreatedAt,
			&leaderboard.UpdatedAt,
		); err != nil {
			return nil, err
		}
		leaderboards = append(leaderboards, &leaderboard)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return leaderboards, nil
}
