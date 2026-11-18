package main

import (
	"encoding/json"
	"io"
	"net/http"
)

// Decode the entire object before allowing a handler to change host settings.
// Decoding into a pointer also distinguishes JSON null from a valid zero value.
func decodeAPIObject[T any](w http.ResponseWriter, r *http.Request, dst *T) bool {
	dec := json.NewDecoder(r.Body)
	var value *T
	if err := dec.Decode(&value); err != nil || value == nil || dec.Decode(new(json.RawMessage)) != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "expected one JSON object matching the request fields"})
		return false
	}
	*dst = *value
	return true
}
