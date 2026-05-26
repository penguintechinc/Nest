package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/penguintechinc/nest/shared/database"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

// SCIMUser represents a SCIM User resource.
type SCIMUser struct {
	ID          string        `json:"id"`
	ExternalID  string        `json:"externalId,omitempty"`
	UserName    string        `json:"userName"`
	DisplayName string        `json:"displayName,omitempty"`
	Emails      []SCIMEmail   `json:"emails,omitempty"`
	Active      bool          `json:"active"`
	Groups      []SCIMGroupRef `json:"groups,omitempty"`
	Meta        SCIMMeta      `json:"meta"`
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

// SCIMStore is a persistent SCIM resource store powered by PenguinDAL.
type SCIMStore struct {
	dal    *database.PenguinDAL
	logger *zap.Logger
}

func NewSCIMStore(dal *database.PenguinDAL, logger *zap.Logger) *SCIMStore {
	dal.DefineTable(&database.SCIMUser{})
	dal.DefineTable(&database.SCIMGroup{})
	return &SCIMStore{
		dal:    dal,
		logger: logger,
	}
}

// User CRUD

func (s *SCIMStore) CreateUser(tenant string, u *SCIMUser) *SCIMUser {
	id := s.dal.UUID()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)

	emails, _ := json.Marshal(u.Emails)
	groups, _ := json.Marshal(u.Groups)

	dbUser := &database.SCIMUser{
		ID:           id,
		ExternalID:   u.ExternalID,
		Tenant:       tenant,
		UserName:     u.UserName,
		DisplayName:  u.DisplayName,
		Emails:       datatypes.JSON(emails),
		Active:       u.Active,
		Groups:       datatypes.JSON(groups),
		ResourceType: "User",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.dal.Insert(dbUser); err != nil {
		s.logger.Error("failed to create SCIM user", zap.Error(err))
		return nil
	}

	u.ID = id
	u.Meta = SCIMMeta{
		ResourceType: "User",
		Created:      nowStr,
		LastModified: nowStr,
		Version:      "1",
	}

	s.logger.Info("user created", zap.String("id", u.ID), zap.String("tenant", tenant), zap.String("userName", u.UserName))
	return u
}

func (s *SCIMStore) GetUser(tenant, id string) (*SCIMUser, error) {
	var dbUser database.SCIMUser
	if err := s.dal.Query().Where("id = ? AND tenant = ?", id, tenant).First(&dbUser).Error; err != nil {
		return nil, fmt.Errorf("user not found: %s", id)
	}

	var emails []SCIMEmail
	var groups []SCIMGroupRef
	json.Unmarshal(dbUser.Emails, &emails)
	json.Unmarshal(dbUser.Groups, &groups)

	return &SCIMUser{
		ID:          dbUser.ID,
		ExternalID:  dbUser.ExternalID,
		UserName:    dbUser.UserName,
		DisplayName: dbUser.DisplayName,
		Emails:      emails,
		Active:      dbUser.Active,
		Groups:      groups,
		Meta: SCIMMeta{
			ResourceType: dbUser.ResourceType,
			Created:      dbUser.CreatedAt.Format(time.RFC3339Nano),
			LastModified: dbUser.UpdatedAt.Format(time.RFC3339Nano),
			Version:      "1",
		},
	}, nil
}

func (s *SCIMStore) ListUsers(tenant string) []*SCIMUser {
	var dbUsers []database.SCIMUser
	s.dal.Query().Where("tenant = ?", tenant).Find(&dbUsers)

	result := make([]*SCIMUser, len(dbUsers))
	for i, dbUser := range dbUsers {
		var emails []SCIMEmail
		var groups []SCIMGroupRef
		json.Unmarshal(dbUser.Emails, &emails)
		json.Unmarshal(dbUser.Groups, &groups)

		result[i] = &SCIMUser{
			ID:          dbUser.ID,
			ExternalID:  dbUser.ExternalID,
			UserName:    dbUser.UserName,
			DisplayName: dbUser.DisplayName,
			Emails:      emails,
			Active:      dbUser.Active,
			Groups:      groups,
			Meta: SCIMMeta{
				ResourceType: dbUser.ResourceType,
				Created:      dbUser.CreatedAt.Format(time.RFC3339Nano),
				LastModified: dbUser.UpdatedAt.Format(time.RFC3339Nano),
				Version:      "1",
			},
		}
	}

	return result
}

func (s *SCIMStore) UpdateUser(tenant, id string, u *SCIMUser) (*SCIMUser, error) {
	var dbUser database.SCIMUser
	if err := s.dal.Query().Where("id = ? AND tenant = ?", id, tenant).First(&dbUser).Error; err != nil {
		return nil, fmt.Errorf("user not found: %s", id)
	}

	now := time.Now().UTC()
	emails, _ := json.Marshal(u.Emails)
	groups, _ := json.Marshal(u.Groups)

	dbUser.ExternalID = u.ExternalID
	dbUser.UserName = u.UserName
	dbUser.DisplayName = u.DisplayName
	dbUser.Emails = datatypes.JSON(emails)
	dbUser.Active = u.Active
	dbUser.Groups = datatypes.JSON(groups)
	dbUser.UpdatedAt = now

	if err := s.dal.Update(&dbUser); err != nil {
		s.logger.Error("failed to update SCIM user", zap.Error(err))
		return nil, err
	}

	u.ID = id
	u.Meta = SCIMMeta{
		ResourceType: "User",
		Created:      dbUser.CreatedAt.Format(time.RFC3339Nano),
		LastModified: now.Format(time.RFC3339Nano),
		Version:      "1",
	}

	s.logger.Info("user updated", zap.String("id", id), zap.String("tenant", tenant), zap.String("userName", u.UserName))
	return u, nil
}

func (s *SCIMStore) DeleteUser(tenant, id string) error {
	result := s.dal.Query().Where("id = ? AND tenant = ?", id, tenant).Delete(&database.SCIMUser{})
	if result.Error != nil || result.RowsAffected == 0 {
		return fmt.Errorf("user not found: %s", id)
	}

	s.logger.Info("user deleted", zap.String("id", id), zap.String("tenant", tenant))
	return nil
}

// Group CRUD

func (s *SCIMStore) CreateGroup(tenant string, g *SCIMGroup) *SCIMGroup {
	id := s.dal.UUID()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)

	members, _ := json.Marshal(g.Members)

	dbGroup := &database.SCIMGroup{
		ID:           id,
		Tenant:       tenant,
		DisplayName:  g.DisplayName,
		Members:      datatypes.JSON(members),
		ResourceType: "Group",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.dal.Insert(dbGroup); err != nil {
		s.logger.Error("failed to create SCIM group", zap.Error(err))
		return nil
	}

	g.ID = id
	g.Meta = SCIMMeta{
		ResourceType: "Group",
		Created:      nowStr,
		LastModified: nowStr,
		Version:      "1",
	}

	s.logger.Info("group created", zap.String("id", g.ID), zap.String("tenant", tenant), zap.String("displayName", g.DisplayName))
	return g
}

func (s *SCIMStore) GetGroup(tenant, id string) (*SCIMGroup, error) {
	var dbGroup database.SCIMGroup
	if err := s.dal.Query().Where("id = ? AND tenant = ?", id, tenant).First(&dbGroup).Error; err != nil {
		return nil, fmt.Errorf("group not found: %s", id)
	}

	var members []SCIMMember
	json.Unmarshal(dbGroup.Members, &members)

	return &SCIMGroup{
		ID:          dbGroup.ID,
		DisplayName: dbGroup.DisplayName,
		Members:     members,
		Meta: SCIMMeta{
			ResourceType: dbGroup.ResourceType,
			Created:      dbGroup.CreatedAt.Format(time.RFC3339Nano),
			LastModified: dbGroup.UpdatedAt.Format(time.RFC3339Nano),
			Version:      "1",
		},
	}, nil
}

func (s *SCIMStore) ListGroups(tenant string) []*SCIMGroup {
	var dbGroups []database.SCIMGroup
	s.dal.Query().Where("tenant = ?", tenant).Find(&dbGroups)

	result := make([]*SCIMGroup, len(dbGroups))
	for i, dbGroup := range dbGroups {
		var members []SCIMMember
		json.Unmarshal(dbGroup.Members, &members)

		result[i] = &SCIMGroup{
			ID:          dbGroup.ID,
			DisplayName: dbGroup.DisplayName,
			Members:     members,
			Meta: SCIMMeta{
				ResourceType: dbGroup.ResourceType,
				Created:      dbGroup.CreatedAt.Format(time.RFC3339Nano),
				LastModified: dbGroup.UpdatedAt.Format(time.RFC3339Nano),
				Version:      "1",
			},
		}
	}

	return result
}

func (s *SCIMStore) DeleteGroup(tenant, id string) error {
	result := s.dal.Query().Where("id = ? AND tenant = ?", id, tenant).Delete(&database.SCIMGroup{})
	if result.Error != nil || result.RowsAffected == 0 {
		return fmt.Errorf("group not found: %s", id)
	}

	s.logger.Info("group deleted", zap.String("id", id), zap.String("tenant", tenant))
	return nil
}
