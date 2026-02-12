package handlers

import (
	"net/http"
	"strings"

	"parenta/internal/api/middleware"
	"parenta/internal/config"
	"parenta/internal/models"
	"parenta/internal/services"
	"parenta/internal/storage"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	storage *storage.Storage
	authSvc *services.AuthService
	jwt     *middleware.AuthMiddleware
	config  *config.Config
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(
	store *storage.Storage,
	authSvc *services.AuthService,
	jwt *middleware.AuthMiddleware,
	cfg *config.Config,
) *AuthHandler {
	return &AuthHandler{
		storage: store,
		authSvc: authSvc,
		jwt:     jwt,
		config:  cfg,
	}
}

// LoginRequest represents login request body
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents login response
type LoginResponse struct {
	Token               string `json:"token"`
	ExpiresIn           int    `json:"expires_in"`
	ForcePasswordChange bool   `json:"force_password_change"`
}

// HandleLogin processes login requests
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req LoginRequest
	if err := ParseJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := h.authSvc.AuthenticateAdmin(req.Username, req.Password)
	if err != nil {
		Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := h.jwt.GenerateToken(user.ID, user.Username, true, h.config.Session.JWTExpiryHours)
	if err != nil {
		Error(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	JSON(w, http.StatusOK, LoginResponse{
		Token:               token,
		ExpiresIn:           h.config.Session.JWTExpiryHours * 3600,
		ForcePasswordChange: user.ForcePasswordChange,
	})
}

// HandleLogout processes logout requests
func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// JWT is stateless, so we just return success
	// Client should discard the token
	JSON(w, http.StatusOK, map[string]bool{"success": true})
}

// HandleMe returns current user info
func (h *AuthHandler) HandleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims := middleware.GetClaims(r)
	if claims == nil {
		Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	admin := h.storage.GetAdminByID(claims.UserID)
	if admin == nil {
		Error(w, http.StatusNotFound, "user not found")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"id":                    admin.ID,
		"username":              admin.Username,
		"display_name":          admin.GetDisplayName(),
		"role":                  admin.Role,
		"force_password_change": admin.ForcePasswordChange,
	})
}

// ChangePasswordRequest represents password change request
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// HandleChangePassword processes password change requests
func (h *AuthHandler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims := middleware.GetClaims(r)
	if claims == nil {
		Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req ChangePasswordRequest
	if err := ParseJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.NewPassword) < 6 {
		Error(w, http.StatusBadRequest, "password must be at least 6 characters")
		return
	}

	if err := h.authSvc.ChangeAdminPassword(claims.UserID, req.OldPassword, req.NewPassword); err != nil {
		if err == services.ErrInvalidCredentials {
			Error(w, http.StatusUnauthorized, "invalid old password")
			return
		}
		Error(w, http.StatusInternalServerError, "failed to change password")
		return
	}

	JSON(w, http.StatusOK, map[string]bool{"success": true})
}

// ============ Admin CRUD Handlers ============

// AdminResponse represents an admin in API responses (without password hash)
type AdminResponse struct {
	ID          string          `json:"id"`
	Username    string          `json:"username"`
	DisplayName string          `json:"display_name"`
	Role        models.UserRole `json:"role"`
	CreatedAt   string          `json:"created_at"`
}

// CreateAdminRequest represents create admin request body
type CreateAdminRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// UpdateAdminRequest represents update admin request body
type UpdateAdminRequest struct {
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// ResetPasswordRequest represents password reset request
type ResetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// HandleListAdmins handles GET (list) and POST (create) for /api/admins
func (h *AuthHandler) HandleListAdmins(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listAdmins(w, r)
	case http.MethodPost:
		h.createAdmin(w, r)
	default:
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *AuthHandler) listAdmins(w http.ResponseWriter, r *http.Request) {
	admins := h.storage.ListAdmins()
	response := make([]AdminResponse, len(admins))
	for i, a := range admins {
		response[i] = AdminResponse{
			ID:          a.ID,
			Username:    a.Username,
			DisplayName: a.GetDisplayName(),
			Role:        a.Role,
			CreatedAt:   a.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	JSON(w, http.StatusOK, response)
}

func (h *AuthHandler) createAdmin(w http.ResponseWriter, r *http.Request) {
	// Check if current user is super admin
	claims := middleware.GetClaims(r)
	if claims == nil {
		Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	currentAdmin := h.storage.GetAdminByID(claims.UserID)
	if currentAdmin == nil || !currentAdmin.IsSuper() {
		Error(w, http.StatusForbidden, "only super admins can create new admins")
		return
	}

	var req CreateAdminRequest
	if err := ParseJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		Error(w, http.StatusBadRequest, "username and password are required")
		return
	}

	if len(req.Password) < 6 {
		Error(w, http.StatusBadRequest, "password must be at least 6 characters")
		return
	}

	role := models.RoleAdmin
	if req.Role == "super" {
		role = models.RoleSuper
	}

	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Username
	}

	admin, err := h.authSvc.CreateAdmin(req.Username, req.Password, displayName, role)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			Error(w, http.StatusConflict, "username already exists")
			return
		}
		Error(w, http.StatusInternalServerError, "failed to create admin")
		return
	}

	JSON(w, http.StatusCreated, AdminResponse{
		ID:          admin.ID,
		Username:    admin.Username,
		DisplayName: admin.GetDisplayName(),
		Role:        admin.Role,
		CreatedAt:   admin.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// HandleAdmin handles single admin operations (GET, PUT, DELETE)
func (h *AuthHandler) HandleAdmin(w http.ResponseWriter, r *http.Request) {
	// Extract ID from URL path: /api/admins/{id}
	path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(path, "/api/admins/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		Error(w, http.StatusBadRequest, "admin ID required")
		return
	}
	adminID := parts[0]

	// Check for password reset endpoint: /api/admins/{id}/reset-password
	if len(parts) > 1 && parts[1] == "reset-password" {
		h.handleResetPassword(w, r, adminID)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getAdmin(w, r, adminID)
	case http.MethodPut:
		h.updateAdmin(w, r, adminID)
	case http.MethodDelete:
		h.deleteAdmin(w, r, adminID)
	default:
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *AuthHandler) getAdmin(w http.ResponseWriter, r *http.Request, adminID string) {
	admin := h.storage.GetAdminByID(adminID)
	if admin == nil {
		Error(w, http.StatusNotFound, "admin not found")
		return
	}

	JSON(w, http.StatusOK, AdminResponse{
		ID:          admin.ID,
		Username:    admin.Username,
		DisplayName: admin.GetDisplayName(),
		Role:        admin.Role,
		CreatedAt:   admin.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

func (h *AuthHandler) updateAdmin(w http.ResponseWriter, r *http.Request, adminID string) {
	claims := middleware.GetClaims(r)
	if claims == nil {
		Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	currentAdmin := h.storage.GetAdminByID(claims.UserID)
	if currentAdmin == nil || !currentAdmin.IsSuper() {
		Error(w, http.StatusForbidden, "only super admins can update admins")
		return
	}

	var req UpdateAdminRequest
	if err := ParseJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	role := models.RoleAdmin
	if req.Role == "super" {
		role = models.RoleSuper
	}

	if err := h.authSvc.UpdateAdmin(adminID, req.DisplayName, role); err != nil {
		if err == services.ErrUserNotFound {
			Error(w, http.StatusNotFound, "admin not found")
			return
		}
		Error(w, http.StatusInternalServerError, "failed to update admin")
		return
	}

	JSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *AuthHandler) deleteAdmin(w http.ResponseWriter, r *http.Request, adminID string) {
	claims := middleware.GetClaims(r)
	if claims == nil {
		Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	currentAdmin := h.storage.GetAdminByID(claims.UserID)
	if currentAdmin == nil || !currentAdmin.IsSuper() {
		Error(w, http.StatusForbidden, "only super admins can delete admins")
		return
	}

	// Prevent self-deletion
	if adminID == claims.UserID {
		Error(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	// Prevent deleting last admin
	if h.storage.AdminCount() <= 1 {
		Error(w, http.StatusBadRequest, "cannot delete the last admin")
		return
	}

	if err := h.storage.DeleteAdmin(adminID); err != nil {
		Error(w, http.StatusInternalServerError, "failed to delete admin")
		return
	}

	JSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *AuthHandler) handleResetPassword(w http.ResponseWriter, r *http.Request, adminID string) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims := middleware.GetClaims(r)
	if claims == nil {
		Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	currentAdmin := h.storage.GetAdminByID(claims.UserID)
	if currentAdmin == nil || !currentAdmin.IsSuper() {
		Error(w, http.StatusForbidden, "only super admins can reset passwords")
		return
	}

	var req ResetPasswordRequest
	if err := ParseJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.NewPassword) < 6 {
		Error(w, http.StatusBadRequest, "password must be at least 6 characters")
		return
	}

	if err := h.authSvc.ResetAdminPassword(adminID, req.NewPassword); err != nil {
		if err == services.ErrUserNotFound {
			Error(w, http.StatusNotFound, "admin not found")
			return
		}
		Error(w, http.StatusInternalServerError, "failed to reset password")
		return
	}

	JSON(w, http.StatusOK, map[string]bool{"success": true})
}
