package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrScoreNotFound = errors.New("score not found")

type LeaderboardRepository interface {
	GetByGameAndName(ctx context.Context, gameID uuid.UUID, uniqueName string) (*Leaderboard, error)
	Create(ctx context.Context, leaderboard *Leaderboard) error
	ListByGame(ctx context.Context, gameID uuid.UUID) ([]*Leaderboard, error)
	Update(ctx context.Context, leaderboard *Leaderboard) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// ttl is how long a repository implementation that caches by (leaderboardID, durationIndex)
// should keep that bucket cached, per BucketCacheTTL — 0 means never expire. Callers always pass
// the value BucketCacheTTL computed for the bucket being touched; a non-caching implementation
// (e.g. pure Postgres) is free to ignore it.
type ScoreRepository interface {
	Upsert(ctx context.Context, score *Score, ttl time.Duration) error
	GetByLeaderboardAndUser(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int) (*Score, error)
	CountByLeaderboard(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, ttl time.Duration) (int, error)
	GetRanking(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, ttl time.Duration, page, pageSize int) ([]*Score, error)
	GetUserRank(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, ttl time.Duration, score int) (int, error)

	// SubmitScoreAtomic serializes read-decide-write for a single (leaderboardID, userID, durationIndex)
	// tuple so concurrent submissions can't race on a stale read. The repository fetches the current
	// score, invokes decide exactly once with it, and — if decide reports shouldSave — persists
	// finalScore, all as one atomic unit. current is nil if no score exists yet for that tuple.
	SubmitScoreAtomic(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int, ttl time.Duration,
		decide func(current *Score) (shouldSave bool, finalScore int, err error)) error

	// DeleteScore removes a single user's score for one period bucket.
	DeleteScore(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int) error

	// DeleteLeaderboardCache clears every cache entry for a leaderboard across all of its period
	// buckets. Deleting the leaderboard row itself is a LeaderboardRepository concern (Postgres
	// cascades its scores automatically); the cache has no such relationship and must be cleared
	// explicitly, or an all-time leaderboard's single permanent bucket (BucketCacheTTL never
	// expires it) would leak in Redis forever after the leaderboard it belongs to is gone.
	DeleteLeaderboardCache(ctx context.Context, leaderboardID uuid.UUID) error
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
	ttl := BucketCacheTTL(leaderboard.IntervalSeconds, durationIndex, time.Now())

	processor, err := s.processorFactory.GetProcessor(leaderboard.Type)
	if err != nil {
		return err
	}

	// The read (current score) and the write must happen as one atomic unit — deciding
	// shouldSave/finalScore from a snapshot read taken outside a lock lets concurrent
	// submissions for the same user/leaderboard/period race (e.g. two additive submissions
	// both reading the same base and one overwrite silently dropping the other's delta).
	return s.scoreRepo.SubmitScoreAtomic(ctx, leaderboard.ID, userID, durationIndex, ttl,
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

func (s *LeaderboardService) ListLeaderboards(ctx context.Context, gameID uuid.UUID) ([]*Leaderboard, error) {
	return s.leaderboardRepo.ListByGame(ctx, gameID)
}

// UpdateLeaderboard renames and/or redescribes a leaderboard. uniqueName/newDescription empty
// means "leave unchanged" (matches EditGame's convention elsewhere in this API). Type and
// IntervalSeconds are deliberately not editable here: changing either would silently reinterpret
// every score already recorded under the old semantics (e.g. existing period buckets would no
// longer line up with a new interval). Create a new leaderboard instead.
func (s *LeaderboardService) UpdateLeaderboard(ctx context.Context, gameID uuid.UUID, uniqueName, newUniqueName, newDescription string) (*Leaderboard, error) {
	leaderboard, err := s.leaderboardRepo.GetByGameAndName(ctx, gameID, uniqueName)
	if err != nil {
		return nil, err
	}
	if newUniqueName != "" {
		leaderboard.UniqueName = newUniqueName
	}
	if newDescription != "" {
		leaderboard.Description = newDescription
	}
	if err := s.leaderboardRepo.Update(ctx, leaderboard); err != nil {
		return nil, err
	}
	return leaderboard, nil
}

func (s *LeaderboardService) DeleteLeaderboard(ctx context.Context, gameID uuid.UUID, uniqueName string) error {
	leaderboard, err := s.leaderboardRepo.GetByGameAndName(ctx, gameID, uniqueName)
	if err != nil {
		return err
	}
	if err := s.leaderboardRepo.Delete(ctx, leaderboard.ID); err != nil {
		return err
	}
	// Best-effort: DeleteLeaderboardCache never returns an error for cache-layer failures (see
	// its doc comment), so this can't leave the leaderboard "half deleted" from the caller's view.
	return s.scoreRepo.DeleteLeaderboardCache(ctx, leaderboard.ID)
}

func (s *LeaderboardService) GetRankings(ctx context.Context, gameID uuid.UUID, leaderboardName string, page, pageSize int, userID string, durationIndex int) ([]*ScoreObject, int, *ScoreObject, error) {
	leaderboard, err := s.leaderboardRepo.GetByGameAndName(ctx, gameID, leaderboardName)
	if err != nil {
		return nil, 0, nil, err
	}

	durationIndex = resolveDurationIndex(leaderboard, durationIndex)
	ttl := BucketCacheTTL(leaderboard.IntervalSeconds, durationIndex, time.Now())

	rankingScores, err := s.scoreRepo.GetRanking(ctx, leaderboard.ID, durationIndex, ttl, page, pageSize)
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
			rank, err = s.scoreRepo.GetUserRank(ctx, leaderboard.ID, durationIndex, ttl, score.Score)
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

	total, err := s.scoreRepo.CountByLeaderboard(ctx, leaderboard.ID, durationIndex, ttl)
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
			rank, err := s.scoreRepo.GetUserRank(ctx, leaderboard.ID, durationIndex, ttl, userScore.Score)
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

// resolveDurationIndex applies GetRankings' existing convention (-1 means "the current period")
// to the score-mutation endpoints too, so all four leaderboard-scoped APIs agree on what
// duration_index means.
func resolveDurationIndex(leaderboard *Leaderboard, durationIndex int) int {
	if durationIndex == -1 {
		return CurrentDurationIndex(leaderboard)
	}
	return durationIndex
}

// SetScore directly overwrites a user's score for one period bucket, bypassing the leaderboard
// type's normal processing (record/additive/onetime). It exists for organizer corrections — "I
// mistyped this score" — where the caller wants their exact value to win regardless of what's
// already there, unlike SubmitScore which is a game client honestly reporting a new attempt.
func (s *LeaderboardService) SetScore(ctx context.Context, gameID uuid.UUID, leaderboardName, userID string, newScore, durationIndex int) error {
	leaderboard, err := s.leaderboardRepo.GetByGameAndName(ctx, gameID, leaderboardName)
	if err != nil {
		return err
	}
	durationIndex = resolveDurationIndex(leaderboard, durationIndex)
	ttl := BucketCacheTTL(leaderboard.IntervalSeconds, durationIndex, time.Now())

	return s.scoreRepo.SubmitScoreAtomic(ctx, leaderboard.ID, userID, durationIndex, ttl,
		func(current *Score) (bool, int, error) {
			return true, newScore, nil
		})
}

func (s *LeaderboardService) DeleteScore(ctx context.Context, gameID uuid.UUID, leaderboardName, userID string, durationIndex int) error {
	leaderboard, err := s.leaderboardRepo.GetByGameAndName(ctx, gameID, leaderboardName)
	if err != nil {
		return err
	}
	durationIndex = resolveDurationIndex(leaderboard, durationIndex)
	return s.scoreRepo.DeleteScore(ctx, leaderboard.ID, userID, durationIndex)
}
