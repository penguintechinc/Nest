package main

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/penguintechinc/nest/shared/database"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	testDB *gorm.DB
)

func getTestDAL() *database.PenguinDAL {
	if testDB == nil {
		testDB, _ = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	}
	// Clean up for each test
	testDB.Exec("DELETE FROM scim_users")
	testDB.Exec("DELETE FROM scim_groups")
	return database.NewPenguinDAL(testDB)
}

func TestNewSCIMStore(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	dal := getTestDAL()
	store := NewSCIMStore(dal, logger)

	if store.logger != logger {
		t.Errorf("logger not set correctly")
	}
}

func TestCreateUser(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	dal := getTestDAL()
	store := NewSCIMStore(dal, logger)

	user := &SCIMUser{
		UserName:    "testuser",
		DisplayName: "Test User",
		Active:      true,
		Emails: []SCIMEmail{
			{Value: "test@example.com", Primary: true},
		},
	}

	created := store.CreateUser("tenant-1", user)

	if created.ID == "" {
		t.Errorf("expected ID to be assigned")
	}

	if created.Meta.ResourceType != "User" {
		t.Errorf("expected ResourceType 'User'")
	}
}

func TestGetUser(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	dal := getTestDAL()
	store := NewSCIMStore(dal, logger)

	user := &SCIMUser{
		UserName: "getuser",
		Active:   true,
	}

	created := store.CreateUser("tenant-1", user)

	retrieved, err := store.GetUser("tenant-1", created.ID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if retrieved.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, retrieved.ID)
	}
}

func TestListUsers(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	dal := getTestDAL()
	store := NewSCIMStore(dal, logger)

	user1 := &SCIMUser{UserName: "user1", Active: true}
	user2 := &SCIMUser{UserName: "user2", Active: true}

	store.CreateUser("tenant-1", user1)
	store.CreateUser("tenant-1", user2)

	users := store.ListUsers("tenant-1")

	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
	
	// Different tenant should be empty
	otherUsers := store.ListUsers("tenant-2")
	if len(otherUsers) != 0 {
		t.Errorf("expected 0 users for other tenant")
	}
}

func TestUpdateUser(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	dal := getTestDAL()
	store := NewSCIMStore(dal, logger)

	user := &SCIMUser{
		UserName:    "updatetest",
		DisplayName: "Original Name",
		Active:      true,
	}

	created := store.CreateUser("tenant-1", user)

	updated := &SCIMUser{
		UserName:    "updatetest",
		DisplayName: "Updated Name",
		Active:      false,
	}

	result, err := store.UpdateUser("tenant-1", created.ID, updated)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result.DisplayName != "Updated Name" {
		t.Errorf("expected updated DisplayName")
	}
}

func TestDeleteUser(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	dal := getTestDAL()
	store := NewSCIMStore(dal, logger)

	user := &SCIMUser{UserName: "deletetest"}
	created := store.CreateUser("tenant-1", user)

	err := store.DeleteUser("tenant-1", created.ID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, err = store.GetUser("tenant-1", created.ID)
	if err == nil {
		t.Errorf("expected error when getting deleted user")
	}
}
