package domain

import "errors"

var ErrUnsupportedLeaderboardType = errors.New("unsupported leaderboard type")

type DefaultScoreProcessorFactory struct {
	processors map[LeaderboardType]ScoreProcessor
}

func NewScoreProcessorFactory() *DefaultScoreProcessorFactory {
	return &DefaultScoreProcessorFactory{
		processors: map[LeaderboardType]ScoreProcessor{
			Record:   NewRecordScoreProcessor(),
			Additive: NewAdditiveScoreProcessor(),
			OneTime:  NewOneTimeScoreProcessor(),
		},
	}
}

func (f *DefaultScoreProcessorFactory) GetProcessor(leaderboardType LeaderboardType) (ScoreProcessor, error) {
	processor, exists := f.processors[leaderboardType]
	if !exists {
		return nil, ErrUnsupportedLeaderboardType
	}
	return processor, nil
}
