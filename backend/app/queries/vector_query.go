package queries

import (
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// EpgQueries struct for queries from Epg model.
type VectorQueries struct {
	*sqlx.DB
}

func (q *VectorQueries) GetTemplateChannelVector(channel_id int64) (*models.TemplateChannelVector, error) {
	vectorChannel := &models.TemplateChannelVector{}

	// Define query string.
	query := `SELECT * FROM templatechannelvectors WHERE channel_id = ?`

	// Send query to database.
	err := q.Select(&vectorChannel, query, channel_id)
	if err != nil {
		// Return empty object and error.
		return vectorChannel, err
	}

	// Return query result.
	return vectorChannel, nil
}

func (q *VectorQueries) GetTemplateChannelVectors() (*[]models.TemplateChannelVector, error) {
	vectorChannels := &[]models.TemplateChannelVector{}

	// Define query string.
	query := `SELECT * FROM templatechannelvectors`

	// Send query to database.
	err := q.Select(&vectorChannels, query)
	if err != nil {
		// Return empty object and error.
		return vectorChannels, err
	}

	// Return query result.
	return vectorChannels, nil
}

func (q *VectorQueries) CreateTemplateChannelVector(channelVector *models.TemplateChannelVector) (int64, error) {

	// Define query string.
	query := `INSERT INTO templatechannelvectors VALUES (null, ?, ?)`
	// Send query to database.
	res, err := q.Exec(query, channelVector.ChannelId, channelVector.VectorId)
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

func (q *VectorQueries) UpdateTemplateChannelVector(channelVector *models.TemplateChannelVector) error {

	// Define query string.
	query := `UPDATE templatechannelvectors SET vector_id = ? WHERE channel_id = ?`
	// Send query to database.
	_, err := q.Exec(query, channelVector.VectorId, channelVector.ChannelId)
	if err != nil {
		// Return empty object and error.
		return err
	}

	// Return query result.
	return nil
}

func (q *VectorQueries) GetPlaylistChannelVector(channel_id int64) (*models.PlaylistChannelVector, error) {
	vectorChannel := &models.PlaylistChannelVector{}

	// Define query string.
	query := `SELECT * FROM playlistchannelvectors WHERE channel_id = ?`

	// Send query to database.
	err := q.Select(&vectorChannel, query, channel_id)
	if err != nil {
		// Return empty object and error.
		return vectorChannel, err
	}

	// Return query result.
	return vectorChannel, nil
}

func (q *VectorQueries) CreatePlaylistChannelVector(channelVector *models.PlaylistChannelVector) (int64, error) {

	// Define query string.
	query := `INSERT INTO playlistchannelvectors VALUES (null, ?, ?)`
	// Send query to database.
	res, err := q.Exec(query, channelVector.ChannelId, channelVector.VectorId)
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

func (q *VectorQueries) UpdatePlaylistChannelVector(channelVector *models.PlaylistChannelVector) error {

	// Define query string.
	query := `UPDATE playlistchannelvectors SET vector_id = ? WHERE channel_id = ?`
	// Send query to database.
	_, err := q.Exec(query, channelVector.VectorId, channelVector.ChannelId)
	if err != nil {
		// Return empty object and error.
		return err
	}

	// Return query result.
	return nil
}

func (q *VectorQueries) GetChannelVector(id int64) (models.ChannelVector, error) {
	vectorChannel := models.ChannelVector{}

	// Define query string.
	query := `SELECT * FROM channelvectors WHERE id = ?`

	// Send query to database.
	err := q.Get(&vectorChannel, query, id)
	if err != nil {
		// Return empty object and error.
		return vectorChannel, err
	}

	// Return query result.
	return vectorChannel, nil
}

func (q *VectorQueries) GetChannelVectorByName(name string) (models.ChannelVector, error) {
	vectorChannel := models.ChannelVector{}

	// Define query string.
	query := `SELECT * FROM channelvectors WHERE name = ?`

	// Send query to database.
	err := q.Get(&vectorChannel, query, name)
	if err != nil {
		// Return empty object and error.
		return vectorChannel, err
	}

	// Return query result.
	return vectorChannel, nil
}

func (q *VectorQueries) CreateChannelVector(channelVector models.ChannelVector) (int64, error) {

	// Define query string.
	query := `INSERT INTO channelvectors VALUES (null, ?, ?)`
	// Send query to database.
	res, err := q.Exec(query, channelVector.Name, channelVector.Vector)
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

func (q *VectorQueries) GetFilter(oldName string) (models.ChannelFilter, error) {
	filter := models.ChannelFilter{}

	// Define query string.
	query := `SELECT * FROM channelfilters WHERE oldname = ?`

	// Send query to database.
	err := q.Get(&filter, query, oldName)
	if err != nil {
		// Return empty object and error.
		return filter, err
	}

	// Return query result.
	return filter, nil
}

func (q *VectorQueries) CreateFilter(channelFilter models.ChannelFilter) error {

	// Define query string.
	query := `INSERT INTO channelfilters VALUES (null, ?, ?)`
	// Send query to database.
	_, err := q.Exec(query, channelFilter.OldName, channelFilter.NewName)
	if err != nil {
		// Return empty object and error.
		return err
	}

	// Return query result.
	return nil
}
