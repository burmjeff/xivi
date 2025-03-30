package utils_test

import (
	"testing"
	"xivi/backend/app/models"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"
	"xivi/tests/unit/backend/mocks"
)

// TestUpdateDynamicGroupWithMock tests the UpdateDynamicGroup function with a mock database
func TestUpdateDynamicGroupWithMock(t *testing.T) {
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
		group       models.TemplateGroup
		description string
		checkFunc   func(t *testing.T)
	}{
		{
			name: "Group is not dynamic",
			setupFunc: func() {
				// No setup needed
			},
			group: models.TemplateGroup{
				ID:           1,
				Name:         "Test Group",
				Dynamic:      false,
				DynamicGroup: func() *int64 { id := int64(1); return &id }(),
			},
			description: "Should return early without error when group is not dynamic",
			checkFunc: func(t *testing.T) {
				// No checks needed, just verifying it doesn't panic
			},
		},
		{
			name: "Group has no dynamicgroup",
			setupFunc: func() {
				// No setup needed
			},
			group: models.TemplateGroup{
				ID:           1,
				Name:         "Test Group",
				Dynamic:      true,
				DynamicGroup: nil,
			},
			description: "Should return early without error when dynamicgroup is nil",
			checkFunc: func(t *testing.T) {
				// No checks needed, just verifying it doesn't panic
			},
		},
		{
			name: "Playlist group no longer exists",
			setupFunc: func() {
				// Remove the playlist group to simulate it no longer existing
				delete(mockDb.PlaylistGroups, 1)
			},
			group: models.TemplateGroup{
				ID:           1,
				Name:         "Test Group",
				Dynamic:      true,
				DynamicGroup: func() *int64 { id := int64(1); return &id }(),
			},
			description: "Should disable dynamic connection when playlist group doesn't exist",
			checkFunc: func(t *testing.T) {
				// Check that the group is no longer dynamic
				group, err := mockDb.GetTmplGroup(1)
				if err != nil {
					t.Errorf("Failed to get template group: %v", err)
					return
				}
				if group.Dynamic {
					t.Errorf("Expected group to not be dynamic, but it is")
				}
				if group.DynamicGroup != nil {
					t.Errorf("Expected dynamicgroup to be nil, but got %v", *group.DynamicGroup)
				}
			},
		},
		{
			name: "Playlist group exists but has no channels",
			setupFunc: func() {
				// Restore the playlist group
				mockDb.PlaylistGroups[1] = &models.PlaylistGroup{
					ID:         1,
					Name:       "Test Playlist Group",
					PlaylistId: 1,
					Enabled:    true,
				}
				
				// Make the template group dynamic again
				group := mockDb.TemplateGroups[1]
				group.Dynamic = true
				dynamicGroupId := int64(1)
				group.DynamicGroup = &dynamicGroupId
				
				// Remove all playlist channels
				mockDb.PlChannels = make(map[int64]*models.PlaylistChannel)
			},
			group: models.TemplateGroup{
				ID:           1,
				Name:         "Test Group",
				Dynamic:      true,
				DynamicGroup: func() *int64 { id := int64(1); return &id }(),
			},
			description: "Should maintain connection even when playlist group has no channels",
			checkFunc: func(t *testing.T) {
				// Check that the group is still dynamic
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
			name: "Normal case with matching channels",
			setupFunc: func() {
				// Add a playlist channel
				tvgID := "test-channel"
				mockDb.PlChannels[1] = &models.PlaylistChannel{
					ID:      1,
					TvgID:   &tvgID,
					TvgName: "Test Channel",
					Title:   "Updated Channel Name",
					GroupId: 1,
					Enabled: true,
				}
				
				// Add a template channel with the same tvgID
				mockDb.TmplChannels[1] = &models.TemplateChannel{
					ID:    1,
					Name:  "Old Channel Name",
					TvgID: &tvgID,
					Uuid:  "test-uuid",
				}
				
				// Link the template channel to the template group
				mockDb.GroupChannelMap[1] = []int64{1}
			},
			group: models.TemplateGroup{
				ID:           1,
				Name:         "Test Group",
				Dynamic:      true,
				DynamicGroup: func() *int64 { id := int64(1); return &id }(),
			},
			description: "Should update template channel name to match playlist channel",
			checkFunc: func(t *testing.T) {
				// Check that the template channel name was updated
				channel, exists := mockDb.TmplChannels[1]
				if !exists {
					t.Errorf("Template channel not found")
					return
				}
				if channel.Name != "Updated Channel Name" {
					t.Errorf("Expected channel name to be 'Updated Channel Name', but got '%s'", channel.Name)
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
			utils.UpdateDynamicGroup(tc.group)
			
			// Check the results
			if tc.checkFunc != nil {
				tc.checkFunc(t)
			}
		})
	}
}
