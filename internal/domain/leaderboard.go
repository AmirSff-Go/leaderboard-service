package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type LeaderboardType string

var (
	ErrLeaderboardNotFound      = errors.New("leaderboard not found")
	ErrDuplicateLeaderboardName = errors.New("leaderboard name already exists for this game")
)

const (
	Record   LeaderboardType = "record"
	Additive LeaderboardType = "additive"
	OneTime  LeaderboardType = "onetime"
)

func IsValidLeaderboardType(t string) bool {
	switch LeaderboardType(t) {
	case Record, Additive, OneTime:
		return true
	}
	return false
}

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
