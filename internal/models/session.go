package models

import "time"

// Session represents an active internet session
type Session struct {
	ID           string    `json:"id"`
	ChildID      string    `json:"child_id"`
	ChildName    string    `json:"child_name"`
	MAC          string    `json:"mac"`
	IP           string    `json:"ip"`
	StartedAt    time.Time `json:"started_at"`
	LastTickAt   time.Time `json:"last_tick_at"`
	IsActive     bool      `json:"is_active"`
	SessionToken string    `json:"session_token,omitempty"` // OpenNDS token
}

// DurationMinutes returns how long this session has been active
func (s *Session) DurationMinutes() int {
	return int(time.Since(s.StartedAt).Minutes())
}
