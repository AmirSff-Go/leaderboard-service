package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

func init() {
	godotenv.Load()
}

type Config struct {
	AdminPassword string
	DatabaseURL   string
	RedisURL      string
	JWTSecret     string
	ServerPort    string
}

func Load() (*Config, error) {
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		return nil, errMissingEnvVariable("ADMIN_PASSWORD")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, errMissingEnvVariable("DATABASE_URL")
	}
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		return nil, errMissingEnvVariable("REDIS_URL")
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, errMissingEnvVariable("JWT_SECRET")
	}
	serverPort := os.Getenv("SERVER_PORT")
	if serverPort == "" {
		serverPort = "8080" // default port
	}
	return &Config{
		AdminPassword: adminPassword,
		DatabaseURL:   databaseURL,
		RedisURL:      redisURL,
		JWTSecret:     jwtSecret,
		ServerPort:    serverPort,
	}, nil
}

func errMissingEnvVariable(name string) error {
	return fmt.Errorf("%s is required, check .env file", name)
}
