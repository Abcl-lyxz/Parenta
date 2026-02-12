package handlers

import (
	"net/http"
	"time"

	"parenta/internal/models"
	"parenta/internal/services"
	"parenta/internal/storage"
)

// SessionsHandler handles session management endpoints
type SessionsHandler struct {
	storage *storage.Storage
	ndsctl  *services.NDSCtl
}

// NewSessionsHandler creates a new SessionsHandler
func NewSessionsHandler(store *storage.Storage, ndsctl *services.NDSCtl) *SessionsHandler {
	return &SessionsHandler{
		storage: store,
		ndsctl:  ndsctl,
	}
}

// SessionResponse represents session in API response
type SessionResponse struct {
	ID           string    `json:"id"`
	ChildID      string    `json:"child_id"`
	ChildName    string    `json:"child_name"`
	MAC          string    `json:"mac"`
	IP           string    `json:"ip"`
	StartedAt    time.Time `json:"started_at"`
	DurationMin  int       `json:"duration_min"`
	RemainingMin int       `json:"remaining_min"`
	IsActive     bool      `json:"is_active"`
}

// toSessionResponse converts Session to SessionResponse
func (h *SessionsHandler) toSessionResponse(s *models.Session) SessionResponse {
	remainingMin := 0
	if child := h.storage.GetChild(s.ChildID); child != nil {
		remainingMin = child.RemainingMinutes()
	}

	return SessionResponse{
		ID:           s.ID,
		ChildID:      s.ChildID,
		ChildName:    s.ChildName,
		MAC:          s.MAC,
		IP:           s.IP,
		StartedAt:    s.StartedAt,
		DurationMin:  s.DurationMinutes(),
		RemainingMin: remainingMin,
		IsActive:     s.IsActive,
	}
}

// Handle handles /api/sessions (list)
func (h *SessionsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessions := h.storage.ListSessions()
	response := make([]SessionResponse, len(sessions))
	for i, s := range sessions {
		response[i] = h.toSessionResponse(s)
	}
	JSON(w, http.StatusOK, response)
}

// HandleByID handles /api/sessions/{id} (get, kick)
func (h *SessionsHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	id := ExtractID(r.URL.Path, "/api/sessions")
	if id == "" {
		Error(w, http.StatusBadRequest, "missing session id")
		return
	}

	action := ExtractAction(r.URL.Path, "/api/sessions")

	switch {
	case action == "kick" && r.Method == http.MethodPost:
		h.kick(w, r, id)
	case action == "extend" && r.Method == http.MethodPost:
		h.extend(w, r, id)
	case r.Method == http.MethodGet:
		h.get(w, r, id)
	case r.Method == http.MethodDelete:
		h.kick(w, r, id)
	default:
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *SessionsHandler) get(w http.ResponseWriter, r *http.Request, id string) {
	session := h.storage.GetSession(id)
	if session == nil {
		Error(w, http.StatusNotFound, "session not found")
		return
	}
	JSON(w, http.StatusOK, h.toSessionResponse(session))
}

func (h *SessionsHandler) kick(w http.ResponseWriter, r *http.Request, id string) {
	session := h.storage.GetSession(id)
	if session == nil {
		Error(w, http.StatusNotFound, "session not found")
		return
	}

	// Deauth from openNDS
	if err := h.ndsctl.Deauth(session.MAC); err != nil {
		// Log but don't fail - device might already be offline
	}

	// Mark session as inactive
	session.IsActive = false
	h.storage.SaveSession(session)

	JSON(w, http.StatusOK, map[string]bool{"success": true})
}

// ExtendRequest represents extend session request
type ExtendRequest struct {
	Minutes int `json:"minutes"`
}

func (h *SessionsHandler) extend(w http.ResponseWriter, r *http.Request, id string) {
	session := h.storage.GetSession(id)
	if session == nil {
		Error(w, http.StatusNotFound, "session not found")
		return
	}

	var req ExtendRequest
	if err := ParseJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Minutes <= 0 {
		Error(w, http.StatusBadRequest, "minutes must be positive")
		return
	}

	// Get child and add to their quota
	child := h.storage.GetChild(session.ChildID)
	if child == nil {
		Error(w, http.StatusNotFound, "child not found")
		return
	}

	// Increase daily quota temporarily (or decrease used time)
	child.DailyQuotaMin += req.Minutes
	h.storage.SaveChild(child)

	JSON(w, http.StatusOK, h.toSessionResponse(session))
}
