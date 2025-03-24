package queries

import (
	"context"
	"database/sql"
	"time"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// SQL query constants
const (
	selectFrequentlyUsedVectorsQuery = `
		WITH vector_refs AS (
			SELECT vector_id, COUNT(*) as ref_count 
			FROM (
				SELECT vector_id FROM playlistchannelvectors
				UNION ALL
				SELECT vector_id FROM templatechannelvectors
			)
			GROUP BY vector_id
		)
		SELECT cv.* FROM channelvectors cv
		JOIN vector_refs ON cv.id = vector_refs.vector_id
		ORDER BY vector_refs.ref_count DESC
		LIMIT ?
	`

	selectTemplateChannelVectorQuery     = `SELECT * FROM templatechannelvectors WHERE channel_id = ?`
	selectAllTemplateChannelVectorsQuery = `SELECT * FROM templatechannelvectors`
	insertTemplateChannelVectorQuery     = `INSERT INTO templatechannelvectors VALUES (null, ?, ?)`
	updateTemplateChannelVectorQuery     = `UPDATE templatechannelvectors SET vector_id = ? WHERE channel_id = ?`

	selectPlaylistChannelVectorsQuery = `
		SELECT pcv.* FROM playlistchannelvectors pcv
		JOIN playlistchannel pc ON pc.id = pcv.channel_id
		JOIN playlistgroup pg ON pg.id = pc.group_id
		WHERE pg.enabled = true
	`

	selectPlaylistChannelVectorsByPlaylistQuery = `
		SELECT pcv.* FROM playlistchannelvectors pcv
		JOIN playlistchannel pc ON pc.id = pcv.channel_id
		JOIN playlistgroup pg ON pg.id = pc.group_id
		JOIN playlist pl ON pl.id = pg.playlist_id
		WHERE pg.enabled = true AND pl.id = ?
	`

	selectPlaylistChannelVectorQuery = `SELECT * FROM playlistchannelvectors WHERE channel_id = ?`
	insertPlaylistChannelVectorQuery = `INSERT INTO playlistchannelvectors VALUES (null, ?, ?)`
	updatePlaylistChannelVectorQuery = `UPDATE playlistchannelvectors SET vector_id = ? WHERE channel_id = ?`

	selectChannelVectorQuery       = `SELECT * FROM channelvectors WHERE id = ?`
	selectChannelVectorByNameQuery = `SELECT * FROM channelvectors WHERE name = ?`
	insertChannelVectorQuery       = `INSERT INTO channelvectors VALUES (null, ?, ?)`
)

type VectorQueries struct {
	BaseQueries
}

// NewVectorQueries creates a new VectorQueries instance
func NewVectorQueries(db *sqlx.DB) *VectorQueries {
	return &VectorQueries{
		BaseQueries: NewBaseQueries(db),
	}
}

// GetFrequentlyUsedVectors gets the most frequently referenced vectors in the database
func (q *VectorQueries) GetFrequentlyUsedVectors(ctx context.Context, limit int) ([]models.ChannelVector, error) {
	vectors := []models.ChannelVector{}

	// Use WithContext to support cancellation and timeouts
	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectFrequentlyUsedVectorsQuery)
		if err != nil {
			return err
		}

		start := time.Now()
		err = stmt.SelectContext(ctx, &vectors, limit)
		duration := time.Since(start)

		if duration > 100*time.Millisecond {
			log.Debug().
				Int("limit", limit).
				Float64("duration_ms", float64(duration.Milliseconds())).
				Msg("slow GetFrequentlyUsedVectors query")
		}

		return err
	})

	if err != nil {
		log.Error().Err(err).Int("limit", limit).Msg("Error getting frequently used vectors")
		return nil, err
	}

	return vectors, nil
}

