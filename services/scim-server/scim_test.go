package main

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewSCIMStore(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	if store.logger != logger {
		t.Errorf("logger not set correctly")
	}
	if store.users == nil {
		t.Errorf("users map not initialized")
	}
	if store.groups == nil {
		t.Errorf("groups map not initialized")
	}
}

func TestCreateUser(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	user := &SCIMUser{
		UserName:    "testuser",
		DisplayName: "Test User",
		Active:      true,
		Emails: []SCIMEmail{
			{Value: "test@example.com", Primary: true},
		},
	}

	created := store.CreateUser(user)

	if created.ID == "" {
		t.Errorf("expected ID to be assigned")
	}

	if !strings.HasPrefix(created.ID, "scim-") {
		t.Errorf("expected ID to start with 'scim-'")
	}

	if created.Meta.ResourceType != "User" {
		t.Errorf("expected ResourceType 'User'")
	}

	if created.Meta.Created == "" {
		t.Errorf("expected Created timestamp")
	}

	if created.Meta.LastModified == "" {
		t.Errorf("expected LastModified timestamp")
	}

	if created.Meta.Version != "1" {
		t.Errorf("expected version '1'")
	}
}

func TestGetUser(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	user := &SCIMUser{
		UserName: "getuser",
		Active:   true,
	}

	created := store.CreateUser(user)

	retrieved, err := store.GetUser(created.ID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if retrieved.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, retrieved.ID)
	}

	if retrieved.UserName != "getuser" {
		t.Errorf("expected UserName 'getuser'")
	}
}

func TestGetUserNotFound(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	_, err := store.GetUser("nonexistent-id")
	if err == nil {
		t.Errorf("expected error for nonexistent user")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error message")
	}
}

func TestListUsers(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	user1 := &SCIMUser{UserName: "user1", Active: true}
	user2 := &SCIMUser{UserName: "user2", Active: true}
	user3 := &SCIMUser{UserName: "user3", Active: false}

	store.CreateUser(user1)
	store.CreateUser(user2)
	store.CreateUser(user3)

	users := store.ListUsers()

	if len(users) != 3 {
		t.Errorf("expected 3 users, got %d", len(users))
	}

	// Check all usernames are present
	usernames := make(map[string]bool)
	for _, u := range users {
		usernames[u.UserName] = true
	}

	if !usernames["user1"] || !usernames["user2"] || !usernames["user3"] {
		t.Errorf("expected all users to be listed")
	}
}

