package store

import (
	"context"
	"pedidos-api/internal/model"
	"time"
)

func (s *Store) KitchenOrders(c context.Context, done bool) (model.KitchenResponse, error) {
	out := model.KitchenResponse{ServerTime: time.Now().UTC(), Orders: []model.KitchenOrder{}}
	rows, e := s.pool.Query(c, `WITH k AS (SELECT o.id,MIN(i.created_at) FILTER(WHERE i.status='pending') oldest FROM orders o JOIN order_items i ON i.order_id=o.id WHERE o.status='open' GROUP BY o.id HAVING $1::boolean OR bool_or(i.status='pending')) SELECT o.id,o.customer_name,o.table_label,o.created_at,k.oldest,i.id,i.product_id,i.product_name,i.quantity,i.unit_price_cents,i.total_cents,i.notes,i.status,i.done_at,i.created_at,i.updated_at FROM k JOIN orders o ON o.id=k.id JOIN order_items i ON i.order_id=o.id ORDER BY k.oldest ASC NULLS LAST,o.created_at,o.id,i.created_at,i.id`, done)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	var current int64 = -1
	for rows.Next() {
		var ko model.KitchenOrder
		var it model.OrderItem
		e = rows.Scan(&ko.ID, &ko.CustomerName, &ko.TableLabel, &ko.CreatedAt, &ko.OldestPendingAt, &it.ID, &it.ProductID, &it.ProductName, &it.Quantity, &it.UnitPriceCents, &it.TotalCents, &it.Notes, &it.Status, &it.DoneAt, &it.CreatedAt, &it.UpdatedAt)
		if e != nil {
			return out, e
		}
		ko.CreatedAt = ko.CreatedAt.UTC()
		if ko.OldestPendingAt != nil {
			v := ko.OldestPendingAt.UTC()
			ko.OldestPendingAt = &v
		}
		if it.DoneAt != nil {
			v := it.DoneAt.UTC()
			it.DoneAt = &v
		}
		it.CreatedAt = it.CreatedAt.UTC()
		it.UpdatedAt = it.UpdatedAt.UTC()
		if ko.ID != current {
			ko.Items = []model.OrderItem{it}
			out.Orders = append(out.Orders, ko)
			current = ko.ID
		} else {
			out.Orders[len(out.Orders)-1].Items = append(out.Orders[len(out.Orders)-1].Items, it)
		}
	}
	return out, rows.Err()
}
