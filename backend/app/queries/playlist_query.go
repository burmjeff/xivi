package queries

import (
	"database/sql"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// PlaylistQueries struct for queries from Playlist model.
type PlaylistQueries struct {
	*sqlx.DB
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
func (q *PlaylistQueries) ChannelUrlExists(playlistID int64, plChannelID int64) (int64, error) {
	var channelID int64

	query := `SELECT id FROM channelurl WHERE playlist_id = ? AND playlist_channel_id = ?`

	err := q.Get(&channelID, query, playlistID, plChannelID)
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

	res, err := q.Exec(query, group.Name, group.PlaylistId)
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

// Delete all Playlist Groups
func (q *PlaylistQueries) DeletePlGroups() error {
	query := `DELETE FROM playlistgroup`

	_, err := q.Exec(query)
	if err != nil {
		return err
	}

	return nil
}

// Get Playlist Group Items method for getting Playlist Group Items.
func (q *PlaylistQueries) GetPlChannels(playlistId int64) (*[]models.PlaylistChannel, error) {
	channels := &[]models.PlaylistChannel{}

	query := `SELECT playlistchannel.* FROM playlistchannel
	JOIN playlistgroup ON playlistchannel.group_id = playlistgroup.id
	WHERE playlistgroup.playlist_id = ?;`

	err := q.Select(channels, query, playlistId)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// GetChannels method for getting all channels by Playlist.
func (q *PlaylistQueries) GetChannelsByPl(id int64) (*[]models.PlaylistChannel, error) {
	channels := &[]models.PlaylistChannel{}

	query := `SELECT playlistchannel.* FROM playlistchannel
	JOIN playlistgroup ON playlistchannel.group_id = playlistgroup.id
	WHERE playlistgroup.playlist_id = ?`

	err := q.Select(channels, query, id)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Get all channels by Playlist Group.
func (q *PlaylistQueries) GetPlGroupChannels(groupId int64) ([]models.PlaylistChannel, error) {
	channels := []models.PlaylistChannel{}

	query := `SELECT playlistchannel.* FROM playlistchannel
	JOIN playlistgroup ON playlistchannel.group_id = playlistgroup.id
	WHERE playlistgroup.id = ?;`

	err := q.Select(&channels, query, groupId)
	if err != nil {
		log.Err(err)
		return nil, err
	}

	return channels, nil
}

// Get one channel by given ID.
func (q *PlaylistQueries) GetPlChannel(id int64) (*models.PlaylistChannel, error) {
	channel := &models.PlaylistChannel{}

	query := `SELECT * FROM playlistchannel WHERE id = ?`

	if err := q.Get(channel, query, id); err != nil {
		return nil, err
	}

	return channel, nil
}

// Get channels by Name.
func (q *PlaylistQueries) GetPlChannelsByName(name string) (*[]models.PlaylistChannel, error) {
	channels := &[]models.PlaylistChannel{}

	query := `SELECT * FROM playlistchannel WHERE name LIKE ?`

	err := q.Select(channels, query, name)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// Get channels by TvgID.
func (q *PlaylistQueries) GetPlChannelsByTvgID(tvgid string) ([]models.PlaylistChannel, error) {
	channels := []models.PlaylistChannel{}

	query := `SELECT * FROM playlistchannel WHERE tvg_id LIKE ?`

	if err := q.Select(&channels, query, tvgid); err != nil {
		return nil, err
	}

	return channels, nil
}

func (q *PlaylistQueries) GetM3UParseByTvgID(tvgId string, groupId int64, playlistId int64) ([]models.PlaylistChannel, error) {
	channels := []models.PlaylistChannel{}

	query := `SELECT playlistchannel.* FROM playlistchannel
		JOIN playlistgroup ON playlistchannel.group_id = playlistgroup.id 
		WHERE playlistgroup.id = ?
		AND playlistgroup.playlist_id = ?
		AND playlistchannel.tvg_id LIKE ?;`

	if err := q.Select(&channels, query, groupId, playlistId, tvgId); err != nil {
		return nil, err
	}

	return channels, nil
}

func (q *PlaylistQueries) GetM3UParseByTvgName(tvg_name string, groupId int64, playlistId int64) ([]models.PlaylistChannel, error) {
	channels := []models.PlaylistChannel{}

	query := `SELECT playlistchannel.* FROM playlistchannel
		JOIN playlistgroup ON playlistchannel.group_id = playlistgroup.id 
		WHERE playlistgroup.id = ?
		AND playlistgroup.playlist_id = ?
		AND playlistchannel.tvg_name LIKE ?;`

	if err := q.Select(&channels, query, groupId, playlistId, tvg_name); err != nil {
		return nil, err
	}

	return channels, nil
}

func (q *PlaylistQueries) GetM3UParseByTitle(title string, groupId int64, playlistId int64) ([]models.PlaylistChannel, error) {
	channels := []models.PlaylistChannel{}

	query := `SELECT playlistchannel.* FROM playlistchannel
		JOIN playlistgroup ON playlistchannel.group_id = playlistgroup.id 
		WHERE playlistgroup.id = ?
		AND playlistgroup.playlist_id = ?
		AND playlistchannel.title LIKE ?;`

	if err := q.Select(&channels, query, groupId, playlistId, title); err != nil {
		return nil, err
	}

	return channels, nil
}

// CreateChannel method for creating a Channel by given Channel object.
func (q *PlaylistQueries) CreatePlChannel(p models.PlaylistChannel) (int64, error) {
	query := `INSERT INTO playlistchannel VALUES (null, ?, ?, ?, ?, ?, true, ?, ?)`

	res, err := q.Exec(query, p.TvgID, p.TvgName, p.Logo, p.Title, p.GroupId, p.CreatedAt, p.UpdatedAt)
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

// UpdatePlaylist method for updating a channel by given Channel object.
func (q *PlaylistQueries) UpdatePlChannel(id int64, p models.PlaylistChannel) error {
	query := `UPDATE playlistchannel SET tvg_id = ?, tvg_name = ?, tvg_logo = ?, title = ?, enabled = ?, updated_at = ? WHERE id = ?`

	_, err := q.Exec(query, p.TvgID, p.TvgName, p.Logo, p.Title, p.Enabled, p.UpdatedAt, id)
	if err != nil {
		return err
	}

	return nil
}

// DeleteChannel method for delete channel by given ID.
func (q *PlaylistQueries) DeletePlChannel(id int64) error {
	query := `DELETE FROM playlistchannel WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}

// Delete Playlist Channels
func (q *PlaylistQueries) DeletePlChannels() error {
	query := `DELETE FROM playlistchannel`

	_, err := q.Exec(query)
	if err != nil {
		return err
	}

	return nil
}

// GetChannel URL by playlist channel_id.
func (q *PlaylistQueries) GetChannelUrl(id int64) (*models.ChannelUrl, error) {
	channel := &models.ChannelUrl{}

	query := `SELECT * FROM channelurl WHERE channel_id = ?`

	err := q.Get(channel, query, id)
	if err != nil {
		return nil, err
	}

	return channel, nil
}

// CreateChannel method for creating a Channel by given Channel object.
func (q *PlaylistQueries) CreateChannelUrl(p models.ChannelUrl) error {
	query := `INSERT INTO channelurl VALUES (null, ?, ?, ?, ?, ?)`

	_, err := q.Exec(query, p.Url, p.ChannelId, p.Order, p.CreatedAt, p.UpdatedAt)
	if err != sql.ErrNoRows && err != nil {
		return err
	}

	return nil
}

// UpdatePlaylist method for updating a channel by given Channel object.
func (q *PlaylistQueries) UpdateChannelUrl(id int64, p models.ChannelUrl) error {
	query := `UPDATE channelurl SET url = ?, channel_id = ?, orderr = ?, updated_at = ? WHERE id = ?`

	_, err := q.Exec(query, p.Url, p.ChannelId, p.Order, p.UpdatedAt, id)
	if err != nil {
		return err
	}

	return nil
}

// DeleteChannel method for delete channel by given ID.
func (q *PlaylistQueries) DeleteChannelUrl(id int64) error {
	query := `DELETE FROM channelurl WHERE id = ?`

	_, err := q.Exec(query, id)
	if err != nil {
		return err
	}

	return nil
}
