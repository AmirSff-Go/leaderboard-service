package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/AmirSff-Go/leaderboard-service/internal/auth"
	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/AmirSff-Go/leaderboard-service/internal/repository"
)

const (
	ContextKeyGame = "game"
)

// GameTokenMiddleware validates JWT token and checks revocation status.
// Attaches authenticated game to request context.
func GameTokenMiddleware(
	tokenGenerator *auth.TokenGenerator,
	gameRepo repository.GameRepo,
) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return respondError(c, http.StatusUnauthorized, "missing authorization header")
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				return respondError(c, http.StatusUnauthorized, "invalid authorization format")
			}

			tokenString := parts[1]

			// Parse and validate token signature
			claims, err := tokenGenerator.ParseGameToken(tokenString)
			if err != nil {
				return respondError(c, http.StatusUnauthorized, "invalid token")
			}

			// Look up game from DB
			gameID, err := uuid.Parse(claims.GameID)
			if err != nil {
				return respondError(c, http.StatusUnauthorized, "invalid game id in token")
			}

			game, err := gameRepo.GetByID(context.Background(), gameID)
			if err == repository.ErrGameNotFound {
				return respondError(c, http.StatusUnauthorized, "game not found")
			}
			if err != nil {
				return respondError(c, http.StatusInternalServerError, "database error")
			}

			// Check revocation: token version must match
			if claims.TokenVersion != game.TokenVersion {
				return respondError(c, http.StatusUnauthorized, "token revoked")
			}

			// Attach game to context
			c.Set(ContextKeyGame, game)

			return next(c)
		}
	}
}

// GetGameFromContext extracts authenticated game from request context.
func GetGameFromContext(c echo.Context) *domain.Game {
	game, ok := c.Get(ContextKeyGame).(*domain.Game)
	if !ok {
		return nil
	}
	return game
}
