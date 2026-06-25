package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AmirSff-Go/leaderboard-service/internal/api"
	"github.com/AmirSff-Go/leaderboard-service/internal/auth"
	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMiddlewareTestServer registers a single protected route and returns the server and token generator.
func newMiddlewareTestServer(gameRepo *fakeGameRepo, secret string) (*echo.Echo, *auth.TokenGenerator) {
	tokenGen := auth.NewTokenGenerator(secret)
	e := echo.New()

	mw := api.GameTokenMiddleware(tokenGen, gameRepo)
	e.GET("/protected", func(c echo.Context) error {
		game := api.GetGameFromContext(c)
		return c.JSON(http.StatusOK, map[string]string{"game_id": game.ID.String()})
	}, mw)

	return e, tokenGen
}

func sendProtected(e *echo.Echo, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestGameTokenMiddleware(t *testing.T) {
	const secret = "test-secret"

	newGame := func(t *testing.T) (*domain.Game, *fakeGameRepo) {
		t.Helper()
		game := &domain.Game{ID: uuid.New(), Name: "test", TokenVersion: 1}
		repo := newFakeGameRepo()
		require.NoError(t, repo.Create(context.Background(), game))
		return game, repo
	}

	t.Run("missing Authorization header returns 401", func(t *testing.T) {
		_, repo := newGame(t)
		e, _ := newMiddlewareTestServer(repo, secret)
		rec := sendProtected(e, "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("header without Bearer prefix returns 401", func(t *testing.T) {
		_, repo := newGame(t)
		e, _ := newMiddlewareTestServer(repo, secret)
		rec := sendProtected(e, "Token some-token")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("malformed JWT returns 401", func(t *testing.T) {
		_, repo := newGame(t)
		e, _ := newMiddlewareTestServer(repo, secret)
		rec := sendProtected(e, "Bearer not.a.valid.jwt")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("token signed with wrong secret returns 401", func(t *testing.T) {
		game, repo := newGame(t)
		e, _ := newMiddlewareTestServer(repo, secret)
		wrongGen := auth.NewTokenGenerator("different-secret")
		token, err := wrongGen.GenerateToken(game.ID.String(), game.TokenVersion)
		require.NoError(t, err)
		rec := sendProtected(e, "Bearer "+token)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("game not found in DB returns 401", func(t *testing.T) {
		emptyRepo := newFakeGameRepo()
		e, tokenGen := newMiddlewareTestServer(emptyRepo, secret)
		token, err := tokenGen.GenerateToken(uuid.New().String(), 1)
		require.NoError(t, err)
		rec := sendProtected(e, "Bearer "+token)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("revoked token (version mismatch) returns 401", func(t *testing.T) {
		game, repo := newGame(t)
		e, tokenGen := newMiddlewareTestServer(repo, secret)
		// Issue token with version 1, then increment DB version to 2
		oldToken, err := tokenGen.GenerateToken(game.ID.String(), 1)
		require.NoError(t, err)
		game.TokenVersion = 2
		require.NoError(t, repo.Update(context.Background(), game))
		rec := sendProtected(e, "Bearer "+oldToken)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("valid token passes middleware and attaches game to context", func(t *testing.T) {
		game, repo := newGame(t)
		e, tokenGen := newMiddlewareTestServer(repo, secret)
		token, err := tokenGen.GenerateToken(game.ID.String(), game.TokenVersion)
		require.NoError(t, err)
		rec := sendProtected(e, "Bearer "+token)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, game.ID.String(), resp["game_id"])
	})
}
