package models

import "time"

// RuleType defines whether a rule is whitelist or blacklist
type RuleType string

const (
	RuleTypeWhitelist RuleType = "whitelist"
	RuleTypeBlacklist RuleType = "blacklist"
)

// FilterRule represents a domain filtering rule
type FilterRule struct {
	ID        string    `json:"id"`
	Domain    string    `json:"domain"`    // e.g., "youtube.com" or "*.youtube.com"
	RuleType  RuleType  `json:"rule_type"` // "whitelist" or "blacklist"
	Category  string    `json:"category"`  // "social", "games", "education", etc.
	CreatedAt time.Time `json:"created_at"`
}
