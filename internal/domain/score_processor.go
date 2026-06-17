package domain

import (
	"context"
)

type ScoreProcessor interface {
	ProcessScore(ctx context.Context, currentScore *Score, newScore int, userID string) (shouldSave bool, finalScore int, err error)
}

type ScoreProcessorFactory interface {
	GetProcessor(leaderboardType LeaderboardType) (ScoreProcessor, error)
}
