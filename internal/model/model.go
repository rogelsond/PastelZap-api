package model

import "time"

const (
	OrderOpen     = "open"
	OrderClosed   = "closed"
	OrderCanceled = "canceled"
	ItemPending   = "pending"
	ItemDone      = "done"
)

type Category struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	ProductsCount int       `json:"products_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type Product struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	CategoryID   int64     `json:"category_id"`
	CategoryName string    `json:"category_name"`
	PriceCents   int64     `json:"price_cents"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type OrderItem struct {
	ID             int64      `json:"id"`
	ProductID      int64      `json:"product_id"`
	ProductName    string     `json:"product_name"`
	Quantity       int        `json:"quantity"`
	UnitPriceCents int64      `json:"unit_price_cents"`
	TotalCents     int64      `json:"total_cents"`
	Notes          *string    `json:"notes"`
	Status         string     `json:"status"`
	DoneAt         *time.Time `json:"done_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
type OrderSummary struct {
	ID           int64      `json:"id"`
	CustomerName *string    `json:"customer_name"`
	TableLabel   *string    `json:"table_label"`
	Status       string     `json:"status"`
	TotalCents   int64      `json:"total_cents"`
	ItemsCount   int        `json:"items_count"`
	PendingCount int        `json:"pending_count"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ClosedAt     *time.Time `json:"closed_at"`
}
type Order struct {
	OrderSummary
	Items []OrderItem `json:"items"`
}
type KitchenOrder struct {
	ID              int64       `json:"id"`
	CustomerName    *string     `json:"customer_name"`
	TableLabel      *string     `json:"table_label"`
	CreatedAt       time.Time   `json:"created_at"`
	OldestPendingAt *time.Time  `json:"oldest_pending_at"`
	Items           []OrderItem `json:"items"`
}
type KitchenResponse struct {
	ServerTime time.Time      `json:"server_time"`
	Orders     []KitchenOrder `json:"orders"`
}
type CategoryInput struct {
	Name *string `json:"name"`
}
type ProductInput struct {
	Name       *string `json:"name"`
	CategoryID *int64  `json:"category_id"`
	PriceCents *int64  `json:"price_cents"`
}
type NewItemInput struct {
	ProductID *int64  `json:"product_id"`
	Quantity  *int    `json:"quantity"`
	Notes     *string `json:"notes"`
}
type CreateOrderInput struct {
	CustomerName *string        `json:"customer_name"`
	TableLabel   *string        `json:"table_label"`
	Items        []NewItemInput `json:"items"`
}
type AddItemsInput struct {
	Items []NewItemInput `json:"items"`
}
type Optional[T any] struct {
	Set   bool
	Null  bool
	Value T
}
type UpdateOrderInput struct {
	CustomerName Optional[string] `json:"customer_name"`
	TableLabel   Optional[string] `json:"table_label"`
}
type UpdateItemInput struct {
	Quantity Optional[int]    `json:"quantity"`
	Notes    Optional[string] `json:"notes"`
}
type ItemStatusInput struct {
	Status *string `json:"status"`
}
type CategoryData struct{ Name string }
type ProductData struct {
	Name       string
	CategoryID int64
	PriceCents int64
}
type NewItem struct {
	ProductID int64
	Quantity  int
	Notes     *string
}
type CreateOrderData struct {
	CustomerName *string
	TableLabel   *string
	Items        []NewItem
}
type OptText struct {
	Set   bool
	Value *string
}
type UpdateOrderData struct {
	CustomerName OptText
	TableLabel   OptText
}
type UpdateItemData struct {
	Quantity *int
	Notes    OptText
}
type OrderFilter struct {
	Status     string
	Limit      int
	ClosedFrom *time.Time
}
