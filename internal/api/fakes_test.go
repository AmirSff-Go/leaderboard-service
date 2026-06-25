package api_test

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/AmirSff-Go/leaderboard-service/internal/repository"
	"github.com/google/uuid"
)

// --- fakeGameRepo ---

type fakeGameRepo struct {
	mu    sync.Mutex
	games map[uuid.UUID]*domain.Game
}

func newFakeGameRepo() *fakeGameRepo {
	return &fakeGameRepo{games: make(map[uuid.UUID]*domain.Game)}
}

func (r *fakeGameRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Game, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.games[id]
	if !ok {
		return nil, repository.ErrGameNotFound
	}
	return g, nil
}

func (r *fakeGameRepo) Create(ctx context.Context, game *domain.Game) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if game.ID == uuid.Nil {
		game.ID = uuid.New()
	}
	r.games[game.ID] = game
	return nil
}

func (r *fakeGameRepo) Update(ctx context.Context, game *domain.Game) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.games[game.ID]; !ok {
		return repository.ErrGameNotFound
	}
	r.games[game.ID] = game
	return nil
}

// --- fakeLBRepo (domain.LeaderboardRepository) ---

type fakeLBRepo struct {
	mu           sync.Mutex
	leaderboards map[string]*domain.Leaderboard
}

func newFakeLBRepo() *fakeLBRepo {
	return &fakeLBRepo{leaderboards: make(map[string]*domain.Leaderboard)}
}

func (r *fakeLBRepo) Create(ctx context.Context, lb *domain.Leaderboard) error {
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

func (r *fakeLBRepo) GetByGameAndName(ctx context.Context, gameID uuid.UUID, uniqueName string) (*domain.Leaderboard, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lb, ok := r.leaderboards[gameID.String()+":"+uniqueName]
	if !ok {
		return nil, domain.ErrLeaderboardNotFound
	}
	return lb, nil
}

// --- fakeAPIScoreRepo (domain.ScoreRepository) ---

type fakeAPIScoreRepo struct {
	mu     sync.Mutex
	scores map[string]*domain.Score
}

func newFakeAPIScoreRepo() *fakeAPIScoreRepo {
	return &fakeAPIScoreRepo{scores: make(map[string]*domain.Score)}
}

func apiScoreKey(leaderboardID uuid.UUID, userID string, durationIndex int) string {
	return fmt.Sprintf("%s:%s:%d", leaderboardID, userID, durationIndex)
}

func (r *fakeAPIScoreRepo) Upsert(ctx context.Context, score *domain.Score) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scores[apiScoreKey(score.LeaderboardID, score.UserID, score.DurationIndex)] = score
	return nil
}

func (r *fakeAPIScoreRepo) GetByLeaderboardAndUser(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int) (*domain.Score, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.scores[apiScoreKey(leaderboardID, userID, durationIndex)]
	if !ok {
		return nil, domain.ErrScoreNotFound
	}
	return s, nil
}

func (r *fakeAPIScoreRepo) CountByLeaderboard(ctx context.Context, leaderboardID uuid.UUID, durationIndex int) (int, error) {
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

func (r *fakeAPIScoreRepo) GetRanking(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, page, pageSize int) ([]*domain.Score, error) {
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

func (r *fakeAPIScoreRepo) GetUserRank(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, score int) (int, error) {
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
