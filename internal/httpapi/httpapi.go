package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"pedidos-api/internal/model"
	"pedidos-api/internal/store"
	"strconv"
	"strings"
	"time"
)

type Store interface {
	Ping(context.Context) error
	ListCategories(context.Context) ([]model.Category, error)
	CreateCategory(context.Context, model.CategoryData) (model.Category, error)
	UpdateCategory(context.Context, int64, model.CategoryData) (model.Category, error)
	DeleteCategory(context.Context, int64) error
	ListProducts(context.Context, *int64) ([]model.Product, error)
	GetProduct(context.Context, int64) (model.Product, error)
	CreateProduct(context.Context, model.ProductData) (model.Product, error)
	UpdateProduct(context.Context, int64, model.ProductData) (model.Product, error)
	DeleteProduct(context.Context, int64) error
	ListOrders(context.Context, model.OrderFilter) ([]model.OrderSummary, error)
	GetOrder(context.Context, int64) (model.Order, error)
	CreateOrder(context.Context, model.CreateOrderData) (model.Order, error)
	UpdateOrder(context.Context, int64, model.UpdateOrderData) (model.Order, error)
	AddItems(context.Context, int64, []model.NewItem) (model.Order, error)
	UpdateItem(context.Context, int64, int64, model.UpdateItemData) (model.Order, error)
	DeleteItem(context.Context, int64, int64) (model.Order, error)
	SetItemStatus(context.Context, int64, int64, string) (model.OrderItem, error)
	CloseOrder(context.Context, int64) (model.Order, error)
	CancelOrder(context.Context, int64) (model.Order, error)
	KitchenOrders(context.Context, bool) (model.KitchenResponse, error)
}
type handlers struct {
	st  Store
	log *slog.Logger
}
type apiError struct {
	Status        int
	Code, Message string
	Fields        map[string]string
}

var (
	errInvalid      = &apiError{400, "invalid_json", "JSON inválido", nil}
	errUnauthorized = &apiError{401, "unauthorized", "não autorizado", nil}
	errNotFound     = &apiError{404, "not_found", "recurso não encontrado", nil}
	errConflict     = &apiError{409, "conflict", "já existe uma categoria com este nome", nil}
	errInUse        = &apiError{409, "category_in_use", "categoria possui produtos ativos", nil}
	errOpen         = &apiError{409, "order_not_open", "pedido não está aberto", nil}
	errEmpty        = &apiError{409, "order_empty", "pedido não possui itens", nil}
	errDone         = &apiError{409, "item_already_done", "item já foi concluído pela cozinha", nil}
	errLarge        = &apiError{413, "payload_too_large", "corpo da requisição excede 64 KB", nil}
	errInternal     = &apiError{500, "internal_error", "erro interno", nil}
	errDB           = &apiError{503, "db_unavailable", "banco de dados indisponível", nil}
)

