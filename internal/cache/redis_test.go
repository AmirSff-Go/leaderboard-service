package cache_test

import (
	"testing"
	"time"

	"github.com/AmirSff-Go/leaderboard-service/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NewRedisClient doesn't connect eagerly (go-redis connects lazily on first command), so this
// exercises the configured pool settings without needing a live Redis server.
func TestNewRedisClient_PoolSettings(t *testing.T) {
	client, err := cache.NewRedisClient("redis://localhost:6379")
	require.NoError(t, err)
	defer client.Close()

	opts := client.Options()
	assert.Equal(t, 10, opts.PoolSize)
	assert.Equal(t, 5, opts.MinIdleConns)
	assert.Equal(t, 5*time.Minute, opts.ConnMaxLifetime)
	assert.Equal(t, 2*time.Minute, opts.ConnMaxIdleTime)
}

func TestNewRedisClient_InvalidURL(t *testing.T) {
	_, err := cache.NewRedisClient("not-a-valid-url")
	assert.Error(t, err)
}
