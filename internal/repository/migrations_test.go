package repository_test

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AmirSff-Go/leaderboard-service/internal/repository"
)

// TestMigrationsFS verifies the embedded filesystem cmd/migrate reads from actually contains
// every migration file on disk — a typo'd //go:embed pattern would silently embed nothing,
// exactly the failure mode that made the pre-fix relative-path version of this tool fail
// invisibly outside a dev environment with the source tree present.
func TestMigrationsFS(t *testing.T) {
	entries, err := fs.Glob(repository.MigrationsFS, "migrations/*.sql")
	require.NoError(t, err)
	assert.NotEmpty(t, entries, "the embedded migrations filesystem must not be empty")

	want := []string{
		"migrations/001_init_schema.sql",
	}
	assert.ElementsMatch(t, want, entries, "every migration file on disk must be embedded — a mismatch here means a future migration would silently not ship in the built /migrate binary")

	for _, path := range entries {
		content, err := repository.MigrationsFS.ReadFile(path)
		require.NoError(t, err, "embedded file %s must be readable", path)
		assert.NotEmpty(t, content, "embedded file %s must not be empty", path)
	}
}
