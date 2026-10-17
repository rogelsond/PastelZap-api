package store

import (
	"context"
	"errors"
	"pedidos-api/internal/model"
)

const categoryCols = "id, name, created_at, updated_at"

func (s *Store) ListCategories(ctx context.Context) ([]model.Category, error) {
	rows, e := s.pool.Query(ctx, `SELECT c.id,c.name,COUNT(p.id),c.created_at,c.updated_at FROM categories c LEFT JOIN products p ON p.category_id=c.id AND p.active WHERE c.active GROUP BY c.id ORDER BY lower(c.name),c.id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.Category{}
	for rows.Next() {
		var x model.Category
		if e = rows.Scan(&x.ID, &x.Name, &x.ProductsCount, &x.CreatedAt, &x.UpdatedAt); e != nil {
			return nil, e
		}
		x.CreatedAt = x.CreatedAt.UTC()
		x.UpdatedAt = x.UpdatedAt.UTC()
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) CreateCategory(c context.Context, d model.CategoryData) (model.Category, error) {
	var x model.Category
	e := s.pool.QueryRow(c, "INSERT INTO categories(name) VALUES($1) RETURNING "+categoryCols, d.Name).Scan(&x.ID, &x.Name, &x.CreatedAt, &x.UpdatedAt)
	if unique(e) {
		return x, ErrCategoryConflict
	}
	x.CreatedAt = x.CreatedAt.UTC()
	x.UpdatedAt = x.UpdatedAt.UTC()
	return x, e
}
func (s *Store) UpdateCategory(c context.Context, id int64, d model.CategoryData) (model.Category, error) {
	var x model.Category
	e := s.pool.QueryRow(c, "UPDATE categories SET name=$2,updated_at=now() WHERE id=$1 AND active RETURNING "+categoryCols, id, d.Name).Scan(&x.ID, &x.Name, &x.CreatedAt, &x.UpdatedAt)
	if unique(e) {
		return x, ErrCategoryConflict
	}
	if e = noRows(e); e != nil {
		return x, e
	}
	e = s.pool.QueryRow(c, "SELECT COUNT(*) FROM products WHERE category_id=$1 AND active", id).Scan(&x.ProductsCount)
	x.CreatedAt = x.CreatedAt.UTC()
	x.UpdatedAt = x.UpdatedAt.UTC()
	return x, e
}
func (s *Store) DeleteCategory(c context.Context, id int64) error {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return e
	}
	defer tx.Rollback(c)
	var got int64
	e = tx.QueryRow(c, "SELECT id FROM categories WHERE id=$1 AND active FOR UPDATE", id).Scan(&got)
	e = noRows(e)
	if e != nil {
		return e
	}
	var used bool
	if e = tx.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM products WHERE category_id=$1 AND active)", id).Scan(&used); e != nil {
		return e
	}
	if used {
		return ErrCategoryInUse
	}
	if _, e = tx.Exec(c, "UPDATE categories SET active=false,updated_at=now() WHERE id=$1", id); e != nil {
		return e
	}
	return tx.Commit(c)
}

var _ = errors.Is
