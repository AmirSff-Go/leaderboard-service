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

	// SubmitScoreAtomic serializes read-decide-write for a single (leaderboardID, userID, durationIndex)
	// tuple so concurrent submissions can't race on a stale read. The repository fetches the current
	// score, invokes decide exactly once with it, and — if decide reports shouldSave — persists
	// finalScore, all as one atomic unit. current is nil if no score exists yet for that tuple.
	SubmitScoreAtomic(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int,
		decide func(current *Score) (shouldSave bool, finalScore int, err error)) error
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

	processor, err := s.processorFactory.GetProcessor(leaderboard.Type)
	if err != nil {
		return err
	}

	// The read (current score) and the write must happen as one atomic unit — deciding
	// shouldSave/finalScore from a snapshot read taken outside a lock lets concurrent
	// submissions for the same user/leaderboard/period race (e.g. two additive submissions
	// both reading the same base and one overwrite silently dropping the other's delta).
	return s.scoreRepo.SubmitScoreAtomic(ctx, leaderboard.ID, userID, durationIndex,
		func(current *Score) (bool, int, error) {
			return processor.ProcessScore(ctx, current, newScore, userID)
		})
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
	// Rank is competition ranking (ties share a rank, e.g. 1,1,3) computed the same way as the
	// single-user lookup below, via GetUserRank — not list position, which would silently give
	// tied scores different ranks depending on where the page boundary falls. Consecutive rows
	// with an identical score reuse the previous row's rank instead of re-querying.
	rankingObjects := make([]*ScoreObject, len(rankingScores))
	var lastScore int
	var lastRank int
	for i, score := range rankingScores {
		rank := lastRank
		if i == 0 || score.Score != lastScore {
			rank, err = s.scoreRepo.GetUserRank(ctx, leaderboard.ID, durationIndex, score.Score)
			if err != nil {
				return nil, 0, nil, err
			}
		}
		rankingObjects[i] = &ScoreObject{
			Rank:   rank,
			UserID: score.UserID,
			Score:  score.Score,
		}
		lastScore, lastRank = score.Score, rank
	}

	total, err := s.scoreRepo.CountByLeaderboard(ctx, leaderboard.ID, durationIndex)
	if err != nil {
		return nil, 0, nil, err
	}

	var userEntry *ScoreObject
	if userID != "" {
		userScore, err := s.scoreRepo.GetByLeaderboardAndUser(ctx, leaderboard.ID, userID, durationIndex)
		if err != nil {
			if errors.Is(err, ErrScoreNotFound) {
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
