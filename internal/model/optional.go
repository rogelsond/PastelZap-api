package model

import "encoding/json"

func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	o.Null = string(b) == "null"
	if o.Null {
		return nil
	}
	return json.Unmarshal(b, &o.Value)
}
