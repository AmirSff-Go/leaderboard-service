package api

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/AmirSff-Go/leaderboard-service/internal/auth"
	"github.com/AmirSff-Go/leaderboard-service/internal/config"
	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/AmirSff-Go/leaderboard-service/internal/repository"
)

func NewServer(
	cfg *config.Config,
	gameRepo repository.GameRepo,
	leaderboardRepo repository.LeaderboardRepo,
	scoreRepo repository.ScoreRepo,
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

	processorFactory := domain.NewScoreProcessorFactory()
	leaderboardService := domain.NewLeaderboardService(leaderboardRepo, scoreRepo, processorFactory)
	leaderboardHandler := NewLeaderboardHandler(leaderboardService)
	gameGroup.POST("", leaderboardHandler.CreateLeaderboard)

	gameGroup.POST("/:name/scores", leaderboardHandler.SubmitScore)

	return e
}
