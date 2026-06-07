package domain

import "context"

type ScoreProcessor interface {
	ProcessScore(ctx context.Context, leaderboardType LeaderboardType,
		currentScore *Score, newScoreValue int, userID string) (bool, int, error)
}
