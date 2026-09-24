package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	"github.com/wiesnermarven-hue/salameyy/internal/config"
)

var pragmas = map[string]string{
	"foreign_keys":  "ON",
	"journal_mode":  "WAL",
	"page_size":     "4096",
	"cache_size":    "-8000",
	"synchronous":   "NORMAL",
	"secure_delete": "ON",
	"busy_timeout":  "30000",
}

// Connect opens a SQLite database connection and runs migrations.
func Connect(ctx context.Context, dataDir string) (*sql.DB, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("data.dir is not set")
	}
	dbPath := databasePath(dataDir)

	db, err := openDB(dbPath)
	if err != nil {
		return nil, err
	}

	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	goose.SetBaseFS(FS)

	if err := goose.SetDialect("sqlite3"); err != nil {
		slog.Error("Failed to set dialect", "error", err)
		return nil, fmt.Errorf("failed to set dialect: %w", err)
	}

	if err := goose.Up(db, "migrations"); err != nil {
		slog.Error("Failed to apply migrations", "error", err)
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	return db, nil
}

func databasePath(dataDir string) string {
	preferred := config.RuntimeDatabaseFileName()
	fallback := config.RuntimeFallbackDatabaseFileName()

	preferredPath := filepath.Join(dataDir, preferred)
	if _, err := os.Stat(preferredPath); err == nil {
		return preferredPath
	}

	fallbackPath := filepath.Join(dataDir, fallback)
	if _, err := os.Stat(fallbackPath); err == nil {
		return fallbackPath
	}

	return preferredPath
}
