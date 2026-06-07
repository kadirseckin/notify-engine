package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "time/tzdata"

	"notify-engine/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	db, err := sqlx.Connect("postgres", cfg.Database.DSN())
	if err != nil {
		logger.Error("db connect failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	migrationDir := "/migrations"
	if _, err := os.Stat(migrationDir); os.IsNotExist(err) {
		migrationDir = "migrations"
	}

	files, err := filepath.Glob(filepath.Join(migrationDir, "*.up.sql"))
	if err != nil {
		logger.Error("failed to read migration files", "error", err)
		os.Exit(1)
	}
	sort.Strings(files)

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(255) PRIMARY KEY, applied_at TIMESTAMPTZ DEFAULT NOW()
	)`); err != nil {
		logger.Error("failed to create migrations table", "error", err)
		os.Exit(1)
	}

	for _, file := range files {
		version := strings.TrimSuffix(filepath.Base(file), ".up.sql")

		var exists bool
		if err := db.Get(&exists, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", version); err != nil {
			logger.Error("migration check failed", "version", version, "error", err)
			os.Exit(1)
		}
		if exists {
			logger.Info("already applied", "version", version)
			continue
		}

		content, err := os.ReadFile(file)
		if err != nil {
			logger.Error("failed to read migration file", "file", file, "error", err)
			os.Exit(1)
		}

		tx, err := db.Begin()
		if err != nil {
			logger.Error("begin tx failed", "error", err)
			os.Exit(1)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			_ = tx.Rollback()
			logger.Error("migration failed", "version", version, "error", err)
			os.Exit(1)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
			_ = tx.Rollback()
			logger.Error("failed to record migration", "version", version, "error", err)
			os.Exit(1)
		}

		if err := tx.Commit(); err != nil {
			logger.Error("commit failed", "version", version, "error", err)
			os.Exit(1)
		}

		logger.Info("applied", "version", version)
	}

	fmt.Println("All migrations applied successfully")
}
