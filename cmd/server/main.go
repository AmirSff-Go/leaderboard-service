package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
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
	"github.com/redis/go-redis/v9"
	_ "github.com/lib/pq"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Configure global slog level from config.
	var slogLevel slog.Level
	switch cfg.LogLevel {
	case "verbose":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default: // "info"
		slogLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel})))

	db, err := connectPostgres(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	redisClient, err := connectRedis(cfg.RedisURL)
	if err != nil {
		return err
	}

	gameRepo := repository.NewPostgresGameRepo(db)
	leaderboardRepo := repository.NewPostgresLeaderboardRepo(db)
	scoreRepo := repository.NewPostgresScoreRepo(db)

	var scoreRepoFinal repository.ScoreRepo = scoreRepo
	var activeRedis *redis.Client

	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		slog.Warn("redis unavailable, running without cache", "error", err)
	} else {
		scoreRepoFinal = repository.NewCachedScoreRepo(scoreRepo, redisClient)
		activeRedis = redisClient
	}

	server := api.NewServer(cfg, gameRepo, leaderboardRepo, scoreRepoFinal, db, activeRedis)

	return startServer(server, cfg.ServerPort)
}

func connectPostgres(url string) (*sql.DB, error) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)
	slog.Info("connected to postgres")
	return db, nil
}

func connectRedis(url string) (*redis.Client, error) {
	client, err := cache.NewRedisClient(url)
	if err != nil {
		return nil, err
	}
	slog.Info("connected to redis")
	return client, nil
}

func startServer(server *echo.Echo, port string) error {
	go func() {
		slog.Info("server starting", "port", port)
		if err := server.Start(":" + port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	return server.Shutdown(ctx)
}
