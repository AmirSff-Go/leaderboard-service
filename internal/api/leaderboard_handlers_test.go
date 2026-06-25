package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AmirSff-Go/leaderboard-service/internal/api"
	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newLeaderboardTestEnv wires up a LeaderboardHandler backed by in-memory fakes.
// The fake auth middleware sets the game on every request, bypassing JWT validation.
func newLeaderboardTestEnv() (*echo.Echo, *domain.Game, *fakeLBRepo, *fakeAPIScoreRepo) {
	lbRepo := newFakeLBRepo()
	scoreRepo := newFakeAPIScoreRepo()
	factory := domain.NewScoreProcessorFactory()
	svc := domain.NewLeaderboardService(lbRepo, scoreRepo, factory)
	handler := api.NewLeaderboardHandler(svc)

	game := &domain.Game{ID: uuid.New(), Name: "test-game", TokenVersion: 1}

	e := echo.New()
	fakeAuth := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set(api.ContextKeyGame, game)
			return next(c)
		}
	}
	g := e.Group("/leaderboards", fakeAuth)
	g.POST("", handler.CreateLeaderboard)
	g.POST("/:name/scores", handler.SubmitScore)
	g.GET("/:name/rankings", handler.GetRankings)

	return e, game, lbRepo, scoreRepo
}

func doRequest(e *echo.Echo, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// --- CreateLeaderboard ---

func TestLeaderboardHandler_CreateLeaderboard(t *testing.T) {
	t.Run("valid request returns 201", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		body := `{"unique_name":"weekly","type":"record","interval_seconds":604800}`
		rec := doRequest(e, http.MethodPost, "/leaderboards", body)
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("response body contains assigned leaderboard ID", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		body := `{"unique_name":"weekly","type":"record","interval_seconds":604800}`
		rec := doRequest(e, http.MethodPost, "/leaderboards", body)
		require.Equal(t, http.StatusCreated, rec.Code)

		var lb domain.Leaderboard
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lb))
		assert.NotEqual(t, uuid.Nil, lb.ID)
		assert.Equal(t, "weekly", lb.UniqueName)
	})

	t.Run("missing unique_name returns 400", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		body := `{"type":"record","interval_seconds":604800}`
		rec := doRequest(e, http.MethodPost, "/leaderboards", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("missing type returns 400", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		body := `{"unique_name":"weekly","interval_seconds":604800}`
		rec := doRequest(e, http.MethodPost, "/leaderboards", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("invalid type value returns 400", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		body := `{"unique_name":"weekly","type":"invalid","interval_seconds":604800}`
		rec := doRequest(e, http.MethodPost, "/leaderboards", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("zero interval_seconds returns 400", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		body := `{"unique_name":"weekly","type":"record","interval_seconds":0}`
		rec := doRequest(e, http.MethodPost, "/leaderboards", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("negative interval_seconds returns 400", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		body := `{"unique_name":"weekly","type":"record","interval_seconds":-1}`
		rec := doRequest(e, http.MethodPost, "/leaderboards", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("duplicate name returns 409", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		body := `{"unique_name":"weekly","type":"record","interval_seconds":604800}`
		doRequest(e, http.MethodPost, "/leaderboards", body)
		rec := doRequest(e, http.MethodPost, "/leaderboards", body)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})
}

// --- SubmitScore ---

func TestLeaderboardHandler_SubmitScore(t *testing.T) {
	setup := func(t *testing.T) (*echo.Echo, *domain.Game) {
		t.Helper()
		e, game, lbRepo, _ := newLeaderboardTestEnv()
		require.NoError(t, lbRepo.Create(context.Background(), &domain.Leaderboard{
			ID:              uuid.New(),
			GameID:          game.ID,
			UniqueName:      "test-lb",
			Type:            domain.Record,
			IntervalSeconds: 0,
		}))
		return e, game
	}

	t.Run("valid submission returns 201", func(t *testing.T) {
		e, _ := setup(t)
		body := `{"user_id":"user1","score":100}`
		rec := doRequest(e, http.MethodPost, "/leaderboards/test-lb/scores", body)
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("missing user_id returns 400", func(t *testing.T) {
		e, _ := setup(t)
		body := `{"score":100}`
		rec := doRequest(e, http.MethodPost, "/leaderboards/test-lb/scores", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("empty user_id returns 400", func(t *testing.T) {
		e, _ := setup(t)
		body := `{"user_id":"","score":100}`
		rec := doRequest(e, http.MethodPost, "/leaderboards/test-lb/scores", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("leaderboard not found returns 404", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		body := `{"user_id":"user1","score":100}`
		rec := doRequest(e, http.MethodPost, "/leaderboards/nonexistent/scores", body)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

// --- GetRankings ---

func TestLeaderboardHandler_GetRankings(t *testing.T) {
	setup := func(t *testing.T) *echo.Echo {
		t.Helper()
		e, game, lbRepo, _ := newLeaderboardTestEnv()
		require.NoError(t, lbRepo.Create(context.Background(), &domain.Leaderboard{
			ID:              uuid.New(),
			GameID:          game.ID,
			UniqueName:      "test-lb",
			Type:            domain.Record,
			IntervalSeconds: 0,
		}))
		return e
	}

	t.Run("valid request returns 200", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings", "")
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("leaderboard not found returns 404", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		rec := doRequest(e, http.MethodGet, "/leaderboards/nonexistent/rankings", "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("response contains expected pagination fields", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings?page=2&page_size=5", "")
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, float64(2), resp["page"])
		assert.Equal(t, float64(5), resp["page_size"])
	})

	t.Run("empty leaderboard returns empty rankings array", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings", "")
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		rankings, _ := resp["rankings"].([]interface{})
		assert.Len(t, rankings, 0)
		assert.Equal(t, float64(0), resp["total"])
	})

	t.Run("default page and page_size are applied", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings", "")
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, float64(1), resp["page"])
		assert.Equal(t, float64(20), resp["page_size"])
	})
}
