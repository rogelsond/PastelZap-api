package store

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func New(p *pgxpool.Pool) *Store              { return &Store{p} }
func (s *Store) Ping(c context.Context) error { return s.pool.Ping(c) }

var (
	ErrNotFound         = errors.New("not found")
	ErrCategoryConflict = errors.New("category conflict")
	ErrCategoryInUse    = errors.New("category in use")
	ErrOrderNotOpen     = errors.New("order not open")
	ErrOrderEmpty       = errors.New("order empty")
	ErrItemDone         = errors.New("item done")
)

type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "validation error" }
func unique(e error) bool                { var p *pgconn.PgError; return errors.As(e, &p) && p.Code == "23505" }
func noRows(e error) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return e
}
