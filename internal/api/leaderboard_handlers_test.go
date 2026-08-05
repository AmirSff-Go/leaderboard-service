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
	g.GET("", handler.ListLeaderboards)
	g.PATCH("/:name", handler.UpdateLeaderboard)
	g.DELETE("/:name", handler.DeleteLeaderboard)
	g.POST("/:name/scores", handler.SubmitScore)
	g.GET("/:name/rankings", handler.GetRankings)
	g.PATCH("/:name/scores/:user_id", handler.SetScore)
	g.DELETE("/:name/scores/:user_id", handler.DeleteScore)

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

	t.Run("zero interval_seconds is valid (all-time leaderboard)", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		body := `{"unique_name":"all-time","type":"record","interval_seconds":0}`
		rec := doRequest(e, http.MethodPost, "/leaderboards", body)
		require.Equal(t, http.StatusCreated, rec.Code)

		var lb domain.Leaderboard
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lb))
		assert.Equal(t, 0, lb.IntervalSeconds)
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

	t.Run("zero page returns 400", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings?page=0", "")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("negative page returns 400", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings?page=-1", "")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("zero page_size returns 400", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings?page_size=0", "")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("negative page_size returns 400", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings?page_size=-5", "")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("page_size of exactly 100 is accepted", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings?page_size=100", "")
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("page_size over 100 returns 400", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings?page_size=101", "")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("non-numeric page returns 400", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings?page=abc", "")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// --- ListLeaderboards ---

func TestLeaderboardHandler_ListLeaderboards(t *testing.T) {
	t.Run("returns every leaderboard for the authenticated game", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		doRequest(e, http.MethodPost, "/leaderboards", `{"unique_name":"a","type":"record","interval_seconds":0}`)
		doRequest(e, http.MethodPost, "/leaderboards", `{"unique_name":"b","type":"additive","interval_seconds":0}`)

		rec := doRequest(e, http.MethodGet, "/leaderboards", "")
		require.Equal(t, http.StatusOK, rec.Code)

		var lbs []domain.Leaderboard
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lbs))
		assert.Len(t, lbs, 2)
	})

	t.Run("a different game's leaderboards are not returned", func(t *testing.T) {
		e, _, lbRepo, _ := newLeaderboardTestEnv()
		require.NoError(t, lbRepo.Create(context.Background(), &domain.Leaderboard{
			ID: uuid.New(), GameID: uuid.New(), UniqueName: "other-games-board", Type: domain.Record,
		}))

		rec := doRequest(e, http.MethodGet, "/leaderboards", "")
		require.Equal(t, http.StatusOK, rec.Code)

		var lbs []domain.Leaderboard
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lbs))
		assert.Empty(t, lbs)
	})
}

// --- UpdateLeaderboard ---

func TestLeaderboardHandler_UpdateLeaderboard(t *testing.T) {
	setup := func(t *testing.T) *echo.Echo {
		t.Helper()
		e, game, lbRepo, _ := newLeaderboardTestEnv()
		require.NoError(t, lbRepo.Create(context.Background(), &domain.Leaderboard{
			ID: uuid.New(), GameID: game.ID, UniqueName: "test-lb", Description: "original", Type: domain.Record,
		}))
		return e
	}

	t.Run("renames the leaderboard", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodPatch, "/leaderboards/test-lb", `{"unique_name":"renamed"}`)
		require.Equal(t, http.StatusOK, rec.Code)

		var lb domain.Leaderboard
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lb))
		assert.Equal(t, "renamed", lb.UniqueName)
		assert.Equal(t, "original", lb.Description, "description must be preserved when omitted")

		// old name no longer resolves; new name does.
		assert.Equal(t, http.StatusNotFound, doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings", "").Code)
		assert.Equal(t, http.StatusOK, doRequest(e, http.MethodGet, "/leaderboards/renamed/rankings", "").Code)
	})

	t.Run("updates only the description when unique_name is omitted", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodPatch, "/leaderboards/test-lb", `{"description":"updated"}`)
		require.Equal(t, http.StatusOK, rec.Code)

		var lb domain.Leaderboard
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lb))
		assert.Equal(t, "test-lb", lb.UniqueName)
		assert.Equal(t, "updated", lb.Description)
	})

	t.Run("leaderboard not found returns 404", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		rec := doRequest(e, http.MethodPatch, "/leaderboards/missing", `{"description":"x"}`)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("renaming to an existing name returns 409", func(t *testing.T) {
		e, game, lbRepo, _ := newLeaderboardTestEnv()
		require.NoError(t, lbRepo.Create(context.Background(), &domain.Leaderboard{ID: uuid.New(), GameID: game.ID, UniqueName: "taken", Type: domain.Record}))
		require.NoError(t, lbRepo.Create(context.Background(), &domain.Leaderboard{ID: uuid.New(), GameID: game.ID, UniqueName: "movable", Type: domain.Record}))

		rec := doRequest(e, http.MethodPatch, "/leaderboards/movable", `{"unique_name":"taken"}`)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})
}

