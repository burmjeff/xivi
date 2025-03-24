package queries

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"xivi/backend/app/models"
	utils "xivi/backend/pkg/dbutils"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// SQL query constants
const (
	selectAllEpgsQuery = `SELECT * FROM epg`
	selectEpgByIdQuery = `SELECT * FROM epg WHERE id = ?`
	insertEpgQuery     = `INSERT INTO epg VALUES (null, ?, ?, ?, ?, ?)`
	updateEpgQuery     = `UPDATE epg SET name = ?, url = ?, orderr = ?, updated_at = ? WHERE id = ?`
	deleteEpgQuery     = `DELETE FROM epg WHERE id = ?`

	selectAllEpgChannelsQuery        = `SELECT * FROM epgchannel`
	selectEpgChannelByIdQuery        = `SELECT * FROM epgchannel WHERE id = ?`
	selectEpgChannelByChannelIdQuery = `SELECT * FROM epgchannel WHERE channelid = ? LIMIT 1`
	insertEpgChannelQuery            = `INSERT INTO epgchannel VALUES (null, ?, ?, ?)`
	updateEpgChannelQuery            = `UPDATE epgchannel SET channelid = ?, displayname = ?, icon = ? WHERE id = ?`
	deleteEpgChannelQuery            = `DELETE FROM epgchannel WHERE id = ?`

	selectAllEpgProgrammesQuery     = `SELECT * FROM epgprogramme`
	selectEpgProgrammesByTvgidQuery = `SELECT * FROM epgprogramme WHERE channel = ?`
	selectEpgProgrammeByTimeQuery   = `
		SELECT * FROM epgprogramme 
		WHERE channel = ? AND start <= ? AND stop > ? 
		LIMIT 1
		-- FORCE INDEX (idx_epgprogramme_channel_time) -- Hint to use index if available
	`
	insertEpgProgrammeQuery = `
		INSERT INTO epgprogramme VALUES (null, ?, ?, ?, ?, ?, ?, ?, ?, 
		?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	deleteEpgProgrammeQuery = `DELETE FROM epgprogramme WHERE id = ?`

	selectEpgChannelItemsByChannelQuery   = `SELECT * FROM epgchannelitem WHERE channel_id = ?`
	selectEpgChannelItemsByProgrammeQuery = `SELECT * FROM epgchannelitem WHERE epg_programme_id = ?`
	selectEpgChannelItemByIdQuery         = `SELECT * FROM epgchannelitem WHERE id = ?`
	insertEpgChannelItemQuery             = `INSERT INTO epgchannelitem VALUES (null, ?, ?, ?)`
	updateEpgChannelItemQuery             = `UPDATE epgchannelitem SET epg_channel_id = ?, epg_programme_id = ? WHERE id = ?`
	deleteEpgChannelItemQuery             = `DELETE FROM epgchannelitem WHERE id = ?`

	selectEpgTvgidsQuery = `SELECT channelid FROM epgchannel WHERE channelid is NOT NULL`

	// Batch insert template for EPG programmes
	batchInsertEpgProgrammeTemplate = `
		INSERT INTO epgprogramme 
		(start, stop, channel, title, lang, subtitle, desc, categories, icon, 
		directors, presenters, producers, actors, episodesystem, episodenum, 
		ratingsystem, ratingvalue, videoquality, date)
		VALUES 
	`
)

// EpgQueries struct for queries from Epg model.
type EpgQueries struct {
	BaseQueries
}

type Res struct {
	Data []string
}

// NewEpgQueries creates a new EpgQueries instance
func NewEpgQueries(db *sqlx.DB) *EpgQueries {
	return &EpgQueries{
		BaseQueries: NewBaseQueries(db),
	}
}

// GetEpgs gets all EPG sources
func (q *EpgQueries) GetEpgs(ctx context.Context) (*[]models.Epg, error) {
	epgs := &[]models.Epg{}

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectAllEpgsQuery)
		if err != nil {
			return err
		}

		return stmt.SelectContext(ctx, epgs)
	})

	if err != nil {
		log.Error().Err(err).Msg("Error retrieving EPG sources")
		return nil, err
	}

	return epgs, nil
}

// GetEpg gets an EPG source by ID
func (q *EpgQueries) GetEpg(ctx context.Context, id int64) (*models.Epg, error) {
	epg := &models.Epg{}

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectEpgByIdQuery)
		if err != nil {
			return err
		}

		return stmt.GetContext(ctx, epg, id)
	})

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		log.Error().Err(err).Int64("id", id).Msg("Error retrieving EPG")
		return nil, err
	}

	return epg, nil
}

// CreateEpg creates a new EPG source
func (q *EpgQueries) CreateEpg(ctx context.Context, p *models.Epg) (int64, error) {
	var id int64

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(insertEpgQuery)
		if err != nil {
			return err
		}

		res, err := stmt.ExecContext(ctx, p.Name, p.URL, p.Order, p.CreatedAt, p.UpdatedAt)
		if err != nil {
			return err
		}

		id, err = res.LastInsertId()
		if err != nil {
			log.Warn().Err(err).Msg("Error retrieving the ID")
			return err
		}

		return nil
	})

	if err != nil {
		return 0, err
	}

	return id, nil
}

// UpdateEpg updates an EPG source
func (q *EpgQueries) UpdateEpg(ctx context.Context, id int64, p *models.Epg) error {
	return q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(updateEpgQuery)
		if err != nil {
			return err
		}

		_, err = stmt.ExecContext(ctx, p.Name, p.URL, p.Order, p.UpdatedAt, id)
		if err != nil {
			log.Error().Err(err).Int64("id", id).Msg("Error updating EPG")
		}
		return err
	})
}

// DeleteEpg deletes an EPG source
func (q *EpgQueries) DeleteEpg(ctx context.Context, id int64) error {
	return q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(deleteEpgQuery)
		if err != nil {
			return err
		}

		_, err = stmt.ExecContext(ctx, id)
		if err != nil {
			log.Error().Err(err).Int64("id", id).Msg("Error deleting EPG")
		}
		return err
	})
}

// GetEpgChannels gets all EPG channels
func (q *EpgQueries) GetEpgChannels(ctx context.Context) (*[]models.EpgChannel, error) {
	epgchannel := &[]models.EpgChannel{}

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectAllEpgChannelsQuery)
		if err != nil {
			return err
		}

		return stmt.SelectContext(ctx, epgchannel)
	})

	if err != nil {
		log.Error().Err(err).Msg("Error retrieving EPG channels")
		return nil, err
	}

	return epgchannel, nil
}

// GetEpgChannel gets an EPG channel by ID
func (q *EpgQueries) GetEpgChannel(ctx context.Context, id int64) (*models.EpgChannel, error) {
	epgchannel := &models.EpgChannel{}

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectEpgChannelByIdQuery)
		if err != nil {
			return err
		}

		return stmt.GetContext(ctx, epgchannel, id)
	})

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		log.Error().Err(err).Int64("id", id).Msg("Error retrieving EPG channel")
		return nil, err
	}

	return epgchannel, nil
}

// GetEpgChannelByChannelId gets an EPG channel by channel ID
func (q *EpgQueries) GetEpgChannelByChannelId(ctx context.Context, channelID string) (*models.EpgChannel, error) {
	epgchannel := &models.EpgChannel{}

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectEpgChannelByChannelIdQuery)
		if err != nil {
			return err
		}

		return stmt.GetContext(ctx, epgchannel, channelID)
	})

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		log.Error().Err(err).Str("channelID", channelID).Msg("Error retrieving EPG channel by channel ID")
		return nil, err
	}

	return epgchannel, nil
}

// CreateEpgChannel creates a new EPG channel
func (q *EpgQueries) CreateEpgChannel(ctx context.Context, p models.EpgChannel) (int64, error) {
	var id int64

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(insertEpgChannelQuery)
		if err != nil {
			return err
		}

		res, err := stmt.ExecContext(ctx, p.ChannelId, p.DisplayName, p.Icon.Src)
		if err != nil {
			return err
		}

		id, err = res.LastInsertId()
		if err != nil {
			log.Warn().Err(err).Msg("Error retrieving the ID")
			return err
		}

		return nil
	})

	if err != nil {
		return 0, err
	}

	return id, nil
}

// BatchCreateEpgChannels creates multiple EPG channels in a batch
func (q *EpgQueries) BatchCreateEpgChannels(ctx context.Context, channels []models.EpgChannel) error {
	return q.WithTransaction(func(tx *sqlx.Tx) error {
		stmt, err := tx.PreparexContext(ctx, insertEpgChannelQuery)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, channel := range channels {
			_, err := stmt.ExecContext(ctx, channel.ChannelId, channel.DisplayName, channel.Icon.Src)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// CreateEpgProgrammes creates multiple EPG programmes efficiently
func (q *EpgQueries) BatchCreateEpgProgrammes(ctx context.Context, programmes []models.EpgProgramme, batchSize int) error {
	if len(programmes) == 0 {
		return nil
	}

	// Default batch size if not specified
	if batchSize <= 0 {
		batchSize = 100
	}

	return q.WithTransaction(func(tx *sqlx.Tx) error {
		// Process in batches
		for i := 0; i < len(programmes); i += batchSize {
			end := i + batchSize
			if end > len(programmes) {
				end = len(programmes)
			}

			batch := programmes[i:end]

			// Build batch query
			query := batchInsertEpgProgrammeTemplate

			valueStrings := make([]string, 0, len(batch))
			valueArgs := make([]interface{}, 0, len(batch)*19)

			for _, prog := range batch {
				valueStrings = append(valueStrings, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")

				valueArgs = append(valueArgs,
					prog.Start,
					prog.Stop,
					prog.Channel,
					prog.Title.Value,
					prog.Title.Lang,
					prog.Subtitle,
					prog.Desc,
					strings.Join(prog.Categories, ","),
					prog.Icon.Src,
					strings.Join(prog.Directors, ","),
					strings.Join(prog.Presenters, ","),
					strings.Join(prog.Producers, ","),
					strings.Join(prog.Actors, ","),
					prog.EpisodeNumber.System,
					prog.EpisodeNumber.Value,
					prog.Rating.System,
					prog.Rating.Value,
					prog.Video.Quality,
					prog.Date)
			}

			query += strings.Join(valueStrings, ",")

			// Execute batch insert
			_, err := tx.ExecContext(ctx, query, valueArgs...)
			if err != nil {
				log.Error().Err(err).
					Int("batch_size", len(batch)).
					Int("total_size", len(programmes)).
					Msg("Error batch inserting EPG programmes")
				return err
			}
		}

		return nil
	})
}

// GetProgrammesBytvgid gets EPG programmes by tvg ID
func (q *EpgQueries) GetProgrammesBytvgid(ctx context.Context, tvgid string) (*[]models.EpgProgramme, error) {
	programmes := &[]models.EpgProgramme{}

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectEpgProgrammesByTvgidQuery)
		if err != nil {
			return err
		}

		return stmt.SelectContext(ctx, programmes, tvgid)
	})

	if err != nil {
		log.Error().Err(err).Str("tvgid", tvgid).Msg("Error retrieving programmes by tvg ID")
		return nil, err
	}

	return programmes, nil
}

// GetProgrammeByTime gets an EPG programme by time
func (q *EpgQueries) GetProgrammeByTime(ctx context.Context, tvgid string, epgTime time.Time) (*models.EpgProgramme, error) {
	programme := &models.EpgProgramme{}

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectEpgProgrammeByTimeQuery)
		if err != nil {
			return err
		}

		start := time.Now()
		err = stmt.GetContext(ctx, programme, tvgid, epgTime, epgTime)
		duration := time.Since(start)

		if duration > 100*time.Millisecond {
			log.Debug().
				Str("tvgid", tvgid).
				Time("epgTime", epgTime).
				Float64("duration_ms", float64(duration.Milliseconds())).
				Msg("slow GetProgrammeByTime query")
		}

		return err
	})

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		log.Error().Err(err).Str("tvgid", tvgid).Time("epgTime", epgTime).Msg("Error retrieving programme by time")
		return nil, err
	}

	return programme, nil
}

// CreateEpgProgramme creates a new EPG programme
func (q *EpgQueries) CreateEpgProgramme(ctx context.Context, p models.EpgProgramme) (int64, error) {
	var id int64

	err := q.WithContext(ctx, func(ctx context.Context) error {
		q.MapperFunc(utils.CustomMapper)
		stmt, err := q.GetPreparedStmt(insertEpgProgrammeQuery)
		if err != nil {
			return err
		}

		res, err := stmt.ExecContext(ctx,
			p.Start,
			p.Stop,
			p.Channel,
			p.Title.Value,
			p.Title.Lang,
			p.Subtitle,
			p.Desc,
			strings.Join(p.Categories, ","),
			p.Icon.Src,
			strings.Join(p.Directors, ","),
			strings.Join(p.Presenters, ","),
			strings.Join(p.Producers, ","),
			strings.Join(p.Actors, ","),
			p.EpisodeNumber.System,
			p.EpisodeNumber.Value,
			p.Rating.System,
			p.Rating.Value,
			p.Video.Quality,
			p.Date)

		if err != nil {
			return err
		}

		id, err = res.LastInsertId()
		if err != nil {
			log.Warn().Err(err).Msg("Error retrieving the ID")
			return err
		}

		return nil
	})

	if err != nil {
		return 0, err
	}

	return id, nil
}

// UpdateEpgProgramme updates an existing EPG programme
func (q *EpgQueries) UpdateEpgProgramme(ctx context.Context, id int64, p *models.EpgProgramme) error {
	updateEpgProgrammeQuery := `
		UPDATE epgprogramme SET 
		start = ?, stop = ?, channel = ?, title = ?, lang = ?, 
		subtitle = ?, desc = ?, categories = ?, icon = ?, 
		directors = ?, presenters = ?, producers = ?, actors = ?, 
		episodesystem = ?, episodenum = ?, ratingsystem = ?, 
		ratingvalue = ?, videoquality = ?, date = ?
		WHERE id = ?
	`

	err := q.WithContext(ctx, func(ctx context.Context) error {
		q.MapperFunc(utils.CustomMapper)

		_, err := q.ExecContext(ctx, updateEpgProgrammeQuery,
			p.Start,
			p.Stop,
			p.Channel,
			p.Title.Value,
			p.Title.Lang,
			p.Subtitle,
			p.Desc,
			strings.Join(p.Categories, ","),
			p.Icon.Src,
			strings.Join(p.Directors, ","),
			strings.Join(p.Presenters, ","),
			strings.Join(p.Producers, ","),
			strings.Join(p.Actors, ","),
			p.EpisodeNumber.System,
			p.EpisodeNumber.Value,
			p.Rating.System,
			p.Rating.Value,
			p.Video.Quality,
			p.Date,
			id)

		if err != nil {
			log.Error().Err(err).Int64("id", id).Msg("Error updating EPG programme")
			return err
		}

		return nil
	})

	return err
}

// GetEpgTvgids gets all EPG TVG IDs
func (q *EpgQueries) GetEpgTvgids(ctx context.Context) ([]string, error) {
	var items []string

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectEpgTvgidsQuery)
		if err != nil {
			return err
		}

		return stmt.SelectContext(ctx, &items)
	})

	if err != nil {
		log.Error().Err(err).Msg("Error retrieving EPG tvg IDs")
		return nil, err
	}

	return items, nil
}
