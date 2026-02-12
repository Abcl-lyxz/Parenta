package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"

	"parenta/internal/models"
	"parenta/internal/storage"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
)

// AuthService handles authentication
type AuthService struct {
	storage      *storage.Storage
	jwtSecret    []byte
	jwtExpiryHrs int
}

// NewAuthService creates a new AuthService
func NewAuthService(store *storage.Storage, jwtSecret string, jwtExpiryHrs int) *AuthService {
	return &AuthService{
		storage:      store,
		jwtSecret:    []byte(jwtSecret),
		jwtExpiryHrs: jwtExpiryHrs,
	}
}

// HashPassword creates a bcrypt hash of a password
func HashPassword(password string) (string, error) {
	// Use cost of 10 for embedded systems (balance security/performance)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword verifies a password against a hash
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// InitializeAdmin creates the default admin user if none exists
func (a *AuthService) InitializeAdmin(username, password string, forceChange bool) error {
	// Check if any admin exists
	if a.storage.AdminCount() > 0 {
		return nil // Admin already exists
	}

	hash, err := HashPassword(password)
	if err != nil {
		return err
	}

	admin := &models.User{
		ID:                  GenerateID(),
		Username:            username,
		PasswordHash:        hash,
		DisplayName:         "Administrator",
		Role:                models.RoleSuper, // First admin is always super
		ForcePasswordChange: forceChange,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	return a.storage.SaveAdmin(admin)
}

// AuthenticateAdmin verifies admin credentials
func (a *AuthService) AuthenticateAdmin(username, password string) (*models.User, error) {
	admin := a.storage.GetAdminByUsername(username)
	if admin == nil {
		return nil, ErrInvalidCredentials
	}

	if !CheckPassword(password, admin.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	return admin, nil
}

// AuthenticateChild verifies child credentials
func (a *AuthService) AuthenticateChild(username, password string) (*models.Child, error) {
	child := a.storage.GetChildByUsername(username)
	if child == nil {
		return nil, ErrUserNotFound
	}

	if !CheckPassword(password, child.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	if !child.IsActive {
		return nil, errors.New("account is disabled")
	}

	return child, nil
}

// ChangeAdminPassword updates an admin's password (user changes their own password)
func (a *AuthService) ChangeAdminPassword(adminID, oldPassword, newPassword string) error {
	admin := a.storage.GetAdminByID(adminID)
	if admin == nil {
		return ErrUserNotFound
	}

	if !CheckPassword(oldPassword, admin.PasswordHash) {
		return ErrInvalidCredentials
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	admin.PasswordHash = hash
	admin.ForcePasswordChange = false
	admin.UpdatedAt = time.Now()

	return a.storage.SaveAdmin(admin)
}

// CreateAdmin creates a new admin user (only super admins can do this)
func (a *AuthService) CreateAdmin(username, password, displayName string, role models.UserRole) (*models.User, error) {
	// Check if username already exists
	if existing := a.storage.GetAdminByUsername(username); existing != nil {
		return nil, errors.New("username already exists")
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}

	admin := &models.User{
		ID:                  GenerateID(),
		Username:            username,
		PasswordHash:        hash,
		DisplayName:         displayName,
		Role:                role,
		ForcePasswordChange: true, // Force new admins to change password
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	if err := a.storage.SaveAdmin(admin); err != nil {
		return nil, err
	}

	return admin, nil
}

// UpdateAdmin updates an admin's non-password fields
func (a *AuthService) UpdateAdmin(id, displayName string, role models.UserRole) error {
	admin := a.storage.GetAdminByID(id)
	if admin == nil {
		return ErrUserNotFound
	}

	admin.DisplayName = displayName
	admin.Role = role
	admin.UpdatedAt = time.Now()

	return a.storage.SaveAdmin(admin)
}

// ResetAdminPassword resets an admin's password (super admin function)
func (a *AuthService) ResetAdminPassword(adminID, newPassword string) error {
	admin := a.storage.GetAdminByID(adminID)
	if admin == nil {
		return ErrUserNotFound
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	admin.PasswordHash = hash
	admin.ForcePasswordChange = true // Force password change on next login
	admin.UpdatedAt = time.Now()

	return a.storage.SaveAdmin(admin)
}

// GenerateID creates a random 16-character hex ID
func GenerateID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GenerateToken creates a random session token
func GenerateToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