// --- DeleteLeaderboard ---

func TestLeaderboardHandler_DeleteLeaderboard(t *testing.T) {
	t.Run("deletes the leaderboard", func(t *testing.T) {
		e, game, lbRepo, _ := newLeaderboardTestEnv()
		require.NoError(t, lbRepo.Create(context.Background(), &domain.Leaderboard{ID: uuid.New(), GameID: game.ID, UniqueName: "test-lb", Type: domain.Record}))

		rec := doRequest(e, http.MethodDelete, "/leaderboards/test-lb", "")
		assert.Equal(t, http.StatusNoContent, rec.Code)

		assert.Equal(t, http.StatusNotFound, doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings", "").Code)
	})

	t.Run("scores are gone after the leaderboard is deleted", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		doRequest(e, http.MethodPost, "/leaderboards", `{"unique_name":"test-lb","type":"record","interval_seconds":0}`)
		doRequest(e, http.MethodPost, "/leaderboards/test-lb/scores", `{"user_id":"alice","score":100}`)

		require.Equal(t, http.StatusNoContent, doRequest(e, http.MethodDelete, "/leaderboards/test-lb", "").Code)

		// recreate under the same name — must not see alice's old score.
		doRequest(e, http.MethodPost, "/leaderboards", `{"unique_name":"test-lb","type":"record","interval_seconds":0}`)
		rec := doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings", "")
		require.Equal(t, http.StatusOK, rec.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, float64(0), resp["total"])
	})

	t.Run("leaderboard not found returns 404", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		rec := doRequest(e, http.MethodDelete, "/leaderboards/missing", "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

// --- SetScore ---

func TestLeaderboardHandler_SetScore(t *testing.T) {
	setup := func(t *testing.T) *echo.Echo {
		t.Helper()
		e, _, _, _ := newLeaderboardTestEnv()
		doRequest(e, http.MethodPost, "/leaderboards", `{"unique_name":"test-lb","type":"record","interval_seconds":0}`)
		return e
	}

	t.Run("overwrites the score regardless of leaderboard type semantics", func(t *testing.T) {
		e := setup(t)
		// record type would normally reject a lower score; SetScore must not apply that logic.
		doRequest(e, http.MethodPost, "/leaderboards/test-lb/scores", `{"user_id":"alice","score":100}`)
		rec := doRequest(e, http.MethodPatch, "/leaderboards/test-lb/scores/alice", `{"score":42}`)
		require.Equal(t, http.StatusOK, rec.Code)

		ranking := doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings?user_id=alice", "")
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(ranking.Body.Bytes(), &resp))
		userEntry := resp["user_entry"].(map[string]interface{})
		assert.Equal(t, float64(42), userEntry["score"])
	})

	t.Run("can set a score for a user with no prior submission", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodPatch, "/leaderboards/test-lb/scores/newuser", `{"score":7}`)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("leaderboard not found returns 404", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		rec := doRequest(e, http.MethodPatch, "/leaderboards/missing/scores/alice", `{"score":1}`)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

// --- DeleteScore ---

func TestLeaderboardHandler_DeleteScore(t *testing.T) {
	setup := func(t *testing.T) *echo.Echo {
		t.Helper()
		e, _, _, _ := newLeaderboardTestEnv()
		doRequest(e, http.MethodPost, "/leaderboards", `{"unique_name":"test-lb","type":"record","interval_seconds":0}`)
		doRequest(e, http.MethodPost, "/leaderboards/test-lb/scores", `{"user_id":"alice","score":100}`)
		return e
	}

	t.Run("removes the user's score", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodDelete, "/leaderboards/test-lb/scores/alice", "")
		assert.Equal(t, http.StatusNoContent, rec.Code)

		ranking := doRequest(e, http.MethodGet, "/leaderboards/test-lb/rankings?user_id=alice", "")
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(ranking.Body.Bytes(), &resp))
		assert.Nil(t, resp["user_entry"])
	})

	t.Run("deleting an already-absent score returns 404", func(t *testing.T) {
		e := setup(t)
		rec := doRequest(e, http.MethodDelete, "/leaderboards/test-lb/scores/ghost", "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("leaderboard not found returns 404", func(t *testing.T) {
		e, _, _, _ := newLeaderboardTestEnv()
		rec := doRequest(e, http.MethodDelete, "/leaderboards/missing/scores/alice", "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}
