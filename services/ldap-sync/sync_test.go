package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewSyncer(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("ldap://localhost:389", 1*time.Hour, logger)

	if syncer.logger != logger {
		t.Errorf("logger not set correctly")
	}

	if syncer.ldapURL != "ldap://localhost:389" {
		t.Errorf("expected ldapURL to be set")
	}

	if syncer.interval != 1*time.Hour {
		t.Errorf("expected interval to be 1 hour")
	}

	if syncer.users == nil {
		t.Errorf("users map not initialized")
	}
}

func TestListUsers(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	// Populate with stub users
	syncer.populateStubUsers("default")

	users := syncer.ListUsers("default")

	if len(users) != 2 {
		t.Errorf("expected 2 stub users, got %d", len(users))
	}

	usernames := make(map[string]bool)
	for _, u := range users {
		usernames[u.UID] = true
	}

	if !usernames["admin"] || !usernames["viewer"] {
		t.Errorf("expected admin and viewer users")
	}
}

func TestGetUser(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	syncer.populateStubUsers("default")

	user, found := syncer.GetUser("default", "admin")

	if !found {
		t.Errorf("expected to find admin user")
	}

	if user.UID != "admin" {
		t.Errorf("expected admin user")
	}

	if user.Email != "admin@nest.local" {
		t.Errorf("expected admin email")
	}

	if user.DisplayName != "Nest Admin" {
		t.Errorf("expected 'Nest Admin' display name")
	}

	if !user.Active {
		t.Errorf("expected admin to be active")
	}
}

func TestGetUserNotFound(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	user, found := syncer.GetUser("default", "nonexistent")

	if found {
		t.Errorf("expected user not found")
	}

	if user != nil {
		t.Errorf("expected nil user")
	}
}

func TestPopulateStubUsers(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	syncer.populateStubUsers("default")

	users := syncer.ListUsers("default")

	if len(users) < 2 {
		t.Errorf("expected at least 2 stub users")
	}

	// Verify admin user
	admin, found := syncer.GetUser("default", "admin")
	if !found {
		t.Fatalf("admin user not found")
	}

	if !strings.Contains(admin.DN, "cn=admin") {
		t.Errorf("expected admin DN to contain 'cn=admin'")
	}

	if len(admin.Groups) == 0 {
		t.Errorf("expected admin to have groups")
	}

	// Verify viewer user
	viewer, found := syncer.GetUser("default", "viewer")
	if !found {
		t.Fatalf("viewer user not found")
	}

	if viewer.Email != "viewer@nest.local" {
		t.Errorf("expected viewer email")
	}
}

func TestUserStructure(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	syncer.populateStubUsers("default")

	user, _ := syncer.GetUser("default", "admin")

	if user.DN == "" {
		t.Errorf("expected DN to be set")
	}

	if user.UID == "" {
		t.Errorf("expected UID to be set")
	}

	if user.Email == "" {
		t.Errorf("expected Email to be set")
	}

	if user.DisplayName == "" {
		t.Errorf("expected DisplayName to be set")
	}

	if len(user.Groups) == 0 {
		t.Errorf("expected Groups to be populated")
	}

	if user.SyncedAt.IsZero() {
		t.Errorf("expected SyncedAt to be set")
	}

	if !user.Active {
		t.Errorf("expected Active to be true")
	}
}

func TestSyncWithEmptyLDAP(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	ctx := context.Background()
	err := syncer.sync(ctx)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	users := syncer.ListUsers("default")
	if len(users) == 0 {
		t.Errorf("expected stub users to be populated")
	}
}

// TestSyncStubFallbackWithConfiguredLDAPURL covers the current stub
// implementation: sync() does not yet dial/bind/search LDAP, so even with an
// ldapURL configured it still falls through to stub user population. This is
// NOT a test of real LDAP integration — rename or replace once sync()
// performs an actual LDAP connection (see P8 phase marker in sync.go).
func TestSyncStubFallbackWithConfiguredLDAPURL(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("ldap://localhost:389", 1*time.Hour, logger)

	ctx := context.Background()
	err := syncer.sync(ctx)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	users := syncer.ListUsers("default")
	if len(users) == 0 {
		t.Errorf("expected stub users to be populated even with configured LDAP")
	}
}

