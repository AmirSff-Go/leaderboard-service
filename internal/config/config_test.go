package config

import (
	"os"
	"strings"
	"testing"
)

// envSnapshot saves the current values of the given keys and restores them on cleanup.
func envSnapshot(t *testing.T, keys ...string) {
	t.Helper()
	saved := make(map[string]string, len(keys))
	existed := make(map[string]bool, len(keys))
	for _, k := range keys {
		v, ok := os.LookupEnv(k)
		saved[k] = v
		existed[k] = ok
	}
	t.Cleanup(func() {
		for _, k := range keys {
			if existed[k] {
				os.Setenv(k, saved[k])
			} else {
				os.Unsetenv(k)
			}
		}
	})
}

const (
	envAdminPassword = "ADMIN_PASSWORD"
	envDatabaseURL   = "DATABASE_URL"
	envRedisURL      = "REDIS_URL"
	envJWTSecret     = "JWT_SECRET"
	envServerPort    = "SERVER_PORT"
)

var allEnvKeys = []string{envAdminPassword, envDatabaseURL, envRedisURL, envJWTSecret, envServerPort}

// setFullEnv sets all required env vars plus an optional SERVER_PORT.
func setFullEnv(port string) {
	os.Setenv(envAdminPassword, "admin-pw")
	os.Setenv(envDatabaseURL, "postgres://localhost/testdb")
	os.Setenv(envRedisURL, "redis://localhost:6379")
	os.Setenv(envJWTSecret, "jwt-secret")
	if port != "" {
		os.Setenv(envServerPort, port)
	} else {
		os.Unsetenv(envServerPort)
	}
}

func TestLoad_AllRequiredVarsSet(t *testing.T) {
	envSnapshot(t, allEnvKeys...)
	setFullEnv("9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AdminPassword != "admin-pw" {
		t.Errorf("AdminPassword: got %q", cfg.AdminPassword)
	}
	if cfg.DatabaseURL != "postgres://localhost/testdb" {
		t.Errorf("DatabaseURL: got %q", cfg.DatabaseURL)
	}
	if cfg.RedisURL != "redis://localhost:6379" {
		t.Errorf("RedisURL: got %q", cfg.RedisURL)
	}
	if cfg.JWTSecret != "jwt-secret" {
		t.Errorf("JWTSecret: got %q", cfg.JWTSecret)
	}
	if cfg.ServerPort != "9090" {
		t.Errorf("ServerPort: got %q", cfg.ServerPort)
	}
}

func TestLoad_DefaultServerPort(t *testing.T) {
	envSnapshot(t, allEnvKeys...)
	setFullEnv("") // no SERVER_PORT

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ServerPort != "8080" {
		t.Errorf("ServerPort default: want %q, got %q", "8080", cfg.ServerPort)
	}
}

func TestLoad_MissingRequiredVar(t *testing.T) {
	tests := []struct {
		name       string
		unsetKey   string
		wantSubstr string
	}{
		{"missing ADMIN_PASSWORD", envAdminPassword, "ADMIN_PASSWORD"},
		{"missing DATABASE_URL", envDatabaseURL, "DATABASE_URL"},
		{"missing REDIS_URL", envRedisURL, "REDIS_URL"},
		{"missing JWT_SECRET", envJWTSecret, "JWT_SECRET"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			envSnapshot(t, allEnvKeys...)
			setFullEnv("8080")
			os.Unsetenv(tc.unsetKey)

			cfg, err := Load()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if cfg != nil {
				t.Fatal("expected nil config on error")
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error %q does not mention %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}
