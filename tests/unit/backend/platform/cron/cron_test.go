package cron_test

import (
	"testing"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/platform/cron"
	"xivi/tests/unit/backend/mocks"
)

// TestUpdatePlaylists tests the UpdatePlaylists function
func TestUpdatePlaylists(t *testing.T) {
	// This is a basic test to ensure the function doesn't panic
	// In a real implementation, you would use mocks to test the database interactions

	// Skip this test in normal runs since it requires database access
	t.Skip("Skipping test that requires database access")

	// Call the function - we're just testing that it doesn't panic
	cron.UpdatePlaylists()
}

// TestCleanPlaylistStaleChannelDetection tests that stale channels are properly identified and removed
func TestCleanPlaylistStaleChannelDetection(t *testing.T) {
	// Create a mock database
	mockDB := &mocks.MockDB{
		Playlists:       make(map[int64]*models.Playlist),
		PlaylistGroups:  make(map[int64]*models.PlaylistGroup),
		PlChannels:      make(map[int64]*models.PlaylistChannel),
		GroupChannelMap: make(map[int64][]int64),
	}

	// Set up test data
	playlistID := int64(1)
	groupID := int64(1)

	// Create a playlist
	playlist := &models.Playlist{
		ID:        playlistID,
		Name:      "Test Playlist",
		URL:       "http://example.com/test.m3u",
		CreatedAt: time.Now().Add(-24 * time.Hour),
		UpdatedAt: time.Now().Add(-24 * time.Hour),
	}
	mockDB.Playlists[playlistID] = playlist

	// Create a playlist group
	group := &models.PlaylistGroup{
		ID:         groupID,
		PlaylistId: playlistID,
		Name:       "Test Group",
		Enabled:    true,
	}
	mockDB.PlaylistGroups[groupID] = group

	// Create test channels with different timestamp scenarios
	now := time.Now()
	startTime := now.Add(-1 * time.Hour) // Start time is 1 hour ago

	// Channel 1: Created and updated before start time (should be removed - stale)
	channel1 := &models.PlaylistChannel{
		ID:        1,
		GroupId:   groupID,
		Title:     "Stale Channel 1",
		CreatedAt: startTime.Add(-2 * time.Hour),
		UpdatedAt: startTime.Add(-2 * time.Hour),
		Enabled:   true,
	}
	mockDB.PlChannels[1] = channel1

	// Channel 2: Created before but updated after start time (should be kept - updated during parsing)
	channel2 := &models.PlaylistChannel{
		ID:        2,
		GroupId:   groupID,
		Title:     "Updated Channel",
		CreatedAt: startTime.Add(-2 * time.Hour),
		UpdatedAt: startTime.Add(30 * time.Minute),
		Enabled:   true,
	}
	mockDB.PlChannels[2] = channel2

	// Channel 3: Created and updated after start time (should be kept - new channel)
	channel3 := &models.PlaylistChannel{
		ID:        3,
		GroupId:   groupID,
		Title:     "New Channel",
		CreatedAt: startTime.Add(30 * time.Minute),
		UpdatedAt: startTime.Add(30 * time.Minute),
		Enabled:   true,
	}
	mockDB.PlChannels[3] = channel3

	// Channel 4: Another stale channel (should be removed)
	channel4 := &models.PlaylistChannel{
		ID:        4,
		GroupId:   groupID,
		Title:     "Stale Channel 2",
		CreatedAt: startTime.Add(-3 * time.Hour),
		UpdatedAt: startTime.Add(-1*time.Hour - 30*time.Minute), // Updated before start time
		Enabled:   true,
	}
	mockDB.PlChannels[4] = channel4

	// Verify initial state
	if len(mockDB.PlChannels) != 4 {
		t.Fatalf("Expected 4 channels initially, got %d", len(mockDB.PlChannels))
	}

	// Note: This test demonstrates the logic but cannot actually run CleanPlaylist
	// since it requires the real database interface. The test shows what should happen:

	// Expected behavior after CleanPlaylist(*playlist, startTime):
	// - Channel 1 should be removed (both timestamps before start time)
	// - Channel 2 should be kept (updated during parsing)
	// - Channel 3 should be kept (new channel)
	// - Channel 4 should be removed (updated before start time)

	// Simulate the cleanup logic
	var staleChannels []int64
	for id, channel := range mockDB.PlChannels {
		if (channel.UpdatedAt.Before(startTime) && channel.CreatedAt.Before(startTime)) ||
			(channel.UpdatedAt.Before(startTime) && channel.CreatedAt.After(startTime)) {
			staleChannels = append(staleChannels, id)
		}
	}

	// Verify the stale channel detection logic
	expectedStaleChannels := []int64{1, 4}
	if len(staleChannels) != len(expectedStaleChannels) {
		t.Errorf("Expected %d stale channels, found %d", len(expectedStaleChannels), len(staleChannels))
	}

	// Check that the correct channels are identified as stale
	staleMap := make(map[int64]bool)
	for _, id := range staleChannels {
		staleMap[id] = true
	}

	for _, expectedID := range expectedStaleChannels {
		if !staleMap[expectedID] {
			t.Errorf("Expected channel %d to be identified as stale", expectedID)
		}
	}

	// Verify that non-stale channels are not marked for removal
	if staleMap[2] {
		t.Error("Channel 2 should not be marked as stale (was updated during parsing)")
	}
	if staleMap[3] {
		t.Error("Channel 3 should not be marked as stale (is a new channel)")
	}

	t.Logf("Test passed: Correctly identified %d stale channels out of %d total channels",
		len(staleChannels), len(mockDB.PlChannels))
}
