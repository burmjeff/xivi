package utils_test

import (
	"testing"
	"xivi/backend/app/models"
	"xivi/backend/pkg/utils"
)

// TestUpdateDynamicGroup tests the UpdateDynamicGroup function
func TestUpdateDynamicGroup(t *testing.T) {
	// Test cases
	testCases := []struct {
		name        string
		group       models.TemplateGroup
		description string
	}{
		{
			name: "Group is not dynamic",
			group: models.TemplateGroup{
				ID:           1,
				Name:         "Test Group",
				Dynamic:      false,
				DynamicGroup: func() *int64 { id := int64(123); return &id }(),
			},
			description: "Should return early without error when group is not dynamic",
		},
		{
			name: "Group has no dynamicgroup",
			group: models.TemplateGroup{
				ID:           2,
				Name:         "Test Group 2",
				Dynamic:      true,
				DynamicGroup: nil,
			},
			description: "Should return early without error when dynamicgroup is nil",
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Execute the function - we're just testing that it doesn't panic
			utils.UpdateDynamicGroup(tc.group)
			// If we get here without panicking, the test passes
		})
	}
}
