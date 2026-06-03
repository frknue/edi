package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"liferpg/internal/services"
)

// writeJSON encodes v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

type errorBody struct {
	Error string `json:"error"`
}

// writeError maps domain errors to HTTP status codes and emits a JSON body.
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, services.ErrValidation):
		status = http.StatusBadRequest
	case errors.Is(err, services.ErrNotFound):
		status = http.StatusNotFound
	}
	writeJSON(w, status, errorBody{Error: err.Error()})
}

// decodeBody parses the JSON request body into dst.
func decodeBody(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return errors.Join(services.ErrValidation, err)
	}
	return nil
}

// pathID parses the {id} path value as int64.
func pathID(r *http.Request, name string) (int64, error) {
	raw := r.PathValue(name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, errors.Join(services.ErrValidation, err)
	}
	return id, nil
}

// queryInt parses an optional integer query param with a default.
func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
