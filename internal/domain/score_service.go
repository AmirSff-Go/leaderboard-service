package domain

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrScoreNotFound = errors.New("score not found")

type LeaderboardRepository interface {
	GetByGameAndName(ctx context.Context, gameID uuid.UUID, uniqueName string) (*Leaderboard, error)
}

type ScoreRepository interface {
	Upsert(ctx context.Context, score *Score) error
	GetByLeaderboardAndUser(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int) (*Score, error)
}

type ScoreService struct {
	leaderboardRepo  LeaderboardRepository
	scoreRepo        ScoreRepository
	processorFactory ScoreProcessorFactory
}

func NewScoreService(leaderboardRepo LeaderboardRepository, scoreRepo ScoreRepository, processorFactory ScoreProcessorFactory) *ScoreService {
	return &ScoreService{
		leaderboardRepo:  leaderboardRepo,
		scoreRepo:        scoreRepo,
		processorFactory: processorFactory,
	}
}

func (s *ScoreService) SubmitScore(ctx context.Context, gameID uuid.UUID, leaderboardName string, userID string, newScore int) error {
	leaderboard, err := s.leaderboardRepo.GetByGameAndName(ctx, gameID, leaderboardName)
	if err != nil {
		return err
	}

	durationIndex := CurrentDurationIndex(leaderboard)

	existingScore, err := s.scoreRepo.GetByLeaderboardAndUser(ctx, leaderboard.ID, userID, durationIndex)
	if err != nil && err != ErrScoreNotFound {
		return err
	}

	processor, err := s.processorFactory.GetProcessor(leaderboard.Type)
	if err != nil {
		return err
	}

	updated, finalScore, err := processor.ProcessScore(ctx, existingScore, newScore, userID)
	if err != nil {
		return err
	}

	if updated {
		score := &Score{
			LeaderboardID: leaderboard.ID,
			UserID:        userID,
			Score:         finalScore,
			DurationIndex: durationIndex,
		}
		err := s.scoreRepo.Upsert(ctx, score)
		if err != nil {
			return err
		}
	}

	return nil
}