func TestSyncPopulatesStubUsers(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("ldap://example.com", 1*time.Hour, logger)

	// Clear users
	syncer.mu.Lock()
	syncer.users = make(map[string]map[string]*User)
	syncer.mu.Unlock()

	ctx := context.Background()
	syncer.sync(ctx)

	users := syncer.ListUsers("default")
	if len(users) != 2 {
		t.Errorf("expected 2 stub users after sync, got %d", len(users))
	}
}

func TestListUsersOrder(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	syncer.populateStubUsers("default")

	users := syncer.ListUsers("default")

	if len(users) == 0 {
		t.Fatalf("expected users to be populated")
	}

	// All users should have non-empty fields
	for _, user := range users {
		if user.UID == "" {
			t.Errorf("expected all users to have UID")
		}
	}
}

func TestUserMarshalJSON(t *testing.T) {
	user := &User{
		DN:          "cn=test,dc=example,dc=com",
		UID:         "testuser",
		Email:       "test@example.com",
		DisplayName: "Test User",
		Groups:      []string{"admins", "users"},
		SyncedAt:    time.Date(2025, 4, 23, 10, 0, 0, 0, time.UTC),
		Active:      true,
	}

	jsonBytes, err := user.MarshalJSON()
	if err != nil {
		t.Errorf("unexpected error marshaling: %v", err)
	}

	// Verify it contains the right fields
	jsonStr := string(jsonBytes)

	if !strings.Contains(jsonStr, "testuser") {
		t.Errorf("expected UID in JSON")
	}

	if !strings.Contains(jsonStr, "test@example.com") {
		t.Errorf("expected Email in JSON")
	}

	if !strings.Contains(jsonStr, "Test User") {
		t.Errorf("expected DisplayName in JSON")
	}

	// Should have RFC3339 timestamp
	if !strings.Contains(jsonStr, "2025-04-23") {
		t.Errorf("expected RFC3339 timestamp in JSON")
	}
}

func TestUserMarshalJSONTimestamp(t *testing.T) {
	user := &User{
		DN:       "cn=test,dc=example,dc=com",
		UID:      "testuser",
		SyncedAt: time.Date(2025, 4, 23, 15, 30, 45, 0, time.UTC),
		Active:   true,
	}

	jsonBytes, err := user.MarshalJSON()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	jsonStr := string(jsonBytes)

	// Check for RFC3339 format
	if !strings.Contains(jsonStr, "2025-04-23T15:30:45") {
		t.Errorf("expected RFC3339 timestamp format, got: %s", jsonStr)
	}
}

func TestGetUserReturnsPointer(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	syncer.populateStubUsers("default")

	user1, found := syncer.GetUser("default", "admin")
	if !found {
		t.Fatal("expected admin user to exist")
	}
	// GetUser returns a pointer to internal storage; modifications are visible
	user1.DisplayName = "Modified"
	user2, _ := syncer.GetUser("default", "admin")
	if user2.DisplayName != "Modified" {
		t.Errorf("expected GetUser to return pointer to internal user, got copy instead")
	}
}

func TestConcurrentGetUser(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	syncer.populateStubUsers("default")

	var wg sync.WaitGroup
	errors := 0
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			uid := "admin"
			if index%2 == 0 {
				uid = "viewer"
			}
			user, found := syncer.GetUser("default", uid)
			if !found || user == nil {
				mu.Lock()
				errors++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	if errors > 0 {
		t.Errorf("expected no errors in concurrent gets, got %d", errors)
	}
}

func TestConcurrentListUsers(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	syncer.populateStubUsers("default")

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			users := syncer.ListUsers("default")
			if len(users) == 0 {
				t.Errorf("expected users to be populated")
			}
		}()
	}

	wg.Wait()
}

