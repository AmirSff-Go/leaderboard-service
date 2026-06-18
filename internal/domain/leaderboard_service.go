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
	CountByLeaderboard(ctx context.Context, leaderboardID uuid.UUID, durationIndex int) (int, error)
	GetRanking(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, page, pageSize int) ([]*Score, error)
	GetUserRank(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, score int) (int, error)
}

type ScoreObject struct {
	Rank   int    `json:"rank"`
	UserID string `json:"user_id"`
	Score  int    `json:"score"`
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

func (s *LeaderboardService) GetRankings(ctx context.Context, gameID uuid.UUID, leaderboardName string, page, pageSize int, userID string, durationIndex int) ([]*ScoreObject, int, *ScoreObject, error) {
	leaderboard, err := s.leaderboardRepo.GetByGameAndName(ctx, gameID, leaderboardName)
	if err != nil {
		return nil, 0, nil, err
	}

	if durationIndex == -1 {
		durationIndex = CurrentDurationIndex(leaderboard)
	}

	rankingScores, err := s.scoreRepo.GetRanking(ctx, leaderboard.ID, durationIndex, page, pageSize)
	if err != nil {
		return nil, 0, nil, err
	}
	rankingObjects := make([]*ScoreObject, len(rankingScores))
	for i, score := range rankingScores {
		rankingObjects[i] = &ScoreObject{
			Rank:   (page-1)*pageSize + i + 1,
			UserID: score.UserID,
			Score:  score.Score,
		}
	}

	total, err := s.scoreRepo.CountByLeaderboard(ctx, leaderboard.ID, durationIndex)
	if err != nil {
		return nil, 0, nil, err
	}

	var userEntry *ScoreObject
	if userID != "" {
		userScore, err := s.scoreRepo.GetByLeaderboardAndUser(ctx, leaderboard.ID, userID, durationIndex)
		if err != nil {
			if err == ErrScoreNotFound {
				userEntry = nil
			} else {
				return nil, 0, nil, err
			}
		} else {
			rank, err := s.scoreRepo.GetUserRank(ctx, leaderboard.ID, durationIndex, userScore.Score)
			if err != nil {
				return nil, 0, nil, err
			}

			userEntry = &ScoreObject{
				Rank:   rank,
				Score:  userScore.Score,
				UserID: userScore.UserID,
			}
		}
	}
	return rankingObjects, total, userEntry, nil
}
