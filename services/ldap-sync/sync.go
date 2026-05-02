package main

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"go.uber.org/zap"
)

// User represents a synced LDAP user.
type User struct {
	DN          string    `json:"dn"`
	UID         string    `json:"uid"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	Groups      []string  `json:"groups"`
	SyncedAt    time.Time `json:"syncedAt"`
	Active      bool      `json:"active"`
}

// MarshalJSON returns the JSON representation of a User.
func (u *User) MarshalJSON() ([]byte, error) {
	type Alias User
	return json.Marshal(&struct {
		SyncedAt string `json:"syncedAt"`
		*Alias
	}{
		SyncedAt: u.SyncedAt.Format(time.RFC3339),
		Alias:    (*Alias)(u),
	})
}

// Syncer polls LDAP/AD directory and maintains an in-memory user store.
type Syncer struct {
	mu       sync.RWMutex
	users    map[string]*User // key: uid
	ldapURL  string
	interval time.Duration
	logger   *zap.Logger
}

// NewSyncer creates a new Syncer instance.
func NewSyncer(ldapURL string, interval time.Duration, logger *zap.Logger) *Syncer {
	return &Syncer{
		users:    make(map[string]*User),
		ldapURL:  ldapURL,
		interval: interval,
		logger:   logger,
	}
}

// Run starts the sync loop. Syncs on startup, then every interval (default 1h).
func (s *Syncer) Run(ctx context.Context) error {
	s.logger.Info("LDAP syncer started", zap.String("ldapURL", s.ldapURL), zap.Duration("interval", s.interval))

	// Perform initial sync
	if err := s.sync(ctx); err != nil {
		s.logger.Error("Initial sync failed", zap.Error(err))
		// Continue despite error - sync will be retried on next interval
	}

	// Setup ticker for periodic syncs
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("LDAP syncer context cancelled")
			return ctx.Err()
		case <-ticker.C:
			if err := s.sync(ctx); err != nil {
				s.logger.Error("Sync cycle failed", zap.Error(err))
				// Continue despite error - next sync will retry
			}
		}
	}
}

// sync performs one sync cycle.
// If ldapURL is empty: generates stub users for development.
// Real impl: dial LDAP, search for users, parse attributes.
func (s *Syncer) sync(ctx context.Context) error {
	s.logger.Debug("Starting sync cycle")

	// For P8, stub implementation: generate stub users if LDAP URL is empty
	if s.ldapURL == "" {
		s.logger.Debug("No LDAP URL configured; using stub users")
		s.populateStubUsers()
		return nil
	}

	// In production: dial LDAP, search for users, parse attributes
	// For now, stub implementation for any configured LDAP URL
	s.logger.Debug("LDAP sync not yet implemented; using stub users",
		zap.String("ldapURL", s.ldapURL))
	s.populateStubUsers()
	return nil
}

// populateStubUsers populates the user store with stub users for development.
func (s *Syncer) populateStubUsers() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	s.users["admin"] = &User{
		DN:          "cn=admin,ou=users,dc=nest,dc=local",
		UID:         "admin",
		Email:       "admin@nest.local",
		DisplayName: "Nest Admin",
		Groups:      []string{"admins", "nest-operators"},
		SyncedAt:    now,
		Active:      true,
	}

	s.users["viewer"] = &User{
		DN:          "cn=viewer,ou=users,dc=nest,dc=local",
		UID:         "viewer",
		Email:       "viewer@nest.local",
		DisplayName: "Nest Viewer",
		Groups:      []string{"viewers"},
		SyncedAt:    now,
		Active:      true,
	}

	s.logger.Debug("Populated stub users", zap.Int("count", len(s.users)))
}

// ListUsers returns all synced users.
func (s *Syncer) ListUsers() []*User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]*User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}
	return users
}

// GetUser returns a user by UID.
func (s *Syncer) GetUser(uid string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, found := s.users[uid]
	return user, found
}
