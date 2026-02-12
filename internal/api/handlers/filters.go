package handlers

import (
	"net/http"
	"time"

	"parenta/internal/models"
	"parenta/internal/services"
	"parenta/internal/storage"
)

// FiltersHandler handles filter rules CRUD endpoints
type FiltersHandler struct {
	storage *storage.Storage
	dnsmasq *services.DnsmasqService
}

// NewFiltersHandler creates a new FiltersHandler
func NewFiltersHandler(store *storage.Storage, dnsmasq *services.DnsmasqService) *FiltersHandler {
	return &FiltersHandler{
		storage: store,
		dnsmasq: dnsmasq,
	}
}

// FilterRequest represents create filter request
type FilterRequest struct {
	Domain   string `json:"domain"`
	RuleType string `json:"rule_type"` // "whitelist" or "blacklist"
	Category string `json:"category"`
}

// Handle handles /api/filters (list and create)
func (h *FiltersHandler) Handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.create(w, r)
	default:
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleByID handles /api/filters/{id} (get, delete)
func (h *FiltersHandler) HandleByID(w http.ResponseWriter, r *http.Request) {
	id := ExtractID(r.URL.Path, "/api/filters")
	if id == "" {
		Error(w, http.StatusBadRequest, "missing filter id")
		return
	}

	switch r.Method {
	case http.MethodDelete:
		h.delete(w, r, id)
	default:
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleReload handles /api/filters/reload
func (h *FiltersHandler) HandleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := h.dnsmasq.ApplyAndReload(); err != nil {
		Error(w, http.StatusInternalServerError, "failed to reload filters: "+err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *FiltersHandler) list(w http.ResponseWriter, r *http.Request) {
	// Optional filter by type
	ruleType := r.URL.Query().Get("type")
	var rt models.RuleType
	if ruleType == "whitelist" {
		rt = models.RuleTypeWhitelist
	} else if ruleType == "blacklist" {
		rt = models.RuleTypeBlacklist
	}

	filters := h.storage.ListFilters(rt)
	JSON(w, http.StatusOK, filters)
}

func (h *FiltersHandler) create(w http.ResponseWriter, r *http.Request) {
	var req FilterRequest
	if err := ParseJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Domain == "" {
		Error(w, http.StatusBadRequest, "domain is required")
		return
	}

	if req.RuleType != "whitelist" && req.RuleType != "blacklist" {
		Error(w, http.StatusBadRequest, "rule_type must be 'whitelist' or 'blacklist'")
		return
	}

	filter := &models.FilterRule{
		ID:        services.GenerateID(),
		Domain:    req.Domain,
		RuleType:  models.RuleType(req.RuleType),
		Category:  req.Category,
		CreatedAt: time.Now(),
	}

	if err := h.storage.SaveFilter(filter); err != nil {
		Error(w, http.StatusInternalServerError, "failed to save filter")
		return
	}

	// Regenerate dnsmasq configs (but don't reload yet)
	h.dnsmasq.RegenerateConfigs()

	JSON(w, http.StatusCreated, filter)
}

func (h *FiltersHandler) delete(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.storage.DeleteFilter(id); err != nil {
		Error(w, http.StatusInternalServerError, "failed to delete filter")
		return
	}

	// Regenerate dnsmasq configs (but don't reload yet)
	h.dnsmasq.RegenerateConfigs()

	JSON(w, http.StatusOK, map[string]bool{"success": true})
}
