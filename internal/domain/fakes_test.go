package domain_test

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/google/uuid"
)

// fakeLeaderboardRepo is an in-memory domain.LeaderboardRepository for unit tests.
type fakeLeaderboardRepo struct {
	mu           sync.Mutex
	leaderboards map[string]*domain.Leaderboard
}

func newFakeLeaderboardRepo() *fakeLeaderboardRepo {
	return &fakeLeaderboardRepo{leaderboards: make(map[string]*domain.Leaderboard)}
}

func (r *fakeLeaderboardRepo) Create(ctx context.Context, lb *domain.Leaderboard) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := lb.GameID.String() + ":" + lb.UniqueName
	if _, exists := r.leaderboards[k]; exists {
		return domain.ErrDuplicateLeaderboardName
	}
	if lb.ID == uuid.Nil {
		lb.ID = uuid.New()
	}
	r.leaderboards[k] = lb
	return nil
}

func (r *fakeLeaderboardRepo) GetByGameAndName(ctx context.Context, gameID uuid.UUID, uniqueName string) (*domain.Leaderboard, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lb, ok := r.leaderboards[gameID.String()+":"+uniqueName]
	if !ok {
		return nil, domain.ErrLeaderboardNotFound
	}
	return lb, nil
}

// fakeScoreRepo is an in-memory domain.ScoreRepository for unit tests.
type fakeScoreRepo struct {
	mu     sync.Mutex
	scores map[string]*domain.Score
}

func newFakeScoreRepo() *fakeScoreRepo {
	return &fakeScoreRepo{scores: make(map[string]*domain.Score)}
}

func domainScoreKey(leaderboardID uuid.UUID, userID string, durationIndex int) string {
	return fmt.Sprintf("%s:%s:%d", leaderboardID, userID, durationIndex)
}

func (r *fakeScoreRepo) Upsert(ctx context.Context, score *domain.Score) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scores[domainScoreKey(score.LeaderboardID, score.UserID, score.DurationIndex)] = score
	return nil
}

func (r *fakeScoreRepo) GetByLeaderboardAndUser(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int) (*domain.Score, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.scores[domainScoreKey(leaderboardID, userID, durationIndex)]
	if !ok {
		return nil, domain.ErrScoreNotFound
	}
	return s, nil
}

func (r *fakeScoreRepo) CountByLeaderboard(ctx context.Context, leaderboardID uuid.UUID, durationIndex int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, s := range r.scores {
		if s.LeaderboardID == leaderboardID && s.DurationIndex == durationIndex {
			n++
		}
	}
	return n, nil
}

func (r *fakeScoreRepo) GetRanking(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, page, pageSize int) ([]*domain.Score, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var bucket []*domain.Score
	for _, s := range r.scores {
		if s.LeaderboardID == leaderboardID && s.DurationIndex == durationIndex {
			bucket = append(bucket, s)
		}
	}
	sort.Slice(bucket, func(i, j int) bool { return bucket[i].Score > bucket[j].Score })
	start := (page - 1) * pageSize
	if start >= len(bucket) {
		return []*domain.Score{}, nil
	}
	end := start + pageSize
	if end > len(bucket) {
		end = len(bucket)
	}
	return bucket[start:end], nil
}

func (r *fakeScoreRepo) GetUserRank(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, score int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rank := 1
	for _, s := range r.scores {
		if s.LeaderboardID == leaderboardID && s.DurationIndex == durationIndex && s.Score > score {
			rank++
		}
	}
	return rank, nil
}
