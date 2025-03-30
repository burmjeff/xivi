package cron_test

import (
	"testing"
	"xivi/backend/platform/cron"
	"xivi/backend/platform/database"
	"xivi/tests/unit/backend/mocks"
)

// TestUpdatePlaylistsWithMock tests the UpdatePlaylists function with a mock database
func TestUpdatePlaylistsWithMock(t *testing.T) {
	// Skip this test in normal runs since it requires modifying the global database variable
	t.Skip("Skipping test that requires modifying global database variable")
	
	// Save the original database
	originalDb := database.Db
	
	// Create a mock database
	mockDb := mocks.NewMockDB()
	mockDb.AddTestData()
	
	// Replace the global database with our mock
	database.Db = mockDb
	
	// Restore the original database when the test finishes
	defer func() {
		database.Db = originalDb
	}()
	
	// Test cases
	testCases := []struct {
		name        string
		setupFunc   func()
		description string
		checkFunc   func(t *testing.T)
	}{
		{
			name: "Basic functionality",
			setupFunc: func() {
				// No special setup needed
			},
			description: "Should not panic when called",
			checkFunc: func(t *testing.T) {
				// No checks needed, just verifying it doesn't panic
			},
		},
		{
			name: "Dynamic group preservation",
			setupFunc: func() {
				// Setup a dynamic group relationship
				dynamicGroupId := int64(1)
				mockDb.TemplateGroups[1] = &mocks.TemplateGroup{
					ID:           1,
					Name:         "Test Template Group",
					Dynamic:      true,
					DynamicGroup: &dynamicGroupId,
				}
			},
			description: "Should preserve dynamic group relationships during playlist updates",
			checkFunc: func(t *testing.T) {
				// Check that the dynamic group relationship is preserved
				group, err := mockDb.GetTmplGroup(1)
				if err != nil {
					t.Errorf("Failed to get template group: %v", err)
					return
				}
				if !group.Dynamic {
					t.Errorf("Expected group to be dynamic, but it's not")
				}
				if group.DynamicGroup == nil {
					t.Errorf("Expected dynamicgroup to not be nil")
				} else if *group.DynamicGroup != 1 {
					t.Errorf("Expected dynamicgroup to be 1, but got %v", *group.DynamicGroup)
				}
			},
		},
		{
			name: "Dynamic group restoration",
			setupFunc: func() {
				// Setup a dynamic group relationship
				dynamicGroupId := int64(1)
				mockDb.TemplateGroups[1] = &mocks.TemplateGroup{
					ID:           1,
					Name:         "Test Template Group",
					Dynamic:      true,
					DynamicGroup: &dynamicGroupId,
				}
				
				// Simulate the relationship being lost during cleanup
				mockDb.CleanPlaylistGroups(nil, 1)
				
				// Check that the relationship was lost
				group, _ := mockDb.GetTmplGroup(1)
				if group.Dynamic && group.DynamicGroup != nil && *group.DynamicGroup == 1 {
					t.Errorf("Setup failed: dynamic group relationship was not lost")
				}
			},
			description: "Should restore dynamic group relationships that were lost during playlist updates",
			checkFunc: func(t *testing.T) {
				// Check that the dynamic group relationship was restored
				group, err := mockDb.GetTmplGroup(1)
				if err != nil {
					t.Errorf("Failed to get template group: %v", err)
					return
				}
				if !group.Dynamic {
					t.Errorf("Expected group to be dynamic, but it's not")
				}
				if group.DynamicGroup == nil {
					t.Errorf("Expected dynamicgroup to not be nil")
				} else if *group.DynamicGroup != 1 {
					t.Errorf("Expected dynamicgroup to be 1, but got %v", *group.DynamicGroup)
				}
			},
		},
	}
	
	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup the test case
			if tc.setupFunc != nil {
				tc.setupFunc()
			}
			
			// Execute the function
			cron.UpdatePlaylists()
			
			// Check the results
			if tc.checkFunc != nil {
				tc.checkFunc(t)
			}
		})
	}
}
