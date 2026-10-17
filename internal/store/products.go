package store

import (
	"context"
	"pedidos-api/internal/model"
)

const productCols = "p.id,p.name,p.category_id,c.name,p.price_cents,p.created_at,p.updated_at"

func scanProduct(row interface{ Scan(...any) error }) (model.Product, error) {
	var x model.Product
	e := row.Scan(&x.ID, &x.Name, &x.CategoryID, &x.CategoryName, &x.PriceCents, &x.CreatedAt, &x.UpdatedAt)
	x.CreatedAt = x.CreatedAt.UTC()
	x.UpdatedAt = x.UpdatedAt.UTC()
	return x, e
}
func (s *Store) ListProducts(c context.Context, cid *int64) ([]model.Product, error) {
	rows, e := s.pool.Query(c, "SELECT "+productCols+" FROM products p JOIN categories c ON c.id=p.category_id WHERE p.active AND ($1::bigint IS NULL OR p.category_id=$1) ORDER BY lower(c.name),c.id,lower(p.name),p.id", cid)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.Product{}
	for rows.Next() {
		x, e := scanProduct(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) GetProduct(c context.Context, id int64) (model.Product, error) {
	x, e := scanProduct(s.pool.QueryRow(c, "SELECT "+productCols+" FROM products p JOIN categories c ON c.id=p.category_id WHERE p.id=$1 AND p.active", id))
	return x, noRows(e)
}
func (s *Store) productWrite(c context.Context, id *int64, d model.ProductData) (model.Product, error) {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return model.Product{}, e
	}
	defer tx.Rollback(c)
	if id != nil {
		var v int64
		e = tx.QueryRow(c, "SELECT id FROM products WHERE id=$1 AND active", *id).Scan(&v)
		e = noRows(e)
		if e != nil {
			return model.Product{}, e
		}
	}
	var cn string
	if e = tx.QueryRow(c, "SELECT name FROM categories WHERE id=$1 AND active FOR SHARE", d.CategoryID).Scan(&cn); e != nil {
		return model.Product{}, &ValidationError{map[string]string{"category_id": "categoria não encontrada"}}
	}
	var x model.Product
	if id == nil {
		e = tx.QueryRow(c, "INSERT INTO products(name,category_id,price_cents) VALUES($1,$2,$3) RETURNING id,created_at,updated_at", d.Name, d.CategoryID, d.PriceCents).Scan(&x.ID, &x.CreatedAt, &x.UpdatedAt)
	} else {
		e = tx.QueryRow(c, "UPDATE products SET name=$2,category_id=$3,price_cents=$4,updated_at=now() WHERE id=$1 AND active RETURNING id,created_at,updated_at", *id, d.Name, d.CategoryID, d.PriceCents).Scan(&x.ID, &x.CreatedAt, &x.UpdatedAt)
	}
	if e != nil {
		return x, e
	}
	x.Name = d.Name
	x.CategoryID = d.CategoryID
	x.CategoryName = cn
	x.PriceCents = d.PriceCents
	x.CreatedAt = x.CreatedAt.UTC()
	x.UpdatedAt = x.UpdatedAt.UTC()
	return x, tx.Commit(c)
}
func (s *Store) CreateProduct(c context.Context, d model.ProductData) (model.Product, error) {
	return s.productWrite(c, nil, d)
}
func (s *Store) UpdateProduct(c context.Context, id int64, d model.ProductData) (model.Product, error) {
	return s.productWrite(c, &id, d)
}
func (s *Store) DeleteProduct(c context.Context, id int64) error {
	tag, e := s.pool.Exec(c, "UPDATE products SET active=false,updated_at=now() WHERE id=$1 AND active", id)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