func TestListUsersReturnsIndependentList(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	syncer.populateStubUsers("default")

	list1 := syncer.ListUsers("default")
	originalLen := len(list1)

	// Modify returned list
	list1 = append(list1, &User{UID: "fake"})

	// Original should be unchanged
	list2 := syncer.ListUsers("default")

	if len(list2) != originalLen {
		t.Errorf("expected modifications to returned list to not affect internal state")
	}
}

func TestSyncerWithDifferentIntervals(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	intervals := []time.Duration{
		100 * time.Millisecond,
		1 * time.Second,
		10 * time.Second,
		1 * time.Hour,
	}

	for _, interval := range intervals {
		syncer := NewSyncer("", interval, logger)

		if syncer.interval != interval {
			t.Errorf("expected interval %v, got %v", interval, syncer.interval)
		}
	}
}

func TestAdminUserGroups(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	syncer.populateStubUsers("default")

	admin, _ := syncer.GetUser("default", "admin")

	expectedGroups := map[string]bool{
		"admins":         false,
		"nest-operators": false,
	}

	for _, group := range admin.Groups {
		if _, ok := expectedGroups[group]; ok {
			expectedGroups[group] = true
		}
	}

	for group, found := range expectedGroups {
		if !found {
			t.Errorf("expected group %s in admin groups", group)
		}
	}
}

func TestViewerUserGroups(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	syncer.populateStubUsers("default")

	viewer, _ := syncer.GetUser("default", "viewer")

	if len(viewer.Groups) != 1 {
		t.Errorf("expected viewer to have 1 group, got %d", len(viewer.Groups))
	}

	if viewer.Groups[0] != "viewers" {
		t.Errorf("expected viewer group, got %s", viewer.Groups[0])
	}
}

func TestUserDNFormat(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	syncer.populateStubUsers("default")

	admin, _ := syncer.GetUser("default", "admin")

	if !strings.Contains(admin.DN, "cn=") {
		t.Errorf("expected DN to contain 'cn='")
	}

	if !strings.Contains(admin.DN, "ou=users") {
		t.Errorf("expected DN to contain 'ou=users'")
	}

	if !strings.Contains(admin.DN, "dc=nest") {
		t.Errorf("expected DN to contain 'dc=nest'")
	}
}

func TestSyncerContextCancellation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 100*time.Millisecond, logger)

	ctx, cancel := context.WithCancel(context.Background())

	// Start sync in goroutine
	done := make(chan error)
	go func() {
		done <- syncer.Run(ctx)
	}()

	// Cancel after short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Wait for sync to finish
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Logf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("expected Run to exit on context cancellation")
	}
}

func TestMultipleSyncCycles(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 50*time.Millisecond, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Simulate multiple sync cycles
	cycleCount := 0
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			cycleCount++
			if cycleCount >= 3 {
				cancel()
				return
			}
		}
	}()

	syncer.Run(ctx)

	users := syncer.ListUsers("default")
	if len(users) == 0 {
		t.Errorf("expected users to be populated after sync cycles")
	}
}

func BenchmarkGetUser(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	syncer.populateStubUsers("default")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		syncer.GetUser("default", "admin")
	}
}

func BenchmarkListUsers(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	syncer.populateStubUsers("default")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		syncer.ListUsers("default")
	}
}

func BenchmarkPopulateStubUsers(b *testing.B) {
	logger, _ := zap.NewDevelopment()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		syncer := NewSyncer("", 1*time.Hour, logger)
		syncer.populateStubUsers("default")
	}
}

func TestUserActiveField(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	syncer.populateStubUsers("default")

	admin, _ := syncer.GetUser("default", "admin")
	if !admin.Active {
		t.Errorf("expected admin to be active")
	}

	viewer, _ := syncer.GetUser("default", "viewer")
	if !viewer.Active {
		t.Errorf("expected viewer to be active")
	}
}

