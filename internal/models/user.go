package models

import "time"

// UserRole represents the admin role type
type UserRole string

const (
	RoleSuper UserRole = "super" // Can manage other admins
	RoleAdmin UserRole = "admin" // Regular admin
)

// User represents the parent/admin user
type User struct {
	ID                  string    `json:"id"`
	Username            string    `json:"username"`
	PasswordHash        string    `json:"password_hash"`
	DisplayName         string    `json:"display_name,omitempty"`
	Role                UserRole  `json:"role"`
	ForcePasswordChange bool      `json:"force_password_change"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// IsSuper returns true if the user has super admin privileges
func (u *User) IsSuper() bool {
	return u.Role == RoleSuper
}

// GetDisplayName returns DisplayName if set, otherwise Username
func (u *User) GetDisplayName() string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Username
}
