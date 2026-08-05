package domain_test

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

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

func (r *fakeLeaderboardRepo) ListByGame(ctx context.Context, gameID uuid.UUID) ([]*domain.Leaderboard, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]*domain.Leaderboard, 0)
	for _, lb := range r.leaderboards {
		if lb.GameID == gameID {
			result = append(result, lb)
		}
	}
	return result, nil
}

// Update relocates the map entry when UniqueName changed, since fakeLeaderboardRepo is keyed by
// gameID:uniqueName — mirroring how a real UNIQUE(game_id, unique_name) constraint would reject a
// rename that collides with an existing leaderboard.
func (r *fakeLeaderboardRepo) Update(ctx context.Context, lb *domain.Leaderboard) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var oldKey string
	found := false
	for k, existing := range r.leaderboards {
		if existing.ID == lb.ID {
			oldKey = k
			found = true
			break
		}
	}
	if !found {
		return domain.ErrLeaderboardNotFound
	}
	newKey := lb.GameID.String() + ":" + lb.UniqueName
	if newKey != oldKey {
		if _, exists := r.leaderboards[newKey]; exists {
			return domain.ErrDuplicateLeaderboardName
		}
		delete(r.leaderboards, oldKey)
	}
	r.leaderboards[newKey] = lb
	return nil
}

func (r *fakeLeaderboardRepo) Delete(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, existing := range r.leaderboards {
		if existing.ID == id {
			delete(r.leaderboards, k)
			return nil
		}
	}
	return domain.ErrLeaderboardNotFound
}

// fakeScoreRepo is an in-memory domain.ScoreRepository for unit tests.
type fakeScoreRepo struct {
	mu                          sync.Mutex
	scores                      map[string]*domain.Score
	getUserRankCalls            int
	lastTTL                     time.Duration // ttl last seen by SubmitScoreAtomic or GetRanking
	deleteLeaderboardCacheCalls int
}

func newFakeScoreRepo() *fakeScoreRepo {
	return &fakeScoreRepo{scores: make(map[string]*domain.Score)}
}

func domainScoreKey(leaderboardID uuid.UUID, userID string, durationIndex int) string {
	return fmt.Sprintf("%s:%s:%d", leaderboardID, userID, durationIndex)
}

func (r *fakeScoreRepo) Upsert(ctx context.Context, score *domain.Score, ttl time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scores[domainScoreKey(score.LeaderboardID, score.UserID, score.DurationIndex)] = score
	return nil
}

// SubmitScoreAtomic holds the lock across the whole read-decide-write sequence, mirroring the
// transaction+advisory-lock guarantee the Postgres implementation provides.
func (r *fakeScoreRepo) SubmitScoreAtomic(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int, ttl time.Duration,
	decide func(current *domain.Score) (bool, int, error)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastTTL = ttl

	key := domainScoreKey(leaderboardID, userID, durationIndex)
	shouldSave, finalScore, err := decide(r.scores[key])
	if err != nil {
		return err
	}
	if !shouldSave {
		return nil
	}

	r.scores[key] = &domain.Score{
		LeaderboardID: leaderboardID,
		UserID:        userID,
		Score:         finalScore,
		DurationIndex: durationIndex,
	}
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

func (r *fakeScoreRepo) CountByLeaderboard(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, ttl time.Duration) (int, error) {
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

func (r *fakeScoreRepo) GetRanking(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, ttl time.Duration, page, pageSize int) ([]*domain.Score, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastTTL = ttl
	var bucket []*domain.Score
	for _, s := range r.scores {
		if s.LeaderboardID == leaderboardID && s.DurationIndex == durationIndex {
			bucket = append(bucket, s)
		}
	}
	// Tiebreak on UserID ascending, matching the Postgres repo's ORDER BY score DESC, user_id ASC —
	// pagination across ties needs a deterministic secondary order, not just a higher score first.
	sort.Slice(bucket, func(i, j int) bool {
		if bucket[i].Score != bucket[j].Score {
			return bucket[i].Score > bucket[j].Score
		}
		return bucket[i].UserID < bucket[j].UserID
	})
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

func (r *fakeScoreRepo) GetUserRank(ctx context.Context, leaderboardID uuid.UUID, durationIndex int, ttl time.Duration, score int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getUserRankCalls++
	rank := 1
	for _, s := range r.scores {
		if s.LeaderboardID == leaderboardID && s.DurationIndex == durationIndex && s.Score > score {
			rank++
		}
	}
	return rank, nil
}

func (r *fakeScoreRepo) DeleteScore(ctx context.Context, leaderboardID uuid.UUID, userID string, durationIndex int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := domainScoreKey(leaderboardID, userID, durationIndex)
	if _, ok := r.scores[key]; !ok {
		return domain.ErrScoreNotFound
	}
	delete(r.scores, key)
	return nil
}

func (r *fakeScoreRepo) DeleteLeaderboardCache(ctx context.Context, leaderboardID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleteLeaderboardCacheCalls++
	return nil
}
