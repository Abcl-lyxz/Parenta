package services

import (
	"log"
	"time"

	"parenta/internal/models"
	"parenta/internal/storage"
)

// SessionTicker periodically checks sessions and enforces quotas
type SessionTicker struct {
	storage  *storage.Storage
	ndsctl   *NDSCtl
	interval time.Duration
	stopChan chan struct{}
	doneChan chan struct{}
}

// NewSessionTicker creates a new SessionTicker
func NewSessionTicker(store *storage.Storage, ndsctl *NDSCtl, intervalSeconds int) *SessionTicker {
	return &SessionTicker{
		storage:  store,
		ndsctl:   ndsctl,
		interval: time.Duration(intervalSeconds) * time.Second,
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
}

// Start begins the ticker loop
func (t *SessionTicker) Start() {
	ticker := time.NewTicker(t.interval)
	go func() {
		defer close(t.doneChan)
		for {
			select {
			case <-ticker.C:
				t.tick()
			case <-t.stopChan:
				ticker.Stop()
				return
			}
		}
	}()
	log.Printf("Session ticker started (interval: %v)", t.interval)
}

// Stop stops the ticker loop
func (t *SessionTicker) Stop() {
	close(t.stopChan)
	<-t.doneChan
	log.Println("Session ticker stopped")
}

// tick performs one quota check cycle
func (t *SessionTicker) tick() {
	now := time.Now()

	// Check for daily reset
	t.checkDailyReset(now)

	// Get all active sessions
	sessions := t.storage.ListSessions()

	for _, session := range sessions {
		if !session.IsActive {
			continue
		}

		child := t.storage.GetChild(session.ChildID)
		if child == nil {
			// Child was deleted, deauth this session
			t.deauthSession(session, "child_deleted")
			continue
		}

		// Calculate minutes since last tick
		var minutesToAdd int
		if session.LastTickAt.IsZero() {
			minutesToAdd = int(now.Sub(session.StartedAt).Minutes())
		} else {
			minutesToAdd = int(now.Sub(session.LastTickAt).Minutes())
		}

		// Update usage
		if minutesToAdd > 0 {
			child.UsedTodayMin += minutesToAdd
			child.UpdatedAt = now
			t.storage.SaveChild(child)

			session.LastTickAt = now
			t.storage.SaveSession(session)
		}

		// Check quota exceeded
		if child.UsedTodayMin >= child.DailyQuotaMin {
			t.deauthSession(session, "quota_exceeded")
			continue
		}

		// Check schedule (if child has a schedule assigned)
		if child.ScheduleID != "" {
			schedule := t.storage.GetSchedule(child.ScheduleID)
			if schedule != nil && !schedule.IsAllowedNow() {
				t.deauthSession(session, "schedule_ended")
				continue
			}
		}
	}
}

// deauthSession deauthenticates a session and marks it inactive
func (t *SessionTicker) deauthSession(session *models.Session, reason string) {
	log.Printf("Deauthenticating %s (child: %s): %s", session.MAC, session.ChildName, reason)

	// Call ndsctl deauth
	if err := t.ndsctl.Deauth(session.MAC); err != nil {
		log.Printf("ndsctl deauth error for %s: %v", session.MAC, err)
	}

	// Mark session as inactive
	session.IsActive = false
	t.storage.SaveSession(session)
}

// checkDailyReset checks if we need to reset daily quotas
func (t *SessionTicker) checkDailyReset(now time.Time) {
	todayStr := now.Format("2006-01-02")

	// Check if any child needs reset
	children := t.storage.ListChildren()
	needsReset := false

	for _, child := range children {
		if child.LastResetDate != todayStr {
			needsReset = true
			break
		}
	}

	if needsReset {
		log.Printf("Resetting daily quotas for %s", todayStr)
		t.storage.ResetDailyQuotas(todayStr)
	}
}
