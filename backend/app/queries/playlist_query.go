package queries

import (
	"xivi/backend/app/models"
	utils "xivi/backend/pkg/dbutils"

	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
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
	query := `SELECT * FROM playlist WHERE id = $1`

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
	query := `INSERT INTO playlist VALUES (null, $1, $2, $3, $4)`

	// Send query to database.
	res, err := q.Exec(query, p.Name, p.URL, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		// Return only error.
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		log.Warnln("Error retrieving the ID: %v", err)
		return 0, err
	}

	// This query returns nothing.
	return id, nil
}

// UpdatePlaylist method for updating playlist by given Playlist object.
func (q *PlaylistQueries) UpdatePlaylist(id int64, p *models.Playlist) error {
	// Define query string.
	query := `UPDATE playlist SET name = $2, url = $3, updated_at = $4 WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id, p.Name, p.URL, p.UpdatedAt)
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
	query := `DELETE FROM playlist WHERE id = $1`

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
	query := `SELECT id FROM channelurl WHERE playlist_id = $1 AND playlist_channel_id = $2`

	// Send query to database.
	err := q.Get(&channelID, query, playlistID, plChannelID)
	if err != nil {
		// Return empty object and error.
		return 0, err
	}

	// Return query result.
	return channelID, nil
}

// Get Playlist Groups method
func (q *PlaylistQueries) GetPlGroups() ([]models.PlaylistGroup, error) {
	playlistgroup := []models.PlaylistGroup{}

	// Define query string.
	query := `SELECT * FROM playlistgroup`

	// Send query to database.
	err := q.Select(&playlistgroup, query)
	if err != nil {
		// Return empty object and error.
		return playlistgroup, err
	}

	// Return query result.
	return playlistgroup, nil
}

// Get Playlist Group method for getting one goup by given ID.
func (q *PlaylistQueries) GetPlGroup(id int64) (models.PlaylistGroup, error) {
	// Define playlist variable.
	group := models.PlaylistGroup{}

	// Define query string.
	query := `SELECT * FROM playlistgroup WHERE id = $1`

	// Send query to database.
	err := q.Get(&group, query, id)
	if err != nil {
		// Return empty object and error.
		return group, err
	}

	// Return query result.
	return group, nil
}

// Get Playlist Group method for getting one group by given Name.
func (q *PlaylistQueries) GetPlGroupByName(name string) (models.PlaylistGroup, error) {
	// Define group variable.
	group := models.PlaylistGroup{}

	// Define query string.
	query := `SELECT * FROM playlistgroup WHERE name = $1`

	// Send query to database.
	err := q.Get(&group, query, name)
	if err != nil {
		// Return empty object and error.
		return group, err
	}

	// Return query result.
	return group, nil
}

// Create Playlist Group method for creating group by given Playlist Group object.
func (q *PlaylistQueries) CreatePlGroup(p *models.PlaylistGroup) (int64, error) {
	// Define query string.
	query := `INSERT INTO playlistgroup VALUES (null, ?)`

	// Send query to database.
	res, err := q.Exec(query, p.Name)
	if err != nil {
		// Return only error.
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Warnln("Error retrieving the ID: %v", err)
		return 0, err
	}

	// This query returns nothing.
	return id, nil
}

// Update Playlist Group method for updating group by given Playlist Group object.
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

// Get Playlist Group method for getting one goup by given ID.
func (q *PlaylistQueries) GetPlItems(id int64) ([]models.PlaylistItem, error) {
	// Define playlist variable.
	items := []models.PlaylistItem{}

	// Define query string.
	query := `SELECT * FROM playlistitem WHERE playlist_id = ?`

	// Send query to database.
	err := q.Select(&items, query, id)
	if err != nil {
		// Return empty object and error.
		return items, err
	}

	// Return query result.
	return items, nil
}

// Get Playlist Group method for getting one goup by given ID.
func (q *PlaylistQueries) GetPlItemExists(item *models.PlaylistItem) error {
	var playlistItemId int64
	// Define query string.
	query := `SELECT id FROM playlistitem WHERE playlist_id = ? AND group_id = ? LIMIT 1`

	// Send query to database.
	err := q.Get(&playlistItemId, query, item.PlaylistId, item.GroupId)
	if err != nil {
		// Return empty object and error.
		return err
	}

	// Return query result.
	return nil
}

// Create Playlist Group method for creating group by given Playlist Group object.
func (q *PlaylistQueries) CreatePlItem(p *models.PlaylistItem) (int64, error) {
	// Define query string.
	query := `INSERT INTO playlistitem VALUES (null, ?, ?)`

	// Send query to database.
	res, err := q.Exec(query, p.PlaylistId, p.GroupId)
	if err != nil {
		// Return only error.
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		log.Warnln("Error retrieving the ID: %v", err)
		return 0, err
	}

	// This query returns nothing.
	return id, nil
}

// Update Playlist Group method for updating group by given Playlist Group object.
func (q *PlaylistQueries) UpdatePlItem(id int64, p *models.PlaylistItem) error {
	// Define query string.
	query := `UPDATE playlistitem SET playlist_id = ?, group_id = ? WHERE id = ?`

	// Send query to database.
	_, err := q.Exec(query, p.PlaylistId, p.GroupId, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}

// GetChannels method for getting all channels.
func (q *PlaylistQueries) GetPlChannels() ([]models.PlaylistChannel, error) {
	channels := []models.PlaylistChannel{}

	// Define query string.
	query := `SELECT * FROM playlistchannel`

	// Send query to database.
	err := q.Select(&channels, query)
	if err != nil {
		// Return empty object and error.
		return channels, err
	}

	// Return query result.
	return channels, nil
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
func (q *PlaylistQueries) GetChannelsByPlGroup(id int64) ([]models.PlaylistChannel, error) {
	channels := []models.PlaylistChannel{}

	// Define query string.
	query := `SELECT * FROM playlistchannel WHERE group_id = ?`

	// Send query to database.
	err := q.Select(&channels, query, id)
	if err != nil {
		// Return empty object and error.
		log.Error(err)
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
	query := `SELECT * FROM playlistchannel WHERE id = $1`

	// Send query to database.
	err := q.Select(&channel, query, id)
	if err != nil {
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
	query := `SELECT * FROM playlistchannel WHERE name LIKE $1`

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
	query := `SELECT * FROM playlistchannel WHERE tvgid LIKE $1`

	// Send query to database.
	err := q.Select(&channels, query, tvgid)
	if err != nil {
		// Return empty object and error.
		return nil, err
	}

	// Return query result.
	return channels, nil
}

// CreateChannel method for creating a Channel by given Channel object.
func (q *PlaylistQueries) CreatePlChannel(p *models.PlaylistChannel) (int64, error) {
	// Define query string.
	query := `INSERT INTO playlistchannel VALUES (null, $1, $2, $3, $4, $5, $6, $7)`

	// Send query to database.
	res, err := q.Exec(query, utils.NewNullString(p.TvgID), p.Name, utils.NewNullString(p.Logo), p.GroupID, p.Enabled, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		// Return only error.
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		log.Warnln("Error retrieving the ID: %v", err)
		return 0, err
	}

	// This query returns nothing.
	return id, nil
}

// UpdatePlaylist method for updating a channel by given Channel object.
func (q *PlaylistQueries) UpdatePlChannel(id int64, p *models.PlaylistChannel) error {
	// Define query string.
	query := `UPDATE playlistchannel SET tvgid = $2, name = $3, tvg_logo = $4, group_id = $5, enabled = $6, updated_at = $7 WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id, p.TvgID, p.Name, p.Logo, p.GroupID, p.Enabled, p.UpdatedAt)
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
	query := `DELETE FROM playlistchannel WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id)
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
	query := `SELECT * FROM channelurl WHERE id = $1`

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
	query := `SELECT * FROM channelurl WHERE playlist_channel+id = $1`

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
	query := `INSERT INTO channelurl VALUES (null, $1, $2, $3, $4, $5, $6)`

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
	query := `UPDATE channelurl SET url = $2, playlist_id = $3, playlist_channel_id = $4, orderr = $5, updated_at = $6 WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id, p.Url, p.PlaylistID, p.PlaylistChannelId, p.Order, p.UpdatedAt)
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
	query := `DELETE FROM channelurl WHERE id = $1`

	// Send query to database.
	_, err := q.Exec(query, id)
	if err != nil {
		// Return only error.
		return err
	}

	// This query returns nothing.
	return nil
}
