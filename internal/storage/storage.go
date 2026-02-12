package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"parenta/internal/models"
)

// Storage handles all data persistence using JSON files
type Storage struct {
	dataDir string
	mu      sync.RWMutex

	// In-memory cache
	admins    []*models.User // Multiple admin support
	children  []*models.Child
	sessions  []*models.Session
	schedules []*models.Schedule
	filters   []*models.FilterRule
}

// New creates a new Storage instance
func New(dataDir string) (*Storage, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	s := &Storage{
		dataDir:   dataDir,
		admins:    make([]*models.User, 0),
		children:  make([]*models.Child, 0),
		sessions:  make([]*models.Session, 0),
		schedules: make([]*models.Schedule, 0),
		filters:   make([]*models.FilterRule, 0),
	}

	// Load existing data
	if err := s.loadAll(); err != nil {
		return nil, err
	}

	return s, nil
}

// loadAll loads all data from JSON files
func (s *Storage) loadAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Load admins - supports both array format and legacy single object
	if data, err := os.ReadFile(s.filePath("admin.json")); err == nil {
		// Try to parse as array first (new format)
		var admins []*models.User
		if err := json.Unmarshal(data, &admins); err == nil {
			s.admins = admins
		} else {
			// Fall back to single object (legacy format) and convert to array
			var admin models.User
			if err := json.Unmarshal(data, &admin); err == nil {
				// Ensure legacy admin has a role
				if admin.Role == "" {
					admin.Role = models.RoleSuper
				}
				s.admins = []*models.User{&admin}
				// Save in new format
				s.saveFile("admin.json", s.admins)
			}
		}
	}

	// Load children
	if data, err := os.ReadFile(s.filePath("children.json")); err == nil {
		json.Unmarshal(data, &s.children)
	}

	// Load sessions
	if data, err := os.ReadFile(s.filePath("sessions.json")); err == nil {
		json.Unmarshal(data, &s.sessions)
	}

	// Load schedules
	if data, err := os.ReadFile(s.filePath("schedules.json")); err == nil {
		json.Unmarshal(data, &s.schedules)
	}

	// Load filters
	if data, err := os.ReadFile(s.filePath("filters.json")); err == nil {
		json.Unmarshal(data, &s.filters)
	}

	return nil
}

// filePath returns the full path for a data file
func (s *Storage) filePath(filename string) string {
	return filepath.Join(s.dataDir, filename)
}

// saveFile atomically writes data to a JSON file
func (s *Storage) saveFile(filename string, data interface{}) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	path := s.filePath(filename)
	tmpPath := path + ".tmp"

	// Write to temp file first
	if err := os.WriteFile(tmpPath, jsonData, 0644); err != nil {
		return err
	}

	// Atomic rename
	return os.Rename(tmpPath, path)
}

// ============ Admin Methods ============

// ListAdmins returns all admin users
func (s *Storage) ListAdmins() []*models.User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*models.User, len(s.admins))
	copy(result, s.admins)
	return result
}

// GetAdmin returns the first admin (for backward compatibility)
func (s *Storage) GetAdmin() *models.User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.admins) > 0 {
		return s.admins[0]
	}
	return nil
}

// GetAdminByID returns an admin by ID
func (s *Storage) GetAdminByID(id string) *models.User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, a := range s.admins {
		if a.ID == id {
			return a
		}
	}
	return nil
}

// GetAdminByUsername returns an admin by username
func (s *Storage) GetAdminByUsername(username string) *models.User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, a := range s.admins {
		if a.Username == username {
			return a
		}
	}
	return nil
}

// SaveAdmin saves or updates an admin user
func (s *Storage) SaveAdmin(admin *models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update existing or append
	found := false
	for i, a := range s.admins {
		if a.ID == admin.ID {
			s.admins[i] = admin
			found = true
			break
		}
	}
	if !found {
		s.admins = append(s.admins, admin)
	}

	return s.saveFile("admin.json", s.admins)
}

// DeleteAdmin removes an admin by ID (prevents deleting last admin)
func (s *Storage) DeleteAdmin(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Prevent deleting the last admin
	if len(s.admins) <= 1 {
		return nil // Silently ignore - can't delete last admin
	}

	for i, a := range s.admins {
		if a.ID == id {
			s.admins = append(s.admins[:i], s.admins[i+1:]...)
			break
		}
	}

	return s.saveFile("admin.json", s.admins)
}

// AdminCount returns the number of admins
func (s *Storage) AdminCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.admins)
}

// ============ Children Methods ============

// ListChildren returns all children
func (s *Storage) ListChildren() []*models.Child {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*models.Child, len(s.children))
	copy(result, s.children)
	return result
}

