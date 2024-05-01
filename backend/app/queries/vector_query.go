package queries

import (
	"database/sql"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

type VectorQueries struct {
	*sqlx.DB
}

func (q *VectorQueries) GetTemplateChannelVector(channel_id int64) (*models.TemplateChannelVector, error) {
	vectorChannel := &models.TemplateChannelVector{}

	query := `SELECT * FROM templatechannelvectors WHERE channel_id = ?`

	err := q.Get(vectorChannel, query, channel_id)
	if err != nil {
		return nil, err
	}

	return vectorChannel, nil
}

func (q *VectorQueries) GetTemplateChannelVectors() ([]models.TemplateChannelVector, error) {
	vectorChannels := []models.TemplateChannelVector{}

	query := `SELECT * FROM templatechannelvectors`

	err := q.Select(&vectorChannels, query)
	if err != nil {
		return nil, err
	}

	return vectorChannels, nil
}

func (q *VectorQueries) CreateTemplateChannelVector(channelVector *models.TemplateChannelVector) (int64, error) {
	query := `INSERT INTO templatechannelvectors VALUES (null, ?, ?)`

	res, err := q.Exec(query, channelVector.ChannelId, channelVector.VectorId)
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

func (q *VectorQueries) UpdateTemplateChannelVector(channelVector *models.TemplateChannelVector) error {
	query := `UPDATE templatechannelvectors SET vector_id = ? WHERE channel_id = ?`

	_, err := q.Exec(query, channelVector.VectorId, channelVector.ChannelId)
	if err != nil {
		return err
	}

	return nil
}

func (q *VectorQueries) GetPlaylistChannelVectors() ([]models.PlaylistChannelVector, error) {
	vectorChannels := []models.PlaylistChannelVector{}

	query := `SELECT playlistchannelvectors.* FROM playlistchannelvectors
			JOIN playlistchannel ON playlistchannel.id = playlistchannelvectors.channel_id
			JOIN playlistgroup ON playlistgroup.id = playlistchannel.group_id
			WHERE playlistgroup.enabled = true`

	if err := q.Select(&vectorChannels, query); err != nil {
		return nil, err
	}

	return vectorChannels, nil
}

func (q *VectorQueries) GetPlChVectorsByPlaylist(playlistId int64) ([]models.PlaylistChannelVector, error) {
	vectorChannels := []models.PlaylistChannelVector{}

	query := `SELECT playlistchannelvectors.* FROM playlistchannelvectors
			JOIN playlistchannel ON playlistchannel.id = playlistchannelvectors.channel_id
			JOIN playlistgroup ON playlistgroup.id = playlistchannel.group_id
			JOIN playlist ON playlist.id = playlistgroup.playlist_id
			WHERE playlistgroup.enabled = true
			AND playlist.id = ?`

	if err := q.Select(&vectorChannels, query, playlistId); err != nil {
		return nil, err
	}

	return vectorChannels, nil
}

func (q *VectorQueries) GetPlaylistChannelVector(channel_id int64) (*models.PlaylistChannelVector, error) {
	vectorChannel := &models.PlaylistChannelVector{}

	query := `SELECT * FROM playlistchannelvectors WHERE channel_id = ?`

	err := q.Get(vectorChannel, query, channel_id)
	if err != nil {
		return nil, err
	}

	return vectorChannel, nil
}

func (q *VectorQueries) CreatePlaylistChannelVector(channelVector *models.PlaylistChannelVector) (int64, error) {
	query := `INSERT INTO playlistchannelvectors VALUES (null, ?, ?)`

	res, err := q.Exec(query, channelVector.ChannelId, channelVector.VectorId)
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

func (q *VectorQueries) UpdatePlaylistChannelVector(channelVector *models.PlaylistChannelVector) error {
	query := `UPDATE playlistchannelvectors SET vector_id = ? WHERE channel_id = ?`

	_, err := q.Exec(query, channelVector.VectorId, channelVector.ChannelId)
	if err != nil {
		return err
	}

	return nil
}

func (q *VectorQueries) GetChannelVector(id int64) (*models.ChannelVector, error) {
	vectorChannel := &models.ChannelVector{}

	query := `SELECT * FROM channelvectors WHERE id = ?`

	err := q.Get(vectorChannel, query, id)
	if err != nil {
		return nil, err
	}

	return vectorChannel, nil
}

func (q *VectorQueries) GetChannelVectorByName(name string) (*models.ChannelVector, error) {
	vectorChannel := &models.ChannelVector{}

	query := `SELECT * FROM channelvectors WHERE name = ?`

	err := q.Get(vectorChannel, query, name)
	if err != nil {
		return nil, err
	}

	return vectorChannel, nil
}

func (q *VectorQueries) CreateChannelVector(channelVector *models.ChannelVector) (int64, error) {
	query := `INSERT INTO channelvectors VALUES (null, ?, ?)`

	res, err := q.Exec(query, channelVector.Name, channelVector.Vector)
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
