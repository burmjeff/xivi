# Playlist-Template Group Relationship Tests

This document describes the tests for ensuring that when a playlist is updated, any associated template groups attached to it via dynamicgroup remain properly connected.

## Background

Template groups can be dynamically linked to playlist groups via the `dynamicgroup` field. When a playlist is updated, we need to ensure that this relationship is maintained, even if the playlist groups are temporarily deleted and recreated during the update process.

## Key Functions Tested

1. `UpdatePlaylists` in `backend/platform/cron/cron.go`
2. `UpdateDynamicGroup` in `backend/pkg/utils/playlist_tools.go`

## Test Cases

### UpdateDynamicGroup Tests

The `UpdateDynamicGroup` function is tested with the following cases:

1. **Group is not dynamic**: The function should return early without error
2. **Group has no dynamicgroup**: The function should return early without error
3. **Playlist group no longer exists**: The function should disable the dynamic connection
4. **Playlist group exists but has no channels**: The function should log a warning but maintain the connection
5. **Normal case with matching channels**: The function should update the template channels to match the playlist channels

### UpdatePlaylists Tests

The `UpdatePlaylists` function is tested with the following cases:

1. **Basic functionality**: The function should not panic when called
2. **Dynamic group preservation**: The function should preserve dynamic group relationships during playlist updates
3. **Dynamic group restoration**: The function should restore dynamic group relationships that were lost during playlist updates

## Mock Implementation

To properly test these functions without requiring a real database, we need to create mock implementations of the database interfaces. Here's an example of how to mock the database for testing:

```go
// MockDB implements the database interface for testing
type MockDB struct {
    TemplateGroups map[int64]*models.TemplateGroup
    PlaylistGroups map[int64]*models.PlaylistGroup
    // Add other necessary mock data
}

// GetAllTmplGroups returns all template groups
func (m *MockDB) GetAllTmplGroups() (*[]models.TemplateGroup, error) {
    var groups []models.TemplateGroup
    for _, group := range m.TemplateGroups {
        groups = append(groups, *group)
    }
    return &groups, nil
}

// GetTmplGroup returns a template group by ID
func (m *MockDB) GetTmplGroup(id int64) (*models.TemplateGroup, error) {
    group, exists := m.TemplateGroups[id]
    if !exists {
        return nil, fmt.Errorf("template group not found")
    }
    return group, nil
}

// UpdateTmplGroup updates a template group
func (m *MockDB) UpdateTmplGroup(group *models.TemplateGroup) error {
    m.TemplateGroups[group.ID] = group
    return nil
}

// GetPlGroup returns a playlist group by ID
func (m *MockDB) GetPlGroup(id int64) (*models.PlaylistGroup, error) {
    group, exists := m.PlaylistGroups[id]
    if !exists {
        return nil, fmt.Errorf("playlist group not found")
    }
    return group, nil
}

// Add other necessary mock methods
```

## Integration Testing

In addition to unit tests, we should also perform integration tests to verify that the entire playlist update process works correctly with real database interactions. These tests would:

1. Create a test playlist and template group
2. Link the template group to a playlist group
3. Update the playlist
4. Verify that the template group is still linked to the playlist group

## Manual Testing

For manual testing, follow these steps:

1. Create a new playlist
2. Create a new template group and set it as dynamic, linked to a group in the playlist
3. Update the playlist (either manually or wait for the cron job)
4. Verify that the template group is still linked to the playlist group

## Future Improvements

1. Add more comprehensive tests for edge cases
2. Implement integration tests with a test database
3. Add tests for the database triggers that handle playlist group deletion
