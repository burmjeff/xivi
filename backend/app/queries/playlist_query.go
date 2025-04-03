package queries

import (
	"database/sql"
	"fmt"
	"math/rand"
	"strings"
	"time"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// PlaylistQueries struct for queries from Playlist model.
type PlaylistQueries struct {
	BaseQueries
}

// NewPlaylistQueries creates a new PlaylistQueries instance
func NewPlaylistQueries(db *sqlx.DB) *PlaylistQueries {
	return &PlaylistQueries{
		BaseQueries: NewBaseQueries(db),
	}
}

// GetPlaylists method
func (q *PlaylistQueries) GetPlaylists() (*[]models.Playlist, error) {
	playlists := &[]models.Playlist{}

	query := `SELECT * FROM playlist`

	err := q.Select(playlists, query)
	if err != nil {
		return nil, err
	}

	return playlists, nil
}

// GetPlaylist method for getting one playlist by given ID.
func (q *PlaylistQueries) GetPlaylist(id int64) (*models.Playlist, error) {
	playlist := &models.Playlist{}

	query := `SELECT * FROM playlist WHERE id = ? LIMIT 1`

	err := q.Get(playlist, query, id)
	if err != nil {
		return playlist, err
	}

	return playlist, nil
}

// CreatePlaylist method for creating a playlist by given Playlist object.
func (q *PlaylistQueries) CreatePlaylist(p models.Playlist) (int64, error) {
	query := `INSERT INTO playlist VALUES (null, ?, ?, ?, ?)`

	res, err := q.Exec(query, p.Name, p.URL, p.CreatedAt, p.UpdatedAt)
	if err != sql.ErrNoRows && err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		log.Warn().Msgf("Error retrieving the ID: %v", err)
		return 0, err
	}

	return id, nil
}

// UpdatePlaylist method for updating playlist by given Playlist object.
func (q *PlaylistQueries) UpdatePlaylist(id int64, p *models.Playlist) error {
	query := `UPDATE playlist SET name = ?, url = ?, updated_at = ? WHERE id = ?`

	_, err := q.Exec(query, p.Name, p.URL, p.UpdatedAt, id)
	if err != nil {
		return err
	}

	return nil
}

// DeletePlaylist method for delete playlist by given ID.
func (q *PlaylistQueries) DeletePlaylist(id int64) error {
	query := `DELETE FROM playlist WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}

// Check if channelURL exists and return id
func (q *PlaylistQueries) ChannelUrlExists(plChannelID int64) (int64, error) {
	var channelID int64

	query := `SELECT id FROM channelurl WHERE channel_id = ?`

	err := q.Get(&channelID, query, plChannelID)
	if err != nil {
		return 0, err
	}

	return channelID, nil
}

// Get Playlist Groups by playlist
func (q *PlaylistQueries) GetPlGroups(playlistId int64) (*[]models.PlaylistGroup, error) {
	playlistgroups := &[]models.PlaylistGroup{}

	query := `SELECT * FROM playlistgroup
	WHERE playlist_id = ?`

	err := q.Select(playlistgroups, query, playlistId)
	if err != nil {
		return nil, err
	}

	return playlistgroups, nil
}

// Get all Playlist Groups
func (q *PlaylistQueries) GetAllPlGroups() (*[]models.PlaylistGroup, error) {
	playlistgroup := &[]models.PlaylistGroup{}

	query := `SELECT * FROM playlistgroup`

	err := q.Select(playlistgroup, query)
	if err != nil {
		return nil, err
	}

	return playlistgroup, nil
}

// Get Playlist Groupby Id
func (q *PlaylistQueries) GetPlGroup(id int64) (*models.PlaylistGroup, error) {
	group := &models.PlaylistGroup{}

	query := `SELECT * FROM playlistgroup WHERE id = ?`

	err := q.Get(group, query, id)
	if err != nil {
		return group, err
	}

	return group, nil
}

// Get Playlist Group by given Name.
func (q *PlaylistQueries) GetPlGroupByName(playlistId int64, name string) (*models.PlaylistGroup, error) {
	group := &models.PlaylistGroup{}

	query := `SELECT * FROM playlistgroup 
	WHERE playlist_id = ?
	AND name = ?`

	err := q.Get(group, query, playlistId, name)
	if err != nil {
		return nil, err
	}

	return group, nil
}

// Create Playlist Group.
func (q *PlaylistQueries) CreatePlGroup(group models.PlaylistGroup) (int64, error) {
	query := `INSERT INTO playlistgroup VALUES (null, ?, ?, true)`

	// Add retry mechanism with exponential backoff
	maxRetries := 5
	baseDelay := 100 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		res, err := q.Exec(query, group.Name, group.PlaylistId)

		if err != nil {
			// Check if it's a database lock error
			if strings.Contains(err.Error(), "database is locked") {
				// Calculate exponential backoff delay with some jitter
				delay := baseDelay * time.Duration(1<<uint(i)) // 2^i * baseDelay
				jitter := time.Duration(rand.Intn(50)) * time.Millisecond
				retryDelay := delay + jitter

				log.Warn().Msgf("Database locked when creating playlist group '%s', retrying in %v (attempt %d/%d)",
					group.Name, retryDelay, i+1, maxRetries)

				time.Sleep(retryDelay)
				continue
			} else if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				// If the group already exists (UNIQUE constraint violation)
				log.Info().Msgf("Playlist group '%s' already exists for playlist ID %d", group.Name, group.PlaylistId)

				// Get the existing group ID
				existingGroup, findErr := q.GetPlGroupByName(group.PlaylistId, group.Name)
				if findErr == nil && existingGroup != nil {
					return existingGroup.ID, nil
				}
				return 0, err
			} else if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
				// Foreign key constraint failed - likely the playlist_id doesn't exist
				log.Error().Msgf("Foreign key constraint failed when creating playlist group '%s' for playlist ID %d: %v",
					group.Name, group.PlaylistId, err)
				return 0, err
			} else {
				// For other errors
				log.Error().Msgf("Failed to create playlist group '%s': %v", group.Name, err)
				return 0, err
			}
		}

		// No error, get the last inserted ID
		id, err := res.LastInsertId()
		if err != nil {
			return 0, err
		}

		log.Info().Msgf("Successfully created playlist group '%s' with ID %d", group.Name, id)
		return id, nil
	}

	// If we reached here, all retries failed
	return 0, fmt.Errorf("failed to create playlist group after %d retries: database remains locked", maxRetries)
}

// Update Playlist Group by given Playlist Group object.
func (q *PlaylistQueries) UpdatePlGroup(p *models.PlaylistGroup) error {
	query := `UPDATE playlistgroup SET enabled = ? WHERE id = ?`

	_, err := q.Exec(query, p.Enabled, p.ID)
	if err != nil {
		return err
	}

	return nil
}

// Delete Playlist Group method for delete Playlist Group by given ID.
func (q *PlaylistQueries) DeletePlGroup(id int64) error {
	query := `DELETE FROM playlistgroup WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}