func writeJSON(w http.ResponseWriter, s int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(s)
	json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, e *apiError) {
	type er struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields,omitempty"`
	}
	writeJSON(w, e.Status, struct {
		Error er `json:"error"`
	}{er{e.Code, e.Message, e.Fields}})
}
func validation(f model.FieldErrors) *apiError {
	return &apiError{422, "validation_error", "dados inválidos", f}
}
func decode(r *http.Request, d any) *apiError {
	j := json.NewDecoder(r.Body)
	j.DisallowUnknownFields()
	if e := j.Decode(d); e != nil {
		var m *http.MaxBytesError
		if errors.As(e, &m) {
			return errLarge
		}
		return errInvalid
	}
	if e := j.Decode(&struct{}{}); e != io.EOF {
		var m *http.MaxBytesError
		if errors.As(e, &m) {
			return errLarge
		}
		return errInvalid
	}
	return nil
}
func id(r *http.Request, k string) (int64, bool) {
	v, e := strconv.ParseInt(r.PathValue(k), 10, 64)
	return v, e == nil && v > 0
}
func (h *handlers) from(r *http.Request, e error) *apiError {
	switch {
	case errors.Is(e, store.ErrNotFound):
		return errNotFound
	case errors.Is(e, store.ErrCategoryConflict):
		return errConflict
	case errors.Is(e, store.ErrCategoryInUse):
		return errInUse
	case errors.Is(e, store.ErrOrderNotOpen):
		return errOpen
	case errors.Is(e, store.ErrOrderEmpty):
		return errEmpty
	case errors.Is(e, store.ErrItemDone):
		return errDone
	}
	var v *store.ValidationError
	if errors.As(e, &v) {
		return validation(v.Fields)
	}
	h.log.Error("erro no store", "error", e, "path", r.URL.Path)
	return errInternal
}
func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	c, x := context.WithTimeout(r.Context(), 2*time.Second)
	defer x()
	if h.st.Ping(c) != nil {
		writeError(w, errDB)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
func (h *handlers) categories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		x, e := h.st.ListCategories(r.Context())
		if e != nil {
			writeError(w, h.from(r, e))
			return
		}
		writeJSON(w, 200, x)
	case "POST":
		var in model.CategoryInput
		if e := decode(r, &in); e != nil {
			writeError(w, e)
			return
		}
		d, f := model.ValidateCategory(in)
		if f.Any() {
			writeError(w, validation(f))
			return
		}
		x, e := h.st.CreateCategory(r.Context(), d)
		if e != nil {
			writeError(w, h.from(r, e))
			return
		}
		writeJSON(w, 201, x)
	}
}
func (h *handlers) category(w http.ResponseWriter, r *http.Request) {
	v, ok := id(r, "id")
	if !ok {
		writeError(w, errNotFound)
		return
	}
	if r.Method == "DELETE" {
		if e := h.st.DeleteCategory(r.Context(), v); e != nil {
			writeError(w, h.from(r, e))
			return
		}
		w.WriteHeader(204)
		return
	}
	var in model.CategoryInput
	if e := decode(r, &in); e != nil {
		writeError(w, e)
		return
	}
	d, f := model.ValidateCategory(in)
	if f.Any() {
		writeError(w, validation(f))
		return
	}
	x, e := h.st.UpdateCategory(r.Context(), v, d)
	if e != nil {
		writeError(w, h.from(r, e))
		return
	}
	writeJSON(w, 200, x)
}
func (h *handlers) products(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		var c *int64
		if s := r.URL.Query().Get("category_id"); s != "" {
			v, e := strconv.ParseInt(s, 10, 64)
			if e != nil || v < 1 {
				writeError(w, validation(model.FieldErrors{"category_id": "valor inválido"}))
				return
			}
			c = &v
		}
		x, e := h.st.ListProducts(r.Context(), c)
		if e != nil {
			writeError(w, h.from(r, e))
			return
		}
		writeJSON(w, 200, x)
		return
	}
	var in model.ProductInput
	if e := decode(r, &in); e != nil {
		writeError(w, e)
		return
	}
	d, f := model.ValidateProduct(in)
	if f.Any() {
		writeError(w, validation(f))
		return
	}
	x, e := h.st.CreateProduct(r.Context(), d)
	if e != nil {
		writeError(w, h.from(r, e))
		return
	}
	writeJSON(w, 201, x)
}
func (h *handlers) product(w http.ResponseWriter, r *http.Request) {
	v, ok := id(r, "id")
	if !ok {
		writeError(w, errNotFound)
		return
	}
	if r.Method == "GET" {
		x, e := h.st.GetProduct(r.Context(), v)
		if e != nil {
			writeError(w, h.from(r, e))
			return
		}
		writeJSON(w, 200, x)
		return
	}
	if r.Method == "DELETE" {
		if e := h.st.DeleteProduct(r.Context(), v); e != nil {
			writeError(w, h.from(r, e))
			return
		}
		w.WriteHeader(204)
		return
	}
	var in model.ProductInput
	if e := decode(r, &in); e != nil {
		writeError(w, e)
		return
	}
	d, f := model.ValidateProduct(in)
	if f.Any() {
		writeError(w, validation(f))
		return
	}
	x, e := h.st.UpdateProduct(r.Context(), v, d)
	if e != nil {
		writeError(w, h.from(r, e))
		return
	}
	writeJSON(w, 200, x)
}
func (h *handlers) orders(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		f, es := model.ValidateOrderFilter(r.URL.Query())
		if es.Any() {
			writeError(w, validation(es))
			return
		}
		x, e := h.st.ListOrders(r.Context(), f)
		if e != nil {
			writeError(w, h.from(r, e))
			return
		}
		writeJSON(w, 200, x)
		return
	}
	var in model.CreateOrderInput
	if e := decode(r, &in); e != nil {
		writeError(w, e)
		return
	}
	d, es := model.ValidateCreateOrder(in)
	if es.Any() {
		writeError(w, validation(es))
		return
	}
	x, e := h.st.CreateOrder(r.Context(), d)
	if e != nil {
		writeError(w, h.from(r, e))
		return
	}
	writeJSON(w, 201, x)
}
func (h *handlers) order(w http.ResponseWriter, r *http.Request) {
	v, ok := id(r, "id")
	if !ok {
		writeError(w, errNotFound)
		return
	}
	if r.Method == "GET" {
		x, e := h.st.GetOrder(r.Context(), v)
		if e != nil {
			writeError(w, h.from(r, e))
			return
		}
		writeJSON(w, 200, x)
		return
	}
	var in model.UpdateOrderInput
	if e := decode(r, &in); e != nil {
		writeError(w, e)
		return
	}
	d, es := model.ValidateUpdateOrder(in)
	if es.Any() {
		writeError(w, validation(es))
		return
	}
	x, e := h.st.UpdateOrder(r.Context(), v, d)
	if e != nil {
		writeError(w, h.from(r, e))
		return
	}
	writeJSON(w, 200, x)
}
func (h *handlers) add(w http.ResponseWriter, r *http.Request) {
	v, ok := id(r, "id")
	if !ok {
		writeError(w, errNotFound)
		return
	}
	var in model.AddItemsInput
	if e := decode(r, &in); e != nil {
		writeError(w, e)
		return
	}
	d, es := model.ValidateAddItems(in)
	if es.Any() {
		writeError(w, validation(es))
		return
	}
	x, e := h.st.AddItems(r.Context(), v, d)
	if e != nil {
		writeError(w, h.from(r, e))
		return
	}
	writeJSON(w, 200, x)
}
func (h *handlers) item(w http.ResponseWriter, r *http.Request) {
	v, ok := id(r, "id")
	i, ok2 := id(r, "itemId")
	if !ok || !ok2 {
		writeError(w, errNotFound)
		return
	}
	if r.Method == "DELETE" {
		x, e := h.st.DeleteItem(r.Context(), v, i)
		if e != nil {
			writeError(w, h.from(r, e))
			return
		}
		writeJSON(w, 200, x)
		return
	}
	var in model.UpdateItemInput
	if e := decode(r, &in); e != nil {
		writeError(w, e)
		return
	}
	d, es := model.ValidateUpdateItem(in)
	if es.Any() {
		writeError(w, validation(es))
		return
	}
	x, e := h.st.UpdateItem(r.Context(), v, i, d)
	if e != nil {
		writeError(w, h.from(r, e))
		return
	}
	writeJSON(w, 200, x)
}
func (h *handlers) itemStatus(w http.ResponseWriter, r *http.Request) {
	v, ok := id(r, "id")
	i, ok2 := id(r, "itemId")
	if !ok || !ok2 {
		writeError(w, errNotFound)
		return
	}
	var in model.ItemStatusInput
	if e := decode(r, &in); e != nil {
		writeError(w, e)
		return
	}
	d, es := model.ValidateItemStatus(in)
	if es.Any() {
		writeError(w, validation(es))
		return
	}
	x, e := h.st.SetItemStatus(r.Context(), v, i, d)
	if e != nil {
		writeError(w, h.from(r, e))
		return
	}
	writeJSON(w, 200, x)
}
func (h *handlers) close(w http.ResponseWriter, r *http.Request) {
	v, ok := id(r, "id")
	if !ok {
		writeError(w, errNotFound)
		return
	}
	x, e := h.st.CloseOrder(r.Context(), v)
	if e != nil {
		writeError(w, h.from(r, e))
		return
	}
	writeJSON(w, 200, x)
}
func (h *handlers) cancel(w http.ResponseWriter, r *http.Request) {
	v, ok := id(r, "id")
	if !ok {
		writeError(w, errNotFound)
		return
	}
	x, e := h.st.CancelOrder(r.Context(), v)
	if e != nil {
		writeError(w, h.from(r, e))
		return
	}
	writeJSON(w, 200, x)
}
func (h *handlers) kitchen(w http.ResponseWriter, r *http.Request) {
	s := r.URL.Query().Get("include_done")
	d := false
	if s != "" {
		if s != "true" && s != "false" {
			writeError(w, validation(model.FieldErrors{"include_done": "deve ser true ou false"}))
			return
		}
		d = s == "true"
	}
	x, e := h.st.KitchenOrders(r.Context(), d)
	if e != nil {
		writeError(w, h.from(r, e))
		return
	}
	writeJSON(w, 200, x)
}

