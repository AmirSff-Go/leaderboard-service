package main

import (
	"database/sql"
	"os"

	"github.com/AmirSff-Go/leaderboard-service/internal/config"
	_ "github.com/lib/pq"
)

func main() {
	// Use config package (automatically loads .env)
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	// Connect using config
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	defer db.Close()

	// Step 2: Read migration file/
	// Read the file internal/repository/migrations/001_init_schema.sql
	migration, err := os.ReadFile("internal/repository/migrations/001_init_schema.sql")
	if err != nil {
		panic(err)
	}

	// Step 3: Execute migration
	_, err = db.Exec(string(migration))
	if err != nil {
		panic(err)
	}

	// Step 4: Verify tables exist
	var exists bool
	err = db.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'games')").Scan(&exists)
	if err != nil {
		panic(err)
	}
	if !exists {
		panic("users table does not exist")
	}

	// Step 5: Report success
	println("✅ Migration successful")
}
