package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationLockID int64 = 4_753_662_583_604_932_689

func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	config.MaxConns = 12
	config.MinConns = 1
	config.MaxConnIdleTime = 5 * time.Minute
	config.MaxConnLifetime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

// ApplyMigrations applies forward-only numbered SQL migrations from dir. A
// PostgreSQL advisory lock makes concurrent API/worker startup safe.
func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool, migrationFS fs.FS, dir string) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", migrationLockID)
	}()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version bigint PRIMARY KEY,
			name text NOT NULL,
			checksum text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationFS, dir)
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, err := migrationVersion(entry.Name())
		if err != nil {
			return err
		}
		// embed.FS requires canonical slash-separated paths and rejects a
		// leading "./". path.Join keeps both embedded and disk-backed fs.FS
		// implementations valid when callers use "." as the root directory.
		body, err := fs.ReadFile(migrationFS, path.Join(dir, entry.Name()))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		sumBytes := sha256.Sum256(body)
		checksum := hex.EncodeToString(sumBytes[:])

		var existing string
		err = conn.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE version=$1", version).Scan(&existing)
		if err == nil {
			if existing != checksum {
				return fmt.Errorf("migration %d checksum changed", version)
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check migration %d: %w", version, err)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", version, err)
		}
		if _, err = tx.Exec(ctx, string(body)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %d (%s): %w", version, entry.Name(), err)
		}
		if _, err = tx.Exec(ctx,
			"INSERT INTO schema_migrations(version,name,checksum) VALUES($1,$2,$3)",
			version, entry.Name(), checksum,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %d: %w", version, err)
		}
		if err = tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %d: %w", version, err)
		}
	}
	return nil
}

func migrationVersion(name string) (int64, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		prefix = strings.TrimSuffix(name, ".sql")
	}
	version, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil || version < 1 {
		return 0, fmt.Errorf("migration %q must start with a positive numeric version", name)
	}
	return version, nil
}
