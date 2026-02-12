package handlers

import (
	"net/http"
	"time"

	"parenta/internal/models"
	"parenta/internal/services"
	"parenta/internal/storage"
)

// SchedulesHandler handles schedule CRUD endpoints
type SchedulesHandler struct {
	storage *storage.Storage
}

// NewSchedulesHandler creates a new SchedulesHandler
func NewSchedulesHandler(store *storage.Storage) *SchedulesHandler {
	return &SchedulesHandler{
		storage: store,
	}
}

// ScheduleRequest represents create/update schedule request
type ScheduleRequest struct {
	Name       string             `json:"name"`
	TimeBlocks []models.TimeBlock `json:"time_blocks"`
	IsDefault  bool               `json:"is_default"`
}

// Handle handles /api/schedules (list and create)
func (h *SchedulesHandler) Handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.create(w, r)
	default:
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleByID handles /api/schedules/{id} (get, update, delete)
func (h *SchedulesHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	id := ExtractID(r.URL.Path, "/api/schedules")
	if id == "" {
		Error(w, http.StatusBadRequest, "missing schedule id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.get(w, r, id)
	case http.MethodPut:
		h.update(w, r, id)
	case http.MethodDelete:
		h.delete(w, r, id)
	default:
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *SchedulesHandler) list(w http.ResponseWriter, r *http.Request) {
	schedules := h.storage.ListSchedules()
	JSON(w, http.StatusOK, schedules)
}

func (h *SchedulesHandler) get(w http.ResponseWriter, r *http.Request, id string) {
	schedule := h.storage.GetSchedule(id)
	if schedule == nil {
		Error(w, http.StatusNotFound, "schedule not found")
		return
	}
	JSON(w, http.StatusOK, schedule)
}

func (h *SchedulesHandler) create(w http.ResponseWriter, r *http.Request) {
	var req ScheduleRequest
	if err := ParseJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		Error(w, http.StatusBadRequest, "name is required")
		return
	}

	schedule := &models.Schedule{
		ID:         services.GenerateID(),
		Name:       req.Name,
		TimeBlocks: req.TimeBlocks,
		IsDefault:  req.IsDefault,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Initialize empty time blocks if nil
	if schedule.TimeBlocks == nil {
		schedule.TimeBlocks = make([]models.TimeBlock, 0)
	}

	if err := h.storage.SaveSchedule(schedule); err != nil {
		Error(w, http.StatusInternalServerError, "failed to save schedule")
		return
	}

	JSON(w, http.StatusCreated, schedule)
}

func (h *SchedulesHandler) update(w http.ResponseWriter, r *http.Request, id string) {
	schedule := h.storage.GetSchedule(id)
	if schedule == nil {
		Error(w, http.StatusNotFound, "schedule not found")
		return
	}

	var req ScheduleRequest
	if err := ParseJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != "" {
		schedule.Name = req.Name
	}
	if req.TimeBlocks != nil {
		schedule.TimeBlocks = req.TimeBlocks
	}
	schedule.IsDefault = req.IsDefault
	schedule.UpdatedAt = time.Now()

	if err := h.storage.SaveSchedule(schedule); err != nil {
		Error(w, http.StatusInternalServerError, "failed to save schedule")
		return
	}

	JSON(w, http.StatusOK, schedule)
}

func (h *SchedulesHandler) delete(w http.ResponseWriter, r *http.Request, id string) {
	schedule := h.storage.GetSchedule(id)
	if schedule == nil {
		Error(w, http.StatusNotFound, "schedule not found")
		return
	}

	if err := h.storage.DeleteSchedule(id); err != nil {
		Error(w, http.StatusInternalServerError, "failed to delete schedule")
		return
	}

	JSON(w, http.StatusOK, map[string]bool{"success": true})
}
