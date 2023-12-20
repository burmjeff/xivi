package queries

import (
	"database/sql"
	"xivi/backend/app/models"
	utils "xivi/backend/pkg/dbutils"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// PlaylistQueries struct for queries from Playlist model.
type PlaylistQueries struct {
	*sqlx.DB
}

// GetPlaylists method
func (q *PlaylistQueries) GetPlaylists() ([]models.Playlist, error) {
	playlists := []models.Playlist{}

	// Define query string.
	query := `SELECT * FROM playlist`

	// Send query to database.
	err := q.Select(&playlists, query)
	if err != nil {
		// Return empty object and error.
		return nil, err
	}

	// Return query result.
	return playlists, nil
}

// GetPlaylist method for getting one playlist by given ID.
func (q *PlaylistQueries) GetPlaylist(id int64) (models.Playlist, error) {
	playlist := models.Playlist{}

	// Define query string.
	query := `SELECT * FROM playlist WHERE id = ?`

	// Send query to database.
	err := q.Get(&playlist, query, id)
	if err != nil {
		// Return empty object and error.
		return playlist, err
	}

	// Return query result.
	return playlist, nil
}

// CreatePlaylist method for creating a playlist by given Playlist object.
func (q *PlaylistQueries) CreatePlaylist(p *models.Playlist) (int64, error) {
	// Define query string.
	query := `INSERT INTO playlist VALUES (null, ?, ?, ?, ?)`

	// Send query to database.
	res, err := q.Exec(query, p.Name, p.URL, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		// Return only error.
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		log.Warn().Msgf("Error retrieving the ID: %v", err)
		return 0, err
	}

	// This query returns nothing.
	return id, nil
}

// UpdatePlaylist method for updating playlist by given Playlist object.
func (q *PlaylistQueries) UpdatePlaylist(id int64, p *models.Playlist) error {
	// Define query string.
	query := `UPDATE playlist SET name = ?, url = ?, updated_at = ? WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, p.Name, p.URL, p.UpdatedAt, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// DeletePlaylist method for delete playlist by given ID.
func (q *PlaylistQueries) DeletePlaylist(id int64) error {
	// Define query string.
	query := `DELETE FROM playlist WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// Check if channelURL exists and return id
func (q *PlaylistQueries) ChannelUrlExists(playlistID int64, plChannelID int64) (int64, error) {
	var channelID int64

	// Define query string.
	query := `SELECT id FROM channelurl WHERE playlist_id = ? AND playlist_channel_id = ?`

	// Send query to database.
	err := q.Get(&channelID, query, playlistID, plChannelID)
	if err != nil {
		// Return empty object and error.
		return 0, err
	}

	// Return query result.
	return channelID, nil
}

// Get Playlist Groups by playlist
func (q *PlaylistQueries) GetPlGroups(playlistId int64) (*[]models.PlaylistGroup, error) {
	playlistgroup := &[]models.PlaylistGroup{}

	// Define query string.
	query := `SELECT playlistgroup.* FROM playlistgroup
	JOIN playlist_group_item ON playlistgroup.id = playlist_group_item.group_id
	WHERE playlist_group_item.playlist_id = ?`

	// Send query to database.
	err := q.Select(playlistgroup, query, playlistId)
	if err != nil {
		// Return empty object and error.
		return nil, err
	}

	// Return query result.
	return playlistgroup, nil
}

// Get all Playlist Groups
func (q *PlaylistQueries) GetAllPlGroups() ([]models.PlaylistGroup, error) {
	playlistgroup := []models.PlaylistGroup{}

	// Define query string.
	query := `SELECT playlistgroup.* FROM playlistgroup`

	// Send query to database.
	err := q.Select(&playlistgroup, query)
	if err != nil {
		// Return empty object and error.
		return playlistgroup, err
	}

	// Return query result.
	return playlistgroup, nil
}

// Get Playlist Groupby Id
func (q *PlaylistQueries) GetPlGroup(id int64) (models.PlaylistGroup, error) {
	// Define playlist variable.
	group := models.PlaylistGroup{}

	// Define query string.
	query := `SELECT * FROM playlistgroup WHERE id = ?`

	// Send query to database.
	err := q.Get(&group, query, id)
	if err != nil {
		// Return empty object and error.
		return group, err
	}

	// Return query result.
	return group, nil
}

// Get Playlist Group by given Name.
func (q *PlaylistQueries) GetPlGroupByName(name string) (models.PlaylistGroup, error) {
	// Define group variable.
	group := models.PlaylistGroup{}

	// Define query string.
	query := `SELECT * FROM playlistgroup WHERE name = ? LIMIT 1`

	// Send query to database.
	err := q.Get(&group, query, name)
	if err != nil {
		// Return empty object and error.
		return group, err
	}

	// Return query result.
	return group, nil
}

// Create Playlist Group.
func (q *PlaylistQueries) CreatePlGroup(p *models.PlaylistGroup) (int64, error) {
	// Define query string.
	query := `INSERT INTO playlistgroup VALUES (null, ?)`

	// Send query to database.
	res, err := q.Exec(query, p.Name)
	if err != sql.ErrNoRows && err != nil {
		// Return only error.
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Warn().Msgf("Error retrieving the ID: %v", err)
		return 0, err
	}

	// This query returns nothing.
	return id, nil
}

// Update Playlist Group by given Playlist Group object.
func (q *PlaylistQueries) UpdatePlGroup(id int64, p *models.PlaylistGroup) error {
	// Define query string.
	query := `UPDATE playlistgroup SET name = ? WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, p.Name, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// Delete Playlist Group method for delete Playlist Group by given ID.
func (q *PlaylistQueries) DeletePlGroup(id int64) error {
	// Define query string.
	query := `DELETE FROM playlistgroup WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// Delete all Playlist Groups
func (q *PlaylistQueries) DeletePlGroups() error {
	// Define query string.
	query := `DELETE FROM playlistgroup`

	// Send query to database.
	_, err := q.Exec(query)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// Create Playlist Group method for creating group by given Playlist Group object.
func (q *PlaylistQueries) CreatePlGroupItem(playlistId int64, groupId int64) (int64, error) {
	// Define query string.
	query := `INSERT INTO playlist_group_item VALUES (?, ?)`

	// Send query to database.
	res, err := q.Exec(query, playlistId, groupId)
	if err != nil {
		// Return only error.
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Warn().Msgf("Error retrieving the ID: %v", err)
		return 0, err
	}

	// This query returns nothing.
	return id, nil
}

// Get Playlist Group Items method for getting Playlist Group Items.
func (q *PlaylistQueries) GetPlChannels(playlistId int64) ([]models.PlaylistChannel, error) {
	// Define playlist variable.
	channels := []models.PlaylistChannel{}

	// Define query string.
	query := `SELECT playlistchannel.*
	FROM playlistchannel
	JOIN playlist_group_channel ON playlistchannel.id = playlist_group_channel.channel_id
	AND playlist_group_channel.group_id = playlist_group_item.group_id
	JOIN playlist_group_item ON playlist_group_channel.group_id = playlist_group_item.group_id
	WHERE playlist_group_item.playlist_id = ?;`

	// Send query to database.
	err := q.Select(&channels, query, playlistId)
	if err != nil {
		// Return empty object and error.
		return channels, err
	}

	// Return query result.
	return channels, nil
}

// Create Playlist Group method for creating group by given Playlist Group object.
func (q *PlaylistQueries) CreatePlGroupChannel(groupId int64, channelId int64) (int64, error) {
	// Define query string.
	query := `INSERT INTO playlist_group_channel VALUES (?, ?)`

	// Send query to database.
	res, err := q.Exec(query, groupId, channelId)
	if err != nil {
		// Return only error.
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Warn().Msgf("Error retrieving the ID: %v", err)
		return 0, err
	}

	// This query returns nothing.
	return id, nil
}

// GetChannels method for getting all channels by Playlist.
func (q *PlaylistQueries) GetChannelsByPl(id int64) ([]models.PlaylistChannel, error) {
	channels := []models.PlaylistChannel{}

	// Define query string.
	query := `SELECT DISTINCT playlistchannel.id
	FROM playlist
	JOIN channelurl ON playlist.id = channelurl.playlist_id
	JOIN playlistchannel ON channelurl.playlist_channel_id = playlistchannel.id 
	WHERE playlist.id = ?`

	// Send query to database.
	err := q.Select(&channels, query, id)
	if err != nil {
		// Return empty object and error.
		return channels, err
	}

	// Return query result.
	return channels, nil
}

// GetChannels method for getting all channels by Playlist Group.
func (q *PlaylistQueries) GetPlGroupChannels(groupId int64) ([]models.PlaylistChannel, error) {
	channels := []models.PlaylistChannel{}

	// Define query string.
	query := `SELECT playlistchannel.* FROM playlistchannel
	JOIN playlist_group_channel ON playlistchannel.id = playlist_group_channel.channel_id
	WHERE playlist_group_channel.group_id = ?;`

	// Send query to database.
	err := q.Select(&channels, query, groupId)
	if err != nil {
		// Return empty object and error.
		log.Err(err)
		return nil, err
	}

	// Return query result.
	return channels, nil
}

// GetChannel method for getting one channel by given ID.
func (q *PlaylistQueries) GetPlChannel(id int64) (models.PlaylistChannel, error) {
	// Define channel variable.
	channel := models.PlaylistChannel{}

	// Define query string.
	query := `SELECT * FROM playlistchannel WHERE id = ?`

	// Send query to database.
	if err := q.Get(&channel, query, id); err != nil {
		// Return empty object and error.
		return channel, err
	}

	// Return query result.
	return channel, nil
}

// Get*PlaylistChannel method for getting one channel by Name.
func (q *PlaylistQueries) GetPlChannelsByName(name string) ([]models.PlaylistChannel, error) {
	// Define channel variable.
	channels := []models.PlaylistChannel{}

	// Define query string.
	query := `SELECT * FROM playlistchannel WHERE name LIKE ?`

	// Send query to database.
	err := q.Select(&channels, query, name)
	if err != nil {
		// Return empty object and error.
		return nil, err
	}

	// Return query result.
	return channels, nil
}

// Get*PlaylistChannel method for getting one channel by TvgID.
func (q *PlaylistQueries) GetPlChannelsByTvgID(tvgid string) ([]models.PlaylistChannel, error) {
	// Define channel variable.
	channels := []models.PlaylistChannel{}

	// Define query string.
	query := `SELECT * FROM playlistchannel WHERE tvg_id LIKE ?`

	// Send query to database.
	if err := q.Select(&channels, query, tvgid); err != nil {
		return nil, err
	}

	// Return query result.
	return channels, nil
}

func (q *PlaylistQueries) GetM3UParseByTvgID(tvgId string, groupId int64, playlistId int64) ([]models.PlaylistChannel, error) {
	// Define channel variable.
	channels := []models.PlaylistChannel{}

	// Define query string.
	query := `SELECT playlistchannel.* FROM playlistchannel
		JOIN playlist_group_channel ON playlist_group_channel.channel_id = playlistchannel.id AND playlist_group_channel.group_id = ?
		JOIN playlist_group_item ON playlist_group_item.group_id = playlist_group_channel.group_id AND playlist_group_item.playlist_id = ?
		WHERE tvg_id LIKE ?;`

	// Send query to database.
	if err := q.Select(&channels, query, groupId, playlistId, tvgId); err != nil {
		return nil, err
	}

	// Return query result.
	return channels, nil
}

func (q *PlaylistQueries) CleanPlaylistGroups(ids []int64) error {
	query := `DELETE FROM playlistgroup 
		JOIN playlist_group_item ON playlist_group_item.group_id = playlistgroup.id
		WHERE playlist_group_item.playlist_id NOT IN ?`

	// Send query to database.
	if _, err := q.Exec(query, ids); err != nil {
		return err
	}

	// Return query result.
	return nil
}

func (q *PlaylistQueries) CleanPlaylistChannels(ids []int64) error {
	query := `DELETE FROM playlistchannel 
		JOIN playlist_group_item ON playlist_group_item.group_id = playlistgroup.id
		JOIN playlist_group_channel ON playlist_group_item.group_id = playlist_group_channel.group_id
		WHERE playlistchannel.id = playlist_group_channel.channel_id
		AND playlist_group_item.playlist_id NOT IN ?`

	// Send query to database.
	if _, err := q.Exec(query, ids); err != nil {
		return err
	}

	// Return query result.
	return nil
}

// CreateChannel method for creating a Channel by given Channel object.
func (q *PlaylistQueries) CreatePlChannel(p *models.PlaylistChannel) (int64, error) {
	// Define query string.
	query := `INSERT INTO playlistchannel VALUES (null, ?, ?, ?, ?, ?, ?, ?)`

	// Send query to database.
	res, err := q.Exec(query, utils.NewNullString(p.TvgID), p.TvgName, utils.NewNullString(p.Logo), p.Title, p.Enabled, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		// Return only error.
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		log.Warn().Msgf("Error retrieving the ID: %v", err)
		return 0, err
	}

	// This query returns nothing.
	return id, nil
}

// UpdatePlaylist method for updating a channel by given Channel object.
func (q *PlaylistQueries) UpdatePlChannel(id int64, p *models.PlaylistChannel) error {
	// Define query string.
	query := `UPDATE playlistchannel SET tvg_id = ?, tvg_name = ?, tvg_logo = ?, title = ?, enabled = ?, updated_at = ? WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, p.TvgID, p.TvgName, p.Logo, p.Title, p.Enabled, p.UpdatedAt, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// DeleteChannel method for delete channel by given ID.
func (q *PlaylistQueries) DeletePlChannel(id int64) error {
	// Define query string.
	query := `DELETE FROM playlistchannel WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// Delete Playlist Channels
func (q *PlaylistQueries) DeletePlChannels() error {
	// Define query string.
	query := `DELETE FROM playlistchannel`

	// Send query to database.
	_, err := q.Exec(query)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// GetChannel method for getting one channel by given ID.
func (q *PlaylistQueries) GetChannelUrl(id int64) (models.ChannelUrl, error) {
	// Define channel variable.
	channel := models.ChannelUrl{}

	// Define query string.
	query := `SELECT * FROM channelurl WHERE id = ?`

	// Send query to database.
	err := q.Select(&channel, query, id)
	if err != nil {
		// Return empty object and error.
		return channel, err
	}

	// Return query result.
	return channel, nil
}

// GetChannel method for getting one channel by given ID.
func (q *PlaylistQueries) GetChannelUrlByPlChannelID(id int64) (models.ChannelUrl, error) {
	// Define channel variable.
	channel := models.ChannelUrl{}

	// Define query string.
	query := `SELECT * FROM channelurl WHERE playlist_channel_id = ?`

	// Send query to database.
	err := q.Select(&channel, query, id)
	if err != nil {
		// Return empty object and error.
		return channel, err
	}

	// Return query result.
	return channel, nil
}

// CreateChannel method for creating a Channel by given Channel object.
func (q *PlaylistQueries) CreateChannelUrl(p *models.ChannelUrl) error {
	// Define query string.
	query := `INSERT INTO channelurl VALUES (null, ?, ?, ?, ?, ?, ?)`

	// Send query to database.
	_, err := q.Exec(query, p.Url, p.PlaylistID, p.PlaylistChannelId, p.Order, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// UpdatePlaylist method for updating a channel by given Channel object.
func (q *PlaylistQueries) UpdateChannelUrl(id int64, p *models.ChannelUrl) error {
	// Define query string.
	query := `UPDATE channelurl SET url = ?, playlist_id = ?, playlist_channel_id = ?, orderr = ?, updated_at = ? WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, p.Url, p.PlaylistID, p.PlaylistChannelId, p.Order, p.UpdatedAt, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// DeleteChannel method for delete channel by given ID.
func (q *PlaylistQueries) DeleteChannelUrl(id int64) error {
	// Define query string.
	query := `DELETE FROM channelurl WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}