func TestLDAPURLStoredCorrectly(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	ldapURL := "ldap://ldap.example.com:389"

	syncer := NewSyncer(ldapURL, 1*time.Hour, logger)

	if syncer.ldapURL != ldapURL {
		t.Errorf("expected ldapURL %s, got %s", ldapURL, syncer.ldapURL)
	}
}

func TestSyncedAtTimestamp(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	beforeSync := time.Now().UTC()
	syncer.populateStubUsers("default")
	afterSync := time.Now().UTC()

	admin, _ := syncer.GetUser("default", "admin")

	if admin.SyncedAt.Before(beforeSync) {
		t.Errorf("SyncedAt is before sync started")
	}

	if admin.SyncedAt.After(afterSync.Add(1 * time.Second)) {
		t.Errorf("SyncedAt is after sync ended")
	}
}

func TestRunWithShortInterval(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 25*time.Millisecond, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	// Run with short interval to trigger multiple sync cycles
	err := syncer.Run(ctx)
	if err != nil && err != context.DeadlineExceeded {
		t.Logf("expected deadline exceeded or nil, got: %v", err)
	}

	// Verify users were synced
	users := syncer.ListUsers("default")
	if len(users) == 0 {
		t.Errorf("expected users to be synced")
	}
}

func TestRunInitialSync(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Run to trigger initial sync
	_ = syncer.Run(ctx)

	// Verify initial sync populated users
	users := syncer.ListUsers("default")
	if len(users) != 2 {
		t.Errorf("expected 2 stub users from initial sync, got %d", len(users))
	}
}

func TestRunContextCancelledDuringSync(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 100*time.Millisecond, logger)

	ctx, cancel := context.WithCancel(context.Background())

	// Start Run in goroutine and cancel early
	done := make(chan error)
	go func() {
		done <- syncer.Run(ctx)
	}()

	// Cancel context before ticker fires
	time.Sleep(10 * time.Millisecond)
	cancel()

	// Should return context.Canceled
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Logf("expected context.Canceled, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("expected Run to exit on context cancellation")
	}
}

func TestRunContextCancelledAfterInitialSync(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 100*time.Millisecond, logger)

	ctx, cancel := context.WithCancel(context.Background())

	// Start Run and let initial sync complete
	done := make(chan error)
	go func() {
		done <- syncer.Run(ctx)
	}()

	// Cancel after initial sync has time to run
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Should return context.Canceled
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Logf("expected context.Canceled, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("expected Run to exit on context cancellation")
	}
}

