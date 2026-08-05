package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

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
			return domain.ErrDuplicateLeaderboardName
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
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrLeaderboardNotFound
		}
		return nil, err
	}
	return &leaderboard, nil
}

func (r *PostgresLeaderboardRepo) Update(ctx context.Context, leaderboard *domain.Leaderboard) error {
	query := `
		UPDATE leaderboards
		SET unique_name = $1, description = $2, updated_at = NOW()
		WHERE id = $3
		RETURNING updated_at
	`
	err := r.db.QueryRowContext(ctx, query,
		leaderboard.UniqueName, leaderboard.Description, leaderboard.ID,
	).Scan(&leaderboard.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrLeaderboardNotFound
		}
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return domain.ErrDuplicateLeaderboardName
		}
		return err
	}
	return nil
}

// Delete removes the leaderboard row. All of its scores go with it via the scores table's
// ON DELETE CASCADE foreign key — the caller is still responsible for invalidating any Redis
// cache for this leaderboard, which has no equivalent foreign-key relationship to fall back on.
func (r *PostgresLeaderboardRepo) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM leaderboards WHERE id = $1`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrLeaderboardNotFound
	}
	return nil
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
