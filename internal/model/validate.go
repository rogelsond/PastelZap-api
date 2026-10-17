package model

import (
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type FieldErrors map[string]string

func (f FieldErrors) Add(k, v string) {
	if _, ok := f[k]; !ok {
		f[k] = v
	}
}
func (f FieldErrors) Any() bool { return len(f) > 0 }
func req(f FieldErrors, k string, v *string, max int) string {
	if v == nil {
		f.Add(k, "obrigatório")
		return ""
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		f.Add(k, "obrigatório")
	}
	if utf8.RuneCountInString(s) > max {
		f.Add(k, "deve ter no máximo "+strconv.Itoa(max)+" caracteres")
	}
	return s
}
func opt(f FieldErrors, k string, v *string, max int) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	if utf8.RuneCountInString(s) > max {
		f.Add(k, "deve ter no máximo "+strconv.Itoa(max)+" caracteres")
	}
	return &s
}
func ValidateCategory(in CategoryInput) (CategoryData, FieldErrors) {
	f := FieldErrors{}
	return CategoryData{req(f, "name", in.Name, 60)}, f
}
func ValidateProduct(in ProductInput) (ProductData, FieldErrors) {
	f := FieldErrors{}
	d := ProductData{Name: req(f, "name", in.Name, 100)}
	if in.CategoryID == nil {
		f.Add("category_id", "obrigatório")
	} else if *in.CategoryID <= 0 {
		f.Add("category_id", "valor inválido")
	} else {
		d.CategoryID = *in.CategoryID
	}
	if in.PriceCents == nil {
		f.Add("price_cents", "obrigatório")
	} else if *in.PriceCents < 0 || *in.PriceCents > 99999999 {
		f.Add("price_cents", "deve estar entre 0 e 99999999")
	} else {
		d.PriceCents = *in.PriceCents
	}
	return d, f
}
func validItems(in []NewItemInput, required bool) ([]NewItem, FieldErrors) {
	f := FieldErrors{}
	if required && len(in) == 0 {
		f.Add("items", "informe ao menos um item")
	}
	if len(in) > 50 {
		f.Add("items", "máximo de 50 itens por requisição")
		return nil, f
	}
	out := make([]NewItem, len(in))
	for i, x := range in {
		k := "items[" + strconv.Itoa(i) + "]."
		if x.ProductID == nil {
			f.Add(k+"product_id", "obrigatório")
		} else if *x.ProductID <= 0 {
			f.Add(k+"product_id", "valor inválido")
		} else {
			out[i].ProductID = *x.ProductID
		}
		if x.Quantity == nil {
			f.Add(k+"quantity", "obrigatório")
		} else if *x.Quantity < 1 || *x.Quantity > 999 {
			f.Add(k+"quantity", "deve estar entre 1 e 999")
		} else {
			out[i].Quantity = *x.Quantity
		}
		out[i].Notes = opt(f, k+"notes", x.Notes, 200)
	}
	return out, f
}
func ValidateCreateOrder(in CreateOrderInput) (CreateOrderData, FieldErrors) {
	items, f := validItems(in.Items, false)
	return CreateOrderData{opt(f, "customer_name", in.CustomerName, 80), opt(f, "table_label", in.TableLabel, 20), items}, f
}
func ValidateAddItems(in AddItemsInput) ([]NewItem, FieldErrors) { return validItems(in.Items, true) }
func ValidateUpdateOrder(in UpdateOrderInput) (UpdateOrderData, FieldErrors) {
	f := FieldErrors{}
	d := UpdateOrderData{}
	if !in.CustomerName.Set && !in.TableLabel.Set {
		f.Add("_", "informe ao menos um campo")
	}
	if in.CustomerName.Set {
		if in.CustomerName.Null {
			d.CustomerName = OptText{Set: true}
		} else {
			d.CustomerName = OptText{Set: true, Value: opt(f, "customer_name", &in.CustomerName.Value, 80)}
		}
	}
	if in.TableLabel.Set {
		if in.TableLabel.Null {
			d.TableLabel = OptText{Set: true}
		} else {
			d.TableLabel = OptText{Set: true, Value: opt(f, "table_label", &in.TableLabel.Value, 20)}
		}
	}
	return d, f
}
func ValidateUpdateItem(in UpdateItemInput) (UpdateItemData, FieldErrors) {
	f := FieldErrors{}
	d := UpdateItemData{}
	if !in.Quantity.Set && !in.Notes.Set {
		f.Add("_", "informe ao menos um campo")
	}
	if in.Quantity.Set {
		if in.Quantity.Null {
			f.Add("quantity", "não pode ser nulo")
		} else if in.Quantity.Value < 1 || in.Quantity.Value > 999 {
			f.Add("quantity", "deve estar entre 1 e 999")
		} else {
			v := in.Quantity.Value
			d.Quantity = &v
		}
	}
	if in.Notes.Set {
		d.Notes.Set = true
		if !in.Notes.Null {
			d.Notes.Value = opt(f, "notes", &in.Notes.Value, 200)
		}
	}
	return d, f
}
func ValidateItemStatus(in ItemStatusInput) (string, FieldErrors) {
	f := FieldErrors{}
	if in.Status == nil {
		f.Add("status", "obrigatório")
		return "", f
	}
	s := strings.TrimSpace(*in.Status)
	if s != ItemPending && s != ItemDone {
		f.Add("status", "deve ser pending ou done")
	}
	return s, f
}
func ValidateOrderFilter(q url.Values) (OrderFilter, FieldErrors) {
	f := FieldErrors{}
	d := OrderFilter{Status: OrderOpen, Limit: 50}
	if s := q.Get("status"); s != "" {
		if s != OrderOpen && s != OrderClosed && s != OrderCanceled {
			f.Add("status", "deve ser open, closed ou canceled")
		} else {
			d.Status = s
		}
	}
	if s := q.Get("limit"); s != "" {
		n, e := strconv.Atoi(s)
		if e != nil || n < 1 || n > 200 {
			f.Add("limit", "deve ser um inteiro entre 1 e 200")
		} else {
			d.Limit = n
		}
	}
	if s := q.Get("closed_from"); s != "" {
		t, e := time.Parse(time.RFC3339, s)
		if e != nil {
			f.Add("closed_from", "data inválida, use RFC 3339")
		} else {
			d.ClosedFrom = &t
		}
	}
	return d, f
}
