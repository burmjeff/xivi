package mocks

import (
	"context"
	"fmt"
	"time"
	"xivi/backend/app/models"
)

// MockDB implements the database interface for testing
type MockDB struct {
	Templates       map[int64]*models.Template
	TemplateGroups  map[int64]*models.TemplateGroup
	PlaylistGroups  map[int64]*models.PlaylistGroup
	Playlists       map[int64]*models.Playlist
	TmplChannels    map[int64]*models.TemplateChannel
	PlChannels      map[int64]*models.PlaylistChannel
	GroupChannelMap map[int64][]int64 // Maps group IDs to channel IDs
}

// NewMockDB creates a new MockDB instance with initialized maps
func NewMockDB() *MockDB {
	return &MockDB{
		Templates:       make(map[int64]*models.Template),
		TemplateGroups:  make(map[int64]*models.TemplateGroup),
		PlaylistGroups:  make(map[int64]*models.PlaylistGroup),
		Playlists:       make(map[int64]*models.Playlist),
		TmplChannels:    make(map[int64]*models.TemplateChannel),
		PlChannels:      make(map[int64]*models.PlaylistChannel),
		GroupChannelMap: make(map[int64][]int64),
	}
}

// GetTemplates returns all templates
func (m *MockDB) GetTemplates() (*[]models.Template, error) {
	var templates []models.Template
	for _, template := range m.Templates {
		templates = append(templates, *template)
	}
	return &templates, nil
}

