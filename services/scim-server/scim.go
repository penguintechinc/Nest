package main

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// SCIMUser represents a SCIM User resource.
type SCIMUser struct {
	ID          string         `json:"id"`
	ExternalID  string         `json:"externalId,omitempty"`
	UserName    string         `json:"userName"`
	DisplayName string         `json:"displayName,omitempty"`
	Emails      []SCIMEmail    `json:"emails,omitempty"`
	Active      bool           `json:"active"`
	Groups      []SCIMGroupRef `json:"groups,omitempty"`
	Meta        SCIMMeta       `json:"meta"`
}

type SCIMEmail struct {
	Value   string `json:"value"`
	Primary bool   `json:"primary"`
	Type    string `json:"type,omitempty"`
}

type SCIMGroupRef struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
}

type SCIMMeta struct {
	ResourceType string `json:"resourceType"`
	Created      string `json:"created"`
	LastModified string `json:"lastModified"`
	Version      string `json:"version,omitempty"`
}

// SCIMGroup represents a SCIM Group resource.
type SCIMGroup struct {
	ID          string       `json:"id"`
	DisplayName string       `json:"displayName"`
	Members     []SCIMMember `json:"members,omitempty"`
	Meta        SCIMMeta     `json:"meta"`
}

type SCIMMember struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
}

// SCIMStore is the in-memory SCIM resource store.
type SCIMStore struct {
	mu     sync.RWMutex
	users  map[string]*SCIMUser
	groups map[string]*SCIMGroup
	logger *zap.Logger
}

func NewSCIMStore(logger *zap.Logger) *SCIMStore {
	return &SCIMStore{
		users:  make(map[string]*SCIMUser),
		groups: make(map[string]*SCIMGroup),
		logger: logger,
	}
}

// User CRUD

func (s *SCIMStore) CreateUser(u *SCIMUser) *SCIMUser {
	s.mu.Lock()
	defer s.mu.Unlock()

	u.ID = fmt.Sprintf("scim-%d", time.Now().UnixNano())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	u.Meta = SCIMMeta{
		ResourceType: "User",
		Created:      now,
		LastModified: now,
		Version:      "1",
	}

	s.users[u.ID] = u
	s.logger.Info("user created", zap.String("id", u.ID), zap.String("userName", u.UserName))

	return u
}

func (s *SCIMStore) GetUser(id string) (*SCIMUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u, ok := s.users[id]
	if !ok {
		return nil, fmt.Errorf("user not found: %s", id)
	}

	return u, nil
}

func (s *SCIMStore) ListUsers() []*SCIMUser {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*SCIMUser, 0, len(s.users))
	for _, u := range s.users {
		result = append(result, u)
	}

	return result
}

func (s *SCIMStore) UpdateUser(id string, u *SCIMUser) (*SCIMUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.users[id]
	if !ok {
		return nil, fmt.Errorf("user not found: %s", id)
	}

	u.ID = existing.ID
	u.Meta = SCIMMeta{
		ResourceType: "User",
		Created:      existing.Meta.Created,
		LastModified: time.Now().UTC().Format(time.RFC3339Nano),
		Version:      "1",
	}

	s.users[id] = u
	s.logger.Info("user updated", zap.String("id", id), zap.String("userName", u.UserName))

	return u, nil
}

func (s *SCIMStore) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[id]; !ok {
		return fmt.Errorf("user not found: %s", id)
	}

	delete(s.users, id)
	s.logger.Info("user deleted", zap.String("id", id))

	return nil
}

// Group CRUD

func (s *SCIMStore) CreateGroup(g *SCIMGroup) *SCIMGroup {
	s.mu.Lock()
	defer s.mu.Unlock()

	g.ID = fmt.Sprintf("scim-%d", time.Now().UnixNano())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	g.Meta = SCIMMeta{
		ResourceType: "Group",
		Created:      now,
		LastModified: now,
		Version:      "1",
	}

	s.groups[g.ID] = g
	s.logger.Info("group created", zap.String("id", g.ID), zap.String("displayName", g.DisplayName))

	return g
}

func (s *SCIMStore) GetGroup(id string) (*SCIMGroup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	g, ok := s.groups[id]
	if !ok {
		return nil, fmt.Errorf("group not found: %s", id)
	}

	return g, nil
}

func (s *SCIMStore) ListGroups() []*SCIMGroup {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*SCIMGroup, 0, len(s.groups))
	for _, g := range s.groups {
		result = append(result, g)
	}

	return result
}

func (s *SCIMStore) DeleteGroup(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.groups[id]; !ok {
		return fmt.Errorf("group not found: %s", id)
	}

	delete(s.groups, id)
	s.logger.Info("group deleted", zap.String("id", id))

	return nil
}
