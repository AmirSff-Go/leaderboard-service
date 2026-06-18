package domain

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrScoreNotFound = errors.New("score not found")

type LeaderboardRepository interface {
	GetByGameAndName(ctx context.Context, gameID uuid.UUID, uniqueName string) (*Leaderboard, error)
	Create(ctx context.Context, leaderboard *Leaderboard) error
}

type ScoreRepository interface {
	Upsert(ctx context.Context, score *Score) error
	GetByLeaderboardAndUser(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int) (*Score, error)
}

type LeaderboardService struct {
	leaderboardRepo  LeaderboardRepository
	scoreRepo        ScoreRepository
	processorFactory ScoreProcessorFactory
}

func NewLeaderboardService(leaderboardRepo LeaderboardRepository, scoreRepo ScoreRepository, processorFactory ScoreProcessorFactory) *LeaderboardService {
	return &LeaderboardService{
		leaderboardRepo:  leaderboardRepo,
		scoreRepo:        scoreRepo,
		processorFactory: processorFactory,
	}
}

func (s *LeaderboardService) SubmitScore(ctx context.Context, gameID uuid.UUID, leaderboardName string, userID string, newScore int) error {
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

func (s *LeaderboardService) CreateLeaderboard(ctx context.Context, gameID uuid.UUID, uniqueName, description string, lbType LeaderboardType, intervalSeconds int) (*Leaderboard, error) {
	leaderboard := &Leaderboard{
		GameID:          gameID,
		UniqueName:      uniqueName,
		Description:     description,
		Type:            lbType,
		IntervalSeconds: intervalSeconds,
	}
	err := s.leaderboardRepo.Create(ctx, leaderboard)
	if err != nil {
		return nil, err
	}
	return leaderboard, nil
}
