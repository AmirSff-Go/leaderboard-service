package domain

import "context"

type DefaultScoreProcessor struct{}

func NewScoreProcessor() *DefaultScoreProcessor {
	return &DefaultScoreProcessor{}
}

func (p *DefaultScoreProcessor) ProcessScore(ctx context.Context, leaderboardType LeaderboardType,
	currentScore *Score, newScoreValue int, userID string) (shouldSave bool, finalValue int, err error) {

	switch leaderboardType {
	case Record:
		if currentScore == nil || newScoreValue > currentScore.ScoreValue {
			return true, newScoreValue, nil
		}
		return false, 0, nil
	case Additive:
		finalValue := newScoreValue
		if currentScore != nil {
			finalValue += currentScore.ScoreValue
		}
		return true, finalValue, nil
	case OneTime:
		if currentScore == nil {
			return true, newScoreValue, nil
		}
		return false, 0, nil
	}
	return false, 0, nil

}