// Delete All Playlist Groups.
func (q *PlaylistQueries) DeletePlGroups() error {
	query := `DELETE FROM playlistgroup`

	_, err := q.Exec(query)
	if err != nil {
		return err
	}

	return nil
}

// Get Playlist channels by playlist ID
func (q *PlaylistQueries) GetPlChannels(playlistId int64) (*[]models.PlaylistChannel, error) {
	channels := &[]models.PlaylistChannel{}

	query := `SELECT pc.* FROM playlistchannel pc
	JOIN playlistgroup pg ON pg.id = pc.group_id
	WHERE pg.playlist_id = ?
	ORDER BY pg.id ASC, pc.orderr ASC`

	err := q.Select(channels, query, playlistId)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Get Playlist channels by ID
func (q *PlaylistQueries) GetChannelsByPl(id int64) (*[]models.PlaylistChannel, error) {
	channels := &[]models.PlaylistChannel{}

	query := `SELECT playlistchannel.* FROM playlistchannel
	JOIN playlistgroup ON playlistgroup.id = playlistchannel.group_id
	WHERE playlistgroup.playlist_id = ?`

	err := q.Select(channels, query, id)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Get Playlist Channels by Group ID
func (q *PlaylistQueries) GetPlGroupChannels(groupId int64) ([]models.PlaylistChannel, error) {
	channels := []models.PlaylistChannel{}

	query := `SELECT pc.* FROM playlistchannel pc
	WHERE pc.group_id = ?
	ORDER BY pc.title ASC`

	err := q.Select(&channels, query, groupId)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Get one channel by given ID.
func (q *PlaylistQueries) GetPlChannel(id int64) (*models.PlaylistChannel, error) {
	channel := &models.PlaylistChannel{}

	query := `SELECT * FROM playlistchannel WHERE id = ?`

	err := q.Get(channel, query, id)
	if err != nil {
		return nil, err
	}

	return channel, nil
}

// Get Playlist channels by name
func (q *PlaylistQueries) GetPlChannelsByName(name string) (*[]models.PlaylistChannel, error) {
	channels := &[]models.PlaylistChannel{}

	query := `SELECT * FROM playlistchannel WHERE title = ?`

	err := q.Select(channels, query, name)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Get Playlist channels by tvgid
func (q *PlaylistQueries) GetPlChannelsByTvgID(tvgid string) ([]models.PlaylistChannel, error) {
	channels := []models.PlaylistChannel{}

	query := `SELECT * FROM playlistchannel WHERE tvg_id = ?`

	err := q.Select(&channels, query, tvgid)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Get Playlist channels by tvgid
func (q *PlaylistQueries) GetM3UParseByTvgID(tvgId string, groupId int64, playlistId int64) ([]models.PlaylistChannel, error) {
	channels := []models.PlaylistChannel{}

	query := `SELECT pc.* FROM playlistchannel pc
	JOIN playlistgroup pg ON pg.id = pc.group_id
	WHERE pc.tvg_id = ? 
	AND pg.id = ?
	AND pg.playlist_id = ?`

	err := q.Select(&channels, query, tvgId, groupId, playlistId)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Get Playlist channels by tvgname
func (q *PlaylistQueries) GetM3UParseByTvgName(tvg_name string, groupId int64, playlistId int64) ([]models.PlaylistChannel, error) {
	channels := []models.PlaylistChannel{}

	query := `SELECT pc.* FROM playlistchannel pc
	JOIN playlistgroup pg ON pg.id = pc.group_id
	WHERE pc.tvg_name = ? 
	AND pg.id = ?
	AND pg.playlist_id = ?`

	err := q.Select(&channels, query, tvg_name, groupId, playlistId)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Get Playlist channels by title
func (q *PlaylistQueries) GetM3UParseByTitle(title string, groupId int64, playlistId int64) ([]models.PlaylistChannel, error) {
	channels := []models.PlaylistChannel{}

	query := `SELECT pc.* FROM playlistchannel pc
	JOIN playlistgroup pg ON pg.id = pc.group_id
	WHERE pc.title = ? 
	AND pg.id = ?
	AND pg.playlist_id = ?`

	err := q.Select(&channels, query, title, groupId, playlistId)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Create a channel by given Channel object.
func (q *PlaylistQueries) CreatePlChannel(p models.PlaylistChannel) (int64, error) {
	query := `INSERT INTO playlistchannel VALUES (null, ?, ?, ?, ?, ?, ?, ?, ?)`

	res, err := q.Exec(query, p.GroupId, p.Title, p.TvgID, p.TvgName, p.Logo, p.Enabled, p.CreatedAt, p.UpdatedAt)
	if err != sql.ErrNoRows && err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Warn().Msgf("Error retrieving the ID: %v", err)
		return 0, err
	}

	return id, nil
}

// Update a channel by given Channel object.
func (q *PlaylistQueries) UpdatePlChannel(id int64, p models.PlaylistChannel) error {
	query := `UPDATE playlistchannel SET group_id = ?, title = ?, tvg_id = ?, tvg_name = ?, tvg_logo = ?, enabled = ?, updated_at = ? WHERE id = ?`

	_, err := q.Exec(query, p.GroupId, p.Title, p.TvgID, p.TvgName, p.Logo, p.Enabled, p.UpdatedAt, id)
	if err != nil {
		return err
	}

	return nil
}

// Delete a channel by given ID.
func (q *PlaylistQueries) DeletePlChannel(id int64) error {
	query := `DELETE FROM playlistchannel WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}

// Delete All Playlist Channels.
func (q *PlaylistQueries) DeletePlChannels() error {
	query := `DELETE FROM playlistchannel`

	_, err := q.Exec(query)
	if err != nil {
		return err
	}

	return nil
}

// Get channel URL by channel ID.
func (q *PlaylistQueries) GetChannelUrl(id int64) (*models.ChannelUrl, error) {
	channelUrl := &models.ChannelUrl{}

	query := `SELECT * FROM channelurl WHERE channel_id = ?`

	err := q.Get(channelUrl, query, id)
	if err != nil {
		return nil, err
	}

	return channelUrl, nil
}

// Create a channel URL by given ChannelUrl object.
func (q *PlaylistQueries) CreateChannelUrl(p models.ChannelUrl) error {
	query := `INSERT INTO channelurl VALUES (null, ?, ?, ?, ?, ?)`

	_, err := q.Exec(query, p.Url, p.ChannelId, p.Order, p.CreatedAt, p.UpdatedAt)
	if err != sql.ErrNoRows && err != nil {
		return err
	}

	return nil
}

// Update a channel URL by given ChannelUrl object.
func (q *PlaylistQueries) UpdateChannelUrl(id int64, p models.ChannelUrl) error {
	query := `UPDATE channelurl SET url = ?, channel_id = ?, orderr = ?, updated_at = ? WHERE id = ?`

	_, err := q.Exec(query, p.Url, p.ChannelId, p.Order, p.UpdatedAt, id)
	if err != nil {
		return err
	}

	return nil
}

// Delete a channel URL by given ID.
func (q *PlaylistQueries) DeleteChannelUrl(id int64) error {
	query := `DELETE FROM channelurl WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}
