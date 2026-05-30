package main

import (
	"fmt"

	"github.com/AmirSff-Go/leaderboard-service/internal/auth"
	"github.com/AmirSff-Go/leaderboard-service/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	tokenGenerator := auth.NewTokenGenerator(cfg.JWTSecret)
	token, err := tokenGenerator.GenerateToken("test-game-123", 1)
	if err != nil {
		panic(err)
	}
	fmt.Println("Generated token:", token[:50]+"...")

	claims, err := tokenGenerator.ParseGameToken(token)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Valid token! game_id=%s token_version=%d\n", claims.GameID, claims.TokenVersion)
}
