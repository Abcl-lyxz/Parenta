package handlers

import (
	"net/http"
	"time"

	"parenta/internal/models"
	"parenta/internal/services"
	"parenta/internal/storage"
)

// ChildrenHandler handles children CRUD endpoints
type ChildrenHandler struct {
	storage *storage.Storage
	authSvc *services.AuthService
}

// NewChildrenHandler creates a new ChildrenHandler
func NewChildrenHandler(store *storage.Storage, authSvc *services.AuthService) *ChildrenHandler {
	return &ChildrenHandler{
		storage: store,
		authSvc: authSvc,
	}
}

// ChildRequest represents create/update child request
type ChildRequest struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	Name          string `json:"name"`
	DailyQuotaMin int    `json:"daily_quota_min"`
	FilterMode    string `json:"filter_mode"`
	ScheduleID    string `json:"schedule_id"`
	IsActive      bool   `json:"is_active"`
}

// ChildResponse represents child in API response (no password)
type ChildResponse struct {
	ID              string          `json:"id"`
	Username        string          `json:"username"`
	Name            string          `json:"name"`
	DailyQuotaMin   int             `json:"daily_quota_min"`
	UsedTodayMin    int             `json:"used_today_min"`
	RemainingMin    int             `json:"remaining_min"`
	FilterMode      string          `json:"filter_mode"`
	ScheduleID      string          `json:"schedule_id"`
	Devices         []models.Device `json:"devices"`
	IsActive        bool            `json:"is_active"`
	LastResetDate   string          `json:"last_reset_date"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// toResponse converts Child to ChildResponse
func toChildResponse(c *models.Child) ChildResponse {
	return ChildResponse{
		ID:            c.ID,
		Username:      c.Username,
		Name:          c.Name,
		DailyQuotaMin: c.DailyQuotaMin,
		UsedTodayMin:  c.UsedTodayMin,
		RemainingMin:  c.RemainingMinutes(),
		FilterMode:    string(c.FilterMode),
		ScheduleID:    c.ScheduleID,
		Devices:       c.Devices,
		IsActive:      c.IsActive,
		LastResetDate: c.LastResetDate,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
}

// Handle handles /api/children (list and create)
func (h *ChildrenHandler) Handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.create(w, r)
	default:
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleByID handles /api/children/{id} (get, update, delete)
func (h *ChildrenHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	id := ExtractID(r.URL.Path, "/api/children")
	if id == "" {
		Error(w, http.StatusBadRequest, "missing child id")
		return
	}

	// Check for actions like /api/children/{id}/reset-quota
	action := ExtractAction(r.URL.Path, "/api/children")

	switch {
	case action == "reset-quota" && r.Method == http.MethodPost:
		h.resetQuota(w, r, id)
	case action == "adjust-quota" && r.Method == http.MethodPost:
		h.adjustQuota(w, r, id)
	case action == "devices" && r.Method == http.MethodPost:
		h.addDevice(w, r, id)
	case action == "devices" && r.Method == http.MethodDelete:
		h.removeDevice(w, r, id)
	case r.Method == http.MethodGet:
		h.get(w, r, id)
	case r.Method == http.MethodPut:
		h.update(w, r, id)
	case r.Method == http.MethodDelete:
		h.delete(w, r, id)
	default:
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *ChildrenHandler) list(w http.ResponseWriter, r *http.Request) {
	children := h.storage.ListChildren()
	response := make([]ChildResponse, len(children))
	for i, c := range children {
		response[i] = toChildResponse(c)
	}
	JSON(w, http.StatusOK, response)
}

func (h *ChildrenHandler) get(w http.ResponseWriter, r *http.Request, id string) {
	child := h.storage.GetChild(id)
	if child == nil {
		Error(w, http.StatusNotFound, "child not found")
		return
	}
	JSON(w, http.StatusOK, toChildResponse(child))
}

func (h *ChildrenHandler) create(w http.ResponseWriter, r *http.Request) {
	var req ChildRequest
	if err := ParseJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate required fields
	if req.Username == "" || req.Password == "" || req.Name == "" {
		Error(w, http.StatusBadRequest, "username, password, and name are required")
		return
	}

	// Check username uniqueness
	if existing := h.storage.GetChildByUsername(req.Username); existing != nil {
		Error(w, http.StatusConflict, "username already exists")
		return
	}

	// Hash password
	hash, err := services.HashPassword(req.Password)
	if err != nil {
		Error(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	// Set defaults
	filterMode := models.FilterModeNormal
	if req.FilterMode == "study" {
		filterMode = models.FilterModeStudy
	}
	if req.DailyQuotaMin == 0 {
		req.DailyQuotaMin = 120 // Default 2 hours
	}

	child := &models.Child{
		ID:            services.GenerateID(),
		Username:      req.Username,
		PasswordHash:  hash,
		Name:          req.Name,
		DailyQuotaMin: req.DailyQuotaMin,
		UsedTodayMin:  0,
		FilterMode:    filterMode,
		ScheduleID:    req.ScheduleID,
		Devices:       make([]models.Device, 0),
		IsActive:      true,
		LastResetDate: time.Now().Format("2006-01-02"),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := h.storage.SaveChild(child); err != nil {
		Error(w, http.StatusInternalServerError, "failed to save child")
		return
	}

	JSON(w, http.StatusCreated, toChildResponse(child))
}

func (h *ChildrenHandler) update(w http.ResponseWriter, r *http.Request, id string) {
	child := h.storage.GetChild(id)
	if child == nil {
		Error(w, http.StatusNotFound, "child not found")
		return
	}

	var req ChildRequest
	if err := ParseJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Update fields if provided
	if req.Name != "" {
		child.Name = req.Name
	}
	if req.Username != "" && req.Username != child.Username {
		// Check uniqueness
		if existing := h.storage.GetChildByUsername(req.Username); existing != nil && existing.ID != id {
			Error(w, http.StatusConflict, "username already exists")
			return
		}
		child.Username = req.Username
	}
	if req.Password != "" {
		hash, err := services.HashPassword(req.Password)
		if err != nil {
			Error(w, http.StatusInternalServerError, "failed to hash password")
			return
		}
		child.PasswordHash = hash
	}
	if req.DailyQuotaMin > 0 {
		child.DailyQuotaMin = req.DailyQuotaMin
	}
	if req.FilterMode != "" {
		child.FilterMode = models.FilterMode(req.FilterMode)
	}
	child.ScheduleID = req.ScheduleID
	child.IsActive = req.IsActive
	child.UpdatedAt = time.Now()

	if err := h.storage.SaveChild(child); err != nil {
		Error(w, http.StatusInternalServerError, "failed to save child")
		return
	}

	JSON(w, http.StatusOK, toChildResponse(child))
}

func (h *ChildrenHandler) delete(w http.ResponseWriter, r *http.Request, id string) {
	child := h.storage.GetChild(id)
	if child == nil {
		Error(w, http.StatusNotFound, "child not found")
		return
	}

	if err := h.storage.DeleteChild(id); err != nil {
		Error(w, http.StatusInternalServerError, "failed to delete child")
		return
	}

	JSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *ChildrenHandler) resetQuota(w http.ResponseWriter, r *http.Request, id string) {
	child := h.storage.GetChild(id)
	if child == nil {
		Error(w, http.StatusNotFound, "child not found")
		return
	}

	child.UsedTodayMin = 0
	child.LastResetDate = time.Now().Format("2006-01-02")
	child.UpdatedAt = time.Now()

	if err := h.storage.SaveChild(child); err != nil {
		Error(w, http.StatusInternalServerError, "failed to reset quota")
		return
	}

	JSON(w, http.StatusOK, toChildResponse(child))
}

// AdjustQuotaRequest represents quota adjustment request
type AdjustQuotaRequest struct {
	Minutes int `json:"minutes"` // Positive to add time, negative to subtract
}

func (h *ChildrenHandler) adjustQuota(w http.ResponseWriter, r *http.Request, id string) {
	child := h.storage.GetChild(id)
	if child == nil {
		Error(w, http.StatusNotFound, "child not found")
		return
	}

	var req AdjustQuotaRequest
	if err := ParseJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Adjust the used time (negative minutes = more remaining time)
	// If we want to give +30 minutes, we subtract from used time
	child.UsedTodayMin -= req.Minutes

	// Ensure used time doesn't go negative
	if child.UsedTodayMin < 0 {
		child.UsedTodayMin = 0
	}

	// Ensure used time doesn't exceed a reasonable maximum
	if child.UsedTodayMin > child.DailyQuotaMin+480 { // Max 8 hours over quota
		child.UsedTodayMin = child.DailyQuotaMin + 480
	}

	child.UpdatedAt = time.Now()

	if err := h.storage.SaveChild(child); err != nil {
		Error(w, http.StatusInternalServerError, "failed to adjust quota")
		return
	}

	JSON(w, http.StatusOK, toChildResponse(child))
}

// DeviceRequest represents add device request
type DeviceRequest struct {
	MAC  string `json:"mac"`
	Name string `json:"name"`
}

func (h *ChildrenHandler) addDevice(w http.ResponseWriter, r *http.Request, id string) {
	child := h.storage.GetChild(id)
	if child == nil {
		Error(w, http.StatusNotFound, "child not found")
		return
	}

	var req DeviceRequest
	if err := ParseJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.MAC == "" {
		Error(w, http.StatusBadRequest, "mac is required")
		return
	}

	child.AddDevice(req.MAC, req.Name)
	child.UpdatedAt = time.Now()

	if err := h.storage.SaveChild(child); err != nil {
		Error(w, http.StatusInternalServerError, "failed to add device")
		return
	}

	JSON(w, http.StatusOK, toChildResponse(child))
}

func (h *ChildrenHandler) removeDevice(w http.ResponseWriter, r *http.Request, id string) {
	child := h.storage.GetChild(id)
	if child == nil {
		Error(w, http.StatusNotFound, "child not found")
		return
	}

	mac := r.URL.Query().Get("mac")
	if mac == "" {
		Error(w, http.StatusBadRequest, "mac query parameter is required")
		return
	}

	// Remove device
	newDevices := make([]models.Device, 0)
	for _, d := range child.Devices {
		if d.MAC != mac {
			newDevices = append(newDevices, d)
		}
	}
	child.Devices = newDevices
	child.UpdatedAt = time.Now()

	if err := h.storage.SaveChild(child); err != nil {
		Error(w, http.StatusInternalServerError, "failed to remove device")
		return
	}

	JSON(w, http.StatusOK, toChildResponse(child))
}