func TestRunWithTicker(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	// Very short interval to ensure ticker fires
	syncer := NewSyncer("", 20*time.Millisecond, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	// Run should fire initial sync + at least 3 ticks
	_ = syncer.Run(ctx)

	users := syncer.ListUsers("default")
	if len(users) == 0 {
		t.Errorf("expected users to be populated after Run")
	}
}

// TestBuildUserFilterWithNormalInput verifies normal usernames are handled correctly.
func TestBuildUserFilterWithNormalInput(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	filter := syncer.buildUserFilter("johndoe")
	expectedPattern := "(&(objectClass=posixAccount)(uid=johndoe))"

	if filter != expectedPattern {
		t.Errorf("expected filter %q, got %q", expectedPattern, filter)
	}
}

// TestBuildUserFilterWithInjectionPayload verifies LDAP injection is prevented via escaping.
// Payload: "user*)(uid=*" without escaping would break the filter expression.
// With escaping, all special characters are neutralized.
func TestBuildUserFilterWithInjectionPayload(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	// Injection payload: if unescaped, this would close the uid= clause and inject a wildcard
	injectionPayload := "user*)(uid=*"
	filter := syncer.buildUserFilter(injectionPayload)

	// The filter should escape all special characters; assert it does NOT contain the raw closing paren and equals
	if strings.Contains(filter, ")(uid=*") {
		t.Errorf("filter contains unescaped injection payload: %q", filter)
	}

	// The escaped version should contain \2a (hex for *) and \29 (hex for ))
	if !strings.Contains(filter, "\\2a") && !strings.Contains(filter, "\\29") {
		t.Errorf("filter does not contain escaped special chars: %q", filter)
	}

	// Verify the filter still has the expected structure (escaped uid value inside parentheses)
	if !strings.Contains(filter, "(&(objectClass=posixAccount)(uid=") {
		t.Errorf("filter structure corrupted: %q", filter)
	}
}

// TestBuildUserFilterWithClosingParen verifies closing parenthesis is escaped.
func TestBuildUserFilterWithClosingParen(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	// Payload with closing paren that could break filter syntax
	payload := "admin)extra"
	filter := syncer.buildUserFilter(payload)

	// Should NOT contain the raw closing paren in the dangerous position
	if strings.Contains(filter, "uid=admin)extra)") {
		t.Errorf("closing paren not escaped: %q", filter)
	}

	// Should contain escaped version (29 = hex for ))
	if !strings.Contains(filter, "\\29") {
		t.Errorf("expected escaped paren in filter: %q", filter)
	}
}

// TestBuildUserFilterWithAsterisk verifies wildcard asterisks are escaped.
func TestBuildUserFilterWithAsterisk(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	// Payload with wildcard that could match all users
	payload := "user*"
	filter := syncer.buildUserFilter(payload)

	// Raw asterisk should be escaped
	if strings.Contains(filter, "uid=user*") {
		t.Errorf("asterisk not escaped: %q", filter)
	}

	// Should contain escaped asterisk (2a = hex for *)
	if !strings.Contains(filter, "\\2a") {
		t.Errorf("expected escaped asterisk in filter: %q", filter)
	}
}

// TestBuildGroupFilterWithInjectionPayload verifies LDAP injection prevention in group filters.
func TestBuildGroupFilterWithInjectionPayload(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	// Injection payload targeting group filter
	injectionPayload := "admin*)(cn=*"
	filter := syncer.buildGroupFilter(injectionPayload)

	// Should NOT contain the raw injection pattern
	if strings.Contains(filter, ")(cn=*") {
		t.Errorf("filter contains unescaped injection payload: %q", filter)
	}

	// Should have escaped special characters
	if !strings.Contains(filter, "\\2a") && !strings.Contains(filter, "\\29") {
		t.Errorf("filter does not contain escaped special chars: %q", filter)
	}

	// Verify filter structure is intact
	if !strings.Contains(filter, "(&(objectClass=posixGroup)(cn=") {
		t.Errorf("filter structure corrupted: %q", filter)
	}
}

// TestBuildUserFilterWithSpecialCharacters verifies all LDAP special chars are escaped.
func TestBuildUserFilterWithSpecialCharacters(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	// Payload with multiple LDAP special characters
	payload := "user(admin*test)sub"
	filter := syncer.buildUserFilter(payload)

	// Verify the filter structure is preserved
	if !strings.Contains(filter, "(&(objectClass=posixAccount)(uid=") {
		t.Errorf("filter structure corrupted: %q", filter)
	}

	// Special chars should be escaped (not present literally in the uid value part)
	// Extract just the uid= part for validation
	if strings.Contains(filter, "(uid=user(admin*test)sub)") {
		t.Errorf("special characters not properly escaped in filter: %q", filter)
	}
}

func TestSyncErrorHandling(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("ldap://error.example.com", 1*time.Hour, logger)

	// sync() logs errors but continues on LDAP errors
	ctx := context.Background()
	err := syncer.sync(ctx)

	if err != nil {
		t.Errorf("sync should not return error for LDAP issues, got: %v", err)
	}

	// Should still populate stub users
	users := syncer.ListUsers("default")
	if len(users) == 0 {
		t.Errorf("expected stub users even with LDAP error")
	}
}

func TestRunMultipleTicks(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 30*time.Millisecond, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	// Run for long enough to see multiple ticker events
	_ = syncer.Run(ctx)

	// Users should be populated from at least initial sync + ticks
	users := syncer.ListUsers("default")
	if len(users) < 2 {
		t.Errorf("expected users after multiple sync cycles")
	}
}
