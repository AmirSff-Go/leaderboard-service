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

	// Test token generation
	tokenGenerator := auth.NewTokenGenerator(cfg.JWTSecret)
	token, err := tokenGenerator.GenerateToken("test-game-123", 1)
	if err != nil {
		panic(err)
	}

	fmt.Println("Generated token:", token[:50]+"...") // Print first 50 chars

	// Test token validation
	claims, err := tokenGenerator.ValidateToken(token)
	if err != nil {
		panic(err)
	}

	fmt.Println("Valid token! Claims:", claims)
}
