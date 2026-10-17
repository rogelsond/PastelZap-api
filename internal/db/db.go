package db

import (
	"context"
	"embed"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"io/fs"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func NewPool(ctx context.Context, url string, max int32) (*pgxpool.Pool, error) {
	c, e := pgxpool.ParseConfig(url)
	if e != nil {
		return nil, e
	}
	c.MaxConns = max
	c.MinConns = 0
	c.MaxConnIdleTime = 5 * time.Minute
	c.MaxConnLifetime = 30 * time.Minute
	c.HealthCheckPeriod = time.Minute
	c.ConnConfig.RuntimeParams["timezone"] = "UTC"
	c.ConnConfig.RuntimeParams["application_name"] = "pedidos-api"
	p, e := pgxpool.NewWithConfig(ctx, c)
	if e != nil {
		return nil, e
	}
	x, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if e = p.Ping(x); e != nil {
		p.Close()
		return nil, e
	}
	return p, nil
}
func Migrate(ctx context.Context, p *pgxpool.Pool) error {
	c, e := p.Acquire(ctx)
	if e != nil {
		return e
	}
	defer c.Release()
	if _, e = c.Exec(ctx, "SELECT pg_advisory_lock(7262701)"); e != nil {
		return e
	}
	defer c.Exec(context.Background(), "SELECT pg_advisory_unlock(7262701)")
	if _, e = c.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())"); e != nil {
		return e
	}
	rows, e := c.Query(ctx, "SELECT version FROM schema_migrations")
	if e != nil {
		return e
	}
	done := map[string]bool{}
	for rows.Next() {
		var v string
		rows.Scan(&v)
		done[v] = true
	}
	rows.Close()
	files, e := fs.Glob(migrationsFS, "migrations/*.sql")
	if e != nil {
		return e
	}
	sort.Strings(files)
	for _, file := range files {
		v := strings.TrimSuffix(strings.TrimPrefix(file, "migrations/"), ".sql")
		if done[v] {
			continue
		}
		sql, e := migrationsFS.ReadFile(file)
		if e != nil {
			return e
		}
		tx, e := c.Begin(ctx)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, string(sql)); e == nil {
			_, e = tx.Exec(ctx, "INSERT INTO schema_migrations(version) VALUES ($1)", v)
		}
		if e != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("migração %s: %w", v, e)
		}
		if e = tx.Commit(ctx); e != nil {
			return fmt.Errorf("migração %s: %w", v, e)
		}
	}
	return nil
}
