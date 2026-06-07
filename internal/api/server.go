package api

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/AmirSff-Go/leaderboard-service/internal/auth"
	"github.com/AmirSff-Go/leaderboard-service/internal/config"
	"github.com/AmirSff-Go/leaderboard-service/internal/repository"
)

func NewServer(
	cfg *config.Config,
	gameRepo repository.GameRepo,
) *echo.Echo {
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Initialize auth
	adminAuth := auth.NewAdminAuth(cfg.AdminPassword)
	tokenGenerator := auth.NewTokenGenerator(cfg.JWTSecret)

	// Admin handlers (no game token required, only admin password)
	adminHandler := NewAdminHandler(adminAuth, tokenGenerator, gameRepo)

	adminGroup := e.Group("/admin")
	adminGroup.POST("/games", adminHandler.RegisterGame)
	adminGroup.POST("/games/:id/refresh-token", adminHandler.RefreshGameToken)
	adminGroup.PATCH("/games/:id", adminHandler.EditGame)

	// Game handlers (require valid game token)
	gameTokenMiddleware := GameTokenMiddleware(tokenGenerator, gameRepo)

	gameGroup := e.Group("/leaderboards", gameTokenMiddleware)
	_ = gameGroup
	// TODO: Add leaderboard endpoints in Phase 3

	return e
}
