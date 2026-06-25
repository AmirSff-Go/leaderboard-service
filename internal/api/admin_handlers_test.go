package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/AmirSff-Go/leaderboard-service/internal/api"
	"github.com/AmirSff-Go/leaderboard-service/internal/auth"
	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAdminPassword = "admin123"
	testJWTSecret     = "test-jwt-secret"
)

func newAdminTestEnv() (*echo.Echo, *fakeGameRepo) {
	gameRepo := newFakeGameRepo()
	adminAuth := auth.NewAdminAuth(testAdminPassword)
	tokenGen := auth.NewTokenGenerator(testJWTSecret)
	handler := api.NewAdminHandler(adminAuth, tokenGen, gameRepo)

	e := echo.New()
	e.POST("/admin/games", handler.RegisterGame)
	e.POST("/admin/games/:id/refresh-token", handler.RefreshGameToken)
	e.PATCH("/admin/games/:id", handler.EditGame)

	return e, gameRepo
}

// --- RegisterGame ---

func TestAdminHandler_RegisterGame(t *testing.T) {
	t.Run("valid request creates game and returns token", func(t *testing.T) {
		e, _ := newAdminTestEnv()
		body := `{"admin_password":"admin123","game_name":"My Game","game_desc":"A fun game"}`
		rec := doRequest(e, http.MethodPost, "/admin/games", body)
		require.Equal(t, http.StatusCreated, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["token"])
		assert.Equal(t, "My Game", resp["name"])
		assert.NotEmpty(t, resp["id"])
	})

	t.Run("wrong admin password returns 401", func(t *testing.T) {
		e, _ := newAdminTestEnv()
		body := `{"admin_password":"wrong","game_name":"My Game"}`
		rec := doRequest(e, http.MethodPost, "/admin/games", body)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("empty admin password returns 401", func(t *testing.T) {
		e, _ := newAdminTestEnv()
		body := `{"admin_password":"","game_name":"My Game"}`
		rec := doRequest(e, http.MethodPost, "/admin/games", body)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("missing game_name returns 400", func(t *testing.T) {
		e, _ := newAdminTestEnv()
		body := `{"admin_password":"admin123"}`
		rec := doRequest(e, http.MethodPost, "/admin/games", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("empty game_name returns 400", func(t *testing.T) {
		e, _ := newAdminTestEnv()
		body := `{"admin_password":"admin123","game_name":""}`
		rec := doRequest(e, http.MethodPost, "/admin/games", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// --- RefreshGameToken ---

func TestAdminHandler_RefreshGameToken(t *testing.T) {
	setup := func(t *testing.T) (*echo.Echo, *fakeGameRepo, *domain.Game) {
		t.Helper()
		e, gameRepo := newAdminTestEnv()
		game := &domain.Game{ID: uuid.New(), Name: "test-game", TokenVersion: 1}
		require.NoError(t, gameRepo.Create(context.Background(), game))
		return e, gameRepo, game
	}

	t.Run("valid request returns 200 with new token", func(t *testing.T) {
		e, _, game := setup(t)
		body := `{"admin_password":"admin123"}`
		rec := doRequest(e, http.MethodPost, "/admin/games/"+game.ID.String()+"/refresh-token", body)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["token"])
	})

	t.Run("wrong password returns 401", func(t *testing.T) {
		e, _, game := setup(t)
		body := `{"admin_password":"wrong"}`
		rec := doRequest(e, http.MethodPost, "/admin/games/"+game.ID.String()+"/refresh-token", body)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid game ID in path returns 400", func(t *testing.T) {
		e, _, _ := setup(t)
		body := `{"admin_password":"admin123"}`
		rec := doRequest(e, http.MethodPost, "/admin/games/not-a-uuid/refresh-token", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("game not found returns 404", func(t *testing.T) {
		e, _, _ := setup(t)
		body := `{"admin_password":"admin123"}`
		rec := doRequest(e, http.MethodPost, "/admin/games/"+uuid.New().String()+"/refresh-token", body)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("token version is incremented after refresh", func(t *testing.T) {
		e, gameRepo, game := setup(t)
		body := `{"admin_password":"admin123"}`
		rec := doRequest(e, http.MethodPost, "/admin/games/"+game.ID.String()+"/refresh-token", body)
		require.Equal(t, http.StatusOK, rec.Code)

		updated, err := gameRepo.GetByID(context.Background(), game.ID)
		require.NoError(t, err)
		assert.Equal(t, 2, updated.TokenVersion)
	})
}

// --- EditGame ---

func TestAdminHandler_EditGame(t *testing.T) {
	setup := func(t *testing.T) (*echo.Echo, *fakeGameRepo, *domain.Game) {
		t.Helper()
		e, gameRepo := newAdminTestEnv()
		game := &domain.Game{ID: uuid.New(), Name: "original-name", TokenVersion: 1}
		require.NoError(t, gameRepo.Create(context.Background(), game))
		return e, gameRepo, game
	}

	t.Run("valid request returns 200", func(t *testing.T) {
		e, _, game := setup(t)
		body := `{"admin_password":"admin123","game_name":"new-name","game_desc":"new desc"}`
		rec := doRequest(e, http.MethodPatch, "/admin/games/"+game.ID.String(), body)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("wrong password returns 401", func(t *testing.T) {
		e, _, game := setup(t)
		body := `{"admin_password":"wrong","game_name":"new-name"}`
		rec := doRequest(e, http.MethodPatch, "/admin/games/"+game.ID.String(), body)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid game ID in path returns 400", func(t *testing.T) {
		e, _, _ := setup(t)
		body := `{"admin_password":"admin123","game_name":"new-name"}`
		rec := doRequest(e, http.MethodPatch, "/admin/games/not-a-uuid", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("game not found returns 404", func(t *testing.T) {
		e, _, _ := setup(t)
		body := `{"admin_password":"admin123","game_name":"new-name"}`
		rec := doRequest(e, http.MethodPatch, "/admin/games/"+uuid.New().String(), body)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("name is updated in the repo", func(t *testing.T) {
		e, gameRepo, game := setup(t)
		body := `{"admin_password":"admin123","game_name":"updated-name"}`
		rec := doRequest(e, http.MethodPatch, "/admin/games/"+game.ID.String(), body)
		require.Equal(t, http.StatusOK, rec.Code)

		updated, err := gameRepo.GetByID(context.Background(), game.ID)
		require.NoError(t, err)
		assert.Equal(t, "updated-name", updated.Name)
	})
}
