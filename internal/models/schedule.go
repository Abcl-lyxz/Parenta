package models

import (
	"time"
)

// TimeBlock represents a time period within a schedule
type TimeBlock struct {
	DayOfWeek  int        `json:"day_of_week"` // 0=Sunday, 6=Saturday
	StartTime  string     `json:"start_time"`  // "HH:MM" format
	EndTime    string     `json:"end_time"`    // "HH:MM" format
	FilterMode FilterMode `json:"filter_mode"` // Override mode for this block
}

// Schedule represents a weekly time schedule
type Schedule struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	TimeBlocks []TimeBlock `json:"time_blocks"`
	IsDefault  bool        `json:"is_default"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

// IsAllowedNow checks if current time falls within any allowed block
func (s *Schedule) IsAllowedNow() bool {
	now := time.Now()
	dayOfWeek := int(now.Weekday())
	currentTime := now.Format("15:04")

	for _, block := range s.TimeBlocks {
		if block.DayOfWeek == dayOfWeek {
			if currentTime >= block.StartTime && currentTime <= block.EndTime {
				return true
			}
		}
	}
	return false
}

// GetCurrentFilterMode returns the filter mode for the current time block
func (s *Schedule) GetCurrentFilterMode() FilterMode {
	now := time.Now()
	dayOfWeek := int(now.Weekday())
	currentTime := now.Format("15:04")

	for _, block := range s.TimeBlocks {
		if block.DayOfWeek == dayOfWeek {
			if currentTime >= block.StartTime && currentTime <= block.EndTime {
				return block.FilterMode
			}
		}
	}
	return FilterModeNormal // Default to normal if no block matches
}
