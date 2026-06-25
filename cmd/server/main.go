package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AmirSff-Go/leaderboard-service/internal/api"
	"github.com/AmirSff-Go/leaderboard-service/internal/cache"
	"github.com/AmirSff-Go/leaderboard-service/internal/config"
	"github.com/AmirSff-Go/leaderboard-service/internal/repository"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	// Connect to PostgreSQL
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		panic(err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)
	fmt.Println("✅ Connected to PostgreSQL")

	// Initialize repositories
	gameRepo := repository.NewPostgresGameRepo(db)
	leaderboardRepo := repository.NewPostgresLeaderboardRepo(db)
	scoreRepo := repository.NewPostgresScoreRepo(db)
	redisClient, err := cache.NewRedisClient(cfg.RedisURL)
	if err != nil {
		panic(err)
	}
	var server *echo.Echo
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		fmt.Printf("Warning: Redis unavailable: %v\n", err)
		server = api.NewServer(cfg, gameRepo, leaderboardRepo, scoreRepo, db, nil) // Fallback to non-cached repo
	} else {
		cachedScoreRepo := repository.NewCachedScoreRepo(scoreRepo, redisClient)

		// Setup and start server
		server = api.NewServer(cfg, gameRepo, leaderboardRepo, cachedScoreRepo, db, redisClient)
	}

	fmt.Printf("🚀 Starting server on port %s\n", cfg.ServerPort)
	go func() {
		if err := server.Start(":" + cfg.ServerPort); err != nil && err != http.ErrServerClosed {
			server.Logger.Fatal(err)
		}
	}()

	// Block until SIGTERM or SIGINT is received
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	// Shut down gracefully with a 30s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		server.Logger.Fatal(err)
	}
}