// GetTemplate returns a template by ID
func (m *MockDB) GetTemplate(id int64) (*models.Template, error) {
	template, exists := m.Templates[id]
	if !exists {
		return nil, fmt.Errorf("template not found")
	}
	return template, nil
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

// GetTmplChannelsByGroup returns template channels by group ID
func (m *MockDB) GetTmplChannelsByGroup(groupId int64) ([]models.TemplateChannel, error) {
	var channels []models.TemplateChannel
	channelIDs, exists := m.GroupChannelMap[groupId]
	if !exists {
		return channels, nil
	}

	for _, id := range channelIDs {
		if channel, exists := m.TmplChannels[id]; exists {
			channels = append(channels, *channel)
		}
	}
	return channels, nil
}

// UpdateTmplChannel updates a template channel
func (m *MockDB) UpdateTmplChannel(channel models.TemplateChannel) error {
	m.TmplChannels[channel.ID] = &channel
	return nil
}

// CreateTmplChannel creates a template channel
func (m *MockDB) CreateTmplChannel(channel models.TemplateChannel) (int64, error) {
	id := int64(len(m.TmplChannels) + 1)
	channel.ID = id
	m.TmplChannels[id] = &channel
	return id, nil
}

// CreateTmplGroupChannel creates a template group channel relationship
func (m *MockDB) CreateTmplGroupChannel(groupChannel models.TemplateGroupChannel) error {
	m.GroupChannelMap[groupChannel.GroupId] = append(m.GroupChannelMap[groupChannel.GroupId], groupChannel.ChannelId)
	return nil
}

// DeleteTmplChannel deletes a template channel
func (m *MockDB) DeleteTmplChannel(id int64) error {
	delete(m.TmplChannels, id)

	// Also remove from group-channel mappings
	for groupID, channelIDs := range m.GroupChannelMap {
		var newChannelIDs []int64
		for _, channelID := range channelIDs {
			if channelID != id {
				newChannelIDs = append(newChannelIDs, channelID)
			}
		}
		m.GroupChannelMap[groupID] = newChannelIDs
	}

	return nil
}

// GetPlaylists returns all playlists
func (m *MockDB) GetPlaylists() (*[]models.Playlist, error) {
	var playlists []models.Playlist
	for _, playlist := range m.Playlists {
		playlists = append(playlists, *playlist)
	}
	return &playlists, nil
}

// GetPlaylist returns a playlist by ID
func (m *MockDB) GetPlaylist(id int64) (*models.Playlist, error) {
	playlist, exists := m.Playlists[id]
	if !exists {
		return nil, fmt.Errorf("playlist not found")
	}
	return playlist, nil
}

// UpdatePlaylist updates a playlist
func (m *MockDB) UpdatePlaylist(id int64, playlist *models.Playlist) error {
	m.Playlists[id] = playlist
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

// GetPlGroups returns playlist groups by playlist ID
func (m *MockDB) GetPlGroups(playlistId int64) (*[]models.PlaylistGroup, error) {
	var groups []models.PlaylistGroup
	for _, group := range m.PlaylistGroups {
		if group.PlaylistId == playlistId {
			groups = append(groups, *group)
		}
	}
	return &groups, nil
}

// GetPlGroupChannels returns playlist channels by group ID
func (m *MockDB) GetPlGroupChannels(groupId int64) ([]models.PlaylistChannel, error) {
	var channels []models.PlaylistChannel
	for _, channel := range m.PlChannels {
		if channel.GroupId == groupId {
			channels = append(channels, *channel)
		}
	}
	return channels, nil
}

// GetPlChannels returns playlist channels by playlist ID
func (m *MockDB) GetPlChannels(playlistId int64) (*[]models.PlaylistChannel, error) {
	var channels []models.PlaylistChannel
	for _, channel := range m.PlChannels {
		group, exists := m.PlaylistGroups[channel.GroupId]
		if exists && group.PlaylistId == playlistId {
			channels = append(channels, *channel)
		}
	}
	return &channels, nil
}

// DeletePlChannel deletes a playlist channel
func (m *MockDB) DeletePlChannel(id int64) error {
	delete(m.PlChannels, id)
	return nil
}

// CleanPlaylistGroups cleans playlist groups
func (m *MockDB) CleanPlaylistGroups(ctx context.Context, id int64) error {
	// Simulate cleaning by removing groups that don't belong to this playlist
	for groupID, group := range m.PlaylistGroups {
		if group.PlaylistId != id {
			delete(m.PlaylistGroups, groupID)
		}
	}
	return nil
}

// CleanPlaylistChannels cleans playlist channels
func (m *MockDB) CleanPlaylistChannels(ctx context.Context, id int64) error {
	// Simulate cleaning by removing channels that don't belong to this playlist's groups
	var validGroupIDs []int64
	for _, group := range m.PlaylistGroups {
		if group.PlaylistId == id {
			validGroupIDs = append(validGroupIDs, group.ID)
		}
	}

	for channelID, channel := range m.PlChannels {
		isValid := false
		for _, groupID := range validGroupIDs {
			if channel.GroupId == groupID {
				isValid = true
				break
			}
		}
		if !isValid {
			delete(m.PlChannels, channelID)
		}
	}
	return nil
}

// CreateTmplChannelItem creates a template channel item
func (m *MockDB) CreateTmplChannelItem(item *models.TemplateChannelItem) error {
	// This is a simplified implementation
	return nil
}

// GetTmplGroupItemsByGroup returns template group items by group ID
func (m *MockDB) GetTmplGroupItemsByGroup(groupId int64) (*[]models.TemplateGroupItem, error) {
	// This is a simplified implementation
	var items []models.TemplateGroupItem
	return &items, nil
}

// VacuumDB simulates vacuuming the database
func (m *MockDB) VacuumDB(ctx context.Context) error {
	// This is a no-op for the mock
	return nil
}

// AddTestData adds test data to the mock database
func (m *MockDB) AddTestData() {
	// Add a test playlist
	playlist := &models.Playlist{
		ID:        1,
		Name:      "Test Playlist",
		URL:       "http://example.com/playlist.m3u",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.Playlists[playlist.ID] = playlist

	// Add a test playlist group
	playlistGroup := &models.PlaylistGroup{
		ID:         1,
		Name:       "Test Playlist Group",
		PlaylistId: playlist.ID,
		Enabled:    true,
	}
	m.PlaylistGroups[playlistGroup.ID] = playlistGroup

	// Add a test template group linked to the playlist group
	dynamicGroupId := playlistGroup.ID
	templateGroup := &models.TemplateGroup{
		ID:           1,
		Name:         "Test Template Group",
		Dynamic:      true,
		DynamicGroup: &dynamicGroupId,
	}
	m.TemplateGroups[templateGroup.ID] = templateGroup

	// Add a test template
	template := &models.Template{
		ID:   1,
		Name: "Test Template",
	}
	m.Templates[template.ID] = template

	// Add a test playlist channel
	tvgID := "test-channel"
	playlistChannel := &models.PlaylistChannel{
		ID:        1,
		TvgID:     &tvgID,
		TvgName:   "Test Channel",
		Title:     "Test Channel",
		GroupId:   playlistGroup.ID,
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.PlChannels[playlistChannel.ID] = playlistChannel

	// Add a test template channel
	templateChannel := &models.TemplateChannel{
		ID:     1,
		Name:   "Test Template Channel",
		TvgID:  &tvgID,
		LogoId: 0,
		Uuid:   "test-uuid",
	}
	m.TmplChannels[templateChannel.ID] = templateChannel

	// Link the template channel to the template group
	m.GroupChannelMap[templateGroup.ID] = []int64{templateChannel.ID}
}