func (q *VectorQueries) GetTemplateChannelVector(channel_id int64) (*models.TemplateChannelVector, error) {
	vectorChannel := &models.TemplateChannelVector{}

	stmt, err := q.GetPreparedStmt(selectTemplateChannelVectorQuery)
	if err != nil {
		return nil, err
	}

	err = stmt.Get(vectorChannel, channel_id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return vectorChannel, nil
}

func (q *VectorQueries) GetTemplateChannelVectors() ([]models.TemplateChannelVector, error) {
	vectorChannels := []models.TemplateChannelVector{}

	stmt, err := q.GetPreparedStmt(selectAllTemplateChannelVectorsQuery)
	if err != nil {
		return nil, err
	}

	err = stmt.Select(&vectorChannels)
	if err != nil {
		return nil, err
	}

	return vectorChannels, nil
}

func (q *VectorQueries) CreateTemplateChannelVector(channelVector *models.TemplateChannelVector) (int64, error) {
	stmt, err := q.GetPreparedStmt(insertTemplateChannelVectorQuery)
	if err != nil {
		return 0, err
	}

	res, err := stmt.Exec(channelVector.ChannelId, channelVector.VectorId)
	if err != nil {
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
	stmt, err := q.GetPreparedStmt(updateTemplateChannelVectorQuery)
	if err != nil {
		return err
	}

	_, err = stmt.Exec(channelVector.VectorId, channelVector.ChannelId)
	return err
}

func (q *VectorQueries) GetPlaylistChannelVectors() ([]models.PlaylistChannelVector, error) {
	vectorChannels := []models.PlaylistChannelVector{}

	stmt, err := q.GetPreparedStmt(selectPlaylistChannelVectorsQuery)
	if err != nil {
		return nil, err
	}

	if err := stmt.Select(&vectorChannels); err != nil {
		return nil, err
	}

	return vectorChannels, nil
}

func (q *VectorQueries) GetPlChVectorsByPlaylist(playlistId int64) ([]models.PlaylistChannelVector, error) {
	vectorChannels := []models.PlaylistChannelVector{}

	stmt, err := q.GetPreparedStmt(selectPlaylistChannelVectorsByPlaylistQuery)
	if err != nil {
		return nil, err
	}

	if err := stmt.Select(&vectorChannels, playlistId); err != nil {
		return nil, err
	}

	return vectorChannels, nil
}

func (q *VectorQueries) GetPlaylistChannelVector(channel_id int64) (*models.PlaylistChannelVector, error) {
	vectorChannel := &models.PlaylistChannelVector{}

	stmt, err := q.GetPreparedStmt(selectPlaylistChannelVectorQuery)
	if err != nil {
		return nil, err
	}

	err = stmt.Get(vectorChannel, channel_id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return vectorChannel, nil
}

func (q *VectorQueries) CreatePlaylistChannelVector(channelVector *models.PlaylistChannelVector) (int64, error) {
	stmt, err := q.GetPreparedStmt(insertPlaylistChannelVectorQuery)
	if err != nil {
		return 0, err
	}

	res, err := stmt.Exec(channelVector.ChannelId, channelVector.VectorId)
	if err != nil {
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
	stmt, err := q.GetPreparedStmt(updatePlaylistChannelVectorQuery)
	if err != nil {
		return err
	}

	_, err = stmt.Exec(channelVector.VectorId, channelVector.ChannelId)
	return err
}

func (q *VectorQueries) GetChannelVector(id int64) (*models.ChannelVector, error) {
	vectorChannel := &models.ChannelVector{}

	stmt, err := q.GetPreparedStmt(selectChannelVectorQuery)
	if err != nil {
		return nil, err
	}

	err = stmt.Get(vectorChannel, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return vectorChannel, nil
}

func (q *VectorQueries) GetChannelVectorByName(name string) (*models.ChannelVector, error) {
	vectorChannel := &models.ChannelVector{}

	stmt, err := q.GetPreparedStmt(selectChannelVectorByNameQuery)
	if err != nil {
		return nil, err
	}

	err = stmt.Get(vectorChannel, name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return vectorChannel, nil
}

func (q *VectorQueries) CreateChannelVector(channelVector *models.ChannelVector) (int64, error) {
	stmt, err := q.GetPreparedStmt(insertChannelVectorQuery)
	if err != nil {
		return 0, err
	}

	res, err := stmt.Exec(channelVector.Name, channelVector.Vector)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		log.Warn().Msgf("Error retrieving the ID: %v", err)
		return 0, err
	}

	return id, nil
}
