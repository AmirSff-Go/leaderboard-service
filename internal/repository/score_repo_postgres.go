package repository

import (
	"context"
	"database/sql"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/google/uuid"
)

type PostgresScoreRepo struct {
	db *sql.DB
}

func NewPostgresScoreRepo(db *sql.DB) *PostgresScoreRepo {
	return &PostgresScoreRepo{db: db}
}

func (r *PostgresScoreRepo) Upsert(ctx context.Context, score *domain.Score) error {
	query := `
		INSERT INTO scores (id, leaderboard_id, user_id, score, duration_index, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (leaderboard_id, user_id, duration_index) DO UPDATE SET score = EXCLUDED.score, updated_at = NOW()
	`
	if score.ID == uuid.Nil {
		score.ID = uuid.New()
	}
	_, err := r.db.ExecContext(ctx, query,
		score.ID,
		score.LeaderboardID,
		score.UserID,
		score.ScoreValue,
		score.DurationIndex,
	)
	return err
}

func (r *PostgresScoreRepo) GetByLeaderboardAndUser(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int) (*domain.Score, error) {
	query := `
		SELECT id, leaderboard_id, user_id, score, duration_index, created_at, updated_at	
		FROM scores
		WHERE leaderboard_id = $1 AND user_id = $2 AND duration_index = $3
	`
	var score domain.Score
	err := r.db.QueryRowContext(ctx, query, leaderboardID, userID, durationIndex).Scan(
		&score.ID,
		&score.LeaderboardID,
		&score.UserID,
		&score.ScoreValue,
		&score.DurationIndex,
		&score.CreatedAt,
		&score.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrScoreNotFound
		}
		return nil, err
	}
	return &score, nil
}

func (r *PostgresScoreRepo) GetRanking(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, page, pageSize int) ([]*domain.Score, error) {
	offset := (page - 1) * pageSize
	query := `
		SELECT id, leaderboard_id, user_id, score, duration_index, created_at, updated_at
		FROM scores
		WHERE leaderboard_id = $1 AND duration_index = $2
		ORDER BY score DESC
		LIMIT $3 OFFSET $4
	`
	rows, err := r.db.QueryContext(ctx, query, leaderboardID, durationIndex, pageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	scores := make([]*domain.Score, 0)
	for rows.Next() {
		var score domain.Score
		if err := rows.Scan(
			&score.ID,
			&score.LeaderboardID,
			&score.UserID,
			&score.ScoreValue,
			&score.DurationIndex,
			&score.CreatedAt,
			&score.UpdatedAt,
		); err != nil {
			return nil, err
		}
		scores = append(scores, &score)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return scores, nil
}

func (r *PostgresScoreRepo) CountByLeaderboard(ctx context.Context, leaderboardID uuid.UUID, durationIndex int) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM scores
		WHERE leaderboard_id = $1 AND duration_index = $2
	`
	var count int
	err := r.db.QueryRowContext(ctx, query, leaderboardID, durationIndex).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}