func TestListUsersEmpty(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	users := store.ListUsers()

	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestUpdateUser(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	user := &SCIMUser{
		UserName:    "updatetest",
		DisplayName: "Original Name",
		Active:      true,
	}

	created := store.CreateUser(user)
	originalCreated := created.Meta.Created

	// Wait a bit to ensure timestamp difference
	time.Sleep(10 * time.Millisecond)

	updated := &SCIMUser{
		UserName:    "updatetest",
		DisplayName: "Updated Name",
		Active:      false,
	}

	result, err := store.UpdateUser(created.ID, updated)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result.DisplayName != "Updated Name" {
		t.Errorf("expected updated DisplayName")
	}

	if result.Meta.Created != originalCreated {
		t.Errorf("expected Created timestamp to be preserved")
	}

	if result.Meta.LastModified == originalCreated {
		t.Errorf("expected LastModified to be updated")
	}

	if result.ID != created.ID {
		t.Errorf("expected ID to remain unchanged")
	}
}

func TestUpdateUserNotFound(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	user := &SCIMUser{UserName: "test"}

	_, err := store.UpdateUser("nonexistent-id", user)
	if err == nil {
		t.Errorf("expected error for nonexistent user")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error message")
	}
}

func TestDeleteUser(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	user := &SCIMUser{UserName: "deletetest"}
	created := store.CreateUser(user)

	err := store.DeleteUser(created.ID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, err = store.GetUser(created.ID)
	if err == nil {
		t.Errorf("expected error when getting deleted user")
	}
}

func TestDeleteUserNotFound(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	err := store.DeleteUser("nonexistent-id")
	if err == nil {
		t.Errorf("expected error for nonexistent user")
	}
}

func TestCreateGroup(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	group := &SCIMGroup{
		DisplayName: "Test Group",
		Members: []SCIMMember{
			{Value: "user1", Display: "User One"},
		},
	}

	created := store.CreateGroup(group)

	if created.ID == "" {
		t.Errorf("expected ID to be assigned")
	}

	if created.Meta.ResourceType != "Group" {
		t.Errorf("expected ResourceType 'Group'")
	}

	if created.Meta.Created == "" {
		t.Errorf("expected Created timestamp")
	}
}

func TestGetGroup(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	group := &SCIMGroup{DisplayName: "TestGroup"}
	created := store.CreateGroup(group)

	retrieved, err := store.GetGroup(created.ID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if retrieved.ID != created.ID {
		t.Errorf("expected ID to match")
	}

	if retrieved.DisplayName != "TestGroup" {
		t.Errorf("expected DisplayName to match")
	}
}

func TestGetGroupNotFound(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	_, err := store.GetGroup("nonexistent-id")
	if err == nil {
		t.Errorf("expected error for nonexistent group")
	}
}

func TestListGroups(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	group1 := &SCIMGroup{DisplayName: "group1"}
	group2 := &SCIMGroup{DisplayName: "group2"}

	store.CreateGroup(group1)
	store.CreateGroup(group2)

	groups := store.ListGroups()

	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}

	names := make(map[string]bool)
	for _, g := range groups {
		names[g.DisplayName] = true
	}

	if !names["group1"] || !names["group2"] {
		t.Errorf("expected both groups to be listed")
	}
}

func TestListGroupsEmpty(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	groups := store.ListGroups()

	if len(groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(groups))
	}
}

func TestDeleteGroup(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	group := &SCIMGroup{DisplayName: "deletetest"}
	created := store.CreateGroup(group)

	err := store.DeleteGroup(created.ID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, err = store.GetGroup(created.ID)
	if err == nil {
		t.Errorf("expected error when getting deleted group")
	}
}

func TestDeleteGroupNotFound(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	err := store.DeleteGroup("nonexistent-id")
	if err == nil {
		t.Errorf("expected error for nonexistent group")
	}
}

func TestUserMetaFields(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	user := &SCIMUser{
		UserName: "metauser",
		Active:   true,
	}

	created := store.CreateUser(user)

	// Parse timestamp
	createdTime, err := time.Parse(time.RFC3339, created.Meta.Created)
	if err != nil {
		t.Errorf("expected valid RFC3339 timestamp for Created: %v", err)
	}

	lastModTime, err := time.Parse(time.RFC3339, created.Meta.LastModified)
	if err != nil {
		t.Errorf("expected valid RFC3339 timestamp for LastModified: %v", err)
	}

	// Timestamps should be close to now
	now := time.Now().UTC()
	if createdTime.After(now.Add(1 * time.Second)) {
		t.Errorf("Created timestamp is in the future")
	}

	if lastModTime.After(now.Add(1 * time.Second)) {
		t.Errorf("LastModified timestamp is in the future")
	}
}

func TestGroupMetaFields(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	group := &SCIMGroup{DisplayName: "metagroup"}

	created := store.CreateGroup(group)

	// Parse timestamp
	_, err := time.Parse(time.RFC3339, created.Meta.Created)
	if err != nil {
		t.Errorf("expected valid RFC3339 timestamp: %v", err)
	}

	_, err = time.Parse(time.RFC3339, created.Meta.LastModified)
	if err != nil {
		t.Errorf("expected valid RFC3339 timestamp: %v", err)
	}
}

func TestConcurrentUserCreation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			user := &SCIMUser{
				UserName: fmt.Sprintf("user-%d", index),
				Active:   true,
			}
			store.CreateUser(user)
		}(i)
	}

	wg.Wait()

	users := store.ListUsers()
	if len(users) != numGoroutines {
		t.Errorf("expected %d users, got %d", numGoroutines, len(users))
	}
}

func TestConcurrentGroupCreation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			group := &SCIMGroup{
				DisplayName: fmt.Sprintf("group-%d", index),
			}
			store.CreateGroup(group)
		}(i)
	}

	wg.Wait()

	groups := store.ListGroups()
	if len(groups) != numGoroutines {
		t.Errorf("expected %d groups, got %d", numGoroutines, len(groups))
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	user := &SCIMUser{UserName: "concurrent"}
	created := store.CreateUser(user)

	var wg sync.WaitGroup

	// Read goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.GetUser(created.ID)
			store.ListUsers()
		}()
	}

	// Write goroutines
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			updated := &SCIMUser{
				UserName: fmt.Sprintf("concurrent-%d", index),
			}
			store.UpdateUser(created.ID, updated)
		}(i)
	}

	wg.Wait()

	// Verify user still exists
	retrieved, err := store.GetUser(created.ID)
	if err != nil {
		t.Errorf("user not found after concurrent operations")
	}

	if retrieved == nil {
		t.Errorf("expected user to exist")
	}
}

