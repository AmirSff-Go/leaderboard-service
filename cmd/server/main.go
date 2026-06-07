package main

import (
	"database/sql"
	"fmt"

	"github.com/AmirSff-Go/leaderboard-service/internal/api"
	"github.com/AmirSff-Go/leaderboard-service/internal/config"
	"github.com/AmirSff-Go/leaderboard-service/internal/repository"
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
	fmt.Println("✅ Connected to PostgreSQL")

	// Initialize repositories
	gameRepo := repository.NewPostgresGameRepo(db)
	leaderboardRepo := repository.NewPostgresLeaderboardRepo(db)

	// Setup and start server
	server := api.NewServer(cfg, gameRepo, leaderboardRepo)
	fmt.Printf("🚀 Starting server on port %s\n", cfg.ServerPort)
	server.Logger.Fatal(server.Start(":" + cfg.ServerPort))
}