// GetChild returns a child by ID
func (s *Storage) GetChild(id string) *models.Child {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, c := range s.children {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// GetChildByUsername returns a child by username
func (s *Storage) GetChildByUsername(username string) *models.Child {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, c := range s.children {
		if c.Username == username {
			return c
		}
	}
	return nil
}

// GetChildByMAC returns a child who has this MAC registered
func (s *Storage) GetChildByMAC(mac string) *models.Child {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, c := range s.children {
		if c.HasDevice(mac) {
			return c
		}
	}
	return nil
}

// SaveChild saves or updates a child
func (s *Storage) SaveChild(child *models.Child) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update existing or append
	found := false
	for i, c := range s.children {
		if c.ID == child.ID {
			s.children[i] = child
			found = true
			break
		}
	}
	if !found {
		s.children = append(s.children, child)
	}

	return s.saveFile("children.json", s.children)
}

// DeleteChild removes a child by ID
func (s *Storage) DeleteChild(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, c := range s.children {
		if c.ID == id {
			s.children = append(s.children[:i], s.children[i+1:]...)
			break
		}
	}

	return s.saveFile("children.json", s.children)
}

// ============ Session Methods ============

// ListSessions returns all active sessions
func (s *Storage) ListSessions() []*models.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*models.Session, 0)
	for _, sess := range s.sessions {
		if sess.IsActive {
			result = append(result, sess)
		}
	}
	return result
}

// GetSession returns a session by ID
func (s *Storage) GetSession(id string) *models.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, sess := range s.sessions {
		if sess.ID == id {
			return sess
		}
	}
	return nil
}

// GetSessionByMAC returns an active session for a MAC address
func (s *Storage) GetSessionByMAC(mac string) *models.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, sess := range s.sessions {
		if sess.MAC == mac && sess.IsActive {
			return sess
		}
	}
	return nil
}

// SaveSession saves or updates a session
func (s *Storage) SaveSession(session *models.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	for i, sess := range s.sessions {
		if sess.ID == session.ID {
			s.sessions[i] = session
			found = true
			break
		}
	}
	if !found {
		s.sessions = append(s.sessions, session)
	}

	return s.saveFile("sessions.json", s.sessions)
}

// DeleteSession removes a session by ID
func (s *Storage) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, sess := range s.sessions {
		if sess.ID == id {
			s.sessions = append(s.sessions[:i], s.sessions[i+1:]...)
			break
		}
	}

	return s.saveFile("sessions.json", s.sessions)
}

// ============ Schedule Methods ============

// ListSchedules returns all schedules
func (s *Storage) ListSchedules() []*models.Schedule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*models.Schedule, len(s.schedules))
	copy(result, s.schedules)
	return result
}

// GetSchedule returns a schedule by ID
func (s *Storage) GetSchedule(id string) *models.Schedule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, sched := range s.schedules {
		if sched.ID == id {
			return sched
		}
	}
	return nil
}

// SaveSchedule saves or updates a schedule
func (s *Storage) SaveSchedule(schedule *models.Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	for i, sched := range s.schedules {
		if sched.ID == schedule.ID {
			s.schedules[i] = schedule
			found = true
			break
		}
	}
	if !found {
		s.schedules = append(s.schedules, schedule)
	}

	return s.saveFile("schedules.json", s.schedules)
}

// DeleteSchedule removes a schedule by ID
func (s *Storage) DeleteSchedule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, sched := range s.schedules {
		if sched.ID == id {
			s.schedules = append(s.schedules[:i], s.schedules[i+1:]...)
			break
		}
	}

	return s.saveFile("schedules.json", s.schedules)
}

// ============ Filter Methods ============

// ListFilters returns all filter rules, optionally filtered by type
func (s *Storage) ListFilters(ruleType models.RuleType) []*models.FilterRule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*models.FilterRule, 0)
	for _, f := range s.filters {
		if ruleType == "" || f.RuleType == ruleType {
			result = append(result, f)
		}
	}
	return result
}

// SaveFilter saves or updates a filter rule
func (s *Storage) SaveFilter(filter *models.FilterRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	for i, f := range s.filters {
		if f.ID == filter.ID {
			s.filters[i] = filter
			found = true
			break
		}
	}
	if !found {
		s.filters = append(s.filters, filter)
	}

	return s.saveFile("filters.json", s.filters)
}

// DeleteFilter removes a filter by ID
func (s *Storage) DeleteFilter(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, f := range s.filters {
		if f.ID == id {
			s.filters = append(s.filters[:i], s.filters[i+1:]...)
			break
		}
	}

	return s.saveFile("filters.json", s.filters)
}

// ============ Utility Methods ============

// ResetDailyQuotas resets all children's used_today to 0
func (s *Storage) ResetDailyQuotas(dateStr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, c := range s.children {
		c.UsedTodayMin = 0
		c.LastResetDate = dateStr
	}

	return s.saveFile("children.json", s.children)
}

// ClearInactiveSessions removes all inactive sessions
func (s *Storage) ClearInactiveSessions() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	active := make([]*models.Session, 0)
	for _, sess := range s.sessions {
		if sess.IsActive {
			active = append(active, sess)
		}
	}
	s.sessions = active

	return s.saveFile("sessions.json", s.sessions)
}
