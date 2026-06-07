package domain

import (
	"time"

	"github.com/google/uuid"
)

type Score struct {
	ID            uuid.UUID `json:"id"`
	LeaderboardID uuid.UUID `json:"leaderboard_id"`
	UserID        string    `json:"user_id"`
	ScoreValue    int       `json:"score_value"`
	DurationIndex int       `json:"duration_index"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
