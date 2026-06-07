package domain

import (
	"time"

	"github.com/google/uuid"
)

type LeaderboardType string

const (
	Record   LeaderboardType = "record"
	Additive LeaderboardType = "additive"
	OneTime  LeaderboardType = "onetime"
)

type Leaderboard struct {
	ID              uuid.UUID       `json:"id"`
	GameID          uuid.UUID       `json:"game_id"`
	UniqueName      string          `json:"unique_name"`
	Description     string          `json:"description"`
	Type            LeaderboardType `json:"type"`
	IntervalSeconds int             `json:"interval_seconds"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}
