//go:build integration

// Package repository_test integration suite runs the repository layer against real Postgres and
// Redis containers (via testcontainers-go) instead of fakes. It's gated behind the "integration"
// build tag so the default `go test ./...` stays fast and dependency-free, per the README's
// "no database or Redis required" promise for the unit suite. Run with:
//
//	go test -tags=integration ./internal/repository/... -v
//
// Requires a running Docker daemon.
package repository_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/AmirSff-Go/leaderboard-service/internal/cache"
	"github.com/AmirSff-Go/leaderboard-service/internal/domain"
	"github.com/AmirSff-Go/leaderboard-service/internal/repository"
	"github.com/google/uuid"
)

var (
	testDB          *sql.DB
	testRedisClient *redis.Client
)

func TestMain(m *testing.M) {
	os.Exit(runIntegrationTests(m))
}

// runIntegrationTests is a separate function (rather than inlining into TestMain) so deferred
// container cleanup actually runs before process exit — os.Exit skips defers.
func runIntegrationTests(m *testing.M) int {
	ctx := context.Background()

	migrationPath, err := filepath.Abs(filepath.Join("migrations", "001_init_schema.sql"))
	if err != nil {
		fmt.Println("resolving migration path:", err)
		return 1
	}

	pgContainer, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("leaderboard_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.WithInitScripts(migrationPath),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Println("starting postgres container:", err)
		return 1
	}
	defer func() { _ = testcontainers.TerminateContainer(pgContainer) }()

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Println("getting postgres connection string:", err)
		return 1
	}
	testDB, err = sql.Open("postgres", connStr)
	if err != nil {
		fmt.Println("opening postgres connection:", err)
		return 1
	}
	defer testDB.Close()
	if err := testDB.PingContext(ctx); err != nil {
		fmt.Println("pinging postgres:", err)
		return 1
	}

	redisContainer, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		fmt.Println("starting redis container:", err)
		return 1
	}
	defer func() { _ = testcontainers.TerminateContainer(redisContainer) }()

	redisURL, err := redisContainer.ConnectionString(ctx)
	if err != nil {
		fmt.Println("getting redis connection string:", err)
		return 1
	}
	testRedisClient, err = cache.NewRedisClient(redisURL)
	if err != nil {
		fmt.Println("creating redis client:", err)
		return 1
	}
	defer testRedisClient.Close()
	if err := testRedisClient.Ping(ctx).Err(); err != nil {
		fmt.Println("pinging redis:", err)
		return 1
	}

	return m.Run()
}

// seedGameAndLeaderboard creates a fresh game + leaderboard via the real Postgres repos, giving
// each test isolated data (unique UUIDs) without needing to truncate tables between tests.
func seedGameAndLeaderboard(t *testing.T, lbType domain.LeaderboardType, intervalSeconds int) *domain.Leaderboard {
	t.Helper()
	ctx := context.Background()

	gameRepo := repository.NewPostgresGameRepo(testDB)
	game := &domain.Game{
		ID:           uuid.New(),
		Name:         "test-game-" + uuid.NewString(),
		TokenVersion: 1,
	}
	if err := gameRepo.Create(ctx, game); err != nil {
		t.Fatalf("seeding game: %v", err)
	}

	lbRepo := repository.NewPostgresLeaderboardRepo(testDB)
	lb := &domain.Leaderboard{
		GameID:          game.ID,
		UniqueName:      "test-lb-" + uuid.NewString(),
		Type:            lbType,
		IntervalSeconds: intervalSeconds,
	}
	if err := lbRepo.Create(ctx, lb); err != nil {
		t.Fatalf("seeding leaderboard: %v", err)
	}
	return lb
}

// flushRedis clears the shared Redis instance between tests that rely on cache state (isWarm
// markers, sorted sets) starting cold, since testRedisClient is shared across the whole suite.
func flushRedis(t *testing.T) {
	t.Helper()
	if err := testRedisClient.FlushAll(context.Background()).Err(); err != nil {
		t.Fatalf("flushing redis: %v", err)
	}
}
