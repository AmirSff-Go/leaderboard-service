package domain

import (
	"context"
)

type RecordScoreProcessor struct{}

func NewRecordScoreProcessor() *RecordScoreProcessor {
	return &RecordScoreProcessor{}
}

func (p *RecordScoreProcessor) ProcessScore(ctx context.Context, currentScore *Score, newScore int, userID string) (shouldSave bool, finalScore int, err error) {
	if currentScore == nil || newScore > currentScore.Score {
		return true, newScore, nil
	}
	return false, 0, nil
}

type AdditiveScoreProcessor struct{}

func NewAdditiveScoreProcessor() *AdditiveScoreProcessor {
	return &AdditiveScoreProcessor{}
}

func (p *AdditiveScoreProcessor) ProcessScore(ctx context.Context, currentScore *Score, newScore int, userID string) (shouldSave bool, finalScore int, err error) {
	finalScore = newScore
	if currentScore != nil {
		finalScore += currentScore.Score
	}
	return true, finalScore, nil
}

type OneTimeScoreProcessor struct{}

func NewOneTimeScoreProcessor() *OneTimeScoreProcessor {
	return &OneTimeScoreProcessor{}
}

func (p *OneTimeScoreProcessor) ProcessScore(ctx context.Context, currentScore *Score, newScore int, userID string) (shouldSave bool, finalScore int, err error) {
	if currentScore == nil {
		return true, newScore, nil
	}
	return false, 0, nil
}
