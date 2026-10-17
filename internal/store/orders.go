package store

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"pedidos-api/internal/model"
	"strconv"
)

type q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func scanItem(r interface{ Scan(...any) error }) (model.OrderItem, error) {
	var x model.OrderItem
	e := r.Scan(&x.ID, &x.ProductID, &x.ProductName, &x.Quantity, &x.UnitPriceCents, &x.TotalCents, &x.Notes, &x.Status, &x.DoneAt, &x.CreatedAt, &x.UpdatedAt)
	if x.DoneAt != nil {
		v := x.DoneAt.UTC()
		x.DoneAt = &v
	}
	x.CreatedAt = x.CreatedAt.UTC()
	x.UpdatedAt = x.UpdatedAt.UTC()
	return x, e
}
func loadOrder(c context.Context, x q, id int64) (model.Order, error) {
	var o model.Order
	o.Items = []model.OrderItem{}
	e := x.QueryRow(c, "SELECT id,customer_name,table_label,status,total_cents,created_at,updated_at,closed_at FROM orders WHERE id=$1", id).Scan(&o.ID, &o.CustomerName, &o.TableLabel, &o.Status, &o.TotalCents, &o.CreatedAt, &o.UpdatedAt, &o.ClosedAt)
	if e = noRows(e); e != nil {
		return o, e
	}
	o.CreatedAt = o.CreatedAt.UTC()
	o.UpdatedAt = o.UpdatedAt.UTC()
	if o.ClosedAt != nil {
		v := o.ClosedAt.UTC()
		o.ClosedAt = &v
	}
	rows, e := x.Query(c, "SELECT id,product_id,product_name,quantity,unit_price_cents,total_cents,notes,status,done_at,created_at,updated_at FROM order_items WHERE order_id=$1 ORDER BY created_at,id", id)
	if e != nil {
		return o, e
	}
	defer rows.Close()
	for rows.Next() {
		i, e := scanItem(rows)
		if e != nil {
			return o, e
		}
		o.Items = append(o.Items, i)
		if i.Status == model.ItemPending {
			o.PendingCount++
		}
	}
	o.ItemsCount = len(o.Items)
	return o, rows.Err()
}
func recalc(c context.Context, tx pgx.Tx, id int64) error {
	_, e := tx.Exec(c, "UPDATE orders SET total_cents=COALESCE((SELECT SUM(total_cents) FROM order_items WHERE order_id=$1),0),updated_at=now() WHERE id=$1", id)
	return e
}
func (s *Store) insertItems(c context.Context, tx pgx.Tx, id int64, items []model.NewItem) error {
	if len(items) == 0 {
		return nil
	}
	ids := []int64{}
	seen := map[int64]bool{}
	for _, i := range items {
		if !seen[i.ProductID] {
			seen[i.ProductID] = true
			ids = append(ids, i.ProductID)
		}
	}
	rows, e := tx.Query(c, "SELECT id,name,price_cents FROM products WHERE id=ANY($1) AND active", ids)
	if e != nil {
		return e
	}
	type p struct {
		name  string
		price int64
	}
	ps := map[int64]p{}
	for rows.Next() {
		var id int64
		var p p
		if e = rows.Scan(&id, &p.name, &p.price); e != nil {
			rows.Close()
			return e
		}
		ps[id] = p
	}
	rows.Close()
	f := map[string]string{}
	for n, i := range items {
		if _, ok := ps[i.ProductID]; !ok {
			f["items["+strconv.Itoa(n)+"].product_id"] = "produto não encontrado ou inativo"
		}
	}
	if len(f) > 0 {
		return &ValidationError{f}
	}
	b := &pgx.Batch{}
	for _, i := range items {
		p := ps[i.ProductID]
		b.Queue("INSERT INTO order_items(order_id,product_id,product_name,quantity,unit_price_cents,total_cents,notes) VALUES($1,$2,$3,$4,$5,$6,$7)", id, i.ProductID, p.name, i.Quantity, p.price, int64(i.Quantity)*p.price, i.Notes)
	}
	br := tx.SendBatch(c, b)
	defer br.Close()
	for range items {
		if _, e = br.Exec(); e != nil {
			return e
		}
	}
	return nil
}
func (s *Store) mutate(c context.Context, id int64, fn func(pgx.Tx) error) (model.Order, error) {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return model.Order{}, e
	}
	defer tx.Rollback(c)
	var status string
	e = tx.QueryRow(c, "SELECT status FROM orders WHERE id=$1 FOR UPDATE", id).Scan(&status)
	e = noRows(e)
	if e != nil {
		return model.Order{}, e
	}
	if status != model.OrderOpen {
		return model.Order{}, ErrOrderNotOpen
	}
	if e = fn(tx); e != nil {
		return model.Order{}, e
	}
	if e = recalc(c, tx, id); e != nil {
		return model.Order{}, e
	}
	o, e := loadOrder(c, tx, id)
	if e != nil {
		return o, e
	}
	return o, tx.Commit(c)
}
func (s *Store) ListOrders(c context.Context, f model.OrderFilter) ([]model.OrderSummary, error) {
	sql := "SELECT o.id,o.customer_name,o.table_label,o.status,o.total_cents,COUNT(i.id),COUNT(i.id) FILTER(WHERE i.status='pending'),o.created_at,o.updated_at,o.closed_at FROM orders o LEFT JOIN order_items i ON i.order_id=o.id WHERE o.status='open' GROUP BY o.id ORDER BY o.created_at,o.id LIMIT $1"
	args := []any{f.Limit}
	if f.Status != model.OrderOpen {
		sql = "SELECT o.id,o.customer_name,o.table_label,o.status,o.total_cents,COUNT(i.id),COUNT(i.id) FILTER(WHERE i.status='pending'),o.created_at,o.updated_at,o.closed_at FROM orders o LEFT JOIN order_items i ON i.order_id=o.id WHERE o.status=$1 AND ($2::timestamptz IS NULL OR o.closed_at >= $2) GROUP BY o.id ORDER BY o.closed_at DESC,o.id DESC LIMIT $3"
		args = []any{f.Status, f.ClosedFrom, f.Limit}
	}
	rows, e := s.pool.Query(c, sql, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.OrderSummary{}
	for rows.Next() {
		var x model.OrderSummary
		if e = rows.Scan(&x.ID, &x.CustomerName, &x.TableLabel, &x.Status, &x.TotalCents, &x.ItemsCount, &x.PendingCount, &x.CreatedAt, &x.UpdatedAt, &x.ClosedAt); e != nil {
			return nil, e
		}
		x.CreatedAt = x.CreatedAt.UTC()
		x.UpdatedAt = x.UpdatedAt.UTC()
		if x.ClosedAt != nil {
			v := x.ClosedAt.UTC()
			x.ClosedAt = &v
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) GetOrder(c context.Context, id int64) (model.Order, error) {
	return loadOrder(c, s.pool, id)
}
func (s *Store) CreateOrder(c context.Context, d model.CreateOrderData) (model.Order, error) {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return model.Order{}, e
	}
	defer tx.Rollback(c)
	var id int64
	if e = tx.QueryRow(c, "INSERT INTO orders(customer_name,table_label) VALUES($1,$2) RETURNING id", d.CustomerName, d.TableLabel).Scan(&id); e != nil {
		return model.Order{}, e
	}
	if e = s.insertItems(c, tx, id, d.Items); e != nil {
		return model.Order{}, e
	}
	if e = recalc(c, tx, id); e != nil {
		return model.Order{}, e
	}
	o, e := loadOrder(c, tx, id)
	if e != nil {
		return o, e
	}
	return o, tx.Commit(c)
}
func (s *Store) UpdateOrder(c context.Context, id int64, d model.UpdateOrderData) (model.Order, error) {
	return s.mutate(c, id, func(tx pgx.Tx) error {
		_, e := tx.Exec(c, "UPDATE orders SET customer_name=CASE WHEN $2::boolean THEN $3::text ELSE customer_name END,table_label=CASE WHEN $4::boolean THEN $5::text ELSE table_label END WHERE id=$1", id, d.CustomerName.Set, d.CustomerName.Value, d.TableLabel.Set, d.TableLabel.Value)
		return e
	})
}
func (s *Store) AddItems(c context.Context, id int64, is []model.NewItem) (model.Order, error) {
	return s.mutate(c, id, func(tx pgx.Tx) error { return s.insertItems(c, tx, id, is) })
}
func (s *Store) UpdateItem(c context.Context, id, i int64, d model.UpdateItemData) (model.Order, error) {
	return s.mutate(c, id, func(tx pgx.Tx) error {
		var st string
		e := tx.QueryRow(c, "SELECT status FROM order_items WHERE id=$1 AND order_id=$2 FOR UPDATE", i, id).Scan(&st)
		e = noRows(e)
		if e != nil {
			return e
		}
		if st == model.ItemDone {
			return ErrItemDone
		}
		_, e = tx.Exec(c, "UPDATE order_items SET quantity=COALESCE($3::int,quantity),total_cents=COALESCE($3::int,quantity)::bigint*unit_price_cents,notes=CASE WHEN $4::boolean THEN $5::text ELSE notes END,updated_at=now() WHERE id=$1", i, id, d.Quantity, d.Notes.Set, d.Notes.Value)
		return e
	})
}
func (s *Store) DeleteItem(c context.Context, id, i int64) (model.Order, error) {
	return s.mutate(c, id, func(tx pgx.Tx) error {
		t, e := tx.Exec(c, "DELETE FROM order_items WHERE id=$1 AND order_id=$2", i, id)
		if e != nil {
			return e
		}
		if t.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}
func (s *Store) close(c context.Context, id int64, status string) (model.Order, error) {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return model.Order{}, e
	}
	defer tx.Rollback(c)
	var st string
	e = tx.QueryRow(c, "SELECT status FROM orders WHERE id=$1 FOR UPDATE", id).Scan(&st)
	e = noRows(e)
	if e != nil {
		return model.Order{}, e
	}
	if st != model.OrderOpen {
		return model.Order{}, ErrOrderNotOpen
	}
	if status == model.OrderClosed {
		var yes bool
		if e = tx.QueryRow(c, "SELECT EXISTS(SELECT 1 FROM order_items WHERE order_id=$1)", id).Scan(&yes); e != nil {
			return model.Order{}, e
		}
		if !yes {
			return model.Order{}, ErrOrderEmpty
		}
	}
	_, e = tx.Exec(c, "UPDATE orders SET status=$2,closed_at=now(),updated_at=now() WHERE id=$1", id, status)
	if e != nil {
		return model.Order{}, e
	}
	o, e := loadOrder(c, tx, id)
	if e != nil {
		return o, e
	}
	return o, tx.Commit(c)
}
func (s *Store) CloseOrder(c context.Context, id int64) (model.Order, error) {
	return s.close(c, id, model.OrderClosed)
}
func (s *Store) CancelOrder(c context.Context, id int64) (model.Order, error) {
	return s.close(c, id, model.OrderCanceled)
}
func (s *Store) SetItemStatus(c context.Context, id, i int64, status string) (model.OrderItem, error) {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return model.OrderItem{}, e
	}
	defer tx.Rollback(c)
	var st string
	e = tx.QueryRow(c, "SELECT status FROM orders WHERE id=$1 FOR UPDATE", id).Scan(&st)
	e = noRows(e)
	if e != nil {
		return model.OrderItem{}, e
	}
	if st != model.OrderOpen {
		return model.OrderItem{}, ErrOrderNotOpen
	}
	row := tx.QueryRow(c, "UPDATE order_items SET status=$3::text,done_at=CASE WHEN $3::text='done' THEN COALESCE(done_at,now()) ELSE NULL END,updated_at=CASE WHEN status=$3::text THEN updated_at ELSE now() END WHERE id=$1 AND order_id=$2 RETURNING id,product_id,product_name,quantity,unit_price_cents,total_cents,notes,status,done_at,created_at,updated_at", i, id, status)
	x, e := scanItem(row)
	e = noRows(e)
	if e != nil {
		return x, e
	}
	_, e = tx.Exec(c, "UPDATE orders SET updated_at=now() WHERE id=$1", id)
	if e != nil {
		return x, e
	}
	return x, tx.Commit(c)
}
