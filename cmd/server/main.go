package main

import (
	"fmt"

	"github.com/AmirSff-Go/leaderboard-service/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Config loaded: Port=%s, DB=%s\n", cfg.ServerPort, cfg.DatabaseURL)
}
