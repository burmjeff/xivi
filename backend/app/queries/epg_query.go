package queries

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/dbutils"
	"xivi/backend/platform/settings"

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
	updateEpgChannelQuery            = `UPDATE epgchannel SET channelid = ?, displayname = ?, "icon.src" = ? WHERE id = ?`
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

	// Batch insert templates
	batchInsertEpgProgrammeTemplate = `
		INSERT INTO epgprogramme
		(start, stop, channel, "title.value", "title.lang", subtitle, desc, categories, "icon.src",
		directors, presenters, producers, actors, "episodenumber.system", "episodenumber.value",
		"rating.system", "rating.value", "video.quality", date)
		VALUES
	`

	batchInsertEpgChannelTemplate = `
		INSERT INTO epgchannel
		(channelid, displayname, "icon.src")
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
	// Use a direct query approach to avoid mapping issues
	query := `
		SELECT id, channelid, displayname, "icon.src"
		FROM epgchannel
		ORDER BY channelid ASC
	`

	// Execute the query with timing
	start := time.Now()
	rows, err := q.QueryContext(ctx, query)
	duration := time.Since(start)

	// Log slow queries for performance monitoring
	if duration > 100*time.Millisecond {
		log.Debug().
			Float64("duration_ms", float64(duration.Milliseconds())).
			Msg("slow GetEpgChannels query")
	}

	if err != nil {
		log.Error().Err(err).Msg("Error retrieving EPG channels")
		return nil, err
	}
	defer rows.Close()

	// Create a slice to hold the results
	epgchannels := &[]models.EpgChannel{}

	// Process each row
	for rows.Next() {
		// Initialize a channel with default values for nested structs
		epgchannel := models.EpgChannel{
			Icon: models.Icon{},
		}

		// Variables to hold the database values
		var (
			id          int64
			channelid   string
			displayname string
			iconSrc     sql.NullString
		)

		// Scan the row into variables
		err := rows.Scan(
			&id, &channelid, &displayname, &iconSrc,
		)

		if err != nil {
			log.Error().Err(err).Msg("Error scanning EPG channel row")
			continue
		}

		// Populate the channel struct
		epgchannel.ID = id
		epgchannel.ChannelId = channelid
		epgchannel.DisplayName = displayname

		// Handle nullable fields
		if iconSrc.Valid {
			epgchannel.Icon.Src = iconSrc.String
		}

		// Add the channel to the results
		*epgchannels = append(*epgchannels, epgchannel)
	}

	// Check for errors from iterating over rows
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Msg("Error iterating over EPG channel rows")
		return nil, err
	}

	// Log the number of channels found
	log.Debug().Int("count", len(*epgchannels)).Msg("Retrieved EPG channels")

	return epgchannels, nil
}

// GetEpgChannel gets an EPG channel by ID
func (q *EpgQueries) GetEpgChannel(ctx context.Context, id int64) (*models.EpgChannel, error) {
	// Use a direct query approach to avoid mapping issues
	query := `
		SELECT id, channelid, displayname, "icon.src"
		FROM epgchannel
		WHERE id = ? LIMIT 1
	`

	// Execute the query with timing
	start := time.Now()
	row := q.QueryRowContext(ctx, query, id)
	duration := time.Since(start)

	// Log slow queries for performance monitoring
	if duration > 50*time.Millisecond {
		log.Debug().
			Int64("id", id).
			Float64("duration_ms", float64(duration.Milliseconds())).
			Msg("slow GetEpgChannel query")
	}

	// Initialize the channel with default values for nested structs
	epgchannel := &models.EpgChannel{
		Icon: models.Icon{},
	}

	// Variables to hold the database values
	var (
		channelid   string
		displayname string
		iconSrc     sql.NullString
	)

	// Scan the row into variables
	err := row.Scan(
		&id, &channelid, &displayname, &iconSrc,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		log.Error().Err(err).Int64("id", id).Msg("Error retrieving EPG channel")
		return nil, err
	}

	// Populate the channel struct
	epgchannel.ID = id
	epgchannel.ChannelId = channelid
	epgchannel.DisplayName = displayname

	// Handle nullable fields
	if iconSrc.Valid {
		epgchannel.Icon.Src = iconSrc.String
	}

	// Debug log to check if Icon.Src is populated
	log.Debug().Int64("id", id).Str("icon.src", epgchannel.Icon.Src).Msg("Retrieved EPG channel by ID")

	return epgchannel, nil
}

// GetEpgChannelByChannelId gets an EPG channel by channel ID
func (q *EpgQueries) GetEpgChannelByChannelId(ctx context.Context, channelID string) (*models.EpgChannel, error) {
	// Use a direct query approach to avoid mapping issues
	query := `
		SELECT id, channelid, displayname, "icon.src"
		FROM epgchannel
		WHERE channelid = ? LIMIT 1
	`

	// Execute the query with timing
	start := time.Now()
	row := q.QueryRowContext(ctx, query, channelID)
	duration := time.Since(start)

	// Log slow queries for performance monitoring
	if duration > 50*time.Millisecond {
		log.Debug().
			Str("channelID", channelID).
			Float64("duration_ms", float64(duration.Milliseconds())).
			Msg("slow GetEpgChannelByChannelId query")
	}

	// Initialize the channel with default values for nested structs
	epgchannel := &models.EpgChannel{
		Icon: models.Icon{},
	}

	// Variables to hold the database values
	var (
		id          int64
		channelid   string
		displayname string
		iconSrc     sql.NullString
	)

	// Scan the row into variables
	err := row.Scan(
		&id, &channelid, &displayname, &iconSrc,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		log.Error().Err(err).Str("channelID", channelID).Msg("Error retrieving EPG channel by channel ID")
		return nil, err
	}

	// Populate the channel struct
	epgchannel.ID = id
	epgchannel.ChannelId = channelid
	epgchannel.DisplayName = displayname

	// Handle nullable fields
	if iconSrc.Valid {
		epgchannel.Icon.Src = iconSrc.String
	}

	// Debug log to check if Icon.Src is populated
	log.Debug().Str("channelID", channelID).Str("icon.src", epgchannel.Icon.Src).Msg("Retrieved EPG channel by channel ID")

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
	if len(channels) == 0 {
		return nil
	}

	// Use optimized batch insert if there are many channels
	if len(channels) > 10 {
		return q.batchInsertEpgChannels(ctx, channels)
	}

	// Use prepared statement for smaller batches
	return q.WithTransaction(func(tx *sqlx.Tx) error {
		stmt, err := tx.PreparexContext(ctx, insertEpgChannelQuery)
		if err != nil {
			return err
		}
		defer stmt.Close()

		var lastError error
		for _, channel := range channels {
			_, err := stmt.ExecContext(ctx, channel.ChannelId, channel.DisplayName, channel.Icon.Src)
			if err != nil {
				// Check if it's a UNIQUE constraint violation
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					// Log the error but continue processing other channels
					log.Debug().Str("channelId", channel.ChannelId).Msg("Channel already exists, skipping")
					lastError = err
				} else {
					// For other errors, return immediately
					return err
				}
			}
		}

		// If we had any UNIQUE constraint violations, return the last one
		// This allows the caller to know there were issues but still continue
		return lastError
	})
}

// UpdateEpgChannel updates an existing EPG channel
func (q *EpgQueries) UpdateEpgChannel(ctx context.Context, id int64, p *models.EpgChannel) error {
	updateEpgChannelQuery := `
		UPDATE epgchannel SET
		channelid = ?, displayname = ?, "icon.src" = ?
		WHERE id = ?
	`

	return q.WithContext(ctx, func(ctx context.Context) error {
		// Execute the update query
		_, err := q.ExecContext(ctx, updateEpgChannelQuery,
			p.ChannelId,
			p.DisplayName,
			p.Icon.Src,
			id)

		if err != nil {
			log.Error().Err(err).Int64("id", id).Str("channelId", p.ChannelId).Msg("Error updating EPG channel")
			return err
		}

		return nil
	})
}

// batchInsertEpgChannels inserts multiple EPG channels using a single SQL statement
func (q *EpgQueries) batchInsertEpgChannels(ctx context.Context, channels []models.EpgChannel) error {
	return q.WithTransaction(func(tx *sqlx.Tx) error {
		// Build batch query with OR IGNORE to handle duplicates gracefully
		query := `
			INSERT OR IGNORE INTO epgchannel
			(channelid, displayname, "icon.src")
			VALUES
		`

		valueStrings := make([]string, 0, len(channels))
		valueArgs := make([]interface{}, 0, len(channels)*3)

		for _, channel := range channels {
			valueStrings = append(valueStrings, "(?, ?, ?)")
			valueArgs = append(valueArgs,
				channel.ChannelId,
				channel.DisplayName,
				channel.Icon.Src)
		}

		// Complete the query
		query += strings.Join(valueStrings, ",")

		// Execute the batch insert
		res, err := tx.ExecContext(ctx, query, valueArgs...)
		if err != nil {
			log.Error().Err(err).Msg("Error in batch insert of EPG channels")
			return err
		}

		// Check how many rows were actually inserted
		rowsAffected, _ := res.RowsAffected()
		if rowsAffected < int64(len(channels)) {
			log.Info().Int64("inserted", rowsAffected).Int("total", len(channels)).Msg("Some EPG channels already existed and were skipped")
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
	// Use a direct query approach to avoid mapping issues
	query := `
		SELECT id, start, stop, channel,
		"title.value", "title.lang", subtitle, desc, categories,
		"icon.src", directors, presenters, producers, actors,
		"episodenumber.system", "episodenumber.value",
		"rating.system", "rating.value", "video.quality", date
		FROM epgprogramme
		WHERE channel = ?
		ORDER BY start ASC
	`

	// Execute the query with timing
	start := time.Now()
	rows, err := q.QueryContext(ctx, query, tvgid)
	duration := time.Since(start)

	// Log performance metrics for large result sets
	if duration > 100*time.Millisecond {
		log.Debug().
			Str("tvgid", tvgid).
			Float64("duration_ms", float64(duration.Milliseconds())).
			Msg("slow GetProgrammesBytvgid query")
	}

	if err != nil {
		log.Error().Err(err).Str("tvgid", tvgid).Msg("Error retrieving programmes by tvg ID")
		return nil, err
	}
	defer rows.Close()

	// Create a slice to hold the results
	programmes := &[]models.EpgProgramme{}

	// Process each row
	for rows.Next() {
		// Initialize a programme with default values for nested structs
		programme := models.EpgProgramme{
			Title:         models.Title{Value: "No Title"},
			Icon:          models.Icon{},
			EpisodeNumber: models.EpisodeNumber{},
			Rating:        models.Rating{},
			Video:         models.Video{},
			Categories:    []string{},
			Directors:     []string{},
			Presenters:    []string{},
			Producers:     []string{},
			Actors:        []string{},
		}

		// Variables to hold the database values
		var (
			id               int64
			startTime        time.Time
			stopTime         time.Time
			channel          string
			titleValue       sql.NullString
			titleLang        sql.NullString
			subtitle         sql.NullString
			desc             sql.NullString
			categories       sql.NullString
			iconSrc          sql.NullString
			directors        sql.NullString
			presenters       sql.NullString
			producers        sql.NullString
			actors           sql.NullString
			episodeNumSystem sql.NullString
			episodeNumValue  sql.NullString
			ratingSystem     sql.NullString
			ratingValue      sql.NullString
			videoQuality     sql.NullString
			date             sql.NullString
		)

		// Scan the row into variables
		err := rows.Scan(
			&id, &startTime, &stopTime, &channel,
			&titleValue, &titleLang, &subtitle, &desc, &categories,
			&iconSrc, &directors, &presenters, &producers, &actors,
			&episodeNumSystem, &episodeNumValue,
			&ratingSystem, &ratingValue, &videoQuality, &date,
		)

		if err != nil {
			log.Error().Err(err).Str("tvgid", tvgid).Msg("Error scanning programme row")
			continue
		}

		// Populate the programme struct
		programme.ID = id
		programme.Start = &models.Time{Time: startTime}
		programme.Stop = &models.Time{Time: stopTime}
		programme.Channel = channel

		// Handle nullable fields
		if titleValue.Valid {
			programme.Title.Value = titleValue.String
		}
		if titleLang.Valid {
			programme.Title.Lang = titleLang.String
		}
		if subtitle.Valid {
			programme.Subtitle = subtitle.String
		}
		if desc.Valid {
			programme.Desc = desc.String
		}
		if categories.Valid && categories.String != "" {
			programme.Categories = strings.Split(categories.String, ",")
		}
		if iconSrc.Valid {
			programme.Icon.Src = iconSrc.String
		}
		if directors.Valid && directors.String != "" {
			programme.Directors = strings.Split(directors.String, ",")
		}
		if presenters.Valid && presenters.String != "" {
			programme.Presenters = strings.Split(presenters.String, ",")
		}
		if producers.Valid && producers.String != "" {
			programme.Producers = strings.Split(producers.String, ",")
		}
		if actors.Valid && actors.String != "" {
			programme.Actors = strings.Split(actors.String, ",")
		}
		if episodeNumSystem.Valid {
			programme.EpisodeNumber.System = episodeNumSystem.String
		}
		if episodeNumValue.Valid {
			programme.EpisodeNumber.Value = episodeNumValue.String
		}
		if ratingSystem.Valid {
			programme.Rating.System = ratingSystem.String
		}
		if ratingValue.Valid {
			programme.Rating.Value = ratingValue.String
		}
		if videoQuality.Valid {
			programme.Video.Quality = videoQuality.String
		}
		if date.Valid {
			programme.Date = date.String
		}

		// Add the programme to the results
		*programmes = append(*programmes, programme)
	}

	// Check for errors from iterating over rows
	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("tvgid", tvgid).Msg("Error iterating over programme rows")
		return nil, err
	}

	// Log the number of programmes found
	log.Debug().Str("tvgid", tvgid).Int("count", len(*programmes)).Msg("Retrieved programmes by tvg ID")

	return programmes, nil
}

// GetProgrammeByTime gets an EPG programme by time
func (q *EpgQueries) GetProgrammeByTime(ctx context.Context, tvgid string, epgTime time.Time) (*models.EpgProgramme, error) {
	// Note: A caching mechanism could be implemented here to avoid repeated lookups
	// for the same programme using a key like: tvgid + "_" + epgTime.Format(time.RFC3339)

	// Convert the input time to the application's configured timezone
	localTime, err := time.LoadLocation(settings.APP_SETTINGS.Application.TZ)
	if err != nil {
		localTime = time.UTC
		log.Warn().Err(err).Msg("Failed to load timezone, using UTC")
	}

	// Ensure the query time is in the configured timezone
	localEpgTime := epgTime.In(localTime)
	log.Debug().Str("tvgid", tvgid).Time("originalTime", epgTime).Time("localizedTime", localEpgTime).Msg("Querying programme with localized time")

	// Use a direct query approach to avoid mapping issues
	query := `
		SELECT id, start, stop, channel,
		"title.value", "title.lang", subtitle, desc, categories,
		"icon.src", directors, presenters, producers, actors,
		"episodenumber.system", "episodenumber.value",
		"rating.system", "rating.value", "video.quality", date
		FROM epgprogramme
		WHERE channel = ? AND start <= ? AND stop > ?
		ORDER BY start ASC LIMIT 1
	`

	// Execute the query with timing
	start := time.Now()
	row := q.QueryRowContext(ctx, query, tvgid, localEpgTime, localEpgTime)
	duration := time.Since(start)

	// Log slow queries for performance monitoring
	if duration > 50*time.Millisecond {
		log.Debug().
			Str("tvgid", tvgid).
			Time("epgTime", localEpgTime).
			Float64("duration_ms", float64(duration.Milliseconds())).
			Msg("slow GetProgrammeByTime query")
	}

	// Initialize the programme with default values for nested structs
	programme := &models.EpgProgramme{
		Title:         models.Title{Value: "No Title"},
		Icon:          models.Icon{},
		EpisodeNumber: models.EpisodeNumber{},
		Rating:        models.Rating{},
		Video:         models.Video{},
		Categories:    []string{},
		Directors:     []string{},
		Presenters:    []string{},
		Producers:     []string{},
		Actors:        []string{},
	}

	// Variables to hold the database values
	var (
		id               int64
		startTime        time.Time
		stopTime         time.Time
		channel          string
		titleValue       sql.NullString
		titleLang        sql.NullString
		subtitle         sql.NullString
		desc             sql.NullString
		categories       sql.NullString
		iconSrc          sql.NullString
		directors        sql.NullString
		presenters       sql.NullString
		producers        sql.NullString
		actors           sql.NullString
		episodeNumSystem sql.NullString
		episodeNumValue  sql.NullString
		ratingSystem     sql.NullString
		ratingValue      sql.NullString
		videoQuality     sql.NullString
		date             sql.NullString
	)

	// Scan the row into variables
	err = row.Scan(
		&id, &startTime, &stopTime, &channel,
		&titleValue, &titleLang, &subtitle, &desc, &categories,
		&iconSrc, &directors, &presenters, &producers, &actors,
		&episodeNumSystem, &episodeNumValue,
		&ratingSystem, &ratingValue, &videoQuality, &date,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		log.Error().Err(err).Str("tvgid", tvgid).Time("epgTime", epgTime).Msg("Error retrieving programme by time")
		return nil, err
	}

	// Populate the programme struct
	programme.ID = id
	programme.Start = &models.Time{Time: startTime}
	programme.Stop = &models.Time{Time: stopTime}
	programme.Channel = channel

	// Handle nullable fields
	if titleValue.Valid {
		programme.Title.Value = titleValue.String
	}
	if titleLang.Valid {
		programme.Title.Lang = titleLang.String
	}
	if subtitle.Valid {
		programme.Subtitle = subtitle.String
	}
	if desc.Valid {
		programme.Desc = desc.String
	}
	if categories.Valid && categories.String != "" {
		programme.Categories = strings.Split(categories.String, ",")
	}
	if iconSrc.Valid {
		programme.Icon.Src = iconSrc.String
	}
	if directors.Valid && directors.String != "" {
		programme.Directors = strings.Split(directors.String, ",")
	}
	if presenters.Valid && presenters.String != "" {
		programme.Presenters = strings.Split(presenters.String, ",")
	}
	if producers.Valid && producers.String != "" {
		programme.Producers = strings.Split(producers.String, ",")
	}
	if actors.Valid && actors.String != "" {
		programme.Actors = strings.Split(actors.String, ",")
	}
	if episodeNumSystem.Valid {
		programme.EpisodeNumber.System = episodeNumSystem.String
	}
	if episodeNumValue.Valid {
		programme.EpisodeNumber.Value = episodeNumValue.String
	}
	if ratingSystem.Valid {
		programme.Rating.System = ratingSystem.String
	}
	if ratingValue.Valid {
		programme.Rating.Value = ratingValue.String
	}
	if videoQuality.Valid {
		programme.Video.Quality = videoQuality.String
	}
	if date.Valid {
		programme.Date = date.String
	}

	// Debug log to check if Title.Value is populated
	log.Debug().Str("tvgid", tvgid).Str("title.value", programme.Title.Value).Msg("Retrieved programme by time")

	return programme, nil
}

// CreateEpgProgramme creates a new EPG programme
func (q *EpgQueries) CreateEpgProgramme(ctx context.Context, p models.EpgProgramme) (int64, error) {
	var id int64

	// Log the Title.Value to help debug
	log.Debug().Str("title.value", p.Title.Value).Msg("Creating EPG programme")

	err := q.WithContext(ctx, func(ctx context.Context) error {
		q.MapperFunc(dbutils.CustomMapper)
		stmt, err := q.GetPreparedStmt(insertEpgProgrammeQuery)
		if err != nil {
			return err
		}

		// Ensure Title.Value is not empty
		if p.Title.Value == "" {
			p.Title.Value = "No Title"
			log.Warn().Str("channel", p.Channel).Msg("Empty Title.Value in EPG programme, using default")
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
		start = ?, stop = ?, channel = ?, "title.value" = ?, "title.lang" = ?,
		subtitle = ?, desc = ?, categories = ?, "icon.src" = ?,
		directors = ?, presenters = ?, producers = ?, actors = ?,
		"episodenumber.system" = ?, "episodenumber.value" = ?, "rating.system" = ?,
		"rating.value" = ?, "video.quality" = ?, date = ?
		WHERE id = ?
	`

	// Log the Title.Value to help debug
	log.Debug().Str("title.value", p.Title.Value).Int64("id", id).Msg("Updating EPG programme")

	err := q.WithContext(ctx, func(ctx context.Context) error {
		q.MapperFunc(dbutils.CustomMapper)

		// Ensure Title.Value is not empty
		if p.Title.Value == "" {
			p.Title.Value = "No Title"
			log.Warn().Str("channel", p.Channel).Int64("id", id).Msg("Empty Title.Value in EPG programme update, using default")
		}

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
