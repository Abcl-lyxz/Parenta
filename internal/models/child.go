package models

import "time"

// FilterMode defines the filtering behavior
type FilterMode string

const (
	FilterModeStudy  FilterMode = "study"  // Whitelist only
	FilterModeNormal FilterMode = "normal" // Blacklist blocked
)

// Device represents a MAC-bound device
type Device struct {
	MAC       string    `json:"mac"`
	Name      string    `json:"name"`
	FirstSeen time.Time `json:"first_seen"`
}

// Child represents a child user profile
type Child struct {
	ID            string     `json:"id"`
	Username      string     `json:"username"`
	PasswordHash  string     `json:"password_hash"`
	Name          string     `json:"name"`
	DailyQuotaMin int        `json:"daily_quota_min"`
	UsedTodayMin  int        `json:"used_today_min"`
	FilterMode    FilterMode `json:"filter_mode"`
	ScheduleID    string     `json:"schedule_id"`
	Devices       []Device   `json:"devices"`
	LastResetDate string     `json:"last_reset_date"` // YYYY-MM-DD
	IsActive      bool       `json:"is_active"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// RemainingMinutes returns the remaining quota for today
func (c *Child) RemainingMinutes() int {
	remaining := c.DailyQuotaMin - c.UsedTodayMin
	if remaining < 0 {
		return 0
	}
	return remaining
}

// HasDevice checks if a MAC is registered to this child
func (c *Child) HasDevice(mac string) bool {
	for _, d := range c.Devices {
		if d.MAC == mac {
			return true
		}
	}
	return false
}

// AddDevice adds a new device to the child's device list
func (c *Child) AddDevice(mac, name string) {
	if c.HasDevice(mac) {
		return
	}
	c.Devices = append(c.Devices, Device{
		MAC:       mac,
		Name:      name,
		FirstSeen: time.Now(),
	})
}