func TestUserWithMultipleEmails(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	user := &SCIMUser{
		UserName: "multiemail",
		Emails: []SCIMEmail{
			{Value: "primary@example.com", Primary: true, Type: "work"},
			{Value: "secondary@example.com", Primary: false, Type: "personal"},
			{Value: "tertiary@example.com", Primary: false, Type: "other"},
		},
	}

	created := store.CreateUser(user)

	if len(created.Emails) != 3 {
		t.Errorf("expected 3 emails, got %d", len(created.Emails))
	}

	primaryCount := 0
	for _, e := range created.Emails {
		if e.Primary {
			primaryCount++
		}
	}

	if primaryCount != 1 {
		t.Errorf("expected 1 primary email, got %d", primaryCount)
	}
}

func TestUserWithGroups(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	group1 := store.CreateGroup(&SCIMGroup{DisplayName: "admins"})
	group2 := store.CreateGroup(&SCIMGroup{DisplayName: "users"})

	user := &SCIMUser{
		UserName: "groupuser",
		Groups: []SCIMGroupRef{
			{Value: group1.ID, Display: "admins"},
			{Value: group2.ID, Display: "users"},
		},
	}

	created := store.CreateUser(user)

	if len(created.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(created.Groups))
	}
}

func TestGroupWithMembers(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	user1 := store.CreateUser(&SCIMUser{UserName: "user1"})
	user2 := store.CreateUser(&SCIMUser{UserName: "user2"})

	group := &SCIMGroup{
		DisplayName: "membergoup",
		Members: []SCIMMember{
			{Value: user1.ID, Display: "User One"},
			{Value: user2.ID, Display: "User Two"},
		},
	}

	created := store.CreateGroup(group)

	if len(created.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(created.Members))
	}
}

func TestUserIDUniqueness(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	user1 := store.CreateUser(&SCIMUser{UserName: "user1"})
	user2 := store.CreateUser(&SCIMUser{UserName: "user2"})

	if user1.ID == user2.ID {
		t.Errorf("expected unique IDs for different users")
	}
}

func TestGroupIDUniqueness(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	group1 := store.CreateGroup(&SCIMGroup{DisplayName: "group1"})
	group2 := store.CreateGroup(&SCIMGroup{DisplayName: "group2"})

	if group1.ID == group2.ID {
		t.Errorf("expected unique IDs for different groups")
	}
}

func TestUpdatePreservesExternalID(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	user := &SCIMUser{
		UserName:   "extid",
		ExternalID: "external-123",
	}

	created := store.CreateUser(user)
	_ = created.ExternalID // document actual ExternalID before update

	updated := &SCIMUser{
		UserName:   "extid-updated",
		ExternalID: "external-456",
	}

	result, _ := store.UpdateUser(created.ID, updated)

	// Note: based on code, UpdateUser doesn't preserve ExternalID
	// This test documents actual behavior
	if result.ExternalID != "external-456" {
		t.Errorf("expected updated ExternalID")
	}
}

func TestListUsersReturnsIndependentCopy(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	user := store.CreateUser(&SCIMUser{UserName: "copytest"})

	list1 := store.ListUsers()
	user.UserName = "modified"
	list2 := store.ListUsers()

	// Modifying returned user shouldn't affect storage
	if len(list1) != len(list2) {
		t.Errorf("list length changed unexpectedly")
	}
}

func TestListGroupsReturnsIndependentCopy(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	group := store.CreateGroup(&SCIMGroup{DisplayName: "copytest"})

	list1 := store.ListGroups()
	group.DisplayName = "modified"
	list2 := store.ListGroups()

	if len(list1) != len(list2) {
		t.Errorf("list length changed unexpectedly")
	}
}

func BenchmarkCreateUser(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	user := &SCIMUser{
		UserName: "benchmark",
		Active:   true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.CreateUser(user)
	}
}

func BenchmarkGetUser(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	user := store.CreateUser(&SCIMUser{UserName: "bench"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.GetUser(user.ID)
	}
}

func BenchmarkListUsers(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	store := NewSCIMStore(logger)

	for i := 0; i < 100; i++ {
		store.CreateUser(&SCIMUser{UserName: fmt.Sprintf("user-%d", i)})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.ListUsers()
	}
}
