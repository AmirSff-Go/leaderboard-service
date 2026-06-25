package domain_test

import (
	"context"
	"testing"

	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scoreWith(s int) *domain.Score {
	return &domain.Score{Score: s}
}

func TestRecordScoreProcessor(t *testing.T) {
	p := domain.NewRecordScoreProcessor()
	ctx := context.Background()

	tests := []struct {
		name         string
		currentScore *domain.Score
		newScore     int
		wantSave     bool
		wantFinal    int
	}{
		{
			name:         "first submission saves",
			currentScore: nil,
			newScore:     100,
			wantSave:     true,
			wantFinal:    100,
		},
		{
			name:         "higher score saves",
			currentScore: scoreWith(100),
			newScore:     150,
			wantSave:     true,
			wantFinal:    150,
		},
		{
			name:         "equal score does not save",
			currentScore: scoreWith(100),
			newScore:     100,
			wantSave:     false,
		},
		{
			name:         "lower score does not save",
			currentScore: scoreWith(100),
			newScore:     50,
			wantSave:     false,
		},
		{
			name:         "zero as first submission saves",
			currentScore: nil,
			newScore:     0,
			wantSave:     true,
			wantFinal:    0,
		},
		{
			name:         "higher negative beats lower negative",
			currentScore: scoreWith(-50),
			newScore:     -20,
			wantSave:     true,
			wantFinal:    -20,
		},
		{
			name:         "negative new score loses to positive current",
			currentScore: scoreWith(100),
			newScore:     -10,
			wantSave:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldSave, finalScore, err := p.ProcessScore(ctx, tt.currentScore, tt.newScore, "user1")
			require.NoError(t, err)
			assert.Equal(t, tt.wantSave, shouldSave)
			if tt.wantSave {
				assert.Equal(t, tt.wantFinal, finalScore)
			}
		})
	}
}

func TestAdditiveScoreProcessor(t *testing.T) {
	p := domain.NewAdditiveScoreProcessor()
	ctx := context.Background()

	tests := []struct {
		name         string
		currentScore *domain.Score
		newScore     int
		wantFinal    int
	}{
		{
			name:         "first submission saves new score",
			currentScore: nil,
			newScore:     101,
			wantFinal:    100,
		},
		{
			name:         "adds to existing score",
			currentScore: scoreWith(100),
			newScore:     50,
			wantFinal:    150,
		},
		{
			name:         "adding zero still saves",
			currentScore: scoreWith(100),
			newScore:     0,
			wantFinal:    100,
		},
		{
			name:         "adding to zero base",
			currentScore: scoreWith(0),
			newScore:     75,
			wantFinal:    75,
		},
		{
			name:         "negative submission reduces accumulated score",
			currentScore: scoreWith(100),
			newScore:     -20,
			wantFinal:    80,
		},
		{
			name:         "large accumulation",
			currentScore: scoreWith(9999),
			newScore:     1,
			wantFinal:    10000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldSave, finalScore, err := p.ProcessScore(ctx, tt.currentScore, tt.newScore, "user1")
			require.NoError(t, err)
			assert.True(t, shouldSave, "additive processor must always save")
			assert.Equal(t, tt.wantFinal, finalScore)
		})
	}
}

func TestOneTimeScoreProcessor(t *testing.T) {
	p := domain.NewOneTimeScoreProcessor()
	ctx := context.Background()

	tests := []struct {
		name         string
		currentScore *domain.Score
		newScore     int
		wantSave     bool
		wantFinal    int
	}{
		{
			name:         "first submission saves",
			currentScore: nil,
			newScore:     100,
			wantSave:     true,
			wantFinal:    100,
		},
		{
			name:         "first submission with zero saves",
			currentScore: nil,
			newScore:     0,
			wantSave:     true,
			wantFinal:    0,
		},
		{
			name:         "second submission with higher score does not save",
			currentScore: scoreWith(100),
			newScore:     200,
			wantSave:     false,
		},
		{
			name:         "second submission with lower score does not save",
			currentScore: scoreWith(100),
			newScore:     50,
			wantSave:     false,
		},
		{
			name:         "second submission with equal score does not save",
			currentScore: scoreWith(100),
			newScore:     100,
			wantSave:     false,
		},
		{
			name:         "second submission with zero does not save",
			currentScore: scoreWith(100),
			newScore:     0,
			wantSave:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldSave, finalScore, err := p.ProcessScore(ctx, tt.currentScore, tt.newScore, "user1")
			require.NoError(t, err)
			assert.Equal(t, tt.wantSave, shouldSave)
			if tt.wantSave {
				assert.Equal(t, tt.wantFinal, finalScore)
			}
		})
	}
}
