package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
)

// JSON sends a JSON response
func JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// Error sends a JSON error response
func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]string{"error": message})
}

// ParseJSON decodes JSON from request body
func ParseJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// ExtractID extracts the ID from a URL path like /api/children/{id}
func ExtractID(path, prefix string) string {
	path = strings.TrimPrefix(path, prefix)
	path = strings.TrimPrefix(path, "/")
	// Handle paths like "123/kick"
	if idx := strings.Index(path, "/"); idx != -1 {
		path = path[:idx]
	}
	return path
}

// ExtractAction extracts action from URL like /api/sessions/{id}/kick
func ExtractAction(path, prefix string) string {
	path = strings.TrimPrefix(path, prefix)
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}
