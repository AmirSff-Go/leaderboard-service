package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
)

var ErrGameNotFound = errors.New("game not found")

type GameRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Game, error)
	Create(ctx context.Context, game *domain.Game) error
	Update(ctx context.Context, game *domain.Game) error
}

type PostgresGameRepo struct {
	db *sql.DB
}

func NewPostgresGameRepo(db *sql.DB) *PostgresGameRepo {
	return &PostgresGameRepo{db: db}
}

func (r *PostgresGameRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Game, error) {
	query := `
		SELECT id, name, description, token_version, created_at, updated_at
		FROM games
		WHERE id = $1
	`

	game := &domain.Game{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&game.ID,
		&game.Name,
		&game.Description,
		&game.TokenVersion,
		&game.CreatedAt,
		&game.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrGameNotFound
		}
		return nil, err
	}

	return game, nil
}

func (r *PostgresGameRepo) Create(ctx context.Context, game *domain.Game) error {
	if game.ID == uuid.Nil {
		game.ID = uuid.New()
	}

	query := `
		INSERT INTO games (id, name, description, token_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		game.ID, game.Name, game.Description, game.TokenVersion,
	).Scan(&game.CreatedAt, &game.UpdatedAt)

	return err
}

func (r *PostgresGameRepo) Update(ctx context.Context, game *domain.Game) error {
	query := `
		UPDATE games
		SET name = $1, description = $2, token_version = $3, updated_at = NOW()
		WHERE id = $4
		RETURNING updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		game.Name, game.Description, game.TokenVersion, game.ID,
	).Scan(&game.UpdatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrGameNotFound
		}
		return err
	}

	return nil
}
