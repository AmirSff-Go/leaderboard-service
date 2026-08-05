package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
                ON CONFLICT (leaderboard_id, user_id, duration_index) DO UPDATE
                SET score = EXCLUDED.score, updated_at = NOW()
        `
	if score.ID == uuid.Nil {
		score.ID = uuid.New()
	}
	_, err := r.db.ExecContext(ctx, query,
		score.ID,
		score.LeaderboardID,
		score.UserID,
		score.Score,
		score.DurationIndex,
	)
	return err
}

// SubmitScoreAtomic serializes read-decide-write for one (leaderboardID, userID, durationIndex)
// tuple using a transaction-scoped Postgres advisory lock. Concurrent submissions for the same
// tuple block on the lock instead of racing on a stale read; submissions for different tuples
// (different users, leaderboards, or periods) are unaffected and proceed in parallel.
func (r *PostgresScoreRepo) SubmitScoreAtomic(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int,
	decide func(current *domain.Score) (bool, int, error)) error {

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	lockKey := fmt.Sprintf("%s:%s:%d", leaderboardID, userID, durationIndex)
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, lockKey); err != nil {
		return err
	}

	current, err := getScoreForUpdate(ctx, tx, leaderboardID, userID, durationIndex)
	if err != nil {
		return err
	}

	shouldSave, finalScore, err := decide(current)
	if err != nil {
		return err
	}
	if !shouldSave {
		return tx.Commit()
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO scores (id, leaderboard_id, user_id, score, duration_index, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (leaderboard_id, user_id, duration_index) DO UPDATE
		SET score = EXCLUDED.score, updated_at = NOW()
	`, uuid.New(), leaderboardID, userID, finalScore, durationIndex)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func getScoreForUpdate(ctx context.Context, tx *sql.Tx, leaderboardID uuid.UUID, userID string, durationIndex int) (*domain.Score, error) {
	query := `
		SELECT id, leaderboard_id, user_id, score, duration_index, created_at, updated_at
		FROM scores
		WHERE leaderboard_id = $1 AND user_id = $2 AND duration_index = $3
	`
	var score domain.Score
	err := tx.QueryRowContext(ctx, query, leaderboardID, userID, durationIndex).Scan(
		&score.ID,
		&score.LeaderboardID,
		&score.UserID,
		&score.Score,
		&score.DurationIndex,
		&score.CreatedAt,
		&score.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &score, nil
}

func (r *PostgresScoreRepo) GetByLeaderboardAndUser(ctx context.Context, leaderboardID uuid.UUID, userID string,
	durationIndex int) (*domain.Score, error) {
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
		&score.Score,
		&score.DurationIndex,
		&score.CreatedAt,
		&score.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrScoreNotFound
		}
		return nil, err
	}

	return &score, nil
}

func (r *PostgresScoreRepo) GetUserRank(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, score int) (int, error) {

	rankQuery := `
				SELECT COUNT(*) + 1 AS rank
				FROM scores
				WHERE leaderboard_id = $1 AND duration_index = $2 AND score > $3
	`
	var rank int
	err := r.db.QueryRowContext(ctx, rankQuery, leaderboardID, durationIndex, score).Scan(&rank)
	if err != nil {
		return -1, err
	}
	return rank, nil
}

func (r *PostgresScoreRepo) GetRanking(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, page,
	pageSize int) ([]*domain.Score, error) {
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
			&score.Score,
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

// ListAllByLeaderboard returns every score for a (leaderboard, duration_index) bucket, ordered by
// score descending, with no pagination. Used to fully hydrate the Redis cache so a partial page
// fetch can never be mistaken for the complete leaderboard.
func (r *PostgresScoreRepo) ListAllByLeaderboard(ctx context.Context, leaderboardID uuid.UUID, durationIndex int) ([]*domain.Score, error) {
	query := `
                SELECT id, leaderboard_id, user_id, score, duration_index, created_at, updated_at
                FROM scores
                WHERE leaderboard_id = $1 AND duration_index = $2
                ORDER BY score DESC
        `
	rows, err := r.db.QueryContext(ctx, query, leaderboardID, durationIndex)
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
			&score.Score,
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

func (r *PostgresScoreRepo) CountByLeaderboard(ctx context.Context, leaderboardID uuid.UUID, durationIndex int) (int,
	error) {
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