type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(n int) { r.status = n; r.ResponseWriter.WriteHeader(n) }
func NewHandler(st Store, token string, log *slog.Logger) http.Handler {
	h := &handlers{st, log}
	m := http.NewServeMux()
	m.HandleFunc("GET /health", h.health)
	m.HandleFunc("GET /categories", h.categories)
	m.HandleFunc("POST /categories", h.categories)
	m.HandleFunc("PUT /categories/{id}", h.category)
	m.HandleFunc("DELETE /categories/{id}", h.category)
	m.HandleFunc("GET /products", h.products)
	m.HandleFunc("POST /products", h.products)
	m.HandleFunc("GET /products/{id}", h.product)
	m.HandleFunc("PUT /products/{id}", h.product)
	m.HandleFunc("DELETE /products/{id}", h.product)
	m.HandleFunc("GET /orders", h.orders)
	m.HandleFunc("POST /orders", h.orders)
	m.HandleFunc("GET /orders/{id}", h.order)
	m.HandleFunc("PATCH /orders/{id}", h.order)
	m.HandleFunc("POST /orders/{id}/items", h.add)
	m.HandleFunc("PATCH /orders/{id}/items/{itemId}", h.item)
	m.HandleFunc("DELETE /orders/{id}/items/{itemId}", h.item)
	m.HandleFunc("PATCH /orders/{id}/items/{itemId}/status", h.itemStatus)
	m.HandleFunc("POST /orders/{id}/close", h.close)
	m.HandleFunc("POST /orders/{id}/cancel", h.cancel)
	m.HandleFunc("GET /kitchen/orders", h.kitchen)
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { writeError(w, errNotFound) })
	body := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		m.ServeHTTP(w, r)
	})
	auth := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/health" {
			body.ServeHTTP(w, r)
			return
		}
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			writeError(w, errUnauthorized)
			return
		}
		body.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &recorder{ResponseWriter: w, status: 200}
		start := time.Now()
		defer func() {
			if p := recover(); p != nil {
				log.Error("panic", "error", p)
				writeError(rec, errInternal)
			}
			lvl := slog.LevelInfo
			if r.URL.Path == "/health" {
				lvl = slog.LevelDebug
			}
			log.Log(r.Context(), lvl, "request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration_ms", time.Since(start).Milliseconds())
		}()
		auth.ServeHTTP(rec, r)
	})
}
